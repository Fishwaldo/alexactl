package httpwrapper_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func newWrapper(t *testing.T, srv *httptest.Server) *httpwrapper.Wrapper {
	t.Helper()
	w, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		Version:    "0.1.0-test",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	require.NoError(t, w.SetSite("https://www.amazon.com"))
	return w
}

func TestDo_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"hello":"world"}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method:   http.MethodPost,
		URL:      srv.URL + "/anything",
		JSONBody: map[string]any{"hello": "world"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"ok":true}`, string(body))
}

func TestDo_RetriesOn503(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/r",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 3, count)
}

func TestDo_CSRFCookieCaptured(t *testing.T) {
	var observed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.Header.Get("csrf")
		http.SetCookie(w, &http.Cookie{Name: "csrf", Value: "csrf-token-xyz", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{Method: http.MethodGet, URL: srv.URL + "/a"})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Second request should carry the captured CSRF token in the header.
	resp2, err := wr.Do(context.Background(), httpwrapper.Request{Method: http.MethodGet, URL: srv.URL + "/b"})
	require.NoError(t, err)
	defer resp2.Body.Close()
	// observed is updated by handler on call 2 — assert second request had the CSRF header.
	assert.Equal(t, "csrf-token-xyz", observed)
}

func TestDo_FormBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "v1", r.FormValue("k1"))
		assert.Equal(t, "v2", r.FormValue("k2"))
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodPost,
		URL:    srv.URL + "/form",
		Form:   map[string]string{"k1": "v1", "k2": "v2"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestDo_HonorsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wr := newWrapper(t, srv)
	_, err := wr.Do(ctx, httpwrapper.Request{Method: http.MethodGet, URL: srv.URL})
	assert.Error(t, err)
}

func TestDo_LogsRedactsCredentialFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Logger:     logger,
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodPost,
		URL:    srv.URL + "/x",
		Form:   map[string]string{"password": "hunter2", "keep": "me"},
	})
	require.NoError(t, err)
	_ = resp.Body.Close()

	out := logBuf.String()
	assert.Contains(t, out, `"password":"[REDACTED]"`)
	assert.Contains(t, out, `"keep":"me"`)
	assert.NotContains(t, out, "hunter2")
	assert.Contains(t, out, `"status":200`)
}

// TestDo_NoRetryPOSTOn503 verifies the idempotent-only retry policy:
// POSTs are NOT retried on 503 (only on 429, RFC 6585).
func TestDo_NoRetryPOSTOn503(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodPost,
		URL:    srv.URL + "/p",
	})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.EqualValues(t, 1, atomic.LoadInt32(&count), "POST on 503 must not be retried")
}

// TestDo_RetryGETOn503 sanity-checks that GET still retries on 503 after
// the idempotent-only refactor (a regression guard for D3).
func TestDo_RetryGETOn503(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/g",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&count), int32(2))
}

// TestDo_RetryPOSTOn429 verifies POST IS retried on 429 (rate-limit).
func TestDo_RetryPOSTOn429(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodPost,
		URL:    srv.URL + "/p429",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&count), int32(2))
}

// TestDo_TransparentRefreshOn401 verifies the refreshFn is invoked on a
// 401 and the original request is retried once. The handler returns 401
// on first call, 200 on the retry, and exposes a counter for the
// refresh-closure invocations.
func TestDo_TransparentRefreshOn401(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	var refreshCount int32
	wr.SetRefreshFn(func(ctx context.Context) error {
		atomic.AddInt32(&refreshCount, 1)
		return nil
	})

	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/r",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 1, atomic.LoadInt32(&refreshCount), "refresh closure must be invoked exactly once")
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls), "request retried exactly once after refresh")
}

// TestDo_NoSecondRefreshOn401AfterRefresh ensures the wrapper does not
// loop refresh on a sustained 401 — the recursive retry surfaces the
// 401 to the caller.
func TestDo_NoSecondRefreshOn401AfterRefresh(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	var refreshCount int32
	wr.SetRefreshFn(func(ctx context.Context) error {
		atomic.AddInt32(&refreshCount, 1)
		return nil
	})

	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/r",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.EqualValues(t, 1, atomic.LoadInt32(&refreshCount), "refresh must run only once even on persistent 401")
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls), "two calls total: initial 401 + post-refresh retry")
}

func TestDo_NoBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		assert.Empty(t, b)
		assert.Empty(t, r.Header.Get("Content-Type"))
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	wr := newWrapper(t, srv)
	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/empty",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", strings.TrimSpace(string(body)))
}

// TestDo_ExhaustedRetriesReturnFinalResponse verifies that when all retry
// attempts are exhausted the wrapper returns (response, nil) so callers
// can inspect the status code, rather than (nil, error). This is the
// load-bearing behavior of retryablehttp.PassthroughErrorHandler installed
// in Do; without it, downstream "if resp.StatusCode >= 400" branches in
// every service method would be unreachable on exhausted 5xx.
func TestDo_ExhaustedRetriesReturnFinalResponse(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&count, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient:   srv.Client(),
		Site:         "https://www.amazon.com",
		RetryMax:     1,
		RetryWaitMin: time.Millisecond,
		RetryWaitMax: time.Millisecond,
		RandReader:   bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	resp, err := wr.Do(context.Background(), httpwrapper.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/x",
	})
	require.NoError(t, err, "exhausted retries must return (resp, nil), not (nil, err)")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.EqualValues(t, 2, atomic.LoadInt32(&count), "two total attempts: initial + 1 retry")
}

func TestDo_RawBody_NotReMarshaled(t *testing.T) {
	var gotBody []byte
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	w := newWrapper(t, srv)
	resp, err := w.Do(t.Context(), httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          srv.URL,
		RawBody:      []byte(`{"already":"encoded"}`),
		ExtraHeaders: http.Header{"Content-Type": []string{"application/graphql-response+json"}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, `{"already":"encoded"}`, string(gotBody),
		"RawBody bytes must be sent verbatim — no double JSON encode")
	assert.Equal(t, "application/graphql-response+json", gotCT,
		"caller-supplied Content-Type must win when RawBody is used")
}
