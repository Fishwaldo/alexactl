package alexactl

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/session/sample.json
var sampleSessionJSON []byte

func TestSession_RoundTrip(t *testing.T) {
	var s Session
	require.NoError(t, json.Unmarshal(sampleSessionJSON, &s))

	assert.Equal(t, "Atna|sample-access", s.AccessToken)
	assert.Equal(t, "Atnr|sample-refresh", s.RefreshToken)
	assert.Equal(t, "0123456789ABCDEF", s.DeviceInfo.DeviceSerialNumber)
	assert.Equal(t, "A2IVLV5VM2W81", s.DeviceInfo.DeviceType)
	assert.Equal(t, "NA", s.CustomerInfo.HomeRegion)
	assert.Equal(t, "com", s.Domain)
	assert.Equal(t, "https://www.amazon.com", s.Site)
	assert.Equal(t, "amzn1.account.SAMPLE", s.AccountCustomerID)
	assert.Equal(t, time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC), s.ExpiresAt.UTC())
	assert.Equal(t, "abc-123", s.WebsiteCookies["session-id"])

	out, err := json.Marshal(&s)
	require.NoError(t, err)

	var rt Session
	require.NoError(t, json.Unmarshal(out, &rt))
	assert.Equal(t, s, rt)
}

func TestSession_Expired(t *testing.T) {
	s := &Session{ExpiresAt: time.Now().Add(-time.Hour)}
	assert.True(t, s.Expired(time.Now()))

	s.ExpiresAt = time.Now().Add(time.Hour)
	assert.False(t, s.Expired(time.Now()))
}
