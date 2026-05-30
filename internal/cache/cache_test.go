// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_SetGet_Hit(t *testing.T) {
	c := newCacheForTest[string](t, "test", time.Hour, nil)
	c.Set("k", "v")
	got, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, "v", got)
}

func TestCache_Get_MissNoLoader(t *testing.T) {
	c := newCacheForTest[string](t, "test", time.Hour, nil)
	got, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.False(t, hit)
	assert.Empty(t, got)
}

func TestCache_TTLExpiry(t *testing.T) {
	c := newCacheForTest[string](t, "test", 10*time.Millisecond, nil)
	c.Set("k", "v")
	time.Sleep(20 * time.Millisecond)
	_, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestCache_Invalidate(t *testing.T) {
	c := newCacheForTest[string](t, "test", time.Hour, nil)
	c.Set("k", "v")
	c.Invalidate("k")
	_, hit, _ := c.Get(context.Background(), "k")
	assert.False(t, hit)
}

func TestCache_InvalidateAll(t *testing.T) {
	c := newCacheForTest[string](t, "test", time.Hour, nil)
	c.Set("a", "1")
	c.Set("b", "2")
	c.InvalidateAll()
	_, hit, _ := c.Get(context.Background(), "a")
	assert.False(t, hit)
	_, hit, _ = c.Get(context.Background(), "b")
	assert.False(t, hit)
}

func TestCache_ZeroTTL_SetThenGetIsMiss(t *testing.T) {
	// TTL=0 disables caching per Config docs.
	c := newCacheForTest[string](t, "test", 0, nil)
	c.Set("k", "v")
	_, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.False(t, hit, "TTL=0 should treat every Get as expired/miss")
}

func TestCache_NegativeTTL_SetThenGetIsMiss(t *testing.T) {
	c := newCacheForTest[string](t, "test", -time.Hour, nil)
	c.Set("k", "v")
	_, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.False(t, hit, "negative TTL should also disable caching")
}

func newCacheForTest[V any](t *testing.T, name string, ttl time.Duration,
	loader func(context.Context, string) (V, error)) *Cache[V] {
	t.Helper()
	return NewCache[V](Config{Name: name, TTL: ttl}, loader)
}

func TestCache_Loader_PopulatesOnMiss(t *testing.T) {
	calls := 0
	loader := func(ctx context.Context, key string) (string, error) {
		calls++
		return "from-loader:" + key, nil
	}
	c := newCacheForTest[string](t, "test", time.Hour, loader)
	got, hit, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, "from-loader:k", got)
	assert.Equal(t, 1, calls)

	// Second call hits the cache.
	got, hit, err = c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, "from-loader:k", got)
	assert.Equal(t, 1, calls)
}

func TestCache_Loader_ErrorNotCached(t *testing.T) {
	calls := 0
	loader := func(ctx context.Context, key string) (string, error) {
		calls++
		return "", assert.AnError
	}
	c := newCacheForTest[string](t, "test", time.Hour, loader)
	_, hit, err := c.Get(context.Background(), "k")
	assert.Error(t, err)
	assert.False(t, hit)
	_, _, err = c.Get(context.Background(), "k")
	assert.Error(t, err)
	assert.Equal(t, 2, calls, "loader must NOT be cached on error")
}

func TestCache_Loader_SingleFlight(t *testing.T) {
	var calls int64
	release := make(chan struct{})
	loader := func(ctx context.Context, key string) (string, error) {
		atomic.AddInt64(&calls, 1)
		<-release
		return "v", nil
	}
	c := newCacheForTest[string](t, "test", time.Hour, loader)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = c.Get(context.Background(), "k")
		}()
	}
	// Let goroutines pile up in the loader, then release.
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls))
}

func TestCache_Loader_ContextCancellation(t *testing.T) {
	loader := func(ctx context.Context, key string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	c := newCacheForTest[string](t, "test", time.Hour, loader)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, hit, err := c.Get(ctx, "k")
	assert.Error(t, err)
	assert.False(t, hit)
}

func TestClassConfig_DefaultTTLs(t *testing.T) {
	cfg := DefaultClassConfig()
	assert.Equal(t, 15*time.Second, cfg.TTL(ClassRealtime))
	assert.Equal(t, 5*time.Minute, cfg.TTL(ClassShort))
	assert.Equal(t, time.Hour, cfg.TTL(ClassLong))
}

func TestClassConfig_OverrideAndDisable(t *testing.T) {
	cfg := DefaultClassConfig()
	cfg.Set(ClassShort, 90*time.Second)
	assert.Equal(t, 90*time.Second, cfg.TTL(ClassShort))
	cfg.Set(ClassRealtime, 0)
	assert.Equal(t, time.Duration(0), cfg.TTL(ClassRealtime))
}
