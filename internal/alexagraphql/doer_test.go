package alexagraphql_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/alexagraphql"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// TestDoer_ForwardsRequestBody verifies the adapter sends pre-marshaled
// GraphQL bytes through httpwrapper.Wrapper without re-encoding, and
// returns the response untouched.
func TestDoer_ForwardsRequestBody(t *testing.T) {
	var gotBody []byte
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"hello":"world"}}`))
	}))
	t.Cleanup(srv.Close)

	w := newTestWrapper(t, srv)
	doer := alexagraphql.NewDoer(w)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		srv.URL+"/nexus/v1/graphql",
		bytes.NewReader([]byte(`{"query":"query{hello}"}`)),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := doer.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/nexus/v1/graphql", gotPath)
	assert.Equal(t, `{"query":"query{hello}"}`, string(gotBody),
		"request body bytes pass through unchanged")
	assert.Equal(t, `{"data":{"hello":"world"}}`, string(body),
		"response body returned to caller untouched")
}

// TestDoer_PropagatesContext verifies that the context attached to the
// inbound *http.Request flows into wrapper.Do so cancellations are
// honored.
func TestDoer_PropagatesContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the context cancels; client should observe a
		// "context canceled" error.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	w := newTestWrapper(t, srv)
	doer := alexagraphql.NewDoer(w)

	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, srv.URL, strings.NewReader(""),
	)
	require.NoError(t, err)

	go func() {
		<-time.After(10 * time.Millisecond)
		cancel()
	}()

	_, err = doer.Do(req)
	require.Error(t, err)
	assert.ErrorContains(t, err, "context")
}

// newTestWrapper builds an httpwrapper.Wrapper sharing srv's HTTP
// client so requests reach the test server. Mirrors the pattern used
// by internal/httpwrapper/do_test.go's newWrapper helper.
func newTestWrapper(t *testing.T, srv *httptest.Server) *httpwrapper.Wrapper {
	t.Helper()
	w, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       srv.URL,
		Version:    "0.0.0-test",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	return w
}
