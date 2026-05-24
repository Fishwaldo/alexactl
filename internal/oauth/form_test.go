package oauth_test

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/oauth"
)

func loadHTML(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}

func TestParseLoginForm_EmailPassword(t *testing.T) {
	html := loadHTML(t, "testdata/login_email_password_form.html")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), nil)
	require.NoError(t, err)

	assert.Equal(t, "POST", form.Method)
	assert.Equal(t, "https://www.amazon.com/ap/signin/abc-123", form.Action)
	assert.Equal(t, "tok-123", form.Inputs["appActionToken"])
	assert.Equal(t, "SIGNIN_PWD_COLLECT", form.Inputs["appAction"])
	// non-hidden inputs are NOT included
	_, hasEmail := form.Inputs["email"]
	assert.False(t, hasEmail)
}

func TestParseLoginForm_OTP(t *testing.T) {
	html := loadHTML(t, "testdata/login_otp_form.html")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), nil)
	require.NoError(t, err)

	assert.True(t, form.HasOTPField)
	assert.Equal(t, "https://www.amazon.com/ap/signin/otp-step", form.Action)
	assert.Equal(t, "otp-tok-456", form.Inputs["otpToken"])
}

func TestParseLoginForm_CaptchaDetected(t *testing.T) {
	html := loadHTML(t, "testdata/captcha_japanese.html")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), nil)
	require.NoError(t, err)
	assert.True(t, form.IsCaptcha)
}

func TestParseLoginForm_NoForm(t *testing.T) {
	_, err := oauth.ParseLoginForm(strings.NewReader("<html><body>no form</body></html>"), nil)
	assert.Error(t, err)
}

func TestParseLoginForm_AbsoluteAction(t *testing.T) {
	html := `<html><body>
<form name="signIn" method="POST" action="https://www.amazon.com/ap/signin/abs">
<input type="hidden" name="token" value="v"/>
</form></body></html>`
	base := mustURL(t, "https://other.example.com/path")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), base)
	require.NoError(t, err)
	assert.Equal(t, "https://www.amazon.com/ap/signin/abs", form.Action)
}

func TestParseLoginForm_RelativeAction(t *testing.T) {
	html := `<html><body>
<form name="signIn" method="POST" action="/ap/signin/abc">
<input type="hidden" name="token" value="v"/>
</form></body></html>`
	base := mustURL(t, "https://www.amazon.com/foo")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), base)
	require.NoError(t, err)
	assert.Equal(t, "https://www.amazon.com/ap/signin/abc", form.Action)
}

func TestParseLoginForm_ProtocolRelativeAction(t *testing.T) {
	html := `<html><body>
<form name="signIn" method="POST" action="//other.amazon.com/x">
<input type="hidden" name="token" value="v"/>
</form></body></html>`
	base := mustURL(t, "https://www.amazon.com/foo")
	form, err := oauth.ParseLoginForm(strings.NewReader(html), base)
	require.NoError(t, err)
	assert.Equal(t, "https://other.amazon.com/x", form.Action)
}
