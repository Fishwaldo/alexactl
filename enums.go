package amazondevices

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   SeqSpeak="Alexa.Speak",
//   SeqAnnouncement="AlexaAnnouncement",
//   SeqSound="Alexa.Sound",
//   SeqMusic="Alexa.Music.PlaySearchPhrase",
//   SeqTextCommand="Alexa.TextCommand",
//   SeqLaunchSkill="Alexa.Operation.SkillConnections.Launch",
//   SeqVolume="Alexa.DeviceControls.Volume",
//   SeqStop="Alexa.DeviceControls.Stop",
//   SeqRoutine="Pseudo.Type.Routines"
// )
type SequenceType string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   CmdPlay="PlayCommand",
//   CmdStop="StopSequence",
//   CmdPause="PauseCommand",
//   CmdNext="NextCommand",
//   CmdPrev="PreviousCommand",
//   CmdRewind="RewindCommand",
//   CmdForward="ForwardCommand"
// )
type MediaCommand string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   PushGenericActivity="PUSH_ACTIVITY",
//   PushConnectionStatus="PUSH_DOPPLER_CONNECTION_CHANGE",
//   PushBluetoothStatus="PUSH_BLUETOOTH_STATE_CHANGE",
//   PushMicrophoneStatus="PUSH_MICROPHONE_STATE",
//   PushNotificationChange="PUSH_NOTIFICATION_CHANGE",
//   PushAudioPlayerState="PUSH_AUDIO_PLAYER_STATE",
//   PushEqualizerStateChange="PUSH_EQUALIZER_STATE_CHANGE",
//   PushMediaQueueChange="PUSH_MEDIA_QUEUE_CHANGE",
//   PushMediaChange="PUSH_MEDIA_CHANGE",
//   PushMediaProgressChange="PUSH_MEDIA_PROGRESS_CHANGE",
//   PushVolumeChange="PUSH_VOLUME_CHANGE",
//   PushMediaSessionsUpdated="NotifyMediaSessionsUpdated",
//   PushNowPlayingUpdated="NotifyNowPlayingUpdated",
//   PushItemChange="PUSH_LIST_ITEM_CHANGE",
//   PushMatterDeviceFound="MATTER_SETUP_NOTIFICATION"
// )
type PushMessageType string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   KindEcho="echo",
//   KindLight="light",
//   KindSwitch="switch",
//   KindDimmer="dimmer",
//   KindThermostat="thermostat",
//   KindLock="lock",
//   KindCamera="camera",
//   KindContact="contact_sensor",
//   KindMotion="motion_sensor",
//   KindAQM="air_quality_monitor",
//   KindSensor="sensor",
//   KindAirPurifier="air_purifier",
//   KindSecurityPanel="security_panel",
//   KindScene="scene",
//   KindOther="other"
// )
type Kind string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   TemperatureScaleCelsius="CELSIUS",
//   TemperatureScaleFahrenheit="FAHRENHEIT",
//   TemperatureScaleKelvin="KELVIN"
// )
type TemperatureScale string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   ThermostatModeHeat="HEAT",
//   ThermostatModeCool="COOL",
//   ThermostatModeAuto="AUTO",
//   ThermostatModeOff="OFF",
//   ThermostatModeEco="ECO",
//   ThermostatModeEmHeat="EM_HEAT"
// )
type ThermostatMode string

//go:generate go tool go-enum -f $GOFILE --marshal --names --values --nocase --noprefix
//
// ENUM(
//   LockStateLocked="LOCKED",
//   LockStateUnlocked="UNLOCKED",
//   LockStateJammed="JAMMED"
// )
type LockState string
