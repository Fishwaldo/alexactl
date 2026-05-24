package scrub_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Fishwaldo/goamazondevices/internal/scrub"
)

func TestFields_TopLevel(t *testing.T) {
	in := map[string]any{
		"access_token": "secret",
		"token":        "another-secret",
		"source_token": "yet-another",
		"password":     "hunter2",
		"name":         "Alice",
		"keep":         "untouched",
	}
	out := scrub.Fields(in).(map[string]any)
	// Credential fields are redacted.
	assert.Equal(t, "[REDACTED]", out["access_token"])
	assert.Equal(t, "[REDACTED]", out["token"])
	assert.Equal(t, "[REDACTED]", out["source_token"])
	assert.Equal(t, "[REDACTED]", out["password"])
	// Non-credential fields pass through unchanged.
	assert.Equal(t, "Alice", out["name"])
	assert.Equal(t, "untouched", out["keep"])
}

func TestFields_Nested(t *testing.T) {
	in := map[string]any{
		"outer": map[string]any{
			"password": "p",
			"keep":     "k",
			"inner": []any{
				map[string]any{"token": "t", "ok": "v"},
			},
		},
	}
	out := scrub.Fields(in).(map[string]any)
	outer := out["outer"].(map[string]any)
	assert.Equal(t, "[REDACTED]", outer["password"])
	assert.Equal(t, "k", outer["keep"])
	inner := outer["inner"].([]any)[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", inner["token"])
	assert.Equal(t, "v", inner["ok"])
}

func TestFields_CaseInsensitive(t *testing.T) {
	in := map[string]any{
		"AccessToken":  "secret-1",
		"Access-Token": "secret-2",
		"accessToken":  "secret-3",
		"PASSWORD":     "hunter2",
		"OtpCode":      "123456",
		"Cookie":       "session=abc",
		"keep":         "visible",
	}
	out := scrub.Fields(in).(map[string]any)
	assert.Equal(t, "[REDACTED]", out["AccessToken"])
	assert.Equal(t, "[REDACTED]", out["Access-Token"])
	assert.Equal(t, "[REDACTED]", out["accessToken"])
	assert.Equal(t, "[REDACTED]", out["PASSWORD"])
	assert.Equal(t, "[REDACTED]", out["OtpCode"])
	assert.Equal(t, "[REDACTED]", out["Cookie"])
	assert.Equal(t, "visible", out["keep"])
}

func TestFields_NewSensitiveKeys(t *testing.T) {
	in := map[string]any{
		"refresh_token":               "rt",
		"adp_token":                   "adp",
		"device_private_key":          "pk",
		"mac_dms":                     "mac",
		"bearer":                      "br",
		"code_verifier":               "cv",
		"authorization_code":          "ac",
		"otp":                         "654321",
		"csrf":                        "csrf",
		"store_authentication_cookie": "sac",
		"website_cookies":             "wc",
		"set-cookie":                  "sc",
	}
	out := scrub.Fields(in).(map[string]any)
	for k := range in {
		assert.Equal(t, "[REDACTED]", out[k], "key %s not redacted", k)
	}
}

func TestFields_NonMapValues(t *testing.T) {
	assert.Equal(t, "hello", scrub.Fields("hello"))
	assert.Equal(t, 42, scrub.Fields(42))
	assert.Nil(t, scrub.Fields(nil))
}
