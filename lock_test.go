package amazondevices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

func TestDevice_Lock_NilWhenNoLockController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{"Alexa.PowerController"}, // no LockController
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Lock(), "Lock() must return nil when device doesn't advertise Alexa.LockController")
}

func TestDevice_Lock_NilWhenNoController(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceLockController},
		// controller intentionally nil — e.g., a *Device built by hand in a test.
	}
	assert.Nil(t, d.Lock(), "Lock() must return nil when device has no controller back-reference")
}

func TestDevice_Lock_NilWhenNoEndpointID(t *testing.T) {
	d := &Device{
		SerialNumber:    "S-1",
		AlexaInterfaces: []string{amzconst.InterfaceLockController},
		controller:      &smartHomeClient{},
	}
	assert.Nil(t, d.Lock(), "Lock() must return nil when device has no EndpointID")
}

func TestLockControl_Lock_Unlock_PostsCorrectFeatureControl(t *testing.T) {
	var lastOpName string
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		vars, _ := body["variables"].(map[string]any)
		reqs, _ := vars["requests"].([]any)
		r0 := reqs[0].(map[string]any)
		lastOpName = r0["featureOperationName"].(string)
		assert.Equal(t, "lock", r0["featureName"])
		assert.Equal(t, "ep-1", r0["endpointId"])
		assert.Nil(t, r0["payload"], "lock/unlock directives must carry no payload")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "data": {"setEndpointFeatures": {
		    "featureControlResponses": [{"endpointId": "ep-1", "featureName": "lock", "featureOperationName": "` + lastOpName + `", "instance": null, "code": "SUCCESS"}],
		    "errors": []
		  }}
		}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceLockController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	lock := d.Lock()
	require.NotNil(t, lock, "Lock() must return non-nil for devices with Alexa.LockController")

	require.NoError(t, lock.Lock(context.Background()))
	assert.Equal(t, "lock", lastOpName)
	require.NotNil(t, d.Sensors["lockState"])
	assert.Equal(t, "LOCKED", d.Sensors["lockState"].Value, "Sensors[lockState] should be optimistically LOCKED after Lock()")

	require.NoError(t, lock.Unlock(context.Background()))
	assert.Equal(t, "unlock", lastOpName)
	require.NotNil(t, d.Sensors["lockState"])
	assert.Equal(t, "UNLOCKED", d.Sensors["lockState"].Value, "Sensors[lockState] should be optimistically UNLOCKED after Unlock()")

	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestLockControl_StructuredErrorWrapsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"setEndpointFeatures":{
		  "featureControlResponses": [],
		  "errors": [{"endpointId": "ep-1", "featureName": "lock", "featureOperationName": "unlock", "instance": null, "code": "NOT_SUPPORTED_IN_CURRENT_MODE", "message": "remote unlock disabled by user"}]
		}}}`))
	}))
	t.Cleanup(srv.Close)

	wr := buildSmartHomeTestWrapper(t, srv)
	d := &Device{
		SerialNumber:    "S-1",
		EndpointID:      "ep-1",
		AlexaInterfaces: []string{amzconst.InterfaceLockController},
		controller:      newSmartHomeClient(wr),
		Sensors:         map[string]*DeviceSensor{},
	}

	err := d.Lock().Unlock(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCannotControlDevice)
	assert.Contains(t, err.Error(), "NOT_SUPPORTED_IN_CURRENT_MODE")
	assert.Contains(t, err.Error(), "remote unlock disabled by user")
	assert.True(t, strings.Contains(err.Error(), "ep-1") || strings.Contains(err.Error(), "S-1"),
		"error message should identify the failed device")
	assert.Nil(t, d.Sensors["lockState"], "Sensors[lockState] must not be written on structured error")
}
