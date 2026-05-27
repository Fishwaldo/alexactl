package amazondevices

import (
	"context"
	"fmt"

	"github.com/Fishwaldo/goamazondevices/internal/amzconst"
)

// LockControl is the typed accessor returned by Device.Lock() for
// devices that expose Alexa.LockController. Two methods — Lock and
// Unlock — each issue one payload-less setEndpointFeatures mutation.
// On HTTP success the corresponding Device.Sensors["lockState"]
// entry is updated optimistically; the next getEndpointState refresh
// overwrites with the canonical value.
//
// Unlock may be unsupported per-device — some smart locks disable
// remote unlock for safety (BHMA/Z-Wave "auto-relock only" modes,
// Yale's "remote unlock disabled" setting, August's "owner-only"
// policy). In that case Amazon returns a structured error in the
// directive response which surfaces as a wrapped
// ErrCannotControlDevice. Lock() should always work where the
// controller is exposed.
//
// No Toggle: lock/unlock asymmetry isn't useful — "flip whatever
// the current state is" semantics on a physical lock is dangerous
// UX. Callers wanting that behavior compose it from
// Sensors["lockState"] themselves.
type LockControl struct {
	device *Device
}

// Lock returns a typed lock-controller accessor, or nil when the
// device does not advertise Alexa.LockController in AlexaInterfaces,
// has no EndpointID, or has no back-reference to a smartHomeClient
// (e.g. a *Device built by hand outside NewClient).
//
// Callers MUST nil-check: dvc.Lock().Lock(ctx) panics if Lock()
// returned nil. This is intentional — a silent no-op on
// non-controllable devices would mask programmer error.
func (d *Device) Lock() *LockControl {
	if d == nil || d.controller == nil || d.EndpointID == "" {
		return nil
	}
	if !d.hasInterface(amzconst.InterfaceLockController) {
		return nil
	}
	return &LockControl{device: d}
}

// Lock issues a lock directive. On HTTP 200 with no structured error,
// Device.Sensors["lockState"].Value is set to "LOCKED".
func (c *LockControl) Lock(ctx context.Context) error {
	return c.send(ctx, "lock", string(LockStateLocked))
}

// Unlock issues an unlock directive. On success, Sensors["lockState"]
// is set to "UNLOCKED". Unlock is the operation most likely to fail
// with a structured error — many locks disable remote unlock by
// policy. The error wraps ErrCannotControlDevice so callers can
// distinguish a refusal from a transport failure.
func (c *LockControl) Unlock(ctx context.Context) error {
	return c.send(ctx, "unlock", string(LockStateUnlocked))
}

// send is the common path: build a single-entry FeatureControl, POST
// via smartHomeClient, translate structured errors into a wrapped
// ErrCannotControlDevice, and apply the optimistic Sensor update on
// success.
func (c *LockControl) send(ctx context.Context, op, optimisticValue string) error {
	_, fails, err := c.device.controller.setEndpointFeatures(ctx, []FeatureControl{
		{
			EndpointID:           c.device.EndpointID,
			FeatureName:          "lock",
			FeatureOperationName: op,
		},
	})
	if err != nil {
		return err
	}
	if len(fails) > 0 {
		f := fails[0]
		msg := ""
		if f.Message != nil {
			msg = *f.Message
		}
		return fmt.Errorf("%w: %s: %s on %s (%s)",
			ErrCannotControlDevice, f.Code, op, c.device.SerialNumber, msg)
	}
	if c.device.Sensors == nil {
		c.device.Sensors = map[string]*DeviceSensor{}
	}
	if existing, ok := c.device.Sensors["lockState"]; ok && existing != nil {
		existing.Value = optimisticValue
	} else {
		c.device.Sensors["lockState"] = &DeviceSensor{
			Name:  "lockState",
			Value: optimisticValue,
		}
	}
	return nil
}
