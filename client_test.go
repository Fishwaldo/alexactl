package amazondevices

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func TestNewClient_NoOptions(t *testing.T) {
	client, err := NewClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.Auth)
	assert.NotNil(t, client.Devices)
	assert.NotNil(t, client.Media)
}

func TestNewClient_WithSession(t *testing.T) {
	sess := &Session{
		AccessToken:       "tok",
		RefreshToken:      "ref",
		Site:              "https://www.amazon.com",
		Domain:            "com",
		AccountCustomerID: "OWNER",
	}
	client, err := NewClient(WithSession(sess))
	require.NoError(t, err)
	assert.Equal(t, sess, client.Auth.Session())
}

func TestNewClient_NilContextSafetyForFutureMethods(t *testing.T) {
	client, err := NewClient()
	require.NoError(t, err)
	_ = context.Background()
	_ = client
}

// TestNewClient_WithSessionBroadcasts proves that passing WithSession at
// construction time propagates the *Session to both Devices and Media,
// not just Auth.
func TestNewClient_WithSessionBroadcasts(t *testing.T) {
	sess := &Session{
		AccessToken:       "tok",
		RefreshToken:      "ref",
		Site:              "https://www.amazon.com",
		Domain:            "com",
		AccountCustomerID: "OWNER",
	}
	c, err := NewClient(WithSession(sess))
	require.NoError(t, err)
	assert.Equal(t, sess, c.Auth.Session())
	assert.Equal(t, sess, c.Devices.snapshotSession())
	assert.Equal(t, sess, c.Media.snapshotSession())
}

// TestClient_PostLoginPropagatesSession runs a full Login against a mock
// server and asserts that the resulting Session is also visible to
// Devices and Media (not just Auth).
func TestClient_PostLoginPropagatesSession(t *testing.T) {
	srv := fullAuthMockServer(t)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	c, err := NewClient(
		WithHTTPClient(hc),
		WithSite(srv.URL),
	)
	require.NoError(t, err)

	// Override registerURL / sleepFn the same way auth_login_test does.
	c.Auth.registerURL = srv.URL + "/auth/register"
	c.Auth.sleepFn = func(_ context.Context, _ int) {}

	sess, err := c.Auth.Login(context.Background(), "alice@example.com", "pw",
		OTPProviderFunc(func(_ context.Context) (string, error) { return "111222", nil }))
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Same pointer reaches all three services.
	assert.NotNil(t, c.Auth.Session(), "Auth must hold the session")
	assert.NotNil(t, c.Devices.snapshotSession(), "Devices must hold the session")
	assert.NotNil(t, c.Media.snapshotSession(), "Media must hold the session")
	assert.Equal(t, sess, c.Auth.Session())
	assert.Equal(t, sess, c.Devices.snapshotSession())
	assert.Equal(t, sess, c.Media.snapshotSession())
}

// TestAuthService_LoginFailureLeavesSessionNil exercises the deferred
// s.session assignment from B4: if obtainCustomerID fails, AuthService
// must remain pre-Login (Session() returns nil, Restore returns
// ErrWrongMethod, Devices/Media must not see a half-built session).
func TestAuthService_LoginFailureLeavesSessionNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ap/signin" && r.Method == http.MethodGet:
			base := "http://" + r.Host
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(loginEmailPasswordHTML, "%s", base)))
		case r.URL.Path == "/ap/signin/passwordstep":
			base := "http://" + r.Host
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(otpPageHTML, "%s", base)))
		case r.URL.Path == "/ap/signin/otpstep":
			http.Redirect(w, r,
				"http://"+r.Host+"/ap/maplanding?openid.oa2.authorization_code=CODE-X",
				http.StatusFound)
		case r.URL.Path == "/ap/maplanding":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/auth/register":
			w.Header().Set("Content-Type", "application/json")
			_ = writeRegisterSuccess(w)
		default:
			// /api/devices-v2/device: return an empty list so the customer-ID
			// lookup retries and ultimately fails with ErrCannotRetrieveData.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"devices":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	c, err := NewClient(WithHTTPClient(hc), WithSite(srv.URL))
	require.NoError(t, err)
	c.Auth.registerURL = srv.URL + "/auth/register"
	c.Auth.sleepFn = func(_ context.Context, _ int) {}

	_, err = c.Auth.Login(context.Background(), "alice@example.com", "pw",
		OTPProviderFunc(func(_ context.Context) (string, error) { return "111222", nil }))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRetrieveData)

	// AuthService must remain unauthenticated; nothing should have leaked
	// to Devices or Media.
	assert.Nil(t, c.Auth.Session(), "Auth.Session must remain nil after partial Login failure")
	assert.Nil(t, c.Devices.snapshotSession(), "Devices must not see a half-built session")
	assert.Nil(t, c.Media.snapshotSession(), "Media must not see a half-built session")

	// Restore must reject (no committed session).
	_, err = c.Auth.Restore(context.Background())
	assert.ErrorIs(t, err, ErrWrongMethod)
}

// TestAuthService_ConcurrentRefreshDoesNotRace runs many goroutines reading
// session state via the public surfaces while another goroutine triggers
// refreshAccessToken. With the mutex protection from B3, `-race` is clean;
// without it, the race detector trips.
func TestAuthService_ConcurrentRefreshDoesNotRace(t *testing.T) {
	// Mock token endpoint: hand back a fresh access token each call.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"NEW-TOK","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: tokenSrv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	c := &Client{
		wrapper:       wr,
		Auth:          &AuthService{wrapper: wr, tokenURL: tokenSrv.URL},
		Devices:       &DevicesService{wrapper: wr},
		Media:         &MediaService{wrapper: wr},
		Sequence:      &SequenceService{wrapper: wr},
		DnD:           &DnDService{wrapper: wr},
		Sensors:       &SensorsService{wrapper: wr},
		Notifications: &NotificationsService{wrapper: wr},
		History:       &HistoryService{wrapper: wr},
		Push:          &PushService{wrapper: wr},
	}
	c.Auth.consumers = []sessionConsumer{
		c.Devices, c.Media, c.Sequence, c.DnD, c.Sensors, c.Notifications, c.History, c.Push,
	}
	c.Auth.broadcast(&Session{
		AccessToken:       "INITIAL-TOK",
		RefreshToken:      "REFRESH",
		AccountCustomerID: "OWNER",
		Domain:            "com",
	})

	var wg sync.WaitGroup
	ctx := context.Background()

	// Reader goroutines: hammer the read sides of all services.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = c.Auth.Session()
				_ = c.Devices.snapshotSession()
				_ = c.Media.snapshotSession()
				_ = c.Sequence.snapshotSession()
				_ = c.Push.session()
			}
		}()
	}

	// Writer goroutines: drive refresh repeatedly.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = c.Auth.refreshAccessToken(ctx)
			}
		}()
	}

	wg.Wait()

	// After all refreshes complete, the latest token must have propagated
	// to every service.
	finalAuth := c.Auth.Session()
	finalDev := c.Devices.snapshotSession()
	finalMed := c.Media.snapshotSession()
	finalSeq := c.Sequence.snapshotSession()
	finalPush := c.Push.session()
	require.NotNil(t, finalAuth)
	require.NotNil(t, finalDev)
	require.NotNil(t, finalMed)
	require.NotNil(t, finalSeq)
	require.NotNil(t, finalPush)
	assert.Equal(t, "NEW-TOK", finalAuth.AccessToken)
	assert.Equal(t, "NEW-TOK", finalDev.AccessToken)
	assert.Equal(t, "NEW-TOK", finalMed.AccessToken)
	assert.Equal(t, "NEW-TOK", finalSeq.AccessToken)
	assert.Equal(t, "NEW-TOK", finalPush.AccessToken)
}

// writeRegisterSuccess writes the same /auth/register success envelope as
// the full-auth mock server in auth_login_test.go.
func writeRegisterSuccess(w http.ResponseWriter) error {
	const body = `{
  "response": {"success": {
    "tokens": {
      "mac_dms": {"adp_token": "adp", "device_private_key": "k"},
      "store_authentication_cookie": {"cookie": "sac"},
      "bearer": {"access_token": "A1", "refresh_token": "R1", "expires_in": "3600"},
      "website_cookies": []
    },
    "extensions": {
      "device_info": {"device_serial_number": "S1", "device_type": "A2IVLV5VM2W81"},
      "customer_info": {"home_region": "NA", "account_id": "acct", "name": "n", "given_name": "g"}
    }
  }}
}`
	_, err := w.Write([]byte(body))
	return err
}
