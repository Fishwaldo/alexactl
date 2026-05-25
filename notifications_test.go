package amazondevices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newNotificationsTestClient mirrors newDnDTestClient / newSensorsTestClient.
// Uses the hostRewriter Transport so URLs built from wrapper.Domain() are
// rewritten to the test server.
func newNotificationsTestClient(t *testing.T, srv *httptest.Server) *Client {
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
	return c
}

func TestNotificationsService_Get_GETsExpectedPath(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	var gotPath, gotMethod, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	_, err = c.Notifications.Get(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(gotPath, "/api/notifications"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodGet, gotMethod)
	// Notifications hit uses the Browser UA per upstream
	// implementation/notification.py.
	assert.Contains(t, gotUA, "Mozilla/")
}

func TestNotificationsService_Get_HTTPErrorReturnsAPIError(t *testing.T) {
	// 403 (vs 500) avoids retryablehttp's retry-and-give-up path on
	// idempotent methods; the wrapper hands the response back and we
	// surface it as *APIError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	_, err := c.Notifications.Get(context.Background())
	// Upstream Python treats this failure as "warn + return nil" rather
	// than an error. We follow the project convention: HTTP error surfaces
	// as *APIError; the calling sensorsHook closure catches it and logs
	// WARN without failing the broader Refresh.
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestNotificationsService_Get_ParsesDailyRecurringAlarm(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	// Pin the clock for deterministic next-occurrence calculation.
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	res, err := c.Notifications.Get(context.Background())
	require.NoError(t, err)
	bucket, ok := res["S-KITCHEN-001"]
	require.True(t, ok)

	alarm, ok := bucket["Alarm"]
	require.True(t, ok, "kitchen device should have an Alarm bucket")
	// Earliest among (Morning alarm daily 07:30, Monday standup 09:00,
	// Wake up music daily 06:00) for the next-day window is 06:00 on
	// 2026-05-26 (the daily 06:00 wake-up music).
	assert.Equal(t, "Wake up music", alarm.Label)
	assert.False(t, alarm.NextOccurrence.IsZero())
}

func TestNotificationsService_Get_ParsesOneShotTimer(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	res, err := c.Notifications.Get(context.Background())
	require.NoError(t, err)
	timer, ok := res["S-LIVING-001"]["Timer"]
	require.True(t, ok)
	assert.Equal(t, "Pasta", timer.Label)
	assert.False(t, timer.NextOccurrence.IsZero())
}

func TestNotificationsService_Get_ParsesOneShotReminder(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	res, err := c.Notifications.Get(context.Background())
	require.NoError(t, err)
	reminder, ok := res["S-LIVING-001"]["Reminder"]
	require.True(t, ok)
	assert.Equal(t, "Call dentist", reminder.Label)
}

func TestNotificationsService_Get_SkipsOffStatus(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	res, err := c.Notifications.Get(context.Background())
	require.NoError(t, err)
	_, present := res["S-OFF-001"]
	assert.False(t, present, "OFF entries must not surface")
}

func TestNotificationsService_Get_SkipsIgnoredDeviceType(t *testing.T) {
	body, err := os.ReadFile("testdata/notifications/list.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newNotificationsTestClient(t, srv)
	c.Notifications.now = func() time.Time {
		return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	}

	res, err := c.Notifications.Get(context.Background())
	require.NoError(t, err)
	_, present := res["S-IGNORED-001"]
	assert.False(t, present, "ignored device type must not surface")
}

func TestNotificationsService_AddHoursMinutes(t *testing.T) {
	cases := []struct {
		rule, ts, want string
	}{
		{"FREQ=DAILY", "07:30:00.000", "FREQ=DAILY;BYHOUR=7;BYMINUTE=30"},
		{"FREQ=DAILY;", "07:30:00.000", "FREQ=DAILY;BYHOUR=7;BYMINUTE=30"},
		{"FREQ=WEEKLY;BYDAY=MO", "09:15:00.000", "FREQ=WEEKLY;BYDAY=MO;BYHOUR=9;BYMINUTE=15"},
		{"FREQ=DAILY;BYHOUR=12", "06:45:00.000", "FREQ=DAILY;BYHOUR=12;BYMINUTE=45"},
		{"FREQ=DAILY;BYHOUR=12;BYMINUTE=30", "06:45:00.000", "FREQ=DAILY;BYHOUR=12;BYMINUTE=30"},
		{"FREQ=DAILY", "", "FREQ=DAILY"},
	}
	for _, c := range cases {
		got := addHoursMinutes(c.rule, c.ts)
		assert.Equal(t, c.want, got, "rule=%q time=%q", c.rule, c.ts)
	}
}
