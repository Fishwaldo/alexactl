// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package httpwrapper_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

func TestRefreshAccessToken_Success(t *testing.T) {
	var capturedForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		capturedForm = map[string]string{}
		for k := range r.Form {
			capturedForm[k] = r.Form.Get(k)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-xyz",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	in := httpwrapper.RefreshInput{
		RefreshToken: "old-refresh",
		TokenURL:     srv.URL + "/auth/token",
	}
	got, ttl, err := wr.RefreshAccessToken(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, "new-access-xyz", got)
	assert.Equal(t, time.Hour, ttl)

	assert.Equal(t, "old-refresh", capturedForm["source_token"])
	assert.Equal(t, "refresh_token", capturedForm["source_token_type"])
	assert.Equal(t, amzconst.RefreshAccessToken, capturedForm["requested_token_type"])
	assert.Equal(t, amzconst.AmazonAppName, capturedForm["app_name"])
}

// TestRefreshAccessToken_DefaultsExpiresIn confirms that a refresh
// response with a missing/non-positive expires_in falls back to the
// 1-hour default rather than returning zero.
func TestRefreshAccessToken_DefaultsExpiresIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access-xyz",
		})
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	tok, ttl, err := wr.RefreshAccessToken(context.Background(), httpwrapper.RefreshInput{
		RefreshToken: "old-refresh",
		TokenURL:     srv.URL + "/auth/token",
	})
	require.NoError(t, err)
	assert.Equal(t, "new-access-xyz", tok)
	assert.Equal(t, time.Hour, ttl, "missing expires_in must default to 1h")
}

func TestRefreshAccessToken_FailureReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	_, _, err = wr.RefreshAccessToken(context.Background(), httpwrapper.RefreshInput{
		RefreshToken: "old-refresh",
		TokenURL:     srv.URL + "/auth/token",
	})
	assert.Error(t, err)
}

// TestRefreshAccessToken_DoesNotRecurseOn401 is the regression guard for
// the refresh-endpoint recursion hazard. If RefreshAccessToken issues its
// Do call WITHOUT the noRefresh context marker, a 401 from /auth/token
// would re-enter Do's transparent-refresh path → refreshFn →
// RefreshAccessToken → … forever. With the fix, the 401 surfaces as
// ErrRefreshHTTP.
//
// The test is bounded by a 3-second context deadline; if it times out,
// recursion happened.
func TestRefreshAccessToken_DoesNotRecurseOn401(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	// Install a refresh closure that itself calls RefreshAccessToken
	// against the same 401-returning endpoint. Without the fix, this
	// would chain refresh calls forever.
	wr.SetRefreshFn(func(ctx context.Context) error {
		_, _, rerr := wr.RefreshAccessToken(ctx, httpwrapper.RefreshInput{
			RefreshToken: "old-refresh",
			TokenURL:     srv.URL + "/auth/token",
		})
		return rerr
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, _, rerr := wr.RefreshAccessToken(ctx, httpwrapper.RefreshInput{
			RefreshToken: "old-refresh",
			TokenURL:     srv.URL + "/auth/token",
		})
		done <- rerr
	}()

	select {
	case rerr := <-done:
		require.Error(t, rerr)
		assert.True(t, errors.Is(rerr, httpwrapper.ErrRefreshHTTP),
			"expected ErrRefreshHTTP, got %v", rerr)
		// One call to /auth/token: the refresh request itself. The
		// no-refresh marker prevents Do from re-entering the refresh
		// path on the 401, so calls must be small and bounded.
		assert.LessOrEqual(t, atomic.LoadInt32(&calls), int32(2),
			"refresh endpoint should be hit at most twice, got %d", atomic.LoadInt32(&calls))
	case <-ctx.Done():
		t.Fatalf("RefreshAccessToken did not return within 3s — recursion regressed (calls=%d)", atomic.LoadInt32(&calls))
	}
}
