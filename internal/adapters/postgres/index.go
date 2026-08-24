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
		title := attr(d.Attributes, "title")
		kind := attr(d.Attributes, "kind")
		cs := attr(d.Attributes, "context_space_id")
		summary := attr(d.Attributes, "summary")
		if summary == "" {
			summary = truncate(d.Text, 512)
		}
		err := idx.pool.WithTenant(ctx, d.OrgID, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
INSERT INTO search_documents (
  organization_id, resource_id, revision_id, text, tags, attributes,
  title, kind, context_space_id, summary, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
ON CONFLICT (organization_id, resource_id) DO UPDATE SET
  revision_id=EXCLUDED.revision_id,
  text=EXCLUDED.text,
  tags=EXCLUDED.tags,
  attributes=EXCLUDED.attributes,
  title=EXCLUDED.title,
  kind=EXCLUDED.kind,
  context_space_id=EXCLUDED.context_space_id,
  summary=EXCLUDED.summary,
  updated_at=now()`,
				d.OrgID, d.ResourceID, d.RevisionID, d.Text, tags, mustJSON(d.Attributes),
				title, kind, cs, summary)
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
	q := strings.TrimSpace(query)
	var hits []ports.SearchHit
	err := idx.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		// Known-ID / resource_id filter: direct lookup, no FTS needed.
		if filters != nil {
			if rid := filters["resource_id"]; rid != "" {
				var resourceID, revisionID string
				err := tx.QueryRow(ctx, `
SELECT resource_id, revision_id FROM search_documents
WHERE organization_id=$1 AND resource_id=$2`, orgID, rid).Scan(&resourceID, &revisionID)
				if err == nil {
					hits = append(hits, ports.SearchHit{ResourceID: resourceID, RevisionID: revisionID, Score: 1})
				}
				return nil
			}
		}

		args := []any{orgID}
		where := []string{"organization_id=$1"}
		argN := 2
		if filters != nil {
			if cs := filters["context_space_id"]; cs != "" {
				where = append(where, "context_space_id=$"+itoa(argN))
				args = append(args, cs)
				argN++
			}
			if tags := filters["include_tags"]; tags != "" {
				need := splitTrim(tags)
				if len(need) > 0 {
					where = append(where, "tags @> $"+itoa(argN)+"::text[]")
					args = append(args, need)
					argN++
				}
			}
		}

		order := "updated_at DESC"
		selectScore := "1.0::float8 AS score"
		if q != "" {
			args = append(args, q)
			tsIdx := argN
			argN++
			// Exact resource_id match ranks highest; else ranked FTS.
			where = append(where, `(resource_id = $`+itoa(tsIdx)+` OR search_tsv @@ plainto_tsquery('english', $`+itoa(tsIdx)+`))`)
			selectScore = `CASE WHEN resource_id = $` + itoa(tsIdx) + ` THEN 1.0
				ELSE ts_rank_cd(search_tsv, plainto_tsquery('english', $` + itoa(tsIdx) + `)) END AS score`
			order = "score DESC, updated_at DESC"
		}

		args = append(args, limit)
		limitIdx := argN
		sql := `SELECT resource_id, revision_id, ` + selectScore + `
FROM search_documents WHERE ` + strings.Join(where, " AND ") + `
ORDER BY ` + order + ` LIMIT $` + itoa(limitIdx)

		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			// Fallback for pre-006 schemas without search_tsv: bounded substring scan.
			return idx.fallbackScan(ctx, tx, orgID, q, limit, filters, &hits)
		}
		defer rows.Close()
		for rows.Next() {
			var resourceID, revisionID string
			var score float64
			if err := rows.Scan(&resourceID, &revisionID, &score); err != nil {
				return err
			}
			hits = append(hits, ports.SearchHit{
				ResourceID: resourceID,
				RevisionID: revisionID,
				Score:      score,
			})
		}
		return rows.Err()
	})
	return hits, err
}

func (idx *Index) fallbackScan(ctx context.Context, tx pgx.Tx, orgID, q string, limit int, filters map[string]string, hits *[]ports.SearchHit) error {
	rows, err := tx.Query(ctx, `
SELECT resource_id, revision_id, text, tags, attributes
FROM search_documents WHERE organization_id=$1`, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ql := strings.ToLower(q)
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
			OrgID: orgID, ResourceID: resourceID, RevisionID: revisionID,
			Text: text, Labels: tags, Attributes: attrs,
		}
		if !matchIndexFilters(doc, filters) {
			continue
		}
		score := 0.0
		if ql == "" {
			score = 1
		} else if strings.EqualFold(doc.ResourceID, q) {
			score = 1
		} else if strings.Contains(strings.ToLower(doc.Text), ql) {
			score = 0.5
		} else {
			continue
		}
		*hits = append(*hits, ports.SearchHit{ResourceID: doc.ResourceID, RevisionID: doc.RevisionID, Score: score})
		if len(*hits) >= limit {
			break
		}
	}
	return rows.Err()
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

func attr(m map[string]string, k string) string {
	if m == nil {
		return ""
	}
	return m[k]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
