package alexactl

import (
	"context"
	"fmt"

	"github.com/Khan/genqlient/graphql"

	"github.com/Fishwaldo/alexactl/internal/alexagraphql"
	"github.com/Fishwaldo/alexactl/internal/amzconst"
	"github.com/Fishwaldo/alexactl/internal/httpwrapper"
)

// smartHomeClient is the internal POST helper that backs every typed
// control accessor (Device.Power(), Device.Brightness(), …) and the
// generic Device.SendDirective escape hatch. It owns nothing besides
// a reference to the shared httpwrapper.Wrapper; concurrent use is
// safe because the wrapper is.
type smartHomeClient struct {
	wrapper *httpwrapper.Wrapper
}

// newSmartHomeClient is the only constructor; called from client.go
// at NewClient time. The wrapper is the same one DevicesService /
// SensorsService use, so auth refresh + retry + scrub all carry over.
func newSmartHomeClient(w *httpwrapper.Wrapper) *smartHomeClient {
	return &smartHomeClient{wrapper: w}
}

// FeatureControl is one entry in a setEndpointFeatures mutation.
// Payload is nil for no-payload operations (turnOn / turnOff); for
// operations that take a payload, the shape mirrors what the captured
// web-UI mutations send (e.g. {"brightness": 45} for setBrightness,
// {"color": {"hue": 348, "saturation": 0.91, "brightness": 1}} for
// setColor).
//
// Instance is the optional feature instance discriminator (e.g.
// "Apparatus.FanSpeed" for a fan-speed mode). Phase 4b leaves it
// empty for Power and Brightness; Phase 4c+ may populate it for
// Range/Mode generic controllers.
type FeatureControl struct {
	EndpointID           string
	FeatureName          string
	FeatureOperationName string
	Instance             string
	Payload              map[string]any
}

// FeatureControlResponse is the public alias for one success entry in
// the setEndpointFeatures response. Callers inspect Code to confirm
// the directive landed (typically "SUCCESS").
type FeatureControlResponse = alexagraphql.FeatureControlSuccess

// FeatureControlErrorResponse is the public alias for one error entry
// in the setEndpointFeatures response. Code is the upstream symbolic
// status (e.g. "INVALID_VALUE", "ENDPOINT_UNREACHABLE"); Message is
// the upstream human-readable string.
type FeatureControlErrorResponse = alexagraphql.FeatureControlError

// setEndpointFeatures POSTs the mutation. Returns the parallel
// success-list / error-list pair that Amazon returns; the caller
// decides per-entry policy (typed control structs return the first
// error wrapped in ErrCannotControlDevice; the batched-write Phase
// 4c+ surface may want partial-success semantics).
//
// A transport-level failure (network down, 5xx, malformed JSON)
// returns a non-nil err wrapped in ErrCannotConnect. Structured
// errors from Amazon (INVALID_VALUE, …) surface in the fails return
// value with a nil err — they're protocol-level, not transport.
func (c *smartHomeClient) setEndpointFeatures(
	ctx context.Context,
	controls []FeatureControl,
) (successes []*FeatureControlResponse, fails []*FeatureControlErrorResponse, err error) {
	if len(controls) == 0 {
		return nil, nil, nil
	}

	reqs := make([]alexagraphql.FeatureControlRequest, len(controls))
	for i, ctrl := range controls {
		ep := ctrl.EndpointID
		reqs[i] = alexagraphql.FeatureControlRequest{
			EndpointId:           &ep,
			FeatureName:          alexagraphql.FeatureName(ctrl.FeatureName),
			FeatureOperationName: alexagraphql.FeatureOperationName(ctrl.FeatureOperationName),
		}
		if ctrl.Instance != "" {
			ins := ctrl.Instance
			reqs[i].Instance = &ins
		}
		if ctrl.Payload != nil {
			var p interface{} = ctrl.Payload
			reqs[i].Payload = &p
		}
	}

	url := fmt.Sprintf("https://alexa.amazon.%s%s",
		c.wrapper.Domain(), amzconst.URINexusGraphQL)
	gql := graphql.NewClient(url, alexagraphql.NewDoer(c.wrapper))
	resp, err := alexagraphql.SetEndpointFeatures(ctx, gql, reqs)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrCannotConnect, err)
	}
	if resp == nil || resp.SetEndpointFeatures == nil {
		return nil, nil, fmt.Errorf("%w: empty setEndpointFeatures response", ErrCannotRetrieveData)
	}
	return resp.SetEndpointFeatures.FeatureControlResponses,
		resp.SetEndpointFeatures.Errors,
		nil
}
