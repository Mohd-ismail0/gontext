package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/application"
	"github.com/xsama/context-fabric/internal/export"
	"github.com/xsama/context-fabric/internal/mcp"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// Server exposes the OpenAPI REST surface over net/http.
type Server struct {
	App    *app.ApplicationService
	Mux    *http.ServeMux
	ready  bool
	started bool
	MCP    *mcp.Server
}

// New constructs and registers routes.
func New(svc *app.ApplicationService) *Server {
	s := &Server{App: svc, Mux: http.NewServeMux(), ready: true, started: true, MCP: mcp.New(svc)}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.Mux }

func (s *Server) SetReady(v bool) { s.ready = v }

func (s *Server) routes() {
	s.Mux.HandleFunc("GET /health/live", s.handleLive)
	s.Mux.HandleFunc("GET /health/startup", s.handleStartup)
	s.Mux.HandleFunc("GET /health/ready", s.handleReady)
	s.Mux.HandleFunc("GET /v1/system/version", s.handleVersion)
	s.Mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handlePRM)
	s.Mux.Handle("POST /mcp", s.authHandler(s.MCP.Handler()))

	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/bootstrap", s.auth(s.handleBootstrap))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/status", s.auth(s.handleOrgStatus))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context:search", s.auth(s.handleSearch))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/resources/{resourceId}", s.auth(s.handleGet))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/resources/{resourceId}:delete", s.auth(s.handleDelete))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context:brief", s.auth(s.handleBrief))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/access-requests", s.auth(s.handleAccess))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/sources", s.auth(s.handleRegisterSource))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/sources", s.auth(s.handleListSources))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/sources/{sourceId}", s.auth(s.handleGetSource))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/sources/{sourceId}:verify", s.auth(s.handleVerifySource))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/sources/{sourceId}:rotate", s.auth(s.handleRotateSource))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/intake", s.auth(s.handleIntake))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/intake:batch", s.auth(s.handleIntakeBatch))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/evidence:presign", s.auth(s.handlePresign))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/changes", s.auth(s.handleChanges))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/audit", s.auth(s.handleAudit))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/webhooks", s.auth(s.handleListWebhooks))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/webhooks", s.auth(s.handleWebhooks))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/webhooks/{subscriptionId}", s.auth(s.handleGetWebhook))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/webhooks/{subscriptionId}/deliveries", s.auth(s.handleListDeliveries))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/webhooks/{subscriptionId}/deliveries/{deliveryId}:replay", s.auth(s.handleWebhookReplay))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/exports", s.auth(s.handleExport))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/exports/{exportId}", s.auth(s.handleGetExport))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/imports", s.auth(s.handleImport))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/quotas", s.auth(s.handleGetQuotas))
	s.Mux.HandleFunc("PUT /v1/organizations/{orgId}/context/quotas", s.auth(s.handleSetQuotas))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/context/diagnose", s.auth(s.handleDiagnose))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/context/diagnose/decision/{auditId}", s.auth(s.handleDiagnoseAudit))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/agents", s.auth(s.handleCreateAgent))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/agents/{agentId}/credentials:rotate", s.auth(s.handleRotateAgent))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/agents/credentials/{credentialId}:revoke", s.auth(s.handleRevokeAgent))
	s.Mux.HandleFunc("GET /v1/organizations/{orgId}/ops/lag", s.auth(s.handleOpsLag))
	s.Mux.HandleFunc("POST /v1/organizations/{orgId}/ops/support-bundle", s.auth(s.handleSupportBundle))
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if strings.TrimSpace(raw) == "" {
			writeErr(w, platform.ErrUnauthorized("missing Authorization header"))
			return
		}
		next(w, r)
	}
}

func (s *Server) authHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		if strings.TrimSpace(raw) == "" {
			writeErr(w, platform.ErrUnauthorized("missing Authorization header"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleStartup(w http.ResponseWriter, _ *http.Request) {
	if !s.started {
		writeErr(w, platform.ErrUnavailable("starting"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.ready {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": map[string]any{
				"process": map[string]any{"ok": false, "detail": "shutting down"},
			},
		})
		return
	}
	ok, checks := s.App.ReadyStatus()
	if checks == nil {
		checks = map[string]any{}
	}
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": checks,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
		"checks": checks,
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.App.Version())
}

func (s *Server) handlePRM(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, mcp.ProtectedResourceMetadata(r))
}

func bearerCreds(r *http.Request) ports.Credentials {
	raw := r.Header.Get("Authorization")
	token := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	creds := ports.Credentials{BearerToken: token}
	// X-Context-Scopes is ignored for authorization unless explicitly enabled.
	if os.Getenv("CONTEXT_FABRIC_ALLOW_SCOPE_HEADER") == "1" {
		scopes := strings.Fields(r.Header.Get("X-Context-Scopes"))
		if len(scopes) > 0 {
			creds.Extra = map[string]string{"scopes": strings.Join(scopes, " ")}
		}
	}
	return creds
}

func scopesOf(creds ports.Credentials) []string {
	if creds.Extra == nil {
		return nil
	}
	return strings.Fields(creds.Extra["scopes"])
}

// allowSkipHMAC is true only when explicitly enabled for demo/memory profiles.
func allowSkipHMAC() bool {
	if os.Getenv("CONTEXT_FABRIC_ALLOW_SKIP_HMAC") != "1" {
		return false
	}
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("CONTEXT_FABRIC_PROFILE")))
	if profile == "" {
		profile = strings.ToLower(strings.TrimSpace(os.Getenv("PROFILE")))
	}
	return profile == "demo" || profile == "memory"
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body app.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	pkt, err := s.App.Search(r.Context(), creds, orgID, scopesOf(creds), body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pkt)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	resourceID := r.PathValue("resourceId")
	creds := bearerCreds(r)
	purpose := r.URL.Query().Get("purpose")
	if purpose == "" {
		writeErr(w, platform.ErrValidation("purpose required"))
		return
	}
	pkt, err := s.App.GetResource(r.Context(), creds, orgID, resourceID, purpose, r.URL.Query().Get("consistency"), scopesOf(creds))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pkt)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	resourceID := r.PathValue("resourceId")
	creds := bearerCreds(r)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	out, err := s.App.DeleteResource(r.Context(), creds, orgID, resourceID, body.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBrief(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body struct {
		Purpose     string `json:"purpose"`
		Scope       struct {
			ResourceID string `json:"resource_id"`
		} `json:"scope"`
		MaxItems    int    `json:"max_items"`
		Consistency string `json:"consistency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	pkt, err := s.App.Brief(r.Context(), creds, orgID, scopesOf(creds), body.Purpose, body.Scope.ResourceID, body.Consistency, body.MaxItems)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pkt)
}

func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body struct {
		ResourceID    string `json:"resource_id"`
		Purpose       string `json:"purpose"`
		Justification string `json:"justification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.RequestAccess(r.Context(), creds, orgID, body.ResourceID, body.Purpose, body.Justification)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleRegisterSource(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body ports.SourceRegistration
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.RegisterSource(r.Context(), creds, orgID, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	items, err := s.App.ListSources(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleVerifySource(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	sourceID := r.PathValue("sourceId")
	creds := bearerCreds(r)
	out, err := s.App.VerifySource(r.Context(), creds, orgID, sourceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	sourceID := r.PathValue("sourceId")
	creds := bearerCreds(r)
	out, err := s.App.GetSource(r.Context(), creds, orgID, sourceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRotateSource(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	sourceID := r.PathValue("sourceId")
	creds := bearerCreds(r)
	out, err := s.App.RotateSourceSecret(r.Context(), creds, orgID, sourceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func parseIntakeEvent(raw []byte, idempotencyFallback string) (ports.IntakeEvent, map[string]any) {
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	var event ports.IntakeEvent
	_ = json.Unmarshal(raw, &event)
	if event.EventID == "" {
		if id, ok := payload["id"].(string); ok {
			event.EventID = id
		}
	}
	if event.SourceSystem == "" {
		if src, ok := payload["source"].(string); ok {
			event.SourceSystem = src
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		if event.ExternalID == "" {
			if v, ok := data["source_external_id"].(string); ok {
				event.ExternalID = v
			}
		}
		if event.SourceRevision == "" {
			if v, ok := data["source_revision"].(string); ok {
				event.SourceRevision = v
			}
		}
		if event.IdempotencyKey == "" {
			if v, ok := data["idempotency_key"].(string); ok {
				event.IdempotencyKey = v
			}
		}
		if event.ContentHash == "" {
			if ev, ok := data["evidence"].(map[string]any); ok {
				if v, ok := ev["sha256"].(string); ok {
					event.ContentHash = v
				}
			}
		}
		attrs := map[string]string{}
		for _, k := range []string{"trust", "authority", "source_authority", "classification", "resource_id", "resource_type"} {
			if v, ok := data[k].(string); ok {
				attrs[k] = v
			}
		}
		if len(attrs) > 0 {
			event.Attributes = attrs
		}
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = idempotencyFallback
	}
	return event, payload
}

func (s *Server) handleIntake(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, platform.ErrValidation("unable to read body"))
		return
	}
	event, payload := parseIntakeEvent(raw, r.Header.Get("Idempotency-Key"))
	sourceID := r.Header.Get("X-Context-Fabric-Source-Id")
	if sourceID == "" {
		sourceID = event.SourceSystem
	}

	out, err := s.App.Intake(r.Context(), creds, orgID, app.IntakeRequest{
		Event:          event,
		Body:           raw,
		Signature:      r.Header.Get("X-Context-Fabric-Signature"),
		Timestamp:      r.Header.Get("X-Context-Fabric-Timestamp"),
		IdempotencyKey: event.IdempotencyKey,
		SourceID:       sourceID,
		Payload:        payload,
		SkipHMAC:       r.Header.Get("X-Context-Fabric-Skip-HMAC") == "1" && allowSkipHMAC(),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusAccepted
	if dup, _ := out["duplicate"].(bool); dup {
		status = http.StatusOK
	}
	writeJSON(w, status, out)
}

func (s *Server) handleIntakeBatch(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeErr(w, platform.ErrValidation("unable to read body"))
		return
	}
	var events []json.RawMessage
	var wrapped struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(raw, &events); err != nil || len(events) == 0 {
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil || len(wrapped.Events) == 0 {
			writeErr(w, platform.ErrValidation("expected CloudEvents array or {events:[]}"))
			return
		}
		events = wrapped.Events
	}
	sourceID := r.Header.Get("X-Context-Fabric-Source-Id")
	sig := r.Header.Get("X-Context-Fabric-Signature")
	ts := r.Header.Get("X-Context-Fabric-Timestamp")
	skip := r.Header.Get("X-Context-Fabric-Skip-HMAC") == "1" && allowSkipHMAC()
	reqs := make([]app.IntakeRequest, 0, len(events))
	for _, evRaw := range events {
		event, payload := parseIntakeEvent(evRaw, r.Header.Get("Idempotency-Key"))
		sid := sourceID
		if sid == "" {
			sid = event.SourceSystem
		}
		reqs = append(reqs, app.IntakeRequest{
			Event:          event,
			Body:           evRaw,
			Signature:      sig,
			Timestamp:      ts,
			IdempotencyKey: event.IdempotencyKey,
			SourceID:       sid,
			Payload:        payload,
			SkipHMAC:       skip,
		})
	}
	out, err := s.App.IntakeBatch(r.Context(), creds, orgID, reqs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) handlePresign(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body struct {
		ObjectKey   string `json:"object_key"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.PresignEvidence(r.Context(), creds, orgID, body.ObjectKey, body.ContentType)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.ListChanges(r.Context(), creds, orgID, r.URL.Query().Get("cursor"), 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.GetAudit(r.Context(), creds, orgID, 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	out, err := s.App.ManageWebhooks(r.Context(), creds, orgID, "upsert", body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.ListWebhooks(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	subID := r.PathValue("subscriptionId")
	creds := bearerCreds(r)
	out, err := s.App.GetWebhook(r.Context(), creds, orgID, subID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListDeliveries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	subID := r.PathValue("subscriptionId")
	creds := bearerCreds(r)
	out, err := s.App.ListDeliveries(r.Context(), creds, orgID, subID, 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleWebhookReplay(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	subID := r.PathValue("subscriptionId")
	deliveryID := r.PathValue("deliveryId")
	creds := bearerCreds(r)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body == nil {
		body = map[string]any{}
	}
	body["subscription_id"] = subID
	if body["event_id"] == nil {
		body["event_id"] = deliveryID
	}
	out, err := s.App.ManageWebhooks(r.Context(), creds, orgID, "replay", body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.StartExport(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) handleGetExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	exportID := r.PathValue("exportId")
	creds := bearerCreds(r)
	out, err := s.App.GetExport(r.Context(), creds, orgID, exportID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var manifest export.Manifest
	if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.ImportExport(r.Context(), creds, orgID, manifest)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) handleGetQuotas(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	q, err := s.App.GetQuotas(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleSetQuotas(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var q ports.Quota
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.SetQuotas(r.Context(), creds, orgID, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDiagnose(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body struct {
		ResourceID string `json:"resource_id"`
		Action     string `json:"action"`
		Purpose    string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.DiagnoseDecision(r.Context(), creds, orgID, body.ResourceID, body.Action, body.Purpose)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDiagnoseAudit(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	auditID := r.PathValue("auditId")
	creds := bearerCreds(r)
	out, err := s.App.DiagnoseByAuditID(r.Context(), creds, orgID, auditID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = orgID
	}
	out, err := s.App.Bootstrap(r.Context(), creds, orgID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleOrgStatus(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.OrgStatus(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	var body ports.CreateAgentCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, platform.ErrValidation("invalid json"))
		return
	}
	out, err := s.App.CreateAgent(r.Context(), creds, orgID, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleRotateAgent(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	agentID := r.PathValue("agentId")
	creds := bearerCreds(r)
	out, err := s.App.RotateAgent(r.Context(), creds, orgID, agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeAgent(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	credID := r.PathValue("credentialId")
	creds := bearerCreds(r)
	out, err := s.App.RevokeAgent(r.Context(), creds, orgID, credID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOpsLag(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.OpsLag(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSupportBundle(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	creds := bearerCreds(r)
	out, err := s.App.SupportBundle(r.Context(), creds, orgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	ae, ok := platform.AsAPIError(err)
	if !ok {
		ae = platform.ErrUnavailable(err.Error())
	}
	status := ae.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	env := map[string]any{
		"error": map[string]any{
			"reason_code": ae.ReasonCode,
			"message":     ae.Message,
			"audit_id":    ae.AuditID,
			"trace_id":    ae.TraceID,
			"retryable":   ae.Retryable,
			"doc_url":     ae.DocURL,
		},
	}
	if ae.RetryAfter > 0 {
		env["error"].(map[string]any)["retry_after_seconds"] = int(ae.RetryAfter / time.Second)
	}
	writeJSON(w, status, env)
}
