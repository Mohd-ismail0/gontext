package platform

import "github.com/xsama/context-fabric/internal/ports"

// RequireOrg enforces that the authenticated principal belongs to pathOrg.
func RequireOrg(principal ports.Principal, pathOrg string) error {
	if pathOrg == "" {
		return ErrValidation("organization required")
	}
	if principal.OrgID == "" {
		return ErrForbidden("principal missing organization")
	}
	if principal.OrgID != pathOrg {
		return ErrForbidden("organization mismatch")
	}
	return nil
}
