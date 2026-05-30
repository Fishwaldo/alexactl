// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

// Package oauth holds helpers used by the AuthService to perform the
// Amazon iOS-app OAuth flow: PKCE verifier/challenge generation and
// HTML form scraping for the login pages.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// pkceVerifierBytes is the default length of the random byte buffer used
// for PKCE code_verifier generation. RFC 7636 requires a verifier of at
// least 43 base64url chars; 32 raw bytes encodes to 43 chars (no padding).
const pkceVerifierBytes = 32

// NewCodeVerifier returns a base64url-encoded random byte sequence suitable
// for use as a PKCE code_verifier. If r is nil, crypto/rand.Reader is used.
// length is the number of raw random bytes (defaults to pkceVerifierBytes
// if <= 0).
func NewCodeVerifier(r io.Reader, length int) (string, error) {
	if length <= 0 {
		length = pkceVerifierBytes
	}
	if r == nil {
		r = rand.Reader
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("oauth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// S256Challenge computes the PKCE S256 challenge for the given verifier.
// Returns base64url-encoded SHA-256 of the verifier bytes, without padding.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
