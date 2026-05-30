package alexactl

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// numberAsInt64 coerces a JSON-decoded value (float64, json.Number, or
// numeric-string) into int64. Used by callers consuming numeric fields
// from /api/notifications and related endpoints.
func numberAsInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// schedulesFetchResult is the per-kind split of the
// /api/notifications response. Each slice carries the raw server
// dicts (map[string]any); per-service Get methods then build typed
// *Alarm / *Reminder / *Timer values from these.
type schedulesFetchResult struct {
	Alarms    []map[string]any // includes MusicAlarm (normalised to Alarm by callers)
	Reminders []map[string]any
	Timers    []map[string]any
}

// fetchSchedules issues the single GET to /api/notifications and
// splits the result by wire kind. Shared by AlarmsService.Get,
// RemindersService.Get, TimersService.Get, AND DevicesService.Refresh's
// sensors-enrichment hook — each consumer iterates only the slice
// it cares about.
//
// Returns ErrCannotConnect-wrapped on transport failure,
// *APIError on 4xx/5xx, ErrCannotRetrieveData-wrapped on JSON
// decode failure.
func fetchSchedules(ctx context.Context, wrapper *httpwrapper.Wrapper) (*schedulesFetchResult, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		wrapper.Domain(), amzconst.URISchedules)

	resp, err := wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodGet,
		URL:          url,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentBrowser}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodGet,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	out := &schedulesFetchResult{}
	entries, _ := data["notifications"].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry == nil {
			continue
		}
		kind, _ := entry["type"].(string)
		switch kind {
		case amzconst.ScheduleKindAlarm, amzconst.ScheduleKindMusicAlarm:
			out.Alarms = append(out.Alarms, entry)
		case amzconst.ScheduleKindReminder:
			out.Reminders = append(out.Reminders, entry)
		case amzconst.ScheduleKindTimer:
			out.Timers = append(out.Timers, entry)
		}
	}
	return out, nil
}

// populateDeviceSchedules routes the fetched per-kind raw entries
// into typed Device.Alarms / Reminders / Timers slices on the
// matching device. Status:OFF entries are dropped (only active
// schedules are surfaced on Device). Uses each service's back-ref
// for typed-value back-refs.
func populateDeviceSchedules(
	alarms *AlarmsService,
	reminders *RemindersService,
	timers *TimersService,
	devices map[string]*Device,
	res *schedulesFetchResult,
) {
	// Reset all per-device slices so this is idempotent across Refresh calls.
	for _, d := range devices {
		d.Alarms = nil
		d.Reminders = nil
		d.Timers = nil
	}
	for _, raw := range res.Alarms {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		a := buildAlarm(alarms, raw)
		if a == nil {
			continue
		}
		if d := devices[a.DeviceSerialNumber]; d != nil {
			d.Alarms = append(d.Alarms, a)
		}
	}
	for _, raw := range res.Reminders {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		r := buildReminder(reminders, raw)
		if r == nil {
			continue
		}
		if d := devices[r.DeviceSerialNumber]; d != nil {
			d.Reminders = append(d.Reminders, r)
		}
	}
	for _, raw := range res.Timers {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		t := buildTimer(timers, raw)
		if t == nil {
			continue
		}
		if d := devices[t.DeviceSerialNumber]; d != nil {
			d.Timers = append(d.Timers, t)
		}
	}
}

// AlarmServerShape returns a defensive copy of the raw server dict
// cached on the *Alarm. Exported for CLI debugging only —
// production callers should rely on the typed fields.
func AlarmServerShape(a *Alarm) map[string]any {
	if a == nil || a.serverShape == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(a.serverShape))
	for k, v := range a.serverShape {
		out[k] = v
	}
	return out
}

// ReminderServerShape mirrors AlarmServerShape for *Reminder.
func ReminderServerShape(r *Reminder) map[string]any {
	if r == nil || r.serverShape == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(r.serverShape))
	for k, v := range r.serverShape {
		out[k] = v
	}
	return out
}

// TimerServerShape mirrors AlarmServerShape for *Timer.
func TimerServerShape(t *Timer) map[string]any {
	if t == nil || t.serverShape == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(t.serverShape))
	for k, v := range t.serverShape {
		out[k] = v
	}
	return out
}
