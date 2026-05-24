package httpwrapper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// defaultRefreshTTL is the fallback access-token lifetime applied when the
// refresh response omits `expires_in` or carries a non-positive value, so
// callers don't crash on a malformed response.
const defaultRefreshTTL = time.Hour

// Sentinel errors returned by RefreshAccessToken. Callers (in particular
// the main amazondevices package) use errors.Is to map these to
// higher-level sentinels (ErrCannotAuthenticate / ErrCannotConnect).
var (
	// ErrRefreshHTTP indicates the /auth/token endpoint returned a non-2xx
	// status.
	ErrRefreshHTTP = errors.New("httpwrapper: refresh http failure")

	// ErrRefreshNoToken indicates the refresh response decoded as JSON but
	// did not contain an access_token field.
	ErrRefreshNoToken = errors.New("httpwrapper: refresh response missing access_token")
)

// RefreshInput carries the parameters needed to call the Amazon
// /auth/token refresh endpoint.
type RefreshInput struct {
	RefreshToken string

	// TokenURL is the full URL of /auth/token. Default
	// "https://api.amazon.com/auth/token" is used if empty.
	TokenURL string
}

// RefreshAccessToken exchanges a refresh token for a fresh access token.
// Returns the new access_token string and the token's lifetime parsed from
// the response's `expires_in` field. If `expires_in` is missing or
// non-positive, defaultRefreshTTL is returned with a debug log entry.
// Domain is taken from the current Wrapper state.
//
// The call is issued with the no-refresh context marker so that a 401
// from the /auth/token endpoint itself surfaces as ErrRefreshHTTP rather
// than triggering Do's transparent-refresh path, which would otherwise
// recurse back into RefreshAccessToken indefinitely.
func (w *Wrapper) RefreshAccessToken(ctx context.Context, in RefreshInput) (string, time.Duration, error) {
	url := in.TokenURL
	if url == "" {
		url = "https://api.amazon.com/auth/token"
	}

	form := map[string]string{
		"app_name":             amzconst.AmazonAppName,
		"app_version":          amzconst.AmazonAppVersion,
		"di.sdk.version":       "6.12.4",
		"source_token":         in.RefreshToken,
		"package_name":         amzconst.AmazonAppBundleID,
		"di.hw.version":        "iPhone",
		"platform":             "iOS",
		"requested_token_type": amzconst.RefreshAccessToken,
		"source_token_type":    "refresh_token",
		"di.os.name":           "iOS",
		"di.os.version":        amzconst.AmazonClientOS,
		"current_version":      "6.12.4",
		"previous_version":     "6.12.4",
		"domain":               fmt.Sprintf("www.amazon.%s", w.Domain()),
	}

	resp, err := w.Do(withNoRefresh(ctx), Request{
		Method: http.MethodPost,
		URL:    url,
		Form:   form,
	})
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return "", 0, fmt.Errorf("%w: status %d", ErrRefreshHTTP, resp.StatusCode)
	}

	body, err := DecodeJSON(resp)
	if err != nil {
		return "", 0, err
	}
	tok, _ := body[amzconst.RefreshAccessToken].(string)
	if tok == "" {
		return "", 0, ErrRefreshNoToken
	}

	ttl := parseExpiresIn(body)
	if ttl <= 0 {
		w.Logger(ctx).DebugContext(ctx, "amazondevices: refresh response missing or non-positive expires_in; using default",
			slog.Duration("default_ttl", defaultRefreshTTL))
		ttl = defaultRefreshTTL
	}
	return tok, ttl, nil
}

// parseExpiresIn coerces the `expires_in` JSON field (typically a number,
// occasionally serialized as a string) into a time.Duration in seconds.
// Returns 0 if the field is absent or unparseable; the caller substitutes
// the default TTL.
func parseExpiresIn(body map[string]any) time.Duration {
	v, ok := body["expires_in"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	case int:
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	case int64:
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	case string:
		var secs int64
		if _, err := fmt.Sscanf(n, "%d", &secs); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}
