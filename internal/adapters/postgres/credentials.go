package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// CredentialStore implements ports.CredentialProvider against agent_credentials.
type CredentialStore struct {
	pool *Pool
}

// NewCredentialStore wraps a pool.
func NewCredentialStore(pool *Pool) *CredentialStore {
	return &CredentialStore{pool: pool}
}

var _ ports.CredentialProvider = (*CredentialStore)(nil)

func (c *CredentialStore) ResolveAPIKey(ctx context.Context, key string) (ports.AgentPrincipal, error) {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	var orgID, agentID, credID, ownerID string
	var expires *time.Time
	var revoked bool
	err := c.pool.QueryRow(ctx, `
SELECT organization_id, id, agent_id, owner_id, expires_at, revoked
FROM resolve_agent_credential($1)`, h).
		Scan(&orgID, &credID, &agentID, &ownerID, &expires, &revoked)
	if err != nil {
		return ports.AgentPrincipal{}, platform.ErrUnauthorized("invalid api key")
	}
	if revoked {
		return ports.AgentPrincipal{}, platform.ErrUnauthorized("invalid api key")
	}
	if expires != nil && time.Now().After(*expires) {
		return ports.AgentPrincipal{}, platform.ErrUnauthorized("api key expired")
	}
	return ports.AgentPrincipal{
		Principal: ports.Principal{
			ID: "agent|" + agentID, Kind: ports.PrincipalKindAgent,
			OrgID: orgID, Subject: agentID, Issuer: "context-fabric/apikey",
			Scopes: []string{"context:search", "context:read"},
		},
		AgentID: agentID, CredentialID: credID, OwnerID: ownerID,
	}, nil
}

func (c *CredentialStore) CreateAgentCredential(ctx context.Context, req ports.CreateAgentCredentialRequest) (ports.AgentCredential, error) {
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
	err = c.pool.WithTenant(ctx, req.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO agent_credentials (id, organization_id, agent_id, owner_id, secret_hash, expires_at, revoked, created_at)
VALUES ($1,$2,$3,$4,$5,$6,false,$7)`,
			id, req.OrgID, req.AgentID, req.OwnerID, h, req.ExpiresAt, now)
		return err
	})
	if err != nil {
		return ports.AgentCredential{}, err
	}
	return ports.AgentCredential{
		CredentialID: id, AgentID: req.AgentID, OrgID: req.OrgID,
		Secret: secret, ExpiresAt: req.ExpiresAt, CreatedAt: now,
	}, nil
}

func (c *CredentialStore) Revoke(ctx context.Context, orgID, credentialID string) error {
	if orgID == "" || credentialID == "" {
		return platform.ErrValidation("org and credential required")
	}
	var ok bool
	err := c.pool.QueryRow(ctx, `SELECT revoke_agent_credential($1, $2)`, orgID, credentialID).Scan(&ok)
	if err != nil {
		return platform.ErrNotFound("credential not found")
	}
	if !ok {
		return platform.ErrNotFound("credential not found")
	}
	return nil
}

// RotateAgentCredential revokes existing credentials for the agent and issues a new one.
func (c *CredentialStore) RotateAgentCredential(ctx context.Context, req ports.CreateAgentCredentialRequest) (ports.AgentCredential, error) {
	_ = c.pool.WithTenant(ctx, req.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
UPDATE agent_credentials SET revoked=true
WHERE organization_id=$1 AND agent_id=$2 AND revoked=false`, req.OrgID, req.AgentID)
		return err
	})
	return c.CreateAgentCredential(ctx, req)
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
