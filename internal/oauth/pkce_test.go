package oauth_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/oauth"
)

func TestNewCodeVerifier_LengthAndCharset(t *testing.T) {
	v, err := oauth.NewCodeVerifier(io.Reader(nil), 32)
	require.NoError(t, err)
	// base64url(32 bytes) without '=' padding is 43 chars
	assert.Len(t, v, 43)
	// chars must be base64url-safe
	for _, c := range v {
		ok := (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_'
		assert.True(t, ok, "char %q not base64url-safe", c)
	}
}

func TestNewCodeVerifier_DeterministicWithInjectedReader(t *testing.T) {
	src := bytes.NewReader(bytes.Repeat([]byte{0xAB}, 64))
	v1, err := oauth.NewCodeVerifier(src, 32)
	require.NoError(t, err)

	src = bytes.NewReader(bytes.Repeat([]byte{0xAB}, 64))
	v2, err := oauth.NewCodeVerifier(src, 32)
	require.NoError(t, err)

	assert.Equal(t, v1, v2)
}

func TestS256Challenge_MatchesSHA256(t *testing.T) {
	verifier := "abcDEF123-_xyzABCdef"
	got := oauth.S256Challenge(verifier)

	hash := sha256.Sum256([]byte(verifier))
	want := strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
	assert.Equal(t, want, got)
}
