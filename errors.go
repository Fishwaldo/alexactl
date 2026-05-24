package amazondevices

import (
	"errors"
	"fmt"
)

// Sentinel errors. Use errors.Is to match a category.
var (
	// ErrCannotAuthenticate is returned when the credentials are rejected
	// or a previously valid session is no longer usable.
	ErrCannotAuthenticate = errors.New("amazondevices: cannot authenticate")

	// ErrCannotConnect is returned when network-level errors prevent any
	// response from being received.
	ErrCannotConnect = errors.New("amazondevices: cannot connect")

	// ErrCannotRetrieveData is returned when Amazon returns a non-OK HTTP
	// status that isn't an auth failure.
	ErrCannotRetrieveData = errors.New("amazondevices: cannot retrieve data")

	// ErrCannotRegisterDevice is returned when /auth/register fails during
	// the OAuth flow.
	ErrCannotRegisterDevice = errors.New("amazondevices: cannot register device")

	// ErrDeviceNotFound is returned when callers reference a device by
	// serial number that isn't present in the cache (e.g. SetDefault).
	ErrDeviceNotFound = errors.New("amazondevices: device not found")

	// ErrWrongMethod is returned when callers invoke a login method that
	// doesn't match the available state (e.g., Restore without a Session).
	ErrWrongMethod = errors.New("amazondevices: wrong login method")

	// ErrReauthRequired signals that the caller must run Auth.Login again.
	// Surfaced by the Phase-3 push event stream before the channel closes.
	ErrReauthRequired = errors.New("amazondevices: reauth required")
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
