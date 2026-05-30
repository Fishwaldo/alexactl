package cmd

import (
	"fmt"
	"os"
	"sort"

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

			status, err := client.DnD.Status(ctx)
			if err != nil {
				return err
			}
			serials := make([]string, 0, len(status))
			for s := range status {
				serials = append(serials, s)
			}
			sort.Strings(serials)

			_, _ = fmt.Fprintf(os.Stdout, "%-32s  %s\n", "SERIAL", "DND")
			for _, s := range serials {
				_, _ = fmt.Fprintf(os.Stdout, "%-32s  %v\n", s, status[s].Value)
			}
			return nil
		},
	}
}

func newDnDSetCmd() *cobra.Command {
	var serial string
	var enable bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Toggle Do Not Disturb on one device",
	}
	cmd.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	cmd.Flags().BoolVar(&enable, "enable", false, "true to enable DnD; false to disable")
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
		return client.DnD.Set(ctx, dvc, enable)
	}
	return cmd
}

func init() {
	dndCmd.AddCommand(newDnDGetCmd())
	dndCmd.AddCommand(newDnDSetCmd())
	rootCmd.AddCommand(dndCmd)
}
