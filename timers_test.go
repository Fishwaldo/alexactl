package amazondevices

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimersService_Get_ParsesTimersOnly(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	timers, err := c.Timers.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, timers, 1)
	assert.Equal(t, "pasta", timers[0].Label)
}

func TestTimersService_Get_ParsesDurationAndFireAt(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	timers, err := c.Timers.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, timers, 1)

	pasta := timers[0]
	// originalDurationInMillis = 600000 = 10 minutes
	assert.Equal(t, 10*time.Minute, pasta.Duration)
	// triggerTime = 1779979283041 ms
	assert.Equal(t, time.UnixMilli(1779979283041), pasta.FireAt)
}

func TestTimersService_Get_SetsBackRef(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	timers, err := c.Timers.Get(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, timers)

	for _, tm := range timers {
		assert.Same(t, c.Timers, tm.service)
		assert.NotNil(t, tm.serverShape)
	}
}

func TestTimersService_Create_PutsBodyToCreateTimer(t *testing.T) {
	respBody, err := os.ReadFile("testdata/schedules/create_timer_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createTimer") {
			gotMethod = r.Method
			gotPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(respBody)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	c.Devices.setCacheForTest(map[string]*Device{
		"GSM427065425023K": {SerialNumber: "GSM427065425023K", DeviceType: "A345JHMQDGG1M5"},
	})

	tm := &Timer{
		DeviceSerialNumber: "GSM427065425023K",
		Duration:           3 * time.Minute,
		Label:              "kettle",
	}
	created, err := c.Timers.Create(context.Background(), tm)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.True(t, strings.HasSuffix(gotPath, "/createTimer"))
	assert.Equal(t, "Timer", gotBody["type"])
	assert.Equal(t, "kettle", gotBody["timerLabel"])
	assert.Equal(t, float64(180000), gotBody["originalDurationInMillis"])

	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID)
	assert.Same(t, c.Timers, created.service)
}

func TestTimersService_Create_MissingDeviceReturnsErrDeviceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	tm := &Timer{
		DeviceSerialNumber: "UNKNOWN",
		Duration:           5 * time.Minute,
	}
	_, err := c.Timers.Create(context.Background(), tm)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestTimersService_Delete_RemovesServerSide(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case http.MethodDelete:
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	timers, err := c.Timers.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, timers, 1)
	target := timers[0]

	err = c.Timers.Delete(context.Background(), target)
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.True(t, strings.Contains(gotPath, target.ID), "DELETE path must include id: %s", gotPath)
}

func TestTimersService_Delete_NilItemReturnsSentinel(t *testing.T) {
	c := newSchedulesTestClient(t, httptest.NewServer(http.NotFoundHandler()))
	err := c.Timers.Delete(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTimerNotFound)
}
