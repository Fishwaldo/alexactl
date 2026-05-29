package amazondevices

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Fishwaldo/goamazondevices/internal/alexagraphql"
	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Khan/genqlient/graphql"
)

type (
	listEndpointsEndpoint = alexagraphql.DevicesEndpoint
)

// List returns all enrichable Amazon devices for the account, including
// Air Quality Monitors. The result is sorted by serial number and is a
// fresh slice; mutations don't affect the cache.
//
// Cached for the Long-class TTL; call Refresh to force a re-fetch.
func (s *DevicesService) List(ctx context.Context) ([]*Device, error) {
	devices, _, err := s.listCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]*Device, 0, len(devices))
	for _, d := range devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SerialNumber < out[j].SerialNumber })
	return out, nil
}

// Get returns the device with the given serial. Refreshes the cache if
// stale. Returns nil and an error wrapping ErrCannotRetrieveData if not
// found after a refresh.
func (s *DevicesService) Get(ctx context.Context, serial string) (*Device, error) {
	devices, _, err := s.listCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	d, ok := devices[serial]
	if !ok {
		return nil, fmt.Errorf("%w: no such device: %s", ErrCannotRetrieveData, serial)
	}
	return d, nil
}

// getCached returns the device from the cache without triggering a
// refresh. Used by Phase 6 schedule services to resolve DeviceType
// from a SerialNumber during Create. Returns nil if not present or if
// the cache is empty.
func (s *DevicesService) getCached(serial string) *Device {
	if s.listCache == nil {
		return nil
	}
	devices, hit, _ := s.listCache.Get(context.Background(), "")
	if !hit {
		return nil
	}
	return devices[serial]
}

// setCacheForTest replaces the cache atomically. ONLY used by tests
// in the same package to seed device state without going through
// Refresh.
func (s *DevicesService) setCacheForTest(c map[string]*Device) {
	s.listCache.Set("", c)
}

// Default returns the user-set default device, or the first online device
// (by sorted serial number) if not set. Iteration is deterministic across
// calls because Go's map range order is randomized.
func (s *DevicesService) Default(ctx context.Context) (*Device, error) {
	devices, _, err := s.listCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	def := s.defaultDevice
	s.mu.RUnlock()
	if def != nil {
		return def, nil
	}
	serials := make([]string, 0, len(devices))
	for serial := range devices {
		serials = append(serials, serial)
	}
	sort.Strings(serials)
	for _, serial := range serials {
		if d := devices[serial]; d.Online {
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
	devices, hit, _ := s.listCache.Get(context.Background(), "")
	if !hit {
		return fmt.Errorf("%w: device cache empty; call Refresh first", ErrDeviceNotFound)
	}
	if _, ok := devices[d.SerialNumber]; !ok {
		return fmt.Errorf("%w: device %q not in cache", ErrDeviceNotFound, d.SerialNumber)
	}
	s.mu.Lock()
	s.defaultDevice = d
	s.mu.Unlock()
	return nil
}

// Refresh forces a re-fetch of the device list, including the GraphQL
// endpoint enrichment. WithoutSensors bypasses the cache loader (which
// always runs sensors) and writes the no-sensors result straight into
// the cache.
func (s *DevicesService) Refresh(ctx context.Context, opts ...RefreshOption) error {
	cfg := refreshConfig{}
	for _, o := range opts {
		o.apply(&cfg)
	}
	if cfg.skipSensors {
		_, err := s.refreshSkipSensors(ctx)
		return err
	}
	// Default path: force fresh by invalidating, then Get to fire the
	// loader (which runs sensors enrichment).
	s.listCache.InvalidateAll()
	_, _, err := s.listCache.Get(ctx, "")
	return err
}

// refreshSkipSensors is the WithoutSensors fast-path. Fetches base +
// endpoints without sensors enrichment, writes the result into
// listCache, and returns the map.
func (s *DevicesService) refreshSkipSensors(ctx context.Context) (map[string]*Device, error) {
	sitePrefix := s.alexaHost()
	base, err := s.fetchBase(ctx, sitePrefix)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := s.fetchEndpoints(ctx, sitePrefix, base); err != nil {
		return nil, err
	}
	s.listCache.Set("", base)
	return base, nil
}

// fetchAndEnrichDevices is the loader for the devices.list cache. Runs
// fetchBase + fetchEndpoints + sensorsHook (when set) and returns the
// enriched device map.
func (s *DevicesService) fetchAndEnrichDevices(ctx context.Context) (map[string]*Device, error) {
	s.wrapper.Logger(ctx).DebugContext(ctx, "amazondevices: refreshing devices")
	sitePrefix := s.alexaHost()

	base, err := s.fetchBase(ctx, sitePrefix)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoints, err := s.fetchEndpoints(ctx, sitePrefix, base)
	if err != nil {
		return nil, err
	}

	// Sensors enrichment — best-effort. Failures log WARN and do NOT
	// fail the load.
	if s.sensorsHook != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if hookErr := s.sensorsHook(ctx, base, endpoints); hookErr != nil {
			s.wrapper.Logger(ctx).WarnContext(ctx,
				"amazondevices: sensor enrichment failed; continuing without it",
				"err", hookErr)
		}
		// Derive AlexaInterfaces from each device's populated sensor
		// keys and merge into d.AlexaInterfaces.
		for _, d := range base {
			if derived := inferAlexaInterfacesFromSensors(d.Sensors); len(derived) > 0 {
				d.AlexaInterfaces = mergeAlexaInterfaces(d.AlexaInterfaces, derived)
			}
		}
	}
	return base, nil
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
	client := graphql.NewClient(url, alexagraphql.NewDoer(s.wrapper))
	resp, err := alexagraphql.ListEndpoints(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	endpoints := map[string]string{}
	if resp == nil || resp.AllDevices == nil {
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

	for _, ep := range resp.AllDevices.Endpoints {
		if ep == nil {
			continue
		}
		endpointID := ep.EndpointId
		graphqlSerial := ""
		if ep.SerialNumber != nil && ep.SerialNumber.Value != nil {
			graphqlSerial = ep.SerialNumber.Value.Text
		}
		// entityId is the bare UUID portion of endpointId. We use it as
		// the cache-key fallback for skill-discovered smart-home endpoints
		// (TP-Link, Hue, Sensibo, etc.) which Amazon returns with
		// serialNumber=null — those devices have no Amazon-issued serial.
		entityID := ""
		if ep.LegacyIdentifiers != nil && ep.LegacyIdentifiers.ChrsIdentifier != nil {
			entityID = ep.LegacyIdentifiers.ChrsIdentifier.EntityId
		}
		friendlyName := ""
		if ep.FriendlyNameObject != nil && ep.FriendlyNameObject.Value != nil {
			friendlyName = ep.FriendlyNameObject.Value.Text
		}

		// Detect AQM endpoints by displayCategories — these are NOT in
		// the base /api/devices-v2/device list, so we add them under
		// the GraphQL serial. Everything else is treated as an
		// enrichment of an existing base entry.
		if endpointIsAQMTyped(ep) {
			if graphqlSerial == "" {
				continue
			}
			endpoints[endpointID] = graphqlSerial
			if _, already := devs[graphqlSerial]; already {
				continue
			}
			deviceType := ""
			if ep.LegacyIdentifiers != nil && ep.LegacyIdentifiers.DmsIdentifier != nil &&
				ep.LegacyIdentifiers.DmsIdentifier.DeviceType != nil &&
				ep.LegacyIdentifiers.DmsIdentifier.DeviceType.Value != nil {
				deviceType = ep.LegacyIdentifiers.DmsIdentifier.DeviceType.Value.Text
			}
			ownerID := ""
			household := false
			if sess := s.snapshotSession(); sess != nil && sess.AccountCustomerID != "" {
				ownerID = sess.AccountCustomerID
				household = true
			}
			manufacturer := ""
			if ep.Manufacturer != nil && ep.Manufacturer.Value != nil {
				manufacturer = ep.Manufacturer.Value.Text
			}
			softwareVersion := ""
			if ep.SoftwareVersion != nil && ep.SoftwareVersion.Value != nil {
				softwareVersion = ep.SoftwareVersion.Value.Text
			}
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
				SoftwareVersion:       softwareVersion,
				EndpointID:            endpointID,
				Sensors:               map[string]*DeviceSensor{},
				// Phase 4a: AQM is its own kind regardless of DisplayCategory.
				Kind:              KindAQM,
				DisplayCategories: extractDisplayCategories(ep),
				AlexaInterfaces:   extractAlexaInterfaces(ep),
				Operations:        extractEndpointOperations(ep),
				controller:        s.smartHome,
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
			// Phase 4a Task 4: surface smart-home endpoints as their own
			// *Device. Echos always match through the GraphQL-serial or
			// friendlyName fallback above, so this branch only fires for
			// non-Echo endpoints (lights, switches, thermostats, locks,
			// cameras, sensors).
			//
			// Cache-key fallback: skill-discovered devices (TP-Link, Hue,
			// Sensibo, Ring, etc.) typically have serialNumber=null in
			// Amazon's response. For those, fall back to chrsIdentifier.
			// entityId (the bare UUID portion of endpointId) so they
			// surface in List output instead of being silently dropped.
			cacheKey := graphqlSerial
			if cacheKey == "" {
				cacheKey = entityID
			}
			if cacheKey == "" {
				continue // truly nothing to key on; drop
			}
			if _, already := devs[cacheKey]; already {
				continue // existing device already enriched
			}
			ownerID := ""
			household := false
			if sess := s.snapshotSession(); sess != nil && sess.AccountCustomerID != "" {
				ownerID = sess.AccountCustomerID
				household = true
			}
			manufacturer := ""
			if ep.Manufacturer != nil && ep.Manufacturer.Value != nil {
				manufacturer = ep.Manufacturer.Value.Text
			}
			modelText := ""
			if ep.Model != nil && ep.Model.Value != nil {
				modelText = ep.Model.Value.Text
			}
			model, hw := parseDeviceDetails(modelText)
			if model == "" {
				model = modelText
			}
			softwareVersion := ""
			if ep.SoftwareVersion != nil && ep.SoftwareVersion.Value != nil {
				softwareVersion = ep.SoftwareVersion.Value.Text
			}
			primaryCategory := ""
			if ep.DisplayCategories != nil && ep.DisplayCategories.Primary != nil &&
				ep.DisplayCategories.Primary.Value != nil {
				primaryCategory = *ep.DisplayCategories.Primary.Value
			}
			devs[cacheKey] = &Device{
				AccountName:           friendlyName,
				SerialNumber:          cacheKey,
				EntityID:              entityID,
				DeviceFamily:          primaryCategory,
				DeviceOwnerCustomerID: ownerID,
				HouseholdDevice:       household,
				Online:                true,
				ClusterMembers:        map[string]string{cacheKey: ""},
				Manufacturer:          manufacturer,
				Model:                 model,
				HardwareVersion:       hw,
				SoftwareVersion:       softwareVersion,
				EndpointID:            endpointID,
				Sensors:               map[string]*DeviceSensor{},

				Kind:              Kind(amzconst.DisplayCategoryToKind(primaryCategory)),
				DisplayCategories: extractDisplayCategories(ep),
				AlexaInterfaces:   extractAlexaInterfaces(ep),
				Operations:        extractEndpointOperations(ep),
				controller:        s.smartHome,
			}
			endpoints[endpointID] = cacheKey
			continue
		}
		endpoints[endpointID] = baseSerial
		d := devs[baseSerial]
		d.EndpointID = endpointID
		if ep.LegacyIdentifiers != nil && ep.LegacyIdentifiers.ChrsIdentifier != nil {
			d.EntityID = ep.LegacyIdentifiers.ChrsIdentifier.EntityId
		}

		// Populate new Phase 4a fields.
		d.Kind = KindEcho
		d.DisplayCategories = extractDisplayCategories(ep)
		d.AlexaInterfaces = extractAlexaInterfaces(ep)
		d.Operations = extractEndpointOperations(ep)
		d.controller = s.smartHome

		modelText := ""
		if ep.Model != nil && ep.Model.Value != nil {
			modelText = ep.Model.Value.Text
		}
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

		if d.Manufacturer == "" && ep.Manufacturer != nil && ep.Manufacturer.Value != nil {
			d.Manufacturer = ep.Manufacturer.Value.Text
		}
	}

	return endpoints, nil
}

// endpointIsAQMTyped reports whether the given typed endpoint entry's
// displayCategories include AIR_QUALITY_MONITOR (either primary or all).
func endpointIsAQMTyped(ep *listEndpointsEndpoint) bool {
	if ep == nil || ep.DisplayCategories == nil {
		return false
	}
	if ep.DisplayCategories.Primary != nil && ep.DisplayCategories.Primary.Value != nil &&
		*ep.DisplayCategories.Primary.Value == "AIR_QUALITY_MONITOR" {
		return true
	}
	for _, c := range ep.DisplayCategories.All {
		if c != nil && c.Value != nil && *c.Value == "AIR_QUALITY_MONITOR" {
			return true
		}
	}
	return false
}

// extractDisplayCategories pulls the .all[].value strings from the typed
// endpoint, in stable order. Returns nil when no categories are present.
func extractDisplayCategories(ep *listEndpointsEndpoint) []string {
	if ep == nil || ep.DisplayCategories == nil {
		return nil
	}
	out := make([]string, 0, len(ep.DisplayCategories.All))
	for _, c := range ep.DisplayCategories.All {
		if c != nil && c.Value != nil && *c.Value != "" {
			out = append(out, *c.Value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// extractAlexaInterfaces pulls the capabilities list from the typed
// endpoint's alexaEnabledMetadata, if present.
//
// CAVEAT: For most third-party smart-home endpoints (skill-discovered
// devices: TP-Link, Hue, Sensibo, Ring, etc.) alexaEnabledMetadata is
// nil, so this returns nil. For first-party Echos it returns Echo-skill
// capability strings (MUSIC_SKILL, BT_PAIRING_FLOW_V2, ...) — NOT
// Alexa.*Controller interface names. The authoritative source of which
// Alexa.*Controllers a device exposes is its sensor data; see
// inferAlexaInterfacesFromSensors which runs after the sensors-hook
// pass during refresh and after applyInspectResponse during Inspect.
func extractAlexaInterfaces(ep *listEndpointsEndpoint) []string {
	if ep == nil || ep.AlexaEnabledMetadata == nil {
		return nil
	}
	if len(ep.AlexaEnabledMetadata.Capabilities) == 0 {
		return nil
	}
	out := make([]string, 0, len(ep.AlexaEnabledMetadata.Capabilities))
	out = append(out, ep.AlexaEnabledMetadata.Capabilities...)
	return out
}

// inferAlexaInterfacesFromSensors returns the set of Alexa.*Controller
// interface strings implied by a device's populated Sensors keys.
//
// Amazon's alexaEnabledMetadata.capabilities does NOT carry the
// Alexa.*Controller strings for third-party smart-home devices — it's
// nil for them and a list of Echo-skill capabilities for first-party
// Alexa devices. The presence of a sensor key (e.g. "powerState",
// "brightness", "lockState") proves the device speaks the
// corresponding interface, so we derive AlexaInterfaces from the
// observed sensors. This is what makes the typed accessors
// (d.Power(), d.Brightness(), d.Lock(), ...) work against
// skill-discovered devices.
//
// Range / Toggle / Mode (generic controllers) are intentionally
// excluded for now — Phase 4e will add InterfaceRangeController etc.
// when the wire payloads are HAR-captured and the typed accessors
// are wired up.
func inferAlexaInterfacesFromSensors(sensors map[string]*DeviceSensor) []string {
	if len(sensors) == 0 {
		return nil
	}
	var out []string
	if _, ok := sensors["powerState"]; ok {
		out = append(out, amzconst.InterfacePowerController)
	}
	if _, ok := sensors["brightness"]; ok {
		out = append(out, amzconst.InterfaceBrightnessController)
	}
	if _, ok := sensors["color"]; ok {
		out = append(out, amzconst.InterfaceColorController)
	}
	if _, ok := sensors["colorTemperature"]; ok {
		out = append(out, amzconst.InterfaceColorTemperatureController)
	}
	if _, ok := sensors["lockState"]; ok {
		out = append(out, amzconst.InterfaceLockController)
	}
	if _, ok := sensors["targetSetpoint"]; ok {
		out = append(out, amzconst.InterfaceThermostatController)
	} else if _, ok := sensors["thermostatMode"]; ok {
		out = append(out, amzconst.InterfaceThermostatController)
	}

	// Mode / Range / Toggle have variable-keyed sensors — canonical
	// instance names (fanSpeed, humidity, ...) AND "<feature>:<instance>"
	// prefix fallbacks. Iterate once and de-dup via seen flags.
	hasMode, hasRange, hasToggle := false, false, false
	for key := range sensors {
		switch {
		case key == "fanSpeed", key == "fanDirection":
			hasMode = true
		case key == "humidity":
			hasRange = true
		case strings.HasPrefix(key, "mode:"):
			hasMode = true
		case strings.HasPrefix(key, "range:"):
			hasRange = true
		case strings.HasPrefix(key, "toggle:"):
			hasToggle = true
		}
	}
	if hasMode {
		out = append(out, amzconst.InterfaceModeController)
	}
	if hasRange {
		out = append(out, amzconst.InterfaceRangeController)
	}
	if hasToggle {
		out = append(out, amzconst.InterfaceToggleController)
	}
	return out
}

// mergeAlexaInterfaces unions two interface-name slices, preserving
// order from `existing` first then appending unseen entries from
// `derived`. Returns nil only when both inputs are empty.
func mergeAlexaInterfaces(existing, derived []string) []string {
	if len(existing) == 0 {
		if len(derived) == 0 {
			return nil
		}
		return derived
	}
	seen := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		seen[s] = struct{}{}
	}
	out := existing
	for _, d := range derived {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// extractEndpointOperations pulls the .operations[].name list from the
// typed endpoint, in stable order. Returns nil when no operations are
// reported.
func extractEndpointOperations(ep *listEndpointsEndpoint) []string {
	if ep == nil {
		return nil
	}
	out := make([]string, 0, len(ep.Operations))
	for _, op := range ep.Operations {
		if op != nil && op.Name != "" {
			out = append(out, op.Name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Inspect issues a per-endpoint deep query (endpointAllFeatures) for
// the given serial and updates the cached *Device in place with fresh
// feature + property + operation + configuration data. Returns the
// updated device. Use this when a caller wants the full state for one
// device without paying for a list-wide Refresh.
//
// If serial is not in the cache, Inspect returns an error wrapping
// ErrDeviceNotFound; the caller should call Refresh first.
func (s *DevicesService) Inspect(ctx context.Context, serial string) (*Device, error) {
	devices, hit, _ := s.listCache.Get(ctx, "")
	if !hit {
		return nil, fmt.Errorf("%w: device cache empty; call Refresh first", ErrDeviceNotFound)
	}
	d, ok := devices[serial]
	if !ok {
		return nil, fmt.Errorf("%w: device %q not in cache", ErrDeviceNotFound, serial)
	}
	if d.EndpointID == "" {
		return nil, fmt.Errorf("%w: device %q has no EndpointID (run Refresh first)", ErrCannotRetrieveData, serial)
	}

	sitePrefix := s.alexaHost()
	url := fmt.Sprintf("%s%s", trimTrailingSlash(sitePrefix), amzconst.URINexusGraphQL)
	client := graphql.NewClient(url, alexagraphql.NewDoer(s.wrapper))
	low := alexagraphql.LatencyToleranceValueLow
	resp, err := alexagraphql.EndpointAllFeatures(ctx, client, d.EndpointID, &low)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp == nil || resp.Endpoint == nil {
		return nil, fmt.Errorf("%w: empty endpoint response", ErrCannotRetrieveData)
	}

	s.applyInspectResponse(ctx, d, resp.Endpoint)
	return d, nil
}

// applyInspectResponse merges the deep-query response into the cached
// *Device in place. Acquires the service write-lock so concurrent
// readers see a consistent snapshot.
func (s *DevicesService) applyInspectResponse(_ context.Context, d *Device, ep *alexagraphql.EndpointAllFeaturesEndpoint) {
	s.mu.Lock()
	if ep.FriendlyNameObject != nil && ep.FriendlyNameObject.Value != nil {
		if name := ep.FriendlyNameObject.Value.Text; name != "" {
			d.AccountName = name
		}
	}
	if ep.DisplayCategories != nil {
		d.DisplayCategories = nil
		for _, c := range ep.DisplayCategories.All {
			if c != nil && c.Value != nil && *c.Value != "" {
				d.DisplayCategories = append(d.DisplayCategories, *c.Value)
			}
		}
		// Preserve sticky kinds: Echos and AQMs are tagged by the base-list
		// or AQM-detection branches in fetchEndpoints, not by DisplayCategory.
		// Re-deriving from DisplayCategory.primary here would flip an Echo
		// (primary "SPEAKER") to KindOther.
		if d.Kind != KindEcho && d.Kind != KindAQM &&
			ep.DisplayCategories.Primary != nil && ep.DisplayCategories.Primary.Value != nil {
			d.Kind = Kind(amzconst.DisplayCategoryToKind(*ep.DisplayCategories.Primary.Value))
		}
	}
	if ep.Operations != nil {
		d.Operations = nil
		for _, op := range ep.Operations {
			if op != nil && op.Name != "" {
				d.Operations = append(d.Operations, op.Name)
			}
		}
	}

	// Parse property data directly from the deep-query response — no
	// second network call. Replaces the prior sensorsHook delegation
	// that issued a second getEndpointState POST.
	if sensors := parseAllFeaturesEndpoint(ep, d); len(sensors) > 0 {
		d.Sensors = sensors
		// Online reflects the reachability property if present, same
		// rule SensorsService.Update uses for the bulk path.
		if r := sensors["reachability"]; r != nil {
			d.Online = r.Value == "OK"
		}
		// Same AlexaInterfaces derivation as the bulk refresh path —
		// see inferAlexaInterfacesFromSensors. Inspect re-derives
		// because the deep query returns richer feature data than the
		// listEndpoints pass; this keeps the typed-accessor gating in
		// sync with the freshest snapshot.
		if derived := inferAlexaInterfacesFromSensors(sensors); len(derived) > 0 {
			d.AlexaInterfaces = mergeAlexaInterfaces(d.AlexaInterfaces, derived)
		}
	}
	s.mu.Unlock()
}
