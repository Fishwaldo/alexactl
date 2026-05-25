package amazondevices

import (
	"log/slog"
	"net/http"
)

// Option configures a Client.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(c *config) { f(c) }

// config holds the resolved options. Internal; populated by New.
type config struct {
	httpClient *http.Client
	logger     *slog.Logger
	session    *Session
	site       string
	userAgent  string
}

func defaultConfig() *config {
	return &config{
		site: "https://www.amazon.com",
	}
}

// WithHTTPClient overrides the default *http.Client (which has a cookie
// jar and HTTP/2 enabled).
func WithHTTPClient(c *http.Client) Option {
	return optionFunc(func(cfg *config) { cfg.httpClient = c })
}

// WithLogger sets the *slog.Logger used by the library. Defaults to
// slog.Default(). Pass slog.New(slog.DiscardHandler) to silence the library.
// The supplied logger's handler is wrapped by an internal scrub.Handler
// that redacts credential-key values from log attributes.
func WithLogger(l *slog.Logger) Option {
	return optionFunc(func(cfg *config) { cfg.logger = l })
}

// WithSession restores a previously serialized Session, skipping the need
// for a fresh Auth.Login if the tokens are still valid.
func WithSession(s *Session) Option {
	return optionFunc(func(cfg *config) { cfg.session = s })
}

// WithSite overrides the default site ("https://www.amazon.com"). Useful
// for accounts whose home region differs from the US default. A change in
// site is also detected after first login (via customer_info.home_region).
func WithSite(site string) Option {
	return optionFunc(func(cfg *config) { cfg.site = site })
}

// WithUserAgent overrides the impersonated browser User-Agent string.
func WithUserAgent(ua string) Option {
	return optionFunc(func(cfg *config) { cfg.userAgent = ua })
}

