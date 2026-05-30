// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var colorTempDevice string

var devicesColorTempCmd = &cobra.Command{
	Use:   "color-temp <kelvin>",
	Short: "Set a color-temperature device's white-point in kelvin",
	Long: `Issues an Alexa.ColorTemperatureController setColorTemperature
directive against the given device. Kelvin is not clamped — Amazon
returns INVALID_VALUE if the device can't reach the requested
temperature, surfaced as a clear error.

Typical ranges:
  candle       1800K
  incandescent 2700K
  warm white   3000K
  daylight     5000K
  cool white   6500K

Example:
  alexactl devices color-temp 2700 --device "Bedroom"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		kelvin, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid kelvin %q: %w", args[0], err)
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

		d, err := resolveDevice(ctx, colorTempDevice, client, store)
		if err != nil {
			return err
		}
		ct := d.ColorTemperature()
		if ct == nil {
			return fmt.Errorf("device %q does not expose Alexa.ColorTemperatureController", d.SerialNumber)
		}
		return ct.Set(ctx, kelvin)
	},
}

func init() {
	devicesCmd.AddCommand(devicesColorTempCmd)
	registerDeviceFlag(devicesColorTempCmd, &colorTempDevice)
}
