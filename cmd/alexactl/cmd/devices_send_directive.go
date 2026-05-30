package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	sendDirectiveInstance string
	sendDirectiveDevice   string
)

var devicesSendDirectiveCmd = &cobra.Command{
	Use:   "send-directive <featureName> <operationName> [json-payload]",
	Short: "Send an arbitrary smart-home directive via the V2 mutation (escape hatch)",
	Long: `Issues any setEndpointFeaturesV2 directive against the target device.
Mirrors Device.SendDirective for exploratory testing of unverified directives — call
any featureName / operationName / payload to see what Amazon accepts.

The target device is the --device flag (serial or name) or the configured
default-device.

Use --instance for features that support multiple instances per
device (Mode, Range, Toggle controllers). For singleton features
(Power, Brightness, Color, ColorTemperature, Thermostat, Lock) you
can leave it off.

Examples (singleton features):
  devices send-directive power "" turnOn --device "Lamp"
  devices send-directive brightness "" setBrightness '{"brightness": 60}'

Examples (mode features with instance):
  devices send-directive mode setMode \
      '{"mode":"FanSpeed.High"}' --instance Pure.FanSpeed --device "Air Purifier"
  devices send-directive mode setMode \
      '{"mode":"Spectrum"}' --instance Genesis.ColorPalette

If Amazon returns a structured error (INVALID_VALUE,
ENDPOINT_UNREACHABLE, MISSING_FIELD, etc.) the message surfaces via
ErrCannotControlDevice — useful for discovering correct payload
field names when wiring new typed accessors.`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		var payload map[string]any
		if len(args) == 3 && args[2] != "" {
			if err := json.Unmarshal([]byte(args[2]), &payload); err != nil {
				return fmt.Errorf("invalid JSON payload %q: %w", args[2], err)
			}
		}

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}
		if err := client.Devices.Refresh(ctx); err != nil {
			return err
		}

		d, err := resolveDevice(ctx, sendDirectiveDevice, client, store)
		if err != nil {
			return err
		}
		if err := d.SendDirective(ctx, args[0], sendDirectiveInstance, args[1], payload); err != nil {
			return err
		}
		fmt.Printf("OK: %s/%s on %s\n", args[0], args[1], d.SerialNumber)
		return nil
	},
}

func init() {
	devicesSendDirectiveCmd.Flags().StringVar(&sendDirectiveInstance, "instance", "",
		"feature instance identifier (e.g. Pure.FanSpeed) for multi-instance controllers")
	devicesCmd.AddCommand(devicesSendDirectiveCmd)
	registerDeviceFlag(devicesSendDirectiveCmd, &sendDirectiveDevice)
}
