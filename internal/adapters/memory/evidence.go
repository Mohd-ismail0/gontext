package memory

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/xsama/context-fabric/internal/platform"
	"github.com/xsama/context-fabric/internal/ports"
)

// EvidenceStore is an in-memory EvidenceStore (tests/demo).
type EvidenceStore struct {
	mu   sync.RWMutex
	objs map[string]map[string]evidenceBlob // key -> version -> blob
}

type evidenceBlob struct {
	meta ports.EvidenceObject
	data []byte
}

// NewEvidence creates an empty in-memory evidence store.
func NewEvidence() *EvidenceStore {
	return &EvidenceStore{objs: make(map[string]map[string]evidenceBlob)}
}

var _ ports.EvidenceStore = (*EvidenceStore)(nil)

func (e *EvidenceStore) Put(_ context.Context, key string, body io.Reader, contentType string, meta map[string]string) (ports.EvidenceObject, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return ports.EvidenceObject{}, err
	}
	ver := platform.NewEventID()
	obj := ports.EvidenceObject{
		Key:         key,
		VersionID:   ver,
		Size:        int64(len(data)),
		ContentType: contentType,
		Metadata:    meta,
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.objs[key]
	if m == nil {
		m = make(map[string]evidenceBlob)
		e.objs[key] = m
	}
	m[ver] = evidenceBlob{meta: obj, data: data}
	return obj, nil
}

func (e *EvidenceStore) Get(_ context.Context, key, versionID string) (io.ReadCloser, ports.EvidenceObject, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	m := e.objs[key]
	if m == nil {
		return nil, ports.EvidenceObject{}, platform.ErrNotFound("evidence not found")
	}
	if versionID == "" {
		for _, b := range m {
			return io.NopCloser(bytes.NewReader(b.data)), b.meta, nil
		}
		return nil, ports.EvidenceObject{}, platform.ErrNotFound("evidence not found")
	}
	b, ok := m[versionID]
	if !ok {
		return nil, ports.EvidenceObject{}, platform.ErrNotFound("evidence version not found")
	}
	return io.NopCloser(bytes.NewReader(b.data)), b.meta, nil
}

func (e *EvidenceStore) PresignPut(_ context.Context, key string, opts ports.PresignOptions) (string, time.Time, error) {
	exp := time.Now().UTC().Add(opts.ExpiresIn)
	if opts.ExpiresIn <= 0 {
		exp = time.Now().UTC().Add(15 * time.Minute)
	}
	return "memory://upload/" + key, exp, nil
}

func (e *EvidenceStore) DeleteVersion(_ context.Context, key, versionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.objs[key]
	if m == nil {
		return platform.ErrNotFound("evidence not found")
	}
	delete(m, versionID)
	return nil
}
