package policy

import (
	"context"
	"strings"

	"github.com/xsama/context-fabric/internal/ports"
)

const Revision = "policy-v1"

// classification rank: higher is more sensitive
var classificationRank = map[string]int{
	"public":       1,
	"internal":     2,
	"confidential": 3,
	"restricted":   4,
	"secret":       5,
}

// Provider is a deterministic Go PolicyProvider.
// It never grants AuthZ access—only purpose/classification obligations after allow.
type Provider struct {
	AllowedPurposes      map[string]struct{}
	MaxClassification    string
	DefaultMaxResults    int
	ConfidentialRedact   string
	PublicRedact         string
}

// New returns a Provider with sensible v1 defaults.
func New() *Provider {
	return &Provider{
		AllowedPurposes: map[string]struct{}{
			"support":            {},
			"account_management": {},
			"marketing":          {},
			"finance":            {},
			"agent_assist":       {},
			"ops":                {},
			"compliance":         {},
			"engineering":        {},
			"research":           {},
		},
		MaxClassification:  "restricted",
		DefaultMaxResults:  25,
		ConfidentialRedact: "pii_mask",
		PublicRedact:       "none",
	}
}

var _ ports.PolicyProvider = (*Provider)(nil)

// Evaluate applies purpose allowlist, classification ceiling, redaction, and result limits.
func (p *Provider) Evaluate(_ context.Context, req ports.PolicyEval) (ports.PolicyResult, error) {
	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	if purpose == "" {
		return ports.PolicyResult{
			Allow:      false,
			ReasonCode: "purpose_required",
		}, nil
	}
	if _, ok := p.AllowedPurposes[purpose]; !ok {
		return ports.PolicyResult{
			Allow:      false,
			ReasonCode: "purpose_not_allowed",
		}, nil
	}

	class := strings.ToLower(strings.TrimSpace(req.Classification))
	if class == "" && req.Record != nil {
		class = strings.ToLower(strings.TrimSpace(req.Record.Classification))
	}
	if class == "" {
		class = "internal"
	}

	if rank(class) > rank(p.MaxClassification) {
		return ports.PolicyResult{
			Allow:      false,
			ReasonCode: "classification_ceiling",
		}, nil
	}

	redaction := p.PublicRedact
	obligations := []string{"audit_disclosure"}
	if rank(class) >= rank("confidential") {
		redaction = p.ConfidentialRedact
		obligations = append(obligations, "mask_pii", "no_export_raw")
	}

	maxResults := p.DefaultMaxResults
	if req.RequestedLimit > 0 && req.RequestedLimit < maxResults {
		maxResults = req.RequestedLimit
	}

	return ports.PolicyResult{
		Allow:            true,
		RedactionProfile: redaction,
		Obligations:      obligations,
		MaxResults:       maxResults,
		ReasonCode:       "policy_ok",
	}, nil
}

func rank(c string) int {
	if r, ok := classificationRank[strings.ToLower(c)]; ok {
		return r
	}
	return classificationRank["internal"]
}
