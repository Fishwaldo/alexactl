package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var powerDevice string

var devicesPowerCmd = &cobra.Command{
	Use:   "power <on|off|toggle>",
	Short: "Turn a smart-home device on/off or toggle its current state",
	Long: `Issues an Alexa.PowerController directive against the given
device. The device must advertise Alexa.PowerController in its
AlexaInterfaces (Phase 4a populates this from the discovery query) and
have a non-empty EndpointID. Run 'devices list' first to confirm.

Examples:
  alexactl devices power on --device "Bedroom"
  alexactl devices power off --device AB1234567890
  alexactl devices power toggle`,
	Args: cobra.ExactArgs(1),
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

		d, err := resolveDevice(ctx, powerDevice, client, store)
		if err != nil {
			return err
		}
		power := d.Power()
		if power == nil {
			return fmt.Errorf("device %q (kind=%s) does not expose Alexa.PowerController",
				d.SerialNumber, alexactl.Kind(d.Kind))
		}

		switch args[0] {
		case "on":
			return power.On(ctx)
		case "off":
			return power.Off(ctx)
		case "toggle":
			return power.Toggle(ctx)
		default:
			return fmt.Errorf("unknown action %q: use on, off, or toggle", args[0])
		}
	},
}

func init() {
	devicesCmd.AddCommand(devicesPowerCmd)
	registerDeviceFlag(devicesPowerCmd, &powerDevice)
}
