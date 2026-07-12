// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// newDeviceRestartTest builds a *DevicesService whose wrapper routes every
// outbound request to the given test server, regardless of the host the
// production code formats into the URL (alexa.amazon.<tld>). Mirrors the
// hostRewriter seam used by newMediaTest / auth_login_test.go.
func newDeviceRestartTest(t *testing.T, handler http.HandlerFunc) (*DevicesService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RetryMax:   -1,
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com", AccessToken: "tok"},
	}
	return ds, srv
}

func TestDevicesRestartPostsRebootWithBearer(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var posted map[string]any
	ds, _ := newDeviceRestartTest(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
		w.WriteHeader(http.StatusOK)
	})

	dev := &Device{SerialNumber: "SER1", DeviceType: "DT1"}
	err := ds.Restart(context.Background(), dev)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v1/alexa/device/reboot/DT1/SER1", gotPath)
	assert.Equal(t, "Bearer tok", gotAuth)
	assert.Equal(t, "SER1", posted["deviceSerialNumber"])
	assert.Equal(t, "DT1", posted["deviceType"])
}

func TestDevicesRestartWrapsErrorOn4xx(t *testing.T) {
	ds, _ := newDeviceRestartTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	dev := &Device{SerialNumber: "SER1", DeviceType: "DT1"}
	err := ds.Restart(context.Background(), dev)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCannotRestartDevice))
}

func TestDevicesRestartNoAccessTokenFails(t *testing.T) {
	ds, _ := newDeviceRestartTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ds.setSession(&Session{AccountCustomerID: "OWNER", Domain: "com"})

	dev := &Device{SerialNumber: "SER1", DeviceType: "DT1"}
	err := ds.Restart(context.Background(), dev)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCannotAuthenticate))
}
