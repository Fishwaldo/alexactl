package amazondevices

import (
	"fmt"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// Client is the entry point for the goamazondevices library. Construct
// with NewClient; safe for concurrent use.
type Client struct {
	cfg *config

	wrapper *httpwrapper.Wrapper

	Auth    *AuthService
	Devices *DevicesService
	Media   *MediaService
}

// NewClient constructs a Client. It performs no I/O.
//
// The first network call happens when the caller invokes Auth.Login,
// Auth.Restore, or any DevicesService / MediaService method.
func NewClient(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt.apply(cfg)
	}

	// If a Session is supplied, override the site to its recorded site so
	// the wrapper's domain/language match what the session was issued for.
	site := cfg.site
	if cfg.session != nil && cfg.session.Site != "" {
		site = cfg.session.Site
	}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: cfg.httpClient,
		Logger:     cfg.logger,
		Site:       site,
		UserAgent:  cfg.userAgent,
		Version:    Version,
	})
	if err != nil {
		return nil, fmt.Errorf("amazondevices: build http wrapper: %w", err)
	}

	c := &Client{
		cfg:     cfg,
		wrapper: wr,
		Auth:    &AuthService{wrapper: wr},
		Devices: &DevicesService{wrapper: wr},
		Media:   &MediaService{wrapper: wr},
	}
	// Wire AuthService to broadcast new sessions to Devices and Media.
	c.Auth.consumers = []sessionConsumer{c.Devices, c.Media}

	// If the caller supplied a Session via WithSession, push it through the
	// same broadcast path so every consumer (and Auth itself) ends up with
	// the same pointer.
	if cfg.session != nil {
		c.Auth.broadcast(cfg.session)
	}
	return c, nil
}
