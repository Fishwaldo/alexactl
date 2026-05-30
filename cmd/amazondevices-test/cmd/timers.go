package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

var (
	timersDeviceFlag   string
	timersLsDeviceFlag string
	timersLabelFlag    string
	timersSoundFlag    string
)

var timersCmd = &cobra.Command{
	Use:   "timers",
	Short: "List, create, and delete countdown timers on Echo devices",
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
		timers, err := client.Timers.Get(ctx)
		if err != nil {
			return err
		}
		byDevice := map[string][]*amazondevices.Timer{}
		for _, t := range timers {
			byDevice[t.DeviceSerialNumber] = append(byDevice[t.DeviceSerialNumber], t)
		}
		for serial, list := range byDevice {
			fmt.Printf("%s — %d active timer(s)\n", serial, len(list))
			for _, t := range list {
				fmt.Printf("  %s  %s  %s  fires at %s\n",
					t.ID, t.Duration, t.Label, t.FireAt.Format(time.RFC3339))
			}
		}
		return nil
	},
}

var timersLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Detailed timer list",
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
		filter, err := resolveFilter(ctx, timersLsDeviceFlag, client)
		if err != nil {
			return err
		}
		timers, err := client.Timers.Get(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-60s %-15s %-20s %s\n", "ID", "DURATION", "FIRE_AT", "LABEL")
		for _, t := range timers {
			if filter != "" && t.DeviceSerialNumber != filter {
				continue
			}
			fmt.Printf("%-60s %-15s %-20s %s\n",
				t.ID, t.Duration, t.FireAt.Format("2006-01-02 15:04"), t.Label)
		}
		return nil
	},
}

var timersCreateCmd = &cobra.Command{
	Use:   "create <duration>",
	Short: "Create a new timer",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dur, err := time.ParseDuration(args[0])
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", args[0], err)
		}
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
		if err := client.Devices.Refresh(ctx); err != nil {
			return err
		}
		dvc, err := resolveDevice(ctx, timersDeviceFlag, client, store)
		if err != nil {
			return err
		}
		t := &amazondevices.Timer{
			DeviceSerialNumber: dvc.SerialNumber,
			Duration:           dur,
			Label:              timersLabelFlag,
			Sound:              timersSoundFlag,
		}
		created, err := client.Timers.Create(ctx, t)
		if err != nil {
			return err
		}
		fmt.Printf("created: %s  (id=%s, fires in %s at %s)\n",
			created.Label, created.ID, created.Duration, created.FireAt.Format(time.RFC3339))
		return nil
	},
}

var timersDeleteCmd = &cobra.Command{
	Use:   "delete <timer-id>",
	Short: "Cancel a timer",
	Args:  cobra.ExactArgs(1),
	RunE: withTimer(func(ctx context.Context, t *amazondevices.Timer, client *amazondevices.Client, _ []string) error {
		return client.Timers.Delete(ctx, t)
	}),
}

var timersShowCmd = &cobra.Command{
	Use:   "show <timer-id>",
	Short: "Print the raw server-shape map for a timer as pretty JSON",
	Args:  cobra.ExactArgs(1),
	RunE: withTimer(func(_ context.Context, t *amazondevices.Timer, _ *amazondevices.Client, _ []string) error {
		raw := amazondevices.TimerServerShape(t)
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}),
}

func withTimer(action func(ctx context.Context, t *amazondevices.Timer, client *amazondevices.Client, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
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
		if err := client.Devices.Refresh(ctx); err != nil {
			return err
		}
		timers, err := client.Timers.Get(ctx)
		if err != nil {
			return err
		}
		id := args[0]
		for _, t := range timers {
			if t.ID == id {
				return action(ctx, t, client, args)
			}
		}
		return fmt.Errorf("timer id %q not found", id)
	}
}

func init() {
	registerDeviceFlag(timersCreateCmd, &timersDeviceFlag)
	timersCreateCmd.Flags().StringVar(&timersLabelFlag, "label", "", "optional label")
	timersCreateCmd.Flags().StringVar(&timersSoundFlag, "sound", "", "optional sound id")

	timersLsCmd.Flags().StringVar(&timersLsDeviceFlag, "device", "", "filter to one device (serial or name); omit for all devices")

	timersCmd.AddCommand(timersLsCmd)
	timersCmd.AddCommand(timersCreateCmd)
	timersCmd.AddCommand(timersDeleteCmd)
	timersCmd.AddCommand(timersShowCmd)
	rootCmd.AddCommand(timersCmd)
}
