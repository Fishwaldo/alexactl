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

	Auth      *AuthService
	Devices   *DevicesService
	Media     *MediaService
	Sequence  *SequenceService
	DnD       *DnDService
	Sensors   *SensorsService
	History   *HistoryService
	Push      *PushService
	Lists     *ListsService
	Alarms    *AlarmsService
	Reminders *RemindersService
	Timers    *TimersService
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
		History:       &HistoryService{wrapper: wr},
		Lists:         &ListsService{wrapper: wr},
		Alarms:        &AlarmsService{wrapper: wr},
		Reminders:     &RemindersService{wrapper: wr},
		Timers:        &TimersService{wrapper: wr},
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
		c.Devices, c.Media, c.Sequence, c.DnD, c.Sensors, c.History, c.Push, c.Lists,
		c.Alarms, c.Reminders, c.Timers,
	}
	// Phase 6: wire devicesLookup closures so Create methods can
	// resolve DeviceType from a SerialNumber via the Devices cache.
	c.Alarms.devicesLookup = func(serial string) (*Device, bool) {
		dvc := c.Devices.getCached(serial)
		return dvc, dvc != nil
	}
	c.Reminders.devicesLookup = func(serial string) (*Device, bool) {
		dvc := c.Devices.getCached(serial)
		return dvc, dvc != nil
	}
	c.Timers.devicesLookup = func(serial string) (*Device, bool) {
		dvc := c.Devices.getCached(serial)
		return dvc, dvc != nil
	}
	c.Media.sequence = c.Sequence

	// Sensors enrichment step for DevicesService.Refresh. DnD status and
	// schedules are best-effort; an individual failure logs WARN and the
	// next step proceeds with nil/empty for that input.
	c.Devices.sensorsHook = func(ctx context.Context, devices map[string]*Device, endpoints map[string]string) error {
		dnd, dndErr := c.DnD.Status(ctx)
		if dndErr != nil {
			c.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: DnD lookup failed; sensors will be applied without DnD",
				"err", dndErr)
			dnd = nil
		}
		// Phase 6: populate per-device typed slices via the shared
		// schedules fetcher. Single HTTP round-trip; result split
		// across Device.Alarms/Reminders/Timers per device.
		sched, sErr := fetchSchedules(ctx, c.wrapper)
		if sErr != nil {
			c.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: schedules lookup failed; per-device Alarms/Reminders/Timers will be empty",
				"err", sErr)
			sched = &schedulesFetchResult{}
		}
		populateDeviceSchedules(c.Alarms, c.Reminders, c.Timers, devices, sched)
		return c.Sensors.Update(ctx, devices, endpoints, dnd)
	}

	// If the caller supplied a Session via WithSession, push it through the
	// same broadcast path so every consumer (and Auth itself) ends up with
	// the same pointer.
	if cfg.session != nil {
		c.Auth.broadcast(cfg.session)
	}
	return c, nil
}
