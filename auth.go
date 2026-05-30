// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/cache"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// OTPProvider supplies the MFA OTP code to AuthService.Login. The callback
// is invoked only AFTER Amazon has accepted the password and served the
// MFA form.
type OTPProvider interface {
	OTP(ctx context.Context) (string, error)
}

// OTPProviderFunc adapts a function to the OTPProvider interface.
type OTPProviderFunc func(ctx context.Context) (string, error)

// OTP implements OTPProvider.
func (f OTPProviderFunc) OTP(ctx context.Context) (string, error) { return f(ctx) }

// AuthService implements the Amazon Alexa OAuth + MFA login flow.
type AuthService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex // protects session
	session *Session

	// consumers is the set of sister services (DevicesService, MediaService)
	// that hold their own *Session pointers and need to be notified when
	// AuthService swaps in a new session (Login, Restore, refreshAccessToken).
	// Wired by NewClient at construction time; not mutated afterwards.
	consumers []sessionConsumer

	// cacheManager is wired by NewClient so broadcast can clear
	// session-tagged caches on session swap.
	cacheManager *cache.Manager

	// customerIDCache caches obtainCustomerIDForSerial lookups keyed by
	// device serial. Long class, session-tagged. No loader — the lookup
	// path writes directly via Set.
	customerIDCache *cache.Cache[string]

	// postBroadcastHook is invoked after every successful broadcast.
	// Wired by NewClient when WithPushCacheBridge is set — the hook
	// itself is gated with sync.Once so it runs once per Client.
	postBroadcastHook func()

	// Test seams. Production leaves these nil.
	sleepFn     func(ctx context.Context, seconds int)
	tokenURL    string // for refresh_data; defaults to api.amazon.com
	registerURL string
}

// Session returns the current session, or nil if no login/restore has
// completed. Returned pointer is immutable; AuthService swaps in a new
// pointer on refresh rather than mutating in place.
func (s *AuthService) Session() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

// broadcast atomically swaps AuthService's session pointer to sess and
// pushes sess to every registered consumer. Callers MUST NOT hold s.mu.
// If a cache Manager is wired, session-tagged caches are cleared before
// the consumer fan-out.
func (s *AuthService) broadcast(sess *Session) {
	s.mu.Lock()
	s.session = sess
	consumers := s.consumers // immutable after construction
	mgr := s.cacheManager
	hook := s.postBroadcastHook
	s.mu.Unlock()
	if mgr != nil {
		mgr.InvalidateTag("session")
	}
	for _, c := range consumers {
		c.setSession(sess)
	}
	if hook != nil {
		hook()
	}
}

// Login performs the full interactive OAuth+PKCE+MFA flow. The OTPProvider
// is invoked only after Amazon serves the MFA form.
//
// On success, the returned Session is also stored in s for subsequent
// service calls. Callers should persist it to disk (mode 0600).
func (s *AuthService) Login(ctx context.Context, email, password string, otp OTPProvider) (*Session, error) {
	logger := s.wrapper.Logger(ctx)
	logger.DebugContext(ctx, "alexactl: starting Login", "email", email)

	authcode, codeVerifier, serial, err := s.doOAuthDance(ctx, email, password, otp, "en-US", nil)
	if err != nil {
		return nil, err
	}

	sess, err := s.registerDevice(ctx, registerInput{
		AuthorizationCode: authcode,
		CodeVerifier:      codeVerifier,
		Serial:            serial,
		ClientIDHex:       buildClientID(serial),
		RegisterURL:       s.registerURL,
	})
	if err != nil {
		return nil, err
	}

	// If customer_info.home_region is FE, redirect to the JP site and
	// refresh cookies. Otherwise keep the current site.
	if sess.CustomerInfo.HomeRegion == "FE" {
		_ = s.wrapper.SetSite(amzconst.FESite)
		sess.Site = amzconst.FESite
		sess.Domain = s.wrapper.Domain()
	}

	// Switch the wrapper from pre-login cookies (frc/map-md/amzn-app-id) to
	// the post-login set so every subsequent request — starting with
	// obtainCustomerIDForSerial — carries the website cookies onto every
	// *.amazon.<tld> host (notably alexa.amazon.<tld>).
	s.wrapper.SetSessionCookies(buildSessionCookies(sess, s.wrapper.Language()))

	// Defer broadcasting the session to consumers (and to AuthService itself)
	// until obtainCustomerID succeeds. Otherwise a failure here would leave
	// AuthService in a half-populated state that pretends to be Restore-able.
	customerID, err := s.obtainCustomerIDForSerial(
		ctx,
		fmt.Sprintf("https://alexa.amazon.%s", s.wrapper.Domain()),
		sess.DeviceInfo.DeviceSerialNumber,
	)
	if err != nil {
		return nil, err
	}
	sess.AccountCustomerID = customerID

	s.broadcast(sess)
	s.wrapper.SetRefreshFn(s.refreshAccessToken)
	return sess, nil
}

// Restore re-validates a Session previously passed via WithSession. It
// refreshes the access token and re-fetches the customer ID if missing.
func (s *AuthService) Restore(ctx context.Context) (*Session, error) {
	s.mu.RLock()
	cur := s.session
	s.mu.RUnlock()
	if cur == nil {
		return nil, ErrWrongMethod
	}
	s.wrapper.Logger(ctx).DebugContext(ctx, "alexactl: restoring session")

	// Seed the wrapper with the persisted website cookies so subsequent
	// requests authenticate. Fresh Login picks these up naturally from
	// Set-Cookie headers during the OAuth dance, but Restore skips that
	// dance and would otherwise hit `alexa.amazon.com` with no auth
	// cookies, get bounced to /ap/signin (HTML 200), and fail to parse.
	s.wrapper.SetSessionCookies(buildSessionCookies(cur, s.wrapper.Language()))

	tok, ttl, err := s.wrapper.RefreshAccessToken(ctx, httpwrapper.RefreshInput{
		RefreshToken: cur.RefreshToken,
		TokenURL:     s.tokenURL,
	})
	if err != nil {
		return nil, mapRefreshError(err)
	}

	// Build a fresh *Session pointer rather than mutating the existing one
	// in place. Other services (Devices, Media) hold the old pointer and
	// will be swapped to the new one via broadcast below.
	next := *cur
	next.AccessToken = tok
	next.ExpiresAt = time.Now().UTC().Add(ttl)

	if next.AccountCustomerID == "" {
		id, err := s.obtainCustomerIDForSerial(
			ctx,
			fmt.Sprintf("https://alexa.amazon.%s", s.wrapper.Domain()),
			next.DeviceInfo.DeviceSerialNumber,
		)
		if err != nil {
			return nil, err
		}
		next.AccountCustomerID = id
	}

	s.broadcast(&next)
	s.wrapper.SetRefreshFn(s.refreshAccessToken)
	return &next, nil
}

// refreshAccessToken is the closure installed on the wrapper via
// SetRefreshFn. On a 401, the wrapper invokes it to obtain a fresh access
// token, then retries the original request once.
//
// To avoid torn reads, this mints a new *Session pointer (copy + new
// AccessToken/ExpiresAt) and broadcasts it so each service swaps its
// pointer atomically rather than racing on shared struct fields.
func (s *AuthService) refreshAccessToken(ctx context.Context) error {
	s.mu.RLock()
	cur := s.session
	s.mu.RUnlock()
	if cur == nil {
		return fmt.Errorf("%w: no session to refresh", ErrCannotAuthenticate)
	}
	tok, ttl, err := s.wrapper.RefreshAccessToken(ctx, httpwrapper.RefreshInput{
		RefreshToken: cur.RefreshToken,
		TokenURL:     s.tokenURL,
	})
	if err != nil {
		return mapRefreshError(err)
	}
	next := *cur
	next.AccessToken = tok
	next.ExpiresAt = time.Now().UTC().Add(ttl)
	s.broadcast(&next)
	return nil
}

// buildSessionCookies constructs the cookie map that the httpwrapper
// re-injects onto every post-login request. Mirrors Python upstream's
// `_load_website_cookies(language)`:
//
//   - all entries from sess.WebsiteCookies
//   - session-token = sess.StoreAuthCookie  (the value Amazon returned
//     in `tokens.store_authentication_cookie.cookie`)
//   - lc-acbit = current BCP-47 language tag
//
// Returns nil if sess is nil.
func buildSessionCookies(sess *Session, language string) map[string]string {
	if sess == nil {
		return nil
	}
	out := make(map[string]string, len(sess.WebsiteCookies)+2)
	for k, v := range sess.WebsiteCookies {
		out[k] = v
	}
	if sess.StoreAuthCookie != "" {
		out["session-token"] = sess.StoreAuthCookie
	}
	if language != "" {
		out["lc-acbit"] = language
	}
	return out
}

// mapRefreshError converts the httpwrapper-level refresh sentinel errors
// into the main-package sentinel categories so callers can match against
// ErrCannotAuthenticate / ErrCannotConnect.
func mapRefreshError(err error) error {
	if errors.Is(err, httpwrapper.ErrRefreshHTTP) || errors.Is(err, httpwrapper.ErrRefreshNoToken) {
		return fmt.Errorf("%w: %w", ErrCannotAuthenticate, err)
	}
	return fmt.Errorf("%w: %w", ErrCannotConnect, err)
}

// marshalCustomerIDEntries / unmarshalCustomerIDEntries persist the
// auth.customerID cache via the engine's Persistence adapter.
func marshalCustomerIDEntries(in map[string]cache.Entry[string]) ([]byte, error) {
	return json.Marshal(in)
}

func unmarshalCustomerIDEntries(b []byte) (map[string]cache.Entry[string], error) {
	out := map[string]cache.Entry[string]{}
	err := json.Unmarshal(b, &out)
	return out, err
}
