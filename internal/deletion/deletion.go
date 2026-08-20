package deletion

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xsama/context-fabric/internal/audit"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const ManifestVersion = "context-fabric.deletion/v1"

// ChangeAppender emits metadata-only change events.
type ChangeAppender interface {
	AppendChange(ctx context.Context, ev ports.ChangeEvent) error
}

// LegalHoldChecker reports whether evidence erase is blocked.
type LegalHoldChecker interface {
	HasLegalHold(ctx context.Context, orgID, resourceID string) (bool, error)
}

// Service runs the deletion saga.
// authorize → revoke visibility (tombstone) → enumerate derivatives →
// remove index/cache → evidence erase or blocked → signed completion → change event.
type Service struct {
	Ledger   ports.LedgerStore
	Evidence ports.EvidenceStore
	Index    ports.IndexProvider
	Authz    ports.AuthorizationProvider
	Audit    audit.Logger
	Changes  ChangeAppender
	Holds    LegalHoldChecker
	SignKey  []byte
	Now      func() time.Time
}

// Request starts a deletion for one resource.
type Request struct {
	OrgID      string
	ResourceID string
	Principal  ports.Principal
	Reason     string
}

// DerivativeRef is an enumerated derived artifact.
type DerivativeRef struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

// CompletionManifest is the signed deletion outcome (no content).
type CompletionManifest struct {
	FormatVersion  string          `json:"format_version"`
	DeletionID     string          `json:"deletion_id"`
	OrganizationID string          `json:"organization_id"`
	ResourceID     string          `json:"resource_id"`
	RevisionID     string          `json:"revision_id"`
	Status         string          `json:"status"` // completed | blocked
	BlockedReason  string          `json:"blocked_reason,omitempty"`
	Derivatives    []DerivativeRef `json:"derivatives"`
	IndexRemoved   bool            `json:"index_removed"`
	EvidenceErased bool            `json:"evidence_erased"`
	CompletedAt    time.Time       `json:"completed_at"`
	Signature      string          `json:"signature"`
}

// Run executes the deletion saga. Visibility is revoked before projection cleanup.
func (s *Service) Run(ctx context.Context, req Request) (CompletionManifest, error) {
	if req.OrgID == "" || req.ResourceID == "" {
		return CompletionManifest{}, platform.ErrValidation("org and resource required")
	}
	now := s.now()

	dec, err := s.Authz.Check(ctx, ports.AuthzCheck{
		Principal:   req.Principal,
		Action:      "can_delete",
		ResourceID:  req.ResourceID,
		Consistency: ports.ConsistencyFullyConsistent,
	})
	if err != nil {
		return CompletionManifest{}, err
	}
	if !dec.Allowed {
		dec2, err2 := s.Authz.Check(ctx, ports.AuthzCheck{
			Principal:   req.Principal,
			Action:      "can_admin",
			ResourceID:  req.ResourceID,
			Consistency: ports.ConsistencyFullyConsistent,
		})
		if err2 != nil {
			return CompletionManifest{}, err2
		}
		if !dec2.Allowed {
			return CompletionManifest{}, platform.ErrForbidden("deletion not authorized")
		}
	}

	rec, err := s.Ledger.GetRecord(ctx, req.OrgID, req.ResourceID)
	if err != nil {
		return CompletionManifest{}, err
	}

	deletionID := platform.NewEventID()
	tombRevID := platform.NewRevisionID()
	tomb := ports.Revision{
		RevisionID: tombRevID,
		ResourceID: req.ResourceID,
		OrgID:      req.OrgID,
		State:      "TOMBSTONED",
		Sequence:   time.Now().UnixNano(),
		Attributes: map[string]string{"deletion_id": deletionID, "reason": req.Reason},
		ObservedAt: now,
		CreatedAt:  now,
	}
	rec.State = "TOMBSTONED"
	rec.CurrentRevID = tombRevID
	rec.UpdatedAt = now

	err = s.Ledger.WithOrgTx(ctx, req.OrgID, func(ctx context.Context, tx ports.Tx) error {
		if err := s.Ledger.AppendRevision(ctx, tx, tomb); err != nil {
			return err
		}
		return s.Ledger.UpsertRecord(ctx, tx, rec)
	})
	if err != nil {
		return CompletionManifest{}, err
	}

	_ = s.Audit.Append(ctx, ports.AuditEvent{
		AuditID: platform.NewEventID(), OrgID: req.OrgID,
		PrincipalID: req.Principal.ID, PrincipalKind: req.Principal.Kind,
		Action: "context.delete", ReasonCode: "VISIBILITY_REVOKED",
		ResourceIDsSample: []string{req.ResourceID},
		Attributes:        map[string]string{"deletion_id": deletionID, "revision_id": tombRevID},
		CreatedAt:         now,
	})

	derivatives := s.enumerate(ctx, req.OrgID, req.ResourceID, rec)

	indexRemoved := false
	if s.Index != nil {
		if err := s.Index.Delete(ctx, req.OrgID, []string{req.ResourceID}); err != nil {
			return CompletionManifest{}, err
		}
		indexRemoved = true
	}

	evidenceErased := false
	blockedReason := ""
	status := "completed"
	hold := false
	if s.Holds != nil {
		hold, _ = s.Holds.HasLegalHold(ctx, req.OrgID, req.ResourceID)
	}
	if hold {
		status = "blocked"
		blockedReason = "LEGAL_HOLD"
		rec.State = "PURGE_PENDING"
		_ = s.Ledger.WithOrgTx(ctx, req.OrgID, func(ctx context.Context, tx ports.Tx) error {
			return s.Ledger.UpsertRecord(ctx, tx, rec)
		})
	} else {
		for _, d := range derivatives {
			if d.Kind != "evidence" || s.Evidence == nil {
				continue
			}
			key, ver := splitEvidenceRef(d.Ref)
			_ = s.Evidence.DeleteVersion(ctx, key, ver)
			evidenceErased = true
		}
		rec.State = "PURGED"
		_ = s.Ledger.WithOrgTx(ctx, req.OrgID, func(ctx context.Context, tx ports.Tx) error {
			return s.Ledger.UpsertRecord(ctx, tx, rec)
		})
	}

	manifest := CompletionManifest{
		FormatVersion:  ManifestVersion,
		DeletionID:     deletionID,
		OrganizationID: req.OrgID,
		ResourceID:     req.ResourceID,
		RevisionID:     tombRevID,
		Status:         status,
		BlockedReason:  blockedReason,
		Derivatives:    derivatives,
		IndexRemoved:   indexRemoved,
		EvidenceErased: evidenceErased,
		CompletedAt:    s.now(),
	}
	manifest.Signature = s.sign(manifest)

	if s.Changes != nil {
		action := "resource.tombstoned"
		if status == "completed" {
			action = "resource.purged"
		}
		_ = s.Changes.AppendChange(ctx, ports.ChangeEvent{
			EventID:    platform.NewEventID(),
			OrgID:      req.OrgID,
			ResourceID: req.ResourceID,
			RevisionID: tombRevID,
			Action:     action,
			Cursor:     tombRevID,
			OccurredAt: manifest.CompletedAt,
		})
	}

	_ = s.Audit.Append(ctx, ports.AuditEvent{
		AuditID: platform.NewEventID(), OrgID: req.OrgID,
		PrincipalID: req.Principal.ID, PrincipalKind: req.Principal.Kind,
		Action: "context.delete", ReasonCode: "DELETION_" + status,
		ResourceIDsSample: []string{req.ResourceID},
		Attributes:        map[string]string{"deletion_id": deletionID, "status": status},
		CreatedAt:         s.now(),
	})

	return manifest, nil
}

func (s *Service) enumerate(ctx context.Context, orgID, resourceID string, rec ports.Record) []DerivativeRef {
	out := make([]DerivativeRef, 0, 8)
	out = append(out, DerivativeRef{Kind: "record", Ref: resourceID})
	if rec.CurrentRevID != "" {
		out = append(out, DerivativeRef{Kind: "revision", Ref: rec.CurrentRevID})
	}
	revs, err := s.Ledger.ListRevisions(ctx, orgID, resourceID, 100)
	if err == nil {
		for _, r := range revs {
			if r.EvidenceRef != "" {
				out = append(out, DerivativeRef{Kind: "evidence", Ref: r.EvidenceRef})
			}
			out = append(out, DerivativeRef{Kind: "revision", Ref: r.RevisionID})
		}
	}
	out = append(out, DerivativeRef{Kind: "index", Ref: resourceID})
	out = append(out, DerivativeRef{Kind: "cache", Ref: resourceID})
	return out
}

func (s *Service) sign(m CompletionManifest) string {
	cp := m
	cp.Signature = ""
	raw, _ := json.Marshal(cp)
	key := s.SignKey
	if len(key) == 0 {
		key = []byte("context-fabric-deletion")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	return "sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func splitEvidenceRef(ref string) (key, version string) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '#' {
			return ref[:i], ref[i+1:]
		}
	}
	return ref, ""
}

// VerifySignature checks a completion manifest HMAC.
func VerifySignature(key []byte, m CompletionManifest) bool {
	want := m.Signature
	cp := m
	cp.Signature = ""
	raw, _ := json.Marshal(cp)
	if len(key) == 0 {
		key = []byte("context-fabric-deletion")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	got := "sha256:" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(want))
}

// IsTombstoned reports whether a record state must dominate search.
func IsTombstoned(state string) bool {
	switch state {
	case "TOMBSTONED", "PURGE_PENDING", "PURGED":
		return true
	default:
		return false
	}
}

// FormatDeletionID is a helper for logs/tests.
func FormatDeletionID(id string) string {
	return fmt.Sprintf("deletion:%s", id)
}
