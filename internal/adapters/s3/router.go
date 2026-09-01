package s3store

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xsama/context-fabric/internal/ports"
)

// RouterConfig configures a multi-bucket evidence router (quarantine/raw/derived).
type RouterConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	PathStyle       bool
	Quarantine      string
	Raw             string
	Derived         string
	MaxBytes        int64
}

// EvidenceRouter routes evidence keys to quarantine, raw, or derived buckets by path segment.
// Keys use {org_id}/{tier}/{object...} where tier is quarantine|raw|derived.
type EvidenceRouter struct {
	quarantine *Store
	raw        *Store
	derived    *Store
}

// NewRouter creates an EvidenceRouter spanning quarantine/raw/derived buckets.
func NewRouter(cfg RouterConfig) (*EvidenceRouter, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("s3 endpoint required")
	}
	if cfg.Quarantine == "" {
		cfg.Quarantine = "context-quarantine"
	}
	if cfg.Raw == "" {
		cfg.Raw = "context-raw"
	}
	if cfg.Derived == "" {
		cfg.Derived = "context-derived"
	}
	base := Config{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		PathStyle:       cfg.PathStyle,
		MaxBytes:        cfg.MaxBytes,
	}
	q, err := New(withBucket(base, cfg.Quarantine))
	if err != nil {
		return nil, fmt.Errorf("quarantine bucket: %w", err)
	}
	raw, err := New(withBucket(base, cfg.Raw))
	if err != nil {
		return nil, fmt.Errorf("raw bucket: %w", err)
	}
	derived, err := New(withBucket(base, cfg.Derived))
	if err != nil {
		return nil, fmt.Errorf("derived bucket: %w", err)
	}
	return &EvidenceRouter{quarantine: q, raw: raw, derived: derived}, nil
}

func withBucket(base Config, bucket string) Config {
	base.Bucket = bucket
	return base
}

var _ ports.EvidenceStore = (*EvidenceRouter)(nil)

// Buckets returns configured bucket names for diagnostics.
func (r *EvidenceRouter) Buckets() (quarantine, raw, derived string) {
	return r.quarantine.bucket, r.raw.bucket, r.derived.bucket
}

func evidenceTier(key string) string {
	parts := strings.SplitN(strings.Trim(key, "/"), "/", 3)
	if len(parts) < 2 {
		return "quarantine"
	}
	switch parts[1] {
	case "raw", "derived", "quarantine":
		return parts[1]
	default:
		return "quarantine"
	}
}

func (r *EvidenceRouter) storeFor(key string) *Store {
	switch evidenceTier(key) {
	case "raw":
		return r.raw
	case "derived":
		return r.derived
	default:
		return r.quarantine
	}
}

func (r *EvidenceRouter) Put(ctx context.Context, key string, body io.Reader, contentType string, meta map[string]string) (ports.EvidenceObject, error) {
	return r.storeFor(key).Put(ctx, key, body, contentType, meta)
}

func (r *EvidenceRouter) Get(ctx context.Context, key, versionID string) (io.ReadCloser, ports.EvidenceObject, error) {
	return r.storeFor(key).Get(ctx, key, versionID)
}

func (r *EvidenceRouter) PresignPut(ctx context.Context, key string, opts ports.PresignOptions) (string, time.Time, error) {
	return r.storeFor(key).PresignPut(ctx, key, opts)
}

func (r *EvidenceRouter) DeleteVersion(ctx context.Context, key, versionID string) error {
	return r.storeFor(key).DeleteVersion(ctx, key, versionID)
}
