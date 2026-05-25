package amzconst

// Volume range supported by Amazon devices.
const (
	VolumeMin = 0
	VolumeMax = 100
)

// MaxCustomerAccountRetries is how many times we'll retry the customer-ID
// lookup right after registration. Each attempt sleeps 2s.
const MaxCustomerAccountRetries = 3

// SensorStateOff is the wire value Amazon returns when a detection sensor
// reports "no detection" (e.g., motion sensor in idle state).
const SensorStateOff = "NOT_DETECTED"

// SensorTemplate describes how to extract a single sensor's value from the
// GraphQL getEndpointState feature payload.
//
//   Name   — the canonical Go-side sensor name (e.g., "temperature").
//            Also matched against feature.properties[].name to find the
//            correct property entry within a feature.
//   Key    — the GraphQL property key that carries the value blob
//            (e.g., "value", "illuminanceValue", "rangeValue").
//   Subkey — when the value blob is an object, the nested key whose value
//            we extract. Empty means the value blob itself is the value.
//   Scale  — when the value blob is an object, the nested key carrying the
//            scale/unit string. Empty means the sensor has no scale.
type SensorTemplate struct {
	Name   string
	Key    string
	Subkey string
	Scale  string
}

// Sensors maps a GraphQL feature.name to the SensorTemplate describing how
// to extract its value. Port of aioamazondevices const/metadata.py SENSORS.
//
// Names match the canonical wire values Amazon returns under
// alexaEnabledMetadata.capabilities[].name in getEndpointState responses.
var Sensors = map[string]SensorTemplate{
	"temperatureSensor": {
		Name:   "temperature",
		Key:    "value",
		Subkey: "value",
		Scale:  "scale",
	},
	"motionSensor": {
		Name:   "detectionState",
		Key:    "detectionStateValue",
		Subkey: "",
		Scale:  "",
	},
	"lightSensor": {
		Name:   "illuminance",
		Key:    "illuminanceValue",
		Subkey: "value",
		Scale:  "",
	},
	"connectivity": {
		Name:   "reachability",
		Key:    "reachabilityStatusValue",
		Subkey: "",
		Scale:  "",
	},
	"range": {
		Name:   "rangeValue",
		Key:    "rangeValue",
		Subkey: "value",
		Scale:  "",
	},
}

// AQMRangeSensor describes one Air Quality Monitor sub-sensor surfaced
// through a generic "range" feature. The AQM device exposes ~6 distinct
// sensors (Humidity, VOC, PM25, PM10, CO, Air Quality) under one GraphQL
// feature; the per-sub-sensor distinction comes from feature.instance.
type AQMRangeSensor struct {
	Name  string
	Scale string
}

// AQMRangeSensors maps the AQM range-feature instance (a string identifier
// like "4", "5", ...) to the sub-sensor it represents. Port of
// aioamazondevices const/metadata.py AQM_RANGE_SENSORS.
var AQMRangeSensors = map[string]AQMRangeSensor{
	"4": {Name: "Humidity", Scale: "%"},
	"5": {Name: "VOC", Scale: ""},
	"6": {Name: "PM25", Scale: "MicroGramsPerCubicMeter"},
	"7": {Name: "PM10", Scale: "MicroGramsPerCubicMeter"},
	"8": {Name: "CO", Scale: "ppm"},
	"9": {Name: "Air Quality", Scale: ""},
}
