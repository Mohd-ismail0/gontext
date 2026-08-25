package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/authzsync"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const FormatVersion = "context-fabric.export/v1"

// Service builds and imports org-scoped export manifests (no secrets).
type Service struct {
	Ledger    ports.LedgerStore
	Evidence  ports.EvidenceStore // optional; when set, evidence refs are packed
	Now       func() time.Time
	RecordCap int // test override; default 10000
	EdgeCap   int // test override; default 50000
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
	Records     []ports.Record             `json:"records,omitempty"`
	Revisions   []ports.Revision           `json:"revisions,omitempty"`
	Sources     []ports.SourceRegistration `json:"sources,omitempty"`
	Tombstones  []Tombstone                `json:"tombstones,omitempty"`
	GraphEdges  []ports.GraphEdge          `json:"graph_edges,omitempty"`
	AuthzTuples []AuthzTupleManifest       `json:"authz_tuples,omitempty"`
	EvidenceRefs []EvidenceRef             `json:"evidence_refs,omitempty"`
}

// EvidenceRef is a portable pointer to an evidence object (no secret material).
type EvidenceRef struct {
	Key         string `json:"key"`
	ResourceID  string `json:"resource_id,omitempty"`
	RevisionID  string `json:"revision_id,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	ByteSize    int64  `json:"byte_size,omitempty"`
}

// AuthzTupleManifest is a portable AuthZ relationship required by sync_authz parent edges.
type AuthzTupleManifest struct {
	Operation string `json:"operation"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
	Subject   string `json:"subject"`
	EdgeID    string `json:"edge_id,omitempty"`
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

	recordHardCap := s.RecordCap
	if recordHardCap <= 0 {
		recordHardCap = 10_000
	}
	edgeHardCap := s.EdgeCap
	if edgeHardCap <= 0 {
		edgeHardCap = 50_000
	}

	records, _, err := s.Ledger.ListRecords(ctx, orgID, recordHardCap+1, "")
	if err != nil {
		return Manifest{}, err
	}
	if len(records) > recordHardCap {
		return Manifest{}, platform.ErrValidation(
			"export exceeds hard cap of 10000 records; use a bounded export job",
		)
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

	edges, err := s.Ledger.ListEdges(ctx, orgID, ports.EdgeListOptions{IncludeDead: true, Limit: edgeHardCap + 1})
	if err != nil {
		return Manifest{}, err
	}
	if len(edges) > edgeHardCap {
		return Manifest{}, platform.ErrValidation(
			"export exceeds hard cap of 50000 edges; use a bounded export job",
		)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeID < edges[j].EdgeID })

	authzTuples := make([]AuthzTupleManifest, 0)
	for _, e := range edges {
		if e.State != "ACTIVE" || !e.SyncAuthz || e.Predicate != ports.EdgeParent {
			continue
		}
		authzTuples = append(authzTuples, AuthzTupleManifest{
			Operation: "write",
			Object:    "resource:" + e.FromID,
			Relation:  "parent",
			Subject:   "resource:" + e.ToID,
			EdgeID:    e.EdgeID,
		})
	}
	sort.Slice(authzTuples, func(i, j int) bool {
		if authzTuples[i].Object != authzTuples[j].Object {
			return authzTuples[i].Object < authzTuples[j].Object
		}
		return authzTuples[i].Subject < authzTuples[j].Subject
	})

	evidenceRefs := collectEvidenceRefs(revisions)
	if s.Evidence != nil {
		for i := range evidenceRefs {
			key, ver := splitEvidenceRef(evidenceRefs[i].Key)
			rc, meta, err := s.Evidence.Get(ctx, key, ver)
			if err == nil {
				n, _ := io.Copy(io.Discard, io.LimitReader(rc, 64<<20))
				_ = rc.Close()
				evidenceRefs[i].ByteSize = n
				if meta.Key != "" {
					evidenceRefs[i].Key = meta.Key
					if meta.VersionID != "" {
						evidenceRefs[i].Key = meta.Key + "@" + meta.VersionID
					}
				}
			}
		}
	}
	sort.Slice(evidenceRefs, func(i, j int) bool { return evidenceRefs[i].Key < evidenceRefs[j].Key })

	recordsJSON := mustCanonicalJSON(records)
	revsJSON := mustCanonicalJSON(revisions)
	sourcesJSON := mustCanonicalJSON(sources)
	tombsJSON := mustCanonicalJSON(tombstones)
	edgesJSON := mustCanonicalJSON(edges)
	authzJSON := mustCanonicalJSON(authzTuples)
	evidenceJSON := mustCanonicalJSON(evidenceRefs)

	contents := []ContentEntry{
		entry("records.json", "records", recordsJSON, len(records)),
		entry("revisions.json", "events", revsJSON, len(revisions)),
		entry("sources.json", "source_registrations", sourcesJSON, len(sources)),
		entry("tombstones.json", "tombstones", tombsJSON, len(tombstones)),
		entry("graph_edges.json", "graph_edges", edgesJSON, len(edges)),
		entry("authz_tuples.json", "authz_tuples", authzJSON, len(authzTuples)),
		entry("evidence_refs.json", "evidence_refs", evidenceJSON, len(evidenceRefs)),
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
			"record":      "context-fabric.record/v1",
			"export":      FormatVersion,
			"packet":      "context-fabric.packet/v1",
			"graph_edge":  "context-fabric.graph-edge/v1",
			"authz_tuple": "context-fabric.authz-tuple/v1",
			"evidence":    "context-fabric.evidence-ref/v1",
		},
		Records:      records,
		Revisions:    revisions,
		Sources:      sources,
		Tombstones:   tombstones,
		GraphEdges:   edges,
		AuthzTuples:  authzTuples,
		EvidenceRefs: evidenceRefs,
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
		// Import edges after records so endpoint FKs / placeholders exist.
		seenAuthz := map[string]bool{}
		for _, e := range m.GraphEdges {
			e.OrgID = targetOrgID
			if err := s.Ledger.UpsertEdge(ctx, tx, e); err != nil {
				return err
			}
			if e.State == "ACTIVE" {
				now := s.now()
				if err := authzsync.EnqueueForEdge(ctx, s.Ledger, tx, e, authzsync.OperationWrite, now); err != nil {
					return err
				}
				if authzsync.NeedsSynchronization(e) {
					seenAuthz["resource:"+e.FromID+"|parent|resource:"+e.ToID] = true
				}
			}
		}
		// Replay tombstones after records so deleted visibility dominates.
		for _, t := range m.Tombstones {
			rec, err := s.Ledger.GetRecordTx(ctx, tx, targetOrgID, t.ResourceID)
			if err != nil {
				rec = ports.Record{
					ResourceID: t.ResourceID, OrgID: targetOrgID, Kind: "resource",
					Title: t.ResourceID, Classification: "internal",
				}
			}
			rec.State = t.State
			if rec.State == "" {
				rec.State = "TOMBSTONED"
			}
			if t.RevisionID != "" {
				rec.CurrentRevID = t.RevisionID
			}
			rec.UpdatedAt = t.At
			if err := s.Ledger.UpsertRecord(ctx, tx, rec); err != nil {
				return err
			}
		}
		// Standalone AuthZ tuple manifests not already covered by sync_authz edges.
		now := s.now()
		for _, tup := range m.AuthzTuples {
			key := tup.Object + "|" + tup.Relation + "|" + tup.Subject
			if seenAuthz[key] {
				continue
			}
			op := tup.Operation
			if op == "" {
				op = "write"
			}
			if err := s.Ledger.EnqueueAuthzTuple(ctx, tx, ports.AuthzTupleOp{
				ID: platform.NewEventID(), OrgID: targetOrgID, Operation: op,
				Object: tup.Object, Relation: tup.Relation, Subject: tup.Subject,
				EdgeID: tup.EdgeID, Status: "pending", CreatedAt: now, UpdatedAt: now, NextAttempt: now,
			}); err != nil {
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
		GraphEdges     []ports.GraphEdge          `json:"graph_edges"`
		AuthzTuples    []AuthzTupleManifest       `json:"authz_tuples"`
		EvidenceRefs   []EvidenceRef              `json:"evidence_refs"`
	}{
		FormatVersion:  m.FormatVersion,
		OrganizationID: m.OrganizationID,
		SchemaVersions: m.SchemaVersions,
		Records:        m.Records,
		Revisions:      m.Revisions,
		Sources:        m.Sources,
		Tombstones:     m.Tombstones,
		GraphEdges:     m.GraphEdges,
		AuthzTuples:    m.AuthzTuples,
		EvidenceRefs:   m.EvidenceRefs,
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

func collectEvidenceRefs(revs []ports.Revision) []EvidenceRef {
	seen := map[string]bool{}
	out := make([]EvidenceRef, 0)
	for _, r := range revs {
		ref := strings.TrimSpace(r.EvidenceRef)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, EvidenceRef{
			Key:         ref,
			ResourceID:  r.ResourceID,
			RevisionID:  r.RevisionID,
			ContentHash: r.ContentHash,
		})
	}
	return out
}

func splitEvidenceRef(ref string) (key, version string) {
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
