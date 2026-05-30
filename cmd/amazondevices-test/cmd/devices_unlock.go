package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var unlockDevice string

var devicesUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock a smart lock",
	Long: `Issues an Alexa.LockController unlock directive against the
given device. Many locks disable remote unlock by policy (Yale's
"remote unlock disabled" setting, August's "owner-only" policy,
BHMA/Z-Wave "auto-relock only" modes). When the device refuses,
Amazon returns a structured error which surfaces as
ErrCannotControlDevice with a clear message.

Example:
  amazondevices-test devices unlock --device "Front Door"`,
	Args: cobra.NoArgs,
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

		d, err := resolveDevice(ctx, unlockDevice, client, store)
		if err != nil {
			return err
		}
		l := d.Lock()
		if l == nil {
			return fmt.Errorf("device %q does not expose Alexa.LockController", d.SerialNumber)
		}
		return l.Unlock(ctx)
	},
}

func init() {
	devicesCmd.AddCommand(devicesUnlockCmd)
	registerDeviceFlag(devicesUnlockCmd, &unlockDevice)
}
