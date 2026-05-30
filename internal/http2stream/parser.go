// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

// Package http2stream parses Amazon's AVS directives stream. The stream
// is delivered as a single HTTP response body with Content-Type
// "multipart/related; boundary=...". Each part is a JSON object whose
// structure is decoded by events.go.
//
// This package is decoder-only — it never opens a network connection
// and is safe to test with synthetic in-memory bodies.
package http2stream

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// ErrBoundaryMissing is returned by NewReader when the supplied
// Content-Type does not include a multipart boundary parameter.
var ErrBoundaryMissing = errors.New("http2stream: multipart boundary missing")

// ErrUnexpectedContentType is returned by Next when a part declares a
// Content-Type other than application/json. The directives stream is
// supposed to only ever carry JSON parts, so the parser refuses to
// guess.
var ErrUnexpectedContentType = errors.New("http2stream: unexpected part content-type")

// Reader streams the parts of a multipart/related body.
type Reader struct {
	body io.Closer
	mr   *multipart.Reader
}

// NewReader builds a Reader from an open HTTP response body and the
// response's Content-Type header value. NewReader does not consume any
// bytes from body; the caller still owns Close (Reader.Close forwards
// to body.Close).
//
// Amazon's real directives stream returns a Content-Type like:
//
//	multipart/related; boundary=------abcde123; type=application/json
//
// where the unquoted `type=application/json` parameter value contains
// a slash. Go's `mime.ParseMediaType` is strict about RFC 2045 token
// rules and rejects unquoted slashes, so we extract the `boundary`
// parameter manually as a fallback.
func NewReader(body io.ReadCloser, contentType string) (*Reader, error) {
	boundary, err := extractBoundary(contentType)
	if err != nil {
		return nil, err
	}
	return &Reader{
		body: body,
		mr:   multipart.NewReader(body, boundary),
	}, nil
}

// extractBoundary pulls the boundary parameter from a Content-Type
// header. Tries the strict stdlib parser first; on failure, falls back
// to a permissive scan that tolerates unquoted token-disallowed
// characters (Amazon's stream emits unquoted "type=application/json").
func extractBoundary(contentType string) (string, error) {
	if _, params, err := mime.ParseMediaType(contentType); err == nil {
		if b := params["boundary"]; b != "" {
			return b, nil
		}
		return "", ErrBoundaryMissing
	}
	// Permissive fallback: split on ';' and look for boundary=...
	// First token is the media type itself; skip it.
	parts := strings.Split(contentType, ";")
	if len(parts) < 2 {
		return "", fmt.Errorf("http2stream: parse content-type %q: no parameters", contentType)
	}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		const key = "boundary="
		if !strings.HasPrefix(p, key) {
			continue
		}
		v := strings.TrimPrefix(p, key)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"`)
		if v == "" {
			return "", ErrBoundaryMissing
		}
		return v, nil
	}
	return "", ErrBoundaryMissing
}

// Next reads the next part and returns its body bytes. It returns
// io.EOF when the stream is exhausted. Empty parts (boundary-only
// chunks) are skipped silently.
func (r *Reader) Next() ([]byte, error) {
	for {
		part, err := r.mr.NextPart()
		if err != nil {
			return nil, err
		}
		ct := part.Header.Get("Content-Type")
		// Strip parameters from the content type for comparison; we
		// only care about the media type itself.
		mt, _, mtErr := mime.ParseMediaType(ct)
		if mtErr != nil || mt != "application/json" {
			_ = part.Close()
			return nil, fmt.Errorf("%w: %q", ErrUnexpectedContentType, ct)
		}
		buf, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil {
			return nil, fmt.Errorf("http2stream: read part: %w", readErr)
		}
		if len(buf) == 0 {
			continue
		}
		return buf, nil
	}
}

// Close releases the underlying body.
func (r *Reader) Close() error {
	if r.body == nil {
		return nil
	}
	return r.body.Close()
}
