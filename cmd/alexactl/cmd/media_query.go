// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

func newMusicProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "music-providers",
		Short: "List available music providers",
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
			providers, err := client.Media.MusicProviders(ctx)
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(providers))
			for id := range providers {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%-30s  %-25s  %-12s  %s\n",
				"ID", "NAME", "AVAILABILITY", "DEFAULT")
			for _, id := range ids {
				p := providers[id]
				_, _ = fmt.Fprintf(out, "%-30s  %-25s  %-12s  %v\n",
					p.ProviderID, p.ProviderName, p.Availability, p.DefaultProvider)
			}
			return nil
		},
	}
}

func newVolumeStateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "volume-state",
		Short: "Print per-device volume + mute state",
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
			volumes, err := client.Media.DeviceVolumes(ctx)
			if err != nil {
				return err
			}
			serials := make([]string, 0, len(volumes))
			for s := range volumes {
				serials = append(serials, s)
			}
			sort.Strings(serials)
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%-32s  %-7s  %s\n", "SERIAL", "VOLUME", "MUTED")
			for _, s := range serials {
				v := volumes[s]
				_, _ = fmt.Fprintf(out, "%-32s  %-7d  %v\n", s, v.Volume, v.Muted)
			}
			return nil
		},
	}
}

func newMediaStateCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "media-state",
		Short: "Print the current playback snapshot for one device",
		Args:  cobra.NoArgs,
	}
	registerDeviceFilterFlag(c, &device)
	c.RunE = func(cmd *cobra.Command, _ []string) error {
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
		filter, err := resolveFilter(ctx, device, client)
		if err != nil {
			return err
		}
		var devices []*alexactl.Device
		if filter == "" {
			devices, err = client.Devices.List(ctx)
		} else {
			var dvc *alexactl.Device
			dvc, err = client.Devices.Get(ctx, filter)
			devices = []*alexactl.Device{dvc}
		}
		if err != nil {
			return err
		}
		states, err := client.Media.MediaStates(ctx, devices)
		if err != nil {
			return err
		}
		serials := make([]string, 0, len(devices))
		for _, d := range devices {
			serials = append(serials, d.SerialNumber)
		}
		sort.Strings(serials)
		out := cmd.OutOrStdout()
		for i, serial := range serials {
			s, ok := states[serial]
			if i > 0 {
				_, _ = fmt.Fprintln(out)
			}
			_, _ = fmt.Fprintf(out, "device   : %s\n", serial)
			if !ok {
				_, _ = fmt.Fprintln(out, "(no media session for this device)")
				continue
			}
			_, _ = fmt.Fprintf(out, "state    : %s\n", s.PlayerState)
			_, _ = fmt.Fprintf(out, "title    : %s\n", s.NowPlayingTitle)
			_, _ = fmt.Fprintf(out, "line1    : %s\n", s.NowPlayingLine1)
			_, _ = fmt.Fprintf(out, "line2    : %s\n", s.NowPlayingLine2)
			_, _ = fmt.Fprintf(out, "provider : %s\n", s.MediaProvider)
			_, _ = fmt.Fprintf(out, "length   : %s\n", s.MediaLength)
			_, _ = fmt.Fprintf(out, "position : %s (as of %s)\n",
				s.MediaPosition, s.MediaPositionUpdated.Format("15:04:05"))
		}
		return nil
	}
	return c
}

func init() {
	rootCmd.AddCommand(newMusicProvidersCmd())
	rootCmd.AddCommand(newVolumeStateCmd())
	rootCmd.AddCommand(newMediaStateCmd())
}
