// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package http2stream

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nopCloser wraps a Reader so it satisfies io.ReadCloser without
// closing anything when Close is called.
type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

func TestReader_StreamsThreeParts(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/push/multipart_sample.bin")
	require.NoError(t, err)

	r, err := NewReader(nopCloser{bytes.NewReader(raw)},
		"multipart/related; boundary=amzn-alexa-stream")
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	var parts []string
	for {
		body, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		parts = append(parts, string(body))
	}
	require.Len(t, parts, 3)
	assert.Contains(t, parts[0], `"PUSH_VOLUME_CHANGE"`)
	assert.Contains(t, parts[1], `"PUSH_AUDIO_PLAYER_STATE"`)
	assert.Contains(t, parts[2], `"PUSH_NOTIFICATION_CHANGE"`)
}

func TestNewReader_MissingBoundary(t *testing.T) {
	_, err := NewReader(nopCloser{strings.NewReader("")},
		"multipart/related")
	require.ErrorIs(t, err, ErrBoundaryMissing)
}

func TestNewReader_UnparseableContentType(t *testing.T) {
	_, err := NewReader(nopCloser{strings.NewReader("")},
		"this is not a media type")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBoundaryMissing,
		"a malformed Content-Type must not be mistaken for a missing boundary")
}

func TestNewReader_AmazonRealContentType_UnquotedSlashParam(t *testing.T) {
	// Amazon's real directives stream sends this exact shape. The
	// unquoted "type=application/json" parameter contains a slash,
	// which RFC 2045 requires to be quoted; mime.ParseMediaType is
	// strict and rejects it. extractBoundary must fall back to a
	// permissive scan.
	body := "" +
		"--------abcde123\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		`{"ok":true}` + "\r\n" +
		"--------abcde123--\r\n"
	r, err := NewReader(nopCloser{strings.NewReader(body)},
		"multipart/related; boundary=------abcde123; type=application/json")
	require.NoError(t, err)
	got, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(got))
}

func TestReader_RejectsNonJSONPart(t *testing.T) {
	body := "" +
		"--bnd\r\n" +
		"Content-Type: text/plain\r\n\r\n" +
		"hello\r\n" +
		"--bnd--\r\n"
	r, err := NewReader(nopCloser{strings.NewReader(body)},
		"multipart/related; boundary=bnd")
	require.NoError(t, err)
	_, err = r.Next()
	require.ErrorIs(t, err, ErrUnexpectedContentType)
}

func TestReader_SkipsEmptyParts(t *testing.T) {
	body := "" +
		"--bnd\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		"\r\n" + // empty body
		"--bnd\r\n" +
		"Content-Type: application/json\r\n\r\n" +
		`{"ok":true}` + "\r\n" +
		"--bnd--\r\n"
	r, err := NewReader(nopCloser{strings.NewReader(body)},
		"multipart/related; boundary=bnd")
	require.NoError(t, err)
	got, err := r.Next()
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(got))
	_, err = r.Next()
	require.ErrorIs(t, err, io.EOF)
}
