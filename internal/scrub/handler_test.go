package scrub_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/scrub"
)

// captureLogger returns a *slog.Logger that writes JSON records to buf via
// the scrub.Handler middleware at Debug level.
func captureLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(scrub.Handler(inner))
}

// records decodes the captured buf as one JSON object per line.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		out = append(out, rec)
	}
	return out
}

func TestHandler_RedactsCredentialKeys(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.DebugContext(context.Background(), "credentials",
		"access_token", "secret-a",
		"password", "hunter2",
		"token", "tok-z",
		"source_token", "src-q",
		"keep", "visible",
	)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	r := recs[0]
	assert.Equal(t, scrub.Replacement, r["access_token"])
	assert.Equal(t, scrub.Replacement, r["password"])
	assert.Equal(t, scrub.Replacement, r["token"])
	assert.Equal(t, scrub.Replacement, r["source_token"])
	assert.Equal(t, "visible", r["keep"])
	assert.NotContains(t, buf.String(), "hunter2")
	assert.NotContains(t, buf.String(), "secret-a")
}

func TestHandler_PassesThroughEmail(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.Info("login", "email", "alice@example.com")

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	assert.Equal(t, "alice@example.com", recs[0]["email"])
}

func TestHandler_RecursesIntoMapAttr(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	body := map[string]any{
		"password": "shh",
		"public":   "ok",
		"nested": map[string]any{
			"token": "deep-secret",
			"keep":  "deep-ok",
		},
	}
	log.Debug("request", "body", body)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	bodyOut := recs[0]["body"].(map[string]any)
	assert.Equal(t, scrub.Replacement, bodyOut["password"])
	assert.Equal(t, "ok", bodyOut["public"])
	nested := bodyOut["nested"].(map[string]any)
	assert.Equal(t, scrub.Replacement, nested["token"])
	assert.Equal(t, "deep-ok", nested["keep"])
	assert.NotContains(t, buf.String(), "shh")
	assert.NotContains(t, buf.String(), "deep-secret")
}

func TestHandler_RecursesIntoFormMapAttr(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	form := map[string]string{"password": "shh", "public": "ok"}
	log.Debug("form-body", "form", form)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	formOut := recs[0]["form"].(map[string]any)
	assert.Equal(t, scrub.Replacement, formOut["password"])
	assert.Equal(t, "ok", formOut["public"])
	assert.NotContains(t, buf.String(), "shh")
}

func TestHandler_RecursesIntoGroup(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.Debug("grouped",
		slog.Group("creds",
			slog.String("password", "shh"),
			slog.String("public", "ok"),
		),
	)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	creds := recs[0]["creds"].(map[string]any)
	assert.Equal(t, scrub.Replacement, creds["password"])
	assert.Equal(t, "ok", creds["public"])
	assert.NotContains(t, buf.String(), "shh")
}

func TestHandler_RedactsCaseInsensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.DebugContext(context.Background(), "mixed-case-credentials",
		"Authorization", "Bearer abc",
		"AccessToken", "tok-1",
		"Access-Token", "tok-2",
		"accessToken", "tok-3",
		"PASSWORD", "hunter2",
		"OtpCode", "123456",
		"Cookie", "session=xyz",
		"Set-Cookie", "session=abc",
		"keep", "visible",
	)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	r := recs[0]
	assert.Equal(t, scrub.Replacement, r["Authorization"])
	assert.Equal(t, scrub.Replacement, r["AccessToken"])
	assert.Equal(t, scrub.Replacement, r["Access-Token"])
	assert.Equal(t, scrub.Replacement, r["accessToken"])
	assert.Equal(t, scrub.Replacement, r["PASSWORD"])
	assert.Equal(t, scrub.Replacement, r["OtpCode"])
	assert.Equal(t, scrub.Replacement, r["Cookie"])
	assert.Equal(t, scrub.Replacement, r["Set-Cookie"])
	assert.Equal(t, "visible", r["keep"])
	assert.NotContains(t, buf.String(), "Bearer abc")
	assert.NotContains(t, buf.String(), "hunter2")
	assert.NotContains(t, buf.String(), "tok-1")
	assert.NotContains(t, buf.String(), "tok-2")
	assert.NotContains(t, buf.String(), "tok-3")
	assert.NotContains(t, buf.String(), "123456")
	assert.NotContains(t, buf.String(), "session=xyz")
	assert.NotContains(t, buf.String(), "session=abc")
}

func TestHandler_RedactsNewSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.DebugContext(context.Background(), "phase1-credentials",
		"refresh_token", "rt-1",
		"adp_token", "adp-1",
		"device_private_key", "pk-1",
		"mac_dms", "mac-1",
		"bearer", "br-1",
		"code_verifier", "cv-1",
		"authorization_code", "ac-1",
		"otp", "654321",
		"csrf", "csrf-1",
		"store_authentication_cookie", "sac-1",
		"website_cookies", "wc-1",
	)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	r := recs[0]
	for _, k := range []string{
		"refresh_token", "adp_token", "device_private_key", "mac_dms",
		"bearer", "code_verifier", "authorization_code", "otp", "csrf",
		"store_authentication_cookie", "website_cookies",
	} {
		assert.Equal(t, scrub.Replacement, r[k], "key %s not redacted", k)
	}
	for _, leak := range []string{
		"rt-1", "adp-1", "pk-1", "mac-1", "br-1", "cv-1", "ac-1", "654321",
		"csrf-1", "sac-1", "wc-1",
	} {
		assert.NotContains(t, buf.String(), leak)
	}
}

func TestHandler_RecursesIntoSliceOfMapsAttr(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	rows := []map[string]any{
		{"password": "shh-1", "id": 1},
		{"refresh_token": "shh-2", "id": 2, "nested": map[string]any{"otp": "shh-3"}},
	}
	log.Debug("rows", "items", rows)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	items, ok := recs[0]["items"].([]any)
	require.True(t, ok, "items should be a JSON array")
	require.Len(t, items, 2)

	first := items[0].(map[string]any)
	assert.Equal(t, scrub.Replacement, first["password"])
	assert.EqualValues(t, 1, first["id"])

	second := items[1].(map[string]any)
	assert.Equal(t, scrub.Replacement, second["refresh_token"])
	assert.EqualValues(t, 2, second["id"])
	nested := second["nested"].(map[string]any)
	assert.Equal(t, scrub.Replacement, nested["otp"])

	assert.NotContains(t, buf.String(), "shh-1")
	assert.NotContains(t, buf.String(), "shh-2")
	assert.NotContains(t, buf.String(), "shh-3")
}

func TestHandler_PassesThroughPlainAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := captureLogger(&buf)

	log.Info("plain", "count", 42, "name", "Alice", "ok", true)

	recs := records(t, &buf)
	require.Len(t, recs, 1)
	r := recs[0]
	assert.EqualValues(t, 42, r["count"])
	// "name" was removed from SensitiveKeys in the earlier commit, so it should pass through.
	assert.Equal(t, "Alice", r["name"])
	assert.Equal(t, true, r["ok"])
}
