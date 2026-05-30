package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRoutinesCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "routines",
		Short: "List and trigger Alexa routines",
	}
	c.AddCommand(newRoutinesListCmd())
	c.AddCommand(newRoutinesCallCmd())
	return c
}

func newRoutinesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List enabled routines on the account",
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
			names, err := client.Sequence.Routines(ctx)
			if err != nil {
				return err
			}
			for _, n := range names {
				_, _ = fmt.Fprintln(os.Stdout, n)
			}
			return nil
		},
	}
}

func newRoutinesCallCmd() *cobra.Command {
	var serial, name string
	c := &cobra.Command{
		Use:   "call",
		Short: "Trigger a routine by name on a device",
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	c.Flags().StringVar(&name, "name", "", "routine name (required)")
	_ = c.MarkFlagRequired("serial")
	_ = c.MarkFlagRequired("name")
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
		return client.Media.CallRoutine(ctx, dvc, name)
	}
	return c
}

func init() {
	rootCmd.AddCommand(newRoutinesCmd())
}
