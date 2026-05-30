package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// registerInput carries the data needed by registerDevice.
type registerInput struct {
	AuthorizationCode string
	CodeVerifier      string
	Serial            string
	ClientIDHex       string

	// RegisterURL defaults to https://api.amazon.com/auth/register.
	RegisterURL string
}

// registerDevice posts to /auth/register with the authorization code
// from the OAuth dance and returns a fully populated *Session.
func (s *AuthService) registerDevice(ctx context.Context, in registerInput) (*Session, error) {
	regURL := in.RegisterURL
	if regURL == "" {
		regURL = "https://api.amazon.com/auth/register"
	}
	clientID := in.ClientIDHex
	if clientID == "" {
		clientID = buildClientID(in.Serial)
	}

	body := map[string]any{
		"requested_extensions": []string{"device_info", "customer_info"},
		"cookies": map[string]any{
			"website_cookies": []any{},
			"domain":          fmt.Sprintf(".amazon.%s", s.wrapper.Domain()),
		},
		"registration_data": map[string]any{
			"domain":           "Device",
			"app_version":      amzconst.AmazonAppVersion,
			"device_type":      amzconst.AmazonDeviceType,
			"device_name":      "%FIRST_NAME%'s%DUPE_STRATEGY_1ST%" + amzconst.AmazonAppName,
			"os_version":       amzconst.AmazonClientOS,
			"device_serial":    in.Serial,
			"device_model":     "iPhone",
			"app_name":         amzconst.AmazonAppName,
			"software_version": amzconst.AmazonDeviceSoftwareVersion,
		},
		"auth_data": map[string]any{
			"use_global_authentication": "true",
			"client_id":                 clientID,
			"authorization_code":        in.AuthorizationCode,
			"code_verifier":             in.CodeVerifier,
			"code_algorithm":            "SHA-256",
			"client_domain":             "DeviceLegacy",
		},
		"user_context_map": map[string]any{
			"frc": s.cookieValue("frc"),
		},
		"requested_token_type": []string{
			"bearer", "mac_dms", "website_cookies", "store_authentication_cookie",
		},
	}

	resp, err := s.wrapper.Do(ctx, httpwrapper.Request{
		Method:   http.MethodPost,
		URL:      regURL,
		JSONBody: body,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, decErr := httpwrapper.DecodeJSON(resp)
	if decErr != nil {
		return nil, decErr
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{
			Category:   ErrCannotRegisterDevice,
			Method:     http.MethodPost,
			StatusCode: resp.StatusCode,
			URL:        regURL,
		}
	}

	success, ok := digMap(data, "response", "success")
	if !ok {
		return nil, fmt.Errorf("%w: malformed register response", ErrCannotRegisterDevice)
	}
	tokens, _ := success["tokens"].(map[string]any)
	exts, _ := success["extensions"].(map[string]any)
	if tokens == nil || exts == nil {
		return nil, fmt.Errorf("%w: register response missing tokens/extensions",
			ErrCannotRegisterDevice)
	}

	macDMS, _ := tokens["mac_dms"].(map[string]any)
	bearer, _ := tokens["bearer"].(map[string]any)
	sac, _ := tokens["store_authentication_cookie"].(map[string]any)
	rawCookies, _ := tokens["website_cookies"].([]any)

	if macDMS == nil || bearer == nil {
		return nil, fmt.Errorf("%w: register response missing mac_dms or bearer",
			ErrCannotRegisterDevice)
	}
	access, accessOK := stringOf(bearer["access_token"])
	refresh, refreshOK := stringOf(bearer["refresh_token"])
	if !accessOK || !refreshOK || access == "" || refresh == "" {
		return nil, fmt.Errorf("%w: register response missing tokens",
			ErrCannotRegisterDevice)
	}

	expiresSec, err := atoi(bearer["expires_in"])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid expires_in: %w", ErrCannotRegisterDevice, err)
	}
	// Missing or zero expires_in defaults to 1h; otherwise honor the value.
	var expiresAt time.Time
	if expiresSec <= 0 {
		expiresAt = time.Now().UTC().Add(time.Hour)
	} else {
		expiresAt = time.Now().UTC().Add(time.Duration(expiresSec) * time.Second)
	}

	cookies := map[string]string{}
	for _, ci := range rawCookies {
		c, ok := ci.(map[string]any)
		if !ok {
			continue
		}
		name, _ := c["Name"].(string)
		val, _ := c["Value"].(string)
		cookies[name] = strings.Trim(val, `"`)
	}

	di, _ := exts["device_info"].(map[string]any)
	ci, _ := exts["customer_info"].(map[string]any)

	adpToken, _ := stringOf(macDMS["adp_token"])
	devPrivKey, _ := stringOf(macDMS["device_private_key"])
	storeAuth, _ := stringOf(sac["cookie"])
	dsn, _ := stringOf(di["device_serial_number"])
	dtype, _ := stringOf(di["device_type"])
	homeRegion, _ := stringOf(ci["home_region"])
	acctID, _ := stringOf(ci["account_id"])
	cName, _ := stringOf(ci["name"])
	gName, _ := stringOf(ci["given_name"])

	sess := &Session{
		AccessToken:      access,
		RefreshToken:     refresh,
		ADPToken:         adpToken,
		DevicePrivateKey: devPrivKey,
		ExpiresAt:        expiresAt,
		WebsiteCookies:   cookies,
		StoreAuthCookie:  storeAuth,
		DeviceInfo: DeviceInfo{
			DeviceSerialNumber: dsn,
			DeviceType:         dtype,
		},
		CustomerInfo: CustomerInfo{
			HomeRegion: homeRegion,
			AccountID:  acctID,
			Name:       cName,
			GivenName:  gName,
		},
		Site:   s.wrapper.Site(),
		Domain: s.wrapper.Domain(),
	}
	s.wrapper.Logger(ctx).DebugContext(ctx, "amazondevices: device registered",
		"serial", sess.DeviceInfo.DeviceSerialNumber)
	return sess, nil
}

// cookieValue returns the value of the named cookie from the wrapper's
// cookie jar, scoped to the current Amazon domain. Returns "" if absent.
// Used by registerDevice to pass the `frc` anti-fraud cookie that Amazon's
// /auth/register endpoint expects in user_context_map.
func (s *AuthService) cookieValue(name string) string {
	client := s.wrapper.HTTPClient()
	if client == nil || client.Jar == nil {
		return ""
	}
	u := &url.URL{Scheme: "https", Host: "www.amazon." + s.wrapper.Domain()}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func digMap(m map[string]any, keys ...string) (map[string]any, bool) {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// stringOf extracts a string value from an opaque JSON map entry. It
// returns (value, true) only when the underlying type is exactly string;
// anything else (including missing keys, which surface as untyped nil)
// returns ("", false). Callers can distinguish "absent / wrong type" from
// "present but empty" with the bool.
func stringOf(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// atoi extracts an integer value from an opaque JSON map entry.
//
// Behavior:
//   - nil (missing key)      → (0, nil)  — caller can default
//   - empty string           → (0, nil)  — caller can default
//   - typed int/int64/float64 → (int(v), nil)
//   - non-numeric string     → (0, error)
//   - any other type         → (0, error)
//
// Returning a typed error for malformed values lets the register-response
// parser distinguish "missing" (which is OK and should default) from
// "garbage" (which means the response is malformed and we should fail).
func atoi(v any) (int, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case string:
		if x == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, fmt.Errorf("atoi: %w", err)
		}
		return n, nil
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case float64:
		return int(x), nil
	}
	return 0, fmt.Errorf("atoi: unexpected type %T", v)
}
