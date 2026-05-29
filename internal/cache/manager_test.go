package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_InvalidateTag(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)

	sessionCache := NewCache[string](Config{Name: "session-bound", TTL: time.Hour, Tags: []string{"session"}}, nil)
	plainCache := NewCache[string](Config{Name: "plain", TTL: time.Hour}, nil)

	Register(m, sessionCache, nil, nil)
	Register(m, plainCache, nil, nil)

	sessionCache.Set("k", "v")
	plainCache.Set("k", "v")

	m.InvalidateTag("session")

	_, hit, _ := sessionCache.Get(context.Background(), "k")
	assert.False(t, hit, "session-tagged cache should be cleared")

	_, hit, _ = plainCache.Get(context.Background(), "k")
	assert.True(t, hit, "untagged cache should remain")
}

func TestManager_RegisterCount(t *testing.T) {
	m := NewManager(slog.Default(), DefaultClassConfig(), nil)
	c1 := NewCache[string](Config{Name: "a", TTL: time.Hour}, nil)
	c2 := NewCache[int](Config{Name: "b", TTL: time.Hour}, nil)
	Register(m, c1, nil, nil)
	Register(m, c2, nil, nil)
	require.Equal(t, 2, m.Len())
}

func TestManager_SnapshotRestoreRoundTrip(t *testing.T) {
	persist := NewMemoryPersistence()
	m := NewManager(slog.Default(), DefaultClassConfig(), persist)
	c := NewCache[string](Config{Name: "round-trip", TTL: time.Hour}, nil)
	Register(m, c,
		func(in map[string]Entry[string]) ([]byte, error) {
			return json.Marshal(in)
		},
		func(b []byte) (map[string]Entry[string], error) {
			out := map[string]Entry[string]{}
			err := json.Unmarshal(b, &out)
			return out, err
		},
	)
	c.Set("k", "v")
	require.NoError(t, m.Snapshot(context.Background()))

	// Construct a fresh manager + cache, restore, assert hit.
	m2 := NewManager(slog.Default(), DefaultClassConfig(), persist)
	c2 := NewCache[string](Config{Name: "round-trip", TTL: time.Hour}, nil)
	Register(m2, c2,
		func(in map[string]Entry[string]) ([]byte, error) { return json.Marshal(in) },
		func(b []byte) (map[string]Entry[string], error) {
			out := map[string]Entry[string]{}
			err := json.Unmarshal(b, &out)
			return out, err
		},
	)
	require.NoError(t, m2.Restore(context.Background()))
	got, hit, err := c2.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, "v", got)
}

func TestManager_RestoreDropsExpiredEntries(t *testing.T) {
	persist := NewMemoryPersistence()
	// Save a snapshot whose entry is already expired.
	expired := map[string]Entry[string]{
		"k": {Value: "v", ExpiresAt: time.Now().Add(-time.Minute)},
	}
	bytes, err := json.Marshal(expired)
	require.NoError(t, err)
	require.NoError(t, persist.Save(context.Background(), []NamedSnapshot{
		{Name: "expired", Bytes: bytes, SavedAt: time.Now()},
	}))

	m := NewManager(slog.Default(), DefaultClassConfig(), persist)
	c := NewCache[string](Config{Name: "expired", TTL: time.Hour}, nil)
	Register(m, c,
		func(in map[string]Entry[string]) ([]byte, error) { return json.Marshal(in) },
		func(b []byte) (map[string]Entry[string], error) {
			out := map[string]Entry[string]{}
			err := json.Unmarshal(b, &out)
			return out, err
		},
	)
	require.NoError(t, m.Restore(context.Background()))
	_, hit, _ := c.Get(context.Background(), "k")
	assert.False(t, hit, "expired entry must NOT survive Restore")
}
