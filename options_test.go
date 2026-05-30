package alexactl

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions_Defaults(t *testing.T) {
	cfg := defaultConfig()
	assert.Equal(t, "https://www.amazon.com", cfg.site)
	assert.Nil(t, cfg.httpClient)
	assert.Nil(t, cfg.logger)
	assert.Nil(t, cfg.session)
}

func TestOptions_Apply(t *testing.T) {
	hc := &http.Client{}
	lg := slog.Default()
	sess := &Session{AccessToken: "x"}

	cfg := defaultConfig()
	for _, opt := range []Option{
		WithHTTPClient(hc),
		WithLogger(lg),
		WithSession(sess),
		WithSite("https://www.amazon.de"),
		WithUserAgent("UA/1"),
	} {
		opt.apply(cfg)
	}

	assert.Same(t, hc, cfg.httpClient)
	assert.Same(t, lg, cfg.logger)
	assert.Same(t, sess, cfg.session)
	assert.Equal(t, "https://www.amazon.de", cfg.site)
	assert.Equal(t, "UA/1", cfg.userAgent)
}
