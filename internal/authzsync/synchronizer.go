// Package authzsync centralizes durable OpenFGA inheritance tuple work.
package authzsync

import (
	"context"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const (
	OperationWrite  = "write"
	OperationDelete = "delete"
)

// EnqueueForEdge records durable inheritance work in the caller's existing
// organization transaction. Knowledge edges that do not explicitly request
// parent inheritance never produce an authorization tuple.
func EnqueueForEdge(ctx context.Context, ledger ports.LedgerStore, tx ports.Tx, edge ports.GraphEdge, operation string, now time.Time) error {
	if !NeedsSynchronization(edge) {
		return nil
	}
	if operation != OperationDelete {
		operation = OperationWrite
	}
	return ledger.EnqueueAuthzTuple(ctx, tx, ports.AuthzTupleOp{
		ID:          platform.NewEventID(),
		OrgID:       edge.OrgID,
		Operation:   operation,
		Object:      ResourceObject(edge.FromID),
		Relation:    ports.EdgeParent,
		Subject:     ResourceObject(edge.ToID),
		EdgeID:      edge.EdgeID,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
		NextAttempt: now,
	})
}

// NeedsSynchronization is deliberately narrow: knowledge edges are facts;
// only an explicitly synchronized parent edge requests AuthZ inheritance.
func NeedsSynchronization(edge ports.GraphEdge) bool {
	return edge.SyncAuthz && edge.Predicate == ports.EdgeParent
}

// ResourceObject returns the OpenFGA object/subject representation for a record.
func ResourceObject(resourceID string) string {
	return "resource:" + resourceID
}
