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
	var device string
	c := &cobra.Command{
		Use:   "call <name>",
		Short: "Trigger a routine by name on a device",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(c, &device)
	c.RunE = func(_ *cobra.Command, args []string) error {
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
		return client.Media.CallRoutine(ctx, dvc, args[0])
	}
	return c
}

func init() {
	rootCmd.AddCommand(newRoutinesCmd())
}
