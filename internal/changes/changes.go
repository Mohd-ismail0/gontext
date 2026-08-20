package changes

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const (
	SchemaVersion  = "context-fabric.change/v1"
	ProfileVersion = "context-fabric.profile/v1"
)

// FeedStore persists change events and webhook registrations.
type FeedStore interface {
	ListChanges(ctx context.Context, orgID, cursor string, limit int) ([]ports.ChangeEvent, string, error)
	AppendChange(ctx context.Context, ev ports.ChangeEvent) error
}

// Subscription is a webhook registration.
type Subscription struct {
	ID        string    `json:"subscription_id"`
	OrgID     string    `json:"organization_id"`
	TargetURL string    `json:"target_url"`
	Events    []string  `json:"events,omitempty"`
	Secret    string    `json:"-"` // never returned on public surfaces
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// DeliveryAttempt records one webhook push try.
type DeliveryAttempt struct {
	DeliveryID     string    `json:"delivery_id"`
	SubscriptionID string    `json:"subscription_id"`
	OrgID          string    `json:"organization_id"`
	EventID        string    `json:"event_id"`
	Attempt        int       `json:"attempt"`
	StatusCode     int       `json:"status_code,omitempty"`
	Success        bool      `json:"success"`
	Error          string    `json:"error,omitempty"`
	PayloadSHA     string    `json:"payload_sha256"`
	CreatedAt      time.Time `json:"created_at"`
}

// PublicEvent is the metadata-only webhook/cursor payload (ADR 0010).
type PublicEvent struct {
	EventID        string    `json:"event_id"`
	OrganizationID string    `json:"organization_id"`
	Cursor         string    `json:"cursor"`
	OccurredAt     time.Time `json:"occurred_at"`
	ChangeType     string    `json:"change_type"`
	ResourceID     string    `json:"resource_id"`
	RevisionID     string    `json:"revision_id"`
	LifecycleState string    `json:"lifecycle_state"`
	SchemaVersion  string    `json:"schema_version"`
	ProfileVersion string    `json:"profile_version"`
}

// Service owns cursor listing, signing, delivery attempts, and replay.
type Service struct {
	Feed   FeedStore
	mu     sync.RWMutex
	subs   map[string]map[string]Subscription // org -> id -> sub
	deliv  map[string][]DeliveryAttempt       // org -> attempts
	secret []byte
	Now    func() time.Time
}

// New creates a change-feed service. webhookSecret signs delivery bodies.
func New(feed FeedStore, webhookSecret []byte) *Service {
	if len(webhookSecret) == 0 {
		webhookSecret = []byte("context-fabric-webhook")
	}
	return &Service{
		Feed:   feed,
		subs:   make(map[string]map[string]Subscription),
		deliv:  make(map[string][]DeliveryAttempt),
		secret: webhookSecret,
	}
}

// List returns a cursor page of public events.
func (s *Service) List(ctx context.Context, orgID, cursor string, limit int) ([]PublicEvent, string, error) {
	if s.Feed == nil {
		return []PublicEvent{}, cursor, nil
	}
	items, next, err := s.Feed.ListChanges(ctx, orgID, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	out := make([]PublicEvent, 0, len(items))
	for _, ev := range items {
		out = append(out, ToPublic(ev))
	}
	return out, next, nil
}

// Append stores a change event via the feed store.
func (s *Service) Append(ctx context.Context, ev ports.ChangeEvent) error {
	if s.Feed == nil {
		return nil
	}
	if ev.EventID == "" {
		ev.EventID = platform.NewEventID()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = s.now()
	}
	if ev.Cursor == "" {
		ev.Cursor = ev.EventID
	}
	return s.Feed.AppendChange(ctx, ev)
}

// AppendChange implements deletion.ChangeAppender / app.ChangeLister shape.
func (s *Service) AppendChange(ctx context.Context, ev ports.ChangeEvent) error {
	return s.Append(ctx, ev)
}

// UpsertWebhook registers or updates a subscription. Returns public view (no secret).
func (s *Service) UpsertWebhook(orgID string, targetURL string, events []string, secret string) (Subscription, error) {
	if orgID == "" || targetURL == "" {
		return Subscription{}, platform.ErrValidation("org and target_url required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.subs[orgID]
	if m == nil {
		m = make(map[string]Subscription)
		s.subs[orgID] = m
	}
	id := platform.NewEventID()
	if secret == "" {
		secret = hex.EncodeToString(s.secret)
	}
	sub := Subscription{
		ID: id, OrgID: orgID, TargetURL: targetURL, Events: events,
		Secret: secret, Enabled: true, CreatedAt: s.now(),
	}
	m[id] = sub
	pub := sub
	pub.Secret = ""
	return pub, nil
}

// ListWebhooks returns public subscriptions for an org.
func (s *Service) ListWebhooks(orgID string) []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.subs[orgID]))
	for _, sub := range s.subs[orgID] {
		pub := sub
		pub.Secret = ""
		out = append(out, pub)
	}
	return out
}

// GetWebhook returns a public subscription view.
func (s *Service) GetWebhook(orgID, subscriptionID string) (Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[orgID][subscriptionID]
	if !ok {
		return Subscription{}, platform.ErrNotFound("webhook subscription not found")
	}
	sub.Secret = ""
	return sub, nil
}

// SignHMAC signs a payload with the org webhook secret (or default).
func (s *Service) SignHMAC(secret string, body []byte) string {
	key := []byte(secret)
	if len(key) == 0 {
		key = s.secret
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// RecordDelivery stores a delivery attempt.
func (s *Service) RecordDelivery(orgID, subscriptionID, eventID string, attempt, statusCode int, success bool, errMsg string, payload []byte) DeliveryAttempt {
	sum := sha256.Sum256(payload)
	d := DeliveryAttempt{
		DeliveryID:     platform.NewEventID(),
		SubscriptionID: subscriptionID,
		OrgID:          orgID,
		EventID:        eventID,
		Attempt:        attempt,
		StatusCode:     statusCode,
		Success:        success,
		Error:          errMsg,
		PayloadSHA:     "sha256:" + hex.EncodeToString(sum[:]),
		CreatedAt:      s.now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliv[orgID] = append(s.deliv[orgID], d)
	return d
}

// ListDeliveries returns delivery attempts for a subscription.
func (s *Service) ListDeliveries(orgID, subscriptionID string, limit int) []DeliveryAttempt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	all := s.deliv[orgID]
	out := make([]DeliveryAttempt, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		if subscriptionID == "" || all[i].SubscriptionID == subscriptionID {
			out = append(out, all[i])
		}
	}
	return out
}

// Replay rebuilds a signed public payload for an event and records a new attempt.
func (s *Service) Replay(ctx context.Context, orgID, subscriptionID, eventID string) (map[string]any, error) {
	s.mu.RLock()
	sub, ok := s.subs[orgID][subscriptionID]
	s.mu.RUnlock()
	if !ok {
		return nil, platform.ErrNotFound("webhook subscription not found")
	}
	items, _, err := s.Feed.ListChanges(ctx, orgID, "", 10_000)
	if err != nil {
		return nil, err
	}
	var found *ports.ChangeEvent
	for i := range items {
		if items[i].EventID == eventID {
			found = &items[i]
			break
		}
	}
	if found == nil {
		return nil, platform.ErrNotFound("change event not found")
	}
	pub := ToPublic(*found)
	body, err := json.Marshal(pub)
	if err != nil {
		return nil, err
	}
	sig := s.SignHMAC(sub.Secret, body)
	d := s.RecordDelivery(orgID, subscriptionID, eventID, 1, 0, true, "replayed", body)
	return map[string]any{
		"delivery":  d,
		"signature": sig,
		"event":     pub,
	}, nil
}

// DeliverSigned builds a signed metadata-only webhook body for an event (no HTTP call in unit tests).
func (s *Service) DeliverSigned(orgID, subscriptionID string, ev ports.ChangeEvent) (body []byte, signature string, delivery DeliveryAttempt, err error) {
	s.mu.RLock()
	sub, ok := s.subs[orgID][subscriptionID]
	s.mu.RUnlock()
	if !ok {
		return nil, "", DeliveryAttempt{}, platform.ErrNotFound("webhook subscription not found")
	}
	pub := ToPublic(ev)
	body, err = json.Marshal(pub)
	if err != nil {
		return nil, "", DeliveryAttempt{}, err
	}
	signature = s.SignHMAC(sub.Secret, body)
	delivery = s.RecordDelivery(orgID, subscriptionID, ev.EventID, 1, 200, true, "", body)
	return body, signature, delivery, nil
}

// DeliverPending HTTP POSTs signed bodies for undelivered change events to matching subscriptions.
func (s *Service) DeliverPending(ctx context.Context, client *http.Client, timeout time.Duration, limit int) int {
	if client == nil {
		client = &http.Client{}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if limit <= 0 {
		limit = 20
	}
	s.mu.RLock()
	orgIDs := make([]string, 0, len(s.subs))
	for org := range s.subs {
		orgIDs = append(orgIDs, org)
	}
	s.mu.RUnlock()

	delivered := 0
	for _, orgID := range orgIDs {
		if delivered >= limit {
			break
		}
		s.mu.RLock()
		subs := make([]Subscription, 0, len(s.subs[orgID]))
		for _, sub := range s.subs[orgID] {
			if sub.Enabled {
				subs = append(subs, sub)
			}
		}
		s.mu.RUnlock()
		if len(subs) == 0 || s.Feed == nil {
			continue
		}
		events, _, err := s.Feed.ListChanges(ctx, orgID, "", 100)
		if err != nil {
			continue
		}
		for _, ev := range events {
			if delivered >= limit {
				break
			}
			for _, sub := range subs {
				if delivered >= limit {
					break
				}
				if s.hasSuccessfulDelivery(orgID, sub.ID, ev.EventID) {
					continue
				}
				if !eventMatches(sub.Events, ev.Action) {
					continue
				}
				if err := s.pushOne(ctx, client, timeout, sub, ev); err == nil {
					delivered++
				} else {
					delivered++ // still count attempt
				}
			}
		}
	}
	return delivered
}

// DeliverTestPing sends a synthetic ping event to a subscription URL.
func (s *Service) DeliverTestPing(ctx context.Context, client *http.Client, timeout time.Duration, orgID, subscriptionID string) (DeliveryAttempt, error) {
	s.mu.RLock()
	sub, ok := s.subs[orgID][subscriptionID]
	s.mu.RUnlock()
	if !ok {
		return DeliveryAttempt{}, platform.ErrNotFound("webhook subscription not found")
	}
	ev := ports.ChangeEvent{
		EventID: platform.NewEventID(), OrgID: orgID, ResourceID: "ping",
		Action: "webhook.ping", Cursor: "ping", OccurredAt: s.now(),
	}
	if client == nil {
		client = &http.Client{}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	err := s.pushOne(ctx, client, timeout, sub, ev)
	attempts := s.ListDeliveries(orgID, subscriptionID, 1)
	if len(attempts) == 0 {
		return DeliveryAttempt{}, err
	}
	return attempts[0], err
}

func (s *Service) pushOne(ctx context.Context, client *http.Client, timeout time.Duration, sub Subscription, ev ports.ChangeEvent) error {
	pub := ToPublic(ev)
	body, err := json.Marshal(pub)
	if err != nil {
		return err
	}
	sig := s.SignHMAC(sub.Secret, body)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.TargetURL, bytes.NewReader(body))
	if err != nil {
		s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, 1, 0, false, err.Error(), body)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Context-Fabric-Signature", sig)
	req.Header.Set("X-Context-Fabric-Event-Id", ev.EventID)
	resp, err := client.Do(req)
	if err != nil {
		s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, 1, 0, false, err.Error(), body)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	errMsg := ""
	if !ok {
		errMsg = fmt.Sprintf("status %d", resp.StatusCode)
	}
	s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, 1, resp.StatusCode, ok, errMsg, body)
	if !ok {
		return fmt.Errorf("webhook delivery failed: %s", errMsg)
	}
	return nil
}

func (s *Service) hasSuccessfulDelivery(orgID, subscriptionID, eventID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.deliv[orgID] {
		if d.SubscriptionID == subscriptionID && d.EventID == eventID && d.Success {
			return true
		}
	}
	return false
}

func eventMatches(filter []string, action string) bool {
	if len(filter) == 0 {
		return true
	}
	changeType := action
	if changeType == "" || changeType == "upsert" {
		changeType = "resource.accepted"
	}
	for _, f := range filter {
		if f == changeType || f == action || f == "*" {
			return true
		}
	}
	return false
}

// ToPublic maps an internal change to the public metadata-only shape.
func ToPublic(ev ports.ChangeEvent) PublicEvent {
	changeType := ev.Action
	if changeType == "" || changeType == "upsert" {
		changeType = "resource.accepted"
	}
	state := lifecycleFor(changeType)
	return PublicEvent{
		EventID:        ev.EventID,
		OrganizationID: ev.OrgID,
		Cursor:         ev.Cursor,
		OccurredAt:     ev.OccurredAt,
		ChangeType:     changeType,
		ResourceID:     ev.ResourceID,
		RevisionID:     ev.RevisionID,
		LifecycleState: state,
		SchemaVersion:  SchemaVersion,
		ProfileVersion: ProfileVersion,
	}
}

func lifecycleFor(changeType string) string {
	switch changeType {
	case "resource.tombstoned":
		return "TOMBSTONED"
	case "resource.purged":
		return "PURGED"
	case "resource.indexed":
		return "INDEXED"
	case "resource.reclassified":
		return "RECLASSIFIED"
	case "resource.failed":
		return "FAILED"
	default:
		return "ACCEPTED"
	}
}

// HasContentFields reports whether a JSON object illegally includes content fields.
func HasContentFields(v map[string]any) bool {
	forbidden := []string{"content", "vector", "embedding", "prompt", "authorization_tuples", "snippet", "body", "text"}
	for _, k := range forbidden {
		if _, ok := v[k]; ok {
			return true
		}
	}
	return false
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
