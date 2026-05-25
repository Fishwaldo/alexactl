package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/teambition/rrule-go"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// NotificationsService fetches alarms, timers, and reminders set on the
// account's Echo devices, expanding recurring rules into the next
// concrete occurrence per (device, type) bucket.
//
// Backed by GET /api/notifications on alexa.amazon.<domain>. Safe for
// concurrent use.
type NotificationsService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	// Test seam. Production leaves nil and reads from time.Now.
	now func() time.Time
}

// setSession is called by AuthService.broadcast on Login/Restore.
func (s *NotificationsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// nowFn returns the current time, honoring the test seam if set.
func (s *NotificationsService) nowFn() time.Time {
	s.mu.RLock()
	fn := s.now
	s.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now()
}

// Get fetches every alarm, timer, and reminder on the account and
// returns them keyed by [device serial][notification type] = *Schedule.
// The inner map's keys are NotificationAlarm / NotificationReminder /
// NotificationTimer; MusicAlarm is normalized to Alarm at parse time.
//
// Within each (serial, type) bucket the earliest future occurrence wins.
// Schedules whose next occurrence is unknown (unparseable rule, all in
// the past) are silently dropped. Schedules with status != "ON" are
// silently dropped.
func (s *NotificationsService) Get(ctx context.Context) (map[string]map[string]*Schedule, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URINotifications)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodGet,
		URL:          url,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentBrowser}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		body := resp.Body
		defer body.Close()
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

	out := map[string]map[string]*Schedule{}
	entries, _ := data["notifications"].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry == nil {
			continue
		}

		schedType, _ := entry["type"].(string)
		if _, ok := amzconst.NotificationsSupported[schedType]; !ok {
			s.wrapper.Logger(ctx).DebugContext(ctx,
				"amazondevices: notifications: unsupported schedule type",
				"type", schedType)
			continue
		}

		devType, _ := entry["deviceType"].(string)
		if _, ignore := amzconst.DeviceTypesToIgnore[devType]; ignore {
			continue
		}

		serial, _ := entry["deviceSerialNumber"].(string)
		if serial == "" {
			continue
		}

		status, _ := entry["status"].(string)
		if status != "ON" {
			continue
		}

		// Normalize MusicAlarm -> Alarm (same shape, different label).
		if schedType == amzconst.NotificationMusicAlarm {
			schedType = amzconst.NotificationAlarm
		}

		next, ok := s.parseNextOccurrence(ctx, entry, schedType)
		if !ok {
			continue
		}

		label, _ := entry[labelKey(schedType)].(string)
		if label == "" && schedType == amzconst.NotificationAlarm {
			label, _ = entry["musicAlarmLabel"].(string)
		}

		sched := &Schedule{
			Type:           schedType,
			Status:         status,
			Label:          label,
			NextOccurrence: next,
		}
		bucket, ok := out[serial]
		if !ok {
			bucket = map[string]*Schedule{}
			out[serial] = bucket
		}
		existing, present := bucket[schedType]
		if !present || existing.NextOccurrence.IsZero() ||
			(!next.IsZero() && next.Before(existing.NextOccurrence)) {
			bucket[schedType] = sched
		}
	}
	return out, nil
}

// labelKey returns the JSON key carrying the user-visible label for a
// schedule type ("Alarm" -> "alarmLabel", etc.). The caller layers a
// "musicAlarmLabel" fallback for entries whose wire type was MusicAlarm
// (which we normalize to Alarm before reaching this helper).
func labelKey(normalizedType string) string {
	return lowerFirst(normalizedType) + "Label"
}

// lowerFirst lowercases just the first rune. Used by labelKey.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]|0x20) + s[1:]
}

// parseNextOccurrence returns the next occurrence >= (now-1m) for the
// given schedule entry. ok=false signals "no usable occurrence" (the
// caller drops the entry).
func (s *NotificationsService) parseNextOccurrence(
	ctx context.Context,
	entry map[string]any,
	schedType string,
) (time.Time, bool) {
	loc := time.Local
	now := s.nowFn().In(loc)
	nowRef := now.Add(-time.Minute)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, loc)
	refStart := startOfDay

	originalDate, _ := entry["originalDate"].(string)
	originalTime, _ := entry["originalTime"].(string)

	if originalDate != "" {
		if d, err := time.ParseInLocation("2006-01-02", originalDate, loc); err == nil {
			refStart = d
		}
	}

	var recurringRules []string
	if rrd, ok := entry["rRuleData"].(map[string]any); ok {
		if rules, ok := rrd["recurrenceRules"].([]any); ok {
			for _, r := range rules {
				if rs, ok := r.(string); ok {
					recurringRules = append(recurringRules, rs)
				}
			}
		}
	}
	if rp, ok := entry["recurringPattern"].(string); ok && rp != "" {
		recurringRules = append(recurringRules, rp)
	}

	if len(recurringRules) > 0 {
		patterns := s.effectivePatterns()
		var candidates []time.Time
		for _, rr := range recurringRules {
			rule := rr
			if !strings.Contains(rule, "FREQ=") {
				expanded, ok := patterns[rule]
				if !ok {
					s.wrapper.Logger(ctx).WarnContext(ctx,
						"amazondevices: notifications: unknown recurring rule",
						"rule", rr, "type", schedType)
					continue
				}
				rule = expanded
			}
			rule = addHoursMinutes(rule, originalTime)
			r, err := rrule.StrToRRule(rule)
			if err != nil {
				s.wrapper.Logger(ctx).WarnContext(ctx,
					"amazondevices: notifications: invalid rrule",
					"rule", rule, "err", err)
				continue
			}
			r.DTStart(refStart)
			next := r.After(nowRef, true)
			if !next.IsZero() {
				candidates = append(candidates, next)
			}
		}
		if len(candidates) == 0 {
			return time.Time{}, false
		}
		earliest := candidates[0]
		for _, c := range candidates[1:] {
			if c.Before(earliest) {
				earliest = c
			}
		}
		return earliest, true
	}

	// Single-event paths.
	switch schedType {
	case amzconst.NotificationAlarm:
		if originalDate == "" || originalTime == "" {
			return time.Time{}, false
		}
		// Trim a fractional-second suffix if present (Amazon sends
		// HH:MM:SS.mmm).
		hms := originalTime
		if i := strings.IndexByte(hms, '.'); i >= 0 {
			hms = hms[:i]
		}
		t, err := time.ParseInLocation("2006-01-02 15:04:05",
			originalDate+" "+hms, loc)
		if err != nil {
			return time.Time{}, false
		}
		if t.After(nowRef) {
			return t, true
		}
		return time.Time{}, false

	case amzconst.NotificationTimer:
		ms, ok := numberAsInt64(entry["triggerTime"])
		if !ok {
			return time.Time{}, false
		}
		t := time.UnixMilli(ms).In(loc)
		if t.After(nowRef) {
			return t, true
		}
		return time.Time{}, false

	case amzconst.NotificationReminder:
		ms, ok := numberAsInt64(entry["alarmTime"])
		if !ok {
			return time.Time{}, false
		}
		t := time.UnixMilli(ms).In(loc)
		if t.After(nowRef) {
			return t, true
		}
		return time.Time{}, false

	default:
		return time.Time{}, false
	}
}

// effectivePatterns returns a copy of amzconst.RecurringPatterns with
// country-group weekend exceptions overlaid. Lookup uses the wrapper's
// current CountryCode().
func (s *NotificationsService) effectivePatterns() map[string]string {
	out := make(map[string]string, len(amzconst.RecurringPatterns))
	for k, v := range amzconst.RecurringPatterns {
		out[k] = v
	}
	cc := strings.ToLower(s.wrapper.CountryCode())
	if cc == "" {
		return out
	}
	for group, countries := range amzconst.CountryGroups {
		for _, c := range countries {
			if c == cc {
				for k, v := range amzconst.WeekendExceptions[group] {
					out[k] = v
				}
				return out
			}
		}
	}
	return out
}

// addHoursMinutes appends BYHOUR/BYMINUTE to an RFC 5545 rule string
// using the HH:MM[:SS[.fff]] originalTime, unless the rule already has
// them. Trailing semicolons are trimmed. Returns the rule unchanged if
// originalTime is empty.
func addHoursMinutes(rule, originalTime string) string {
	rule = strings.TrimSuffix(rule, ";")
	if originalTime == "" {
		return rule
	}
	parts := strings.Split(originalTime, ":")
	if len(parts) < 2 {
		return rule
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return rule
	}
	minute, err := strconv.Atoi(strings.SplitN(parts[1], ".", 2)[0])
	if err != nil {
		return rule
	}
	if !strings.Contains(rule, "BYHOUR=") {
		rule += fmt.Sprintf(";BYHOUR=%d", hour)
	}
	if !strings.Contains(rule, "BYMINUTE=") {
		rule += fmt.Sprintf(";BYMINUTE=%d", minute)
	}
	return rule
}

// numberAsInt64 extracts an int64 from an `any` that may carry a
// json.Number-equivalent float64, int, or numeric string. Returns
// (0, false) for unparseable input.
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
