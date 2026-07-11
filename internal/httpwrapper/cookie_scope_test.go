// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package httpwrapper

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInjectInitialCookiesAlsoSeedsAmazonComForNonUS is a regression test
// for upstream #926: non-US marketplaces (here co.uk) must also receive
// their session cookies on the global amazon.com domain, since global
// endpoints (api.amazon.com, the bob-dispatch AVS host, the comms host)
// live there regardless of the marketplace TLD.
//
// injectInitialCookies is called before every outbound request (see Do),
// keyed to that request's own URL each time — so across a session, a
// call to a marketplace host (www.amazon.co.uk) seeds the marketplace
// domain, and a call to a global host (api.amazon.com) seeds amazon.com.
// Go's cookiejar only accepts a cookie whose Domain attribute
// domain-matches the request's host, so this test exercises both call
// shapes rather than asserting both domains from a single call.
func TestInjectInitialCookiesAlsoSeedsAmazonComForNonUS(t *testing.T) {
	w, err := New(Config{
		Site:       "https://www.amazon.co.uk",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	w.SetSessionCookies(map[string]string{"session-id": "abc"})

	// A normal marketplace-host request seeds amazon.co.uk.
	w.injectInitialCookies("https://www.amazon.co.uk/some/path")
	// A global-host request (e.g. device registration) must ALSO get
	// cookies, even though the account's marketplace TLD is co.uk.
	w.injectInitialCookies("https://api.amazon.com/auth/register")

	comURL, err := url.Parse("https://api.amazon.com/")
	require.NoError(t, err)
	comCookies := w.httpClient.Jar.Cookies(comURL)
	assert.NotEmpty(t, comCookies, "amazon.com must receive seeded cookies for non-US accounts")

	ukURL, err := url.Parse("https://api.amazon.co.uk/")
	require.NoError(t, err)
	ukCookies := w.httpClient.Jar.Cookies(ukURL)
	assert.NotEmpty(t, ukCookies, "marketplace TLD (amazon.co.uk) must still receive seeded cookies")
}

// TestInjectInitialCookiesDoesNotDoubleSeedForUS confirms a .com account
// does not attempt to re-seed amazon.com a second time under a different
// code path — exactly one session-id cookie should land, with no error.
func TestInjectInitialCookiesDoesNotDoubleSeedForUS(t *testing.T) {
	w, err := New(Config{
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	w.SetSessionCookies(map[string]string{"session-id": "abc"})

	w.injectInitialCookies("https://api.amazon.com/auth/register")

	comURL, err := url.Parse("https://api.amazon.com/")
	require.NoError(t, err)
	cookies := w.httpClient.Jar.Cookies(comURL)
	assert.Len(t, cookies, 1, "exactly one session-id cookie, seeded once for a .com account")
}
