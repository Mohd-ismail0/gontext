// Package ingest implements generic CloudEvents intake with HMAC, mapping, and outbox.
package ingest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/mapping"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const (
	HeaderSignature = "X-Context-Fabric-Signature"
	HeaderTimestamp = "X-Context-Fabric-Timestamp"
	HeaderKeyID     = "X-Context-Fabric-Key-Id"
	DefaultReplay   = 300
	OutboxSubject   = "context.work.v1"
)

// IntakeService validates, quarantines, maps, and commits intake events.
type IntakeService struct {
	Ledger   ports.LedgerStore
	Evidence ports.EvidenceStore
	Now      func() time.Time
}

// Request is a single intake attempt.
type Request struct {
	OrgID          string
	SourceID       string
	Body           []byte
	Signature      string
	Timestamp      string
	IdempotencyKey string
	Mapping        mapping.Spec
	Payload        map[string]any
	Event          ports.IntakeEvent
	DryRun         bool
	SkipHMAC       bool // tests only when source has no secret
}

// Result is the intake outcome.
type Result struct {
	EventID          string `json:"event_id"`
	OrganizationID   string `json:"organization_id"`
	ResourceID       string `json:"resource_id,omitempty"`
	RevisionID       string `json:"revision_id,omitempty"`
	Status           string `json:"status"` // accepted | duplicate | quarantined
	Duplicate        bool   `json:"duplicate"`
	IdempotentReplay bool   `json:"idempotent_replay"`
	EvidenceRef      string `json:"evidence_ref,omitempty"`
	ContentHash      string `json:"content_hash,omitempty"`
}

// Intake runs the generic ingestion path.
func (s *IntakeService) Intake(ctx context.Context, req Request) (Result, error) {
	now := s.now()
	if req.OrgID == "" {
		return Result{}, platform.ErrValidation("organization_id required")
	}
	if req.SourceID == "" {
		req.SourceID = req.Event.SourceSystem
	}
	if req.SourceID == "" {
		return Result{}, platform.ErrValidation("source_id required")
	}

	src, err := s.Ledger.GetSource(ctx, req.OrgID, req.SourceID)
	if err != nil {
		return Result{}, err
	}
	if !src.Enabled {
		return Result{}, platform.ErrForbidden("source disabled")
	}
	if src.OrgID != "" && src.OrgID != req.OrgID {
		return Result{}, platform.ErrForbidden("source organization mismatch")
	}

	body := req.Body
	if len(body) == 0 && len(req.Payload) > 0 {
		body, _ = json.Marshal(req.Payload)
	}
	if len(body) == 0 {
		body, _ = json.Marshal(req.Event)
	}

	secret := src.SigningSecret
	if secret == "" && src.Attributes != nil {
		secret = src.Attributes["signing_secret"]
	}
	if !req.SkipHMAC {
		if secret == "" {
			return Result{}, platform.ErrUnauthorized("source signing secret not configured")
		}
		if err := VerifyHMAC(secret, body, req.Signature); err != nil {
			return Result{}, err
		}
		window := src.ReplayWindowSeconds
		if window <= 0 {
			if v := src.Attributes["replay_window_seconds"]; v != "" {
				window, _ = strconv.Atoi(v)
			}
		}
		if window <= 0 {
			window = DefaultReplay
		}
		if err := CheckReplayWindow(req.Timestamp, now, time.Duration(window)*time.Second); err != nil {
			return Result{}, err
		}
	}

	idemKey := req.IdempotencyKey
	if idemKey == "" {
		idemKey = req.Event.IdempotencyKey
	}
	if idemKey == "" && req.Event.EventID != "" {
		idemKey = req.Event.EventID
	}
	if idemKey != "" {
		prev, err := s.Ledger.GetIdempotency(ctx, req.OrgID, idemKey)
		if err == nil {
			return Result{
				EventID:          prev.EventID,
				OrganizationID:   req.OrgID,
				ResourceID:       prev.ResourceID,
				RevisionID:       prev.RevisionID,
				Status:           "duplicate",
				Duplicate:        true,
				IdempotentReplay: true,
			}, nil
		}
		if !platform.IsAPIError(err) {
			return Result{}, err
		}
	}

	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
		_ = json.Unmarshal(body, &payload)
	}

	sum := sha256.Sum256(body)
	contentHash := "sha256:" + hex.EncodeToString(sum[:])

	evidenceKey := req.OrgID + "/quarantine/" + contentHash
	var evidenceRef string
	if s.Evidence != nil && !req.DryRun {
		obj, err := s.Evidence.Put(ctx, evidenceKey, bytes.NewReader(body), "application/json", map[string]string{
			"organization_id": req.OrgID,
			"source_id":       req.SourceID,
			"content_hash":    contentHash,
		})
		if err != nil {
			return Result{}, err
		}
		evidenceRef = obj.Key
		if obj.VersionID != "" {
			evidenceRef = obj.Key + "@" + obj.VersionID
		}
	}

	ceilings := mapping.SourceCeilings{
		OrganizationID:        req.OrgID,
		TrustCeiling:          src.EffectiveTrustCeiling(),
		AuthorityCeiling:      firstNonEmpty(src.AuthorityCeiling, "source_of_truth"),
		ClassificationCeiling: src.EffectiveClassificationCeiling(),
		AllowedVisibilityRefs: src.AllowedVisibilityRefs,
		AllowedRecordTypes:    src.AllowedRecordTypes,
	}
	spec := req.Mapping
	if spec.ID == "" && src.MappingSpec != "" {
		spec.ID = src.MappingSpec
		spec.OrganizationID = req.OrgID
	}
	if spec.Mappings.SourceExternalID == nil && spec.Mappings.ResourceType == nil {
		// zero mapping: derive from event / payload defaults
		spec = defaultSpec(req)
	}

	canonical, err := mapping.Apply(spec, payload, ceilings, mapping.Options{DryRun: req.DryRun})
	if err != nil {
		return Result{}, err
	}
	if req.DryRun {
		return Result{
			EventID:        firstNonEmpty(req.Event.EventID, platform.NewEventID()),
			OrganizationID: req.OrgID,
			ResourceID:     canonical.ResourceID,
			Status:         "accepted",
			ContentHash:    contentHash,
			EvidenceRef:    evidenceRef,
		}, nil
	}

	eventID := req.Event.EventID
	if eventID == "" {
		eventID = platform.NewEventID()
	}
	rec := canonical.ToRecord(req.OrgID, firstNonEmpty(src.System, req.SourceID))
	if req.Event.ExternalID != "" && rec.ExternalID == "" {
		rec.ExternalID = req.Event.ExternalID
	}

	generated := strings.EqualFold(canonical.Trust, "generated") ||
		strings.EqualFold(canonical.Authority, "user_claim") ||
		strings.EqualFold(canonical.Authority, "inferred") ||
		strings.EqualFold(rec.Kind, "observation")

	existing, getErr := s.Ledger.GetRecord(ctx, req.OrgID, rec.ResourceID)
	hasExisting := getErr == nil
	if generated && hasExisting {
		if isSourceOfTruth(existing) {
			// Generated observations cannot overwrite source_of_truth facts.
			if wouldOverwriteSoT(existing, rec) {
				return Result{}, platform.ErrForbidden("generated observation cannot overwrite source_of_truth")
			}
			rec = preserveSoT(existing, rec)
		}
	}

	rev := ports.Revision{
		RevisionID:  platform.NewRevisionID(),
		ResourceID:  rec.ResourceID,
		OrgID:       req.OrgID,
		ContentHash: firstNonEmpty(req.Event.ContentHash, contentHash),
		EvidenceRef: firstNonEmpty(evidenceRef, req.Event.EvidenceRef, canonical.ContentLocator),
		State:       "ACCEPTED",
		Attributes:  map[string]string{"trust": canonical.Trust, "authority": canonical.Authority},
		ObservedAt:  canonical.ObservedAt,
		CreatedAt:   now,
	}
	if generated {
		rev.Attributes["observation"] = "generated"
	}
	rec.CurrentRevID = rev.RevisionID
	if !hasExisting {
		rec.CreatedAt = now
	} else {
		rec.CreatedAt = existing.CreatedAt
	}
	rec.UpdatedAt = now

	outboxPayload, _ := json.Marshal(map[string]any{
		"event_id":    eventID,
		"org_id":      req.OrgID,
		"resource_id": rec.ResourceID,
		"revision_id": rev.RevisionID,
		"source_id":   req.SourceID,
	})

	err = s.Ledger.WithOrgTx(ctx, req.OrgID, func(ctx context.Context, tx ports.Tx) error {
		if err := s.Ledger.UpsertRecord(ctx, tx, rec); err != nil {
			return err
		}
		if err := s.Ledger.AppendRevision(ctx, tx, rev); err != nil {
			return err
		}
		if err := s.Ledger.EnqueueOutbox(ctx, tx, ports.OutboxEntry{
			ID:      eventID,
			OrgID:   req.OrgID,
			Subject: OutboxSubject,
			Payload: outboxPayload,
			Headers: map[string]string{"Nats-Msg-Id": eventID, "event_id": eventID},
			CreatedAt: now,
		}); err != nil {
			return err
		}
		if idemKey != "" {
			return s.Ledger.PutIdempotency(ctx, tx, ports.IdempotencyRecord{
				OrgID:          req.OrgID,
				IdempotencyKey: idemKey,
				EventID:        eventID,
				ResourceID:     rec.ResourceID,
				RevisionID:     rev.RevisionID,
				CreatedAt:      now,
			})
		}
		return nil
	})
	if err != nil {
		if platform.IsAPIError(err) {
			ae, _ := platform.AsAPIError(err)
			if ae != nil && ae.HTTPStatus == 409 {
				prev, gerr := s.Ledger.GetIdempotency(ctx, req.OrgID, idemKey)
				if gerr == nil {
					return Result{
						EventID: prev.EventID, OrganizationID: req.OrgID,
						ResourceID: prev.ResourceID, RevisionID: prev.RevisionID,
						Status: "duplicate", Duplicate: true, IdempotentReplay: true,
					}, nil
				}
			}
		}
		return Result{}, err
	}

	return Result{
		EventID:        eventID,
		OrganizationID: req.OrgID,
		ResourceID:     rec.ResourceID,
		RevisionID:     rev.RevisionID,
		Status:         "accepted",
		EvidenceRef:    rev.EvidenceRef,
		ContentHash:    rev.ContentHash,
	}, nil
}

// VerifyHMAC checks hex-encoded HMAC-SHA256(secret, body).
func VerifyHMAC(secret string, body []byte, signature string) error {
	sig := strings.TrimSpace(signature)
	sig = strings.TrimPrefix(sig, "sha256=")
	sig = strings.TrimPrefix(sig, "v1=")
	if sig == "" {
		return platform.ErrUnauthorized("missing signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(want))) {
		return platform.ErrUnauthorized("invalid HMAC signature")
	}
	return nil
}

// SignHMAC returns hex HMAC-SHA256(secret, body).
func SignHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// CheckReplayWindow validates timestamp within +/- window of now.
func CheckReplayWindow(ts string, now time.Time, window time.Duration) error {
	if strings.TrimSpace(ts) == "" {
		return platform.ErrUnauthorized("missing timestamp")
	}
	var t time.Time
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		// unix seconds or millis
		if n > 1_000_000_000_000 {
			t = time.UnixMilli(n).UTC()
		} else {
			t = time.Unix(n, 0).UTC()
		}
	} else if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		t = parsed.UTC()
	} else if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
		t = parsed.UTC()
	} else {
		return platform.ErrUnauthorized("invalid timestamp")
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = -delta
	}
	if delta > window {
		return platform.ErrUnauthorized("timestamp outside replay window")
	}
	return nil
}

func (s *IntakeService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func defaultSpec(req Request) mapping.Spec {
	rules := map[string]string{}
	if req.Event.ExternalID != "" {
		rules["source_external_id"] = `"` + req.Event.ExternalID + `"`
	} else {
		rules["source_external_id"] = `$.id`
	}
	if req.Event.SourceRevision != "" {
		rules["source_revision"] = `"` + req.Event.SourceRevision + `"`
	} else {
		rules["source_revision"] = `$.id`
	}
	rules["resource_type"] = `"event"`
	rules["context_space_id"] = `"default"`
	rules["content_locator"] = `""`
	if req.Event.Attributes != nil {
		if v := req.Event.Attributes["trust"]; v != "" {
			rules["trust"] = `"` + v + `"`
		}
		if v := req.Event.Attributes["authority"]; v != "" {
			rules["source_authority"] = `"` + v + `"`
		}
		if v := req.Event.Attributes["classification"]; v != "" {
			rules["classification"] = `"` + v + `"`
		}
		if v := req.Event.Attributes["resource_id"]; v != "" {
			rules["resource_id"] = `"` + v + `"`
		}
		if v := req.Event.Attributes["resource_type"]; v != "" {
			rules["resource_type"] = `"` + v + `"`
		}
	}
	return mapping.SpecFromPorts(ports.MappingSpec{
		OrgID: req.OrgID,
		Rules: rules,
	})
}

func isSourceOfTruth(rec ports.Record) bool {
	if rec.Attributes == nil {
		return false
	}
	a := rec.Attributes["authority"]
	if a == "" {
		a = rec.Attributes["source_authority"]
	}
	return strings.EqualFold(a, "source_of_truth")
}

func wouldOverwriteSoT(existing, incoming ports.Record) bool {
	if incoming.Attributes != nil {
		a := incoming.Attributes["authority"]
		if a == "" {
			a = incoming.Attributes["source_authority"]
		}
		if strings.EqualFold(a, "source_of_truth") {
			return true
		}
	}
	// Changing authoritative title/classification via generated write is overwrite.
	if incoming.Title != "" && existing.Title != "" && incoming.Title != existing.Title {
		return true
	}
	if incoming.Classification != "" && existing.Classification != "" && incoming.Classification != existing.Classification {
		return true
	}
	return false
}

func preserveSoT(existing, incoming ports.Record) ports.Record {
	incoming.Title = existing.Title
	incoming.Classification = existing.Classification
	incoming.Labels = existing.Labels
	incoming.Kind = existing.Kind
	incoming.ExternalID = existing.ExternalID
	incoming.SourceSystem = existing.SourceSystem
	if existing.Attributes != nil {
		if incoming.Attributes == nil {
			incoming.Attributes = map[string]string{}
		}
		for k, v := range existing.Attributes {
			if k == "authority" || k == "source_authority" || k == "trust" || k == "visibility_ref" {
				incoming.Attributes[k] = v
			}
		}
	}
	return incoming
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// HashBody returns sha256 hex of r (and restores by reading all).
func HashBody(r io.Reader) (string, []byte, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), b, nil
}
