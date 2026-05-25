package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// List returns all enrichable Amazon devices for the account, including
// Air Quality Monitors. The result is sorted by serial number and is a
// fresh slice; mutations don't affect the cache.
//
// Caches results for ~24h; call Refresh to force a re-fetch.
func (s *DevicesService) List(ctx context.Context) ([]*Device, error) {
	if err := s.refreshIfStale(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Device, 0, len(s.cache))
	for _, d := range s.cache {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SerialNumber < out[j].SerialNumber })
	return out, nil
}

// Get returns the device with the given serial. Refreshes the cache if
// stale. Returns nil and an error wrapping ErrCannotRetrieveData if not
// found after a refresh.
func (s *DevicesService) Get(ctx context.Context, serial string) (*Device, error) {
	if err := s.refreshIfStale(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.cache[serial]
	if !ok {
		return nil, fmt.Errorf("%w: no such device: %s", ErrCannotRetrieveData, serial)
	}
	return d, nil
}

// Default returns the user-set default device, or the first online device
// (by sorted serial number) if not set. Iteration is deterministic across
// calls because Go's map range order is randomized.
func (s *DevicesService) Default(ctx context.Context) (*Device, error) {
	if err := s.refreshIfStale(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.defaultDevice != nil {
		return s.defaultDevice, nil
	}
	serials := make([]string, 0, len(s.cache))
	for serial := range s.cache {
		serials = append(serials, serial)
	}
	sort.Strings(serials)
	for _, serial := range serials {
		if d := s.cache[serial]; d.Online {
			return d, nil
		}
	}
	return nil, fmt.Errorf("%w: no online devices", ErrCannotRetrieveData)
}

// SetDefault overrides the default device. The supplied *Device must be
// one previously returned by List/Get/Default so it is present in the
// service's cache; otherwise this returns an error wrapping
// ErrDeviceNotFound. A nil *Device is rejected with a separate error.
//
// Breaking change vs v0.1.0: returns an error.
func (s *DevicesService) SetDefault(d *Device) error {
	if d == nil {
		return fmt.Errorf("amazondevices: SetDefault: nil device")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cache[d.SerialNumber]; !ok {
		return fmt.Errorf("%w: device %q not in cache", ErrDeviceNotFound, d.SerialNumber)
	}
	s.defaultDevice = d
	return nil
}

// Refresh forces a re-fetch of the device list, including the GraphQL
// endpoint enrichment.
func (s *DevicesService) Refresh(ctx context.Context, opts ...RefreshOption) error {
	cfg := refreshConfig{}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return s.refresh(ctx, cfg)
}

// refreshIfStale calls refresh if the cache is older than the daily window.
// Concurrent callers are coalesced via singleflight: only one HTTP refresh
// happens; every caller sees the same result.
func (s *DevicesService) refreshIfStale(ctx context.Context) error {
	s.mu.RLock()
	stale := s.cache == nil || time.Since(s.lastDailyFetch) >= 24*time.Hour
	s.mu.RUnlock()
	if !stale {
		return nil
	}
	_, err, _ := s.refreshGroup.Do("refresh", func() (any, error) {
		return nil, s.refresh(ctx, refreshConfig{})
	})
	return err
}

func (s *DevicesService) refresh(ctx context.Context, cfg refreshConfig) error {
	s.wrapper.Logger(ctx).DebugContext(ctx, "amazondevices: refreshing devices")
	sitePrefix := s.alexaHost()

	base, err := s.fetchBase(ctx, sitePrefix)
	if err != nil {
		return err
	}
	// Cheap insurance: if the caller cancelled between sub-fetches, bail
	// out before issuing the GraphQL request.
	if err := ctx.Err(); err != nil {
		return err
	}
	endpoints, err := s.fetchEndpoints(ctx, sitePrefix, base)
	if err != nil {
		return err
	}

	// Sensors enrichment — best-effort. Failures (network, malformed
	// GraphQL response) log a WARN and do NOT fail Refresh. Callers that
	// don't care about sensors can pass WithoutSensors() to skip the
	// extra HTTP roundtrip entirely.
	if !cfg.skipSensors && s.sensorsHook != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if hookErr := s.sensorsHook(ctx, base, endpoints); hookErr != nil {
			s.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: sensor enrichment failed; continuing without it",
				"err", hookErr)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = base
	s.endpoints = endpoints
	s.lastDailyFetch = time.Now()
	s.lastEndpointFix = time.Now()
	return nil
}

func (s *DevicesService) alexaHost() string {
	dom := s.wrapper.Domain()
	if dom == "" {
		dom = "com"
	}
	return fmt.Sprintf("https://alexa.amazon.%s", dom)
}

// fetchEndpoints calls the Nexus GraphQL endpoint and returns the
// endpointID → serialNumber map. Also enriches the entries in `devs` with
// model/manufacturer/endpoint/entity values, and inserts AQM devices that
// are not present in the base device list.
//
// AQMs are surfaced regardless of manufacturer. This diverges from
// upstream Python (aioamazondevices), which only surfaces Amazon-
// manufactured AQMs; the user explicitly chose to surface third-party
// AQMs too.
func (s *DevicesService) fetchEndpoints(
	ctx context.Context,
	sitePrefix string,
	devs map[string]*Device,
) (map[string]string, error) {
	url := fmt.Sprintf("%s%s", trimTrailingSlash(sitePrefix), amzconst.URINexusGraphQL)
	payload := map[string]any{
		"operationName": "getDevicesBaseData",
		"query":         amzconst.QueryDeviceData,
	}
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:       http.MethodPost,
		URL:          url,
		JSONBody:     payload,
		ExtraHeaders: http.Header{"User-Agent": []string{amzconst.UserAgentAmazon}},
	})
	if err != nil {
		return nil, err
	}
	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, err
	}

	endpoints := map[string]string{}
	dataNode, _ := data["data"].(map[string]any)
	if dataNode == nil {
		return endpoints, nil
	}

	all, _ := dataNode["allDevices"].(map[string]any)
	if all == nil {
		return endpoints, nil
	}
	// nameToSerial is a fallback lookup for the JOIN between GraphQL
	// endpoints and base /api/devices-v2/device entries. Amazon's two
	// APIs use different serial schemas — the base list returns
	// physical/registration serials like "GSM427065425023K" while
	// GraphQL's serialNumber.value.text is a MAC-derived or hashed ID
	// (e.g., "MTCJEJIYZYT7C"). When they don't match, we fall back to
	// friendlyName == accountName, which is unique enough in practice
	// (Alexa app prevents most name collisions). Built lazily — only
	// when we hit at least one serial-miss.
	var nameToSerial map[string]string
	lookupByName := func(friendlyName string) string {
		if friendlyName == "" {
			return ""
		}
		if nameToSerial == nil {
			nameToSerial = make(map[string]string, len(devs))
			for serial, dvc := range devs {
				if dvc.AccountName != "" {
					nameToSerial[dvc.AccountName] = serial
				}
			}
		}
		return nameToSerial[friendlyName]
	}

	for _, ei := range toAnySlice(all["endpoints"]) {
		ep, _ := ei.(map[string]any)
		if ep == nil {
			continue
		}
		graphqlSerial := digFirstString(ep, "serialNumber", "value", "text")
		endpointID, _ := stringOf(ep["endpointId"])
		friendlyName := digFirstString(ep, "friendlyNameObject", "value", "text")

		// Detect AQM endpoints by displayCategories — these are NOT in
		// the base /api/devices-v2/device list, so we add them under
		// the GraphQL serial. Everything else is treated as an
		// enrichment of an existing base entry.
		if endpointIsAQM(ep) {
			if graphqlSerial == "" {
				continue
			}
			endpoints[endpointID] = graphqlSerial
			if _, already := devs[graphqlSerial]; already {
				continue
			}
			deviceType := digFirstString(ep, "legacyIdentifiers", "dmsIdentifier", "deviceType", "value", "text")
			ownerID := ""
			household := false
			if sess := s.snapshotSession(); sess != nil && sess.AccountCustomerID != "" {
				ownerID = sess.AccountCustomerID
				household = true
			}
			manufacturer := digFirstString(ep, "manufacturer", "value", "text")
			devs[graphqlSerial] = &Device{
				AccountName:           friendlyName,
				SerialNumber:          graphqlSerial,
				DeviceType:            deviceType,
				DeviceFamily:          "AIR_QUALITY_MONITOR",
				DeviceOwnerCustomerID: ownerID,
				HouseholdDevice:       household,
				Online:                true,
				ClusterMembers:        map[string]string{graphqlSerial: amzconst.DeviceTypeAQM},
				Manufacturer:          manufacturer,
				Model:                 amzconst.DeviceTypeHardcodedMetadata[amzconst.DeviceTypeAQM].Model,
				SoftwareVersion:       digFirstString(ep, "softwareVersion", "value", "text"),
				EndpointID:            endpointID,
				Sensors:               map[string]*DeviceSensor{},
				Notifications:         map[string]*Schedule{},
			}
			continue
		}

		// Non-AQM endpoint — find the matching base entry. Try the
		// GraphQL serial first; on miss, fall back to friendlyName.
		baseSerial := ""
		if graphqlSerial != "" {
			if _, ok := devs[graphqlSerial]; ok {
				baseSerial = graphqlSerial
			}
		}
		if baseSerial == "" {
			baseSerial = lookupByName(friendlyName)
		}
		if baseSerial == "" {
			continue // no matching base device — drop
		}
		endpoints[endpointID] = baseSerial
		d := devs[baseSerial]
		d.EndpointID = endpointID
		d.EntityID = digFirstString(ep, "legacyIdentifiers", "chrsIdentifier", "entityId")

		modelText := digFirstString(ep, "model", "value", "text")
		model, hw := parseDeviceDetails(modelText)
		if model != "" {
			d.Model = model
			d.HardwareVersion = hw
		} else if hm, ok := amzconst.DeviceTypeHardcodedMetadata[d.DeviceType]; ok {
			d.Model = hm.Model
			if d.Manufacturer == "" {
				d.Manufacturer = hm.Manufacturer
			}
		}

		if d.Manufacturer == "" {
			d.Manufacturer = digFirstString(ep, "manufacturer", "value", "text")
		}
	}

	return endpoints, nil
}

// endpointIsAQM reports whether the given GraphQL endpoint entry's
// displayCategories include AIR_QUALITY_MONITOR (either primary or all).
func endpointIsAQM(ep map[string]any) bool {
	cats, _ := ep["displayCategories"].(map[string]any)
	if cats == nil {
		return false
	}
	if primary, _ := cats["primary"].(map[string]any); primary != nil {
		if v, _ := stringOf(primary["value"]); v == "AIR_QUALITY_MONITOR" {
			return true
		}
	}
	for _, ai := range toAnySlice(cats["all"]) {
		entry, _ := ai.(map[string]any)
		if entry == nil {
			continue
		}
		if v, _ := stringOf(entry["value"]); v == "AIR_QUALITY_MONITOR" {
			return true
		}
	}
	return false
}

func digFirstString(m map[string]any, keys ...string) string {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			if s, ok := cur[k].(string); ok {
				return s
			}
			return ""
		}
		nxt, ok := cur[k].(map[string]any)
		if !ok {
			return ""
		}
		cur = nxt
	}
	return ""
}

func toAnySlice(v any) []any {
	a, _ := v.([]any)
	return a
}
