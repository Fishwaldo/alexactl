package alexactl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

func TestObtainCustomerID_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, amzconst.URIDevices)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"devices": []any{
				map[string]any{
					"deviceType":            amzconst.AmazonDeviceType,
					"serialNumber":          "ANY",
					"accountName":           "dummy",
					"deviceOwnerCustomerId": "amzn1.account.OWNER",
					"appDeviceList": []any{
						map[string]any{"serialNumber": "DEADBEEF00112233"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	auth := &AuthService{wrapper: wr, session: &Session{
		DeviceInfo: DeviceInfo{DeviceSerialNumber: "DEADBEEF00112233"},
	}}

	id, err := auth.obtainCustomerID(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "amzn1.account.OWNER", id)
}

func TestObtainCustomerID_NotFoundAfterRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": []any{}})
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	auth := &AuthService{wrapper: wr, session: &Session{
		DeviceInfo: DeviceInfo{DeviceSerialNumber: "DEADBEEF00112233"},
	}, sleepFn: func(_ context.Context, _ int) {}} // skip 2s sleeps in test

	_, err = auth.obtainCustomerID(context.Background(), srv.URL)
	assert.ErrorIs(t, err, ErrCannotRetrieveData)
}
