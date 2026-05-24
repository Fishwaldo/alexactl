package amazondevices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// MediaService sends Alexa.Speak / AlexaAnnouncement / SetVolume operations
// to one or more devices via the /api/behaviors/preview endpoint.
type MediaService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session
}

// setSession swaps in a fresh *Session pointer. Invoked by AuthService
// after Login / Restore / refreshAccessToken.
func (s *MediaService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// snapshotSession returns the current *Session pointer under the read
// lock. Callers may dereference the returned pointer freely — AuthService
// always swaps in a fresh pointer rather than mutating in place.
func (s *MediaService) snapshotSession() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

// Speak utters text on the given device (and on each cluster member if
// the device is part of a group).
func (s *MediaService) Speak(ctx context.Context, d *Device, text string) error {
	nodes, err := s.buildClusterNodes(d, SeqSpeak, text)
	if err != nil {
		return err
	}
	return s.postSequence(ctx, nodes)
}

// Announce broadcasts a message on the device's cluster. Cluster fanout
// happens inside the single operation payload (target.devices), so only
// one operation node is needed.
func (s *MediaService) Announce(ctx context.Context, d *Device, text string) error {
	sess := s.snapshotSession()
	if sess == nil || sess.AccountCustomerID == "" {
		return fmt.Errorf("%w: missing customer ID", ErrCannotAuthenticate)
	}
	members := sortedKeys(d.ClusterMembers)
	if len(members) == 0 {
		members = []string{d.SerialNumber}
	}
	devices := make([]map[string]any, 0, len(members))
	for _, serial := range members {
		dt := d.ClusterMembers[serial]
		if dt == "" {
			dt = d.DeviceType
		}
		devices = append(devices, map[string]any{
			"deviceSerialNumber": serial,
			"deviceTypeId":       dt,
		})
	}
	payload := map[string]any{
		"deviceType":         d.DeviceType,
		"deviceSerialNumber": d.SerialNumber,
		"locale":             s.wrapper.Language(),
		"customerId":         sess.AccountCustomerID,
		"expireAfter":        "PT5S",
		"content": []any{
			map[string]any{
				"locale":  s.wrapper.Language(),
				"display": map[string]any{"title": "goamazondevices", "body": text},
				"speak":   map[string]any{"type": "text", "value": text},
			},
		},
		"target": map[string]any{
			"customerId": sess.AccountCustomerID,
			"devices":    devices,
		},
		"skillId": "amzn1.ask.1p.routines.messaging",
	}
	node := s.opaqueNode(string(SeqAnnouncement), payload)
	return s.postSequence(ctx, []map[string]any{node})
}

// SetVolume sets a volume in [0, 100] on each cluster member.
func (s *MediaService) SetVolume(ctx context.Context, d *Device, vol int) error {
	if vol < amzconst.VolumeMin || vol > amzconst.VolumeMax {
		return fmt.Errorf("amazondevices: volume %d not in [%d,%d]",
			vol, amzconst.VolumeMin, amzconst.VolumeMax)
	}
	nodes, err := s.buildClusterNodes(d, SeqVolume, vol)
	if err != nil {
		return err
	}
	return s.postSequence(ctx, nodes)
}

// buildClusterNodes constructs one operation node per cluster member for
// per-member fanout (Speak, SetVolume).
func (s *MediaService) buildClusterNodes(d *Device, msgType SequenceType, msgBody any) ([]map[string]any, error) {
	sess := s.snapshotSession()
	if sess == nil || sess.AccountCustomerID == "" {
		return nil, fmt.Errorf("%w: missing customer ID", ErrCannotAuthenticate)
	}
	members := sortedKeys(d.ClusterMembers)
	if len(members) == 0 {
		members = []string{d.SerialNumber}
	}
	out := make([]map[string]any, 0, len(members))
	for _, serial := range members {
		dt := d.ClusterMembers[serial]
		if dt == "" {
			dt = d.DeviceType
		}
		var payload map[string]any
		switch msgType {
		case SeqSpeak:
			payload = map[string]any{
				"deviceType":         dt,
				"deviceSerialNumber": serial,
				"locale":             s.wrapper.Language(),
				"customerId":         sess.AccountCustomerID,
				"textToSpeak":        msgBody,
				"target": map[string]any{
					"customerId": sess.AccountCustomerID,
					"devices": []any{
						map[string]any{
							"deviceSerialNumber": serial,
							"deviceTypeId":       dt,
						},
					},
				},
				"skillId": "amzn1.ask.1p.saysomething",
			}
		case SeqVolume:
			payload = map[string]any{
				"deviceType":         dt,
				"deviceSerialNumber": serial,
				"locale":             s.wrapper.Language(),
				"customerId":         sess.AccountCustomerID,
				"value":              msgBody,
			}
		default:
			return nil, fmt.Errorf("amazondevices: media: unsupported message type %s", msgType)
		}
		out = append(out, s.opaqueNode(string(msgType), payload))
	}
	return out, nil
}

func (s *MediaService) opaqueNode(messageType string, payload map[string]any) map[string]any {
	return map[string]any{
		"@type":            "com.amazon.alexa.behaviors.model.OpaquePayloadOperationNode",
		"type":             messageType,
		"operationPayload": payload,
	}
}

// postSequence POSTs the operation nodes to /api/behaviors/preview, wrapping
// them in a SerialNode.
func (s *MediaService) postSequence(ctx context.Context, nodes []map[string]any) error {
	if len(nodes) == 0 {
		return nil
	}
	s.wrapper.Logger(ctx).DebugContext(ctx, "amazondevices: sending sequence",
		"node_count", len(nodes))

	seq := map[string]any{
		"@type": "com.amazon.alexa.behaviors.model.Sequence",
		"startNode": map[string]any{
			"@type":          "com.amazon.alexa.behaviors.model.SerialNode",
			"nodesToExecute": nodes,
		},
	}
	seqJSON, err := json.Marshal(seq)
	if err != nil {
		return fmt.Errorf("amazondevices: marshal sequence: %w", err)
	}

	body := map[string]any{
		"behaviorId":   "PREVIEW",
		"sequenceJson": string(seqJSON),
		"status":       "ENABLED",
	}

	url := fmt.Sprintf("https://alexa.amazon.%s%s", s.wrapper.Domain(), amzconst.URIBehaviorsPreview)
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPost,
		URL:      url,
		JSONBody: body,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
