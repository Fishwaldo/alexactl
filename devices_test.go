package amazondevices

import (
	"bytes"
	"context"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

//go:embed testdata/devices/base_list.json
var baseListJSON []byte

func TestDevices_ListBase_FiltersIgnoredAndNoMicrophone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.URL.Path, amzconst.URIDevices))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(baseListJSON)
	}))
	defer srv.Close()

	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: srv.Client(),
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)

	ds := &DevicesService{wrapper: wr, session: &Session{AccountCustomerID: "OWNER", Domain: "com"}}

	devs, err := ds.fetchBase(context.Background(), srv.URL)
	require.NoError(t, err)

	assert.Len(t, devs, 2)
	assert.Contains(t, devs, "S-KITCHEN-001")
	assert.Contains(t, devs, "S-PAIR-001")

	kitchen := devs["S-KITCHEN-001"]
	assert.True(t, kitchen.Online)
	assert.True(t, kitchen.SchedulesSupported, "REMINDERS in capabilities → true")
	assert.True(t, kitchen.MediaPlayerSupported, "AUDIO_PLAYER in capabilities → true")
	assert.True(t, kitchen.HouseholdDevice, "deviceOwnerCustomerId matches session")

	pair := devs["S-PAIR-001"]
	assert.True(t, pair.IsCluster())
	assert.Contains(t, pair.ClusterMembers, "S-PAIR-LEFT")
	assert.Contains(t, pair.ClusterMembers, "S-PAIR-RIGHT")
}
