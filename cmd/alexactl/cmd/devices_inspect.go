// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var inspectDevice string

var devicesInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Fetch the full per-endpoint feature set for one device",
	Args:  cobra.NoArgs,
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

		target, err := resolveDevice(ctx, inspectDevice, client, store)
		if err != nil {
			return err
		}
		d, err := client.Devices.Inspect(ctx, target.SerialNumber)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	},
}

func init() {
	devicesCmd.AddCommand(devicesInspectCmd)
	registerDeviceFlag(devicesInspectCmd, &inspectDevice)
}
