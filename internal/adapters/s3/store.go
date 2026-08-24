package s3store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

const defaultMaxBytes int64 = 64 << 20 // 64 MiB

// Config configures the S3 evidence adapter.
type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	PathStyle       bool
	Bucket          string
	// MaxBytes caps Put body size (default 64<<20 / EVIDENCE_MAX_BYTES).
	// Oversized uploads are rejected with a validation error before PutObject.
	MaxBytes int64
}

// Store is an S3-compatible EvidenceStore.
type Store struct {
	client  *s3.Client
	bucket  string
	presign *s3.PresignClient
	maxBytes int64
}

// New creates an EvidenceStore. Use path-style for MinIO.
func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("s3 endpoint required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Bucket == "" {
		cfg.Bucket = "context-quarantine"
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxBytesFromEnv()
	}
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...any) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               cfg.Endpoint,
			HostnameImmutable: true,
			SigningRegion:     cfg.Region,
		}, nil
	})
	awsCfg := aws.Config{
		Region:                      cfg.Region,
		Credentials:                 credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		EndpointResolverWithOptions: resolver,
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.PathStyle
	})
	return &Store{client: client, bucket: cfg.Bucket, presign: s3.NewPresignClient(client), maxBytes: maxBytes}, nil
}

// MaxBytesFromEnv reads EVIDENCE_MAX_BYTES (default 64 MiB).
func MaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("EVIDENCE_MAX_BYTES"))
	if raw == "" {
		return defaultMaxBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return defaultMaxBytes
	}
	return n
}

var _ ports.EvidenceStore = (*Store)(nil)

func (s *Store) Put(ctx context.Context, key string, body io.Reader, contentType string, meta map[string]string) (ports.EvidenceObject, error) {
	max := s.maxBytes
	if max <= 0 {
		max = defaultMaxBytes
	}
	// Read at most max+1 bytes so we can detect overflow without loading unbounded input.
	limited := io.LimitReader(body, max+1)
	hasher := sha256.New()
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.TeeReader(limited, hasher))
	if err != nil {
		return ports.EvidenceObject{}, err
	}
	if n > max {
		return ports.EvidenceObject{}, platform.ErrValidation(
			fmt.Sprintf("evidence object exceeds max size of %d bytes", max),
		)
	}
	hash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	md := map[string]string{}
	for k, v := range meta {
		md[k] = v
	}
	md["content-hash"] = hash
	out, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String(contentType),
		Metadata:    md,
	})
	if err != nil {
		return ports.EvidenceObject{}, err
	}
	ver := ""
	if out.VersionId != nil {
		ver = *out.VersionId
	}
	return ports.EvidenceObject{
		Key: key, VersionID: ver, ContentHash: hash,
		Size: n, ContentType: contentType, Metadata: md,
	}, nil
}

func (s *Store) Get(ctx context.Context, key, versionID string) (io.ReadCloser, ports.EvidenceObject, error) {
	in := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if versionID != "" {
		in.VersionId = aws.String(versionID)
	}
	out, err := s.client.GetObject(ctx, in)
	if err != nil {
		return nil, ports.EvidenceObject{}, platform.ErrNotFound("evidence not found")
	}
	meta := ports.EvidenceObject{
		Key: key, VersionID: versionID, ContentType: aws.ToString(out.ContentType),
		Size: aws.ToInt64(out.ContentLength), Metadata: out.Metadata,
	}
	if out.Metadata != nil {
		meta.ContentHash = out.Metadata["content-hash"]
	}
	return out.Body, meta, nil
}

func (s *Store) PresignPut(ctx context.Context, key string, opts ports.PresignOptions) (string, time.Time, error) {
	exp := opts.ExpiresIn
	if exp <= 0 {
		exp = 15 * time.Minute
	}
	in := &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if opts.ContentType != "" {
		in.ContentType = aws.String(opts.ContentType)
	}
	res, err := s.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(exp))
	if err != nil {
		return "", time.Time{}, err
	}
	return res.URL, time.Now().UTC().Add(exp), nil
}

func (s *Store) DeleteVersion(ctx context.Context, key, versionID string) error {
	in := &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}
	if versionID != "" {
		in.VersionId = aws.String(versionID)
	}
	_, err := s.client.DeleteObject(ctx, in)
	return err
}
