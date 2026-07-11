// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// newCommsTest builds a *CommunicationsService whose wrapper routes every
// outbound request to the given test server, regardless of the absolute
// CommSite host formatted into the URL. Mirrors the hostRewriter seam used
// by newDeviceRestartTest / auth_login_test.go.
func newCommsTest(t *testing.T, handler http.HandlerFunc) (*CommunicationsService, *httptest.Server) {
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
	cs := &CommunicationsService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com", AccessToken: "tok"},
	}
	return cs, srv
}

func TestCommsGetPreferencesKeepsOnlyAllowed(t *testing.T) {
	var gotPath string
	cs, _ := newCommsTest(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devicePermissionsPreferences":[
			{"devicePreference":"communications","state":"ON","allowed":true},
			{"devicePreference":"dropin","state":"Off","allowed":false},
			{"devicePreference":"announcements","state":"ON","allowed":true}
		]}`))
	})

	dev := &Device{SerialNumber: "SER", DeviceType: "DT"}
	got, err := cs.GetPreferences(context.Background(), []*Device{dev})
	require.NoError(t, err)

	require.Contains(t, got, "SER")
	prefs := got["SER"]
	assert.Equal(t, "ON", prefs["communications"])
	assert.Equal(t, "ON", prefs["announcements"])
	_, hasDropin := prefs["dropin"]
	assert.False(t, hasDropin, "dropin should be filtered out since allowed=false")
	assert.Contains(t, gotPath, "/devicesTypes/DT/deviceId/SER/preferences")
}

func TestCommsSetAnnouncementStatusPATCHes(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	cs, _ := newCommsTest(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})

	dev := &Device{SerialNumber: "SER", DeviceType: "DT"}
	err := cs.SetAnnouncementStatus(context.Background(), dev, true)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPatch, gotMethod)
	assert.True(t, hasSuffix(gotPath, "/devicesTypes/DT/deviceId/SER/preferences/announcements"))
	assert.Equal(t, "ON", body["state"])
}

func TestCommsSetDropInStatusPATCHes(t *testing.T) {
	var gotMethod, gotPath string
	var body map[string]any
	cs, _ := newCommsTest(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})

	dev := &Device{SerialNumber: "SER", DeviceType: "DT"}
	err := cs.SetDropInStatus(context.Background(), dev, DropInStatusHome)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPatch, gotMethod)
	assert.True(t, hasSuffix(gotPath, "/devicesTypes/DT/deviceId/SER/preferences/dropin"))
	assert.Equal(t, "Home", body["state"])
}

func TestCommsSkipsSpeakerGroupAndAQM(t *testing.T) {
	calls := 0
	cs, _ := newCommsTest(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devicePermissionsPreferences":[]}`))
	})

	wha := &Device{SerialNumber: "WHA1", DeviceType: "DT", DeviceFamily: amzconst.SpeakerGroupFamily}
	aqm := &Device{SerialNumber: "AQM1", DeviceType: amzconst.DeviceTypeAQM}
	got, err := cs.GetPreferences(context.Background(), []*Device{wha, aqm})
	require.NoError(t, err)

	assert.Equal(t, 0, calls)
	assert.NotContains(t, got, "WHA1")
	assert.NotContains(t, got, "AQM1")
}

// hasSuffix is a tiny local helper to avoid importing strings twice for one check.
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
