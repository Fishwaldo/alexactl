package queries_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Khan/genqlient/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/alexagraphql"
)

// TestListEndpoints_RoundTrip uses the existing devices_endpoints fixture
// to confirm the generated client parses a real-shaped response into
// typed Go structs.
func TestListEndpoints_RoundTrip(t *testing.T) {
	fixture, err := os.ReadFile("../../../testdata/devices/endpoints_graphql.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	client := graphql.NewClient(srv.URL, http.DefaultClient)
	resp, err := alexagraphql.ListEndpoints(context.Background(), client)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.AllDevices, "allDevices should not be nil")
	assert.NotEmpty(t, resp.AllDevices.Endpoints, "fixture should yield at least one endpoint")
}
