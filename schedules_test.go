package alexactl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSchedules_ParsesAllKinds(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	out, err := fetchSchedules(context.Background(), c.wrapper)
	require.NoError(t, err)

	// Fixture has: 3 Alarms (1 one-shot + 1 recurring + 1 OFF + 1 MusicAlarm
	// normalised → 4 total raw entries of Alarm-ish), 2 Reminders, 1 Timer.
	// fetchSchedules returns raw entries split by kind without status
	// filtering — Get(ctx) on each service filters later.
	assert.Len(t, out.Alarms, 4, "Alarm + MusicAlarm raw entries (normalised by callers)")
	assert.Len(t, out.Reminders, 2)
	assert.Len(t, out.Timers, 1)
}

func TestFetchSchedules_WrapsTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	_, err := fetchSchedules(context.Background(), c.wrapper)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotConnect)
}

func TestFetchSchedules_WrapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	_, err := fetchSchedules(context.Background(), c.wrapper)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	assert.ErrorIs(t, err, ErrCannotRetrieveData)
}

// newSchedulesTestClient mirrors newListsTestClient. Defined here so
// all three new service test files share it across the package.
func newSchedulesTestClient(t *testing.T, srv *httptest.Server) *Client {
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
