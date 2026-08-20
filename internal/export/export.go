package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const FormatVersion = "context-fabric.export/v1"

// Service builds and imports org-scoped export manifests (no secrets).
type Service struct {
	Ledger ports.LedgerStore
	Now    func() time.Time
}

// ContentEntry describes one hashed payload section.
type ContentEntry struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	SHA256      string `json:"sha256"`
	ByteSize    int    `json:"byte_size"`
	RecordCount int    `json:"record_count,omitempty"`
}

// Manifest is the portable export document.
type Manifest struct {
	FormatVersion  string            `json:"format_version"`
	ExportID       string            `json:"export_id"`
	OrganizationID string            `json:"organization_id"`
	CreatedAt      time.Time         `json:"created_at"`
	CreatedBy      string            `json:"created_by,omitempty"`
	Status         string            `json:"status"`
	Checksums      map[string]string `json:"checksums"`
	Contents       []ContentEntry    `json:"contents"`
	SchemaVersions map[string]string `json:"schema_versions,omitempty"`
	// Embedded payloads for memory/demo round-trip (never include secrets).
	Records    []ports.Record             `json:"records,omitempty"`
	Revisions  []ports.Revision           `json:"revisions,omitempty"`
	Sources    []ports.SourceRegistration `json:"sources,omitempty"`
	Tombstones []Tombstone                `json:"tombstones,omitempty"`
}

// Tombstone is a metadata-only deleted resource marker.
type Tombstone struct {
	ResourceID string    `json:"resource_id"`
	RevisionID string    `json:"revision_id"`
	State      string    `json:"state"`
	At         time.Time `json:"at"`
}

// Job is a running/completed export job view.
type Job struct {
	ExportID       string    `json:"export_id"`
	OrganizationID string    `json:"organization_id"`
	Status         string    `json:"status"`
	FormatVersion  string    `json:"format_version"`
	CreatedAt      time.Time `json:"created_at"`
	Manifest       *Manifest `json:"manifest,omitempty"`
}

// Build creates a completed export manifest for an organization.
func (s *Service) Build(ctx context.Context, orgID, createdBy string) (Manifest, error) {
	if orgID == "" {
		return Manifest{}, platform.ErrValidation("organization_id required")
	}
	now := s.now()
	exportID := platform.NewEventID()

	records, _, err := s.Ledger.ListRecords(ctx, orgID, 10_000, "")
	if err != nil {
		return Manifest{}, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ResourceID < records[j].ResourceID })

	revisions := make([]ports.Revision, 0)
	tombstones := make([]Tombstone, 0)
	for _, rec := range records {
		revs, err := s.Ledger.ListRevisions(ctx, orgID, rec.ResourceID, 10_000)
		if err != nil {
			return Manifest{}, err
		}
		sort.Slice(revs, func(i, j int) bool { return revs[i].RevisionID < revs[j].RevisionID })
		revisions = append(revisions, revs...)
		if isTomb(rec.State) {
			tombstones = append(tombstones, Tombstone{
				ResourceID: rec.ResourceID,
				RevisionID: rec.CurrentRevID,
				State:      rec.State,
				At:         rec.UpdatedAt,
			})
		}
	}
	sort.Slice(tombstones, func(i, j int) bool { return tombstones[i].ResourceID < tombstones[j].ResourceID })

	sources, err := s.Ledger.ListSources(ctx, orgID)
	if err != nil {
		return Manifest{}, err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].SourceID < sources[j].SourceID })
	// Strip secret-bearing attributes.
	for i := range sources {
		sources[i].Attributes = scrubSecrets(sources[i].Attributes)
	}
	for i := range records {
		records[i].Attributes = scrubSecrets(records[i].Attributes)
	}

	recordsJSON := mustCanonicalJSON(records)
	revsJSON := mustCanonicalJSON(revisions)
	sourcesJSON := mustCanonicalJSON(sources)
	tombsJSON := mustCanonicalJSON(tombstones)

	contents := []ContentEntry{
		entry("records.json", "records", recordsJSON, len(records)),
		entry("revisions.json", "events", revsJSON, len(revisions)),
		entry("sources.json", "source_registrations", sourcesJSON, len(sources)),
		entry("tombstones.json", "tombstones", tombsJSON, len(tombstones)),
	}

	m := Manifest{
		FormatVersion:  FormatVersion,
		ExportID:       exportID,
		OrganizationID: orgID,
		CreatedAt:      now,
		CreatedBy:      createdBy,
		Status:         "completed",
		Contents:       contents,
		SchemaVersions: map[string]string{
			"record":  "context-fabric.record/v1",
			"export":  FormatVersion,
			"packet":  "context-fabric.packet/v1",
		},
		Records:    records,
		Revisions:  revisions,
		Sources:    sources,
		Tombstones: tombstones,
	}
	m.Checksums = map[string]string{
		"manifest_sha256": HashManifest(m),
	}
	return m, nil
}

// ImportInto loads a manifest into an isolated target organization with hash verification.
func (s *Service) ImportInto(ctx context.Context, targetOrgID string, m Manifest) error {
	if targetOrgID == "" {
		return platform.ErrValidation("target organization_id required")
	}
	if m.FormatVersion != FormatVersion {
		return platform.ErrValidation("unsupported export format_version")
	}
	want := m.Checksums["manifest_sha256"]
	got := HashManifest(m)
	if want == "" || want != got {
		return platform.ErrValidation("manifest hash verification failed")
	}

	if _, err := s.Ledger.GetOrganization(ctx, targetOrgID); err != nil {
		_ = s.Ledger.CreateOrganization(ctx, ports.Organization{
			ID: targetOrgID, Name: targetOrgID, CreatedAt: s.now(),
		})
	}

	return s.Ledger.WithOrgTx(ctx, targetOrgID, func(ctx context.Context, tx ports.Tx) error {
		for _, rec := range m.Records {
			rec.OrgID = targetOrgID
			rec.Attributes = scrubSecrets(rec.Attributes)
			if err := s.Ledger.UpsertRecord(ctx, tx, rec); err != nil {
				return err
			}
		}
		for _, rev := range m.Revisions {
			rev.OrgID = targetOrgID
			if err := s.Ledger.AppendRevision(ctx, tx, rev); err != nil {
				return err
			}
		}
		for _, src := range m.Sources {
			src.OrgID = targetOrgID
			src.Attributes = scrubSecrets(src.Attributes)
			if err := s.Ledger.UpsertSource(ctx, tx, src); err != nil {
				return err
			}
		}
		return nil
	})
}

// HashManifest computes a stable sha256 over canonical export payload (excluding checksums/export_id timestamps variance).
// For round-trip stability we hash records+revisions+sources+tombstones+schema_versions only.
func HashManifest(m Manifest) string {
	payload := struct {
		FormatVersion  string                     `json:"format_version"`
		OrganizationID string                     `json:"organization_id"`
		SchemaVersions map[string]string          `json:"schema_versions"`
		Records        []ports.Record             `json:"records"`
		Revisions      []ports.Revision           `json:"revisions"`
		Sources        []ports.SourceRegistration `json:"sources"`
		Tombstones     []Tombstone                `json:"tombstones"`
	}{
		FormatVersion:  m.FormatVersion,
		OrganizationID: m.OrganizationID,
		SchemaVersions: m.SchemaVersions,
		Records:        m.Records,
		Revisions:      m.Revisions,
		Sources:        m.Sources,
		Tombstones:     m.Tombstones,
	}
	raw := mustCanonicalJSON(payload)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func entry(path, kind string, raw []byte, count int) ContentEntry {
	sum := sha256.Sum256(raw)
	return ContentEntry{
		Path:        path,
		Kind:        kind,
		SHA256:      "sha256:" + hex.EncodeToString(sum[:]),
		ByteSize:    len(raw),
		RecordCount: count,
	}
}

func mustCanonicalJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	var buf bytes.Buffer
	_ = json.Compact(&buf, raw)
	return buf.Bytes()
}

func scrubSecrets(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		lk := k
		switch lk {
		case "secret", "token", "api_key", "password", "authorization", "bearer", "client_secret", "signing_secret":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

func isTomb(state string) bool {
	switch state {
	case "TOMBSTONED", "PURGE_PENDING", "PURGED":
		return true
	default:
		return false
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
