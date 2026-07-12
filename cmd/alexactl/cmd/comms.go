// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var commsCmd = &cobra.Command{
	Use:   "comms",
	Short: "Read/set communications, announcement, and drop-in preferences",
}

func newCommsPrefsCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "prefs",
		Short: "Show the allowed comms preferences for one device",
		RunE: func(_ *cobra.Command, _ []string) error {
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
			dvc, err := resolveDevice(ctx, device, client, store)
			if err != nil {
				return err
			}

			prefs, err := client.Communications.GetPreferences(ctx, []*alexactl.Device{dvc})
			if err != nil {
				return err
			}
			p, ok := prefs[dvc.SerialNumber]
			if !ok {
				_, _ = fmt.Fprintf(os.Stdout, "no allowed comms preferences for %s (%s)\n",
					dvc.AccountName, dvc.SerialNumber)
				return nil
			}
			names := make([]string, 0, len(p))
			for k := range p {
				names = append(names, k)
			}
			sort.Strings(names)
			_, _ = fmt.Fprintf(os.Stdout, "%-20s  %s\n", "PREFERENCE", "STATE")
			for _, n := range names {
				_, _ = fmt.Fprintf(os.Stdout, "%-20s  %s\n", n, p[n])
			}
			return nil
		},
	}
	registerDeviceFlag(cmd, &device)
	return cmd
}

func newCommsSetCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "set <communications|announcements|dropin> <value>",
		Short: "Set a comms preference on one device",
		Long: `Sets one of the three comms preferences on a device:

  communications  on|off
  announcements   on|off
  dropin          All|Home|Off

Examples:
  alexactl comms set announcements on --device "Kitchen"
  alexactl comms set dropin Home --device "Kitchen"`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
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
			dvc, err := resolveDevice(ctx, device, client, store)
			if err != nil {
				return err
			}

			pref := strings.ToLower(args[0])
			value := args[1]
			switch pref {
			case "communications":
				on, err := parseOnOff(value)
				if err != nil {
					return err
				}
				return client.Communications.SetCommunicationStatus(ctx, dvc, on)
			case "announcements":
				on, err := parseOnOff(value)
				if err != nil {
					return err
				}
				return client.Communications.SetAnnouncementStatus(ctx, dvc, on)
			case "dropin":
				status, err := alexactl.ParseDropInStatus(value)
				if err != nil {
					return fmt.Errorf("invalid dropin value %q: %w", value, err)
				}
				return client.Communications.SetDropInStatus(ctx, dvc, status)
			default:
				return fmt.Errorf("unknown preference %q: use communications, announcements, or dropin", args[0])
			}
		},
	}
	registerDeviceFlag(cmd, &device)
	return cmd
}

func init() {
	commsCmd.AddCommand(newCommsPrefsCmd())
	commsCmd.AddCommand(newCommsSetCmd())
	rootCmd.AddCommand(commsCmd)
}
