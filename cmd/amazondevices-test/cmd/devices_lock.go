package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var devicesLockCmd = &cobra.Command{
	Use:   "lock <serial>",
	Short: "Lock a smart lock",
	Long: `Issues an Alexa.LockController lock directive against the given
device. The device must advertise Alexa.LockController in its
AlexaInterfaces (Phase 4a populates this from the discovery query) and
have a non-empty EndpointID. Run 'devices list' first to confirm.

Example:
  amazondevices-test devices lock AB1234567890`,
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

		d, err := client.Devices.Get(ctx, args[0])
		if err != nil {
			return err
		}
		l := d.Lock()
		if l == nil {
			return fmt.Errorf("device %q does not expose Alexa.LockController", d.SerialNumber)
		}
		return l.Lock(ctx)
	},
}

func init() {
	devicesCmd.AddCommand(devicesLockCmd)
}
