package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RemoteOptions configures a live-base-URL conformance probe.
type RemoteOptions struct {
	BaseURL string
	Token   string
	OrgID   string
	Client  *http.Client
}

// RemoteCheck is one remote probe result.
type RemoteCheck struct {
	Name    string
	Passed  bool
	Detail  string
	Error   string
	Skipped bool
}

// RemoteReport aggregates remote probes.
type RemoteReport struct {
	BaseURL string
	Checks  []RemoteCheck
}

// Passed reports whether every non-skipped check passed.
func (r RemoteReport) Passed() bool {
	for _, c := range r.Checks {
		if !c.Skipped && !c.Passed {
			return false
		}
	}
	return true
}

// RemoteRunner hits a live Context Fabric base URL for health, PRM, and MCP JSON-RPC.
type RemoteRunner struct {
	BaseURL string
	Token   string
	OrgID   string
	Client  *http.Client
}

// NewRemoteRunner builds a runner from options.
func NewRemoteRunner(opts RemoteOptions) *RemoteRunner {
	c := opts.Client
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	return &RemoteRunner{
		BaseURL: strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		Token:   strings.TrimSpace(opts.Token),
		OrgID:   strings.TrimSpace(opts.OrgID),
		Client:  c,
	}
}

// Run executes health, PRM, MCP initialize/list/call, REST search, and parity probes.
func (r *RemoteRunner) Run(ctx context.Context) (RemoteReport, error) {
	if r.BaseURL == "" {
		return RemoteReport{}, fmt.Errorf("base URL required")
	}
	rep := RemoteReport{BaseURL: r.BaseURL}
	rep.Checks = append(rep.Checks, r.checkHealth(ctx))
	rep.Checks = append(rep.Checks, r.checkPRM(ctx))
	rep.Checks = append(rep.Checks, r.checkMCPUnauthorized(ctx))
	if r.Token == "" {
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.initialize", Skipped: true, Detail: "token required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.tools/list", Skipped: true, Detail: "token required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.tools/call", Skipped: true, Detail: "token required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "rest.context.search", Skipped: true, Detail: "token required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.rest.parity", Skipped: true, Detail: "token required",
		})
		return rep, nil
	}
	rep.Checks = append(rep.Checks, r.checkMCP(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "cf-sandbox", "version": "1.0.0"},
	}, "protocolVersion"))
	rep.Checks = append(rep.Checks, r.checkMCP(ctx, "tools/list", nil, "tools"))
	if r.OrgID == "" {
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.tools/call", Skipped: true, Detail: "org required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "rest.context.search", Skipped: true, Detail: "org required",
		})
		rep.Checks = append(rep.Checks, RemoteCheck{
			Name: "mcp.rest.parity", Skipped: true, Detail: "org required",
		})
		return rep, nil
	}
	mcpPkt, mcpCheck := r.checkMCPSearch(ctx)
	rep.Checks = append(rep.Checks, mcpCheck)
	restPkt, restCheck := r.checkRESTSearch(ctx)
	rep.Checks = append(rep.Checks, restCheck)
	rep.Checks = append(rep.Checks, parityCheck(mcpPkt, restPkt, mcpCheck.Passed, restCheck.Passed))
	return rep, nil
}

func (r *RemoteRunner) checkHealth(ctx context.Context) RemoteCheck {
	name := "health.ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/health/ready", nil)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("status %d: %s", res.StatusCode, string(body))}
	}
	return RemoteCheck{Name: name, Passed: true, Detail: strings.TrimSpace(string(body))}
}

func (r *RemoteRunner) checkPRM(ctx context.Context) RemoteCheck {
	name := "prm.oauth-protected-resource"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/.well-known/oauth-protected-resource", nil)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("status %d: %s", res.StatusCode, string(body))}
	}
	var meta map[string]any
	if err := json.Unmarshal(body, &meta); err != nil {
		return RemoteCheck{Name: name, Error: "invalid JSON: " + err.Error()}
	}
	if _, ok := meta["resource"]; !ok {
		return RemoteCheck{Name: name, Error: "missing resource field", Detail: string(body)}
	}
	return RemoteCheck{Name: name, Passed: true, Detail: "resource metadata present"}
}

func (r *RemoteRunner) checkMCP(ctx context.Context, method string, params any, expectKey string) RemoteCheck {
	name := "mcp." + method
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/mcp", bytes.NewReader(raw))
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Token)
	if r.OrgID != "" {
		req.Header.Set("X-Organization-Id", r.OrgID)
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 300 {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("status %d: %s", res.StatusCode, string(body))}
	}
	var rpc struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpc); err != nil {
		return RemoteCheck{Name: name, Error: "invalid JSON-RPC: " + err.Error()}
	}
	if rpc.Error != nil {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("rpc %d: %s", rpc.Error.Code, rpc.Error.Message)}
	}
	if expectKey != "" {
		if _, ok := rpc.Result[expectKey]; !ok {
			return RemoteCheck{Name: name, Error: "missing result." + expectKey, Detail: string(body)}
		}
	}
	return RemoteCheck{Name: name, Passed: true, Detail: method + " ok"}
}

func (r *RemoteRunner) checkMCPUnauthorized(ctx context.Context) RemoteCheck {
	name := "mcp.unauthorized"
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := r.Client.Do(req)
	if err != nil {
		return RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode != http.StatusUnauthorized {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("want 401, got %d", res.StatusCode)}
	}
	if !strings.Contains(strings.ToLower(res.Header.Get("WWW-Authenticate")), "bearer") {
		return RemoteCheck{Name: name, Error: "missing WWW-Authenticate Bearer challenge"}
	}
	return RemoteCheck{Name: name, Passed: true, Detail: "401 + WWW-Authenticate"}
}

type remotePacket struct {
	Citations []struct {
		ResourceID string `json:"resource_id"`
	} `json:"citations"`
	OrganizationID string `json:"organization_id"`
	Purpose        string `json:"purpose"`
}

func (r *RemoteRunner) checkMCPSearch(ctx context.Context) (remotePacket, RemoteCheck) {
	name := "mcp.tools/call"
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "context.search",
			"arguments": map[string]any{
				"organization_id": r.OrgID,
				"query":           "conformance",
				"purpose":         "support",
				"max_items":       5,
			},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Token)
	req.Header.Set("X-Organization-Id", r.OrgID)
	res, err := r.Client.Do(req)
	if err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 300 {
		return remotePacket{}, RemoteCheck{Name: name, Error: fmt.Sprintf("status %d: %s", res.StatusCode, string(body))}
	}
	var rpc struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &rpc); err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: "invalid JSON-RPC: " + err.Error()}
	}
	if rpc.Error != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: fmt.Sprintf("rpc %d: %s", rpc.Error.Code, rpc.Error.Message)}
	}
	if rpc.Result.IsError {
		return remotePacket{}, RemoteCheck{Name: name, Error: "isError true: " + string(body)}
	}
	var text string
	for _, c := range rpc.Result.Content {
		if c.Type == "text" && c.Text != "" {
			text = c.Text
			break
		}
	}
	if text == "" {
		return remotePacket{}, RemoteCheck{Name: name, Error: "empty tool content"}
	}
	var pkt remotePacket
	if err := json.Unmarshal([]byte(text), &pkt); err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: "packet json: " + err.Error()}
	}
	return pkt, RemoteCheck{Name: name, Passed: true, Detail: "context.search packet"}
}

func (r *RemoteRunner) checkRESTSearch(ctx context.Context) (remotePacket, RemoteCheck) {
	name := "rest.context.search"
	payload, _ := json.Marshal(map[string]any{
		"query": "conformance", "purpose": "support", "max_items": 5,
	})
	url := r.BaseURL + "/v1/organizations/" + r.OrgID + "/context:search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Token)
	res, err := r.Client.Do(req)
	if err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: err.Error()}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 300 {
		return remotePacket{}, RemoteCheck{Name: name, Error: fmt.Sprintf("status %d: %s", res.StatusCode, string(body))}
	}
	var pkt remotePacket
	if err := json.Unmarshal(body, &pkt); err != nil {
		return remotePacket{}, RemoteCheck{Name: name, Error: "packet json: " + err.Error()}
	}
	return pkt, RemoteCheck{Name: name, Passed: true, Detail: "REST search packet"}
}

func parityCheck(mcpPkt, restPkt remotePacket, mcpOK, restOK bool) RemoteCheck {
	name := "mcp.rest.parity"
	if !mcpOK || !restOK {
		return RemoteCheck{Name: name, Skipped: true, Detail: "search probes did not both pass"}
	}
	mcpIDs := citationSet(mcpPkt)
	restIDs := citationSet(restPkt)
	if len(mcpIDs) != len(restIDs) {
		return RemoteCheck{Name: name, Error: fmt.Sprintf("citation count mcp=%d rest=%d", len(mcpIDs), len(restIDs))}
	}
	for id := range mcpIDs {
		if _, ok := restIDs[id]; !ok {
			return RemoteCheck{Name: name, Error: "citation mismatch for " + id}
		}
	}
	return RemoteCheck{Name: name, Passed: true, Detail: "citation resource_ids match"}
}

func citationSet(pkt remotePacket) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range pkt.Citations {
		if c.ResourceID != "" {
			out[c.ResourceID] = struct{}{}
		}
	}
	return out
}
