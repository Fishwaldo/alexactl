package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var (
	thermoTargetDevice string
	thermoModeDevice   string
)

var devicesThermostatCmd = &cobra.Command{
	Use:   "thermostat <subcommand>",
	Short: "Control a thermostat device (set-target | set-mode)",
	Long: `Parent command for thermostat directives. Run with a subcommand
('set-target' or 'set-mode'). See the subcommand --help for details.`,
}

var devicesThermostatSetTargetCmd = &cobra.Command{
	Use:   "set-target <value> [c|f]",
	Short: "Set a thermostat's target setpoint",
	Long: `Issues a setTargetSetpoint directive. Scale defaults to celsius
('c'); pass 'f' for fahrenheit. Amazon rejects values outside the
device's supported range with INVALID_VALUE.

The target device is the --device flag (serial or name) or the configured
default-device.

Example:
  alexactl devices thermostat set-target 21.5 c --device "Hallway"
  alexactl devices thermostat set-target 72 f`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		value, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			return fmt.Errorf("invalid value %q: %w", args[0], err)
		}
		scale := alexactl.TemperatureScaleCelsius
		if len(args) == 2 {
			scale, err = parseScaleArg(args[1])
			if err != nil {
				return err
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

		d, err := resolveDevice(ctx, thermoTargetDevice, client, store)
		if err != nil {
			return err
		}
		th := d.Thermostat()
		if th == nil {
			return fmt.Errorf("device %q does not expose Alexa.ThermostatController", d.SerialNumber)
		}
		return th.SetTarget(ctx, value, scale)
	},
}

var devicesThermostatSetModeCmd = &cobra.Command{
	Use:   "set-mode <heat|cool|auto|off|eco|em_heat>",
	Short: "Set a thermostat's operating mode",
	Long: `Issues a setThermostatMode directive. Mode is case-insensitive.
Per-device support varies — query the device's
Sensors["thermostatMode"].Configuration["supportedModes"] (via
'devices inspect') to confirm what each device accepts before
calling.

The target device is the --device flag (serial or name) or the configured
default-device.

Examples:
  alexactl devices thermostat set-mode cool --device "Hallway"
  alexactl devices thermostat set-mode heat`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		mode, err := alexactl.ParseThermostatMode(args[0])
		if err != nil {
			return fmt.Errorf("invalid mode %q: %w", args[0], err)
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

		d, err := resolveDevice(ctx, thermoModeDevice, client, store)
		if err != nil {
			return err
		}
		th := d.Thermostat()
		if th == nil {
			return fmt.Errorf("device %q does not expose Alexa.ThermostatController", d.SerialNumber)
		}
		return th.SetMode(ctx, mode)
	},
}

// parseScaleArg accepts 'c' / 'C' / 'celsius' or 'f' / 'F' /
// 'fahrenheit'. Returns TemperatureScaleCelsius by default; errors on
// anything else.
func parseScaleArg(s string) (alexactl.TemperatureScale, error) {
	switch strings.ToLower(s) {
	case "c", "celsius":
		return alexactl.TemperatureScaleCelsius, nil
	case "f", "fahrenheit":
		return alexactl.TemperatureScaleFahrenheit, nil
	case "k", "kelvin":
		return alexactl.TemperatureScaleKelvin, nil
	default:
		return "", fmt.Errorf("invalid scale %q: want c|f|k", s)
	}
}

func init() {
	devicesThermostatCmd.AddCommand(devicesThermostatSetTargetCmd)
	devicesThermostatCmd.AddCommand(devicesThermostatSetModeCmd)
	devicesCmd.AddCommand(devicesThermostatCmd)
	registerDeviceFlag(devicesThermostatSetTargetCmd, &thermoTargetDevice)
	registerDeviceFlag(devicesThermostatSetModeCmd, &thermoModeDevice)
}
