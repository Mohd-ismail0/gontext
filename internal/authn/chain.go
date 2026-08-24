package authn

import (
	"context"
	"strings"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// DefaultAgentScopes is the OAuth scope ceiling for API-key agents.
var DefaultAgentScopes = []string{"context:search", "context:read"}

// Chained authenticates OIDC/local JWTs first, then agent API keys.
// API keys are never an authorization boundary beyond the stored principal's org and scopes.
type Chained struct {
	Primary ports.IdentityProvider
	Keys    *APIKeyResolver
}

// WithAPIKeys wraps primary with CredentialProvider-backed API key resolution.
func WithAPIKeys(primary ports.IdentityProvider, creds ports.CredentialProvider) ports.IdentityProvider {
	if creds == nil {
		return primary
	}
	return &Chained{Primary: primary, Keys: NewAPIKeyResolver(creds)}
}

var _ ports.IdentityProvider = (*Chained)(nil)

// Discover delegates to the primary provider.
func (c *Chained) Discover(ctx context.Context) (ports.OIDCMetadata, error) {
	if c.Primary != nil {
		return c.Primary.Discover(ctx)
	}
	return ports.OIDCMetadata{}, nil
}

// Authenticate accepts:
//  1. credentials.APIKey (explicit)
//  2. Bearer JWT via primary (OIDC)
//  3. Bearer API key (cfak_ / opaque) via CredentialProvider
//  4. Primary fallback (local tokens)
func (c *Chained) Authenticate(ctx context.Context, credentials ports.Credentials) (ports.Principal, error) {
	if key := strings.TrimSpace(credentials.APIKey); key != "" {
		return c.authenticateKey(ctx, key)
	}
	token := strings.TrimSpace(credentials.BearerToken)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if looksLikeJWT(token) {
		if c.Primary == nil {
			return ports.Principal{}, platform.ErrUnauthorized("identity provider not configured")
		}
		return c.Primary.Authenticate(ctx, credentials)
	}
	if c.Keys != nil && token != "" {
		prin, err := c.authenticateKey(ctx, token)
		if err == nil {
			return prin, nil
		}
	}
	if c.Primary != nil {
		return c.Primary.Authenticate(ctx, credentials)
	}
	return ports.Principal{}, platform.ErrUnauthorized("missing credentials")
}

func (c *Chained) authenticateKey(ctx context.Context, key string) (ports.Principal, error) {
	if c.Keys == nil {
		return ports.Principal{}, platform.ErrUnauthorized("invalid api key")
	}
	agent, _, err := c.Keys.Resolve(ctx, key)
	if err != nil {
		return ports.Principal{}, err
	}
	prin := agent.Principal
	if prin.Kind == "" {
		prin.Kind = ports.PrincipalKindAgent
	}
	if len(prin.Scopes) == 0 {
		prin.Scopes = append([]string{}, DefaultAgentScopes...)
	}
	return prin, nil
}

func looksLikeJWT(token string) bool {
	if token == "" || strings.HasPrefix(token, "local:") || strings.HasPrefix(token, "cfak_") {
		return false
	}
	return strings.Count(token, ".") == 2
}
