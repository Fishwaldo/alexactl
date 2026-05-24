package amazondevices

import "time"

// sessionConsumer is the internal interface implemented by services that
// hold a *Session pointer. AuthService.broadcast pushes a new *Session
// pointer to every consumer atomically (each consumer is responsible for
// its own locking).
type sessionConsumer interface {
	setSession(*Session)
}

// Session is the full authenticated state returned by AuthService.Login.
// Marshal it to disk with json.Marshal and pass it back via WithSession on
// the next run. Contains long-lived refresh tokens and cookies — protect
// the file (mode 0600 on POSIX).
type Session struct {
	// OAuth tokens
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ADPToken         string    `json:"adp_token"`
	DevicePrivateKey string    `json:"device_private_key"`
	ExpiresAt        time.Time `json:"expires_at"`

	// Cookies re-applied per request.
	// WebsiteCookies is immutable after a Session is returned by NewClient/Auth.Login/Auth.Restore — services treat the map as read-only.
	WebsiteCookies  map[string]string `json:"website_cookies"`
	StoreAuthCookie string            `json:"store_authentication_cookie"`

	// Registered device identity
	DeviceInfo   DeviceInfo   `json:"device_info"`
	CustomerInfo CustomerInfo `json:"customer_info"`

	// Region / domain state
	Site              string `json:"site"`   // e.g. "https://www.amazon.com"
	Domain            string `json:"domain"` // e.g. "com", "co.jp"
	AccountCustomerID string `json:"account_customer_id"`

	// Phase-3 marker — true once device capabilities are registered.
	CapabilitiesRegistered bool `json:"capabilities_registered,omitempty"`
}

// Expired reports whether the access token has expired relative to now.
// Refresh-token lifetime is much longer; refresh logic lives in the HTTP
// wrapper.
func (s *Session) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt)
}

// DeviceInfo identifies the virtual Alexa client device that this Session
// is registered as.
type DeviceInfo struct {
	DeviceSerialNumber string `json:"device_serial_number"`
	DeviceType         string `json:"device_type"`
}

// CustomerInfo carries the Amazon account customer metadata returned at
// registration time.
type CustomerInfo struct {
	HomeRegion string `json:"home_region"` // "NA" | "EU" | "FE" | ...
	AccountID  string `json:"account_id"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
}
