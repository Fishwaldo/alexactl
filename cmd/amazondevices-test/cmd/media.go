package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// newSpeakCmd builds the `speak` subcommand. The text to speak is a
// positional argument; the target device is the standard --device flag
// (serial or name, falling back to the configured default-device).
func newSpeakCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "speak <text>",
		Short: "Speak text on a device",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(cmd, &device)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.Speak(ctx, dvc, args[0])
	}
	return cmd
}

// newAnnounceCmd builds the `announce` subcommand. The message text is a
// positional argument; the target device is the standard --device flag.
func newAnnounceCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "announce <text>",
		Short: "Announce a message on a device (cluster fanout)",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(cmd, &device)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.Announce(ctx, dvc, args[0])
	}
	return cmd
}

// newVolumeCmd builds the `volume` subcommand — the level is a positional
// argument and the target device is the standard --device flag.
//
// Usage: amazondevices-test volume 42 --device "Bedroom"
func newVolumeCmd() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:   "volume <0..100>",
		Short: "Set device volume",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(cmd, &device)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		value, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("volume must be an integer in [0,100], got %q", args[0])
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
		return client.Media.SetVolume(ctx, dvc, value)
	}
	return cmd
}

func init() {
	rootCmd.AddCommand(newSpeakCmd())
	rootCmd.AddCommand(newAnnounceCmd())
	rootCmd.AddCommand(newVolumeCmd())
}
