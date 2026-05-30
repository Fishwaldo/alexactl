package alexactl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// fullAuthMockServer responds to every path needed for a complete Login.
func fullAuthMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch {
		case r.URL.Path == "/ap/signin" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(loginEmailPasswordHTML, "%s", base)))
		case r.URL.Path == "/ap/signin/passwordstep":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(otpPageHTML, "%s", base)))
		case r.URL.Path == "/ap/signin/otpstep":
			http.Redirect(w, r, base+"/ap/maplanding?openid.oa2.authorization_code=CODE-X", http.StatusFound)
		case r.URL.Path == "/ap/maplanding":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/auth/register":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"success": map[string]any{
						"tokens": map[string]any{
							"mac_dms":                     map[string]any{"adp_token": "adp", "device_private_key": "k"},
							"store_authentication_cookie": map[string]any{"cookie": "sac"},
							"bearer": map[string]any{
								"access_token":  "A1",
								"refresh_token": "R1",
								"expires_in":    "3600",
							},
							"website_cookies": []any{},
						},
						"extensions": map[string]any{
							"device_info":   map[string]any{"device_serial_number": "S1", "device_type": amzconst.AmazonDeviceType},
							"customer_info": map[string]any{"home_region": "NA", "account_id": "acct", "name": "n", "given_name": "g"},
						},
					},
				},
			})
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"devices": []any{
					map[string]any{
						"deviceType":            amzconst.AmazonDeviceType,
						"serialNumber":          "X",
						"accountName":           "n",
						"deviceOwnerCustomerId": "OWNER",
						"appDeviceList":         []any{map[string]any{"serialNumber": "S1"}},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLogin_EndToEnd(t *testing.T) {
	srv := fullAuthMockServer(t)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	auth := &AuthService{
		wrapper:     wr,
		registerURL: srv.URL + "/auth/register",
		sleepFn:     func(ctx context.Context, _ int) {},
	}

	sess, err := auth.Login(context.Background(), "alice@example.com", "pw",
		OTPProviderFunc(func(_ context.Context) (string, error) { return "111222", nil }))
	require.NoError(t, err)
	assert.Equal(t, "A1", sess.AccessToken)
	assert.Equal(t, "R1", sess.RefreshToken)
	assert.Equal(t, "S1", sess.DeviceInfo.DeviceSerialNumber)
	assert.Equal(t, "OWNER", sess.AccountCustomerID)
}

// hostRewriter routes any request to the test server's base URL,
// preserving the request's path. This lets the code use the real
// "https://www.amazon.com" / "alexa.amazon.com" paths internally.
type hostRewriter struct {
	base  string
	inner http.RoundTripper
}

func (h *hostRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	parsedBase, _ := req.URL.Parse(h.base)
	req.URL.Scheme = parsedBase.Scheme
	req.URL.Host = parsedBase.Host
	req.Host = parsedBase.Host
	// When the production code formats a URL using a domain that itself
	// contains a "://" (e.g., a test Site of "http://127.0.0.1:NNNN"), the
	// resulting parsed path is "//127.0.0.1:NNNN/<real path>". Strip that
	// stray host-like prefix so tests can assert on the real path.
	if strings.HasPrefix(req.URL.Path, "//"+parsedBase.Host) {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "//"+parsedBase.Host)
	}
	t := h.inner
	if t == nil {
		t = http.DefaultTransport
	}
	return t.RoundTrip(req)
}
