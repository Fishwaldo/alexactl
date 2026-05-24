package amazondevices

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func TestRegisterDevice_Success(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/register", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))

		// Build a representative success response per Amazon's shape.
		resp := map[string]any{
			"response": map[string]any{
				"success": map[string]any{
					"tokens": map[string]any{
						"mac_dms": map[string]any{
							"adp_token":          "adp-zzz",
							"device_private_key": "PRIVKEY",
						},
						"store_authentication_cookie": map[string]any{"cookie": "store-cookie"},
						"bearer": map[string]any{
							"access_token":  "ACCESS-OK",
							"refresh_token": "REFRESH-OK",
							"expires_in":    "3600",
						},
						"website_cookies": []any{
							map[string]any{"Name": "session-id", "Value": `"abc"`},
						},
					},
					"extensions": map[string]any{
						"device_info": map[string]any{
							"device_serial_number": "DEADBEEF00112233",
							"device_type":          "A2IVLV5VM2W81",
						},
						"customer_info": map[string]any{
							"home_region": "NA",
							"account_id":  "amzn1.account.TEST",
							"name":        "Alice",
							"given_name":  "Alice",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	auth := &AuthService{wrapper: wr}

	sess, err := auth.registerDevice(context.Background(), registerInput{
		AuthorizationCode: "AUTH-OK-001",
		CodeVerifier:      "verifier-abc",
		Serial:            "DEADBEEF00112233",
		ClientIDHex:       buildClientID("DEADBEEF00112233"),
		RegisterURL:       srv.URL + "/auth/register",
	})
	require.NoError(t, err)

	authData := captured["auth_data"].(map[string]any)
	assert.Equal(t, "AUTH-OK-001", authData["authorization_code"])
	assert.Equal(t, "verifier-abc", authData["code_verifier"])

	assert.Equal(t, "ACCESS-OK", sess.AccessToken)
	assert.Equal(t, "REFRESH-OK", sess.RefreshToken)
	assert.Equal(t, "adp-zzz", sess.ADPToken)
	assert.Equal(t, "PRIVKEY", sess.DevicePrivateKey)
	assert.Equal(t, "store-cookie", sess.StoreAuthCookie)
	assert.Equal(t, "abc", sess.WebsiteCookies["session-id"])
	assert.Equal(t, "DEADBEEF00112233", sess.DeviceInfo.DeviceSerialNumber)
	assert.Equal(t, "A2IVLV5VM2W81", sess.DeviceInfo.DeviceType)
	assert.Equal(t, "NA", sess.CustomerInfo.HomeRegion)
	assert.True(t, sess.ExpiresAt.After(time.Now()))
}

func TestRegisterDevice_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"error": map[string]any{"message": "invalid_grant"},
			},
		})
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{HTTPClient: srv.Client(), Site: "https://www.amazon.com", RandReader: bytes.NewReader(bytes.Repeat([]byte{0x01}, 1024))})
	require.NoError(t, err)
	auth := &AuthService{wrapper: wr}

	_, err = auth.registerDevice(context.Background(), registerInput{
		AuthorizationCode: "x", CodeVerifier: "v", Serial: "s",
		RegisterURL: srv.URL + "/auth/register",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRegisterDevice)
}

// registerSuccessShape builds the canonical /auth/register success envelope.
// Pass a mutator to perturb specific fields for the validation tests below.
func registerSuccessShape(mutate func(success map[string]any)) map[string]any {
	success := map[string]any{
		"tokens": map[string]any{
			"mac_dms": map[string]any{
				"adp_token":          "adp",
				"device_private_key": "k",
			},
			"store_authentication_cookie": map[string]any{"cookie": "sac"},
			"bearer": map[string]any{
				"access_token":  "A1",
				"refresh_token": "R1",
				"expires_in":    "3600",
			},
			"website_cookies": []any{},
		},
		"extensions": map[string]any{
			"device_info":   map[string]any{"device_serial_number": "S1", "device_type": "T1"},
			"customer_info": map[string]any{"home_region": "NA"},
		},
	}
	if mutate != nil {
		mutate(success)
	}
	return map[string]any{"response": map[string]any{"success": success}}
}

func runRegisterValidationCase(t *testing.T, mutate func(success map[string]any)) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(registerSuccessShape(mutate))
	}))
	t.Cleanup(srv.Close)

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	auth := &AuthService{wrapper: wr}
	_, err = auth.registerDevice(context.Background(), registerInput{
		AuthorizationCode: "x", CodeVerifier: "v", Serial: "s",
		RegisterURL: srv.URL + "/auth/register",
	})
	return err
}

func TestRegisterDevice_BearerMissing(t *testing.T) {
	err := runRegisterValidationCase(t, func(success map[string]any) {
		tokens := success["tokens"].(map[string]any)
		delete(tokens, "bearer")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRegisterDevice)
}

func TestRegisterDevice_MacDMSMissing(t *testing.T) {
	err := runRegisterValidationCase(t, func(success map[string]any) {
		tokens := success["tokens"].(map[string]any)
		delete(tokens, "mac_dms")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRegisterDevice)
}

func TestRegisterDevice_AccessTokenEmpty(t *testing.T) {
	err := runRegisterValidationCase(t, func(success map[string]any) {
		bearer := success["tokens"].(map[string]any)["bearer"].(map[string]any)
		bearer["access_token"] = ""
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRegisterDevice)
}

func TestRegisterDevice_RefreshTokenEmpty(t *testing.T) {
	err := runRegisterValidationCase(t, func(success map[string]any) {
		bearer := success["tokens"].(map[string]any)["bearer"].(map[string]any)
		bearer["refresh_token"] = ""
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotRegisterDevice)
}
