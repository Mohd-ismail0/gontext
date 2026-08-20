package authn

import (
	"context"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// DefaultAPIKeyAudience is the audience bound into short-lived agent claims.
const DefaultAPIKeyAudience = "context-fabric"

// AgentClaims is a short-lived, audience-bound claims struct issued after API key
// resolution. It is intentionally not a full JWT library artifact—callers may
// serialize it as JSON or embed it in an internal signed envelope.
//
// Structure:
//
//	iss: "context-fabric/apikey"
//	sub: agent principal subject
//	aud: resource audience (e.g. "context-fabric")
//	org: organization id
//	agt: agent id
//	cid: credential id
//	iat / exp: issue and expiry unix seconds
//	scope: optional delegated action list
type AgentClaims struct {
	Issuer       string   `json:"iss"`
	Subject      string   `json:"sub"`
	Audience     string   `json:"aud"`
	OrgID        string   `json:"org"`
	AgentID      string   `json:"agt"`
	CredentialID string   `json:"cid"`
	IssuedAt     int64    `json:"iat"`
	ExpiresAt    int64    `json:"exp"`
	Scope        []string `json:"scope,omitempty"`
}

// APIKeyResolver resolves API keys via CredentialProvider and issues AgentClaims.
type APIKeyResolver struct {
	Credentials ports.CredentialProvider
	Audience    string
	TTL         time.Duration
	Now         func() time.Time
}

// NewAPIKeyResolver constructs an API key resolver.
func NewAPIKeyResolver(creds ports.CredentialProvider) *APIKeyResolver {
	return &APIKeyResolver{
		Credentials: creds,
		Audience:    DefaultAPIKeyAudience,
		TTL:         5 * time.Minute,
		Now:         time.Now,
	}
}

// Resolve authenticates an API key and returns the agent principal plus claims.
func (r *APIKeyResolver) Resolve(ctx context.Context, apiKey string) (ports.AgentPrincipal, AgentClaims, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return ports.AgentPrincipal{}, AgentClaims{}, platform.ErrUnauthorized("missing api key")
	}
	if r.Credentials == nil {
		return ports.AgentPrincipal{}, AgentClaims{}, platform.ErrUnavailable("credential provider not configured")
	}
	agent, err := r.Credentials.ResolveAPIKey(ctx, key)
	if err != nil {
		return ports.AgentPrincipal{}, AgentClaims{}, err
	}
	now := r.Now().UTC()
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	aud := r.Audience
	if aud == "" {
		aud = DefaultAPIKeyAudience
	}
	claims := AgentClaims{
		Issuer:       "context-fabric/apikey",
		Subject:      agent.Subject,
		Audience:     aud,
		OrgID:        agent.OrgID,
		AgentID:      agent.AgentID,
		CredentialID: agent.CredentialID,
		IssuedAt:     now.Unix(),
		ExpiresAt:    now.Add(ttl).Unix(),
	}
	return agent, claims, nil
}
