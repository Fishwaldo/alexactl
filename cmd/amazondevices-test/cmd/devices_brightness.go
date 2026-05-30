package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var brightnessDevice string

var devicesBrightnessCmd = &cobra.Command{
	Use:   "brightness <percent>",
	Short: "Set a dimmable device's brightness to <percent> (0..100)",
	Long: `Issues an Alexa.BrightnessController setBrightness directive
against the given device. Percent is clamped to [0,100] before sending.

Example:
  amazondevices-test devices brightness 75 --device "Bedroom"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		pct, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid percent %q: %w", args[0], err)
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

		d, err := resolveDevice(ctx, brightnessDevice, client, store)
		if err != nil {
			return err
		}
		brightness := d.Brightness()
		if brightness == nil {
			return fmt.Errorf("device %q does not expose Alexa.BrightnessController", d.SerialNumber)
		}
		return brightness.Set(ctx, pct)
	},
}

func init() {
	devicesCmd.AddCommand(devicesBrightnessCmd)
	registerDeviceFlag(devicesBrightnessCmd, &brightnessDevice)
}
