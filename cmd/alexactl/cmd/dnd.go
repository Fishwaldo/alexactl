// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var dndCmd = &cobra.Command{
	Use:   "dnd",
	Short: "Do Not Disturb commands",
}

func newDnDGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "List the Do Not Disturb state of every device",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signalContext()
			defer cancel()

			client, sess, err := newClient(true)
			if err != nil {
				return err
			}
			if sess == nil {
				return fmt.Errorf("no saved session — run `login` first")
			}
			if _, err := client.Auth.Restore(ctx); err != nil {
				return err
			}

			status, err := client.DnD.Status(ctx)
			if err != nil {
				return err
			}
			serials := make([]string, 0, len(status))
			for s := range status {
				serials = append(serials, s)
			}
			sort.Strings(serials)

			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%-32s  %s\n", "SERIAL", "DND")
			for _, s := range serials {
				_, _ = fmt.Fprintf(out, "%-32s  %v\n", s, status[s].Value)
			}
			return nil
		},
	}
}

func newDnDSetCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "set <on|off>",
		Short: "Toggle Do Not Disturb on one device",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(cmd, &device)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		enable, err := parseOnOff(args[0])
		if err != nil {
			return err
		}
		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session — run `login` first")
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.DnD.Set(ctx, dvc, enable)
	}
	return cmd
}

// parseOnOff maps "on"/"off" (case-insensitive) to a boolean.
func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	}
	return false, fmt.Errorf("expected \"on\" or \"off\", got %q", s)
}

func init() {
	dndCmd.AddCommand(newDnDGetCmd())
	dndCmd.AddCommand(newDnDSetCmd())
	rootCmd.AddCommand(dndCmd)
}
