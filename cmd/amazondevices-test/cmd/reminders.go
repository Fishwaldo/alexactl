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
	remindersSerialFlag     string
	remindersAtFlag         string
	remindersLabelFlag      string
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
		byDevice := map[string][]*amazondevices.Reminder{}
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
		reminders, err := client.Reminders.Get(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-60s %-20s %-30s %s\n", "ID", "FIRE_AT", "LABEL", "RECURRENCE")
		for _, r := range reminders {
			if remindersSerialFlag != "" && r.DeviceSerialNumber != remindersSerialFlag {
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
	Use:   "create",
	Short: "Create a new reminder",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if remindersSerialFlag == "" {
			return fmt.Errorf("--serial is required")
		}
		if remindersAtFlag == "" {
			return fmt.Errorf("--at is required")
		}
		if remindersLabelFlag == "" {
			return fmt.Errorf("--label is required (this is what Alexa speaks)")
		}
		fireAt, err := parseAtFlag(remindersAtFlag)
		if err != nil {
			return err
		}
		var recur *amazondevices.Recurrence
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
		r := &amazondevices.Reminder{
			DeviceSerialNumber: remindersSerialFlag,
			FireAt:             fireAt,
			Label:              remindersLabelFlag,
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
	RunE: withReminder(func(ctx context.Context, r *amazondevices.Reminder, client *amazondevices.Client, _ []string) error {
		return client.Reminders.Enable(ctx, r)
	}),
}

var remindersDisableCmd = &cobra.Command{
	Use:   "disable <reminder-id>",
	Short: "Set status=OFF on a reminder",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *amazondevices.Reminder, client *amazondevices.Client, _ []string) error {
		return client.Reminders.Disable(ctx, r)
	}),
}

var remindersAcknowledgeCmd = &cobra.Command{
	Use:   "acknowledge <reminder-id>",
	Short: "Mark a reminder as acknowledged",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *amazondevices.Reminder, client *amazondevices.Client, _ []string) error {
		return client.Reminders.Acknowledge(ctx, r)
	}),
}

var remindersDeleteCmd = &cobra.Command{
	Use:   "delete <reminder-id>",
	Short: "Delete a reminder",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(ctx context.Context, r *amazondevices.Reminder, client *amazondevices.Client, _ []string) error {
		return client.Reminders.Delete(ctx, r)
	}),
}

var remindersShowCmd = &cobra.Command{
	Use:   "show <reminder-id>",
	Short: "Print the raw server-shape map for a reminder as pretty JSON",
	Args:  cobra.ExactArgs(1),
	RunE: withReminder(func(_ context.Context, r *amazondevices.Reminder, _ *amazondevices.Client, _ []string) error {
		raw := amazondevices.ReminderServerShape(r)
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}),
}

func withReminder(action func(ctx context.Context, r *amazondevices.Reminder, client *amazondevices.Client, args []string) error) func(*cobra.Command, []string) error {
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
	remindersCreateCmd.Flags().StringVar(&remindersSerialFlag, "serial", "", "target device serial")
	remindersCreateCmd.Flags().StringVar(&remindersAtFlag, "at", "", "fire time, \"YYYY-MM-DD HH:MM\" or \"HH:MM\"")
	remindersCreateCmd.Flags().StringVar(&remindersLabelFlag, "label", "", "what Alexa speaks (REQUIRED)")
	remindersCreateCmd.Flags().StringVar(&remindersRecurrenceFlag, "recurrence", "",
		"recurrence: daily | weekdays | weekends | weekly:MO,WE,FR | rrule:\"FREQ=...\"")

	remindersLsCmd.Flags().StringVar(&remindersSerialFlag, "serial", "", "optional device serial filter")

	remindersCmd.AddCommand(remindersLsCmd)
	remindersCmd.AddCommand(remindersCreateCmd)
	remindersCmd.AddCommand(remindersEnableCmd)
	remindersCmd.AddCommand(remindersDisableCmd)
	remindersCmd.AddCommand(remindersAcknowledgeCmd)
	remindersCmd.AddCommand(remindersDeleteCmd)
	remindersCmd.AddCommand(remindersShowCmd)
	rootCmd.AddCommand(remindersCmd)
}
