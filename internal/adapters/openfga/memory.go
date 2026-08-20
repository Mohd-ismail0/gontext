package openfga

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

// Memory evaluates AuthZ against an in-process relationship map (tests/fixtures).
// Key shape: object -> relation -> subject -> true
// Example: relations["resource:res1"]["can_read"]["user:alice"] = true
type Memory struct {
	mu         sync.RWMutex
	Relations  map[string]map[string]map[string]bool
	ModelID    string
	OrgMembers map[string]map[string]bool // orgID -> subject -> member
}

// NewMemory creates an empty memory AuthZ provider.
func NewMemory() *Memory {
	return &Memory{
		Relations:  make(map[string]map[string]map[string]bool),
		ModelID:    "memory-model",
		OrgMembers: make(map[string]map[string]bool),
	}
}

var _ ports.AuthorizationProvider = (*Memory)(nil)

// Grant adds a relationship tuple.
func (m *Memory) Grant(object, relation, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Relations[object] == nil {
		m.Relations[object] = make(map[string]map[string]bool)
	}
	if m.Relations[object][relation] == nil {
		m.Relations[object][relation] = make(map[string]bool)
	}
	m.Relations[object][relation][subject] = true
}

// AddOrgMember marks a principal as an organization member.
func (m *Memory) AddOrgMember(orgID, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.OrgMembers[orgID] == nil {
		m.OrgMembers[orgID] = make(map[string]bool)
	}
	m.OrgMembers[orgID][subject] = true
}

func (m *Memory) Check(ctx context.Context, req ports.AuthzCheck) (ports.AuthzDecision, error) {
	outs, err := m.BatchCheck(ctx, []ports.AuthzCheck{req})
	if err != nil {
		return ports.AuthzDecision{}, err
	}
	return outs[0], nil
}

func (m *Memory) BatchCheck(_ context.Context, reqs []ports.AuthzCheck) ([]ports.AuthzDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ports.AuthzDecision, len(reqs))
	now := time.Now().UTC()
	for i, r := range reqs {
		user := formatUser(r.Principal)
		obj := formatObject(r.ResourceID)
		rel := mapRelation(r.Action)
		allowed := m.has(obj, rel, user)
		// Also honor reader/restricted_reader if can_read requested.
		if !allowed && rel == "can_read" {
			allowed = m.has(obj, "reader", user) || m.has(obj, "restricted_reader", user) ||
				m.has(obj, "owner", user) || m.has(obj, "assignee", user)
		}
		if !allowed && rel == "can_delete" {
			allowed = m.has(obj, "owner", user) || m.has(obj, "can_manage", user) || m.has(obj, "can_admin", user)
		}
		code := "AUTHZ_DENY"
		if allowed {
			code = "AUTHZ_ALLOW"
		}
		out[i] = ports.AuthzDecision{
			Allowed:       allowed,
			ReasonCode:    code,
			Consistency:   r.Consistency,
			ModelRevision: m.ModelID,
			CheckedAt:     now,
		}
	}
	return out, nil
}

func (m *Memory) ResolveCandidateScope(_ context.Context, req ports.ScopeResolve) (ports.CandidateScope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	org := req.Principal.OrgID
	subj := req.Principal.Subject
	if subj == "" {
		subj = req.Principal.ID
	}
	members := m.OrgMembers[org]
	if members != nil && (members[subj] || members[formatUser(req.Principal)]) {
		return ports.CandidateScope{OrgID: org, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
	}
	// Fall back: if any relation exists for this user in org resources, allow scope.
	user := formatUser(req.Principal)
	for obj, rels := range m.Relations {
		if !strings.HasPrefix(obj, "resource:") && !strings.HasPrefix(obj, "organization:") {
			continue
		}
		for _, subjects := range rels {
			if subjects[user] {
				return ports.CandidateScope{OrgID: org, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
			}
		}
	}
	return ports.CandidateScope{OrgID: org, ReasonCode: "AUTHZ_SCOPE_DENY"}, nil
}

func (m *Memory) has(object, relation, subject string) bool {
	rels := m.Relations[object]
	if rels == nil {
		return false
	}
	subs := rels[relation]
	if subs == nil {
		return false
	}
	return subs[subject]
}
