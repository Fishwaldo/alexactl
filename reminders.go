// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// RemindersService manages reminders (typed text labels that Alexa
// speaks at the scheduled time) on Echo devices.
//
// Wire endpoints (cookie + CSRF auth on alexa.amazon.<tld>):
//
//	GET    /api/notifications                       — shared read (filtered to Reminder)
//	PUT    /api/notifications/createReminder        — Create
//	PUT    /api/notifications/<id>                  — Update / Enable / Disable / Acknowledge
//	DELETE /api/notifications/<id>                  — Delete
//
// Safe for concurrent use.
type RemindersService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	devicesLookup  func(serial string) (*Device, bool)
	schedulesFetch func(ctx context.Context) (*schedulesFetchResult, error)
}

// Reminder is a labeled one-shot or recurring event Alexa speaks.
type Reminder struct {
	ID                 string
	DeviceSerialNumber string
	DeviceType         string
	FireAt             time.Time
	Label              string // required — what Alexa speaks
	Recurrence         *Recurrence
	Status             string
	Acknowledged       bool
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time

	service     *RemindersService
	serverShape map[string]any
}

func (s *RemindersService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// Get fetches every active reminder on the account, sorted by FireAt
// ascending. Entries with status != "ON" are dropped.
func (s *RemindersService) Get(ctx context.Context) ([]*Reminder, error) {
	res, err := s.schedulesFetch(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Reminder, 0, len(res.Reminders))
	for _, raw := range res.Reminders {
		status, _ := raw["status"].(string)
		if status != "ON" {
			continue
		}
		r := buildReminder(s, raw)
		if r == nil {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].FireAt.Before(out[j].FireAt)
	})
	return out, nil
}

func buildReminder(s *RemindersService, raw map[string]any) *Reminder {
	id, _ := raw["id"].(string)
	if id == "" {
		return nil
	}
	serial, _ := raw["deviceSerialNumber"].(string)
	devType, _ := raw["deviceType"].(string)
	label, _ := raw["reminderLabel"].(string)
	status, _ := raw["status"].(string)
	version := intFromAny(raw["version"])
	fireAt := alarmTimeFromAny(raw["alarmTime"])
	created := parseListTime(raw["createdDate"])
	updated := parseListTime(raw["lastUpdatedDate"])
	recur := extractRecurrence(raw["rRuleData"])
	ack := extractAcknowledged(raw["followUpMetadata"])

	return &Reminder{
		ID:                 id,
		DeviceSerialNumber: serial,
		DeviceType:         devType,
		FireAt:             fireAt,
		Label:              label,
		Recurrence:         recur,
		Status:             status,
		Acknowledged:       ack,
		Version:            version,
		CreatedAt:          created,
		UpdatedAt:          updated,
		service:            s,
		serverShape:        raw,
	}
}

// extractAcknowledged pulls followUpStatus out of the
// followUpMetadata sub-object. Returns true when status is exactly
// "ACKNOWLEDGED"; any other value (including missing) is false.
func extractAcknowledged(v any) bool {
	m, _ := v.(map[string]any)
	if m == nil {
		return false
	}
	s, _ := m["followUpStatus"].(string)
	return s == "ACKNOWLEDGED"
}

// Create posts a new reminder.
func (s *RemindersService) Create(ctx context.Context, r *Reminder) (*Reminder, error) {
	dvc, ok := s.lookupDevice(r.DeviceSerialNumber)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrDeviceNotFound, r.DeviceSerialNumber)
	}
	r.DeviceType = dvc.DeviceType
	body := buildReminderCreateBody(r)
	raw, err := s.put(ctx, "/api/notifications/createReminder", body, http.MethodPut)
	if err != nil {
		return nil, err
	}
	created := buildReminder(s, raw)
	if created == nil {
		return nil, fmt.Errorf("%w: createReminder response missing id", ErrCannotRetrieveData)
	}
	dvc.Reminders = append(dvc.Reminders, created)
	return created, nil
}

// Update sends the reminder's typed label/time/recurrence to the
// server, layered over the cached serverShape echo. Returns
// ErrReminderNotFound if r is nil or owned by a different service.
func (s *RemindersService) Update(ctx context.Context, r *Reminder) error {
	if r == nil || r.service != s {
		id := ""
		if r != nil {
			id = r.ID
		}
		return fmt.Errorf("%w: %q", ErrReminderNotFound, id)
	}
	overlay := map[string]any{
		"reminderLabel": r.Label,
		"alarmTime":     r.FireAt.UnixMilli(),
		"originalDate":  r.FireAt.Format("2006-01-02"),
		"originalTime":  r.FireAt.Format("15:04:05.000"),
	}
	if r.Recurrence != nil {
		overlay["rRuleData"] = map[string]any{
			"recurrenceRules": []string{r.Recurrence.buildFullRRule(r.FireAt)},
		}
	}
	return s.updateItem(ctx, r, overlay)
}

// Delete removes the reminder from the account. Returns
// ErrReminderNotFound if r is nil or owned by a different service.
func (s *RemindersService) Delete(ctx context.Context, r *Reminder) error {
	if r == nil || r.service != s {
		id := ""
		if r != nil {
			id = r.ID
		}
		return fmt.Errorf("%w: %q", ErrReminderNotFound, id)
	}
	url := fmt.Sprintf("https://alexa.amazon.%s/api/notifications/%s",
		s.wrapper.Domain(), r.ID)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodDelete,
		URL:          url,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentBrowser}},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodDelete,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	if dvc, ok := s.lookupDevice(r.DeviceSerialNumber); ok {
		for i, x := range dvc.Reminders {
			if x == r {
				dvc.Reminders = append(dvc.Reminders[:i], dvc.Reminders[i+1:]...)
				break
			}
		}
	}
	return nil
}

// Enable flips status to ON. No-op + nil if already ON.
func (s *RemindersService) Enable(ctx context.Context, r *Reminder) error {
	if r == nil || r.service != s {
		id := ""
		if r != nil {
			id = r.ID
		}
		return fmt.Errorf("%w: %q", ErrReminderNotFound, id)
	}
	if r.Status == "ON" {
		return nil
	}
	return s.updateItem(ctx, r, map[string]any{"status": "ON"})
}

// Disable flips status to OFF. No-op + nil if already OFF.
func (s *RemindersService) Disable(ctx context.Context, r *Reminder) error {
	if r == nil || r.service != s {
		id := ""
		if r != nil {
			id = r.ID
		}
		return fmt.Errorf("%w: %q", ErrReminderNotFound, id)
	}
	if r.Status == "OFF" {
		return nil
	}
	return s.updateItem(ctx, r, map[string]any{"status": "OFF"})
}

// Acknowledge flips followUpMetadata.followUpStatus to ACKNOWLEDGED.
// No-op + nil if already ACKNOWLEDGED.
func (s *RemindersService) Acknowledge(ctx context.Context, r *Reminder) error {
	if r == nil || r.service != s {
		id := ""
		if r != nil {
			id = r.ID
		}
		return fmt.Errorf("%w: %q", ErrReminderNotFound, id)
	}
	if r.Acknowledged {
		return nil
	}
	overlay := map[string]any{
		"followUpMetadata": map[string]any{
			"followUpStatus":   "ACKNOWLEDGED",
			"requiresFollowUp": false,
		},
	}
	if err := s.updateItem(ctx, r, overlay); err != nil {
		return err
	}
	r.Acknowledged = true
	return nil
}

func (s *RemindersService) updateItem(ctx context.Context, r *Reminder, overrides map[string]any) error {
	body := make(map[string]any, len(r.serverShape))
	for k, v := range r.serverShape {
		body[k] = v
	}
	for k, v := range overrides {
		body[k] = v
	}
	raw, err := s.put(ctx, "/api/notifications/"+r.ID, body, http.MethodPut)
	if err != nil {
		return err
	}
	applyReminderResponse(r, raw)
	return nil
}

func (s *RemindersService) put(ctx context.Context, path string, body any, method string) (map[string]any, error) {
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
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

func (s *RemindersService) lookupDevice(serial string) (*Device, bool) {
	if s.devicesLookup == nil {
		return nil, false
	}
	return s.devicesLookup(serial)
}

func buildReminderCreateBody(r *Reminder) map[string]any {
	body := map[string]any{
		"type":               "Reminder",
		"status":             "ON",
		"alarmTime":          r.FireAt.UnixMilli(),
		"originalDate":       r.FireAt.Format("2006-01-02"),
		"originalTime":       r.FireAt.Format("15:04:05.000"),
		"timeZoneId":         nil,
		"deviceSerialNumber": r.DeviceSerialNumber,
		"deviceType":         r.DeviceType,
		"reminderLabel":      r.Label,
	}
	if r.Recurrence != nil {
		body["rRuleData"] = map[string]any{
			"recurrenceRules": []string{r.Recurrence.buildFullRRule(r.FireAt)},
		}
	}
	return body
}

func applyReminderResponse(r *Reminder, raw map[string]any) {
	if v, ok := raw["status"].(string); ok {
		r.Status = v
	}
	if v := intFromAny(raw["version"]); v != 0 {
		r.Version = v
	}
	if t := parseListTime(raw["lastUpdatedDate"]); !t.IsZero() {
		r.UpdatedAt = t
	}
	if t := alarmTimeFromAny(raw["alarmTime"]); !t.IsZero() {
		r.FireAt = t
	}
	if v, ok := raw["reminderLabel"].(string); ok && v != "" {
		r.Label = v
	}
	r.Acknowledged = extractAcknowledged(raw["followUpMetadata"])
	r.serverShape = raw
}

// suppress unused-import warning for time when no other reference exists.
var _ = time.Now
