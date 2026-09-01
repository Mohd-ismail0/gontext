package authn

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// OIDCConfig configures the OIDC IdentityProvider adapter.
type OIDCConfig struct {
	Issuer       string
	Audience     string
	DiscoveryURL string
	JWKSURL      string
	ClaimSubject string
	ClaimEmail   string
	ClaimGroups  string
	ClaimOrg     string
	// ClaimScopes is the primary OAuth scope claim (default "scope").
	// If empty, Authenticate also tries "scp" (Azure AD style).
	ClaimScopes  string
	HTTPClient   *http.Client
	CacheTTL     time.Duration
}

// OIDCProvider validates bearer JWTs via JWKS and maps claims to Principal.
type OIDCProvider struct {
	cfg    OIDCConfig
	client *http.Client

	mu        sync.RWMutex
	jwks      jwksDoc
	jwksURL   string
	meta      ports.OIDCMetadata
	fetchedAt time.Time
}

// DefaultHTTPClient returns an HTTP client suitable for OIDC discovery/JWKS fetches.
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// NewOIDC creates an OIDC identity adapter.
func NewOIDC(cfg OIDCConfig) *OIDCProvider {
	if cfg.ClaimSubject == "" {
		cfg.ClaimSubject = "sub"
	}
	if cfg.ClaimEmail == "" {
		cfg.ClaimEmail = "email"
	}
	if cfg.ClaimGroups == "" {
		cfg.ClaimGroups = "groups"
	}
	if cfg.ClaimOrg == "" {
		cfg.ClaimOrg = "org_id"
	}
	if cfg.ClaimScopes == "" {
		cfg.ClaimScopes = "scope"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 10 * time.Minute
	}
	client := cfg.HTTPClient
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &OIDCProvider{cfg: cfg, client: client, jwksURL: cfg.JWKSURL}
}

var _ ports.IdentityProvider = (*OIDCProvider)(nil)

// Discover loads OIDC metadata (cached).
func (p *OIDCProvider) Discover(ctx context.Context) (ports.OIDCMetadata, error) {
	if err := p.ensureMeta(ctx); err != nil {
		return ports.OIDCMetadata{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.meta, nil
}

// Authenticate validates a bearer JWT and maps issuer+sub to Principal.
func (p *OIDCProvider) Authenticate(ctx context.Context, credentials ports.Credentials) (ports.Principal, error) {
	token := strings.TrimSpace(credentials.BearerToken)
	if token == "" {
		return ports.Principal{}, platform.ErrUnauthorized("missing bearer token")
	}
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if err := p.ensureJWKS(ctx); err != nil {
		return ports.Principal{}, platform.ErrUnavailable("jwks unavailable: " + err.Error())
	}

	header, claims, err := parseAndVerifyJWT(token, func(kid, alg string) (any, error) {
		return p.keyFor(kid, alg)
	}, p.cfg.Issuer, p.cfg.Audience)
	_ = header
	if err != nil {
		return ports.Principal{}, platform.ErrUnauthorized(err.Error())
	}

	sub := claimString(claims, p.cfg.ClaimSubject)
	if sub == "" {
		return ports.Principal{}, platform.ErrUnauthorized("missing subject claim")
	}
	iss := claimString(claims, "iss")
	org := claimString(claims, p.cfg.ClaimOrg)
	email := claimString(claims, p.cfg.ClaimEmail)
	groups := claimStringSlice(claims, p.cfg.ClaimGroups)

	kind := ports.PrincipalKindUser
	if t := claimString(claims, "token_use"); t == "access" {
		// still a user/service access token; detect service via claim if present
	}
	if claimString(claims, "principal_kind") == "service" {
		kind = ports.PrincipalKindService
	}

	return ports.Principal{
		ID:      iss + "|" + sub,
		Kind:    kind,
		OrgID:   org,
		Issuer:  iss,
		Subject: sub,
		Email:   email,
		Groups:  groups,
		Roles:   claimStringSlice(claims, "roles"),
		Scopes:  claimScopes(claims, p.cfg.ClaimScopes),
	}, nil
}

// claimScopes extracts OAuth scopes from scope (space-delimited string) or scp (array/string).
func claimScopes(claims map[string]any, primary string) []string {
	if primary == "" {
		primary = "scope"
	}
	if scopes := normalizeScopes(claims[primary]); len(scopes) > 0 {
		return scopes
	}
	if primary != "scp" {
		if scopes := normalizeScopes(claims["scp"]); len(scopes) > 0 {
			return scopes
		}
	}
	return nil
}

func normalizeScopes(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		return uniqueNonEmpty(strings.Fields(t))
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, strings.Fields(s)...)
			}
		}
		return uniqueNonEmpty(out)
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			out = append(out, strings.Fields(s)...)
		}
		return uniqueNonEmpty(out)
	default:
		return nil
	}
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (p *OIDCProvider) keyFor(kid, alg string) (any, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, k := range p.jwks.Keys {
		if kid != "" && k.Kid != kid {
			continue
		}
		if alg != "" && k.Alg != "" && k.Alg != alg {
			continue
		}
		switch k.Kty {
		case "RSA":
			return k.rsaPublic()
		case "EC":
			return k.ecPublic()
		}
	}
	return nil, fmt.Errorf("no matching jwk for kid=%s alg=%s", kid, alg)
}

func (p *OIDCProvider) ensureMeta(ctx context.Context) error {
	p.mu.RLock()
	fresh := time.Since(p.fetchedAt) < p.cfg.CacheTTL && p.meta.Issuer != ""
	p.mu.RUnlock()
	if fresh {
		return nil
	}

	url := p.cfg.DiscoveryURL
	if url == "" && p.cfg.Issuer != "" {
		url = strings.TrimRight(p.cfg.Issuer, "/") + "/.well-known/openid-configuration"
	}
	if url == "" {
		// Synthetic metadata from static config.
		p.mu.Lock()
		p.meta = ports.OIDCMetadata{Issuer: p.cfg.Issuer, JWKSURI: p.cfg.JWKSURL}
		p.fetchedAt = time.Now()
		p.mu.Unlock()
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery status %d", res.StatusCode)
	}
	var meta ports.OIDCMetadata
	if err := json.NewDecoder(res.Body).Decode(&meta); err != nil {
		return err
	}
	p.mu.Lock()
	p.meta = meta
	if meta.JWKSURI != "" {
		p.jwksURL = meta.JWKSURI
	}
	p.fetchedAt = time.Now()
	p.mu.Unlock()
	return nil
}

func (p *OIDCProvider) ensureJWKS(ctx context.Context) error {
	_ = p.ensureMeta(ctx)
	p.mu.RLock()
	fresh := time.Since(p.fetchedAt) < p.cfg.CacheTTL && len(p.jwks.Keys) > 0
	url := p.jwksURL
	p.mu.RUnlock()
	if fresh {
		return nil
	}
	if url == "" {
		url = p.cfg.JWKSURL
	}
	if url == "" {
		return fmt.Errorf("jwks url not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", res.StatusCode)
	}
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	p.mu.Lock()
	p.jwks = doc
	p.jwksURL = url
	p.fetchedAt = time.Now()
	p.mu.Unlock()
	return nil
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) rsaPublic() (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	var eInt int
	for _, b := range eb {
		eInt = eInt<<8 + int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: eInt}, nil
}

func (k jwk) ecPublic() (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve %s", k.Crv)
	}
	xb, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

func parseAndVerifyJWT(token string, keyFn func(kid, alg string) (any, error), wantIss, wantAud string) (jwtHeader, map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtHeader{}, nil, fmt.Errorf("malformed jwt")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtHeader{}, nil, fmt.Errorf("invalid header encoding")
	}
	var header jwtHeader
	if err := json.Unmarshal(hb, &header); err != nil {
		return jwtHeader{}, nil, fmt.Errorf("invalid header json")
	}
	switch header.Alg {
	case "RS256", "ES256":
	default:
		return jwtHeader{}, nil, fmt.Errorf("unsupported alg %s", header.Alg)
	}

	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtHeader{}, nil, fmt.Errorf("invalid signature encoding")
	}
	pub, err := keyFn(header.Kid, header.Alg)
	if err != nil {
		return jwtHeader{}, nil, err
	}
	sum := sha256.Sum256([]byte(signingInput))
	switch header.Alg {
	case "RS256":
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return jwtHeader{}, nil, fmt.Errorf("rsa key required")
		}
		if err := rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, sum[:], sig); err != nil {
			return jwtHeader{}, nil, fmt.Errorf("invalid signature")
		}
	case "ES256":
		ecKey, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return jwtHeader{}, nil, fmt.Errorf("ec key required")
		}
		if len(sig) != 64 {
			return jwtHeader{}, nil, fmt.Errorf("invalid es256 signature length")
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(ecKey, sum[:], r, s) {
			return jwtHeader{}, nil, fmt.Errorf("invalid signature")
		}
	}

	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtHeader{}, nil, fmt.Errorf("invalid claims encoding")
	}
	claims := map[string]any{}
	if err := json.Unmarshal(cb, &claims); err != nil {
		return jwtHeader{}, nil, fmt.Errorf("invalid claims json")
	}

	now := time.Now().Unix()
	if exp, ok := claimInt64(claims, "exp"); ok && now >= exp {
		return jwtHeader{}, nil, fmt.Errorf("token expired")
	}
	if nbf, ok := claimInt64(claims, "nbf"); ok && now < nbf {
		return jwtHeader{}, nil, fmt.Errorf("token not yet valid")
	}
	if wantIss != "" {
		if iss := claimString(claims, "iss"); iss != wantIss {
			return jwtHeader{}, nil, fmt.Errorf("issuer mismatch")
		}
	}
	if wantAud != "" && !audienceContains(claims["aud"], wantAud) {
		return jwtHeader{}, nil, fmt.Errorf("audience mismatch")
	}
	return header, claims, nil
}

func audienceContains(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func claimString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func claimStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func claimInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
}
