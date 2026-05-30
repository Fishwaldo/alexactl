package amazondevices

// SequenceType is an Alexa sequence command name as it appears on the wire.
//
// ENUM(
//
//	SeqSpeak="Alexa.Speak",
//	SeqAnnouncement="AlexaAnnouncement",
//	SeqSound="Alexa.Sound",
//	SeqMusic="Alexa.Music.PlaySearchPhrase",
//	SeqTextCommand="Alexa.TextCommand",
//	SeqLaunchSkill="Alexa.Operation.SkillConnections.Launch",
//	SeqVolume="Alexa.DeviceControls.Volume",
//	SeqStop="Alexa.DeviceControls.Stop",
//	SeqRoutine="Pseudo.Type.Routines"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type SequenceType string

// MediaCommand is a media transport command name as it appears on the wire.
//
// ENUM(
//
//	CmdPlay="PlayCommand",
//	CmdStop="StopSequence",
//	CmdPause="PauseCommand",
//	CmdNext="NextCommand",
//	CmdPrev="PreviousCommand",
//	CmdRewind="RewindCommand",
//	CmdForward="ForwardCommand"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type MediaCommand string

// PushMessageType is a push-event wire type sent over the event stream.
//
// ENUM(
//
//	PushGenericActivity="PUSH_ACTIVITY",
//	PushConnectionStatus="PUSH_DOPPLER_CONNECTION_CHANGE",
//	PushBluetoothStatus="PUSH_BLUETOOTH_STATE_CHANGE",
//	PushMicrophoneStatus="PUSH_MICROPHONE_STATE",
//	PushNotificationChange="PUSH_NOTIFICATION_CHANGE",
//	PushAudioPlayerState="PUSH_AUDIO_PLAYER_STATE",
//	PushEqualizerStateChange="PUSH_EQUALIZER_STATE_CHANGE",
//	PushMediaQueueChange="PUSH_MEDIA_QUEUE_CHANGE",
//	PushMediaChange="PUSH_MEDIA_CHANGE",
//	PushMediaProgressChange="PUSH_MEDIA_PROGRESS_CHANGE",
//	PushVolumeChange="PUSH_VOLUME_CHANGE",
//	PushMediaSessionsUpdated="NotifyMediaSessionsUpdated",
//	PushNowPlayingUpdated="NotifyNowPlayingUpdated",
//	PushItemChange="PUSH_LIST_ITEM_CHANGE",
//	PushMatterDeviceFound="MATTER_SETUP_NOTIFICATION"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type PushMessageType string

// Kind is the device classification derived from its capabilities.
//
// ENUM(
//
//	KindEcho="echo",
//	KindLight="light",
//	KindSwitch="switch",
//	KindDimmer="dimmer",
//	KindThermostat="thermostat",
//	KindLock="lock",
//	KindCamera="camera",
//	KindContact="contact_sensor",
//	KindMotion="motion_sensor",
//	KindAQM="air_quality_monitor",
//	KindSensor="sensor",
//	KindAirPurifier="air_purifier",
//	KindSecurityPanel="security_panel",
//	KindScene="scene",
//	KindOther="other"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type Kind string

// TemperatureScale is the unit a thermostat reports temperatures in.
//
// ENUM(
//
//	TemperatureScaleCelsius="CELSIUS",
//	TemperatureScaleFahrenheit="FAHRENHEIT",
//	TemperatureScaleKelvin="KELVIN"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type TemperatureScale string

// ThermostatMode is the operating mode a thermostat is set to.
//
// ENUM(
//
//	ThermostatModeHeat="HEAT",
//	ThermostatModeCool="COOL",
//	ThermostatModeAuto="AUTO",
//	ThermostatModeOff="OFF",
//	ThermostatModeEco="ECO",
//	ThermostatModeEmHeat="EM_HEAT"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type ThermostatMode string

// LockState is the reported state of a smart lock.
//
// ENUM(
//
//	LockStateLocked="LOCKED",
//	LockStateUnlocked="UNLOCKED",
//	LockStateJammed="JAMMED"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type LockState string

// ListType is the kind of Alexa list (shopping vs. custom).
//
// ENUM(
//
//	ListTypeShopping="SHOPPING_LIST",
//	ListTypeCustom="LIST"
//
// )
//
//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
type ListType string
