// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"errors"
	"fmt"
)

// Sentinel errors. Use errors.Is to match a category.
var (
	// ErrCannotAuthenticate is returned when the credentials are rejected
	// or a previously valid session is no longer usable.
	ErrCannotAuthenticate = errors.New("alexactl: cannot authenticate")

	// ErrCannotConnect is returned when network-level errors prevent any
	// response from being received.
	ErrCannotConnect = errors.New("alexactl: cannot connect")

	// ErrCannotRetrieveData is returned when Amazon returns a non-OK HTTP
	// status that isn't an auth failure.
	ErrCannotRetrieveData = errors.New("alexactl: cannot retrieve data")

	// ErrCannotRegisterDevice is returned when /auth/register fails during
	// the OAuth flow.
	ErrCannotRegisterDevice = errors.New("alexactl: cannot register device")

	// ErrDeviceNotFound is returned when callers reference a device by
	// serial number that isn't present in the cache.
	ErrDeviceNotFound = errors.New("alexactl: device not found")

	// ErrWrongMethod is returned when callers invoke a login method that
	// doesn't match the available state (e.g., Restore without a Session).
	ErrWrongMethod = errors.New("alexactl: wrong login method")

	// ErrReauthRequired signals that the caller must run Auth.Login again.
	// Surfaced by the Phase-3 push event stream before the channel closes.
	ErrReauthRequired = errors.New("alexactl: reauth required")

	// ErrCannotControlDevice is returned when Amazon's setEndpointFeatures
	// mutation responds with a structured error for one of the requested
	// FeatureControl entries (e.g. "INVALID_VALUE" for out-of-range
	// brightness, "ENDPOINT_UNREACHABLE" for an offline smart plug).
	// Wrapped errors carry the upstream `code` and `message` via the
	// formatted string; callers can `errors.Is(err, ErrCannotControlDevice)`
	// to distinguish protocol-level rejections from transport errors
	// (which return ErrCannotConnect / ErrCannotRetrieveData).
	ErrCannotControlDevice = errors.New("alexactl: cannot control device")

	// ErrCannotRestartDevice is returned when the reboot endpoint rejects
	// a DevicesService.Restart call. Mirrors upstream CannotRestartDevice.
	ErrCannotRestartDevice = errors.New("alexactl: cannot restart device")

	// ErrControllerNotImplemented is returned by typed control accessors
	// whose wire payload shape has not yet been confirmed against real
	// hardware. The stub Device.Range / Device.Toggle
	// accessors return this sentinel — when callers see it, the
	// stub also emits a WARN log directing them to file an issue with a
	// HAR capture so we can wire the real path.
	//
	//	if err := dvc.Range("FanSpeed").Set(ctx, 5.0); errors.Is(err, ErrControllerNotImplemented) {
	//	    // expected — Range write surface is HAR-capture-gated
	//	}
	ErrControllerNotImplemented = errors.New("alexactl: typed controller not yet wired — HAR capture needed")

	// ErrListItemNotFound is returned by *List write methods (Complete,
	// Uncomplete, Delete, Rename) when the passed *ListItem is nil or
	// belongs to a different *List than the receiver. Distinct from
	// transport-level 4xx errors so callers can branch on it directly:
	//
	//	if errors.Is(err, ErrListItemNotFound) {
	//	    // we passed the wrong item to the wrong list; reload and retry
	//	}
	ErrListItemNotFound = errors.New("alexactl: list item not found")

	// ErrAlarmNotFound is returned by *AlarmsService write methods
	// (Update, Delete, Enable, Disable, Snooze) when the passed *Alarm
	// is nil or belongs to a different *AlarmsService than the receiver.
	// Distinct from transport-level 4xx errors so callers can branch
	// on it directly:
	//
	//	if errors.Is(err, ErrAlarmNotFound) {
	//	    // wrong item; reload and retry
	//	}
	ErrAlarmNotFound = errors.New("alexactl: alarm not found")

	// ErrReminderNotFound is the same sentinel as ErrAlarmNotFound but
	// for *RemindersService write methods.
	ErrReminderNotFound = errors.New("alexactl: reminder not found")

	// ErrTimerNotFound is the same sentinel as ErrAlarmNotFound but
	// for *TimersService write methods.
	ErrTimerNotFound = errors.New("alexactl: timer not found")
)

// APIError carries HTTP-level detail about a failed request. Its Unwrap
// returns one of the sentinel category errors above so callers can
// errors.Is against a category, and errors.As to extract details.
//
// APIError intentionally does NOT include the response body — callers may log
// errors through non-scrubbing loggers, and response bodies can echo back
// credential material (tokens, cookies, OTPs). Use logs at the HTTP layer for
// body diagnostics.
type APIError struct {
	// Category is one of the sentinel errors above.
	Category error

	// Method is the HTTP method of the failed request (e.g. "GET", "POST").
	// Empty when the failure was not tied to a specific HTTP call.
	Method string

	// StatusCode is the HTTP status returned, or 0 if the failure was
	// pre-response (e.g., network error).
	StatusCode int

	// URL is the absolute URL of the failed request.
	URL string
}

// Error implements error.
//
// Format depends on which fields are populated:
//   - Method != "" or StatusCode != 0: "<category>: <method> <status> <url>"
//   - URL != "":                       "<category>: <url>"
//   - otherwise:                       "<category>"
//
// This avoids the misleading "<category>:  0 " rendering when the failure
// was not tied to a specific HTTP call (e.g., pre-request validation).
func (e *APIError) Error() string {
	switch {
	case e.Method != "" || e.StatusCode != 0:
		return fmt.Sprintf("%s: %s %d %s", e.Category, e.Method, e.StatusCode, e.URL)
	case e.URL != "":
		return fmt.Sprintf("%s: %s", e.Category, e.URL)
	default:
		return e.Category.Error()
	}
}

// Unwrap returns the Category sentinel so errors.Is matches.
func (e *APIError) Unwrap() error { return e.Category }
