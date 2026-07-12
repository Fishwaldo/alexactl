// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var restartDevice string

var devicesRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Reboot a device",
	Long: `Reboots the target device via Amazon's reboot endpoint. The
device drops offline for roughly 30 seconds and then reconnects on its
own; this command does not wait for it to come back.

Requires a saved OAuth access token (the same session established by
'login').

Examples:
  alexactl devices restart --device "Kitchen"
  alexactl devices restart --device AB1234567890`,
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

		d, err := resolveDevice(ctx, restartDevice, client, store)
		if err != nil {
			return err
		}

		if err := client.Devices.Restart(ctx, d); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(os.Stdout, "restart requested for %s (%s); it will reconnect shortly\n",
			d.AccountName, d.SerialNumber)
		return nil
	},
}

func init() {
	devicesCmd.AddCommand(devicesRestartCmd)
	registerDeviceFlag(devicesRestartCmd, &restartDevice)
}
