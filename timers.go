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

// TimersService manages countdown timers on Echo devices.
//
// Wire endpoints (cookie + CSRF auth on alexa.amazon.<tld>):
//
//	GET    /api/notifications                       — shared read (filtered to Timer)
//	PUT    /api/notifications/createTimer           — Create
//	DELETE /api/notifications/<id>                  — Delete
//
// No Update (duration is immutable once running — Delete + Create).
// No Pause / Resume (not exposed by the cookie-auth API; use
// Sequence.TextCommand(serial, "pause my timer") instead).
//
// Safe for concurrent use.
type TimersService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	devicesLookup  func(serial string) (*Device, bool)
	schedulesFetch func(ctx context.Context) (*schedulesFetchResult, error)
}

// Timer is a countdown-from-creation event. Always one-shot —
// Amazon doesn't support recurring timers (use *Alarm with
// *Recurrence instead).
type Timer struct {
	ID                 string
	DeviceSerialNumber string
	DeviceType         string
	Duration           time.Duration // total countdown length
	Label              string        // optional ("pasta")
	Remaining          time.Duration // read-only: time left until fire
	Sound              string        // optional
	FireAt             time.Time     // CreatedAt + Duration (= triggerTime)
	Status             string        // "ON"
	Version            int
	CreatedAt          time.Time

	service     *TimersService
	serverShape map[string]any
}

func (s *TimersService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// Get fetches every active timer on the account, sorted by FireAt
// ascending. Entries with status != "ON" are dropped.
func (s *TimersService) Get(ctx context.Context) ([]*Timer, error) {
	res, err := s.schedulesFetch(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Timer, 0, len(res.Timers))
	for _, raw := range res.Timers {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		t := buildTimer(s, raw)
		if t == nil {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].FireAt.Before(out[j].FireAt)
	})
	return out, nil
}

func buildTimer(s *TimersService, raw map[string]any) *Timer {
	id, _ := raw["id"].(string)
	if id == "" {
		return nil
	}
	serial, _ := raw["deviceSerialNumber"].(string)
	devType, _ := raw["deviceType"].(string)
	label, _ := raw["timerLabel"].(string)
	status, _ := raw["status"].(string)
	version := intFromAny(raw["version"])

	durMs := intFromAny(raw["originalDurationInMillis"])
	dur := time.Duration(durMs) * time.Millisecond
	remMs := intFromAny(raw["remainingTime"])
	rem := time.Duration(remMs) * time.Millisecond
	fireAt := alarmTimeFromAny(raw["triggerTime"])
	created := parseListTime(raw["createdDate"])
	sound := extractSoundID(raw["sound"])

	return &Timer{
		ID:                 id,
		DeviceSerialNumber: serial,
		DeviceType:         devType,
		Duration:           dur,
		Label:              label,
		Remaining:          rem,
		Sound:              sound,
		FireAt:             fireAt,
		Status:             status,
		Version:            version,
		CreatedAt:          created,
		service:            s,
		serverShape:        raw,
	}
}

// Create posts a new timer. Timer must have DeviceSerialNumber and
// Duration set; DeviceType is auto-populated from the Devices cache.
// Returns ErrDeviceNotFound (without hitting the network) when the
// serial isn't in the cache.
func (s *TimersService) Create(ctx context.Context, t *Timer) (*Timer, error) {
	dvc, ok := s.lookupDevice(t.DeviceSerialNumber)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, t.DeviceSerialNumber)
	}
	t.DeviceType = dvc.DeviceType

	body := buildTimerCreateBody(t)
	raw, err := s.put(ctx, "/api/notifications/createTimer", body, http.MethodPut)
	if err != nil {
		return nil, err
	}
	created := buildTimer(s, raw)
	if created == nil {
		return nil, fmt.Errorf("%w: createTimer response missing id", ErrCannotRetrieveData)
	}
	dvc.Timers = append(dvc.Timers, created)
	return created, nil
}

// Delete cancels the timer. Returns ErrTimerNotFound if t is nil or
// owned by a different service.
func (s *TimersService) Delete(ctx context.Context, t *Timer) error {
	if t == nil || t.service != s {
		id := ""
		if t != nil {
			id = t.ID
		}
		return fmt.Errorf("%w: %q", ErrTimerNotFound, id)
	}
	url := fmt.Sprintf("https://alexa.amazon.%s/api/notifications/%s",
		s.wrapper.Domain(), t.ID)
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
	if dvc, ok := s.lookupDevice(t.DeviceSerialNumber); ok {
		for i, x := range dvc.Timers {
			if x == t {
				dvc.Timers = append(dvc.Timers[:i], dvc.Timers[i+1:]...)
				break
			}
		}
	}
	return nil
}

func (s *TimersService) put(ctx context.Context, path string, body any, method string) (map[string]any, error) {
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

func (s *TimersService) lookupDevice(serial string) (*Device, bool) {
	if s.devicesLookup == nil {
		return nil, false
	}
	return s.devicesLookup(serial)
}

func buildTimerCreateBody(t *Timer) map[string]any {
	durMs := t.Duration.Milliseconds()
	trigger := time.Now().Add(t.Duration).UnixMilli()
	body := map[string]any{
		"type":                     "Timer",
		"status":                   "ON",
		"triggerTime":              trigger,
		"originalDurationInMillis": durMs,
		"deviceSerialNumber":       t.DeviceSerialNumber,
		"deviceType":               t.DeviceType,
		"timerLabel":               t.Label,
	}
	if t.Sound != "" {
		body["sound"] = map[string]any{"id": t.Sound, "providerId": "ECHO"}
	}
	return body
}
