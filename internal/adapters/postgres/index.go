package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/xsama/context-fabric/internal/ports"
)

// Index implements ports.IndexProvider against search_documents (durable shared projection).
type Index struct {
	pool *Pool
}

// NewIndex wraps a pool as IndexProvider.
func NewIndex(pool *Pool) *Index {
	return &Index{pool: pool}
}

var _ ports.IndexProvider = (*Index)(nil)

func (idx *Index) Upsert(ctx context.Context, docs []ports.IndexDocument) error {
	for _, d := range docs {
		if d.OrgID == "" || d.ResourceID == "" {
			continue
		}
		tags := d.Labels
		if tags == nil {
			tags = []string{}
		}
		err := idx.pool.WithTenant(ctx, d.OrgID, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
INSERT INTO search_documents (organization_id, resource_id, revision_id, text, tags, attributes, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,now())
ON CONFLICT (organization_id, resource_id) DO UPDATE SET
  revision_id=EXCLUDED.revision_id,
  text=EXCLUDED.text,
  tags=EXCLUDED.tags,
  attributes=EXCLUDED.attributes,
  updated_at=now()`,
				d.OrgID, d.ResourceID, d.RevisionID, d.Text, tags, mustJSON(d.Attributes))
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (idx *Index) Delete(ctx context.Context, orgID string, resourceIDs []string) error {
	if len(resourceIDs) == 0 {
		return nil
	}
	return idx.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
DELETE FROM search_documents WHERE organization_id=$1 AND resource_id = ANY($2)`,
			orgID, resourceIDs)
		return err
	})
}

func (idx *Index) SearchCandidates(ctx context.Context, orgID string, query string, limit int, filters map[string]string) ([]ports.SearchHit, error) {
	if limit <= 0 {
		limit = 12
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var hits []ports.SearchHit
	err := idx.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT resource_id, revision_id, text, tags, attributes
FROM search_documents WHERE organization_id=$1`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var resourceID, revisionID, text string
			var tags []string
			var attrsRaw []byte
			if err := rows.Scan(&resourceID, &revisionID, &text, &tags, &attrsRaw); err != nil {
				return err
			}
			attrs := map[string]string{}
			_ = json.Unmarshal(attrsRaw, &attrs)
			doc := ports.IndexDocument{
				OrgID:      orgID,
				ResourceID: resourceID,
				RevisionID: revisionID,
				Text:       text,
				Labels:     tags,
				Attributes: attrs,
			}
			if !matchIndexFilters(doc, filters) {
				continue
			}
			score := 0.0
			if q == "" {
				score = 1
			} else if rid := filters["resource_id"]; rid != "" && doc.ResourceID == rid {
				score = 1
			} else if strings.EqualFold(doc.ResourceID, q) {
				score = 1
			} else if strings.Contains(strings.ToLower(doc.Text), q) {
				score = 1
			} else {
				continue
			}
			hits = append(hits, ports.SearchHit{
				ResourceID: doc.ResourceID,
				RevisionID: doc.RevisionID,
				Score:      score,
			})
			if len(hits) >= limit {
				break
			}
		}
		return rows.Err()
	})
	return hits, err
}

// GetDocument implements retrieval.SnippetSource.
func (idx *Index) GetDocument(orgID, resourceID string) (ports.IndexDocument, bool) {
	var doc ports.IndexDocument
	err := idx.pool.WithTenant(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
		var tags []string
		var attrsRaw []byte
		err := tx.QueryRow(ctx, `
SELECT resource_id, revision_id, text, tags, attributes
FROM search_documents WHERE organization_id=$1 AND resource_id=$2`, orgID, resourceID).
			Scan(&doc.ResourceID, &doc.RevisionID, &doc.Text, &tags, &attrsRaw)
		if err != nil {
			return err
		}
		doc.OrgID = orgID
		doc.Labels = tags
		_ = json.Unmarshal(attrsRaw, &doc.Attributes)
		return nil
	})
	if err != nil {
		return ports.IndexDocument{}, false
	}
	return doc, true
}

func matchIndexFilters(d ports.IndexDocument, filters map[string]string) bool {
	if filters == nil {
		return true
	}
	if purpose := filters["purpose"]; purpose != "" {
		ok := false
		for _, l := range d.Labels {
			if l == "purpose:"+purpose || strings.EqualFold(l, purpose) {
				ok = true
				break
			}
		}
		if !ok && d.Attributes != nil {
			if allow := d.Attributes["purpose_allowlist"]; allow != "" {
				for _, p := range strings.Split(allow, ",") {
					if strings.TrimSpace(p) == purpose {
						ok = true
						break
					}
				}
			}
		}
		if !ok {
			return false
		}
	}
	if ceiling := filters["classification_ceiling"]; ceiling != "" {
		docClass := d.Attributes["classification"]
		if docClass == "" {
			docClass = "internal"
		}
		if classRank(docClass) > classRank(ceiling) {
			return false
		}
	}
	if tags := filters["include_tags"]; tags != "" {
		need := strings.Split(tags, ",")
		have := map[string]struct{}{}
		for _, l := range d.Labels {
			have[l] = struct{}{}
		}
		for _, t := range need {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if _, ok := have[t]; !ok {
				return false
			}
		}
	}
	if cs := filters["context_space_id"]; cs != "" {
		if d.Attributes["context_space_id"] != cs {
			return false
		}
	}
	if rid := filters["resource_id"]; rid != "" {
		if d.ResourceID != rid {
			return false
		}
	}
	return true
}

func classRank(c string) int {
	switch strings.ToLower(c) {
	case "public":
		return 1
	case "internal":
		return 2
	case "confidential":
		return 3
	case "restricted":
		return 4
	default:
		return 2
	}
}
