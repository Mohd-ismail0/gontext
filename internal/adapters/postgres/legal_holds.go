package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/xsama/context-fabric/internal/deletion"
)

// LegalHoldStore implements deletion.LegalHoldChecker against legal_holds (RLS).
type LegalHoldStore struct {
	pool *Pool
}

// NewLegalHoldStore wraps a pool.
func NewLegalHoldStore(pool *Pool) *LegalHoldStore {
	return &LegalHoldStore{pool: pool}
}

var _ deletion.LegalHoldChecker = (*LegalHoldStore)(nil)

// HasLegalHold reports whether an active hold blocks evidence purge for resourceID.
func (h *LegalHoldStore) HasLegalHold(ctx context.Context, orgID, resourceID string) (bool, error) {
	if orgID == "" || resourceID == "" {
		return false, nil
	}
	var held bool
	err := h.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
SELECT held
FROM legal_holds
WHERE organization_id = $1
  AND resource_id = $2
  AND released_at IS NULL
  AND held = true`, orgID, resourceID).Scan(&held)
		if errors.Is(err, pgx.ErrNoRows) {
			held = false
			return nil
		}
		return err
	})
	return held, err
}
