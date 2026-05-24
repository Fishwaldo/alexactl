package amazondevices

// Device describes one Amazon Echo (or Alexa-enabled) device. Returned by
// DevicesService.List as a read-only snapshot — mutations on a *Device do
// NOT update the client's internal cache. Call DevicesService.Refresh to
// re-fetch.
type Device struct {
	AccountName            string
	SerialNumber           string
	DeviceType             string
	DeviceFamily           string
	DeviceOwnerCustomerID  string
	HouseholdDevice        bool
	Online                 bool
	Capabilities           []string          // raw Amazon capability strings
	ClusterMembers         map[string]string // serial -> device type ("" if unknown)
	Manufacturer           string
	Model                  string
	SoftwareVersion        string
	HardwareVersion        string
	EntityID               string
	EndpointID             string
	Sensors                map[string]*DeviceSensor // populated in Phase 2
	Notifications          map[string]*Schedule     // populated in Phase 2
	NotificationsSupported bool
	MediaPlayerSupported   bool
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

// DeviceSensor is a single sensor reading. Populated in Phase 2.
type DeviceSensor struct {
	Name      string
	Value     any // string | int | float64
	Error     bool
	ErrorType string
	ErrorMsg  string
	Scale     string
}

// Schedule is a single timer/alarm/reminder. Populated in Phase 2.
type Schedule struct {
	Type           string // "alarm" | "reminder" | "timer"
	Status         string
	Label          string
	NextOccurrence string // ISO-8601; parsed lazily
}
