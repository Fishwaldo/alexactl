// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	modeSetDevice    string
	modeAdjustDevice string
)

var devicesModeCmd = &cobra.Command{
	Use:   "mode <instance> <subcommand>",
	Short: "Control a generic Alexa.ModeController feature (set | adjust)",
	Long: `Parent command for ModeController directives. Run with a
subcommand ('set' or 'adjust'). See the subcommand --help for details.

A single device can expose multiple Mode instances (e.g. a Sensibo
air-conditioner has Apparatus.FanSpeed AND Apparatus.FanDirection).
The instance argument selects which one. List available instances
+ supported values via:

  alexactl devices inspect --device <serial-or-name> | jq '.Sensors |
      to_entries | map(select(.value.Configuration._type ==
      "ModeConfiguration"))'`,
}

var devicesModeSetCmd = &cobra.Command{
	Use:   "set <instance> <value>",
	Short: "Set a Mode instance to the given value",
	Long: `Issues a setMode directive. The value must be one of the
strings advertised in the device's ModeConfiguration.modeOptions
(see Sensors[<key>].Configuration["modeOptions"] from 'devices
inspect'); invalid values are rejected by Amazon with INVALID_VALUE
which surfaces as ErrCannotControlDevice.

The target device is the --device flag (serial or name) or the configured
default-device.

Examples:
  alexactl devices mode set Pure.FanSpeed FanSpeed.High --device "Air Purifier"
  alexactl devices mode set Genesis.ColorPalette Spectrum
  alexactl devices mode set Apparatus.FanSpeed L3`,
	Args: cobra.ExactArgs(2),
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

		d, err := resolveDevice(ctx, modeSetDevice, client, store)
		if err != nil {
			return err
		}
		m := d.Mode(args[0])
		if m == nil {
			return fmt.Errorf("device %q does not expose Alexa.ModeController", d.SerialNumber)
		}
		return m.Set(ctx, args[1])
	},
}

var devicesModeAdjustCmd = &cobra.Command{
	Use:   "adjust <instance> <delta>",
	Short: "Step a Mode instance by an integer delta",
	Long: `Issues an adjustMode directive with the given integer delta
(positive or negative). The device decides how to interpret the
step. No optimistic update — the resulting absolute value depends
on the device's current state.

The target device is the --device flag (serial or name) or the configured
default-device.

Example:
  alexactl devices mode adjust Pure.FanSpeed 1 --device "Air Purifier"
  alexactl devices mode adjust Apparatus.FanSpeed -2`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		delta, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid delta %q: %w", args[1], err)
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

		d, err := resolveDevice(ctx, modeAdjustDevice, client, store)
		if err != nil {
			return err
		}
		m := d.Mode(args[0])
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
	registerDeviceFlag(devicesModeSetCmd, &modeSetDevice)
	registerDeviceFlag(devicesModeAdjustCmd, &modeAdjustDevice)
}
