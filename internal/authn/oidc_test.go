package authn

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

func TestOIDCAuthenticateScopesAndOrg(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)

	p := NewOIDC(OIDCConfig{
		Issuer:   "https://issuer.example",
		Audience: "context-fabric",
		JWKSURL:  srv.URL,
	})

	token := signRS256(t, priv, kid, map[string]any{
		"iss":    "https://issuer.example",
		"aud":    "context-fabric",
		"sub":    "user-1",
		"org_id": "org-a",
		"scope":  "context:search context:read",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	prin, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: token})
	if err != nil {
		t.Fatal(err)
	}
	if prin.OrgID != "org-a" {
		t.Fatalf("org=%q", prin.OrgID)
	}
	if len(prin.Scopes) != 2 {
		t.Fatalf("scopes=%v", prin.Scopes)
	}
}

func TestOIDCAuthenticateScpClaim(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)

	p := NewOIDC(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "context-fabric", JWKSURL: srv.URL,
	})
	token := signRS256(t, priv, kid, map[string]any{
		"iss": "https://issuer.example", "aud": "context-fabric", "sub": "u",
		"org_id": "o1", "scp": []any{"context:search", "openid"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	prin, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: token})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range prin.Scopes {
		if s == "context:search" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected context:search in %v", prin.Scopes)
	}
}

func TestOIDCRejectsWrongIssuerAudience(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)

	p := NewOIDC(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "context-fabric", JWKSURL: srv.URL,
	})

	badIss := signRS256(t, priv, kid, map[string]any{
		"iss": "https://evil.example", "aud": "context-fabric", "sub": "u",
		"org_id": "o", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: badIss}); err == nil {
		t.Fatal("expected issuer mismatch")
	}

	badAud := signRS256(t, priv, kid, map[string]any{
		"iss": "https://issuer.example", "aud": "other-api", "sub": "u",
		"org_id": "o", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: badAud}); err == nil {
		t.Fatal("expected audience mismatch")
	}
}

func TestOIDCMissingOrgStillAuthenticates(t *testing.T) {
	priv, kid, srv := testJWKS(t)
	t.Cleanup(srv.Close)
	p := NewOIDC(OIDCConfig{
		Issuer: "https://issuer.example", Audience: "context-fabric", JWKSURL: srv.URL,
	})
	token := signRS256(t, priv, kid, map[string]any{
		"iss": "https://issuer.example", "aud": "context-fabric", "sub": "u",
		"scope": "context:search", "exp": time.Now().Add(time.Hour).Unix(),
	})
	prin, err := p.Authenticate(context.Background(), ports.Credentials{BearerToken: token})
	if err != nil {
		t.Fatal(err)
	}
	if prin.OrgID != "" {
		t.Fatalf("expected empty org, got %q", prin.OrgID)
	}
}

func testJWKS(t *testing.T) (*rsa.PrivateKey, string, *httptest.Server) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-kid"
	jwks := jwksDoc{Keys: []jwk{{
		Kty: "RSA", Kid: kid, Alg: "RS256", Use: "sig",
		N: base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
	}}}
	jwksRaw, _ := json.Marshal(jwks)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksRaw)
	}))
	return priv, kid, srv
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	cb, _ := json.Marshal(claims)
	h := base64.RawURLEncoding.EncodeToString(hb)
	c := base64.RawURLEncoding.EncodeToString(cb)
	input := h + "." + c
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}
