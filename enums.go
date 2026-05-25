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
