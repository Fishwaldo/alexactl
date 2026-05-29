package amazondevices

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/cache"
)

func TestMedia_Sounds_DecodesEntities(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/behaviors/entities")
		assert.Contains(t, r.URL.RawQuery, "skillId=amzn1.ask.1p.sound")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	got, err := ms.Sounds(context.Background())
	require.NoError(t, err)

	// AVAILABLE entries are surfaced.
	require.Contains(t, got, "amzn_sfx_doorbell_01")
	assert.Equal(t, "Doorbell 1", got["amzn_sfx_doorbell_01"].DisplayName)
	assert.Equal(t, "bell_and_buzzers", got["amzn_sfx_doorbell_01"].Category)
	assert.Equal(t, "Bells & Buzzers", got["amzn_sfx_doorbell_01"].CategoryDisplayName)
	require.Contains(t, got, "amzn_sfx_cat_meow_1x_01")
	require.Contains(t, got, "christmas_05")

	// UNAVAILABLE entry dropped.
	_, hasDeprecated := got["deprecated_sound"]
	assert.False(t, hasDeprecated)

	// Empty-id entry dropped.
	_, hasEmpty := got[""]
	assert.False(t, hasEmpty)

	// Empty-displayName entry dropped.
	_, hasMissingName := got["missing_name_sound"]
	assert.False(t, hasMissingName)
}

func TestMedia_Sounds_CacheReturnsWithoutSecondCall(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	// newMediaTest already wires a 1h TTL cache; leave it.

	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)
	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

func TestMedia_Sounds_CacheExpiresAfterTTL(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	// Override the cache TTL to a short window for expiry verification.
	ms.soundsCache = cache.NewCache[map[string]Sound](
		cache.Config{Name: "media.sounds", TTL: 50 * time.Millisecond},
		func(ctx context.Context, _ string) (map[string]Sound, error) { return ms.fetchSounds(ctx) },
	)

	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))

	time.Sleep(80 * time.Millisecond)
	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestMedia_Sounds_NoCacheWhenTTLZero(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	// Override with a TTL=0 cache to disable caching.
	ms.soundsCache = cache.NewCache[map[string]Sound](
		cache.Config{Name: "media.sounds", TTL: 0},
		func(ctx context.Context, _ string) (map[string]Sound, error) { return ms.fetchSounds(ctx) },
	)

	for i := 0; i < 3; i++ {
		_, err := ms.Sounds(context.Background())
		require.NoError(t, err)
	}
	assert.Equal(t, int32(3), atomic.LoadInt32(&hits))
}

func TestMedia_Sounds_ReturnedMapIsDefensiveCopy(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	first, err := ms.Sounds(context.Background())
	require.NoError(t, err)
	// Mutate the returned map.
	first["amzn_sfx_doorbell_01"] = Sound{ID: "MUTATED"}
	delete(first, "amzn_sfx_cat_meow_1x_01")

	// Second call (within TTL) returns a fresh copy of the unmutated cache.
	second, err := ms.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Doorbell 1", second["amzn_sfx_doorbell_01"].DisplayName)
	assert.Contains(t, second, "amzn_sfx_cat_meow_1x_01")
}

func TestMedia_Sounds_SessionRefreshInvalidatesCache(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))

	// Simulate account switch: a fresh Session pushed via the broadcast
	// pipeline should drop the cached catalogue.
	ms.setSession(&Session{AccountCustomerID: "OTHER", Domain: "com"})

	_, err = ms.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestMedia_Sounds_APIError5xx(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})

	_, err := ms.Sounds(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 500, apiErr.StatusCode)
	assert.True(t, errors.Is(apiErr.Category, ErrCannotRetrieveData))
}

func TestMedia_Sounds_APIError4xx(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := ms.Sounds(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 401, apiErr.StatusCode)
}

func TestMedia_Sounds_DecodeError(t *testing.T) {
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not-json}`))
	})

	_, err := ms.Sounds(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCannotRetrieveData),
		"expected ErrCannotRetrieveData, got %v", err)
}

func TestMedia_Sounds_ConcurrentCallersShareFlight(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	ms, _ := newMediaTest(t, func(w http.ResponseWriter, _ *http.Request) {
		// Slow the response a bit so the second goroutine reliably
		// enters Sounds() while the first is still in flight.
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ms.Sounds(context.Background())
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// First caller fetches; the other three see the cached value.
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

func TestMedia_Sounds_SessionTagClearsCache(t *testing.T) {
	body, err := os.ReadFile("testdata/media/sounds.json")
	require.NoError(t, err)
	var hits int32
	c, srv := newClientForSoundsTest(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	defer srv.Close()

	_, err = c.Media.Sounds(context.Background())
	require.NoError(t, err)

	// Swap session → manager's session-tag invalidation fires.
	c.Auth.broadcast(&Session{AccountCustomerID: "OTHER", Domain: "com"})

	_, err = c.Media.Sounds(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

// newClientForSoundsTest stands up a real Client wired to a test server.
func newClientForSoundsTest(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}
	c, err := NewClient(
		WithHTTPClient(hc),
		WithSite(srv.URL),
		WithSession(&Session{AccountCustomerID: "OWNER", Domain: "com"}),
	)
	require.NoError(t, err)
	return c, srv
}
