package openfga

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Client is an HTTP OpenFGA AuthorizationProvider wrapper.
type Client struct {
	APIURL     string
	StoreID    string
	ModelID    string
	APIToken   string
	HTTPClient *http.Client
}

// NewFromEnv constructs a Client from OPENFGA_API_URL, OPENFGA_STORE_ID, OPENFGA_MODEL_ID.
func NewFromEnv() (*Client, error) {
	api := firstNonEmpty(os.Getenv("OPENFGA_API_URL"), os.Getenv("OPENFGA_URL"))
	store := os.Getenv("OPENFGA_STORE_ID")
	model := firstNonEmpty(os.Getenv("OPENFGA_MODEL_ID"), os.Getenv("AUTHZ_MODEL_ID"))
	if api == "" || store == "" || model == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL, OPENFGA_STORE_ID, and OPENFGA_MODEL_ID are required")
	}
	return &Client{
		APIURL:     strings.TrimRight(api, "/"),
		StoreID:    store,
		ModelID:    model,
		APIToken:   os.Getenv("OPENFGA_API_TOKEN"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

var _ ports.AuthorizationProvider = (*Client)(nil)

// ConsistencyPreference maps ports.ConsistencyMode to OpenFGA preference strings.
func ConsistencyPreference(mode ports.ConsistencyMode) string {
	switch mode {
	case ports.ConsistencyFullyConsistent:
		return "HIGHER_CONSISTENCY"
	case ports.ConsistencyMinLatency, "":
		return "MINIMIZE_LATENCY"
	default:
		return "MINIMIZE_LATENCY"
	}
}

func (c *Client) Check(ctx context.Context, req ports.AuthzCheck) (ports.AuthzDecision, error) {
	outs, err := c.BatchCheck(ctx, []ports.AuthzCheck{req})
	if err != nil {
		return ports.AuthzDecision{}, err
	}
	if len(outs) == 0 {
		return ports.AuthzDecision{Allowed: false, ReasonCode: "AUTHZ_EMPTY", Consistency: req.Consistency, CheckedAt: time.Now().UTC()}, nil
	}
	return outs[0], nil
}

func (c *Client) BatchCheck(ctx context.Context, reqs []ports.AuthzCheck) ([]ports.AuthzDecision, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	consistency := ConsistencyPreference(reqs[0].Consistency)
	tupleKeys := make([]map[string]string, 0, len(reqs))
	for _, r := range reqs {
		tupleKeys = append(tupleKeys, map[string]string{
			"user":     formatUser(r.Principal),
			"relation": mapRelation(r.Action),
			"object":   formatObject(r.ResourceID),
		})
	}
	body := map[string]any{
		"checks": []map[string]any{
			{
				"tuple_keys":  tupleKeys,
				"consistency": consistency,
			},
		},
		"authorization_model_id": c.ModelID,
	}
	// OpenFGA batch-check API shape varies; use per-check fallback for compatibility.
	out := make([]ports.AuthzDecision, len(reqs))
	for i, r := range reqs {
		allowed, err := c.checkOne(ctx, r)
		if err != nil {
			return nil, err
		}
		out[i] = ports.AuthzDecision{
			Allowed:       allowed,
			ReasonCode:    reason(allowed),
			Consistency:   r.Consistency,
			ModelRevision: c.ModelID,
			CheckedAt:     time.Now().UTC(),
		}
	}
	_ = body
	return out, nil
}

func (c *Client) checkOne(ctx context.Context, req ports.AuthzCheck) (bool, error) {
	payload := map[string]any{
		"tuple_key": map[string]string{
			"user":     formatUser(req.Principal),
			"relation": mapRelation(req.Action),
			"object":   formatObject(req.ResourceID),
		},
		"authorization_model_id": c.ModelID,
		"consistency":            ConsistencyPreference(req.Consistency),
	}
	var resp struct {
		Allowed bool `json:"allowed"`
	}
	if err := c.postJSON(ctx, fmt.Sprintf("/stores/%s/check", c.StoreID), payload, &resp); err != nil {
		return false, err
	}
	return resp.Allowed, nil
}

func (c *Client) ResolveCandidateScope(ctx context.Context, req ports.ScopeResolve) (ports.CandidateScope, error) {
	// v1: coarse org membership check; resource IDs come from index + batch check.
	dec, err := c.Check(ctx, ports.AuthzCheck{
		Principal:   req.Principal,
		Action:      "can_read",
		ResourceID:  "organization:" + req.Principal.OrgID,
		Consistency: req.Consistency,
	})
	if err != nil {
		return ports.CandidateScope{}, err
	}
	if !dec.Allowed {
		return ports.CandidateScope{OrgID: req.Principal.OrgID, ReasonCode: "AUTHZ_SCOPE_DENY"}, nil
	}
	return ports.CandidateScope{OrgID: req.Principal.OrgID, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, out any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(httpReq)
	if err != nil {
		return platform.ErrUnavailable(err.Error())
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return platform.ErrUnavailable(fmt.Sprintf("openfga status %d: %s", res.StatusCode, string(body)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func formatUser(p ports.Principal) string {
	prefix := "user"
	switch p.Kind {
	case ports.PrincipalKindAgent:
		prefix = "agent"
	case ports.PrincipalKindService:
		prefix = "service"
	}
	id := p.Subject
	if id == "" {
		id = p.ID
	}
	if strings.Contains(id, ":") {
		return id
	}
	return prefix + ":" + id
}

func formatObject(resourceID string) string {
	if strings.Contains(resourceID, ":") {
		return resourceID
	}
	return "resource:" + resourceID
}

func mapRelation(action string) string {
	switch action {
	case "context.search", "context.get", "context.brief", "can_read", "reader":
		return "can_read"
	case "can_manage", "context.manage", "can_admin", "can_delete", "context.delete":
		if action == "context.delete" {
			return "can_delete"
		}
		if action == "can_admin" || action == "can_manage" || action == "context.manage" {
			return "can_manage"
		}
		return "can_delete"
	default:
		if action == "" {
			return "can_read"
		}
		return action
	}
}

func reason(allowed bool) string {
	if allowed {
		return "AUTHZ_ALLOW"
	}
	return "AUTHZ_DENY"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
