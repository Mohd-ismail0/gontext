package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"os"
	"strings"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// OAuth scope constants (RFC 9728 / MCP resource scopes).
const (
	ScopeSearch       = "context:search"
	ScopeRead         = "context:read"
	ScopeIngest       = "context:ingest"
	ScopeManagePolicy = "context:manage_policy"
	ScopeAuditRead    = "context:audit_read"
)

// PlatformBootstrapTokenEnv is the one-time token for first org bootstrap (non-demo).
const PlatformBootstrapTokenEnv = "PLATFORM_BOOTSTRAP_TOKEN"

func scopesOf(principal ports.Principal, extra []string) []string {
	if len(principal.Scopes) > 0 {
		return append([]string{}, principal.Scopes...)
	}
	return extra
}

func hasScope(scopes []string, required string) bool {
	for _, s := range scopes {
		if s == required {
			return true
		}
		// read satisfies search
		if required == ScopeSearch && s == ScopeRead {
			return true
		}
		// manage satisfies audit read
		if required == ScopeAuditRead && s == ScopeManagePolicy {
			return true
		}
	}
	return false
}

func (s *ApplicationService) requireScope(scopes []string, required string) error {
	if hasScope(scopes, required) {
		return nil
	}
	return platform.ErrForbidden("missing oauth scope " + required)
}

func (s *ApplicationService) requireAuthzManage(ctx context.Context, principal ports.Principal, orgID string) error {
	if s.Authz == nil {
		return platform.ErrUnavailable("authorization not configured")
	}
	orgObject := "organization:" + orgID
	// Organization type exposes manager/knowledge_admin, not can_manage; try both.
	for _, action := range []string{"can_manage", "manager", "knowledge_admin"} {
		dec, err := s.Authz.Check(ctx, ports.AuthzCheck{
			Principal:   principal,
			Action:      action,
			ResourceID:  orgObject,
			Consistency: ports.ConsistencyFullyConsistent,
		})
		if err != nil {
			return err
		}
		if dec.Allowed {
			return nil
		}
	}
	return platform.ErrForbidden("can_manage required")
}

// requireManage enforces context:manage_policy scope and OpenFGA can_manage on the org.
func (s *ApplicationService) requireManage(ctx context.Context, principal ports.Principal, orgID string, extraScopes []string) error {
	scopes := scopesOf(principal, extraScopes)
	if err := s.requireScope(scopes, ScopeManagePolicy); err != nil {
		return err
	}
	return s.requireAuthzManage(ctx, principal, orgID)
}

func (s *ApplicationService) requireIngest(ctx context.Context, principal ports.Principal, orgID string, extraScopes []string) error {
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return err
	}
	scopes := scopesOf(principal, extraScopes)
	return s.requireScope(scopes, ScopeIngest)
}

func (s *ApplicationService) requireAuditRead(ctx context.Context, principal ports.Principal, orgID string, extraScopes []string) error {
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return err
	}
	scopes := scopesOf(principal, extraScopes)
	return s.requireScope(scopes, ScopeAuditRead)
}

// requireRead enforces context:read (or search) scope for org-scoped reads.
func (s *ApplicationService) requireRead(ctx context.Context, principal ports.Principal, orgID string, extraScopes []string) error {
	if err := platform.RequireOrg(principal, orgID); err != nil {
		return err
	}
	scopes := scopesOf(principal, extraScopes)
	if err := s.requireScope(scopes, ScopeRead); err != nil {
		return s.requireScope(scopes, ScopeSearch)
	}
	return nil
}

func platformBootstrapToken() string {
	return strings.TrimSpace(os.Getenv(PlatformBootstrapTokenEnv))
}

func validPlatformBootstrap(creds ports.Credentials) bool {
	want := platformBootstrapToken()
	if want == "" {
		return false
	}
	got := strings.TrimSpace(creds.BearerToken)
	got = strings.TrimPrefix(got, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func isPlatformAdmin(principal ports.Principal) bool {
	for _, r := range principal.Roles {
		if strings.EqualFold(r, "platform_admin") || strings.EqualFold(r, "owner") {
			return true
		}
	}
	for _, g := range principal.Groups {
		if strings.EqualFold(g, "platform_admin") {
			return true
		}
	}
	return false
}

// requireOrgBootstrap authorizes first-time org creation via platform token or manage policy.
func (s *ApplicationService) requireOrgBootstrap(ctx context.Context, creds ports.Credentials, principal ports.Principal, orgID string, extraScopes []string) error {
	if validPlatformBootstrap(creds) || isPlatformAdmin(principal) {
		return nil
	}
	// Existing org path: must manage.
	_, err := s.Ledger.GetOrganization(ctx, orgID)
	if err != nil {
		// New org: platform token or platform admin only.
		return platform.ErrForbidden("organization bootstrap requires platform credentials")
	}
	return s.requireManage(ctx, principal, orgID, extraScopes)
}

// generateWebhookSecret returns a cryptographically random hex secret for a subscription.
func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
