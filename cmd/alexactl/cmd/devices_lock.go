// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var lockDevice string

var devicesLockCmd = &cobra.Command{
	Use:   "lock <lock|unlock>",
	Short: "Lock or unlock a smart lock",
	Long: `Issues an Alexa.LockController directive against the given device.
The device must advertise Alexa.LockController in its AlexaInterfaces
(the discovery query populates this) and have a non-empty EndpointID.
Run 'devices list' first to confirm.

Many locks disable remote unlock by policy (Yale's "remote unlock
disabled" setting, August's "owner-only" policy, BHMA/Z-Wave
"auto-relock only" modes). When the device refuses an unlock, Amazon
returns a structured error which surfaces as ErrCannotControlDevice
with a clear message.

Examples:
  alexactl devices lock lock --device "Front Door"
  alexactl devices lock unlock --device "Front Door"`,
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

		d, err := resolveDevice(ctx, lockDevice, client, store)
		if err != nil {
			return err
		}
		l := d.Lock()
		if l == nil {
			return fmt.Errorf("device %q does not expose Alexa.LockController", d.SerialNumber)
		}
		switch args[0] {
		case "lock":
			return l.Lock(ctx)
		case "unlock":
			return l.Unlock(ctx)
		default:
			return fmt.Errorf("unknown action %q: use lock or unlock", args[0])
		}
	},
}

func init() {
	devicesCmd.AddCommand(devicesLockCmd)
	registerDeviceFlag(devicesLockCmd, &lockDevice)
}
