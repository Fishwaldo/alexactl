package amazondevices

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// loginEmailPasswordHTML is the page returned by GET /ap/signin.
const loginEmailPasswordHTML = `<!DOCTYPE html><html><body>
<form name="signIn" method="POST" action="%s/ap/signin/passwordstep">
  <input type="hidden" name="appActionToken" value="tok-123" />
  <input type="hidden" name="appAction" value="SIGNIN_PWD_COLLECT" />
  <input type="email" name="email" />
  <input type="password" name="password" />
</form></body></html>`

// otpPageHTML is returned after a successful password POST.
const otpPageHTML = `<!DOCTYPE html><html><body>
<form method="POST" action="%s/ap/signin/otpstep">
  <input type="hidden" name="otpToken" value="otp-tok-456" />
  <input type="text" id="auth-mfa-otpcode" name="otpCode" />
  <input type="hidden" name="mfaSubmit" value="" />
  <input type="hidden" name="rememberDevice" value="" />
</form></body></html>`

func newAuthTestWrapper(t *testing.T, srv *httptest.Server) *httpwrapper.Wrapper {
	t.Helper()
	w, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	return w
}

// authMockServer returns a server that walks the OAuth dance and redirects
// at the end to a maplanding URL with the authcode in the query.
func authMockServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var step atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		switch r.URL.Path {
		case "/ap/signin":
			step.Store(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(loginEmailPasswordHTML, "%s", base)))
		case "/ap/signin/passwordstep":
			step.Store(2)
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "alice@example.com", r.Form.Get("email"))
			assert.Equal(t, "pw", r.Form.Get("password"))
			assert.Equal(t, "tok-123", r.Form.Get("appActionToken"))
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.ReplaceAll(otpPageHTML, "%s", base)))
		case "/ap/signin/otpstep":
			step.Store(3)
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "111222", r.Form.Get("otpCode"))
			assert.Equal(t, "otp-tok-456", r.Form.Get("otpToken"))
			http.Redirect(w, r, base+"/ap/maplanding?openid.oa2.authorization_code=AUTH-OK-001", http.StatusFound)
		case "/ap/maplanding":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("landing"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &step
}

func TestAuth_OAuthDance_Success(t *testing.T) {
	srv, _ := authMockServer(t)
	wr := newAuthTestWrapper(t, srv)

	auth := &AuthService{wrapper: wr}

	authcode, codeVerifier, serial, err := auth.doOAuthDance(
		context.Background(),
		"alice@example.com", "pw",
		OTPProviderFunc(func(_ context.Context) (string, error) { return "111222", nil }),
		"en-US",
		func(verifier, clientID, lang string) string {
			u := srv.URL + "/ap/signin"
			vals := url.Values{}
			vals.Set("verifier", verifier)
			vals.Set("client_id", clientID)
			vals.Set("language", lang)
			return u + "?" + vals.Encode()
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "AUTH-OK-001", authcode)
	assert.NotEmpty(t, codeVerifier)
	assert.NotEmpty(t, serial)
}

func TestAuth_OAuthDance_OTPProviderError(t *testing.T) {
	srv, _ := authMockServer(t)
	wr := newAuthTestWrapper(t, srv)
	auth := &AuthService{wrapper: wr}

	wantErr := errors.New("user canceled OTP prompt")
	_, _, _, err := auth.doOAuthDance(
		context.Background(),
		"alice@example.com", "pw",
		OTPProviderFunc(func(_ context.Context) (string, error) { return "", wantErr }),
		"en-US",
		func(verifier, clientID, lang string) string {
			return srv.URL + "/ap/signin"
		},
	)
	assert.ErrorIs(t, err, wantErr)
}
