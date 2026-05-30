package alexactl

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/alexagraphql"
)

// TestParseAllFeaturesEndpoint_PopulatesSensors uses the
// endpoint_all_features.json fixture (the captured Sideroom
// Airconditioner response from Phase 4a Task 8) and asserts the
// parser produces the expected Device.Sensors map without going
// through sensorsHook.
//
// Coverage:
//   - power → "powerState" sensor ("ON")
//   - connectivity → "reachability" sensor ("OK")
//   - temperatureSensor → "temperature" sensor (28.0 / CELSIUS)
//   - thermostat → "thermostatMode" ("COOL") + "targetSetpoint" (20.0)
//   - thermostat.configuration → carried onto both thermostat sensors
func TestParseAllFeaturesEndpoint_PopulatesSensors(t *testing.T) {
	body, err := os.ReadFile("testdata/devices/endpoint_all_features.json")
	require.NoError(t, err)

	var resp struct {
		Data struct {
			Endpoint *alexagraphql.EndpointAllFeaturesEndpoint `json:"endpoint"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.NotNil(t, resp.Data.Endpoint)

	dvc := &Device{
		SerialNumber: "S-AC-001",
		EndpointID:   "amzn1.alexa.endpoint.32f10fc0-9933-4cfa-b8eb-c223a779c190",
		Sensors:      map[string]*DeviceSensor{},
	}
	parsed := parseAllFeaturesEndpoint(resp.Data.Endpoint, dvc)

	require.NotNil(t, parsed["powerState"])
	assert.Equal(t, "ON", parsed["powerState"].Value)

	require.NotNil(t, parsed["reachability"])
	assert.Equal(t, "OK", parsed["reachability"].Value)

	require.NotNil(t, parsed["temperature"])
	assert.EqualValues(t, 28.0, parsed["temperature"].Value)
	assert.Equal(t, "CELSIUS", parsed["temperature"].Scale)

	require.NotNil(t, parsed["thermostatMode"])
	assert.Equal(t, "COOL", parsed["thermostatMode"].Value)

	require.NotNil(t, parsed["targetSetpoint"])
	assert.EqualValues(t, 20.0, parsed["targetSetpoint"].Value)
	assert.Equal(t, "CELSIUS", parsed["targetSetpoint"].Scale)

	// Configuration carried on the thermostat sensors.
	cfg := parsed["thermostatMode"].Configuration
	require.NotNil(t, cfg, "thermostat sensor must carry Configuration")
	assert.Equal(t, "ThermostatConfiguration", cfg["_type"])
}
