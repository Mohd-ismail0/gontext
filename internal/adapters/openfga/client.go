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

	"github.com/xsama/context-fabric/internal/config"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Client is an HTTP OpenFGA AuthorizationProvider + RelationshipWriter.
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
	store := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID"))
	if store == "" || store == "replace-after-bootstrap" {
		if path := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID_FILE")); path != "" {
			if b, err := os.ReadFile(path); err == nil {
				store = strings.TrimSpace(string(b))
			}
		}
	}
	model := firstNonEmpty(os.Getenv("OPENFGA_MODEL_ID"), os.Getenv("AUTHZ_MODEL_ID"))
	if config.IsOpenFGAModelPlaceholder(model) {
		if path := strings.TrimSpace(os.Getenv("OPENFGA_MODEL_ID_FILE")); path != "" {
			if b, err := os.ReadFile(path); err == nil {
				model = strings.TrimSpace(string(b))
			}
		}
	}
	if api == "" || store == "" || model == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL, OPENFGA_STORE_ID, and OPENFGA_MODEL_ID are required")
	}
	if store == "replace-after-bootstrap" {
		return nil, fmt.Errorf("OPENFGA_STORE_ID is still %q; run bootstrap or set a real store id", store)
	}
	return &Client{
		APIURL:     strings.TrimRight(api, "/"),
		StoreID:    store,
		ModelID:    model,
		APIToken:   os.Getenv("OPENFGA_API_TOKEN"),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

var (
	_ ports.AuthorizationProvider = (*Client)(nil)
	_ ports.RelationshipWriter    = (*Client)(nil)
	_ ports.RelationshipInspector = (*Client)(nil)
)

// ConsistencyPreference maps ports.ConsistencyMode to OpenFGA preference strings.
func ConsistencyPreference(mode ports.ConsistencyMode) string {
	switch mode {
	case ports.ConsistencyFullyConsistent:
		return "HIGHER_CONSISTENCY"
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
	// Prefer native OpenFGA batch-check (one RTT per candidate set).
	if out, err := c.batchCheckNative(ctx, reqs); err == nil {
		return out, nil
	}
	// Fallback: sequential checks (older OpenFGA / partial outages).
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
	return out, nil
}

func (c *Client) batchCheckNative(ctx context.Context, reqs []ports.AuthzCheck) ([]ports.AuthzDecision, error) {
	checks := make([]map[string]any, 0, len(reqs))
	mode := ports.ConsistencyMinLatency
	for i, r := range reqs {
		if r.Consistency == ports.ConsistencyFullyConsistent {
			mode = ports.ConsistencyFullyConsistent
		}
		checks = append(checks, map[string]any{
			"correlation_id": fmt.Sprintf("%d", i),
			"tuple_key": map[string]string{
				"user":     formatUser(r.Principal),
				"relation": mapRelation(r.Action),
				"object":   formatObject(r.ResourceID),
			},
		})
	}
	payload := map[string]any{
		"authorization_model_id": c.ModelID,
		"checks":                 checks,
		"consistency":            ConsistencyPreference(mode),
	}
	var resp struct {
		Result map[string]struct {
			Allowed bool `json:"allowed"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		} `json:"result"`
	}
	if err := c.postJSON(ctx, fmt.Sprintf("/stores/%s/batch-check", c.StoreID), payload, &resp); err != nil {
		return nil, err
	}
	out := make([]ports.AuthzDecision, len(reqs))
	now := time.Now().UTC()
	for i, r := range reqs {
		key := fmt.Sprintf("%d", i)
		entry, ok := resp.Result[key]
		allowed := ok && entry.Allowed && entry.Error == nil
		out[i] = ports.AuthzDecision{
			Allowed:       allowed,
			ReasonCode:    reason(allowed),
			Consistency:   r.Consistency,
			ModelRevision: c.ModelID,
			CheckedAt:     now,
		}
	}
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
	// Coarse org membership: organization#member (model.fga), not can_read.
	user := formatUser(req.Principal)
	payload := map[string]any{
		"tuple_key": map[string]string{
			"user":     user,
			"relation": "member",
			"object":   "organization:" + req.Principal.OrgID,
		},
		"authorization_model_id": c.ModelID,
		"consistency":            ConsistencyPreference(req.Consistency),
	}
	var resp struct {
		Allowed bool `json:"allowed"`
	}
	if err := c.postJSON(ctx, fmt.Sprintf("/stores/%s/check", c.StoreID), payload, &resp); err != nil {
		return ports.CandidateScope{}, err
	}
	if !resp.Allowed {
		return ports.CandidateScope{OrgID: req.Principal.OrgID, ReasonCode: "AUTHZ_SCOPE_DENY"}, nil
	}
	return ports.CandidateScope{OrgID: req.Principal.OrgID, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
}

// WriteTuples implements ports.RelationshipWriter via OpenFGA /write.
func (c *Client) WriteTuples(ctx context.Context, tuples []ports.RelationshipTuple) error {
	return c.mutateTuples(ctx, tuples, false)
}

// DeleteTuples implements ports.RelationshipWriter via OpenFGA /write deletes.
func (c *Client) DeleteTuples(ctx context.Context, tuples []ports.RelationshipTuple) error {
	return c.mutateTuples(ctx, tuples, true)
}

// HasTuple implements ports.RelationshipInspector via OpenFGA /read.
func (c *Client) HasTuple(ctx context.Context, tuple ports.RelationshipTuple) (bool, error) {
	if tuple.Object == "" || tuple.Relation == "" || tuple.Subject == "" {
		return false, nil
	}
	payload := map[string]any{
		"tuple_key": map[string]string{
			"user":     tuple.Subject,
			"relation": tuple.Relation,
			"object":   tuple.Object,
		},
	}
	var resp struct {
		Tuples []json.RawMessage `json:"tuples"`
	}
	if err := c.postJSON(ctx, fmt.Sprintf("/stores/%s/read", c.StoreID), payload, &resp); err != nil {
		return false, err
	}
	return len(resp.Tuples) > 0, nil
}

func (c *Client) mutateTuples(ctx context.Context, tuples []ports.RelationshipTuple, delete bool) error {
	if len(tuples) == 0 {
		return nil
	}
	const batchSize = 40
	for i := 0; i < len(tuples); i += batchSize {
		end := i + batchSize
		if end > len(tuples) {
			end = len(tuples)
		}
		keys := make([]map[string]string, 0, end-i)
		for _, t := range tuples[i:end] {
			if t.Object == "" || t.Relation == "" || t.Subject == "" {
				continue
			}
			keys = append(keys, map[string]string{
				"user":     t.Subject,
				"relation": t.Relation,
				"object":   t.Object,
			})
		}
		if len(keys) == 0 {
			continue
		}
		writes := map[string]any{}
		if delete {
			writes["deletes"] = map[string]any{"tuple_keys": keys}
		} else {
			writes["writes"] = map[string]any{"tuple_keys": keys}
		}
		payload := map[string]any{
			"authorization_model_id": c.ModelID,
			"writes":                 writes["writes"],
			"deletes":                writes["deletes"],
		}
		// Clean nil keys for OpenFGA.
		if delete {
			payload = map[string]any{
				"authorization_model_id": c.ModelID,
				"deletes":                map[string]any{"tuple_keys": keys},
			}
		} else {
			payload = map[string]any{
				"authorization_model_id": c.ModelID,
				"writes":                 map[string]any{"tuple_keys": keys},
			}
		}
		if err := c.postJSON(ctx, fmt.Sprintf("/stores/%s/write", c.StoreID), payload, nil); err != nil {
			// Treat already-exists / not-found as idempotent success for retries.
			msg := err.Error()
			if strings.Contains(strings.ToLower(msg), "already exists") ||
				strings.Contains(strings.ToLower(msg), "cannot write a tuple which already exists") ||
				(delete && (strings.Contains(strings.ToLower(msg), "not found") || strings.Contains(strings.ToLower(msg), "does not exist"))) {
				continue
			}
			return err
		}
	}
	return nil
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
	case "context.search", "context.get", "context.brief", "context.graph", "can_read", "reader":
		return "can_read"
	case "can_manage", "context.manage", "can_admin", "context.delete", "can_delete":
		// model.fga has no can_delete; deletions require can_manage.
		return "can_manage"
	case "member":
		return "member"
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
