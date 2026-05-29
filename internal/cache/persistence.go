package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NamedSnapshot is the on-the-wire form of a cache's persisted entries.
// Bytes is opaque to the adapter — each cache serializes its own value
// type (typically JSON) before handing the snapshot to the manager.
type NamedSnapshot struct {
	Name    string
	Bytes   []byte
	SavedAt time.Time
}

// Persistence is the adapter contract. Implementations are supplied by
// the importing application via the root-package WithCachePersistence
// option. Errors returned from Load/Save are logged at WARN inside the
// library and never fail the operation that triggered them.
//
// Load is invoked once during NewClient. Save is invoked when the
// caller invokes Client.SnapshotCaches(ctx) — the library does not
// flush on its own.
type Persistence interface {
	Load(ctx context.Context) ([]NamedSnapshot, error)
	Save(ctx context.Context, snapshots []NamedSnapshot) error
}

// MemoryPersistence is a test helper that stores snapshots in a
// goroutine-safe map.
type MemoryPersistence struct {
	mu    sync.Mutex
	state map[string]NamedSnapshot
}

// NewMemoryPersistence constructs an empty MemoryPersistence.
func NewMemoryPersistence() *MemoryPersistence {
	return &MemoryPersistence{state: map[string]NamedSnapshot{}}
}

// Save implements Persistence.
func (m *MemoryPersistence) Save(_ context.Context, snaps []NamedSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range snaps {
		m.state[s.Name] = s
	}
	return nil
}

// Load implements Persistence.
func (m *MemoryPersistence) Load(_ context.Context) ([]NamedSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]NamedSnapshot, 0, len(m.state))
	for _, s := range m.state {
		out = append(out, s)
	}
	return out, nil
}

// FilePersistence stores snapshots as a JSON array at path. Writes are
// atomic (temp file + rename). Not safe for cross-process concurrent
// writes; intended for single-instance daemons.
type FilePersistence struct {
	path string
	mu   sync.Mutex
}

// NewFilePersistence constructs a FilePersistence rooted at path.
func NewFilePersistence(path string) *FilePersistence {
	return &FilePersistence{path: path}
}

// Save implements Persistence with an atomic temp+rename write.
func (f *FilePersistence) Save(_ context.Context, snaps []NamedSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := json.Marshal(snaps)
	if err != nil {
		return fmt.Errorf("cache: marshal snapshots: %w", err)
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "cache-*.json.tmp")
	if err != nil {
		return fmt.Errorf("cache: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache: write temp: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache: close temp: %w", cerr)
	}
	if rerr := os.Rename(tmpPath, f.path); rerr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache: rename %s -> %s: %w", tmpPath, f.path, rerr)
	}
	return nil
}

// Load implements Persistence. Missing files return ([], nil).
func (f *FilePersistence) Load(_ context.Context) ([]NamedSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache: read %s: %w", f.path, err)
	}
	var out []NamedSnapshot
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("cache: unmarshal %s: %w", f.path, err)
	}
	return out, nil
}
