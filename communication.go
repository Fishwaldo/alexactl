// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// CommunicationsService toggles per-device communications, announcement,
// and drop-in preferences via Amazon's global comms host. Bearer-authed.
//
// Safe for concurrent use.
type CommunicationsService struct {
	wrapper *httpwrapper.Wrapper

	mu      sync.RWMutex
	session *Session

	// commSite is amzconst.CommSite in production; overridable by tests.
	commSite string
}

func (s *CommunicationsService) setSession(sess *Session) {
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
}

func (s *CommunicationsService) site() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.commSite != "" {
		return s.commSite
	}
	return amzconst.CommSite
}

func (s *CommunicationsService) accessToken() (string, error) {
	s.mu.RLock()
	sess := s.session
	s.mu.RUnlock()
	if sess == nil || sess.AccessToken == "" {
		return "", fmt.Errorf("%w: no access token", ErrCannotAuthenticate)
	}
	return sess.AccessToken, nil
}

func (s *CommunicationsService) prefBase(d *Device) string {
	return s.site() + fmt.Sprintf(amzconst.URICommPreferences, d.DeviceType, d.SerialNumber)
}

func (s *CommunicationsService) setState(ctx context.Context, pref string, d *Device, state string) error {
	tok, err := s.accessToken()
	if err != nil {
		return err
	}
	u := s.prefBase(d) + "/" + pref
	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPatch,
		URL:      u,
		JSONBody: map[string]any{"state": state},
		ExtraHeaders: http.Header{
			"Authorization": []string{"Bearer " + tok},
			"User-Agent":    []string{amzconst.UserAgentAmazon},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &APIError{Category: ErrCannotRetrieveData, Method: http.MethodPatch, StatusCode: resp.StatusCode, URL: u}
	}
	return nil
}

// SetCommunicationStatus enables/disables calling+messaging for the device.
func (s *CommunicationsService) SetCommunicationStatus(ctx context.Context, d *Device, on bool) error {
	return s.setState(ctx, "communications", d, onOff(on))
}

// SetAnnouncementStatus enables/disables announcements for the device.
func (s *CommunicationsService) SetAnnouncementStatus(ctx context.Context, d *Device, on bool) error {
	return s.setState(ctx, "announcements", d, onOff(on))
}

// SetDropInStatus sets the drop-in permission (All / Home / Off).
func (s *CommunicationsService) SetDropInStatus(ctx context.Context, d *Device, status DropInStatus) error {
	return s.setState(ctx, "dropin", d, string(status))
}

func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

// GetPreferences fetches allowed comms preferences per device serial.
// Devices in a speaker group (WHA) or AQMs are skipped. Per-device
// fetch failures are logged and skipped, not fatal.
func (s *CommunicationsService) GetPreferences(ctx context.Context, devices []*Device) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	tok, err := s.accessToken()
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.DeviceFamily == amzconst.SpeakerGroupFamily || d.DeviceType == amzconst.DeviceTypeAQM {
			continue
		}
		q := url.Values{}
		q.Set("devicePreferences", "communications,dropin,announcements")
		u := s.prefBase(d) + "?" + q.Encode()
		resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
			Method: http.MethodGet,
			URL:    u,
			ExtraHeaders: http.Header{
				"Authorization": []string{"Bearer " + tok},
				"User-Agent":    []string{amzconst.UserAgentAmazon},
			},
		})
		if err != nil {
			s.wrapper.Logger(ctx).WarnContext(ctx, "alexactl: comms prefs fetch failed; skipping",
				"device", d.AccountName, "err", err)
			continue
		}
		func() {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode >= 400 {
				s.wrapper.Logger(ctx).WarnContext(ctx, "alexactl: comms prefs non-OK; skipping",
					"device", d.AccountName, "status", resp.StatusCode)
				return
			}
			data, derr := httpwrapper.DecodeJSON(resp)
			if derr != nil {
				s.wrapper.Logger(ctx).WarnContext(ctx, "alexactl: comms prefs decode failed; skipping",
					"device", d.AccountName, "err", derr)
				return
			}
			prefs := map[string]string{}
			raw, _ := data["devicePermissionsPreferences"].([]any)
			for _, pi := range raw {
				p, _ := pi.(map[string]any)
				if p == nil {
					continue
				}
				if allowed, _ := p["allowed"].(bool); !allowed {
					continue
				}
				name, _ := p["devicePreference"].(string)
				state, _ := p["state"].(string)
				if name != "" {
					prefs[name] = state
				}
			}
			out[d.SerialNumber] = prefs
		}()
	}
	return out, nil
}
