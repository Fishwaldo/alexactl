package amazondevices

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/oauth"
)

// newSerialNumber returns a 32-char uppercase hex device serial, matching
// Python's uuid.uuid4().hex.upper() shape used by AmazonLogin._serial_number.
// Returns a non-nil error if the OS random source fails.
func newSerialNumber() (string, error) {
	return newSerialNumberFrom(rand.Reader)
}

// newSerialNumberFrom is the test seam for newSerialNumber. Production code
// should call newSerialNumber, which uses crypto/rand.Reader.
func newSerialNumberFrom(r io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("auth: read random for serial number: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(b[:])), nil
}

// buildClientID returns the OAuth client_id derived from the device serial
// number and Amazon's device type. Format mirrors the Python:
//
//	hex(<serial> + "#" + <device_type>)
func buildClientID(serial string) string {
	return strings.ToLower(hex.EncodeToString([]byte(serial + "#" + amzconst.AmazonDeviceType)))
}

// buildOAuthURL constructs the URL to GET to begin the OAuth flow for the
// Amazon Alexa iOS device emulation.
func buildOAuthURL(verifier, clientID, registrationLanguage string) string {
	q := url.Values{}
	q.Set("openid.return_to", "https://www.amazon.com/ap/maplanding")
	q.Set("openid.oa2.code_challenge_method", "S256")
	q.Set("openid.assoc_handle", "amzn_dp_project_dee_ios")
	q.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("pageId", "amzn_dp_project_dee_ios")
	q.Set("accountStatusPolicy", "P1")
	q.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")
	q.Set("openid.mode", "checkid_setup")
	q.Set("openid.ns.oa2", "http://www.amazon.com/ap/ext/oauth/2")
	q.Set("openid.oa2.client_id", "device:"+clientID)
	q.Set("language", registrationLanguage)
	q.Set("openid.ns.pape", "http://specs.openid.net/extensions/pape/1.0")
	q.Set("openid.oa2.code_challenge", oauth.S256Challenge(verifier))
	q.Set("openid.oa2.scope", "device_auth_access")
	q.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	q.Set("openid.pape.max_auth_age", "0")
	q.Set("openid.oa2.response_type", "code")

	return fmt.Sprintf("https://www.amazon.com%s?%s", amzconst.URISignin, q.Encode())
}

// extractAuthCodeFromURL pulls the authorization code from the redirect URL
// returned at the end of the OAuth dance.
func extractAuthCodeFromURL(u *url.URL) (string, error) {
	if u == nil {
		return "", fmt.Errorf("auth: nil redirect URL")
	}
	c := u.Query().Get("openid.oa2.authorization_code")
	if c == "" {
		return "", fmt.Errorf("auth: redirect URL missing authorization_code: %s", u.String())
	}
	return c, nil
}
