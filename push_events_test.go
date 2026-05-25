package amazondevices

// Compile-time assertions: every concrete event type satisfies PushEvent.
var (
	_ PushEvent = (*VolumeChangeEvent)(nil)
	_ PushEvent = (*GenericActivityEvent)(nil)
	_ PushEvent = (*ConnectionStatusEvent)(nil)
	_ PushEvent = (*BluetoothStatusEvent)(nil)
	_ PushEvent = (*MicrophoneStatusEvent)(nil)
	_ PushEvent = (*NotificationChangeEvent)(nil)
	_ PushEvent = (*AudioPlayerStateEvent)(nil)
	_ PushEvent = (*EqualizerStateChangeEvent)(nil)
	_ PushEvent = (*MediaQueueChangeEvent)(nil)
	_ PushEvent = (*MediaChangeEvent)(nil)
	_ PushEvent = (*MediaProgressChangeEvent)(nil)
	_ PushEvent = (*MediaSessionsUpdatedEvent)(nil)
	_ PushEvent = (*NowPlayingUpdatedEvent)(nil)
	_ PushEvent = (*ItemChangeEvent)(nil)
	_ PushEvent = (*MatterDeviceFoundEvent)(nil)
	_ PushEvent = (*UnknownPushEvent)(nil)
	_ PushEvent = (*ReauthRequiredEvent)(nil)
)
