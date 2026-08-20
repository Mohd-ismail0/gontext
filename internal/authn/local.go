package authn

import (
	"context"
	"strings"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const localIssuer = "local"

// LocalProvider is a demo/test IdentityProvider.
// Bearer tokens of the form local:<org>:<sub>:<role> are accepted.
type LocalProvider struct{}

// NewLocal returns a local development identity adapter.
func NewLocal() *LocalProvider {
	return &LocalProvider{}
}

var _ ports.IdentityProvider = (*LocalProvider)(nil)

// Discover returns synthetic local OIDC metadata.
func (p *LocalProvider) Discover(_ context.Context) (ports.OIDCMetadata, error) {
	return ports.OIDCMetadata{
		Issuer:                localIssuer,
		AuthorizationEndpoint: "local://authorize",
		TokenEndpoint:         "local://token",
		JWKSURI:               "local://jwks",
		ScopesSupported:       []string{"openid", "profile"},
	}, nil
}

// Authenticate parses Bearer local:<org>:<sub>:<role>.
func (p *LocalProvider) Authenticate(_ context.Context, credentials ports.Credentials) (ports.Principal, error) {
	raw := strings.TrimSpace(credentials.BearerToken)
	raw = strings.TrimPrefix(raw, "Bearer ")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ports.Principal{}, platform.ErrUnauthorized("missing bearer token")
	}
	if !strings.HasPrefix(raw, "local:") {
		return ports.Principal{}, platform.ErrUnauthorized("not a local token")
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 4 {
		return ports.Principal{}, platform.ErrUnauthorized("local token must be local:<org>:<sub>:<role>")
	}
	org, sub, role := parts[1], parts[2], parts[3]
	if org == "" || sub == "" || role == "" {
		return ports.Principal{}, platform.ErrUnauthorized("local token fields must be non-empty")
	}
	kind := ports.PrincipalKindUser
	switch strings.ToLower(role) {
	case "agent":
		kind = ports.PrincipalKindAgent
	case "service":
		kind = ports.PrincipalKindService
	case "group":
		kind = ports.PrincipalKindGroup
	}
	return ports.Principal{
		ID:      localIssuer + "|" + sub,
		Kind:    kind,
		OrgID:   org,
		Issuer:  localIssuer,
		Subject: sub,
		Roles:   []string{role},
	}, nil
}
