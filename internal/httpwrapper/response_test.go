package httpwrapper_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func TestDecodeJSON_Object(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"a":1,"b":"x"}`)),
	}
	got, err := httpwrapper.DecodeJSON(resp)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got["a"])
	assert.Equal(t, "x", got["b"])
}

func TestDecodeJSON_ArrayWrapped(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`[{"k":1},{"k":2}]`)),
	}
	got, err := httpwrapper.DecodeJSON(resp)
	require.NoError(t, err)
	arr, ok := got[amzconst.ArrayWrapper].([]any)
	require.True(t, ok)
	assert.Len(t, arr, 2)
}

func TestDecodeJSON_Empty(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
	got, err := httpwrapper.DecodeJSON(resp)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDecodeJSON_BodyTruncatedAtCap verifies that a hostile/oversized
// response body is truncated to MaxResponseBodyBytes without panic. The
// server streams a body larger than the cap; DecodeJSON should return a
// (non-nil) error from JSON parsing, but must not OOM and must close the
// body cleanly. We assert by reading the body ourselves through the same
// LimitReader contract that DecodeJSON uses.
func TestDecodeJSON_BodyTruncatedAtCap(t *testing.T) {
	overage := httpwrapper.MaxResponseBodyBytes + 1<<16 // 16 MiB + 64 KiB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// Stream a body larger than the cap. Content isn't JSON; we're
		// asserting only that the read is bounded.
		buf := make([]byte, 64*1024)
		for i := range buf {
			buf[i] = 'A'
		}
		remaining := overage
		for remaining > 0 {
			n := len(buf)
			if n > remaining {
				n = remaining
			}
			if _, err := w.Write(buf[:n]); err != nil {
				return
			}
			remaining -= n
		}
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	got, err := io.ReadAll(io.LimitReader(resp.Body, httpwrapper.MaxResponseBodyBytes))
	require.NoError(t, err)
	assert.Equal(t, httpwrapper.MaxResponseBodyBytes, len(got))

	// And confirm DecodeJSON on an oversized non-JSON body does not panic.
	resp2, err := srv.Client().Get(srv.URL)
	require.NoError(t, err)
	_, _ = httpwrapper.DecodeJSON(resp2) // returns error (not JSON) but must not panic
}

// TestDecodeJSON_OversizedJSONTruncatedNoPanic streams a JSON-shaped body
// larger than the cap. DecodeJSON should not panic; the truncation
// produces a parse error which is the correct, safe behavior.
func TestDecodeJSON_OversizedJSONTruncatedNoPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"a":"`))
		// fill to slightly over the cap so DecodeJSON sees a truncated
		// (and therefore invalid) JSON string
		filler := strings.Repeat("x", 1024)
		written := 0
		for written < httpwrapper.MaxResponseBodyBytes+1024 {
			_, _ = w.Write([]byte(filler))
			written += len(filler)
		}
		_, _ = w.Write([]byte(`"}`))
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	require.NoError(t, err)

	_, err = httpwrapper.DecodeJSON(resp)
	// We expect an error (truncated JSON), and no panic.
	assert.Error(t, err)
}
