package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// newSpeakCmd builds the `speak` subcommand with its flags scoped to a
// closure rather than package-level globals. Each invocation has fresh
// flag-bound variables, so concurrent or sequential test harness reuse
// can't leak state between runs.
func newSpeakCmd() *cobra.Command {
	var serial, text string
	cmd := &cobra.Command{
		Use:   "speak",
		Short: "Speak text on a device",
	}
	cmd.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	cmd.Flags().StringVar(&text, "text", "", "text to speak (required)")
	_ = cmd.MarkFlagRequired("serial")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
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
		return client.Media.Speak(ctx, dvc, text)
	}
	return cmd
}

// newAnnounceCmd builds the `announce` subcommand with closure-scoped flags.
func newAnnounceCmd() *cobra.Command {
	var serial, text string
	cmd := &cobra.Command{
		Use:   "announce",
		Short: "Announce a message on a device (cluster fanout)",
	}
	cmd.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	cmd.Flags().StringVar(&text, "text", "", "text to announce (required)")
	_ = cmd.MarkFlagRequired("serial")
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
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
		return client.Media.Announce(ctx, dvc, text)
	}
	return cmd
}

// newVolumeCmd builds the `volume` subcommand — value is a positional
// argument.
//
// Usage: amazondevices-test volume --serial S 42
func newVolumeCmd() *cobra.Command {
	var serial string
	cmd := &cobra.Command{
		Use:   "volume <0..100>",
		Short: "Set device volume",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	_ = cmd.MarkFlagRequired("serial")
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
		dvc, err := client.Devices.Get(ctx, serial)
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
