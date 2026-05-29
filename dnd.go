package amazondevices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/cache"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// DnDService gets and sets the per-device Do Not Disturb flag.
//
// Backed by two endpoints on alexa.amazon.<domain>:
//   - GET  /api/dnd/device-status-list  — read all devices' DnD state
//   - PUT  /api/dnd/status               — toggle DnD on one device
//
// Safe for concurrent use.
type DnDService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	// statusCache caches Status() for the Short-class TTL. Set() invalidates
	// it on successful write.
	statusCache *cache.Cache[map[string]*DeviceSensor]
}

// setSession swaps in a fresh *Session pointer. Invoked by AuthService
// after Login / Restore / refreshAccessToken via the broadcast pipeline.
func (s *DnDService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

// Status returns the Do Not Disturb state for every device on the
// account, keyed by device serial number. Cached for the Short-class TTL.
// Returned map is a defensive copy.
func (s *DnDService) Status(ctx context.Context) (map[string]*DeviceSensor, error) {
	got, _, err := s.statusCache.Get(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]*DeviceSensor, len(got))
	for k, v := range got {
		cp := *v
		out[k] = &cp
	}
	return out, nil
}

// fetchStatus is the loader for dnd.status.
func (s *DnDService) fetchStatus(ctx context.Context) (map[string]*DeviceSensor, error) {
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIDnDStatusAll)

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodGet,
		URL:    url,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp.StatusCode >= 400 {
		body := resp.Body
		defer body.Close()
		return nil, &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodGet,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}

	data, err := httpwrapper.DecodeJSON(resp)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotRetrieveData, err)
	}

	out := map[string]*DeviceSensor{}
	list, _ := data["doNotDisturbDeviceStatusList"].([]any)
	for _, entry := range list {
		row, _ := entry.(map[string]any)
		if row == nil {
			continue
		}
		serial, _ := row["deviceSerialNumber"].(string)
		if serial == "" {
			continue
		}
		enabled, _ := row["enabled"].(bool)
		out[serial] = &DeviceSensor{
			Name:  "dnd",
			Value: enabled,
		}
	}
	return out, nil
}

// Set toggles the Do Not Disturb flag for one device. The change applies
// only to the supplied device's specific (serial, deviceType) pair — DnD
// is not a cluster-fanout operation in Amazon's API.
func (s *DnDService) Set(ctx context.Context, d *Device, enabled bool) error {
	if d == nil {
		return fmt.Errorf("amazondevices: DnD.Set: nil device")
	}
	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		s.wrapper.Domain(), amzconst.URIDnDStatusDevice)

	payload := map[string]any{
		"deviceSerialNumber": d.SerialNumber,
		"deviceType":         d.DeviceType,
		"enabled":            enabled,
	}

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPut,
		URL:      url,
		JSONBody: payload,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRetrieveData,
			Method:     http.MethodPut,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	// Drop the cached snapshot so the next Status reflects the change.
	if s.statusCache != nil {
		s.statusCache.InvalidateAll()
	}
	return nil
}

// marshalDnDEntries / unmarshalDnDEntries persist the dnd.status cache.
func marshalDnDEntries(in map[string]cache.Entry[map[string]*DeviceSensor]) ([]byte, error) {
	return json.Marshal(in)
}

func unmarshalDnDEntries(b []byte) (map[string]cache.Entry[map[string]*DeviceSensor], error) {
	out := map[string]cache.Entry[map[string]*DeviceSensor]{}
	err := json.Unmarshal(b, &out)
	return out, err
}
