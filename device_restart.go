// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package alexactl

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// Restart reboots the given device via Amazon's reboot endpoint. The
// device drops offline for ~30s and reconnects. Requires the account's
// OAuth access token (bearer-authed, not cookie-authed).
//
// Returns an error wrapping ErrCannotRestartDevice on a non-2xx response,
// ErrCannotAuthenticate if no access token is available, or
// ErrCannotConnect on transport failure.
func (s *DevicesService) Restart(ctx context.Context, d *Device) error {
	sess := s.snapshotSession()
	if sess == nil || sess.AccessToken == "" {
		return fmt.Errorf("%w: no access token", ErrCannotAuthenticate)
	}

	host := s.alexaHost()
	url := fmt.Sprintf("%s%s", trimTrailingSlash(host),
		fmt.Sprintf(amzconst.URIReboot, d.DeviceType, d.SerialNumber))

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method: http.MethodPost,
		URL:    url,
		JSONBody: map[string]any{
			"deviceSerialNumber": d.SerialNumber,
			"deviceType":         d.DeviceType,
		},
		ExtraHeaders: http.Header{
			"Authorization": []string{"Bearer " + sess.AccessToken},
			"User-Agent":    []string{amzconst.UserAgentAmazon},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return &APIError{
			Category:   ErrCannotRestartDevice,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        url,
		}
	}
	return nil
}
