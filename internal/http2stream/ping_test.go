// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package http2stream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPing_204Succeeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/ping", r.URL.Path)
		assert.Equal(t, "Bearer atok", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := Ping(context.Background(), srv.Client(), srv.URL+"/ping", "atok")
	require.NoError(t, err)
}

func TestPing_401ReturnsErrPingReauthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := Ping(context.Background(), srv.Client(), srv.URL+"/ping", "atok")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPingReauthRequired))
}

func TestPing_403ReturnsErrPingReauthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	err := Ping(context.Background(), srv.Client(), srv.URL+"/ping", "atok")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPingReauthRequired))
}

func TestPing_500ReturnsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := Ping(context.Background(), srv.Client(), srv.URL+"/ping", "atok")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrPingReauthRequired),
		"500 must not signal reauth — it's a transient server error")
}
