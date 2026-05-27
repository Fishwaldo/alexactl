package amzconst

// DisplayCategoryToKind maps Amazon's `DisplayCategory` strings (the
// "primary" display category Amazon reports for each endpoint) to the
// canonical Kind value the library exposes via Device.Kind.
//
// The full DisplayCategory list is much larger than the cases here;
// anything not in the table falls through to "other" via
// DisplayCategoryToKind below. KindEcho is NEVER returned from this
// table — Echo classification happens at a higher level in
// fetchEndpoints, using the base /api/devices-v2/device list.
var displayCategoryToKind = map[string]string{
	"LIGHT":               "light",
	"SMARTPLUG":           "switch",
	"SWITCH":              "switch",
	"FAN":                 "switch", // fans are treated as on/off switches at the Kind level; speed is a sensor
	"THERMOSTAT":          "thermostat",
	"AIR_CONDITIONER":     "thermostat",
	"HEATER":              "thermostat",
	"LOCK":                "lock",
	"CAMERA":              "camera",
	"CONTACT_SENSOR":      "contact_sensor",
	"MOTION_SENSOR":       "motion_sensor",
	"AIR_QUALITY_MONITOR": "air_quality_monitor",
	"TEMPERATURE_SENSOR":  "sensor",
	"AIR_PURIFIER":        "air_purifier",
	"SECURITY_PANEL":      "security_panel",
	"SCENE_TRIGGER":       "scene",
	// OTHER and APPLICATION are explicit no-ops — they map to the
	// fallback value "other" via the function below. Listed here so
	// the table documents that we've seen these categories and
	// intentionally treat them as the catch-all.
	"OTHER":       "other",
	"APPLICATION": "other",
}

// DisplayCategoryToKind returns the canonical Kind string for a given
// Alexa DisplayCategory. Unknown / empty categories return "other".
func DisplayCategoryToKind(category string) string {
	if k, ok := displayCategoryToKind[category]; ok {
		return k
	}
	return "other"
}
