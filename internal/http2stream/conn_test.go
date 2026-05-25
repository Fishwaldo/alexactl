package http2stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMultipartPart writes one valid multipart part with boundary "B"
// containing the supplied JSON body. The caller writes the closing
// boundary itself.
func writeMultipartPart(w http.ResponseWriter, body string) {
	fmt.Fprintf(w, "--B\r\nContent-Type: application/json\r\n\r\n%s\r\n", body)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func TestConn_OpensAndStreamsParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer atok", r.Header.Get("Authorization"))
		assert.Equal(t, "multipart/related", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		writeMultipartPart(w, `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE"}]}}}`)
		writeMultipartPart(w, `{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_AUDIO_PLAYER_STATE"}]}}}`)
		fmt.Fprint(w, "--B--\r\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	conn := &Conn{Client: srv.Client(), URL: srv.URL + "/dir", AccessToken: "atok"}
	require.NoError(t, conn.Open(context.Background()))
	defer conn.Close()

	parts := conn.Parts()
	var got []string
	for p := range parts {
		got = append(got, string(p))
	}
	require.NoError(t, conn.Err(),
		"clean stream-end is not an error")
	require.Len(t, got, 2)
	assert.Contains(t, got[0], `"PUSH_VOLUME_CHANGE"`)
	assert.Contains(t, got[1], `"PUSH_AUDIO_PLAYER_STATE"`)
}

func TestConn_AuthFailureReturnsErrStreamReauthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	conn := &Conn{Client: srv.Client(), URL: srv.URL + "/dir", AccessToken: "atok"}
	err := conn.Open(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrStreamReauthRequired))
}

func TestConn_MissingBoundaryReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "multipart/related") // no boundary
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	conn := &Conn{Client: srv.Client(), URL: srv.URL + "/dir", AccessToken: "atok"}
	err := conn.Open(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrBoundaryMissing))
}

func TestConn_Connected_ClosesOnFirstSuccessfulOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "multipart/related; boundary=B")
		w.WriteHeader(http.StatusOK)
		// Keep the connection open by writing one part then sleeping.
		writeMultipartPart(w, `{"directive":{"payload":{"renderingUpdates":[]}}}`)
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "--B--\r\n")
	}))
	defer srv.Close()
	conn := &Conn{Client: srv.Client(), URL: srv.URL + "/dir", AccessToken: "atok"}
	require.NoError(t, conn.Open(context.Background()))
	defer conn.Close()

	select {
	case <-conn.Connected():
		// expected: Connected() fires as soon as headers are read
	case <-time.After(time.Second):
		t.Fatal("Connected() was not closed after a successful Open")
	}

	// Drain so the goroutine doesn't leak.
	for range conn.Parts() {
	}
}

// Compile-time interface assertions: Conn.Parts returns a recv-only
// json-bytes channel.
var _ func() <-chan []byte = (&Conn{}).Parts

// io.EOF is the standard sentinel; we should not surface it as an
// error from Err(), which is what TestConn_OpensAndStreamsParts
// asserts. This compile-time check just makes sure we're using the
// stdlib symbol.
var _ = io.EOF
