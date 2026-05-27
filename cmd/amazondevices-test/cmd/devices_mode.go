package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var devicesModeCmd = &cobra.Command{
	Use:   "mode <serial> <instance> <subcommand>",
	Short: "Control a generic Alexa.ModeController feature (set | adjust)",
	Long: `Parent command for ModeController directives. Run with a
subcommand ('set' or 'adjust'). See the subcommand --help for details.

A single device can expose multiple Mode instances (e.g. a Sensibo
air-conditioner has Apparatus.FanSpeed AND Apparatus.FanDirection).
The instance argument selects which one. List available instances
+ supported values via:

  amazondevices-test devices inspect <serial> | jq '.Sensors |
      to_entries | map(select(.value.Configuration._type ==
      "ModeConfiguration"))'`,
}

var devicesModeSetCmd = &cobra.Command{
	Use:   "set <serial> <instance> <value>",
	Short: "Set a Mode instance to the given value",
	Long: `Issues a setMode directive. The value must be one of the
strings advertised in the device's ModeConfiguration.modeOptions
(see Sensors[<key>].Configuration["modeOptions"] from 'devices
inspect'); invalid values are rejected by Amazon with INVALID_VALUE
which surfaces as ErrCannotControlDevice.

Examples:
  amazondevices-test devices mode set 54fd2f18-... Pure.FanSpeed FanSpeed.High
  amazondevices-test devices mode set ee61722d-... Genesis.ColorPalette Spectrum
  amazondevices-test devices mode set 32f10fc0-... Apparatus.FanSpeed L3`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

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
		m := d.Mode(args[1])
		if m == nil {
			return fmt.Errorf("device %q does not expose Alexa.ModeController", d.SerialNumber)
		}
		return m.Set(ctx, args[2])
	},
}

var devicesModeAdjustCmd = &cobra.Command{
	Use:   "adjust <serial> <instance> <delta>",
	Short: "Step a Mode instance by an integer delta",
	Long: `Issues an adjustMode directive with the given integer delta
(positive or negative). The device decides how to interpret the
step. No optimistic update — the resulting absolute value depends
on the device's current state.

Example:
  amazondevices-test devices mode adjust 54fd2f18-... Pure.FanSpeed 1
  amazondevices-test devices mode adjust 32f10fc0-... Apparatus.FanSpeed -2`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		delta, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid delta %q: %w", args[2], err)
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
		m := d.Mode(args[1])
		if m == nil {
			return fmt.Errorf("device %q does not expose Alexa.ModeController", d.SerialNumber)
		}
		return m.Adjust(ctx, delta)
	},
}

func init() {
	devicesModeCmd.AddCommand(devicesModeSetCmd)
	devicesModeCmd.AddCommand(devicesModeAdjustCmd)
	devicesCmd.AddCommand(devicesModeCmd)
}
