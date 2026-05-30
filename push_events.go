package alexactl

// PushEvent is the common interface implemented by every concrete push
// event type. Concrete event types live in this file. Switch on the
// concrete type with a type-switch when you need typed-field access; use
// RawPayload() when you need fields that have not yet been promoted to
// typed accessors.
type PushEvent interface {
	// Type returns the wire-level event identifier.
	Type() PushMessageType

	// SerialNumber returns the device-scoped serial number when the
	// payload includes one, or the empty string when the event is not
	// device-scoped (e.g., MatterDeviceFound carries a setup nonce
	// rather than a serial).
	SerialNumber() string

	// RawPayload returns the parsed JSON object that produced this
	// event. Callers MUST NOT mutate it — the decoder shares the
	// underlying map across all subscribers.
	RawPayload() map[string]any
}

// UnknownPushEvent is delivered whenever the decoder sees a resourceId
// it does not recognise. Surfaces unknown events rather than silently
// dropping them (better for debuggability than upstream's WARN-and-drop).
type UnknownPushEvent struct {
	// WireType is the literal resourceId string we received.
	WireType string
	// Serial is the device serial number if the payload happened to
	// include one in the expected place; otherwise empty.
	Serial string
	// Payload is the parsed JSON.
	Payload map[string]any
}

// Type returns the empty PushMessageType — by definition the wire string
// did not correspond to a known member. Callers inspect WireType
// directly for the raw resourceId.
func (e *UnknownPushEvent) Type() PushMessageType { return "" }

// SerialNumber implements PushEvent.
func (e *UnknownPushEvent) SerialNumber() string { return e.Serial }

// RawPayload implements PushEvent.
func (e *UnknownPushEvent) RawPayload() map[string]any { return e.Payload }

// ReauthRequiredEvent is delivered by PushService as the
// final event before the subscriber's channel closes whenever the stream
// returns 401/403 even after a token refresh attempt. The type is defined
// so callers can write `switch evt.(type)` against it.
type ReauthRequiredEvent struct{}

// Type implements PushEvent.
func (e *ReauthRequiredEvent) Type() PushMessageType { return "" }

// SerialNumber implements PushEvent.
func (e *ReauthRequiredEvent) SerialNumber() string { return "" }

// RawPayload implements PushEvent.
func (e *ReauthRequiredEvent) RawPayload() map[string]any { return nil }

// pushEventBase is embedded by every concrete typed push event. It
// provides the three interface methods without re-typing them on
// every concrete struct.
type pushEventBase struct {
	serial string
	raw    map[string]any
	kind   PushMessageType
}

func (b *pushEventBase) Type() PushMessageType      { return b.kind }
func (b *pushEventBase) SerialNumber() string       { return b.serial }
func (b *pushEventBase) RawPayload() map[string]any { return b.raw }

// VolumeChangeEvent reports that a device's volume or mute state
// changed.
//
// Volume is the post-change volume on 0..100; Muted is the post-change
// mute state. Both fields are decoded from the payload at construction
// time; if either is absent, the zero value is used.
type VolumeChangeEvent struct {
	pushEventBase
	Volume int
	Muted  bool
}

// All remaining concrete events expose only the interface methods.
// Add typed fields incrementally as use-cases emerge.

// GenericActivityEvent reports a generic device activity (PUSH_ACTIVITY).
type GenericActivityEvent struct{ pushEventBase }

// ConnectionStatusEvent reports a device's cloud connection state change.
type ConnectionStatusEvent struct{ pushEventBase }

// BluetoothStatusEvent reports a device's Bluetooth state change.
type BluetoothStatusEvent struct{ pushEventBase }

// MicrophoneStatusEvent reports a device's microphone (mute) state change.
type MicrophoneStatusEvent struct{ pushEventBase }

// NotificationChangeEvent reports an alarm/timer/reminder change. Even notificationVersion duplicates are filtered by the decoder.
type NotificationChangeEvent struct{ pushEventBase }

// AudioPlayerStateEvent reports a device's audio-player state transition.
type AudioPlayerStateEvent struct{ pushEventBase }

// EqualizerStateChangeEvent reports a device's equalizer setting change.
type EqualizerStateChangeEvent struct{ pushEventBase }

// MediaQueueChangeEvent reports a change to a device's media queue.
type MediaQueueChangeEvent struct{ pushEventBase }

// MediaChangeEvent reports a change of currently playing media.
type MediaChangeEvent struct{ pushEventBase }

// MediaProgressChangeEvent reports playback-position progress within the current media.
type MediaProgressChangeEvent struct{ pushEventBase }

// MediaSessionsUpdatedEvent reports a refresh of the device's media-sessions list.
type MediaSessionsUpdatedEvent struct{ pushEventBase }

// NowPlayingUpdatedEvent reports an update to the "now playing" metadata.
type NowPlayingUpdatedEvent struct{ pushEventBase }

// ItemChangeEvent reports a change to an Alexa list item (shopping list, to-do, etc.).
type ItemChangeEvent struct{ pushEventBase }

// MatterDeviceFoundEvent reports a Matter device discovered for setup. Not device-scoped (SerialNumber() returns "").
type MatterDeviceFoundEvent struct{ pushEventBase }

// NewPushEventBase is the helper that the internal/http2stream decoder
// uses to populate the embedded base of any typed push event. Exposed
// here (rather than letting the decoder reach unexported fields)
// because the decoder lives in a sub-package.
func NewPushEventBase(serial string, raw map[string]any, kind PushMessageType) PushEventBase {
	return PushEventBase{serial: serial, raw: raw, kind: kind}
}

// PushEventBase is the exported alias for pushEventBase used only by
// the decoder helpers below. Callers should not use this type directly
// — switch on the concrete typed events instead.
type PushEventBase = pushEventBase

// NewVolumeChangeEvent — decoder constructor.
func NewVolumeChangeEvent(base PushEventBase, volume int, muted bool) *VolumeChangeEvent {
	return &VolumeChangeEvent{pushEventBase: base, Volume: volume, Muted: muted}
}

// NewGenericActivityEvent — decoder constructor.
func NewGenericActivityEvent(b PushEventBase) *GenericActivityEvent {
	return &GenericActivityEvent{pushEventBase: b}
}

// NewConnectionStatusEvent — decoder constructor.
func NewConnectionStatusEvent(b PushEventBase) *ConnectionStatusEvent {
	return &ConnectionStatusEvent{pushEventBase: b}
}

// NewBluetoothStatusEvent — decoder constructor.
func NewBluetoothStatusEvent(b PushEventBase) *BluetoothStatusEvent {
	return &BluetoothStatusEvent{pushEventBase: b}
}

// NewMicrophoneStatusEvent — decoder constructor.
func NewMicrophoneStatusEvent(b PushEventBase) *MicrophoneStatusEvent {
	return &MicrophoneStatusEvent{pushEventBase: b}
}

// NewNotificationChangeEvent — decoder constructor.
func NewNotificationChangeEvent(b PushEventBase) *NotificationChangeEvent {
	return &NotificationChangeEvent{pushEventBase: b}
}

// NewAudioPlayerStateEvent — decoder constructor.
func NewAudioPlayerStateEvent(b PushEventBase) *AudioPlayerStateEvent {
	return &AudioPlayerStateEvent{pushEventBase: b}
}

// NewEqualizerStateChangeEvent — decoder constructor.
func NewEqualizerStateChangeEvent(b PushEventBase) *EqualizerStateChangeEvent {
	return &EqualizerStateChangeEvent{pushEventBase: b}
}

// NewMediaQueueChangeEvent — decoder constructor.
func NewMediaQueueChangeEvent(b PushEventBase) *MediaQueueChangeEvent {
	return &MediaQueueChangeEvent{pushEventBase: b}
}

// NewMediaChangeEvent — decoder constructor.
func NewMediaChangeEvent(b PushEventBase) *MediaChangeEvent {
	return &MediaChangeEvent{pushEventBase: b}
}

// NewMediaProgressChangeEvent — decoder constructor.
func NewMediaProgressChangeEvent(b PushEventBase) *MediaProgressChangeEvent {
	return &MediaProgressChangeEvent{pushEventBase: b}
}

// NewMediaSessionsUpdatedEvent — decoder constructor.
func NewMediaSessionsUpdatedEvent(b PushEventBase) *MediaSessionsUpdatedEvent {
	return &MediaSessionsUpdatedEvent{pushEventBase: b}
}

// NewNowPlayingUpdatedEvent — decoder constructor.
func NewNowPlayingUpdatedEvent(b PushEventBase) *NowPlayingUpdatedEvent {
	return &NowPlayingUpdatedEvent{pushEventBase: b}
}

// NewItemChangeEvent — decoder constructor.
func NewItemChangeEvent(b PushEventBase) *ItemChangeEvent {
	return &ItemChangeEvent{pushEventBase: b}
}

// NewMatterDeviceFoundEvent — decoder constructor.
func NewMatterDeviceFoundEvent(b PushEventBase) *MatterDeviceFoundEvent {
	return &MatterDeviceFoundEvent{pushEventBase: b}
}
