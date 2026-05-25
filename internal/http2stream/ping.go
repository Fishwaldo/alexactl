package http2stream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrPingReauthRequired is returned when a ping receives an auth-class
// status (401, 403, 407). The caller is expected to either refresh and
// retry or, if refresh fails, surface ReauthRequiredEvent to
// subscribers and stop.
var ErrPingReauthRequired = errors.New("http2stream: ping requires reauth")

// Ping sends a single keepalive POST to the AVS /ping endpoint with
// the supplied bearer token. Returns nil on 2xx. Returns
// ErrPingReauthRequired wrapped via fmt.Errorf on 401/403/407 so
// callers can errors.Is. Returns a plain error on any other non-2xx.
//
// The HTTP client passed in MUST be configured for the same site as
// the stream itself — caller-owned to keep this package free of
// region/site logic.
func Ping(ctx context.Context, client *http.Client, url, accessToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("http2stream: build ping request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http2stream: ping: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusProxyAuthRequired:
		return fmt.Errorf("%w: status %d", ErrPingReauthRequired, resp.StatusCode)
	}
	return fmt.Errorf("http2stream: ping: unexpected status %d", resp.StatusCode)
}
