// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHistoryTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	sess := &Session{
		AccessToken:       "Atna|TESTBEARER",
		RefreshToken:      "Atnr|TESTREFRESH",
		AccountCustomerID: "amzn1.account.TEST",
		Site:              "https://www.amazon.com",
		Domain:            "com",
		DeviceInfo: DeviceInfo{
			DeviceSerialNumber: "TEST-SERIAL",
			DeviceType:         "TEST-TYPE",
		},
	}
	hc := &http.Client{Transport: &hostRewriter{base: srv.URL, inner: srv.Client().Transport}}
	c, err := NewClient(
		WithHTTPClient(hc),
		WithSite("https://www.amazon.com"),
		WithSession(sess),
	)
	require.NoError(t, err)
	// Speed up the 2s backend wait in tests.
	c.History.sleep = func(ctx context.Context, _ time.Duration) error {
		return nil
	}
	return c
}

func TestHistoryService_Vocal_FetchesCSRFAndRecords(t *testing.T) {
	frontend, err := os.ReadFile("testdata/history/frontend.html")
	require.NoError(t, err)
	recordsBody, err := os.ReadFile("testdata/history/records.json")
	require.NoError(t, err)

	var gotCSRF, gotBearer string
	var gotPostPath string
	var gotPostBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/alexa-privacy/apd/rvh"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(frontend)
		case strings.HasSuffix(r.URL.Path, "/alexa-privacy/apd/rah/alexa-history-records-v2"):
			gotPostPath = r.URL.Path
			gotCSRF = r.Header.Get("anti-csrftoken-a2z")
			gotBearer = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&gotPostBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(recordsBody)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newHistoryTestClient(t, srv)

	res, err := c.History.Vocal(context.Background())
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(gotPostPath, "/alexa-privacy/apd/rah/alexa-history-records-v2"))
	assert.Equal(t, "CSRF-TOKEN-FIXTURE-VALUE", gotCSRF)
	assert.True(t, strings.HasPrefix(gotBearer, "Bearer "), "bearer header: %q", gotBearer)
	assert.Contains(t, gotBearer, "Atna|") // tokens start with this prefix
	require.NotNil(t, gotPostBody)
	_, hasToken := gotPostBody["previousRequestToken"]
	assert.True(t, hasToken)

	// Latest record per serial wins; ASR_TIMEOUT is filtered.
	require.Contains(t, res, "S-KITCHEN-001")
	assert.Equal(t, "PlayMusic", res["S-KITCHEN-001"].Intent)
	require.Contains(t, res, "S-LIVING-001")
	assert.Equal(t, "SetTimer", res["S-LIVING-001"].Intent,
		"ASR_TIMEOUT must be filtered; the SetTimer record should win")
}

func TestVocalHandlesRecordTypeAndArrayDeviceInfo(t *testing.T) {
	frontend, err := os.ReadFile("testdata/history/frontend.html")
	require.NoError(t, err)
	recordsBody, err := os.ReadFile("testdata/history/records.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/alexa-privacy/apd/rvh"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(frontend)
		case strings.HasSuffix(r.URL.Path, "/alexa-privacy/apd/rah/alexa-history-records-v2"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(recordsBody)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := newHistoryTestClient(t, srv)

	got, err := c.History.Vocal(context.Background())
	require.NoError(t, err)

	require.NotNil(t, got["SERIAL_ARR"], "array-shaped deviceInfo should be kept")

	require.NotNil(t, got["SERIAL_RT"])
	assert.Equal(t, "CustomerReview", got["SERIAL_RT"].HistoryType)
	assert.Equal(t, "Unknown", got["SERIAL_RT"].Intent)

	for serial := range got {
		assert.NotEqual(t, "", serial, "null deviceInfo record must not add a phantom serial")
	}
}

func TestHistoryService_Vocal_MissingCSRFErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/alexa-privacy/apd/rvh") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body>no csrf here</body></html>"))
			return
		}
		t.Errorf("history POST should not fire when CSRF is missing")
	}))
	t.Cleanup(srv.Close)

	c := newHistoryTestClient(t, srv)
	_, err := c.History.Vocal(context.Background())
	require.Error(t, err)
}
