package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

func newMusicProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "music-providers",
		Short: "List available music providers",
		RunE: func(_ *cobra.Command, _ []string) error {
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
			fmt.Fprintf(os.Stdout, "%-30s  %-25s  %-12s  %s\n",
				"ID", "NAME", "AVAILABILITY", "DEFAULT")
			for _, id := range ids {
				p := providers[id]
				fmt.Fprintf(os.Stdout, "%-30s  %-25s  %-12s  %v\n",
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
		RunE: func(_ *cobra.Command, _ []string) error {
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
			fmt.Fprintf(os.Stdout, "%-32s  %-7s  %s\n", "SERIAL", "VOLUME", "MUTED")
			for _, s := range serials {
				v := volumes[s]
				fmt.Fprintf(os.Stdout, "%-32s  %-7d  %v\n", s, v.Volume, v.Muted)
			}
			return nil
		},
	}
}

func newMediaStateCmd() *cobra.Command {
	var serial string
	c := &cobra.Command{
		Use:   "media-state",
		Short: "Print the current playback snapshot for one device",
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	_ = c.MarkFlagRequired("serial")
	c.RunE = func(_ *cobra.Command, _ []string) error {
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		states, err := client.Media.MediaStates(ctx, []*amazondevices.Device{dvc})
		if err != nil {
			return err
		}
		s, ok := states[serial]
		if !ok {
			fmt.Fprintln(os.Stdout, "(no media session for this device)")
			return nil
		}
		fmt.Fprintf(os.Stdout, "state    : %s\n", s.PlayerState)
		fmt.Fprintf(os.Stdout, "title    : %s\n", s.NowPlayingTitle)
		fmt.Fprintf(os.Stdout, "line1    : %s\n", s.NowPlayingLine1)
		fmt.Fprintf(os.Stdout, "line2    : %s\n", s.NowPlayingLine2)
		fmt.Fprintf(os.Stdout, "provider : %s\n", s.MediaProvider)
		fmt.Fprintf(os.Stdout, "length   : %s\n", s.MediaLength)
		fmt.Fprintf(os.Stdout, "position : %s (as of %s)\n",
			s.MediaPosition, s.MediaPositionUpdated.Format("15:04:05"))
		return nil
	}
	return c
}

func init() {
	rootCmd.AddCommand(newMusicProvidersCmd())
	rootCmd.AddCommand(newVolumeStateCmd())
	rootCmd.AddCommand(newMediaStateCmd())
}
