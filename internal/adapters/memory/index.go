package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/xsama/context-fabric/internal/ports"
)

// Index is an in-memory IndexProvider with org-keyed documents.
type Index struct {
	mu   sync.RWMutex
	docs map[string][]ports.IndexDocument // org -> docs
}

// NewIndex creates an empty in-memory index.
func NewIndex() *Index {
	return &Index{docs: make(map[string][]ports.IndexDocument)}
}

var _ ports.IndexProvider = (*Index)(nil)

func (idx *Index) Upsert(_ context.Context, docs []ports.IndexDocument) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, d := range docs {
		list := idx.docs[d.OrgID]
		replaced := false
		for i := range list {
			if list[i].ResourceID == d.ResourceID {
				list[i] = d
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, d)
		}
		idx.docs[d.OrgID] = list
	}
	return nil
}

func (idx *Index) Delete(_ context.Context, orgID string, resourceIDs []string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	set := map[string]struct{}{}
	for _, id := range resourceIDs {
		set[id] = struct{}{}
	}
	list := idx.docs[orgID]
	out := list[:0]
	for _, d := range list {
		if _, drop := set[d.ResourceID]; !drop {
			out = append(out, d)
		}
	}
	idx.docs[orgID] = out
	return nil
}

// SearchCandidates returns IDs only, constrained by org and mandatory filters.
// Never searches across organizations. Tags/filters only AND-narrow.
func (idx *Index) SearchCandidates(_ context.Context, orgID string, query string, limit int, filters map[string]string) ([]ports.SearchHit, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if limit <= 0 {
		limit = 12
	}
	q := strings.ToLower(strings.TrimSpace(query))
	hits := make([]ports.SearchHit, 0, limit)
	for _, d := range idx.docs[orgID] {
		if !matchFilters(d, filters) {
			continue
		}
		score := 0.0
		if q == "" {
			score = 1
		} else if strings.Contains(strings.ToLower(d.Text), q) {
			score = 1
		} else {
			continue
		}
		hits = append(hits, ports.SearchHit{
			ResourceID: d.ResourceID,
			RevisionID: d.RevisionID,
			Score:      score,
		})
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}

func matchFilters(d ports.IndexDocument, filters map[string]string) bool {
	if filters == nil {
		return true
	}
	// purpose: record must list purpose (stored in attributes or labels as purpose:<p>)
	if purpose := filters["purpose"]; purpose != "" {
		ok := false
		for _, l := range d.Labels {
			if l == "purpose:"+purpose || strings.EqualFold(l, purpose) {
				ok = true
				break
			}
		}
		if !ok {
			if d.Attributes != nil {
				if allow := d.Attributes["purpose_allowlist"]; allow != "" {
					for _, p := range strings.Split(allow, ",") {
						if strings.TrimSpace(p) == purpose {
							ok = true
							break
						}
					}
				}
			}
		}
		if !ok {
			return false
		}
	}
	// classification ceiling: document classification rank must be <= filter ceiling
	if ceiling := filters["classification_ceiling"]; ceiling != "" {
		docClass := d.Attributes["classification"]
		if docClass == "" {
			docClass = "internal"
		}
		if classRank(docClass) > classRank(ceiling) {
			return false
		}
	}
	// include_tags: AND-narrow only — every requested tag must be present
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

// GetDocument returns indexed text for hydration snippets (org-scoped).
func (idx *Index) GetDocument(orgID, resourceID string) (ports.IndexDocument, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, d := range idx.docs[orgID] {
		if d.ResourceID == resourceID {
			return d, true
		}
	}
	return ports.IndexDocument{}, false
}
