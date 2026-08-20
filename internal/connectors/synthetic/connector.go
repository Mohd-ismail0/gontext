// Package synthetic provides a conformance connector that uses only public intake APIs.
package synthetic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/ingest"
	"github.com/xsama/context-fabric/internal/platform"
)

// Config for the synthetic conformance connector.
type Config struct {
	GatewayURL    string
	OrgID         string
	SourceID      string
	SigningSecret string
	BearerToken   string
	HTTPClient    *http.Client
}

// Connector emits synthetic CloudEvents for conformance.
type Connector struct {
	cfg Config
}

// New creates a synthetic connector.
func New(cfg Config) *Connector {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.SourceID == "" {
		cfg.SourceID = "synthetic"
	}
	return &Connector{cfg: cfg}
}

// EmitOnce posts one synthetic event with the given external id/revision.
func (c *Connector) EmitOnce(ctx context.Context, externalID, revision, title string) (map[string]any, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	idem := c.cfg.SourceID + ":" + externalID + ":" + revision
	ce := map[string]any{
		"specversion":     "1.0",
		"id":              platform.NewEventID(),
		"source":          c.cfg.SourceID,
		"type":            "com.contextfabric.ingest.synthetic.fact.v1",
		"time":            now,
		"datacontenttype": "application/json",
		"organizationid":  c.cfg.OrgID,
		"data": map[string]any{
			"producer":           "synthetic",
			"occurred_at":        now,
			"observed_at":        now,
			"classification":     "internal",
			"trust":              "trusted_internal",
			"source_authority":   "corroborating",
			"schema_id":          "synthetic.fact",
			"schema_version":     "1",
			"idempotency_key":    idem,
			"source_external_id": externalID,
			"source_revision":    revision,
			"content_locator":    "synthetic://" + externalID + "/" + revision,
			"title":              title,
			"resource_type":      "event",
			"resource_id":        "synthetic:" + externalID,
			"visibility_ref":     "case:" + externalID + "#viewer",
		},
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
	req.Header.Set("Idempotency-Key", idem)
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

// RunCLI is the `context-fabric connector synthetic` entrypoint.
func RunCLI(args []string) error {
	cfg := Config{
		GatewayURL:    env("CONTEXT_FABRIC_URL", "http://127.0.0.1:8080"),
		OrgID:         env("CONTEXT_FABRIC_ORG_ID", ""),
		SourceID:      env("CONTEXT_FABRIC_SOURCE_ID", "synthetic"),
		SigningSecret: env("CONTEXT_FABRIC_SOURCE_SECRET", ""),
		BearerToken:   env("CONTEXT_FABRIC_TOKEN", ""),
	}
	if cfg.OrgID == "" || cfg.SigningSecret == "" || cfg.BearerToken == "" {
		return fmt.Errorf("require CONTEXT_FABRIC_ORG_ID, CONTEXT_FABRIC_SOURCE_SECRET, CONTEXT_FABRIC_TOKEN")
	}
	ext, rev, title := "syn-001", "1", "synthetic fact"
	if len(args) > 0 {
		ext = args[0]
	}
	if len(args) > 1 {
		rev = args[1]
	}
	if len(args) > 2 {
		title = args[2]
	}
	out, err := New(cfg).EmitOnce(context.Background(), ext, rev, title)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
