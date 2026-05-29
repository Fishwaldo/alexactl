package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// AlarmsService manages clock-time alarms on Echo devices.
//
// Wire endpoints (cookie + CSRF auth on alexa.amazon.<tld>):
//
//	GET    /api/notifications                       — shared read (filtered to Alarm + MusicAlarm)
//	PUT    /api/notifications/createAlarm           — Create
//	PUT    /api/notifications/<id>                  — Update / Enable / Disable / Snooze
//	DELETE /api/notifications/<id>                  — Delete
//
// MusicAlarm wire entries are normalised to typed *Alarm at parse
// time (same shape, different label). The label-field fallback is
// musicAlarmLabel when alarmLabel is empty.
//
// Safe for concurrent use.
type AlarmsService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	// devicesLookup is injected by NewClient so AlarmsService.Create
	// can resolve DeviceType from a SerialNumber. Returns (nil, false)
	// when the cache doesn't contain the serial.
	devicesLookup func(serial string) (*Device, bool)

	// schedulesFetch is injected by NewClient; routes Get() through the
	// shared schedules.notifications cache so Alarms/Reminders/Timers
	// share one fetch.
	schedulesFetch func(ctx context.Context) (*schedulesFetchResult, error)
}

// Alarm is a clock-time wake event tied to a specific device.
type Alarm struct {
	ID                 string
	DeviceSerialNumber string
	DeviceType         string
	FireAt             time.Time
	Label              string
	Sound              string
	Recurrence         *Recurrence
	Status             string // "ON" | "OFF" | "PAUSED"
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time

	service     *AlarmsService
	serverShape map[string]any
}

func (s *AlarmsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// Get fetches every active alarm on the account, sorted by FireAt
// ascending. Entries with status != "ON" are dropped.
func (s *AlarmsService) Get(ctx context.Context) ([]*Alarm, error) {
	res, err := s.schedulesFetch(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Alarm, 0, len(res.Alarms))
	for _, raw := range res.Alarms {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		a := buildAlarm(s, raw)
		if a == nil {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].FireAt.Before(out[j].FireAt)
	})
	return out, nil
}

// buildAlarm constructs a typed *Alarm from a single notifications
// entry. Returns nil for entries missing required fields.
func buildAlarm(s *AlarmsService, raw map[string]any) *Alarm {
	id, _ := raw["id"].(string)
	if id == "" {
		return nil
	}
	serial, _ := raw["deviceSerialNumber"].(string)
	devType, _ := raw["deviceType"].(string)
	kind, _ := raw["type"].(string)

	label, _ := raw["alarmLabel"].(string)
	if label == "" && kind == amzconst.ScheduleKindMusicAlarm {
		label, _ = raw["musicAlarmLabel"].(string)
	}

	status, _ := raw["status"].(string)
	version := intFromAny(raw["version"])
	fireAt := alarmTimeFromAny(raw["alarmTime"])
	created := parseListTime(raw["createdDate"])
	updated := parseListTime(raw["lastUpdatedDate"])

	sound := extractSoundID(raw["sound"])
	recur := extractRecurrence(raw["rRuleData"])

	return &Alarm{
		ID:                 id,
		DeviceSerialNumber: serial,
		DeviceType:         devType,
		FireAt:             fireAt,
		Label:              label,
		Sound:              sound,
		Recurrence:         recur,
		Status:             status,
		Version:            version,
		CreatedAt:          created,
		UpdatedAt:          updated,
		service:            s,
		serverShape:        raw,
	}
}

// alarmTimeFromAny coerces the wire's alarmTime / triggerTime field
// (numeric milliseconds since epoch) to a time.Time.
func alarmTimeFromAny(v any) time.Time {
	switch x := v.(type) {
	case float64:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(int64(x))
	case int64:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(x)
	case int:
		if x <= 0 {
			return time.Time{}
		}
		return time.UnixMilli(int64(x))
	}
	return time.Time{}
}

// extractSoundID pulls the "id" out of the sound sub-object. Returns
// empty string if sound is nil or has no id (server picks default).
func extractSoundID(v any) string {
	m, _ := v.(map[string]any)
	if m == nil {
		return ""
	}
	id, _ := m["id"].(string)
	return id
}

// extractRecurrence builds a *Recurrence from the wire's rRuleData
// sub-object. Uses recurrenceRules[0] if populated, otherwise nil.
// Discards the denormalised typed fields (byWeekDays, intervals, etc.)
// per Path A in the spec.
func extractRecurrence(v any) *Recurrence {
	rd, _ := v.(map[string]any)
	if rd == nil {
		return nil
	}
	rules, _ := rd["recurrenceRules"].([]any)
	if len(rules) == 0 {
		return nil
	}
	s, _ := rules[0].(string)
	if s == "" {
		return nil
	}
	return &Recurrence{rrule: s}
}

// Create posts a new alarm. The Alarm must have DeviceSerialNumber
// and FireAt set; DeviceType is auto-populated from the Devices
// cache. Returns *Alarm with server-assigned ID + Version + serverShape.
// On success the alarm is appended to the matching cached
// *Device.Alarms slice (if the device is in cache).
//
// Returns ErrDeviceNotFound (without hitting the network) when the
// serial isn't in the cache.
func (s *AlarmsService) Create(ctx context.Context, a *Alarm) (*Alarm, error) {
	dvc, ok := s.lookupDevice(a.DeviceSerialNumber)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, a.DeviceSerialNumber)
	}
	a.DeviceType = dvc.DeviceType

	body := buildAlarmCreateBody(a)
	raw, err := s.put(ctx, "/api/notifications/createAlarm", body, http.MethodPut)
	if err != nil {
		return nil, err
	}
	created := buildAlarm(s, raw)
	if created == nil {
		return nil, fmt.Errorf("%w: createAlarm response missing id", ErrCannotRetrieveData)
	}
	dvc.Alarms = append(dvc.Alarms, created)
	return created, nil
}

// Update sends the cached serverShape echo with mutated user-set
// fields. On success the *Alarm is rewritten in place; on error,
// untouched.
func (s *AlarmsService) Update(ctx context.Context, a *Alarm) error {
	if a == nil || a.service != s {
		id := ""
		if a != nil {
			id = a.ID
		}
		return fmt.Errorf("%w: %q", ErrAlarmNotFound, id)
	}
	overlay := map[string]any{
		"alarmLabel":   a.Label,
		"alarmTime":    a.FireAt.UnixMilli(),
		"originalDate": a.FireAt.Format("2006-01-02"),
		"originalTime": a.FireAt.Format("15:04:05.000"),
	}
	if a.Sound != "" {
		overlay["sound"] = map[string]any{"id": a.Sound, "providerId": "ECHO"}
	}
	if a.Recurrence != nil {
		overlay["rRuleData"] = map[string]any{
			"recurrenceRules": []string{a.Recurrence.buildFullRRule(a.FireAt)},
		}
	}
	return s.updateItem(ctx, a, overlay)
}

// Delete removes the alarm both server-side and from the matching
// cached *Device.Alarms slice. Returns ErrAlarmNotFound for nil or
// wrong-parent items.
func (s *AlarmsService) Delete(ctx context.Context, a *Alarm) error {
	if a == nil || a.service != s {
		id := ""
		if a != nil {
			id = a.ID
		}
		return fmt.Errorf("%w: %q", ErrAlarmNotFound, id)
	}
	url := fmt.Sprintf("https://alexa.amazon.%s/api/notifications/%s",
		s.wrapper.Domain(), a.ID)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodDelete,
		URL:          url,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentBrowser}},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodDelete,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	if dvc, ok := s.lookupDevice(a.DeviceSerialNumber); ok {
		for i, x := range dvc.Alarms {
			if x == a {
				dvc.Alarms = append(dvc.Alarms[:i], dvc.Alarms[i+1:]...)
				break
			}
		}
	}
	return nil
}

// Enable flips status to ON. No-op + nil if already ON.
func (s *AlarmsService) Enable(ctx context.Context, a *Alarm) error {
	if a == nil || a.service != s {
		id := ""
		if a != nil {
			id = a.ID
		}
		return fmt.Errorf("%w: %q", ErrAlarmNotFound, id)
	}
	if a.Status == "ON" {
		return nil
	}
	return s.updateItem(ctx, a, map[string]any{"status": "ON"})
}

// Disable flips status to OFF. No-op + nil if already OFF.
func (s *AlarmsService) Disable(ctx context.Context, a *Alarm) error {
	if a == nil || a.service != s {
		id := ""
		if a != nil {
			id = a.ID
		}
		return fmt.Errorf("%w: %q", ErrAlarmNotFound, id)
	}
	if a.Status == "OFF" {
		return nil
	}
	return s.updateItem(ctx, a, map[string]any{"status": "OFF"})
}

// Snooze pushes the next fire by dur. Maps to the snoozedToTime
// field.
func (s *AlarmsService) Snooze(ctx context.Context, a *Alarm, dur time.Duration) error {
	if a == nil || a.service != s {
		id := ""
		if a != nil {
			id = a.ID
		}
		return fmt.Errorf("%w: %q", ErrAlarmNotFound, id)
	}
	snoozedTo := time.Now().Add(dur).UnixMilli()
	return s.updateItem(ctx, a, map[string]any{"snoozedToTime": snoozedTo})
}

// updateItem builds a PUT body from item.serverShape + overrides,
// sends to /api/notifications/<id>, applies the response back to
// the in-memory *Alarm.
func (s *AlarmsService) updateItem(ctx context.Context, a *Alarm, overrides map[string]any) error {
	body := make(map[string]any, len(a.serverShape))
	for k, v := range a.serverShape {
		body[k] = v
	}
	for k, v := range overrides {
		body[k] = v
	}
	raw, err := s.put(ctx, "/api/notifications/"+a.ID, body, http.MethodPut)
	if err != nil {
		return err
	}
	applyAlarmResponse(a, raw)
	return nil
}

// put issues a request with the given body to the path on
// alexa.amazon.<tld> and returns the decoded JSON response.
func (s *AlarmsService) put(ctx context.Context, path string, body any, method string) (map[string]any, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s", s.wrapper.Domain(), path)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       method,
		URL:          url,
		JSONBody:     body,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentBrowser}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		respBody := resp.Body
		defer respBody.Close()
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     method,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	raw, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}
	return raw, nil
}

func (s *AlarmsService) lookupDevice(serial string) (*Device, bool) {
	if s.devicesLookup == nil {
		return nil, false
	}
	return s.devicesLookup(serial)
}

func buildAlarmCreateBody(a *Alarm) map[string]any {
	body := map[string]any{
		"type":               "Alarm",
		"status":             "ON",
		"alarmTime":          a.FireAt.UnixMilli(),
		"originalDate":       a.FireAt.Format("2006-01-02"),
		"originalTime":       a.FireAt.Format("15:04:05.000"),
		"timeZoneId":         nil,
		"deviceSerialNumber": a.DeviceSerialNumber,
		"deviceType":         a.DeviceType,
		"alarmLabel":         a.Label,
	}
	if a.Sound != "" {
		body["sound"] = map[string]any{"id": a.Sound, "providerId": "ECHO"}
	}
	if a.Recurrence != nil {
		body["rRuleData"] = map[string]any{
			"recurrenceRules": []string{a.Recurrence.buildFullRRule(a.FireAt)},
		}
	}
	return body
}

func applyAlarmResponse(a *Alarm, raw map[string]any) {
	if v, ok := raw["status"].(string); ok {
		a.Status = v
	}
	if v := intFromAny(raw["version"]); v != 0 {
		a.Version = v
	}
	if t := parseListTime(raw["lastUpdatedDate"]); !t.IsZero() {
		a.UpdatedAt = t
	}
	if t := alarmTimeFromAny(raw["alarmTime"]); !t.IsZero() {
		a.FireAt = t
	}
	if v, ok := raw["alarmLabel"].(string); ok && v != "" {
		a.Label = v
	}
	a.serverShape = raw
}
