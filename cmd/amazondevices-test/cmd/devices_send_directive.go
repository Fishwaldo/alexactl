package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	sendDirectiveInstance string
)

var devicesSendDirectiveCmd = &cobra.Command{
	Use:   "send-directive <serial> <featureName> <operationName> [json-payload]",
	Short: "Send an arbitrary smart-home directive via the V2 mutation (escape hatch)",
	Long: `Issues any setEndpointFeaturesV2 directive against the named device.
Mirrors Device.SendDirective for Phase 4e exploratory testing — call
any featureName / operationName / payload to see what Amazon accepts.

Use --instance for features that support multiple instances per
device (Mode, Range, Toggle controllers). For singleton features
(Power, Brightness, Color, ColorTemperature, Thermostat, Lock) you
can leave it off.

Examples (singleton features):
  devices send-directive AB1234... power "" turnOn
  devices send-directive AB1234... brightness "" setBrightness '{"brightness": 60}'

Examples (mode features with instance):
  devices send-directive 54fd2f18-... mode setMode \
      '{"mode":"FanSpeed.High"}' --instance Pure.FanSpeed
  devices send-directive ee61722d-... mode setMode \
      '{"mode":"Spectrum"}' --instance Genesis.ColorPalette

If Amazon returns a structured error (INVALID_VALUE,
ENDPOINT_UNREACHABLE, MISSING_FIELD, etc.) the message surfaces via
ErrCannotControlDevice — useful for discovering correct payload
field names when wiring new typed accessors.`,
	Args: cobra.RangeArgs(3, 4),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		var payload map[string]any
		if len(args) == 4 && args[3] != "" {
			if err := json.Unmarshal([]byte(args[3]), &payload); err != nil {
				return fmt.Errorf("invalid JSON payload %q: %w", args[3], err)
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

		d, err := client.Devices.Get(ctx, args[0])
		if err != nil {
			return err
		}
		if err := d.SendDirective(ctx, args[1], sendDirectiveInstance, args[2], payload); err != nil {
			return err
		}
		fmt.Printf("OK: %s/%s on %s\n", args[1], args[2], d.SerialNumber)
		return nil
	},
}

func init() {
	devicesSendDirectiveCmd.Flags().StringVar(&sendDirectiveInstance, "instance", "",
		"feature instance identifier (e.g. Pure.FanSpeed) for multi-instance controllers")
	devicesCmd.AddCommand(devicesSendDirectiveCmd)
}
