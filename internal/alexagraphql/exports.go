package alexagraphql

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

// Exported type aliases for the generated (unexported) types produced by the
// getEndpointState query. Callers in package amazondevices use these names to
// avoid referencing the long internal generated names directly.

// EndpointStateEndpoint is the generated endpoint type for getEndpointState.
type EndpointStateEndpoint = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpoint

// EndpointStateFeature is the generated feature type for getEndpointState.
type EndpointStateFeature = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeature

// EndpointStateProperty is the generated polymorphic property interface.
type EndpointStateProperty = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesFeatureProperty

// EndpointStatePropReachability is the Reachability property concrete type.
type EndpointStatePropReachability = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesReachability

// EndpointStatePropTemperature is the TemperatureSensor property concrete type.
type EndpointStatePropTemperature = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesTemperatureSensor

// EndpointStatePropDetectionState is the DetectionState property concrete type.
type EndpointStatePropDetectionState = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesDetectionState

// EndpointStatePropIlluminance is the Illuminance property concrete type.
type EndpointStatePropIlluminance = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesIlluminance

// EndpointStatePropRangeValue is the RangeValue property concrete type.
type EndpointStatePropRangeValue = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesRangeValue

// EndpointStatePropPower is the Power property concrete type produced by
// the getEndpointState query.
type EndpointStatePropPower = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesPower

// EndpointStatePropBrightness is the Brightness property concrete type.
type EndpointStatePropBrightness = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesBrightness

// EndpointStatePropColor is the Color property concrete type.
type EndpointStatePropColor = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesColor

// EndpointStatePropColorTemperature is the ColorTemperature property concrete type.
type EndpointStatePropColorTemperature = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesColorTemperature

// EndpointStatePropLock is the Lock property concrete type.
type EndpointStatePropLock = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesLock

// EndpointStatePropSetpoint is the Setpoint property concrete type.
type EndpointStatePropSetpoint = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesSetpoint

// EndpointStatePropThermostatMode is the ThermostatMode property concrete type.
type EndpointStatePropThermostatMode = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesThermostatMode

// EndpointStatePropMode is the generic Mode property concrete type
// (Alexa.ModeController state), distinct from ThermostatMode.
type EndpointStatePropMode = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesMode

// EndpointStatePropToggleState is the ToggleState property concrete type
// (Alexa.ToggleController state).
type EndpointStatePropToggleState = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesToggleState

// EndpointStatePropEnablementState is the EnablementState property
// concrete type — used by motion-sensor sub-properties to indicate
// whether the sensor is ENABLED / DISABLED.
type EndpointStatePropEnablementState = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesEnablementState

// EndpointStatePropDetectionSensitivity is the DetectionSensitivity
// property concrete type — LOW / MEDIUM / HIGH motion-sensor setting.
type EndpointStatePropDetectionSensitivity = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesDetectionSensitivity

// EndpointStatePropDetectionRange is the DetectionRange property
// concrete type — LOW / MEDIUM / HIGH motion-sensor range setting.
type EndpointStatePropDetectionRange = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesDetectionRange

// EndpointStatePropDetectionTechnology is the DetectionTechnology
// property concrete type — ULTRASONIC_BASIC /
// ULTRASONIC_ENHANCED_WITH_WIFI_CSI etc., the underlying motion-sensor
// hardware mode.
type EndpointStatePropDetectionTechnology = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeaturePropertiesDetectionTechnology

// EndpointStateConfigThermostat is the exported alias for the generated
// ThermostatConfiguration variant of feature.configuration.
type EndpointStateConfigThermostat = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeatureConfigurationThermostatConfiguration

// EndpointStateConfigMode is the ModeConfiguration variant of
// feature.configuration — carries the list of valid mode option
// strings the device accepts plus a friendly name per option.
type EndpointStateConfigMode = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeatureConfigurationModeConfiguration

// EndpointStateConfigRange is the RangeConfiguration variant of
// feature.configuration — carries min / max / precision, the unit of
// measure, and any named presets the device exposes.
type EndpointStateConfigRange = getEndpointStateListEndpointsListEndpointsResponseEndpointsEndpointFeaturesFeatureConfigurationRangeConfiguration

// endpointStateListEndpoints is an unexported alias used only within this
// package to keep GetEndpointStateResponse's field type concise.
type endpointStateListEndpoints = getEndpointStateListEndpointsListEndpointsResponse

// DevicesEndpoint is the exported alias for the generated (unexported) endpoint
// type produced by the getDevicesBaseData query. Callers outside this package
// use this type to avoid referencing the long internal type name.
type DevicesEndpoint = getDevicesBaseDataAllDevicesListEndpointsResponseEndpointsEndpoint

// DevicesAllDevices is the exported alias for the generated (unexported) allDevices
// wrapper type produced by the getDevicesBaseData query.
type DevicesAllDevices = getDevicesBaseDataAllDevicesListEndpointsResponse

// ListEndpointsResponse is the exported response type for ListEndpoints, mirroring
// the structure of the generated unexported response so callers can access AllDevices
// and AllDevices.Endpoints without importing internal generated names.
type ListEndpointsResponse struct {
	AllDevices *DevicesAllDevices
}

// ListEndpoints calls the getDevicesBaseData GraphQL query and returns the
// response. The wire operationName is "getDevicesBaseData" (matching what
// Amazon's Nexus endpoint expects). The generated function and its response
// types are unexported because genqlient derives Go names from the query
// operation name; this wrapper re-exports the result as a typed struct.
func ListEndpoints(ctx context.Context, client graphql.Client) (*ListEndpointsResponse, error) {
	resp, err := getDevicesBaseData(ctx, client)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &ListEndpointsResponse{AllDevices: resp.AllDevices}, nil
}

// GetEndpointStateResponse is the exported response type for GetEndpointState.
// It mirrors the structure of the generated unexported response so callers can
// access ListEndpoints without referencing internal generated names.
type GetEndpointStateResponse struct {
	ListEndpoints *endpointStateListEndpoints
}

// GetEndpointState calls the getEndpointState GraphQL query and returns the
// response. The wire operationName is "getEndpointState" (matching what
// Amazon's Nexus endpoint expects). The generated function and its response
// types are unexported because genqlient derives Go names from the query
// operation name; this wrapper re-exports the result as a typed struct.
//
// endpointIDs is converted to []*string internally because the schema
// declares endpointIds as [String] (nullable element type).
func GetEndpointState(ctx context.Context, client graphql.Client, endpointIDs []string) (*GetEndpointStateResponse, error) {
	ptrs := make([]*string, len(endpointIDs))
	for i := range endpointIDs {
		s := endpointIDs[i]
		ptrs[i] = &s
	}
	resp, err := getEndpointState(ctx, client, ptrs)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &GetEndpointStateResponse{ListEndpoints: resp.ListEndpoints}, nil
}

// EndpointAllFeaturesEndpoint is the exported alias for the generated
// (unexported) endpoint type produced by the endpointAllFeatures query.
type EndpointAllFeaturesEndpoint = endpointAllFeaturesEndpoint

// EndpointAllFeaturesProperty is the exported alias for the generated
// polymorphic property interface produced by the endpointAllFeatures
// query. Distinct from EndpointStateProperty because genqlient
// generates per-operation types.
type EndpointAllFeaturesProperty = endpointAllFeaturesEndpointFeaturesFeaturePropertiesFeatureProperty

// EndpointAllFeaturesEndpointFeaturesFeature is the alias for the
// generated Feature type produced by the endpointAllFeatures query —
// referenced as a parameter type by inspect_parser.go.
type EndpointAllFeaturesEndpointFeaturesFeature = endpointAllFeaturesEndpointFeaturesFeature

// EndpointAllFeaturesConfigThermostat is the alias for the
// ThermostatConfiguration variant of feature.configuration in the
// endpointAllFeatures deep query.
type EndpointAllFeaturesConfigThermostat = endpointAllFeaturesEndpointFeaturesFeatureConfigurationThermostatConfiguration

// EndpointAllFeaturesConfigMode is the ModeConfiguration variant of
// feature.configuration on the deep inspect query path.
type EndpointAllFeaturesConfigMode = endpointAllFeaturesEndpointFeaturesFeatureConfigurationModeConfiguration

// EndpointAllFeaturesConfigRange is the RangeConfiguration variant of
// feature.configuration on the deep inspect query path.
type EndpointAllFeaturesConfigRange = endpointAllFeaturesEndpointFeaturesFeatureConfigurationRangeConfiguration

// EndpointAllFeaturesPropPower is the Power property concrete type
// produced by the endpointAllFeatures (deep) query.
type EndpointAllFeaturesPropPower = endpointAllFeaturesEndpointFeaturesFeaturePropertiesPower

// The following aliases provide one exported name per concrete property
// type the endpointAllFeatures (deep) parser switches on.

// EndpointAllFeaturesPropBrightness is the Brightness property concrete type.
type EndpointAllFeaturesPropBrightness = endpointAllFeaturesEndpointFeaturesFeaturePropertiesBrightness

// EndpointAllFeaturesPropColor is the Color property concrete type.
type EndpointAllFeaturesPropColor = endpointAllFeaturesEndpointFeaturesFeaturePropertiesColor

// EndpointAllFeaturesPropColorTemperature is the ColorTemperature property concrete type.
type EndpointAllFeaturesPropColorTemperature = endpointAllFeaturesEndpointFeaturesFeaturePropertiesColorTemperature

// EndpointAllFeaturesPropLock is the Lock property concrete type.
type EndpointAllFeaturesPropLock = endpointAllFeaturesEndpointFeaturesFeaturePropertiesLock

// EndpointAllFeaturesPropSetpoint is the Setpoint property concrete type.
type EndpointAllFeaturesPropSetpoint = endpointAllFeaturesEndpointFeaturesFeaturePropertiesSetpoint

// EndpointAllFeaturesPropThermostatMode is the ThermostatMode property concrete type.
type EndpointAllFeaturesPropThermostatMode = endpointAllFeaturesEndpointFeaturesFeaturePropertiesThermostatMode

// EndpointAllFeaturesPropTemperatureSensor is the TemperatureSensor property concrete type.
type EndpointAllFeaturesPropTemperatureSensor = endpointAllFeaturesEndpointFeaturesFeaturePropertiesTemperatureSensor

// EndpointAllFeaturesPropRangeValue is the RangeValue property concrete type.
type EndpointAllFeaturesPropRangeValue = endpointAllFeaturesEndpointFeaturesFeaturePropertiesRangeValue

// EndpointAllFeaturesPropIlluminance is the Illuminance property concrete type.
type EndpointAllFeaturesPropIlluminance = endpointAllFeaturesEndpointFeaturesFeaturePropertiesIlluminance

// EndpointAllFeaturesPropDetectionState is the DetectionState property concrete type.
type EndpointAllFeaturesPropDetectionState = endpointAllFeaturesEndpointFeaturesFeaturePropertiesDetectionState

// EndpointAllFeaturesPropReachability is the Reachability property concrete type.
type EndpointAllFeaturesPropReachability = endpointAllFeaturesEndpointFeaturesFeaturePropertiesReachability

// EndpointAllFeaturesPropMode is the generic Mode property concrete type.
type EndpointAllFeaturesPropMode = endpointAllFeaturesEndpointFeaturesFeaturePropertiesMode

// EndpointAllFeaturesPropToggleState is the ToggleState property concrete type.
type EndpointAllFeaturesPropToggleState = endpointAllFeaturesEndpointFeaturesFeaturePropertiesToggleState

// EndpointAllFeaturesPropEnablementState is the EnablementState property concrete type.
type EndpointAllFeaturesPropEnablementState = endpointAllFeaturesEndpointFeaturesFeaturePropertiesEnablementState

// EndpointAllFeaturesPropDetectionSensitivity is the DetectionSensitivity property concrete type.
type EndpointAllFeaturesPropDetectionSensitivity = endpointAllFeaturesEndpointFeaturesFeaturePropertiesDetectionSensitivity

// EndpointAllFeaturesPropDetectionRange is the DetectionRange property concrete type.
type EndpointAllFeaturesPropDetectionRange = endpointAllFeaturesEndpointFeaturesFeaturePropertiesDetectionRange

// EndpointAllFeaturesPropDetectionTechnology is the DetectionTechnology property concrete type.
type EndpointAllFeaturesPropDetectionTechnology = endpointAllFeaturesEndpointFeaturesFeaturePropertiesDetectionTechnology

// EndpointAllFeaturesResponse is the exported response type for
// EndpointAllFeatures, mirroring the structure of the generated
// unexported response.
type EndpointAllFeaturesResponse struct {
	Endpoint *EndpointAllFeaturesEndpoint
}

// EndpointAllFeatures calls the endpointAllFeatures GraphQL query
// (typically used by DevicesService.Inspect). The wire operationName is
// "endpointAllFeatures" (matches what the live web UI sends). Pass
// nil for latencyTolerance to use the schema's default; LOW is the
// usual choice (fresh data, no caching).
func EndpointAllFeatures(ctx context.Context, client graphql.Client, endpointID string, latencyTolerance *LatencyToleranceValue) (*EndpointAllFeaturesResponse, error) {
	resp, err := endpointAllFeatures(ctx, client, endpointID, latencyTolerance)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &EndpointAllFeaturesResponse{Endpoint: resp.Endpoint}, nil
}

// SetEndpointFeaturesResponse is the exported response type for
// SetEndpointFeatures, mirroring the structure of the generated
// unexported response so callers can access featureControlResponses
// and errors without referencing internal generated names.
type SetEndpointFeaturesResponse struct {
	SetEndpointFeatures *setEndpointFeaturesSetEndpointFeaturesSetEndpointFeaturesResponse
}

// FeatureControlSuccess is the exported alias for one entry in the
// successful-responses array of setEndpointFeatures. Callers inspect
// the upstream `code` field to confirm the directive landed.
type FeatureControlSuccess = setEndpointFeaturesSetEndpointFeaturesSetEndpointFeaturesResponseFeatureControlResponsesFeatureControlResponse

// FeatureControlError is the exported alias for one entry in the
// errors array of setEndpointFeatures. Callers read `code` and
// `message` to populate the wrapped error returned by the typed
// control structs.
type FeatureControlError = setEndpointFeaturesSetEndpointFeaturesSetEndpointFeaturesResponseErrorsFeatureControlError

// SetEndpointFeatures calls the setEndpointFeatures GraphQL mutation
// (used by smartHomeClient). The wire operationName is
// "setEndpointFeatures" (matches what the live web UI sends). Pass an
// arbitrary number of FeatureControlRequest entries; Phase 4b only
// ever sends one. The generated FeatureControlRequest input type and
// the FeatureName / FeatureOperationName enums are re-exported below
// for service-layer use.
func SetEndpointFeatures(ctx context.Context, client graphql.Client, requests []FeatureControlRequest) (*SetEndpointFeaturesResponse, error) {
	ptrs := make([]*FeatureControlRequest, len(requests))
	for i := range requests {
		r := requests[i]
		ptrs[i] = &r
	}
	resp, err := setEndpointFeatures(ctx, client, ptrs)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &SetEndpointFeaturesResponse{SetEndpointFeatures: resp.SetEndpointFeatures}, nil
}

// SetEndpointFeaturesV2Response is the exported response type for
// SetEndpointFeaturesV2, mirroring the generated unexported response.
type SetEndpointFeaturesV2Response struct {
	SetEndpointFeaturesV2 *setEndpointFeaturesV2SetEndpointFeaturesV2SetGenericEndpointFeaturesResponse
}

// SetEndpointFeaturesV2 calls the setEndpointFeaturesV2 GraphQL mutation
// (used by Device.SendDirective). Unlike SetEndpointFeatures, this
// variant accepts string-based feature/operation names — useful for
// directives we haven't typed in the schema enums.
func SetEndpointFeaturesV2(ctx context.Context, client graphql.Client, input SetGenericEndpointFeaturesInput) (*SetEndpointFeaturesV2Response, error) {
	resp, err := setEndpointFeaturesV2(ctx, client, &input)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return &SetEndpointFeaturesV2Response{SetEndpointFeaturesV2: resp.SetEndpointFeaturesV2}, nil
}
