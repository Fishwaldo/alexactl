// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package httpwrapper_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

func TestInitialCookies_HasFRCAndMapMD(t *testing.T) {
	src := bytes.NewReader(bytes.Repeat([]byte{0x5A}, 1024))
	cookies, err := httpwrapper.BuildInitialCookies(src)
	require.NoError(t, err)

	assert.Equal(t, amzconst.AmazonAppID, cookies["amzn-app-id"])

	frc := cookies["frc"]
	assert.NotEmpty(t, frc)
	assert.NotContains(t, frc, "=") // raw base64 (no padding)
	// frc must be base64-decodable
	_, err = base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(frc)
	require.NoError(t, err)

	mapMD := cookies["map-md"]
	assert.NotEmpty(t, mapMD)
	assert.NotContains(t, mapMD, "=")
	decoded, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(mapMD)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), amzconst.AmazonAppVersion)
	assert.Contains(t, string(decoded), amzconst.AmazonAppBundleID)
}
