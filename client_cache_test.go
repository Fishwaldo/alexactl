package amazondevices

import (
	"context"
	"testing"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_DefaultsConstructCacheManager(t *testing.T) {
	c, err := NewClient()
	require.NoError(t, err)
	require.NotNil(t, c.cacheManager)
	// Defaults wired through.
	assert.Equal(t, 15*time.Second, c.cacheManager.Classes().TTL(cache.ClassRealtime))
	assert.Equal(t, 5*time.Minute, c.cacheManager.Classes().TTL(cache.ClassShort))
	assert.Equal(t, time.Hour, c.cacheManager.Classes().TTL(cache.ClassLong))
}

func TestClient_WithCacheTTL_Overrides(t *testing.T) {
	c, err := NewClient(WithCacheTTL(ClassShort, 90*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, c.cacheManager.Classes().TTL(cache.ClassShort))
}

func TestClient_DisableCache(t *testing.T) {
	c, err := NewClient(DisableCache(ClassRealtime))
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), c.cacheManager.Classes().TTL(cache.ClassRealtime))
}

func TestClient_SnapshotCaches_NoPersistenceIsNoop(t *testing.T) {
	c, err := NewClient()
	require.NoError(t, err)
	require.NoError(t, c.SnapshotCaches(context.Background()))
}

func TestClient_WithCachePersistence_RestoresOnConstruction(t *testing.T) {
	p := cache.NewMemoryPersistence()
	c, err := NewClient(WithCachePersistence(p))
	require.NoError(t, err)
	require.NotNil(t, c.cacheManager.Persistence())
	require.NoError(t, c.SnapshotCaches(context.Background()))
}
