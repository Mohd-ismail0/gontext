// Package natsbus provides a JetStream EventBus and a NoopBus for tests.
package natsbus

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/xsama/context-fabric/internal/config"
	"github.com/xsama/context-fabric/internal/ports"
)

const defaultStream = "CONTEXT"

// Bus is a NATS JetStream EventBus.
type Bus struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	stream string
	mu     sync.Mutex
	subs   []*nats.Subscription
}

// Connect opens a JetStream connection. Returns NoopBus-compatible error path via ConnectOrNoop.
func Connect(url string, opts ...nats.Option) (*Bus, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("nats url required")
	}
	nc, err := nats.Connect(url, append(defaultConnectOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return newBus(nc)
}

// ConnectConfig connects using runtime NATS configuration (URL + optional credentials file).
func ConnectConfig(cfg config.NATSConfig) (*Bus, error) {
	url := strings.TrimSpace(cfg.URL)
	if url == "" {
		return nil, fmt.Errorf("nats url required")
	}
	opts := defaultConnectOptions()
	if path := credentialsFilePath(cfg.Credentials); path != "" {
		opts = append(opts, nats.UserCredentials(path))
	}
	return Connect(url, opts...)
}

func credentialsFilePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if st, err := os.Stat(raw); err == nil && !st.IsDir() {
		return raw
	}
	return ""
}

func defaultConnectOptions() []nats.Option {
	return []nats.Option{
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectBufSize(4 << 20),
		nats.Timeout(10 * time.Second),
	}
}

func newBus(nc *nats.Conn) (*Bus, error) {
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	b := &Bus{nc: nc, js: js, stream: defaultStream}
	if err := b.ensureStream(); err != nil {
		nc.Close()
		return nil, err
	}
	return b, nil
}

// ConnectOrNoop returns a live bus when reachable, otherwise NoopBus.
// Prefer Connect in non-demo deployments — silent noop drops events.
func ConnectOrNoop(url string) ports.EventBus {
	if strings.TrimSpace(url) == "" {
		return NewNoop()
	}
	b, err := Connect(url)
	if err != nil {
		return NewNoop()
	}
	return b
}

func (b *Bus) ensureStream() error {
	_, err := b.js.StreamInfo(b.stream)
	if err == nil {
		return nil
	}
	_, err = b.js.AddStream(&nats.StreamConfig{
		Name:      b.stream,
		Subjects:  []string{"context.>"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
	})
	return err
}

// Publish implements ports.EventBus with Nats-Msg-Id = event_id when present.
func (b *Bus) Publish(ctx context.Context, subject string, data []byte, headers map[string]string) error {
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	eventID := ""
	for k, v := range headers {
		msg.Header.Set(k, v)
		if strings.EqualFold(k, "Nats-Msg-Id") || strings.EqualFold(k, "event_id") {
			eventID = v
		}
	}
	if eventID != "" {
		msg.Header.Set("Nats-Msg-Id", eventID)
	}
	opts := []nats.PubOpt{}
	if eventID != "" {
		opts = append(opts, nats.MsgId(eventID))
	}
	_, err := b.js.PublishMsg(msg, opts...)
	return err
}

// Subscribe starts an async push consumer (best-effort). Prefer PullSubscribe for workers.
func (b *Bus) Subscribe(ctx context.Context, subject string, handler func(msg ports.EventMessage) error) (ports.Subscription, error) {
	sub, err := b.js.Subscribe(subject, func(m *nats.Msg) {
		em := ports.EventMessage{
			Subject:   m.Subject,
			Data:      m.Data,
			Headers:   headerMap(m.Header),
			MsgID:     m.Header.Get("Nats-Msg-Id"),
			Timestamp: time.Now().UTC(),
		}
		if err := handler(em); err != nil {
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	}, nats.Durable("context-push"), nats.ManualAck())
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	return &jsSub{sub: sub}, nil
}

// PullConsumer is a durable JetStream pull subscription helper.
type PullConsumer struct {
	sub *nats.Subscription
}

// PullSubscribe creates or binds a durable pull consumer.
func (b *Bus) PullSubscribe(consumer, subject string) (*PullConsumer, error) {
	sub, err := b.js.PullSubscribe(subject, consumer, nats.BindStream(b.stream))
	if err != nil {
		// create durable via SubscribeSync options
		sub, err = b.js.PullSubscribe(subject, consumer)
		if err != nil {
			return nil, err
		}
	}
	return &PullConsumer{sub: sub}, nil
}

// Fetch pulls up to max messages waiting up to wait.
func (c *PullConsumer) Fetch(ctx context.Context, max int, wait time.Duration) ([]ports.EventMessage, error) {
	if max <= 0 {
		max = 10
	}
	if wait <= 0 {
		wait = 2 * time.Second
	}
	msgs, err := c.sub.Fetch(max, nats.MaxWait(wait), nats.Context(ctx))
	if err != nil && err != nats.ErrTimeout {
		return nil, err
	}
	out := make([]ports.EventMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, ports.EventMessage{
			Subject:   m.Subject,
			Data:      append([]byte(nil), m.Data...),
			Headers:   headerMap(m.Header),
			MsgID:     m.Header.Get("Nats-Msg-Id"),
			Timestamp: time.Now().UTC(),
		})
		_ = m.Ack()
	}
	return out, nil
}

// FetchIndex pulls messages for the index projector consumer.
func (b *Bus) FetchIndex(ctx context.Context, max int, wait time.Duration) ([]ports.EventMessage, error) {
	pc, err := b.PullSubscribe(indexConsumerName, "context.>")
	if err != nil {
		return nil, err
	}
	return pc.Fetch(ctx, max, wait)
}

const indexConsumerName = "context-index-projector"

// Close unsubscribes consumers, drains, and closes the connection.
func (b *Bus) Close() {
	if b == nil || b.nc == nil {
		return
	}
	b.mu.Lock()
	subs := append([]*nats.Subscription(nil), b.subs...)
	b.subs = nil
	b.mu.Unlock()
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	_ = b.nc.Drain()
	b.nc.Close()
}

type jsSub struct{ sub *nats.Subscription }

func (s *jsSub) Unsubscribe() error { return s.sub.Unsubscribe() }

func headerMap(h nats.Header) map[string]string {
	out := map[string]string{}
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// NoopBus is an in-process EventBus for tests when NATS is unavailable.
type NoopBus struct {
	mu   sync.Mutex
	msgs []ports.EventMessage
}

// NewNoop returns a NoopBus.
func NewNoop() *NoopBus { return &NoopBus{} }

var _ ports.EventBus = (*NoopBus)(nil)
var _ ports.EventBus = (*Bus)(nil)

func (n *NoopBus) Publish(_ context.Context, subject string, data []byte, headers map[string]string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := ""
	if headers != nil {
		id = headers["Nats-Msg-Id"]
		if id == "" {
			id = headers["event_id"]
		}
	}
	n.msgs = append(n.msgs, ports.EventMessage{
		Subject: subject, Data: append([]byte(nil), data...), Headers: headers,
		MsgID: id, Timestamp: time.Now().UTC(),
	})
	return nil
}

func (n *NoopBus) Subscribe(_ context.Context, _ string, _ func(msg ports.EventMessage) error) (ports.Subscription, error) {
	return noopSub{}, nil
}

// Published returns a copy of published messages (tests).
func (n *NoopBus) Published() []ports.EventMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]ports.EventMessage, len(n.msgs))
	copy(out, n.msgs)
	return out
}

type noopSub struct{}

func (noopSub) Unsubscribe() error { return nil }
