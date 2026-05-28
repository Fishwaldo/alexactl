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

func TestRemindersService_Get_ParsesRemindersOnly(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	reminders, err := c.Reminders.Get(context.Background())
	require.NoError(t, err)
	require.Len(t, reminders, 2)

	labels := []string{}
	for _, r := range reminders {
		labels = append(labels, r.Label)
	}
	assert.Contains(t, labels, "Meeting")
	assert.Contains(t, labels, "feed the cat")
}

func TestRemindersService_Get_AcknowledgedMaps(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	reminders, err := c.Reminders.Get(context.Background())
	require.NoError(t, err)

	for _, r := range reminders {
		switch r.Label {
		case "Meeting":
			assert.False(t, r.Acknowledged, "Meeting reminder is UNACKNOWLEDGED")
		case "feed the cat":
			assert.True(t, r.Acknowledged, "feed the cat reminder is ACKNOWLEDGED")
		}
	}
}

func TestRemindersService_Get_SetsBackRef(t *testing.T) {
	body, err := os.ReadFile("testdata/schedules/get_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	reminders, err := c.Reminders.Get(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, reminders)

	for _, r := range reminders {
		assert.Same(t, c.Reminders, r.service)
		assert.NotNil(t, r.serverShape)
	}
}

func TestRemindersService_Get_WrapsTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	_, err := c.Reminders.Get(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotConnect)
}

func TestRemindersService_Create_PutsBodyToCreateReminder(t *testing.T) {
	respBody, err := os.ReadFile("testdata/schedules/create_reminder_response.json")
	require.NoError(t, err)

	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createReminder") {
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

	r := &Reminder{
		DeviceSerialNumber: "GSM427065425023K",
		FireAt:             time.Date(2026, 5, 29, 8, 30, 0, 0, time.UTC),
		Label:              "test reminder",
	}
	created, err := c.Reminders.Create(context.Background(), r)
	require.NoError(t, err)
	require.NotNil(t, created)

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.True(t, strings.HasSuffix(gotPath, "/api/notifications/createReminder"))
	assert.Equal(t, "Reminder", gotBody["type"])
	assert.Equal(t, "GSM427065425023K", gotBody["deviceSerialNumber"])
	assert.Equal(t, "A345JHMQDGG1M5", gotBody["deviceType"])
	assert.Equal(t, "test reminder", gotBody["reminderLabel"])
	assert.Equal(t, "2026-05-29", gotBody["originalDate"])
	assert.Equal(t, "08:30:00.000", gotBody["originalTime"])

	assert.Equal(t, "A345JHMQDGG1M5-GSM427065425023K-new-reminder_REMINDER", created.ID)
	assert.Equal(t, 1, created.Version)
	assert.NotNil(t, created.serverShape)
	assert.Same(t, c.Reminders, created.service)
}

func TestRemindersService_Create_RecurringSendsRRule(t *testing.T) {
	respBody, err := os.ReadFile("testdata/schedules/create_reminder_response.json")
	require.NoError(t, err)

	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createReminder") {
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

	r := &Reminder{
		DeviceSerialNumber: "GSM427065425023K",
		FireAt:             time.Date(2026, 5, 29, 8, 30, 0, 0, time.UTC),
		Label:              "feed cat",
		Recurrence:         RecurrenceDaily(),
	}
	_, err = c.Reminders.Create(context.Background(), r)
	require.NoError(t, err)

	rd, _ := gotBody["rRuleData"].(map[string]any)
	require.NotNil(t, rd)
	rules, _ := rd["recurrenceRules"].([]any)
	require.Len(t, rules, 1)
	assert.Equal(t,
		"FREQ=DAILY;BYHOUR=8;BYMINUTE=30;BYSECOND=0;INTERVAL=1",
		rules[0])
}

func TestRemindersService_Create_MissingDeviceReturnsErrDeviceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	r := &Reminder{
		DeviceSerialNumber: "UNKNOWN-SERIAL",
		FireAt:             time.Date(2026, 5, 29, 8, 30, 0, 0, time.UTC),
		Label:              "x",
	}
	_, err := c.Reminders.Create(context.Background(), r)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestRemindersService_Acknowledge_PatchesFollowUpStatus(t *testing.T) {
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
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	reminders, err := c.Reminders.Get(context.Background())
	require.NoError(t, err)

	var meeting *Reminder
	for _, r := range reminders {
		if r.Label == "Meeting" {
			meeting = r
			break
		}
	}
	require.NotNil(t, meeting)
	require.False(t, meeting.Acknowledged)

	err = c.Reminders.Acknowledge(context.Background(), meeting)
	require.NoError(t, err)

	fum, _ := gotBody["followUpMetadata"].(map[string]any)
	require.NotNil(t, fum)
	assert.Equal(t, "ACKNOWLEDGED", fum["followUpStatus"])
	assert.Equal(t, "Meeting", gotBody["reminderLabel"], "echo body must carry full serverShape")
	assert.True(t, meeting.Acknowledged)
}

func TestRemindersService_Acknowledge_NoOpWhenAlreadyAcknowledged(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/notifications/") &&
			!strings.HasSuffix(r.URL.Path, "/api/notifications") {
			called = true
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	r := &Reminder{
		ID:           "x",
		Acknowledged: true,
		service:      c.Reminders,
	}
	err := c.Reminders.Acknowledge(context.Background(), r)
	require.NoError(t, err)
	assert.False(t, called, "Acknowledge must short-circuit when already ACKNOWLEDGED")
}

func TestRemindersService_Delete_NilItemReturnsSentinel(t *testing.T) {
	c := newSchedulesTestClient(t, httptest.NewServer(http.NotFoundHandler()))
	err := c.Reminders.Delete(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReminderNotFound)
}

func TestRemindersService_Update_PutsEchoBodyWithLabel(t *testing.T) {
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
			out, _ := json.Marshal(gotBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(srv.Close)

	c := newSchedulesTestClient(t, srv)
	reminders, err := c.Reminders.Get(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, reminders)

	target := reminders[0]
	target.Label = "new label"
	err = c.Reminders.Update(context.Background(), target)
	require.NoError(t, err)
	assert.Equal(t, "new label", gotBody["reminderLabel"])
	assert.Equal(t, "new label", target.Label)
	assert.Equal(t, 2, target.Version)
}
