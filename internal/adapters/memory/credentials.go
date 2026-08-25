package memory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// CredentialStore is an in-memory ports.CredentialProvider.
type CredentialStore struct {
	mu      sync.RWMutex
	byKey   map[string]agentCred // sha256(secret) -> cred
	byID    map[string]agentCred // credentialID -> cred
	byAgent map[string][]string  // org|agent -> credentialIDs
}

type agentCred struct {
	meta   ports.AgentCredential
	hash   string
	orgID  string
	owner  string
	revoked bool
	agent  ports.AgentPrincipal
}

// NewCredentialStore creates an empty credential store.
func NewCredentialStore() *CredentialStore {
	return &CredentialStore{
		byKey:   make(map[string]agentCred),
		byID:    make(map[string]agentCred),
		byAgent: make(map[string][]string),
	}
}

var _ ports.CredentialProvider = (*CredentialStore)(nil)

func (c *CredentialStore) ResolveAPIKey(_ context.Context, key string) (ports.AgentPrincipal, error) {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	c.mu.RLock()
	defer c.mu.RUnlock()
	cred, ok := c.byKey[h]
	if !ok || cred.revoked {
		return ports.AgentPrincipal{}, platform.ErrUnauthorized("invalid api key")
	}
	if cred.meta.ExpiresAt != nil && time.Now().After(*cred.meta.ExpiresAt) {
		return ports.AgentPrincipal{}, platform.ErrUnauthorized("api key expired")
	}
	return cred.agent, nil
}

func (c *CredentialStore) CreateAgentCredential(_ context.Context, req ports.CreateAgentCredentialRequest) (ports.AgentCredential, error) {
	if req.OrgID == "" || req.AgentID == "" {
		return ports.AgentCredential{}, platform.ErrValidation("org and agent required")
	}
	secret, err := randomSecret()
	if err != nil {
		return ports.AgentCredential{}, err
	}
	id := platform.NewEventID()
	sum := sha256.Sum256([]byte(secret))
	h := hex.EncodeToString(sum[:])
	now := time.Now().UTC()
	meta := ports.AgentCredential{
		CredentialID: id,
		AgentID:      req.AgentID,
		OrgID:        req.OrgID,
		Secret:       secret,
		ExpiresAt:    req.ExpiresAt,
		CreatedAt:    now,
	}
	agent := ports.AgentPrincipal{
		Principal: ports.Principal{
			ID: "agent|" + req.AgentID, Kind: ports.PrincipalKindAgent,
			OrgID: req.OrgID, Subject: req.AgentID, Issuer: "context-fabric/apikey",
			Scopes: []string{"context:search", "context:read"},
		},
		AgentID: req.AgentID, CredentialID: id, OwnerID: req.OwnerID,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rec := agentCred{meta: meta, hash: h, orgID: req.OrgID, owner: req.OwnerID, agent: agent}
	rec.meta.Secret = "" // store without secret
	c.byKey[h] = rec
	c.byID[id] = rec
	key := req.OrgID + "|" + req.AgentID
	c.byAgent[key] = append(c.byAgent[key], id)
	meta.Secret = secret // return once
	return meta, nil
}

func (c *CredentialStore) Revoke(_ context.Context, orgID, credentialID string) error {
	if orgID == "" || credentialID == "" {
		return platform.ErrValidation("org and credential required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cred, ok := c.byID[credentialID]
	if !ok || cred.orgID != orgID {
		return platform.ErrNotFound("credential not found")
	}
	cred.revoked = true
	c.byID[credentialID] = cred
	c.byKey[cred.hash] = cred
	return nil
}

// RotateAgentCredential revokes current keys for an agent and issues a new one.
func (c *CredentialStore) RotateAgentCredential(ctx context.Context, req ports.CreateAgentCredentialRequest) (ports.AgentCredential, error) {
	c.mu.Lock()
	ids := append([]string{}, c.byAgent[req.OrgID+"|"+req.AgentID]...)
	c.mu.Unlock()
	for _, id := range ids {
		_ = c.Revoke(ctx, req.OrgID, id)
	}
	return c.CreateAgentCredential(ctx, req)
}

func randomSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "cfak_" + hex.EncodeToString(b), nil
}

// Legal holds on the memory ledger.

// SetLegalHold marks a resource as under legal hold.
func (s *Store) SetLegalHold(_ context.Context, orgID, resourceID string, held bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.holds == nil {
		s.holds = make(map[string]map[string]bool)
	}
	m := s.holds[orgID]
	if m == nil {
		m = make(map[string]bool)
		s.holds[orgID] = m
	}
	m[resourceID] = held
	return nil
}

// HasLegalHold implements deletion.LegalHoldChecker.
func (s *Store) HasLegalHold(_ context.Context, orgID, resourceID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.holds == nil {
		return false, nil
	}
	return s.holds[orgID][resourceID], nil
}

// PutExportJob stores an export job.
func (s *Store) PutExportJob(_ context.Context, orgID, jobID, status string, manifest any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.exports[orgID]
	if m == nil {
		m = make(map[string]ExportJob)
		s.exports[orgID] = m
	}
	m[jobID] = ExportJob{JobID: jobID, OrgID: orgID, Status: status, CreatedAt: time.Now().UTC(), Manifest: manifest}
	return nil
}

// GetExportJob returns an export job for ApplicationService.ExportJobStore.
func (s *Store) GetExportJob(_ context.Context, orgID, jobID string) (string, string, any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.exports[orgID]
	if m == nil {
		return "", "", nil, platform.ErrNotFound("export not found")
	}
	j, ok := m[jobID]
	if !ok {
		return "", "", nil, platform.ErrNotFound("export not found")
	}
	return j.JobID, j.Status, j.Manifest, nil
}

// LookupExportJob returns the full ExportJob struct.
func (s *Store) LookupExportJob(_ context.Context, orgID, jobID string) (ExportJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.exports[orgID]
	if m == nil {
		return ExportJob{}, platform.ErrNotFound("export not found")
	}
	j, ok := m[jobID]
	if !ok {
		return ExportJob{}, platform.ErrNotFound("export not found")
	}
	return j, nil
}
