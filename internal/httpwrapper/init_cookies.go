package httpwrapper

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Fishwaldo/alexactl/internal/amzconst"
)

// BuildInitialCookies returns the pre-login cookie set Amazon expects to
// avoid serving a captcha on the password page. If r is nil,
// crypto/rand.Reader is used.
//
// Ported from aioamazondevices http_wrapper._build_init_cookies.
func BuildInitialCookies(r io.Reader) (map[string]string, error) {
	if r == nil {
		r = rand.Reader
	}
	buf := make([]byte, 313)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("httpwrapper: read random: %w", err)
	}
	frc := base64.RawStdEncoding.EncodeToString(buf)

	mapMD := map[string]any{
		"device_user_dictionary": []any{},
		"device_registration_data": map[string]any{
			"software_version": amzconst.AmazonDeviceSoftwareVersion,
		},
		"app_identifier": map[string]any{
			"app_version": amzconst.AmazonAppVersion,
			"bundle_id":   amzconst.AmazonAppBundleID,
		},
	}
	mapMDJSON, err := json.Marshal(mapMD)
	if err != nil {
		return nil, fmt.Errorf("httpwrapper: marshal map-md: %w", err)
	}
	mapMDStr := base64.RawStdEncoding.EncodeToString(mapMDJSON)

	return map[string]string{
		"amzn-app-id": amzconst.AmazonAppID,
		"frc":         frc,
		"map-md":      mapMDStr,
	}, nil
}
