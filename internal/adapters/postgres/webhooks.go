package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/xsama/context-fabric/internal/changes"
	"github.com/xsama/context-fabric/internal/platform"
)

// WebhookStore persists webhook subscriptions and deliveries.
type WebhookStore struct {
	pool *Pool
}

// NewWebhookStore wraps a pool.
func NewWebhookStore(pool *Pool) *WebhookStore {
	return &WebhookStore{pool: pool}
}

// SaveSubscription upserts a subscription including signing secret.
func (w *WebhookStore) SaveSubscription(ctx context.Context, sub changes.Subscription) error {
	return w.pool.WithTenant(ctx, sub.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		events := sub.Events
		if events == nil {
			events = []string{}
		}
		_, err := tx.Exec(ctx, `
INSERT INTO webhook_subscriptions (
  id, organization_id, target_url, secret_hash, signing_secret, events, enabled, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (organization_id, id) DO UPDATE SET
  target_url=EXCLUDED.target_url,
  signing_secret=EXCLUDED.signing_secret,
  events=EXCLUDED.events,
  enabled=EXCLUDED.enabled`,
			sub.ID, sub.OrgID, sub.TargetURL, "", sub.Secret, events, sub.Enabled, sub.CreatedAt)
		return err
	})
}

// ListSubscriptions returns subscriptions for an org (includes signing secrets for delivery).
func (w *WebhookStore) ListSubscriptions(ctx context.Context, orgID string) ([]changes.Subscription, error) {
	var out []changes.Subscription
	err := w.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, target_url, COALESCE(signing_secret,''), events, enabled, created_at
FROM webhook_subscriptions WHERE organization_id=$1`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s changes.Subscription
			if err := rows.Scan(&s.ID, &s.OrgID, &s.TargetURL, &s.Secret, &s.Events, &s.Enabled, &s.CreatedAt); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out, err
}

// GetSubscription loads one subscription including signing secret.
func (w *WebhookStore) GetSubscription(ctx context.Context, orgID, subscriptionID string) (changes.Subscription, error) {
	var s changes.Subscription
	err := w.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
SELECT id, organization_id, target_url, COALESCE(signing_secret,''), events, enabled, created_at
FROM webhook_subscriptions WHERE organization_id=$1 AND id=$2`, orgID, subscriptionID).
			Scan(&s.ID, &s.OrgID, &s.TargetURL, &s.Secret, &s.Events, &s.Enabled, &s.CreatedAt)
	})
	if err != nil {
		return changes.Subscription{}, platform.ErrNotFound("webhook subscription not found")
	}
	return s, nil
}

// SaveDelivery persists a delivery attempt.
func (w *WebhookStore) SaveDelivery(ctx context.Context, d changes.DeliveryAttempt) error {
	return w.pool.WithTenant(ctx, d.OrgID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
INSERT INTO webhook_deliveries (
  id, organization_id, subscription_id, event_id, status, attempts, last_error,
  created_at, delivered_at, next_attempt, payload_sha256, status_code)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (organization_id, id) DO UPDATE SET
  status=EXCLUDED.status,
  attempts=EXCLUDED.attempts,
  last_error=EXCLUDED.last_error,
  delivered_at=EXCLUDED.delivered_at,
  next_attempt=EXCLUDED.next_attempt,
  status_code=EXCLUDED.status_code`,
			d.DeliveryID, d.OrgID, d.SubscriptionID, d.EventID, d.Status, d.Attempt, d.Error,
			d.CreatedAt, deliveredAt(d), d.NextAttempt, d.PayloadSHA, d.StatusCode)
		return err
	})
}

// ListDeliveries returns delivery attempts for a subscription.
func (w *WebhookStore) ListDeliveries(ctx context.Context, orgID, subscriptionID string, limit int) ([]changes.DeliveryAttempt, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []changes.DeliveryAttempt
	err := w.pool.WithTenant(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
SELECT id, organization_id, subscription_id, event_id, status, attempts,
       COALESCE(last_error,''), created_at, next_attempt, COALESCE(payload_sha256,''), status_code
FROM webhook_deliveries
WHERE organization_id=$1 AND subscription_id=$2
ORDER BY created_at DESC LIMIT $3`, orgID, subscriptionID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d changes.DeliveryAttempt
			if err := rows.Scan(&d.DeliveryID, &d.OrgID, &d.SubscriptionID, &d.EventID, &d.Status, &d.Attempt,
				&d.Error, &d.CreatedAt, &d.NextAttempt, &d.PayloadSHA, &d.StatusCode); err != nil {
				return err
			}
			d.Success = d.Status == changes.StatusDelivered || d.Status == changes.StatusDuplicateIgnored
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

func deliveredAt(d changes.DeliveryAttempt) *time.Time {
	if d.Status == changes.StatusDelivered {
		t := d.CreatedAt
		return &t
	}
	return nil
}
