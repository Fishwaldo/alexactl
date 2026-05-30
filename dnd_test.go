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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDnDTestClient builds a Client pointed at srv, with a Session
// pre-installed via WithSession so DnDService can resolve its
// AccountCustomerID without going through Auth.Login. Returns the
// already-wired *DnDService.
//
// Uses the hostRewriter Transport pattern (see auth_login_test.go) so the
// production code keeps building real https://alexa.amazon.com URLs while
// every outbound request gets redirected to the httptest server.
func newDnDTestClient(t *testing.T, srv *httptest.Server) *DnDService {
	t.Helper()
	sess := &Session{
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
	return c.DnD
}

func TestDnDService_Status_ParsesAllEntries(t *testing.T) {
	body, err := os.ReadFile("testdata/dnd/status.json")
	require.NoError(t, err)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(gotPath, "/api/dnd/device-status-list"),
		"unexpected request path: %s", gotPath)

	require.Len(t, status, 3)
	require.NotNil(t, status["S-KITCHEN-001"])
	assert.Equal(t, "dnd", status["S-KITCHEN-001"].Name)
	assert.Equal(t, true, status["S-KITCHEN-001"].Value)
	assert.False(t, status["S-KITCHEN-001"].Error)
	assert.Equal(t, false, status["S-LIVING-001"].Value)
	assert.Equal(t, false, status["S-BEDROOM-001"].Value)
}

func TestDnDService_Status_EmptyResponseReturnsEmptyMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"doNotDisturbDeviceStatusList":[]}`))
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)
	status, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.Empty(t, status)
}

func TestDnDService_Status_HTTPErrorReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)
	_, err := svc.Status(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestDnDService_Set_SendsExpectedPayload(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)

	dvc := &Device{
		SerialNumber: "S-LIVING-001",
		DeviceType:   "A18O6U1U9KIYDM",
	}
	err := svc.Set(context.Background(), dvc, true)
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(gotPath, "/api/dnd/status"),
		"path was %s", gotPath)
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "S-LIVING-001", gotBody["deviceSerialNumber"])
	assert.Equal(t, "A18O6U1U9KIYDM", gotBody["deviceType"])
	assert.Equal(t, true, gotBody["enabled"])
}

func TestDnDService_Set_RejectsNilDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("server should not be called for nil device")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)
	err := svc.Set(context.Background(), nil, true)
	require.Error(t, err)
}

func TestDnDService_Set_HTTPErrorReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	svc := newDnDTestClient(t, srv)
	dvc := &Device{
		SerialNumber: "S-LIVING-001",
		DeviceType:   "A18O6U1U9KIYDM",
	}
	err := svc.Set(context.Background(), dvc, true)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}
