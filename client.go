package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/Fishwaldo/goamazondevices/internal/cache"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// Client is the entry point for the goamazondevices library. Construct
// with NewClient; safe for concurrent use.
type Client struct {
	cfg *config

	wrapper      *httpwrapper.Wrapper
	cacheManager *cache.Manager

	// schedulesCache is shared across AlarmsService, RemindersService,
	// TimersService, and the Devices.Refresh sensors hook — one HTTP
	// fetch on /api/notifications dedupes all four callers.
	schedulesCache *cache.Cache[*schedulesFetchResult]

	// pushBridgeOnce starts the push cache bridge exactly once, on the
	// first session broadcast (Auth.Login or Auth.Restore). Set when
	// cfg.pushCacheBridge is true; nil otherwise.
	pushBridgeOnce *sync.Once

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

	classes := cache.DefaultClassConfig()
	for class, d := range cfg.cacheTTLOverrides {
		classes.Set(class, d)
	}
	mgr := cache.NewManager(wr.Logger(context.Background()), classes, cfg.cachePersistence)

	// Per-cache TTL overrides (deprecated convenience options). WithSoundsTTL
	// targets media.sounds only — falls back to ClassLong otherwise.
	soundsTTL := classes.TTL(cache.ClassLong)
	if cfg.soundsTTL > 0 {
		soundsTTL = cfg.soundsTTL
	}

	c := &Client{
		cfg:          cfg,
		wrapper:      wr,
		cacheManager: mgr,
		Auth:         &AuthService{wrapper: wr, cacheManager: mgr},
		Devices:      &DevicesService{wrapper: wr},
		Media:        &MediaService{wrapper: wr},
		Sequence:     &SequenceService{wrapper: wr},
		DnD:          &DnDService{wrapper: wr},
		Sensors:      &SensorsService{wrapper: wr},
		History:      &HistoryService{wrapper: wr},
		Lists:        &ListsService{wrapper: wr},
		Alarms:       &AlarmsService{wrapper: wr},
		Reminders:    &RemindersService{wrapper: wr},
		Timers:       &TimersService{wrapper: wr},
	}
	// Phase 4b: smart-home write client. Same wrapper as Devices so
	// auth refresh / retry / scrub carry over. Back-referenced on
	// every cached *Device so accessors can route writes without a
	// global.
	c.Devices.smartHome = newSmartHomeClient(wr)

	// media.sounds cache (engine-managed).
	c.Media.soundsCache = cache.NewCache[map[string]Sound](
		cache.Config{Name: "media.sounds", TTL: soundsTTL, Tags: []string{"session"}},
		func(ctx context.Context, _ string) (map[string]Sound, error) {
			return c.Media.fetchSounds(ctx)
		},
	)
	cache.Register(mgr, c.Media.soundsCache, marshalSoundsEntries, unmarshalSoundsEntries)

	// sequence.routines cache (engine-managed, Long class).
	c.Sequence.routinesCache = cache.NewCache[map[string]map[string]any](
		cache.Config{Name: "sequence.routines", TTL: classes.TTL(cache.ClassLong)},
		func(ctx context.Context, _ string) (map[string]map[string]any, error) {
			return c.Sequence.fetchRoutines(ctx)
		},
	)
	cache.Register(mgr, c.Sequence.routinesCache, marshalRoutinesEntries, unmarshalRoutinesEntries)

	// history.csrf cache (engine-managed, Long class, session-tagged).
	c.History.csrfCache = cache.NewCache[string](
		cache.Config{Name: "history.csrf", TTL: classes.TTL(cache.ClassLong), Tags: []string{"session"}},
		func(ctx context.Context, _ string) (string, error) {
			return c.History.fetchCSRFToken(ctx)
		},
	)
	cache.Register(mgr, c.History.csrfCache, marshalCSRFEntries, unmarshalCSRFEntries)

	// media.musicProviders cache (engine-managed, Long, session-tagged).
	c.Media.musicProvidersCache = cache.NewCache[map[string]MusicProvider](
		cache.Config{Name: "media.musicProviders", TTL: classes.TTL(cache.ClassLong), Tags: []string{"session"}},
		func(ctx context.Context, _ string) (map[string]MusicProvider, error) {
			return c.Media.fetchMusicProviders(ctx)
		},
	)
	cache.Register(mgr, c.Media.musicProvidersCache, marshalMusicProvidersEntries, unmarshalMusicProvidersEntries)

	// media.deviceVolumes cache (engine-managed, Realtime, no persistence).
	c.Media.deviceVolumesCache = cache.NewCache[map[string]*VolumeState](
		cache.Config{Name: "media.deviceVolumes", TTL: classes.TTL(cache.ClassRealtime)},
		func(ctx context.Context, _ string) (map[string]*VolumeState, error) {
			return c.Media.fetchDeviceVolumes(ctx)
		},
	)
	cache.Register(mgr, c.Media.deviceVolumesCache, nil, nil)

	// media.mediaStates cache (engine-managed, Realtime, no persistence).
	c.Media.mediaStatesCache = cache.NewCache[map[string]map[string]any](
		cache.Config{Name: "media.mediaStates", TTL: classes.TTL(cache.ClassRealtime)},
		func(ctx context.Context, _ string) (map[string]map[string]any, error) {
			return c.Media.fetchMediaStates(ctx)
		},
	)
	cache.Register(mgr, c.Media.mediaStatesCache, nil, nil)

	// dnd.status cache (engine-managed, Short class).
	c.DnD.statusCache = cache.NewCache[map[string]*DeviceSensor](
		cache.Config{Name: "dnd.status", TTL: classes.TTL(cache.ClassShort)},
		func(ctx context.Context, _ string) (map[string]*DeviceSensor, error) {
			return c.DnD.fetchStatus(ctx)
		},
	)
	cache.Register(mgr, c.DnD.statusCache, marshalDnDEntries, unmarshalDnDEntries)

	// lists.items cache (engine-managed, Short class, no persistence —
	// *List carries back-refs into the service).
	c.Lists.itemsCache = cache.NewCache[[]*List](
		cache.Config{Name: "lists.items", TTL: classes.TTL(cache.ClassShort)},
		func(ctx context.Context, _ string) ([]*List, error) {
			return c.Lists.fetchItems(ctx)
		},
	)
	cache.Register(mgr, c.Lists.itemsCache, nil, nil)

	// schedules.notifications cache: shared across Alarms/Reminders/Timers
	// and the Devices.Refresh sensors hook. Account-wide endpoint, sentinel
	// key. Short class.
	c.schedulesCache = cache.NewCache[*schedulesFetchResult](
		cache.Config{Name: "schedules.notifications", TTL: classes.TTL(cache.ClassShort)},
		func(ctx context.Context, _ string) (*schedulesFetchResult, error) {
			return fetchSchedules(ctx, wr)
		},
	)
	cache.Register(mgr, c.schedulesCache, nil, nil)
	schedulesGet := func(ctx context.Context) (*schedulesFetchResult, error) {
		got, _, err := c.schedulesCache.Get(ctx, "")
		return got, err
	}
	c.Alarms.schedulesFetch = schedulesGet
	c.Reminders.schedulesFetch = schedulesGet
	c.Timers.schedulesFetch = schedulesGet

	// auth.customerID cache (engine-managed, Long, session-tagged, no loader).
	c.Auth.customerIDCache = cache.NewCache[string](
		cache.Config{Name: "auth.customerID", TTL: classes.TTL(cache.ClassLong), Tags: []string{"session"}},
		nil,
	)
	cache.Register(mgr, c.Auth.customerIDCache, marshalCustomerIDEntries, unmarshalCustomerIDEntries)

	// devices.list cache (engine-managed, Long, no persistence — *Device
	// holds pointer back-refs to smartHome and service that don't
	// round-trip cleanly through JSON).
	c.Devices.listCache = cache.NewCache[map[string]*Device](
		cache.Config{Name: "devices.list", TTL: classes.TTL(cache.ClassLong)},
		func(ctx context.Context, _ string) (map[string]*Device, error) {
			return c.Devices.fetchAndEnrichDevices(ctx)
		},
	)
	cache.Register(mgr, c.Devices.listCache, nil, nil)

	if cfg.pushCacheBridge {
		c.pushBridgeOnce = &sync.Once{}
		// Hook into Auth.broadcast so the bridge starts after the first
		// session is set (Subscribe requires a session).
		c.Auth.postBroadcastHook = func() {
			c.pushBridgeOnce.Do(c.startPushCacheBridge)
		}
	}

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
		// schedules fetcher. Routes through the engine cache so
		// Alarms/Reminders/Timers and this hook share one fetch.
		sched, sErr := schedulesGet(ctx)
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

	if mgr.Persistence() != nil {
		if err := mgr.Restore(context.Background()); err != nil {
			wr.Logger(context.Background()).WarnContext(context.Background(),
				"amazondevices: cache Restore failed; continuing with empty caches",
				"err", err)
		}
	}
	return c, nil
}

// SnapshotCaches persists every registered cache via the configured
// Persistence adapter. Returns nil when no adapter is configured.
// Callers typically invoke this on shutdown or from a periodic ticker.
func (c *Client) SnapshotCaches(ctx context.Context) error {
	return c.cacheManager.Snapshot(ctx)
}
