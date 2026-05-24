package amazondevices

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClientID(t *testing.T) {
	serial := "DEADBEEF00112233"
	got := buildClientID(serial)
	// Format: hex of (serial + "#" + AmazonDeviceType)
	want := strings.ToLower(toHex(serial + "#" + "A2IVLV5VM2W81"))
	assert.Equal(t, want, got)
}

func TestNewSerialNumber_UpperHex32(t *testing.T) {
	s, err := newSerialNumber()
	require.NoError(t, err)
	assert.Len(t, s, 32)
	assert.Equal(t, strings.ToUpper(s), s)
	for _, c := range s {
		ok := (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')
		assert.True(t, ok)
	}
}

type errReader struct{ err error }

func (r errReader) Read(_ []byte) (int, error) { return 0, r.err }

func TestNewSerialNumber_RandFailureSurfaced(t *testing.T) {
	want := errors.New("rng broken")
	_, err := newSerialNumberFrom(errReader{err: want})
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

// silence unused-import linter if the io import is dropped by future edits.
var _ io.Reader = errReader{}

func TestBuildOAuthURL_Shape(t *testing.T) {
	verifier := "test-verifier-abc"
	cid := "0123abc"
	urlStr := buildOAuthURL(verifier, cid, "en-US")

	u, err := url.Parse(urlStr)
	require.NoError(t, err)
	assert.Equal(t, "www.amazon.com", u.Host)
	assert.Equal(t, "/ap/signin", u.Path)

	q := u.Query()
	assert.Equal(t, "https://www.amazon.com/ap/maplanding", q.Get("openid.return_to"))
	assert.Equal(t, "S256", q.Get("openid.oa2.code_challenge_method"))
	assert.Equal(t, "amzn_dp_project_dee_ios", q.Get("openid.assoc_handle"))
	assert.Equal(t, "device:"+cid, q.Get("openid.oa2.client_id"))
	assert.Equal(t, "device_auth_access", q.Get("openid.oa2.scope"))
	assert.Equal(t, "code", q.Get("openid.oa2.response_type"))
	assert.Equal(t, "en-US", q.Get("language"))
	assert.NotEmpty(t, q.Get("openid.oa2.code_challenge"))
}

func TestExtractAuthCodeFromURL(t *testing.T) {
	u, _ := url.Parse("https://www.amazon.com/ap/maplanding?openid.oa2.authorization_code=AUTHCODE123&foo=bar")
	got, err := extractAuthCodeFromURL(u)
	require.NoError(t, err)
	assert.Equal(t, "AUTHCODE123", got)
}

func TestExtractAuthCodeFromURL_Missing(t *testing.T) {
	u, _ := url.Parse("https://example.com/x?y=1")
	_, err := extractAuthCodeFromURL(u)
	assert.Error(t, err)
}

// Test-only helper, mirrors what buildClientID does internally.
func toHex(s string) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(s)*2)
	for i := 0; i < len(s); i++ {
		c := s[i]
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}
