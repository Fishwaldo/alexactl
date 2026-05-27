package amazondevices

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/alexagraphql"
	"github.com/Fishwaldo/goamazondevices/internal/httpwrapper"
)

func TestSmartHomeClient_SetEndpointFeatures_PostsCorrectPayload(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {
		    "setEndpointFeatures": {
		      "featureControlResponses": [
		        {"endpointId": "ep-1", "featureName": "power", "featureOperationName": "turnOn", "instance": null, "code": "SUCCESS"}
		      ],
		      "errors": []
		    }
		  }
		}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	c := newSmartHomeClient(wr)
	successes, fails, err := c.setEndpointFeatures(context.Background(), []FeatureControl{
		{EndpointID: "ep-1", FeatureName: "power", FeatureOperationName: "turnOn"},
	})
	require.NoError(t, err)
	assert.Len(t, successes, 1)
	assert.Empty(t, fails)
	assert.True(t, strings.HasSuffix(gotPath, "/nexus/v1/graphql"),
		"unexpected path: %s", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "setEndpointFeatures", gotBody["operationName"])

	vars, _ := gotBody["variables"].(map[string]any)
	require.NotNil(t, vars)
	reqs, _ := vars["requests"].([]any)
	require.Len(t, reqs, 1)
	r0 := reqs[0].(map[string]any)
	assert.Equal(t, "ep-1", r0["endpointId"])
	assert.Equal(t, "power", r0["featureName"])
	assert.Equal(t, "turnOn", r0["featureOperationName"])
}

func TestSmartHomeClient_SetEndpointFeatures_StructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {
		    "setEndpointFeatures": {
		      "featureControlResponses": [],
		      "errors": [
		        {"endpointId": "ep-1", "featureName": "brightness", "featureOperationName": "setBrightness", "instance": null, "code": "INVALID_VALUE", "message": "out of range"}
		      ]
		    }
		  }
		}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	c := newSmartHomeClient(wr)
	successes, fails, err := c.setEndpointFeatures(context.Background(), []FeatureControl{
		{EndpointID: "ep-1", FeatureName: "brightness", FeatureOperationName: "setBrightness", Payload: map[string]any{"brightness": 150}},
	})
	require.NoError(t, err, "structured errors should not be returned as Go errors — caller maps them")
	assert.Empty(t, successes)
	require.Len(t, fails, 1)
	assert.Equal(t, "INVALID_VALUE", fails[0].Code)
	require.NotNil(t, fails[0].Message)
	assert.Equal(t, "out of range", *fails[0].Message)
}

func TestSmartHomeClient_SetEndpointFeatures_BatchShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{"featureControlResponses":[],"errors":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	c := newSmartHomeClient(wr)
	_, _, err := c.setEndpointFeatures(context.Background(), []FeatureControl{
		{EndpointID: "ep-1", FeatureName: "power", FeatureOperationName: "turnOn"},
		{EndpointID: "ep-2", FeatureName: "power", FeatureOperationName: "turnOff"},
	})
	require.NoError(t, err)

	vars, _ := gotBody["variables"].(map[string]any)
	reqs, _ := vars["requests"].([]any)
	assert.Len(t, reqs, 2, "batched requests pass through as a single mutation")
}

// buildSmartHomeTestWrapper builds a httpwrapper.Wrapper pointed at
// srv with the hostRewriter so production code's https://alexa.amazon.com
// URLs land on the httptest server. The session is irrelevant for write
// path (no auth refresh is exercised by these tests).
func buildSmartHomeTestWrapper(t *testing.T, srv *httptest.Server) *httpwrapper.Wrapper {
	t.Helper()
	hc := srv.Client()
	hc.Transport = &hostRewriter{base: srv.URL, inner: srv.Client().Transport}
	wr, err := httpwrapper.New(httpwrapper.Config{
		HTTPClient: hc,
		Site:       srv.URL,
		RandReader: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)),
	})
	require.NoError(t, err)
	return wr
}

// Compile-time check that the alexagraphql shim aliases referenced
// in smarthome.go exist; if Task 2's regeneration changed names, this
// test catches it before runtime.
var _ alexagraphql.FeatureControlRequest
