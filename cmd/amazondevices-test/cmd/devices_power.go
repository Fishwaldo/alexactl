package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

var devicesPowerCmd = &cobra.Command{
	Use:   "power <serial> <on|off|toggle>",
	Short: "Turn a smart-home device on/off or toggle its current state",
	Long: `Issues an Alexa.PowerController directive against the given
device. The device must advertise Alexa.PowerController in its
AlexaInterfaces (Phase 4a populates this from the discovery query) and
have a non-empty EndpointID. Run 'devices list' first to confirm.

Examples:
  amazondevices-test devices power AB1234567890 on
  amazondevices-test devices power AB1234567890 off
  amazondevices-test devices power AB1234567890 toggle`,
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

		d, err := client.Devices.Get(ctx, args[0])
		if err != nil {
			return err
		}
		power := d.Power()
		if power == nil {
			return fmt.Errorf("device %q (kind=%s) does not expose Alexa.PowerController",
				d.SerialNumber, amazondevices.Kind(d.Kind))
		}

		switch args[1] {
		case "on":
			return power.On(ctx)
		case "off":
			return power.Off(ctx)
		case "toggle":
			return power.Toggle(ctx)
		default:
			return fmt.Errorf("unknown action %q: use on, off, or toggle", args[1])
		}
	},
}

func init() {
	devicesCmd.AddCommand(devicesPowerCmd)
}
