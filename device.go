// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

// Device describes one Amazon Echo (or Alexa-enabled) device. Returned by
// DevicesService.List as a read-only snapshot — mutations on a *Device do
// NOT update the client's internal cache. Call DevicesService.Refresh to
// re-fetch.
type Device struct {
	AccountName           string
	SerialNumber          string
	DeviceType            string
	DeviceFamily          string
	DeviceOwnerCustomerID string
	HouseholdDevice       bool
	Online                bool
	Capabilities          []string          // raw Amazon capability strings
	ClusterMembers        map[string]string // serial -> device type ("" if unknown)
	Manufacturer          string
	Model                 string
	SoftwareVersion       string
	HardwareVersion       string
	EntityID              string
	EndpointID            string
	Sensors               map[string]*DeviceSensor // populated by SensorsService during Refresh
	SchedulesSupported    bool
	MediaPlayerSupported  bool

	// CommunicationSettings is reserved for future enrichment that would
	// attach the allowed comms preferences (keys: "communications",
	// "dropin", "announcements"; values are the per-pref state) to each
	// device. It is NOT currently populated:
	// Client.Communications.GetPreferences returns the preference data
	// directly as a standalone map keyed by serial and never writes back
	// to *Device. This field is always nil today.
	CommunicationSettings map[string]string

	// Typed schedule slices populated by DevicesService.Refresh's
	// sensors-enrichment hook (one shared /api/notifications fetch routed
	// across all devices). Slice is nil for devices that don't support
	// schedules (SchedulesSupported == false).
	Alarms    []*Alarm
	Reminders []*Reminder
	Timers    []*Timer

	// Smart-home endpoint classification and description fields.
	//
	// Kind is the canonical device-class (KindLight, KindThermostat, …).
	// Derived from DisplayCategories.primary via amzconst.DisplayCategoryToKind
	// for non-Echo endpoints; Echos are tagged KindEcho regardless of the
	// GraphQL-reported DisplayCategory.
	Kind Kind

	// DisplayCategories carries every Alexa DisplayCategory reported for
	// this endpoint (e.g. ["THERMOSTAT", "AIR_CONDITIONER",
	// "TEMPERATURE_SENSOR"] for an AC unit). Useful for UI consumers that
	// want to render composite-device hints. Empty for Echos that aren't
	// surfaced via the GraphQL endpoint graph.
	DisplayCategories []string

	// AlexaInterfaces lists the Alexa capability interfaces Amazon
	// advertises for this endpoint (e.g. "Alexa.PowerController",
	// "Alexa.BrightnessController"). Sourced from
	// AlexaEnabledMetadata.capabilities; empty when the endpoint isn't
	// ALEXA_VOICE_ENABLED.
	AlexaInterfaces []string

	// Operations lists the operation names Amazon's directive system
	// accepts for this endpoint (e.g. ["turnOn", "turnOff",
	// "setBrightness"]). The schema-level manifest of what the
	// setEndpointFeatures mutation would accept. The discovery query
	// populates this from Endpoint.operations; it drives typed control
	// surfaces.
	Operations []string

	// controller is the back-reference to the per-Client smartHomeClient
	// that backs the typed control accessors (Power(), Brightness(), …).
	// Set by DevicesService when a device is added to the cache; nil for
	// devices that were never registered through a Client (e.g. tests
	// that build a *Device by hand can leave it nil — accessors then
	// return nil for safety).
	controller *smartHomeClient
}

// HasCapability reports whether the device advertises the given capability.
func (d *Device) HasCapability(c string) bool {
	for _, x := range d.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}

// IsCluster reports whether the device is grouped with other devices
// (e.g., a stereo pair or Echo group).
func (d *Device) IsCluster() bool {
	return len(d.ClusterMembers) > 1
}

// DeviceSensor is a single sensor reading. Populated by SensorsService during Refresh.
type DeviceSensor struct {
	Name      string
	Value     any // string | int | float64
	Error     bool
	ErrorType string
	ErrorMsg  string
	Scale     string

	// Fields that surface the feature's action space.
	//
	// Configuration carries the typed Feature.configuration values
	// (e.g. ThermostatConfiguration.supportedModes). Map keys are
	// configuration-variant-specific; common keys are documented per
	// feature. Empty when the feature has no Configuration or we don't
	// extract it.
	Configuration map[string]any

	// Operations lists the feature-level operation names the directive
	// system accepts for this sensor's feature (e.g. ["setBrightness",
	// "adjustBrightness"] for a brightness sensor). Empty when the
	// feature exposes no operations.
	Operations []string
}
