package alexactl

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

func TestAlarmsService_Get_ParsesAlarmsOnly(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	alarms, err := c.Alarms.Get(context.Background())
	require.NoError(t, err)

	// 4 raw Alarm-ish entries: 1 one-shot + 1 recurring + 1 OFF + 1 MusicAlarm
	// (normalised). Get filters status:OFF → 3 returned.
	require.Len(t, alarms, 3)

	// Find each by Label
	labels := []string{}
	for _, a := range alarms {
		labels = append(labels, a.Label)
	}
	assert.Contains(t, labels, "wake up")
	assert.Contains(t, labels, "morning workout")
	assert.Contains(t, labels, "Spotify alarm") // MusicAlarm normalised
}

func TestAlarmsService_Get_ParsesRecurringWithRRule(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	alarms, err := c.Alarms.Get(context.Background())
	require.NoError(t, err)

	var workout *Alarm
	for _, a := range alarms {
		if a.Label == "morning workout" {
			workout = a
			break
		}
	}
	require.NotNil(t, workout)
	require.NotNil(t, workout.Recurrence)
	assert.Equal(t,
		"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=30;BYSECOND=0;INTERVAL=1;",
		workout.Recurrence.RRule(),
		"Recurrence.RRule must equal the wire's recurrenceRules[0] verbatim")
}

func TestAlarmsService_Get_SetsBackRef(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	alarms, err := c.Alarms.Get(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, alarms)

	for _, a := range alarms {
		assert.Same(t, c.Alarms, a.service, "alarm %q missing back-ref", a.ID)
		assert.NotNil(t, a.serverShape, "alarm %q missing serverShape", a.ID)
	}
}

func TestAlarmsService_Get_WrapsTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	_, err := c.Alarms.Get(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotConnect)
}

func TestAlarmsService_Get_WrapsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	_, err := c.Alarms.Get(context.Background())
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func TestAlarmsService_Create_PutsBodyToCreateAlarm(t *testing.T) {
	respBody, err := os.ReadFile("testdata/schedules/create_alarm_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createAlarm") {
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
		"G000RA0991330DAS": {SerialNumber: "G000RA0991330DAS", DeviceType: "AWZZ5CVHX2CD"},
	})

	a := &Alarm{
		DeviceSerialNumber: "G000RA0991330DAS",
		FireAt:             time.Date(2026, 5, 29, 7, 30, 0, 0, time.UTC),
		Label:              "test alarm",
	}
	created, err := c.Alarms.Create(context.Background(), a)
	require.NoError(t, err)
	require.NotNil(t, created)

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.True(t, strings.HasSuffix(gotPath, "/api/notifications/createAlarm"))
	assert.Equal(t, "Alarm", gotBody["type"])
	assert.Equal(t, "G000RA0991330DAS", gotBody["deviceSerialNumber"])
	assert.Equal(t, "AWZZ5CVHX2CD", gotBody["deviceType"])
	assert.Equal(t, "test alarm", gotBody["alarmLabel"])
	assert.Equal(t, "2026-05-29", gotBody["originalDate"])
	assert.Equal(t, "07:30:00.000", gotBody["originalTime"])

	assert.Equal(t, "AWZZ5CVHX2CD-G000RA0991330DAS-new-alarm", created.ID)
	assert.Equal(t, 1, created.Version)
	assert.NotNil(t, created.serverShape)
	assert.Same(t, c.Alarms, created.service)
}

func TestAlarmsService_Create_RecurringSendsRRule(t *testing.T) {
	respBody, err := os.ReadFile("testdata/schedules/create_alarm_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createAlarm") {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(respBody)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	c.Devices.setCacheForTest(map[string]*Device{
		"G000RA0991330DAS": {SerialNumber: "G000RA0991330DAS", DeviceType: "AWZZ5CVHX2CD"},
	})

	a := &Alarm{
		DeviceSerialNumber: "G000RA0991330DAS",
		FireAt:             time.Date(2026, 5, 29, 7, 30, 0, 0, time.UTC),
		Label:              "weekday wake",
		Recurrence:         RecurrenceWeekdays(),
	}
	_, err = c.Alarms.Create(context.Background(), a)
	require.NoError(t, err)

	rd, _ := gotBody["rRuleData"].(map[string]any)
	require.NotNil(t, rd)
	rules, _ := rd["recurrenceRules"].([]any)
	require.Len(t, rules, 1)
	assert.Equal(t,
		"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=7;BYMINUTE=30;BYSECOND=0;INTERVAL=1",
		rules[0])
}

func TestAlarmsService_Create_MissingDeviceReturnsErrDeviceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	a := &Alarm{
		DeviceSerialNumber: "UNKNOWN-SERIAL",
		FireAt:             time.Date(2026, 5, 29, 7, 30, 0, 0, time.UTC),
	}
	_, err := c.Alarms.Create(context.Background(), a)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestAlarmsService_Disable_PutsEchoBody(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/notifications"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		default:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			gotBody["version"] = float64(2)
			gotBody["lastUpdatedDate"] = float64(1779990000000)
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	alarms, err := c.Alarms.Get(context.Background())
	require.NoError(t, err)
	var workout *Alarm
	for _, a := range alarms {
		if a.Label == "morning workout" {
			workout = a
			break
		}
	}
	require.NotNil(t, workout)

	err = c.Alarms.Disable(context.Background(), workout)
	require.NoError(t, err)
	assert.Equal(t, "OFF", gotBody["status"])
	assert.Equal(t, "morning workout", gotBody["alarmLabel"], "echo body must carry full serverShape")
	assert.Equal(t, "OFF", workout.Status)
	assert.Equal(t, 2, workout.Version)
}

func TestAlarmsService_Delete_NilItemReturnsSentinel(t *testing.T) {
	c := newSchedulesTestClient(t, httptest.NewServer(http.NotFoundHandler()))
	err := c.Alarms.Delete(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlarmNotFound)
}

func TestAlarmsService_Snooze_BumpsSnoozedToTime(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/notifications"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		default:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			gotBody["version"] = float64(3)
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	alarms, err := c.Alarms.Get(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, alarms)

	target := alarms[0]
	err = c.Alarms.Snooze(context.Background(), target, 10*time.Minute)
	require.NoError(t, err)
	snoozed, ok := gotBody["snoozedToTime"].(float64)
	require.True(t, ok)
	assert.Greater(t, snoozed, gotBody["alarmTime"].(float64))
}
