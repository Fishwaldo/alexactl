package amazondevices

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

// customerIDSleepSeconds is how long obtainCustomerIDForSerial waits between
// retries of the /api/devices-v2/device lookup. Amazon takes a few seconds
// after registration before the virtual device shows up in the list.
const customerIDSleepSeconds = 2

// obtainCustomerID polls /api/devices-v2/device until it finds an entry
// for the registered virtual device's serial, then extracts the owning
// customer ID. Mirrors Python obtain_account_customer_id.
//
// Looks up the serial from s.session — callers that have a *Session in
// hand but have not yet committed it to s.session should use
// obtainCustomerIDForSerial directly.
func (s *AuthService) obtainCustomerID(ctx context.Context, sitePrefix string) (string, error) {
	s.mu.RLock()
	var serial string
	if s.session != nil {
		serial = s.session.DeviceInfo.DeviceSerialNumber
	}
	s.mu.RUnlock()
	return s.obtainCustomerIDForSerial(ctx, sitePrefix, serial)
}

// obtainCustomerIDForSerial is the inner form of obtainCustomerID that takes
// the device serial directly. Lets Login defer committing the new *Session
// to s.session until after this lookup succeeds.
func (s *AuthService) obtainCustomerIDForSerial(
	ctx context.Context, sitePrefix, deviceSerial string,
) (string, error) {
	if s.customerIDCache != nil {
		if v, hit, _ := s.customerIDCache.Get(ctx, deviceSerial); hit {
			return v, nil
		}
	}
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(sitePrefix, "/"), amzconst.URIDevices)
	logger := s.wrapper.Logger(ctx)

	for i := 0; i < amzconst.MaxCustomerAccountRetries; i++ {
		if i > 0 {
			s.sleep(ctx, customerIDSleepSeconds)
		}
		logger.DebugContext(ctx, "amazondevices: looking up customer ID", "attempt", i+1)

		resp, err := s.wrapper.Do(ctx, httpwrapper.Request{Method: http.MethodGet, URL: url})
		if err != nil {
			return "", err
		}
		data, err := httpwrapper.DecodeJSON(resp)
		if err != nil {
			return "", err
		}

		devs, _ := data["devices"].([]any)
		for _, d := range devs {
			dm, _ := d.(map[string]any)
			dt, _ := stringOf(dm["deviceType"])
			if dt != amzconst.AmazonDeviceType {
				continue
			}
			subList, _ := dm["appDeviceList"].([]any)
			for _, sd := range subList {
				sm, _ := sd.(map[string]any)
				sn, _ := stringOf(sm["serialNumber"])
				if sn == deviceSerial {
					owner, _ := stringOf(dm["deviceOwnerCustomerId"])
					if owner != "" && s.customerIDCache != nil {
						s.customerIDCache.Set(deviceSerial, owner)
					}
					return owner, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%w: customer ID not found after retries: GET %s",
		ErrCannotRetrieveData, url)
}

// sleep is a test seam.
func (s *AuthService) sleep(ctx context.Context, seconds int) {
	if s.sleepFn != nil {
		s.sleepFn(ctx, seconds)
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(seconds) * time.Second):
	}
}
