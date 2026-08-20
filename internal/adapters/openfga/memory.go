package openfga

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

// Memory evaluates AuthZ against an in-process relationship map aligned with model.fga.
// Writable relations only: reader, restricted_reader, owner, assignee, parent, member, …
// can_read / can_manage are computed (never granted directly).
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

var (
	_ ports.AuthorizationProvider = (*Memory)(nil)
	_ ports.RelationshipWriter    = (*Memory)(nil)
)

// Grant adds a relationship tuple. Prefer model-writable relations (reader, owner, parent, member).
func (m *Memory) Grant(object, relation, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grantLocked(object, relation, subject)
}

func (m *Memory) grantLocked(object, relation, subject string) {
	// Map legacy direct can_read grants to reader for model parity.
	if relation == "can_read" {
		relation = "reader"
	}
	if relation == "can_delete" || relation == "can_admin" {
		relation = "owner"
	}
	if m.Relations[object] == nil {
		m.Relations[object] = make(map[string]map[string]bool)
	}
	if m.Relations[object][relation] == nil {
		m.Relations[object][relation] = make(map[string]bool)
	}
	m.Relations[object][relation][subject] = true
	if strings.HasPrefix(object, "organization:") && relation == "member" {
		orgID := strings.TrimPrefix(object, "organization:")
		if m.OrgMembers[orgID] == nil {
			m.OrgMembers[orgID] = make(map[string]bool)
		}
		m.OrgMembers[orgID][subject] = true
	}
}

// Revoke removes a relationship tuple.
func (m *Memory) Revoke(object, relation, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if relation == "can_read" {
		relation = "reader"
	}
	if m.Relations[object] != nil && m.Relations[object][relation] != nil {
		delete(m.Relations[object][relation], subject)
	}
}

// WriteTuples implements ports.RelationshipWriter.
func (m *Memory) WriteTuples(_ context.Context, tuples []ports.RelationshipTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range tuples {
		if t.Object == "" || t.Relation == "" || t.Subject == "" {
			continue
		}
		m.grantLocked(t.Object, t.Relation, t.Subject)
	}
	return nil
}

// DeleteTuples implements ports.RelationshipWriter.
func (m *Memory) DeleteTuples(_ context.Context, tuples []ports.RelationshipTuple) error {
	for _, t := range tuples {
		if t.Object == "" || t.Relation == "" || t.Subject == "" {
			continue
		}
		m.Revoke(t.Object, t.Relation, t.Subject)
	}
	return nil
}

// AddOrgMember marks a principal as an organization member.
func (m *Memory) AddOrgMember(orgID, subject string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.OrgMembers[orgID] == nil {
		m.OrgMembers[orgID] = make(map[string]bool)
	}
	m.OrgMembers[orgID][subject] = true
	user := subject
	if !strings.Contains(subject, ":") {
		user = "user:" + subject
	}
	m.grantLocked("organization:"+orgID, "member", user)
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
		allowed := m.can(obj, rel, user, map[string]bool{})
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
	user := formatUser(req.Principal)
	if m.has("organization:"+org, "member", user) {
		return ports.CandidateScope{OrgID: org, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
	}
	subj := req.Principal.Subject
	if subj == "" {
		subj = req.Principal.ID
	}
	members := m.OrgMembers[org]
	if members != nil && (members[subj] || members[user] || members["user:"+subj]) {
		return ports.CandidateScope{OrgID: org, ReasonCode: "AUTHZ_SCOPE_OK"}, nil
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

// can evaluates model.fga can_read / can_manage including parent inheritance.
func (m *Memory) can(object, relation, subject string, visiting map[string]bool) bool {
	key := object + "|" + relation
	if visiting[key] {
		return false
	}
	visiting[key] = true

	switch relation {
	case "can_read":
		if m.has(object, "reader", subject) ||
			m.has(object, "restricted_reader", subject) ||
			m.has(object, "owner", subject) ||
			m.has(object, "assignee", subject) {
			return true
		}
		if m.can(object, "can_manage", subject, visiting) {
			return true
		}
		for parent := range m.Relations[object]["parent"] {
			if m.can(parent, "can_read", subject, visiting) {
				return true
			}
		}
		return false
	case "can_manage":
		if m.has(object, "owner", subject) {
			return true
		}
		// manager / knowledge_admin from organization
		for org := range m.Relations[object]["organization"] {
			if m.has(org, "manager", subject) || m.has(org, "knowledge_admin", subject) {
				return true
			}
		}
		for parent := range m.Relations[object]["parent"] {
			if m.can(parent, "can_manage", subject, visiting) {
				return true
			}
		}
		return false
	case "member":
		return m.has(object, "member", subject)
	default:
		return m.has(object, relation, subject)
	}
}
