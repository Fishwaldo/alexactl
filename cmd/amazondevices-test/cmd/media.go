package cmd

import (
	"fmt"

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

// newVolumeCmd builds the `volume` subcommand with closure-scoped flags.
func newVolumeCmd() *cobra.Command {
	var serial string
	var value int
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Set device volume (0..100)",
	}
	cmd.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	cmd.Flags().IntVar(&value, "value", 50, "volume value (0..100)")
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
		return client.Media.SetVolume(ctx, dvc, value)
	}
	return cmd
}

func init() {
	rootCmd.AddCommand(newSpeakCmd())
	rootCmd.AddCommand(newAnnounceCmd())
	rootCmd.AddCommand(newVolumeCmd())
}
