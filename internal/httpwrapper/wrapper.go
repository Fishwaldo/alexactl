// Package httpwrapper provides the authenticated HTTP request layer used
// by all public services in the amazondevices package. It owns the
// session cookies, CSRF token, retry behavior, and token refresh.
package httpwrapper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/locale"
	"github.com/Fishwaldo/goamazondevices/internal/scrub"
)

// recentRefreshWindow is the deduplication window for transparent 401
// refreshes. If a concurrent 401-refresh sequence completed within this
// window, a sibling refresh is treated as a no-op.
const recentRefreshWindow = 5 * time.Second

// Wrapper holds the authenticated request state. Safe for concurrent use.
type Wrapper struct {
	mu sync.RWMutex

	httpClient *http.Client
	logger     *slog.Logger

	site        string
	domain      string
	language    string
	countryCode string

	initialCookies map[string]string
	// sessionCookies is the post-login cookie set. When non-nil, it takes
	// precedence over initialCookies for the per-request seed in
	// injectInitialCookies. Mirrors Python upstream's pattern where
	// `_session_request` switches between `self._cookies` (pre-login) and
	// `_load_website_cookies()` (post-login) on every call.
	sessionCookies map[string]string

	csrf      string
	userAgent string
	version   string

	// Transparent 401-refresh state. refreshFn is injected by the
	// AuthService; when non-nil, Do will transparently refresh on a 401
	// and retry the original request once. refreshMu serializes concurrent
	// refresh attempts; refreshedAt records the last successful refresh so
	// siblings can short-circuit.
	refreshFn   func(ctx context.Context) error
	refreshMu   sync.Mutex
	refreshedAt time.Time
}

// Domain returns the current Amazon domain suffix (e.g., "com", "co.jp").
func (w *Wrapper) Domain() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.domain
}

// Language returns the current BCP-47 language tag for the session.
func (w *Wrapper) Language() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.language
}

// Site returns the full URL prefix for the session (e.g., "https://www.amazon.com").
func (w *Wrapper) Site() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.site
}

// SetCSRF stores the latest CSRF token.
func (w *Wrapper) SetCSRF(token string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.csrf = token
}

// ClearCSRF discards the CSRF token. Used after a domain redirect.
func (w *Wrapper) ClearCSRF() { w.SetCSRF("") }

// SetVersion sets the build-version header value (emitted as x-amzn-build-version).
func (w *Wrapper) SetVersion(v string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.version = v
}

// HeadersForRequest builds the default header set for a request.
func (w *Wrapper) HeadersForRequest() http.Header {
	w.mu.RLock()
	defer w.mu.RUnlock()

	h := http.Header{}
	h.Set("Accept-Charset", "utf-8")
	// NOTE: Do NOT set Accept-Encoding here. Go's net/http Transport adds
	// `Accept-Encoding: gzip` automatically AND transparently decompresses
	// the response body. Setting it explicitly disables auto-decompression
	// (see net/http/transport.go: "DisableCompression"), which left us with
	// a binary-gzip body that goquery couldn't parse.
	h.Set("Connection", "keep-alive")
	ua := w.userAgent
	if ua == "" {
		ua = amzconst.UserAgentBrowser
	}
	h.Set("User-Agent", ua)
	if w.language != "" {
		h.Set("Accept-Language", w.language)
	}
	h.Set("x-amzn-client", "github.com/Fishwaldo/goamazondevices")
	if w.version != "" {
		h.Set("x-amzn-build-version", w.version)
	}
	if w.csrf != "" {
		h.Set("csrf", w.csrf)
	}
	return h
}

// Logger returns the configured logger. If the caller did not supply a
// logger via WithLogger, the default is slog.Default(). The handler is
// always wrapped by scrub.Handler so credential and email attributes are
// redacted before being emitted. Callers wanting silence should pass
// slog.New(slog.DiscardHandler) via WithLogger.
//
// The ctx parameter is accepted so future context-aware handlers can
// attach trace IDs etc.
func (w *Wrapper) Logger(ctx context.Context) *slog.Logger {
	_ = ctx
	return w.logger
}

// Config carries the construction-time settings for a Wrapper.
type Config struct {
	HTTPClient *http.Client // optional; default has cookie jar + HTTP/2
	Logger     *slog.Logger // optional; defaults to slog.Default()
	Site       string       // initial site (e.g., "https://www.amazon.com")
	UserAgent  string       // optional override
	Version    string       // version string emitted in x-amzn-build-version
	RandReader io.Reader    // optional; used for initial cookies (tests inject)
}

// New constructs a Wrapper with the given config. Performs no I/O.
func New(cfg Config) (*Wrapper, error) {
	if cfg.Site == "" {
		cfg.Site = amzconst.DefaultSite
	}
	if cfg.HTTPClient == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("httpwrapper: cookiejar: %w", err)
		}
		cfg.HTTPClient = &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	cfg.Logger = slog.New(scrub.Handler(cfg.Logger.Handler()))

	w := &Wrapper{
		httpClient: cfg.HTTPClient,
		logger:     cfg.Logger,
		userAgent:  cfg.UserAgent,
		version:    cfg.Version,
	}
	if err := w.SetSite(cfg.Site); err != nil {
		return nil, err
	}

	ck, err := BuildInitialCookies(cfg.RandReader)
	if err != nil {
		return nil, err
	}
	w.initialCookies = ck

	return w, nil
}

// SetSite updates the current site/domain/language, deriving the BCP-47
// language tag from the country code embedded in the domain.
func (w *Wrapper) SetSite(site string) error {
	domain := strings.TrimPrefix(site, "https://www.amazon.")
	domain = strings.ToLower(domain)
	cc := "us"
	if domain != "com" {
		if i := strings.LastIndex(domain, "."); i >= 0 {
			cc = domain[i+1:]
		} else {
			cc = domain
		}
	}
	lang, err := locale.ForCountry(cc)
	if err != nil {
		lang = "en-US"
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.site = site
	w.domain = domain
	w.language = lang
	w.countryCode = cc
	return nil
}

// CountryCode returns the ISO-3166 alpha-2 country code derived from the
// current site domain (e.g., "us" for "https://www.amazon.com", "jp" for
// "https://www.amazon.co.jp"). Lowercased.
func (w *Wrapper) CountryCode() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.countryCode
}

// HTTPClient exposes the underlying client. Callers should use Do; this is
// for cases where the cookie jar needs direct manipulation.
func (w *Wrapper) HTTPClient() *http.Client {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.httpClient
}

// SetRefreshFn installs the closure used by Do for transparent token
// refresh on 401 responses. The closure should call
// w.RefreshAccessToken and propagate the new access_token back into the
// caller's session state. Passing nil disables transparent refresh.
func (w *Wrapper) SetRefreshFn(fn func(context.Context) error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.refreshFn = fn
}

// SetSessionCookies installs the post-login cookie set that will be
// re-injected onto every outbound request (overriding the pre-login
// initialCookies). Pass nil to revert to pre-login behavior.
//
// Typical contents: the WebsiteCookies map returned by /auth/register,
// plus session-token (from store_authentication_cookie) and lc-acbit
// (= the BCP-47 language tag). Callers are responsible for assembling
// the full map; the wrapper just stores and replays it.
func (w *Wrapper) SetSessionCookies(cookies map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if cookies == nil {
		w.sessionCookies = nil
		return
	}
	cp := make(map[string]string, len(cookies))
	for k, v := range cookies {
		cp[k] = v
	}
	w.sessionCookies = cp
}

// doRefresh serializes concurrent refresh attempts. If a refresh
// completed within recentRefreshWindow, this returns nil immediately so
// sibling requests do not double-refresh.
func (w *Wrapper) doRefresh(ctx context.Context) error {
	w.refreshMu.Lock()
	defer w.refreshMu.Unlock()

	if time.Since(w.refreshedAt) < recentRefreshWindow {
		return nil
	}

	w.mu.RLock()
	fn := w.refreshFn
	w.mu.RUnlock()
	if fn == nil {
		return nil
	}

	if err := fn(ctx); err != nil {
		return err
	}
	w.refreshedAt = time.Now()
	return nil
}
