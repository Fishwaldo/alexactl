package httpwrapper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
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
	defer resp.Body.Close()

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
