package alexactl

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Fishwaldo/alexactl/internal/scrub"
)

// newPushLogCapture builds a slog.Logger backed by an in-memory text
// handler wrapped in scrub.Handler so tests can verify production-shape
// redaction of push-stream body logs.
func newPushLogCapture(t *testing.T, level slog.Level) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})
	return slog.New(scrub.Handler(inner)), &buf
}

func TestLogPushPart_JSON_LoggedStructured(t *testing.T) {
	logger, buf := newPushLogCapture(t, slog.LevelDebug)
	part := []byte(`{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_VOLUME_CHANGE","resourceMetadata":{"payload":{"deviceName":"echo-1"}}}]}}}`)

	logPushPart(context.Background(), logger, part)

	out := buf.String()
	assert.Contains(t, out, "push body")
	assert.Contains(t, out, "echo-1",
		"non-sensitive fields survive the scrub")
}

func TestLogPushPart_RedactsSensitiveFields(t *testing.T) {
	logger, buf := newPushLogCapture(t, slog.LevelDebug)
	// Synthetic part with a credential-shaped field deep in the
	// payload. The scrub.Handler recurses through nested maps so
	// redaction must reach it.
	part := []byte(`{"directive":{"payload":{"renderingUpdates":[{"resourceId":"PUSH_X","resourceMetadata":{"payload":{"accessToken":"SECRET","refresh_token":"SHHH","ok":true}}}]}}}`)

	logPushPart(context.Background(), logger, part)

	out := buf.String()
	assert.NotContains(t, out, "SECRET",
		"scrub must redact accessToken")
	assert.NotContains(t, out, "SHHH",
		"scrub must redact refresh_token")
	assert.Contains(t, out, scrub.Replacement)
	assert.Contains(t, out, "ok")
}

func TestLogPushPart_DebugDisabled_NoOp(t *testing.T) {
	logger, buf := newPushLogCapture(t, slog.LevelInfo)
	part := []byte(`{"directive":{"payload":{"renderingUpdates":[]}}}`)

	logPushPart(context.Background(), logger, part)

	assert.Empty(t, buf.String(),
		"no log line when DEBUG disabled")
}

func TestLogPushPart_NonJSON_LogsSizeOnly(t *testing.T) {
	logger, buf := newPushLogCapture(t, slog.LevelDebug)

	logPushPart(context.Background(), logger, []byte("not json at all"))

	out := buf.String()
	assert.Contains(t, out, "non-json")
	assert.NotContains(t, out, "not json at all",
		"non-JSON payload contents must not be logged")
}
