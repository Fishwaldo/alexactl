package amazondevices

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

//go:embed testdata/devices/endpoints_graphql.json
var endpointsGraphQLJSON []byte

//go:embed testdata/devices/endpoints_graphql_non_amazon_aqm.json
var endpointsGraphQLNonAmazonAQMJSON []byte

//go:embed testdata/devices/endpoints_graphql_serial_less.json
var endpointsGraphQLSerialLessJSON []byte

func TestDevices_Enrichment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "getDevicesBaseData", body["operationName"])
			assert.NotEmpty(t, body["query"])

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Use hostRewriter so the production code's https://alexa.amazon.com URLs
	// land on the httptest server.
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}
	ds.initListCacheForTest(nil)

	devs, err := ds.List(context.Background())
	require.NoError(t, err)

	// 2 voice devices from base + 1 AQM endpoint + 1 smart-plug endpoint = 4 total
	assert.Len(t, devs, 4)

	var kitchen *Device
	for _, d := range devs {
		if d.SerialNumber == "S-KITCHEN-001" {
			kitchen = d
			break
		}
	}
	require.NotNil(t, kitchen)
	assert.Equal(t, "Echo Dot", kitchen.Model)
	assert.Equal(t, "5th Gen", kitchen.HardwareVersion)
	assert.Equal(t, "Amazon", kitchen.Manufacturer)
	assert.Equal(t, "entity-kitchen", kitchen.EntityID)
	assert.Equal(t, "ep-kitchen", kitchen.EndpointID)

	var aqm *Device
	for _, d := range devs {
		if d.SerialNumber == "S-AQM-001" {
			aqm = d
			break
		}
	}
	require.NotNil(t, aqm)
	assert.Equal(t, "AIR_QUALITY_MONITOR", aqm.DeviceFamily)
	assert.Equal(t, "Air Quality Monitor", aqm.Model)
	assert.Equal(t, "Amazon", aqm.Manufacturer)
	assert.Equal(t, "999", aqm.SoftwareVersion)
}

// TestRefresh_Singleflight stands up an httptest server that counts how
// many base-data fetches it serves. With singleflight in place, 10
// concurrent List() calls against a stale cache should produce exactly 1
// underlying refresh.
func TestRefresh_Singleflight(t *testing.T) {
	var baseHits, graphHits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			atomic.AddInt32(&baseHits, 1)
			// Small stall to widen the race window so concurrent
			// callers actually pile up on the singleflight key.
			time.Sleep(50 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			atomic.AddInt32(&graphHits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}
	ds.initListCacheForTest(nil)

	const N = 10
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = ds.List(context.Background())
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		require.NoError(t, e, "goroutine %d failed", i)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&baseHits),
		"expected exactly one base-data fetch with singleflight")
	assert.Equal(t, int32(1), atomic.LoadInt32(&graphHits),
		"expected exactly one GraphQL fetch with singleflight")
}

// TestRefresh_SurfacesNonAmazonAQM verifies that AQMs with a non-Amazon
// manufacturer are still surfaced in List output, with the manufacturer
// field set to the parsed value (not hardcoded to "Amazon"). This
// behavior diverges from upstream Python (aioamazondevices) by design.
func TestRefresh_SurfacesNonAmazonAQM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			// Empty base list — the AQM is the only device.
			_, _ = w.Write([]byte(`{"devices": []}`))
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLNonAmazonAQMJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}
	ds.initListCacheForTest(nil)

	devs, err := ds.List(context.Background())
	require.NoError(t, err)

	var aqm *Device
	for _, d := range devs {
		if d.SerialNumber == "S-AQM-ACME-1" {
			aqm = d
			break
		}
	}
	require.NotNil(t, aqm, "non-Amazon AQM must be surfaced in List output")
	assert.Equal(t, "AIR_QUALITY_MONITOR", aqm.DeviceFamily)
	assert.Equal(t, "Acme", aqm.Manufacturer,
		"AQM Manufacturer must be the parsed value, not hardcoded to Amazon")
	assert.Equal(t, "42", aqm.SoftwareVersion)
	assert.Equal(t, "Office Acme Monitor", aqm.AccountName)
	assert.Equal(t, "ep-aqm-acme", aqm.EndpointID)
	// AQMs come from the GraphQL response without a deviceOwnerCustomerId
	// field, so we attribute ownership to the session's AccountCustomerID
	// and HouseholdDevice mirrors the non-AQM branch: true when ownership
	// resolves to the current account.
	assert.Equal(t, "OWNER", aqm.DeviceOwnerCustomerID,
		"AQM DeviceOwnerCustomerID should default to session AccountCustomerID")
	assert.True(t, aqm.HouseholdDevice,
		"AQM HouseholdDevice should be true when the session attributes ownership")
}

// TestRefresh_SurfacesSerialLessSmartHomeEndpoint verifies that smart-home
// endpoints with no serialNumber.value.text — typical for skill-discovered
// devices like TP-Link/Hue/Sensibo — surface in List output keyed by their
// chrsIdentifier.entityId UUID. Without this fallback, the entire class of
// third-party smart-home devices is silently dropped (which was the case
// before this bugfix: only Echos + AQMs + iOS app registrations appeared).
func TestRefresh_SurfacesSerialLessSmartHomeEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			// Empty base list — the smart-home endpoints are the only
			// devices on the account.
			_, _ = w.Write([]byte(`{"devices": []}`))
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLSerialLessJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}
	ds.initListCacheForTest(nil)

	devs, err := ds.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, devs, 2, "both serial-less smart-home endpoints must surface")

	var lamp, thermostat *Device
	for _, d := range devs {
		switch d.AccountName {
		case "Bedroom Lamp":
			lamp = d
		case "Side room Sensibo":
			thermostat = d
		}
	}

	require.NotNil(t, lamp, "TP-Link LIGHT endpoint must surface even though serialNumber is nil")
	assert.Equal(t, "b235b90d-fb31-4473-8042-64adb79bf9d9", lamp.SerialNumber,
		"SerialNumber must fall back to chrsIdentifier.entityId when graphqlSerial is empty")
	assert.Equal(t, "b235b90d-fb31-4473-8042-64adb79bf9d9", lamp.EntityID)
	assert.Equal(t, "amzn1.alexa.endpoint.b235b90d-fb31-4473-8042-64adb79bf9d9", lamp.EndpointID)
	assert.Equal(t, "TP-LINK", lamp.Manufacturer)
	assert.Equal(t, "HS100", lamp.Model)
	assert.Equal(t, Kind("light"), lamp.Kind)
	assert.Equal(t, "LIGHT", lamp.DeviceFamily)

	require.NotNil(t, thermostat, "Sensibo THERMOSTAT endpoint must surface even though serialNumber is nil")
	assert.Equal(t, "4603608e-380a-4635-8b5d-66c3241b0bdc", thermostat.SerialNumber)
	assert.Equal(t, "4603608e-380a-4635-8b5d-66c3241b0bdc", thermostat.EntityID)
	assert.Equal(t, "amzn1.alexa.endpoint.4603608e-380a-4635-8b5d-66c3241b0bdc", thermostat.EndpointID)
	assert.Equal(t, Kind("thermostat"), thermostat.Kind)
}

// TestInferAlexaInterfacesFromSensors_PowerAndBrightness verifies that
// a device with powerState + brightness sensor keys gets both interfaces
// derived. Skill-discovered third-party devices have empty
// alexaEnabledMetadata.capabilities, so without this derivation the
// typed accessors d.Power() / d.Brightness() would return nil even
// though the device clearly speaks the corresponding controllers.
func TestInferAlexaInterfacesFromSensors_PowerAndBrightness(t *testing.T) {
	sensors := map[string]*DeviceSensor{
		"powerState":   {Name: "powerState", Value: "OFF"},
		"brightness":   {Name: "brightness", Value: 75},
		"reachability": {Name: "reachability", Value: "OK"},
	}
	got := inferAlexaInterfacesFromSensors(sensors)
	assert.Contains(t, got, amzconst.InterfacePowerController)
	assert.Contains(t, got, amzconst.InterfaceBrightnessController)
	assert.NotContains(t, got, amzconst.InterfaceColorController)
	assert.NotContains(t, got, amzconst.InterfaceLockController)
	assert.NotContains(t, got, amzconst.InterfaceThermostatController)
}

// TestInferAlexaInterfacesFromSensors_FullLight verifies a color
// temperature + color RGB light gets all four light-related interfaces.
func TestInferAlexaInterfacesFromSensors_FullLight(t *testing.T) {
	sensors := map[string]*DeviceSensor{
		"powerState":       {Name: "powerState", Value: "ON"},
		"brightness":       {Name: "brightness", Value: 100},
		"color":            {Name: "color", Value: map[string]any{"hue": 38.0}},
		"colorTemperature": {Name: "colorTemperature", Value: 2700},
		"reachability":     {Name: "reachability", Value: "OK"},
	}
	got := inferAlexaInterfacesFromSensors(sensors)
	assert.ElementsMatch(t, []string{
		amzconst.InterfacePowerController,
		amzconst.InterfaceBrightnessController,
		amzconst.InterfaceColorController,
		amzconst.InterfaceColorTemperatureController,
	}, got)
}

// TestInferAlexaInterfacesFromSensors_Thermostat verifies that either
// targetSetpoint OR thermostatMode triggers the ThermostatController
// interface (not both — the de-dup is intentional).
func TestInferAlexaInterfacesFromSensors_Thermostat(t *testing.T) {
	bothKeys := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"targetSetpoint": {Name: "targetSetpoint", Value: 20.0, Scale: "CELSIUS"},
		"thermostatMode": {Name: "thermostatMode", Value: "COOL"},
	})
	thermCount := 0
	for _, s := range bothKeys {
		if s == amzconst.InterfaceThermostatController {
			thermCount++
		}
	}
	assert.Equal(t, 1, thermCount, "ThermostatController must appear exactly once even with both sensor keys")

	onlySetpoint := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"targetSetpoint": {Name: "targetSetpoint", Value: 20.0, Scale: "CELSIUS"},
	})
	assert.Contains(t, onlySetpoint, amzconst.InterfaceThermostatController)

	onlyMode := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"thermostatMode": {Name: "thermostatMode", Value: "COOL"},
	})
	assert.Contains(t, onlyMode, amzconst.InterfaceThermostatController)
}

// TestInferAlexaInterfacesFromSensors_Lock verifies LockController
// derivation from the lockState sensor key.
func TestInferAlexaInterfacesFromSensors_Lock(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"lockState":    {Name: "lockState", Value: "LOCKED"},
		"reachability": {Name: "reachability", Value: "OK"},
	})
	assert.Contains(t, got, amzconst.InterfaceLockController)
}

// TestInferAlexaInterfacesFromSensors_Mode_Canonical verifies that
// canonical Mode sensor keys (from amzconst.ModeInstances:
// "fanSpeed", "fanDirection") trigger InterfaceModeController
// derivation. The Sensibo Apparatus.FanSpeed instance maps to
// "fanSpeed" via the canonical table.
func TestInferAlexaInterfacesFromSensors_Mode_Canonical(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"fanSpeed":     {Name: "fanSpeed", Value: "Auto"},
		"reachability": {Name: "reachability", Value: "OK"},
	})
	assert.Contains(t, got, amzconst.InterfaceModeController)
}

// TestInferAlexaInterfacesFromSensors_Mode_PrefixedInstance
// verifies that unknown Mode instances (anything Amazon returns
// that's not in amzconst.ModeInstances) surface as "mode:<instance>"
// sensor keys and still trigger InterfaceModeController.
func TestInferAlexaInterfacesFromSensors_Mode_PrefixedInstance(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"mode:Genesis.ColorPalette": {Name: "mode:Genesis.ColorPalette", Value: "Flare"},
	})
	assert.Contains(t, got, amzconst.InterfaceModeController)
}

// TestInferAlexaInterfacesFromSensors_Range_Canonical verifies the
// canonical Range key "humidity" (from amzconst.RangeInstances)
// triggers InterfaceRangeController.
func TestInferAlexaInterfacesFromSensors_Range_Canonical(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"humidity":   {Name: "humidity", Value: 47.5, Scale: "%"},
		"powerState": {Name: "powerState", Value: "OFF"},
	})
	assert.Contains(t, got, amzconst.InterfaceRangeController)
	// Also still derives PowerController — same sensors can imply
	// multiple interfaces.
	assert.Contains(t, got, amzconst.InterfacePowerController)
}

// TestInferAlexaInterfacesFromSensors_Range_PrefixedInstance
// verifies unknown Range instances (range:<instance> fallback)
// still trigger InterfaceRangeController.
func TestInferAlexaInterfacesFromSensors_Range_PrefixedInstance(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"range:Apparatus.WhateverNew": {Name: "range:Apparatus.WhateverNew", Value: 99.0},
	})
	assert.Contains(t, got, amzconst.InterfaceRangeController)
}

// TestInferAlexaInterfacesFromSensors_Toggle verifies that
// toggle:<instance> sensor keys trigger InterfaceToggleController.
// There is no canonical Toggle instance table yet — every toggle
// uses the toggle:<instance> prefix.
func TestInferAlexaInterfacesFromSensors_Toggle(t *testing.T) {
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"toggle:Skill.Ionizer":   {Name: "toggle:Skill.Ionizer", Value: "ON"},
		"toggle:Skill.ChildLock": {Name: "toggle:Skill.ChildLock", Value: "OFF"},
	})
	assert.Contains(t, got, amzconst.InterfaceToggleController)
	// De-dup: two toggle keys should only add the interface once.
	count := 0
	for _, s := range got {
		if s == amzconst.InterfaceToggleController {
			count++
		}
	}
	assert.Equal(t, 1, count, "ToggleController should appear exactly once even with multiple toggle:* keys")
}

// TestInferAlexaInterfacesFromSensors_EmptyAndNoMatches verifies the
// no-sensor and unrelated-sensor cases return nil so callers don't
// see a non-nil empty slice.
func TestInferAlexaInterfacesFromSensors_EmptyAndNoMatches(t *testing.T) {
	assert.Nil(t, inferAlexaInterfacesFromSensors(nil))
	assert.Nil(t, inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{}))
	got := inferAlexaInterfacesFromSensors(map[string]*DeviceSensor{
		"reachability": {Name: "reachability", Value: "OK"},
		"temperature":  {Name: "temperature", Value: 22.5, Scale: "CELSIUS"},
	})
	assert.Nil(t, got, "no controller-implying sensors present → nil result")
}

// TestMergeAlexaInterfaces_Union verifies the union semantics: existing
// entries preserve their order at the head; derived entries that aren't
// already present append in order.
func TestMergeAlexaInterfaces_Union(t *testing.T) {
	merged := mergeAlexaInterfaces(
		[]string{"Custom.Foo", amzconst.InterfacePowerController},
		[]string{amzconst.InterfacePowerController, amzconst.InterfaceBrightnessController},
	)
	assert.Equal(t, []string{
		"Custom.Foo",
		amzconst.InterfacePowerController,
		amzconst.InterfaceBrightnessController,
	}, merged)

	// Empty existing → derived returned as-is.
	assert.Equal(t,
		[]string{amzconst.InterfaceLockController},
		mergeAlexaInterfaces(nil, []string{amzconst.InterfaceLockController}),
	)

	// Both empty → nil.
	assert.Nil(t, mergeAlexaInterfaces(nil, nil))
}

func TestDevicesService_Inspect_RewritesCacheInPlace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			opName, _ := body["operationName"].(string)
			w.Header().Set("Content-Type", "application/json")
			switch opName {
			case "getDevicesBaseData":
				_, _ = w.Write(endpointsGraphQLJSON)
			case "endpointAllFeatures":
				http.ServeFile(w, r, "testdata/devices/endpoint_all_features.json")
			case "getEndpointState":
				// Reached during Refresh()'s sensorsHook, not during Inspect().
				_, _ = w.Write([]byte(`{"data":{"listEndpoints":{"endpoints":[]}}}`))
			default:
				http.Error(w, "unexpected op: "+opName, http.StatusBadRequest)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	c, err := NewClient(WithHTTPClient(hc), WithSite(srv.URL), WithSession(&Session{AccountCustomerID: "OWNER", Domain: "com"}))
	require.NoError(t, err)

	require.NoError(t, c.Devices.Refresh(context.Background()))

	devs, err := c.Devices.List(context.Background())
	require.NoError(t, err)
	var target *Device
	for _, d := range devs {
		if d.SerialNumber == "TESTPLUG001" {
			target = d
			break
		}
	}
	require.NotNil(t, target, "smart-plug should be in cache after Refresh")

	// Override the EndpointID to the fixture's id so the test fixture matches.
	target.EndpointID = "amzn1.alexa.endpoint.32f10fc0-9933-4cfa-b8eb-c223a779c190"

	updated, err := c.Devices.Inspect(context.Background(), target.SerialNumber)
	require.NoError(t, err)
	assert.Same(t, target, updated, "Inspect updates in place")

	assert.Contains(t, updated.DisplayCategories, "THERMOSTAT")
	assert.Equal(t, KindThermostat, updated.Kind)
	assert.Equal(t, "Sideroom Airconditioner", updated.AccountName)
}

func TestFetchEndpoints_PopulatesPhase4aFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}
	ds.initListCacheForTest(nil)

	devs, err := ds.List(context.Background())
	require.NoError(t, err)

	var echo *Device
	for _, d := range devs {
		if d.Kind == KindEcho {
			echo = d
			break
		}
	}
	require.NotNil(t, echo, "fixture should yield at least one Echo")

	var aqm *Device
	for _, d := range devs {
		if d.Kind == KindAQM {
			aqm = d
			break
		}
	}
	require.NotNil(t, aqm, "fixture should yield the AQM entry tagged KindAQM")

	// Phase 4a Task 4: smart-home endpoints now surface as *Device.
	var smartPlug *Device
	for _, d := range devs {
		if d.SerialNumber == "TESTPLUG001" {
			smartPlug = d
			break
		}
	}
	require.NotNil(t, smartPlug, "smart-plug should now be surfaced after Task 4")
	assert.Equal(t, KindSwitch, smartPlug.Kind, "SMARTPLUG → KindSwitch")
	assert.Contains(t, smartPlug.DisplayCategories, "SMARTPLUG")
	assert.Equal(t, []string{"turnOn", "turnOff"}, smartPlug.Operations)
}
