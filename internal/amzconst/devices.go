// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package amzconst

// Special device types referenced by the device handler.
const (
	DeviceTypeAQM          = "AEZME1X38KDRA"
	DeviceTypeCubeGen3     = "A2JKHJ0PX4J3L3"
	DeviceTypeSpeakerGroup = "A3C9PE6TNYLTCH"
	DeviceTypeStereoPair   = "AP1F6KUH00XPV"
)

// SpeakerGroupFamily is the deviceFamily value for grouped speakers.
const SpeakerGroupFamily = "WHA"

// DeviceTypeHardcodedMetadata supplies model/manufacturer for device types
// that don't carry it in the GraphQL response. Modeled on
// aioamazondevices const/devices.py DEVICE_TYPES_HARDCODED_METADATA.
var DeviceTypeHardcodedMetadata = map[string]struct {
	Model        string
	Manufacturer string
}{
	DeviceTypeAQM:          {Model: "Air Quality Monitor", Manufacturer: "Amazon"},
	DeviceTypeCubeGen3:     {Model: "Fire TV Cube (3rd Generation)", Manufacturer: "Amazon"},
	DeviceTypeSpeakerGroup: {Model: "Speaker Group", Manufacturer: "Amazon"},
	DeviceTypeStereoPair:   {Model: "Stereo Pair", Manufacturer: "Amazon"},
}

// Alexa capability-interface names that gate the typed control accessors
// on *Device. These are the literal strings Amazon reports under
// alexaEnabledMetadata.capabilities for endpoints that expose the
// corresponding controller. Constants used by Device.Power(),
// Device.Brightness(), etc. to decide whether to return a non-nil
// control struct.
const (
	InterfacePowerController            = "Alexa.PowerController"
	InterfaceBrightnessController       = "Alexa.BrightnessController"
	InterfaceColorController            = "Alexa.ColorController"
	InterfaceColorTemperatureController = "Alexa.ColorTemperatureController"
	InterfaceThermostatController       = "Alexa.ThermostatController"
	InterfaceLockController             = "Alexa.LockController"
	InterfaceModeController             = "Alexa.ModeController"
	InterfaceRangeController            = "Alexa.RangeController"
	InterfaceToggleController           = "Alexa.ToggleController"
)

// DeviceTypesToIgnore lists device types we strip from List responses
// because they're known non-controllable or buggy. Modeled on
// aioamazondevices DEVICE_TYPES_TO_IGNORE.
var DeviceTypesToIgnore = map[string]struct{}{
	AmazonDeviceType: {}, // Alexa App for iOS (we ARE this device)

	"A133UZ2CB0IB8":  {}, // Sony Soundbar Sony HT-A5000
	"A14AIWB3T3AS1Z": {}, // Samsung Soundbar HW-Q950A
	"A15ERDAKK5HQQG": {}, // unsupported Sonos devices
	"A18BI6KPKDOEI4": {}, // ecobee4 Smart Thermostat
	"A189LTZOO5ZTGA": {}, // Sony WH-1000XM5
	"A1L4KDRIILU6N9": {}, // Sony WH-CH700N
	"A1NPP2J03FTS0I": {}, // Eero Pro 6
	"A1NQ0LXWBGVQS9": {}, // Samsung 2012 QLED TV
	"A1P7E7V3FCZKU6": {}, // Toshiba TV
	"A1RTAM01W29CUP": {}, // Alexa App for PC
	"A1X5IB2CRN3E8G": {}, // Fitbit Versa 3
	"A21Z3CGI8UIP0F": {}, // Denon AVR-X1600H
	"A23ZD3FSVQM5EE": {}, // Sony WH-1000XM2
	"A28X5U5THTXXM1": {}, // Jabra Elite 85t
	"A2IJJ9QXVOSYK0": {}, // JBL TUNE770NC
	"A2M9HB23M9MSSM": {}, // Amazfit Bip U Pro
	"A2QDHDQIWC2LTG": {}, // Echo Buds (Left)
	"A2TF17PFR55MTB": {}, // Alexa App for Android
	"A2Y04QPFCANLPQ": {}, // Bose QC 35 II
	"A31PMVIWCRNTX2": {}, // Echo Buds (Right)
	"A3BW5ZVFHRCQPO": {}, // BMW Mini Car System
	"A3GZUE7F9MEB4U": {}, // Sony WH-1000XM3
	"A3HVREY4JWAZ6K": {}, // Echo Buds (Charger)
	"A3KOTUS4DKHU1W": {}, // Samsung Fridge
	"A3PAHYZLPKL73D": {}, // EERO 6 Wifi AP
	"A3SSG6GR8UU7SN": {}, // Echo Sub
	"A7S41FQ5TWBC9":  {}, // Sony WH-1000XM4
	"AHL4H6CKH3AUP":  {}, // BMW Car System
	"AKOAGQTKAS9YB":  {}, // Echo Connect
	"AN630UQPG2CA4":  {}, // Insignia TV
	"APHEAY6LX7T13":  {}, // Samsung Fridge RS22T
	"AYHO3NTIQQ04G":  {}, // Nextbase Dash Cam
}
