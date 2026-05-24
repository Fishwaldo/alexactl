package amazondevices

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

//go:embed testdata/devices/endpoints_graphql.json
var endpointsGraphQLJSON []byte

//go:embed testdata/devices/endpoints_graphql_non_amazon_aqm.json
var endpointsGraphQLNonAmazonAQMJSON []byte

func TestDevices_Enrichment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "getDevicesBaseData", body["operationName"])
			assert.NotEmpty(t, body["query"])

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Use hostRewriter so the production code's https://alexa.amazon.com URLs
	// land on the httptest server.
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}

	devs, err := ds.List(context.Background())
	require.NoError(t, err)

	// 2 voice devices from base + 1 AQM endpoint = 3 total
	assert.Len(t, devs, 3)

	var kitchen *Device
	for _, d := range devs {
		if d.SerialNumber == "S-KITCHEN-001" {
			kitchen = d
			break
		}
	}
	require.NotNil(t, kitchen)
	assert.Equal(t, "Echo Dot", kitchen.Model)
	assert.Equal(t, "5th Gen", kitchen.HardwareVersion)
	assert.Equal(t, "Amazon", kitchen.Manufacturer)
	assert.Equal(t, "entity-kitchen", kitchen.EntityID)
	assert.Equal(t, "ep-kitchen", kitchen.EndpointID)

	var aqm *Device
	for _, d := range devs {
		if d.SerialNumber == "S-AQM-001" {
			aqm = d
			break
		}
	}
	require.NotNil(t, aqm)
	assert.Equal(t, "AIR_QUALITY_MONITOR", aqm.DeviceFamily)
	assert.Equal(t, "Air Quality Monitor", aqm.Model)
	assert.Equal(t, "Amazon", aqm.Manufacturer)
	assert.Equal(t, "999", aqm.SoftwareVersion)
}

// TestDefault_DeterministicAcrossCalls verifies that the fallback path in
// Default (no user-set device) returns the same device for every call. The
// underlying cache is a map, whose iteration order is randomized by Go, so
// this would fail without the sort.Strings step.
func TestDefault_DeterministicAcrossCalls(t *testing.T) {
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: &http.Client{},
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
		cache: map[string]*Device{
			"S-MIDDLE": {SerialNumber: "S-MIDDLE", Online: true},
			"S-AAA":    {SerialNumber: "S-AAA", Online: true},
			"S-ZZZ":    {SerialNumber: "S-ZZZ", Online: true},
		},
		// Non-zero so refreshIfStale doesn't try to hit the network.
		lastDailyFetch: time.Now(),
	}

	first, err := ds.Default(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	// The lowest-sorted serial should win.
	assert.Equal(t, "S-AAA", first.SerialNumber)

	for i := 0; i < 50; i++ {
		got, err := ds.Default(context.Background())
		require.NoError(t, err)
		assert.Same(t, first, got, "Default() must return the same device every call")
	}
}

// TestDefault_RespectsUserSet verifies that a user-set default takes
// priority over the sorted-fallback path.
func TestDefault_RespectsUserSet(t *testing.T) {
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: &http.Client{},
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	userPick := &Device{SerialNumber: "S-ZZZ", Online: true}
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
		cache: map[string]*Device{
			"S-AAA":  {SerialNumber: "S-AAA", Online: true},
			"S-ZZZ":  userPick,
			"S-MMMM": {SerialNumber: "S-MMMM", Online: true},
		},
		lastDailyFetch: time.Now(),
	}
	require.NoError(t, ds.SetDefault(userPick))

	got, err := ds.Default(context.Background())
	require.NoError(t, err)
	assert.Same(t, userPick, got, "Default() must return the user-set device, not the sorted fallback")
	assert.Equal(t, "S-ZZZ", got.SerialNumber)
}

// TestRefresh_Singleflight stands up an httptest server that counts how
// many base-data fetches it serves. With singleflight in place, 10
// concurrent List() calls against a stale cache should produce exactly 1
// underlying refresh.
func TestRefresh_Singleflight(t *testing.T) {
	var baseHits, graphHits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			atomic.AddInt32(&baseHits, 1)
			// Small stall to widen the race window so concurrent
			// callers actually pile up on the singleflight key.
			time.Sleep(50 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(baseListJSON)
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			atomic.AddInt32(&graphHits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
		// lastDailyFetch zero-value -> stale.
	}

	const N = 10
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = ds.List(context.Background())
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		require.NoError(t, e, "goroutine %d failed", i)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&baseHits),
		"expected exactly one base-data fetch with singleflight")
	assert.Equal(t, int32(1), atomic.LoadInt32(&graphHits),
		"expected exactly one GraphQL fetch with singleflight")
}

// TestRefresh_SurfacesNonAmazonAQM verifies that AQMs with a non-Amazon
// manufacturer are still surfaced in List output, with the manufacturer
// field set to the parsed value (not hardcoded to "Amazon"). This
// behavior diverges from upstream Python (aioamazondevices) by design.
func TestRefresh_SurfacesNonAmazonAQM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, amzconst.URIDevices):
			w.Header().Set("Content-Type", "application/json")
			// Empty base list — the AQM is the only device.
			_, _ = w.Write([]byte(`{"devices": []}`))
		case strings.Contains(r.URL.Path, amzconst.URINexusGraphQL):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(endpointsGraphQLNonAmazonAQMJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		session: &Session{AccountCustomerID: "OWNER", Domain: "com"},
	}

	devs, err := ds.List(context.Background())
	require.NoError(t, err)

	var aqm *Device
	for _, d := range devs {
		if d.SerialNumber == "S-AQM-ACME-1" {
			aqm = d
			break
		}
	}
	require.NotNil(t, aqm, "non-Amazon AQM must be surfaced in List output")
	assert.Equal(t, "AIR_QUALITY_MONITOR", aqm.DeviceFamily)
	assert.Equal(t, "Acme", aqm.Manufacturer,
		"AQM Manufacturer must be the parsed value, not hardcoded to Amazon")
	assert.Equal(t, "42", aqm.SoftwareVersion)
	assert.Equal(t, "Office Acme Monitor", aqm.AccountName)
	assert.Equal(t, "ep-aqm-acme", aqm.EndpointID)
	// AQMs come from the GraphQL response without a deviceOwnerCustomerId
	// field, so we attribute ownership to the session's AccountCustomerID
	// and HouseholdDevice mirrors the non-AQM branch: true when ownership
	// resolves to the current account.
	assert.Equal(t, "OWNER", aqm.DeviceOwnerCustomerID,
		"AQM DeviceOwnerCustomerID should default to session AccountCustomerID")
	assert.True(t, aqm.HouseholdDevice,
		"AQM HouseholdDevice should be true when the session attributes ownership")
}

// TestSetDefault_NilReturnsError verifies that SetDefault rejects a nil
// *Device with a descriptive error.
func TestSetDefault_NilReturnsError(t *testing.T) {
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: &http.Client{},
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper:        wr,
		cache:          map[string]*Device{},
		lastDailyFetch: time.Now(),
	}
	err = ds.SetDefault(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil device")
}

// TestSetDefault_NotInCacheReturnsError verifies that SetDefault rejects a
// *Device whose serial isn't currently in the cache.
func TestSetDefault_NotInCacheReturnsError(t *testing.T) {
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: &http.Client{},
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	ds := &DevicesService{
		wrapper: wr,
		cache: map[string]*Device{
			"S-AAA": {SerialNumber: "S-AAA", Online: true},
		},
		lastDailyFetch: time.Now(),
	}
	stranger := &Device{SerialNumber: "S-NOT-IN-CACHE", Online: true}
	err = ds.SetDefault(stranger)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// TestSetDefault_ValidDevicePersisted verifies the happy path: a device
// already in the cache is accepted and is returned by subsequent calls to
// Default().
func TestSetDefault_ValidDevicePersisted(t *testing.T) {
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: &http.Client{},
		Site:       "https://www.amazon.com",
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	pick := &Device{SerialNumber: "S-PICK", Online: true}
	ds := &DevicesService{
		wrapper: wr,
		cache: map[string]*Device{
			"S-AAA":  {SerialNumber: "S-AAA", Online: true},
			"S-PICK": pick,
		},
		lastDailyFetch: time.Now(),
	}
	require.NoError(t, ds.SetDefault(pick))
	got, err := ds.Default(context.Background())
	require.NoError(t, err)
	assert.Same(t, pick, got)
}
