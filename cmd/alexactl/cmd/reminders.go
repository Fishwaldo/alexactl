// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var (
	remindersDeviceFlag     string
	remindersLsDeviceFlag   string
	remindersRecurrenceFlag string
)

var remindersCmd = &cobra.Command{
	Use:   "reminders",
	Short: "List, create, and modify reminders on Echo devices",
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
		reminders, err := client.Reminders.Get(ctx)
		if err != nil {
			return err
		}
		byDevice := map[string][]*alexactl.Reminder{}
		for _, r := range reminders {
			byDevice[r.DeviceSerialNumber] = append(byDevice[r.DeviceSerialNumber], r)
		}
		for serial, list := range byDevice {
			fmt.Printf("%s — %d active reminder(s)\n", serial, len(list))
			for _, r := range list {
				rec := "(one-shot)"
				if r.Recurrence != nil {
					rec = r.Recurrence.Describe()
				}
				ack := ""
				if r.Acknowledged {
					ack = "  [ACKNOWLEDGED]"
				}
				fmt.Printf("  %s  %s  %s  [%s]%s\n",
					r.ID, r.FireAt.Format(time.RFC3339), r.Label, rec, ack)
			}
		}
		return nil
	},
}

var remindersLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Detailed reminder list",
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
		filter, err := resolveFilter(ctx, remindersLsDeviceFlag, client)
		if err != nil {
			return err
		}
		reminders, err := client.Reminders.Get(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-60s %-20s %-30s %s\n", "ID", "FIRE_AT", "LABEL", "RECURRENCE")
		for _, r := range reminders {
			if filter != "" && r.DeviceSerialNumber != filter {
				continue
			}
			rec := "(one-shot)"
			if r.Recurrence != nil {
				rec = r.Recurrence.Describe()
			}
			fmt.Printf("%-60s %-20s %-30s %s\n",
				r.ID, r.FireAt.Format("2006-01-02 15:04"), r.Label, rec)
		}
		return nil
	},
}

var remindersCreateCmd = &cobra.Command{
	Use:   "create <at> <label>",
	Short: "Create a new reminder",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fireAt, err := parseAtFlag(args[0])
		if err != nil {
			return err
		}
		label := args[1]
		var recur *alexactl.Recurrence
		if remindersRecurrenceFlag != "" {
			recur, err = parseRecurrenceFlag(remindersRecurrenceFlag)
			if err != nil {
				return err
			}
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
		dvc, err := resolveDevice(ctx, remindersDeviceFlag, client, store)
		if err != nil {
			return err
		}
		r := &alexactl.Reminder{
			DeviceSerialNumber: dvc.SerialNumber,
			FireAt:             fireAt,
			Label:              label,
			Recurrence:         recur,
		}
		created, err := client.Reminders.Create(ctx, r)
		if err != nil {
			return err
		}
		fmt.Printf("created: %s  (id=%s, fires at %s)\n",
			created.Label, created.ID, created.FireAt.Format(time.RFC3339))
		return nil
	},
}

var remindersEnableCmd = &cobra.Command{
	Use:   "enable <reminder-id>",
	Short: "Set status=ON on a reminder",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *alexactl.Reminder, client *alexactl.Client, _ []string) error {
		return client.Reminders.Enable(ctx, r)
	}),
}

var remindersDisableCmd = &cobra.Command{
	Use:   "disable <reminder-id>",
	Short: "Set status=OFF on a reminder",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *alexactl.Reminder, client *alexactl.Client, _ []string) error {
		return client.Reminders.Disable(ctx, r)
	}),
}

var remindersAcknowledgeCmd = &cobra.Command{
	Use:   "acknowledge <reminder-id>",
	Short: "Mark a reminder as acknowledged",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *alexactl.Reminder, client *alexactl.Client, _ []string) error {
		return client.Reminders.Acknowledge(ctx, r)
	}),
}

var remindersDeleteCmd = &cobra.Command{
	Use:   "delete <reminder-id>",
	Short: "Delete a reminder",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *alexactl.Reminder, client *alexactl.Client, _ []string) error {
		return client.Reminders.Delete(ctx, r)
	}),
}

var remindersShowCmd = &cobra.Command{
	Use:   "show <reminder-id>",
	Short: "Print the raw server-shape map for a reminder as pretty JSON",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(_ context.Context, r *alexactl.Reminder, _ *alexactl.Client, _ []string) error {
		raw := alexactl.ReminderServerShape(r)
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}),
}

func withReminder(action func(ctx context.Context, r *alexactl.Reminder, client *alexactl.Client, args []string) error) func(*cobra.Command, []string) error {
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
		reminders, err := client.Reminders.Get(ctx)
		if err != nil {
			return err
		}
		id := args[0]
		for _, r := range reminders {
			if r.ID == id {
				return action(ctx, r, client, args)
			}
		}
		return fmt.Errorf("reminder id %q not found", id)
	}
}

func init() {
	registerDeviceFlag(remindersCreateCmd, &remindersDeviceFlag)
	remindersCreateCmd.Flags().StringVar(&remindersRecurrenceFlag, "recurrence", "",
		"recurrence: daily | weekdays | weekends | weekly:MO,WE,FR | rrule:\"FREQ=...\"")

	remindersLsCmd.Flags().StringVar(&remindersLsDeviceFlag, "device", "", "filter to one device (serial or name); omit for all devices")

	remindersCmd.AddCommand(remindersLsCmd)
	remindersCmd.AddCommand(remindersCreateCmd)
	remindersCmd.AddCommand(remindersEnableCmd)
	remindersCmd.AddCommand(remindersDisableCmd)
	remindersCmd.AddCommand(remindersAcknowledgeCmd)
	remindersCmd.AddCommand(remindersDeleteCmd)
	remindersCmd.AddCommand(remindersShowCmd)
	rootCmd.AddCommand(remindersCmd)
}
