package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/xsama/context-fabric/internal/mcp"
)

func callMCPSearch(srv *mcp.Server, token, orgID, query, purpose string) (mapPacket, error) {
	args := map[string]any{
		"organization_id": orgID,
		"query":           query,
		"purpose":         purpose,
		"max_items":       5,
	}
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "context.search",
			"arguments": args,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		return mapPacket{}, fmt.Errorf("mcp status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		return mapPacket{}, err
	}
	if resp.Error != nil {
		return mapPacket{}, fmt.Errorf("mcp error: %s", resp.Error.Message)
	}
	if resp.Result.IsError {
		return mapPacket{}, fmt.Errorf("mcp tool error: %s", rr.Body.String())
	}
	var raw []byte
	for _, c := range resp.Result.Content {
		if c.Type == "text" && c.Text != "" {
			raw = []byte(c.Text)
			break
		}
	}
	if len(raw) == 0 {
		return mapPacket{}, fmt.Errorf("empty mcp content: %s", rr.Body.String())
	}
	var pkt mapPacket
	if err := json.Unmarshal(raw, &pkt); err != nil {
		return mapPacket{}, fmt.Errorf("decode packet: %w", err)
	}
	return pkt, nil
}

type mapPacket struct {
	Citations      []any  `json:"citations"`
	PolicyRevision string `json:"policy_revision"`
	AuthzRevision  string `json:"authz_revision"`
}
