package amazondevices

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// Client is the entry point for the goamazondevices library. Construct
// with NewClient; safe for concurrent use.
type Client struct {
	cfg *config

	wrapper *httpwrapper.Wrapper

	Auth          *AuthService
	Devices       *DevicesService
	Media         *MediaService
	Sequence      *SequenceService
	DnD           *DnDService
	Sensors       *SensorsService
	Notifications *NotificationsService
	History       *HistoryService
	Push          *PushService
}

// NewClient constructs a Client. It performs no I/O.
//
// The first network call happens when the caller invokes Auth.Login,
// Auth.Restore, or any service method.
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
		cfg:           cfg,
		wrapper:       wr,
		Auth:          &AuthService{wrapper: wr},
		Devices:       &DevicesService{wrapper: wr},
		Media:         &MediaService{wrapper: wr},
		Sequence:      &SequenceService{wrapper: wr},
		DnD:           &DnDService{wrapper: wr},
		Sensors:       &SensorsService{wrapper: wr},
		Notifications: &NotificationsService{wrapper: wr},
		History:       &HistoryService{wrapper: wr},
	}
	// Phase 4b: smart-home write client. Same wrapper as Devices so
	// auth refresh / retry / scrub carry over. Back-referenced on
	// every cached *Device so accessors can route writes without a
	// global.
	c.Devices.smartHome = newSmartHomeClient(wr)

	c.Push = &PushService{
		wrapper: wr,
		streamClient: &http.Client{
			Timeout: 0, // streaming — no top-level timeout
		},
	}
	c.Push.initialSyncFn = func(ctx context.Context) (map[string]*VolumeState, error) {
		return c.Media.DeviceVolumes(ctx)
	}
	c.Auth.consumers = []sessionConsumer{
		c.Devices, c.Media, c.Sequence, c.DnD, c.Sensors, c.Notifications, c.History, c.Push,
	}
	c.Media.sequence = c.Sequence

	// Sensors enrichment step for DevicesService.Refresh. DnD status,
	// Notifications, and Sensors are all best-effort; an individual
	// failure logs WARN and the next step proceeds with nil for that
	// input.
	c.Devices.sensorsHook = func(ctx context.Context, devices map[string]*Device, endpoints map[string]string) error {
		dnd, dndErr := c.DnD.Status(ctx)
		if dndErr != nil {
			c.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: DnD lookup failed; sensors will be applied without DnD",
				"err", dndErr)
			dnd = nil
		}
		notifications, nErr := c.Notifications.Get(ctx)
		if nErr != nil {
			c.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: notifications lookup failed; sensors will be applied without notifications",
				"err", nErr)
			notifications = nil
		}
		return c.Sensors.Update(ctx, devices, endpoints, dnd, notifications)
	}

	// If the caller supplied a Session via WithSession, push it through the
	// same broadcast path so every consumer (and Auth itself) ends up with
	// the same pointer.
	if cfg.session != nil {
		c.Auth.broadcast(cfg.session)
	}
	return c, nil
}
