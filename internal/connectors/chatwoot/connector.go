// Package chatwoot converts Chatwoot webhooks into CloudEvents intake POSTs.
package chatwoot

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
)

//go:embed mapping.yaml
var MappingYAML []byte

// Config for the Chatwoot connector.
type Config struct {
	GatewayURL     string
	OrgID          string
	SourceID       string
	SigningSecret  string
	BearerToken    string
	HTTPClient     *http.Client
}

// Connector posts mapped CloudEvents to the gateway intake API.
type Connector struct {
	cfg Config
}

// New creates a Chatwoot connector.
func New(cfg Config) *Connector {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.SourceID == "" {
		cfg.SourceID = "chatwoot"
	}
	return &Connector{cfg: cfg}
}

// MappingSpecJSON returns MappingSpec JSON derived from the embedded sample (YAML subset as JSON-compatible).
func MappingSpecJSON() ([]byte, error) {
	// mapping.yaml is JSON-compatible YAML (no anchors); try JSON first after light cleanup.
	raw := bytes.TrimSpace(MappingYAML)
	if json.Valid(raw) {
		return raw, nil
	}
	// Minimal YAML→JSON for our flat sample: use a simple converter via known structure.
	var spec mapping.Spec
	if err := decodeLooseYAML(raw, &spec); err != nil {
		return nil, err
	}
	return json.Marshal(spec)
}

// CloudEvent is the intake envelope.
type CloudEvent struct {
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Source          string         `json:"source"`
	Type            string         `json:"type"`
	Time            string         `json:"time"`
	DataContentType string         `json:"datacontenttype"`
	OrganizationID  string         `json:"organizationid"`
	Data            map[string]any `json:"data"`
}

// ToCloudEvent maps a Chatwoot webhook JSON body to a CloudEvents intake envelope.
func ToCloudEvent(orgID, sourceID string, webhook map[string]any) (CloudEvent, error) {
	event := "message_created"
	if v, ok := webhook["event"].(string); ok {
		event = v
	}
	conv, _ := webhook["conversation"].(map[string]any)
	msg, _ := webhook["message"].(map[string]any)
	if msg == nil {
		msg, _ = webhook["content_attributes"].(map[string]any)
	}
	convID := stringify(dig(conv, "id"))
	if convID == "" {
		convID = stringify(webhook["id"])
	}
	msgID := stringify(dig(msg, "id"))
	if msgID == "" {
		msgID = stringify(webhook["id"])
	}
	created := stringify(dig(msg, "created_at"))
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	content := stringify(dig(msg, "content"))
	subject := stringify(dig(conv, "meta", "sender", "name"))
	if subject == "" {
		subject = "conversation:" + convID
	}
	idem := sourceID + ":" + convID + ":" + msgID
	eventID := platform.NewEventID()
	return CloudEvent{
		SpecVersion:     "1.0",
		ID:              eventID,
		Source:          sourceID,
		Type:            "com.contextfabric.ingest.chatwoot." + sanitizeType(event) + ".v1",
		Time:            time.Now().UTC().Format(time.RFC3339Nano),
		DataContentType: "application/json",
		OrganizationID:  orgID,
		Data: map[string]any{
			"producer":           "chatwoot",
			"occurred_at":        created,
			"observed_at":        time.Now().UTC().Format(time.RFC3339Nano),
			"classification":     "internal",
			"trust":              "untrusted_external",
			"source_authority":   "corroborating",
			"schema_id":          "chatwoot.webhook",
			"schema_version":     "1",
			"idempotency_key":    idem,
			"source_external_id": convID,
			"source_revision":    msgID,
			"content_locator":    "chatwoot://conversation/" + convID + "/message/" + msgID,
			"title":              subject,
			"resource_type":      "message",
			"resource_id":        "chatwoot:" + convID,
			"visibility_ref":     "case:" + convID + "#viewer",
			"attributes": map[string]any{
				"channel": "chatwoot",
				"preview": truncate(content, 200),
				"event":   event,
			},
		},
	}, nil
}

// HandleWebhook converts and POSTs to gateway intake with HMAC.
func (c *Connector) HandleWebhook(ctx context.Context, webhookJSON []byte) (map[string]any, error) {
	var wh map[string]any
	if err := json.Unmarshal(webhookJSON, &wh); err != nil {
		return nil, fmt.Errorf("invalid chatwoot webhook: %w", err)
	}
	ce, err := ToCloudEvent(c.cfg.OrgID, c.cfg.SourceID, wh)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(ce)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(c.cfg.GatewayURL, "/") + "/v1/organizations/" + c.cfg.OrgID + "/context/intake"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BearerToken)
	req.Header.Set("Idempotency-Key", stringify(ce.Data["idempotency_key"]))
	req.Header.Set("X-Context-Fabric-Source-Id", c.cfg.SourceID)
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set("X-Context-Fabric-Timestamp", ts)
	req.Header.Set("X-Context-Fabric-Signature", ingest.SignHMAC(c.cfg.SigningSecret, body))

	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(respBody, &out)
	if res.StatusCode >= 300 {
		return out, fmt.Errorf("intake status %d: %s", res.StatusCode, string(respBody))
	}
	return out, nil
}

// RunCLI is the `context-fabric connector chatwoot` entrypoint.
func RunCLI(args []string) error {
	cfg := Config{
		GatewayURL:    firstEnv("http://127.0.0.1:8080", "CONTEXT_FABRIC_URL", "GATEWAY_URL"),
		OrgID:         firstEnv("", "CONTEXT_FABRIC_ORG_ID", "ORG_ID"),
		SourceID:      firstEnv("chatwoot", "CONTEXT_FABRIC_SOURCE_ID", "SOURCE_ID"),
		SigningSecret: firstEnv("", "CONTEXT_FABRIC_SOURCE_SECRET", "SOURCE_SECRET"),
		BearerToken:   firstEnv("", "CONTEXT_FABRIC_TOKEN", "BEARER_TOKEN"),
	}
	if cfg.OrgID == "" || cfg.SigningSecret == "" || cfg.BearerToken == "" {
		return fmt.Errorf("require CONTEXT_FABRIC_ORG_ID, CONTEXT_FABRIC_SOURCE_SECRET, CONTEXT_FABRIC_TOKEN")
	}
	c := New(cfg)

	// If a file path is provided, post once; else read stdin.
	var raw []byte
	var err error
	if len(args) > 0 {
		raw, err = os.ReadFile(args[0])
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		fmt.Println("chatwoot connector: waiting for webhook JSON on stdin (or pass a file path)")
		return nil
	}
	out, err := c.HandleWebhook(context.Background(), raw)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func sanitizeType(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		b, _ := json.Marshal(t)
		return strings.Trim(string(b), `"`)
	}
}

func firstEnv(def string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return def
}

// decodeLooseYAML parses our simple mapping.yaml without a YAML dependency:
// it accepts JSON, or a restricted YAML subset already authored as JSON-like.
func decodeLooseYAML(raw []byte, dest *mapping.Spec) error {
	if err := json.Unmarshal(raw, dest); err == nil {
		return nil
	}
	// Fallback: strip comments and rely on json after converting keys — for the
	// checked-in sample we keep JSON-compatible content inside mapping.yaml.
	return fmt.Errorf("mapping.yaml must be JSON-compatible; got parse error")
}
