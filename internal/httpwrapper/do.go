package httpwrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// Retry tuning for the retryablehttp client wrapping each request.
//
// retryMaxAttempts is the maximum number of retries after the initial
// attempt (so total attempts = retryMaxAttempts + 1). retryWaitMin and
// retryWaitMax bound the exponential backoff. The 21s cap keeps total
// worst-case time per request bounded under a minute while still giving
// Amazon's 429-throttling some breathing room.
const (
	retryMaxAttempts = 6
	retryWaitMin     = 1 * time.Second
	retryWaitMax     = 21 * time.Second
)

// Request describes a single Amazon API request. Either JSONBody or Form
// (or neither) may be set, never both.
type Request struct {
	Method string
	URL    string

	// JSONBody is marshaled to JSON. Content-Type is set automatically.
	JSONBody any

	// Form is sent as application/x-www-form-urlencoded.
	Form map[string]string

	// ExtraHeaders are appended on top of the defaults.
	ExtraHeaders http.Header
}

// noRefreshKey is a context-internal sentinel used by Do to mark its own
// recursive retry after a transparent 401 refresh. Carrying the marker on
// ctx (rather than mutating req) keeps the public Request semantics
// honest: external callers always see refresh-on-401 as the default.
type noRefreshKeyType struct{}

var noRefreshKey = noRefreshKeyType{}

// withNoRefresh marks the context so the nested Do call will not attempt
// a second token refresh on a 401.
func withNoRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRefreshKey, true)
}

// isNoRefresh reports whether ctx carries the no-refresh marker.
func isNoRefresh(ctx context.Context) bool {
	v, _ := ctx.Value(noRefreshKey).(bool)
	return v
}

// idempotentRetryStatus reports whether retrying on `status` is safe for
// `method`. Per the design notes:
//   - GET/HEAD/PUT/DELETE: retry on 500/503/429.
//   - POST/PATCH: retry only on 429 (RFC 6585 rate-limit signal).
//
// OPTIONS/TRACE are intentionally excluded — Amazon's API does not use
// them, and treating them inconsistently with each other is a foot-gun.
func idempotentRetryStatus(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusInternalServerError, http.StatusServiceUnavailable:
		switch strings.ToUpper(method) {
		case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
			return true
		default:
			return false
		}
	}
	return false
}

// Do executes a single request with default headers, retries on transient
// 5xx/429 errors (idempotent methods only for 5xx), transparent 401
// token refresh, and CSRF-cookie capture from responses. The caller is
// responsible for closing resp.Body.
func (w *Wrapper) Do(ctx context.Context, req Request) (*http.Response, error) {
	if req.JSONBody != nil && req.Form != nil {
		return nil, fmt.Errorf("httpwrapper: cannot set both JSONBody and Form")
	}
	// External callers always get default-on transparent-refresh behavior;
	// the wrapper's recursive retry opts out via the noRefreshKey context
	// marker injected by withNoRefresh.
	allowRefresh := !isNoRefresh(ctx)

	headers := w.HeadersForRequest()

	var body io.Reader
	switch {
	case req.JSONBody != nil:
		buf, err := json.Marshal(req.JSONBody)
		if err != nil {
			return nil, fmt.Errorf("httpwrapper: marshal json body: %w", err)
		}
		body = bytes.NewReader(buf)
		headers.Set("Content-Type", "application/json; charset=utf-8")
	case req.Form != nil:
		v := url.Values{}
		for k, val := range req.Form {
			v.Set(k, val)
		}
		body = strings.NewReader(v.Encode())
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	for k, vs := range req.ExtraHeaders {
		for _, v := range vs {
			headers.Add(k, v)
		}
	}

	logger := w.Logger(ctx)
	reqAttrs := []any{
		slog.String("method", req.Method),
		slog.String("url", req.URL),
	}
	if req.JSONBody != nil {
		reqAttrs = append(reqAttrs, slog.Any("body", req.JSONBody))
	} else if req.Form != nil {
		reqAttrs = append(reqAttrs, slog.Any("body", req.Form))
	}
	logger.DebugContext(ctx, "amazondevices: http request", reqAttrs...)

	// Re-seed pre-login cookies before every request, mirroring Python's
	// `cookie_jar.update_cookies(self._cookies, URL("amazon.<tld>"))` pattern.
	// A previous "only if jar is empty" guard meant that once Amazon set any
	// cookie (e.g. session-id on the very first response), our frc/map-md/
	// amzn-app-id stopped being re-asserted — and any server response that
	// overwrote one of them would silently strip it from the jar for the
	// remainder of the flow.
	w.injectInitialCookies(req.URL)

	rc := retryablehttp.NewClient()
	rc.HTTPClient = w.httpClient
	rc.Logger = nil
	rc.RetryMax = retryMaxAttempts
	rc.RetryWaitMin = retryWaitMin
	rc.RetryWaitMax = retryWaitMax
	rc.CheckRetry = func(_ context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, nil
		}
		if idempotentRetryStatus(req.Method, resp.StatusCode) {
			return true, nil
		}
		return false, nil
	}

	rreq, err := retryablehttp.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, fmt.Errorf("httpwrapper: build request: %w", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			rreq.Header.Add(k, v)
		}
	}

	resp, err := rc.Do(rreq)
	if err != nil {
		return nil, fmt.Errorf("httpwrapper: do request: %w", err)
	}

	// Transparent 401 refresh. If the server says the access token is
	// invalid, the request is the user-facing entry (not the post-refresh
	// retry), and a refresh closure has been injected, drain+close the
	// body, refresh the token, and retry once.
	if resp.StatusCode == http.StatusUnauthorized && allowRefresh {
		w.mu.RLock()
		refreshSet := w.refreshFn != nil
		w.mu.RUnlock()
		if refreshSet {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if rerr := w.doRefresh(ctx); rerr != nil {
				return nil, rerr
			}
			return w.Do(withNoRefresh(ctx), req)
		}
	}

	// Capture CSRF cookie if present.
	for _, c := range resp.Cookies() {
		if c.Name == "csrf" {
			w.SetCSRF(c.Value)
		}
	}

	logger.DebugContext(ctx, "amazondevices: http response",
		"method", req.Method, "url", req.URL, "status", resp.StatusCode)

	// Body-level DEBUG logging. Skipped for /auth/ and /ap/ endpoints
	// because their request/response payloads contain raw credential
	// material (access tokens, MAP cookies, OTP secrets) in arbitrary
	// shapes that the structured scrub.Handler can't reliably redact.
	// Auth-flow diagnostics already log enough at the request layer.
	if !isAuthURL(req.URL) {
		LogResponseBody(ctx, logger, req.Method, req.URL, resp)
	}

	return resp, nil
}

// isAuthURL reports whether the URL is part of the auth flow whose
// bodies must not be logged. Conservative: a parse failure is treated
// as auth (skip body logging) so a malformed URL never accidentally
// exposes credential material.
func isAuthURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	p := u.Path
	return strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/ap/")
}

// injectInitialCookies (re-)seeds the pre-login cookies (frc, map-md,
// amzn-app-id) onto the parent amazon.<tld> domain so they propagate to
// every *.amazon.<tld> subdomain — notably api.amazon.<tld> for device
// registration. Called before every request to mirror Python upstream's
// `cookie_jar.update_cookies(self._cookies, URL("amazon.<tld>"))` pattern.
// Cookies with the same Name+Domain+Path canonical ID are overwritten in
// place by stdlib cookiejar, so this is safe to call repeatedly and any
// server response that clears one of our initial cookies is undone on the
// next request.
func (w *Wrapper) injectInitialCookies(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.httpClient.Jar == nil {
		return
	}
	// Scope the cookies to amazon.<tld> so they apply across subdomains
	// (www.amazon.com → api.amazon.com → alexa.amazon.com). Without an
	// explicit Domain, Go's cookiejar makes them host-only and the OAuth
	// flow's anti-fraud cookies never reach /auth/register.
	//
	// Post-login sessionCookies take precedence over the pre-login
	// initialCookies — this mirrors Python's ternary in `_session_request`:
	//   _cookies = _load_website_cookies() if login_stored_data else self._cookies
	domain := "amazon." + w.domain
	cookies := w.initialCookies
	if w.sessionCookies != nil {
		cookies = w.sessionCookies
	}
	var cs []*http.Cookie
	for name, val := range cookies {
		cs = append(cs, &http.Cookie{
			Name:   name,
			Value:  val,
			Path:   "/",
			Domain: domain,
		})
	}
	w.httpClient.Jar.SetCookies(u, cs)
}
