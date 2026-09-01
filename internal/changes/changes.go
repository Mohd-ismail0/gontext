package changes

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/observability"
	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const (
	SchemaVersion  = "context-fabric.change/v1"
	ProfileVersion = "context-fabric.profile/v1"

	StatusPending           = "pending"
	StatusDelivered         = "delivered"
	StatusDuplicateIgnored  = "duplicate_ignored"
	StatusDead              = "dead"
)

// FeedStore persists change events and webhook registrations.
type FeedStore interface {
	ListChanges(ctx context.Context, orgID, cursor string, limit int) ([]ports.ChangeEvent, string, error)
	AppendChange(ctx context.Context, ev ports.ChangeEvent) error
}

// Durability optionally persists webhook subscriptions and deliveries.
type Durability interface {
	SaveSubscription(ctx context.Context, sub Subscription) error
	ListSubscriptions(ctx context.Context, orgID string) ([]Subscription, error)
	GetSubscription(ctx context.Context, orgID, subscriptionID string) (Subscription, error)
	SaveDelivery(ctx context.Context, d DeliveryAttempt) error
	ListDeliveries(ctx context.Context, orgID, subscriptionID string, limit int) ([]DeliveryAttempt, error)
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
	DeliveryID     string     `json:"delivery_id"`
	SubscriptionID string     `json:"subscription_id"`
	OrgID          string     `json:"organization_id"`
	EventID        string     `json:"event_id"`
	Attempt        int        `json:"attempt"`
	StatusCode     int        `json:"status_code,omitempty"`
	Success        bool       `json:"success"`
	Status         string     `json:"status,omitempty"` // pending|delivered|duplicate_ignored|dead
	Error          string     `json:"error,omitempty"`
	PayloadSHA     string     `json:"payload_sha256"`
	NextAttempt    *time.Time `json:"next_attempt,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
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
	Feed         FeedStore
	Durable      Durability // optional Postgres-backed webhook state
	mu           sync.RWMutex
	subs         map[string]map[string]Subscription // org -> id -> sub
	deliv        map[string][]DeliveryAttempt       // org -> attempts
	secret       []byte
	RequireSecret bool // when true, empty constructor secret is rejected at runtime
	MaxAttempts  int // dead-letter after this many failed pushes (default 8)
	Now          func() time.Time
}

// New creates a change-feed service. webhookSecret signs delivery bodies.
// Pass nil/empty only for demo; production must set WEBHOOK_SIGNING_SECRET.
func New(feed FeedStore, webhookSecret []byte) *Service {
	return &Service{
		Feed:        feed,
		subs:        make(map[string]map[string]Subscription),
		deliv:       make(map[string][]DeliveryAttempt),
		secret:      append([]byte(nil), webhookSecret...),
		MaxAttempts: 8,
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
	if err := ValidateWebhookTarget(targetURL); err != nil {
		return Subscription{}, platform.ErrValidation(err.Error())
	}
	if secret == "" {
		var err error
		secret, err = generateSubscriptionSecret()
		if err != nil {
			return Subscription{}, platform.ErrUnavailable("webhook secret generation failed")
		}
	}
	id := platform.NewEventID()
	sub := Subscription{
		ID: id, OrgID: orgID, TargetURL: targetURL, Events: events,
		Secret: secret, Enabled: true, CreatedAt: s.now(),
	}
	if s.Durable != nil {
		if err := s.Durable.SaveSubscription(context.Background(), sub); err != nil {
			return Subscription{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.subs[orgID]
	if m == nil {
		m = make(map[string]Subscription)
		s.subs[orgID] = m
	}
	m[id] = sub
	pub := sub
	pub.Secret = ""
	return pub, nil
}

func generateSubscriptionSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ListWebhooks returns public subscriptions for an org.
func (s *Service) ListWebhooks(orgID string) []Subscription {
	if s.Durable != nil {
		if items, err := s.Durable.ListSubscriptions(context.Background(), orgID); err == nil && len(items) > 0 {
			out := make([]Subscription, 0, len(items))
			for _, sub := range items {
				pub := sub
				pub.Secret = ""
				out = append(out, pub)
			}
			return out
		}
	}
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
	if s.Durable != nil {
		if sub, err := s.Durable.GetSubscription(context.Background(), orgID, subscriptionID); err == nil {
			sub.Secret = ""
			return sub, nil
		}
	}
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
	observability.Inc(observability.WebhookDeliveries)
	sum := sha256.Sum256(payload)
	status := StatusPending
	if success {
		status = StatusDelivered
	}
	d := DeliveryAttempt{
		DeliveryID:     platform.NewEventID(),
		SubscriptionID: subscriptionID,
		OrgID:          orgID,
		EventID:        eventID,
		Attempt:        attempt,
		StatusCode:     statusCode,
		Success:        success,
		Status:         status,
		Error:          errMsg,
		PayloadSHA:     "sha256:" + hex.EncodeToString(sum[:]),
		CreatedAt:      s.now(),
	}
	if !success {
		max := s.MaxAttempts
		if max <= 0 {
			max = 8
		}
		if attempt >= max {
			d.Status = StatusDead
		} else {
			backoff := time.Duration(1<<minInt(attempt, 6)) * time.Second
			next := s.now().Add(backoff)
			d.NextAttempt = &next
		}
	}
	if s.Durable != nil {
		_ = s.Durable.SaveDelivery(context.Background(), d)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliv[orgID] = append(s.deliv[orgID], d)
	return d
}

// RecordDuplicate records an idempotent duplicate delivery (already delivered).
func (s *Service) RecordDuplicate(orgID, subscriptionID, eventID string, payload []byte) DeliveryAttempt {
	sum := sha256.Sum256(payload)
	d := DeliveryAttempt{
		DeliveryID:     platform.NewEventID(),
		SubscriptionID: subscriptionID,
		OrgID:          orgID,
		EventID:        eventID,
		Attempt:        0,
		Success:        true,
		Status:         StatusDuplicateIgnored,
		PayloadSHA:     "sha256:" + hex.EncodeToString(sum[:]),
		CreatedAt:      s.now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliv[orgID] = append(s.deliv[orgID], d)
	return d
}

// ListDLQ returns dead-letter delivery attempts for an org (inspectable).
func (s *Service) ListDLQ(orgID string, limit int) []DeliveryAttempt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]DeliveryAttempt, 0, limit)
	all := s.deliv[orgID]
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		if all[i].Status == StatusDead {
			out = append(out, all[i])
		}
	}
	return out
}

// EnqueueAndAttempt records a push attempt for an event (used by conformance / ops).
func (s *Service) EnqueueAndAttempt(ctx context.Context, subscriptionID string, ev ports.ChangeEvent) error {
	sub, err := s.lookupSubscription(ctx, ev.OrgID, subscriptionID)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	return s.pushOne(ctx, client, 2*time.Second, sub, ev)
}

func (s *Service) lookupSubscription(ctx context.Context, orgID, subscriptionID string) (Subscription, error) {
	if s.Durable != nil {
		if sub, err := s.Durable.GetSubscription(ctx, orgID, subscriptionID); err == nil {
			return sub, nil
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[orgID][subscriptionID]
	if !ok {
		return Subscription{}, platform.ErrNotFound("webhook subscription not found")
	}
	return sub, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ListDeliveries returns delivery attempts for a subscription.
func (s *Service) ListDeliveries(orgID, subscriptionID string, limit int) []DeliveryAttempt {
	if s.Durable != nil {
		if items, err := s.Durable.ListDeliveries(context.Background(), orgID, subscriptionID, limit); err == nil && len(items) > 0 {
			return items
		}
	}
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
	sub, err := s.lookupSubscription(ctx, orgID, subscriptionID)
	if err != nil {
		return nil, err
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
	sub, err := s.lookupSubscription(context.Background(), orgID, subscriptionID)
	if err != nil {
		return nil, "", DeliveryAttempt{}, err
	}
	pub := ToPublic(ev)
	body, err = json.Marshal(pub)
	if err != nil {
		return nil, "", DeliveryAttempt{}, err
	}
	signature = s.SignHMAC(sub.Secret, body)
	if s.hasSuccessfulDelivery(orgID, subscriptionID, ev.EventID) {
		delivery = s.RecordDuplicate(orgID, subscriptionID, ev.EventID, body)
		return body, signature, delivery, nil
	}
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
		subs := s.enabledSubs(ctx, orgID)
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
	sub, err := s.lookupSubscription(ctx, orgID, subscriptionID)
	if err != nil {
		return DeliveryAttempt{}, err
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
	err = s.pushOne(ctx, client, timeout, sub, ev)
	attempts := s.ListDeliveries(orgID, subscriptionID, 1)
	if len(attempts) == 0 {
		return DeliveryAttempt{}, err
	}
	return attempts[0], err
}

func (s *Service) pushOne(ctx context.Context, client *http.Client, timeout time.Duration, sub Subscription, ev ports.ChangeEvent) error {
	if s.hasSuccessfulDelivery(sub.OrgID, sub.ID, ev.EventID) {
		pub := ToPublic(ev)
		body, _ := json.Marshal(pub)
		s.RecordDuplicate(sub.OrgID, sub.ID, ev.EventID, body)
		return nil
	}
	attempt := s.failedAttemptCount(sub.OrgID, sub.ID, ev.EventID) + 1
	if next, ok := s.nextAttemptAt(sub.OrgID, sub.ID, ev.EventID); ok && next.After(s.now()) {
		return fmt.Errorf("webhook backoff until %s", next.UTC().Format(time.RFC3339))
	}
	max := s.MaxAttempts
	if max <= 0 {
		max = 8
	}
	if attempt > max {
		return fmt.Errorf("webhook dead-lettered after %d attempts", max)
	}

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
		s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, attempt, 0, false, err.Error(), body)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Context-Fabric-Signature", sig)
	req.Header.Set("X-Context-Fabric-Event-Id", ev.EventID)
	resp, err := client.Do(req)
	if err != nil {
		s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, attempt, 0, false, err.Error(), body)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	errMsg := ""
	if !ok {
		errMsg = fmt.Sprintf("status %d", resp.StatusCode)
	}
	s.RecordDelivery(sub.OrgID, sub.ID, ev.EventID, attempt, resp.StatusCode, ok, errMsg, body)
	if !ok {
		return fmt.Errorf("webhook delivery failed: %s", errMsg)
	}
	return nil
}

func (s *Service) failedAttemptCount(orgID, subscriptionID, eventID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, d := range s.deliv[orgID] {
		if d.SubscriptionID == subscriptionID && d.EventID == eventID && !d.Success && d.Status != StatusDuplicateIgnored {
			if d.Attempt > n {
				n = d.Attempt
			}
		}
	}
	return n
}

func (s *Service) nextAttemptAt(orgID, subscriptionID, eventID string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *time.Time
	for i := range s.deliv[orgID] {
		d := &s.deliv[orgID][i]
		if d.SubscriptionID == subscriptionID && d.EventID == eventID && d.NextAttempt != nil {
			if latest == nil || d.NextAttempt.After(*latest) {
				latest = d.NextAttempt
			}
		}
	}
	if latest == nil {
		return time.Time{}, false
	}
	return *latest, true
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

func (s *Service) enabledSubs(ctx context.Context, orgID string) []Subscription {
	if s.Durable != nil {
		if items, err := s.Durable.ListSubscriptions(ctx, orgID); err == nil && len(items) > 0 {
			out := make([]Subscription, 0, len(items))
			for _, sub := range items {
				if sub.Enabled {
					out = append(out, sub)
				}
			}
			return out
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.subs[orgID]))
	for _, sub := range s.subs[orgID] {
		if sub.Enabled {
			out = append(out, sub)
		}
	}
	return out
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
