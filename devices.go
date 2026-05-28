package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// DevicesService discovers Amazon devices associated with the account and
// enriches them with GraphQL endpoint metadata.
type DevicesService struct {
	wrapper *httpwrapper.Wrapper

	// mu guards session, cache, endpoints, timestamps, and defaultDevice.
	mu              sync.RWMutex
	session         *Session
	cache           map[string]*Device // serial -> device
	endpoints       map[string]string  // endpoint ID -> serial
	lastDailyFetch  time.Time
	lastEndpointFix time.Time
	defaultDevice   *Device

	// refreshGroup dedupes concurrent refresh calls. The key is a constant
	// ("refresh") so at most one refresh is in flight per DevicesService.
	refreshGroup singleflight.Group

	// sensorsHook, if non-nil, is invoked at the end of refresh to enrich
	// devices with sensor state and DnD. Wired by NewClient; nil when the
	// service is constructed bare in tests. Failures from the hook are
	// logged at WARN level and do NOT fail the refresh.
	sensorsHook func(ctx context.Context, devices map[string]*Device, endpoints map[string]string) error

	// smartHome is the per-Client write helper; non-nil after NewClient
	// runs. Devices added to the cache get this back-reference so their
	// typed control accessors can find it.
	smartHome *smartHomeClient
}

// RefreshOption tunes DevicesService.Refresh behavior. Returned by
// functional-option constructors below; pass to Refresh via variadic
// arguments.
type RefreshOption interface {
	apply(*refreshConfig)
}

// refreshConfig is the internal options bag accumulated from RefreshOption
// values. Defaults are zero-valued.
type refreshConfig struct {
	skipSensors bool
}

type refreshOptionFunc func(*refreshConfig)

func (f refreshOptionFunc) apply(cfg *refreshConfig) { f(cfg) }

// WithoutSensors instructs Refresh to skip the sensor-enrichment step.
// Useful for fast-path callers that don't need Device.Sensors populated.
// Online flags are then taken from the base /api/devices-v2/device
// response only.
func WithoutSensors() RefreshOption {
	return refreshOptionFunc(func(cfg *refreshConfig) { cfg.skipSensors = true })
}

// setSession swaps in a fresh *Session pointer. Invoked by AuthService
// after Login / Restore / refreshAccessToken.
func (s *DevicesService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// snapshotSession returns the current *Session pointer under the read
// lock. Callers may dereference the returned pointer freely — AuthService
// always swaps in a fresh pointer rather than mutating in place.
func (s *DevicesService) snapshotSession() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

// fetchBase issues GET /api/devices-v2/device and returns the parsed,
// filtered device map keyed by serial number. It does not populate the
// cache — callers (List, Refresh) do that.
func (s *DevicesService) fetchBase(ctx context.Context, sitePrefix string) (map[string]*Device, error) {
	url := fmt.Sprintf("%s%s", trimTrailingSlash(sitePrefix), amzconst.URIDevices)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{Method: http.MethodGet, URL: url})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, err
	}

	rawDevs, _ := data["devices"].([]any)
	out := map[string]*Device{}
	serialToType := map[string]string{}

	for _, di := range rawDevs {
		raw, _ := di.(map[string]any)
		if raw == nil {
			continue
		}
		dt, _ := stringOf(raw["deviceType"])
		if _, ignore := amzconst.DeviceTypesToIgnore[dt]; ignore {
			continue
		}

		caps := toStringSlice(raw["capabilities"])
		// Previously this skipped any device without MICROPHONE capability,
		// which excluded virtual app-emulation devices (e.g. our own
		// AioAmazonDevices registrations of type A2IVLV5VM2W81). Keep them
		// in the list so callers can see and manage their own session
		// devices alongside real Echo speakers.

		serial, _ := stringOf(raw["serialNumber"])
		if serial == "" {
			continue
		}

		clusterRaw := toStringSlice(raw["clusterMembers"])
		if len(clusterRaw) == 0 {
			clusterRaw = []string{serial}
		}
		cluster := make(map[string]string, len(clusterRaw))
		for _, m := range clusterRaw {
			cluster[m] = ""
		}

		ownerID, _ := stringOf(raw["deviceOwnerCustomerId"])
		household := false
		if sess := s.snapshotSession(); sess != nil && ownerID != "" && ownerID == sess.AccountCustomerID {
			household = true
		}

		var swVer string
		if sliceContains(caps, "SUPPORTS_SOFTWARE_VERSION") {
			swVer, _ = stringOf(raw["softwareVersion"])
		}

		accountName, _ := stringOf(raw["accountName"])
		deviceFamily, _ := stringOf(raw["deviceFamily"])
		model, _ := stringOf(raw["deviceTypeFriendlyName"])
		d := &Device{
			AccountName:           accountName,
			SerialNumber:          serial,
			DeviceType:            dt,
			DeviceFamily:          deviceFamily,
			DeviceOwnerCustomerID: ownerID,
			HouseholdDevice:       household,
			Online:                boolOf(raw["online"]),
			Capabilities:          caps,
			ClusterMembers:        cluster,
			Model:                 model,
			SoftwareVersion:       swVer,
			Sensors:               map[string]*DeviceSensor{},
			SchedulesSupported:    sliceContains(caps, "REMINDERS") || sliceContains(caps, "TIMERS_AND_ALARMS"),
			MediaPlayerSupported:  sliceContains(caps, "AUDIO_PLAYER"),
			controller:            s.smartHome,
		}
		out[serial] = d
		serialToType[serial] = dt
	}

	for _, d := range out {
		for member := range d.ClusterMembers {
			d.ClusterMembers[member] = serialToType[member]
		}
	}
	return out, nil
}

func trimTrailingSlash(s string) string {
	if len(s) > 0 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}

func toStringSlice(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sliceContains(haystack []string, needle string) bool {
	for _, x := range haystack {
		if x == needle {
			return true
		}
	}
	return false
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}
