package alexactl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDevice_SendDirective_PostsV1Mutation verifies SendDirective
// routes through the V1 setEndpointFeatures mutation (NOT the gated
// V2 variant). The instance parameter is forwarded as the requests[]
// instance field.
func TestDevice_SendDirective_PostsV1Mutation(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[{"endpointId":"ep-1","featureName":"customX","featureOperationName":"customOp","instance":"Apparatus.X","code":"SUCCESS"}],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber: "S-1",
		EndpointID:   "ep-1",
		controller:   newSmartHomeClient(wr),
	}

	err := d.SendDirective(context.Background(), "customX", "Apparatus.X", "customOp", map[string]any{"foo": "bar"})
	require.NoError(t, err)
	assert.Equal(t, "setEndpointFeatures", gotBody["operationName"],
		"SendDirective must call V1 (setEndpointFeatures), NOT V2 (setEndpointFeaturesV2)")

	vars, _ := gotBody["variables"].(map[string]any)
	require.NotNil(t, vars)
	reqs, _ := vars["requests"].([]any)
	require.Len(t, reqs, 1)
	r0 := reqs[0].(map[string]any)
	assert.Equal(t, "ep-1", r0["endpointId"])
	assert.Equal(t, "customX", r0["featureName"])
	assert.Equal(t, "customOp", r0["featureOperationName"])
	assert.Equal(t, "Apparatus.X", r0["instance"],
		"instance should be propagated to the V1 request")
	payload, _ := r0["payload"].(map[string]any)
	require.NotNil(t, payload)
	assert.Equal(t, "bar", payload["foo"])
}

func TestDevice_SendDirective_ErrWithoutEndpointID(t *testing.T) {
	d := &Device{SerialNumber: "S-1", controller: &smartHomeClient{}}
	err := d.SendDirective(context.Background(), "power", "", "turnOn", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

func TestDevice_SendDirective_ErrWithoutController(t *testing.T) {
	d := &Device{SerialNumber: "S-1", EndpointID: "ep-1"}
	err := d.SendDirective(context.Background(), "power", "", "turnOn", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}
