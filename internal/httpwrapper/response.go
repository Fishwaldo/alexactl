package httpwrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// MaxResponseBodyBytes caps the size of response bodies decoded by
// DecodeJSON. Amazon API responses are well under this; the cap exists so
// a hostile/buggy server cannot exhaust memory by streaming an unbounded
// body. Bodies larger than this are truncated cleanly (no error).
const MaxResponseBodyBytes = 16 << 20 // 16 MiB

// DecodeJSON reads resp.Body and decodes JSON into a map. If the response
// is a JSON array, it's wrapped under the synthetic key ArrayWrapper to
// preserve the "always a map" contract used by callers.
//
// Closes resp.Body before returning. Reads are bounded by
// MaxResponseBodyBytes.
func DecodeJSON(resp *http.Response) (map[string]any, error) {
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("httpwrapper: read body: %w", err)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var direct map[string]any
	if jerr := json.Unmarshal(raw, &direct); jerr == nil {
		if direct == nil {
			return map[string]any{}, nil
		}
		return direct, nil
	}

	var arr []any
	if jerr := json.Unmarshal(raw, &arr); jerr == nil {
		return map[string]any{amzconst.ArrayWrapper: arr}, nil
	}

	return nil, fmt.Errorf("httpwrapper: response is neither JSON object nor array")
}

// LogResponseBody peeks resp.Body, attempts to decode it as JSON, logs
// the result at DEBUG, and restores resp.Body so downstream readers can
// still consume it. Sensitive fields in the body are auto-redacted by
// the scrub.Handler slog middleware as long as they are passed as a
// map[string]any — which a JSON object decode produces directly.
//
// Behavior:
//   - No-op when the supplied logger is not enabled at DEBUG (no
//     allocation, no body read).
//   - Auth-endpoint exclusion is the caller's responsibility (Do skips
//     /auth/ and /ap/ URLs before invoking this).
//   - JSON objects log as `body=<map>`; JSON arrays log as `body=<slice>`.
//   - Non-JSON bodies log size + "non-json" kind without the payload —
//     credential values could appear in arbitrary positions in
//     unstructured text and the scrub handler can only redact keyed
//     fields.
//   - Body bytes are capped at MaxResponseBodyBytes; bodies larger than
//     that are truncated for logging AND for downstream readers (the
//     restored body is the truncated slice). Real Amazon responses are
//     well under this cap; the truncation is a memory-safety net.
func LogResponseBody(ctx context.Context, logger *slog.Logger, method, url string, resp *http.Response) {
	if !logger.Enabled(ctx, slog.LevelDebug) {
		return
	}
	if resp == nil || resp.Body == nil {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodyBytes))
	if err != nil {
		logger.DebugContext(ctx, "alexactl: http response body (read error)",
			"method", method, "url", url, "read_err", err.Error())
		return
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(raw))

	if len(raw) == 0 {
		logger.DebugContext(ctx, "alexactl: http response body",
			"method", method, "url", url, "bytes", 0)
		return
	}

	var obj map[string]any
	if jerr := json.Unmarshal(raw, &obj); jerr == nil && obj != nil {
		logger.DebugContext(ctx, "alexactl: http response body",
			"method", method, "url", url, "body", obj)
		return
	}
	var arr []any
	if jerr := json.Unmarshal(raw, &arr); jerr == nil {
		logger.DebugContext(ctx, "alexactl: http response body",
			"method", method, "url", url, "body", arr)
		return
	}
	logger.DebugContext(ctx, "alexactl: http response body",
		"method", method, "url", url, "bytes", len(raw), "kind", "non-json")
}
