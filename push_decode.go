package amazondevices

import (
	"encoding/json"
	"fmt"
)

// DecodeRenderingUpdate parses one part of the AVS directives stream
// (already extracted from its multipart envelope by the
// internal/http2stream.Reader) and returns zero or more typed events.
// The slice may be empty when the part contained only duplicate or
// malformed updates.
//
// Unknown resourceId values produce *UnknownPushEvent rather than
// errors — known-unknowns are useful diagnostics for callers tailing
// the stream.
//
// Lives in the root package (not internal/http2stream) so the stream
// sub-package has no dependency on the public event types and
// push.go can safely import internal/http2stream for Conn/Ping/Reader.
func DecodeRenderingUpdate(part []byte) ([]PushEvent, error) {
	var envelope struct {
		Directive struct {
			Payload struct {
				RenderingUpdates []map[string]any `json:"renderingUpdates"`
			} `json:"payload"`
		} `json:"directive"`
	}
	if err := json.Unmarshal(part, &envelope); err != nil {
		return nil, fmt.Errorf("amazondevices: decode envelope: %w", err)
	}

	updates := envelope.Directive.Payload.RenderingUpdates
	out := make([]PushEvent, 0, len(updates))
	for _, u := range updates {
		evt, ok := decodeOne(u)
		if !ok {
			continue
		}
		out = append(out, evt)
	}
	return out, nil
}

// decodeOne turns a single rendering-update node into a typed event.
// The second return value is false when the node was malformed and
// should be silently dropped (logged by the caller, if it cares).
func decodeOne(node map[string]any) (PushEvent, bool) {
	wireType, _ := node["resourceId"].(string)
	if wireType == "" {
		return nil, false
	}
	meta, _ := node["resourceMetadata"].(map[string]any)
	if meta == nil {
		return nil, false
	}
	payload, _ := meta["payload"].(map[string]any)
	if payload == nil {
		return nil, false
	}

	if wireType == string(PushNotificationChange) &&
		isNotificationChangeDuplicate(payload) {
		return nil, false
	}

	serial := extractSerial(payload)
	parsed, parseErr := ParsePushMessageType(wireType)
	if parseErr != nil {
		return &UnknownPushEvent{
			WireType: wireType,
			Serial:   serial,
			Payload:  payload,
		}, true
	}
	return buildTypedEvent(parsed, serial, payload), true
}

// extractSerial pulls payload.dopplerId.deviceSerialNumber out of the
// payload map, returning "" when any step is missing or the wrong type.
func extractSerial(payload map[string]any) string {
	doppler, _ := payload["dopplerId"].(map[string]any)
	if doppler == nil {
		return ""
	}
	s, _ := doppler["deviceSerialNumber"].(string)
	return s
}

// isNotificationChangeDuplicate filters the even-version duplicates
// that the Amazon API emits for every NotificationChange. Upstream
// observation: every odd-version event is followed by an even-version
// event carrying identical data, and the even one is always redundant.
// Missing or non-integer notificationVersion is treated as not-a-dup
// (the safer default — surface, don't drop).
func isNotificationChangeDuplicate(payload map[string]any) bool {
	v, ok := payload["notificationVersion"]
	if !ok {
		return false
	}
	// JSON numbers decode as float64 in encoding/json.
	f, ok := v.(float64)
	if !ok {
		return false
	}
	if f != float64(int64(f)) {
		// Not an integer — surface.
		return false
	}
	return int64(f)%2 == 0
}

// buildTypedEvent constructs the concrete event struct for a recognised
// PushMessageType. VolumeChange gets its typed Volume/Muted fields
// populated; every other type ships with only the interface methods.
func buildTypedEvent(kind PushMessageType, serial string, payload map[string]any) PushEvent {
	base := NewPushEventBase(serial, payload, kind)
	switch kind {
	case PushVolumeChange:
		vol, _ := payload["volumeSetting"].(float64)
		muted, _ := payload["isMuted"].(bool)
		return NewVolumeChangeEvent(base, int(vol), muted)
	case PushGenericActivity:
		return NewGenericActivityEvent(base)
	case PushConnectionStatus:
		return NewConnectionStatusEvent(base)
	case PushBluetoothStatus:
		return NewBluetoothStatusEvent(base)
	case PushMicrophoneStatus:
		return NewMicrophoneStatusEvent(base)
	case PushNotificationChange:
		return NewNotificationChangeEvent(base)
	case PushAudioPlayerState:
		return NewAudioPlayerStateEvent(base)
	case PushEqualizerStateChange:
		return NewEqualizerStateChangeEvent(base)
	case PushMediaQueueChange:
		return NewMediaQueueChangeEvent(base)
	case PushMediaChange:
		return NewMediaChangeEvent(base)
	case PushMediaProgressChange:
		return NewMediaProgressChangeEvent(base)
	case PushMediaSessionsUpdated:
		return NewMediaSessionsUpdatedEvent(base)
	case PushNowPlayingUpdated:
		return NewNowPlayingUpdatedEvent(base)
	case PushItemChange:
		return NewItemChangeEvent(base)
	case PushMatterDeviceFound:
		return NewMatterDeviceFoundEvent(base)
	}
	// Defensive: ParsePushMessageType returned no error but the enum
	// value isn't one we know how to build. Treat as unknown.
	return &UnknownPushEvent{
		WireType: string(kind),
		Serial:   serial,
		Payload:  payload,
	}
}
