// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var (
	alarmsDeviceFlag     string
	alarmsLsDeviceFlag   string
	alarmsLabelFlag      string
	alarmsSoundFlag      string
	alarmsRecurrenceFlag string
)

var alarmsCmd = &cobra.Command{
	Use:   "alarms",
	Short: "List, create, and modify clock-time alarms on Echo devices",
	Long: `Read or modify alarms tied to your Amazon account.

With no subcommand, prints a summary of all active alarms grouped
by device.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, sess, err := newClient(true)
		if err != nil {
			return err
		}
		if sess == nil {
			return fmt.Errorf("no saved session at %s — run `login` first", sessionFilePath())
		}
		if _, err := client.Auth.Restore(ctx); err != nil {
			return err
		}
		alarms, err := client.Alarms.Get(ctx)
		if err != nil {
			return err
		}
		byDevice := map[string][]*alexactl.Alarm{}
		for _, a := range alarms {
			byDevice[a.DeviceSerialNumber] = append(byDevice[a.DeviceSerialNumber], a)
		}
		for serial, list := range byDevice {
			fmt.Printf("%s — %d active alarm(s)\n", serial, len(list))
			for _, a := range list {
				rec := "(one-shot)"
				if a.Recurrence != nil {
					rec = a.Recurrence.Describe()
				}
				fmt.Printf("  %s  %s  %s  [%s]\n", a.ID, a.FireAt.Format(time.RFC3339), a.Label, rec)
			}
		}
		return nil
	},
}

var alarmsLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Detailed alarm list",
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
		filter, err := resolveFilter(ctx, alarmsLsDeviceFlag, client)
		if err != nil {
			return err
		}
		alarms, err := client.Alarms.Get(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("%-50s %-20s %-30s %s\n", "ID", "FIRE_AT", "LABEL", "RECURRENCE")
		for _, a := range alarms {
			if filter != "" && a.DeviceSerialNumber != filter {
				continue
			}
			rec := "(one-shot)"
			if a.Recurrence != nil {
				rec = a.Recurrence.Describe()
			}
			fmt.Printf("%-50s %-20s %-30s %s\n",
				a.ID, a.FireAt.Format("2006-01-02 15:04"), a.Label, rec)
		}
		return nil
	},
}

var alarmsCreateCmd = &cobra.Command{
	Use:   "create <at>",
	Short: "Create a new alarm",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fireAt, err := parseAtFlag(args[0])
		if err != nil {
			return err
		}
		var recur *alexactl.Recurrence
		if alarmsRecurrenceFlag != "" {
			recur, err = parseRecurrenceFlag(alarmsRecurrenceFlag)
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
		dvc, err := resolveDevice(ctx, alarmsDeviceFlag, client, store)
		if err != nil {
			return err
		}
		a := &alexactl.Alarm{
			DeviceSerialNumber: dvc.SerialNumber,
			FireAt:             fireAt,
			Label:              alarmsLabelFlag,
			Sound:              alarmsSoundFlag,
			Recurrence:         recur,
		}
		created, err := client.Alarms.Create(ctx, a)
		if err != nil {
			return err
		}
		fmt.Printf("created: %s  (id=%s, fires at %s)\n",
			created.Label, created.ID, created.FireAt.Format(time.RFC3339))
		return nil
	},
}

var alarmsEnableCmd = &cobra.Command{
	Use:   "enable <alarm-id>",
	Short: "Set status=ON on an alarm",
	Args:  cobra.ExactArgs(1),
	RunE: withAlarm(func(ctx context.Context, a *alexactl.Alarm, client *alexactl.Client, _ []string) error {
		return client.Alarms.Enable(ctx, a)
	}),
}

var alarmsDisableCmd = &cobra.Command{
	Use:   "disable <alarm-id>",
	Short: "Set status=OFF on an alarm",
	Args:  cobra.ExactArgs(1),
	RunE: withAlarm(func(ctx context.Context, a *alexactl.Alarm, client *alexactl.Client, _ []string) error {
		return client.Alarms.Disable(ctx, a)
	}),
}

var alarmsSnoozeCmd = &cobra.Command{
	Use:   "snooze <alarm-id> <duration>",
	Short: "Snooze an alarm by a duration (e.g. 10m, 1h)",
	Args:  cobra.ExactArgs(2),
	RunE: withAlarm(func(ctx context.Context, a *alexactl.Alarm, client *alexactl.Client, args []string) error {
		dur, err := time.ParseDuration(args[1])
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", args[1], err)
		}
		return client.Alarms.Snooze(ctx, a, dur)
	}),
}

var alarmsDeleteCmd = &cobra.Command{
	Use:   "delete <alarm-id>",
	Short: "Delete an alarm",
	Args:  cobra.ExactArgs(1),
	RunE: withAlarm(func(ctx context.Context, a *alexactl.Alarm, client *alexactl.Client, _ []string) error {
		return client.Alarms.Delete(ctx, a)
	}),
}

var alarmsShowCmd = &cobra.Command{
	Use:   "show <alarm-id>",
	Short: "Print the raw server-shape map for an alarm as pretty JSON",
	Args:  cobra.ExactArgs(1),
	RunE: withAlarm(func(_ context.Context, a *alexactl.Alarm, _ *alexactl.Client, _ []string) error {
		raw := alexactl.AlarmServerShape(a)
		out, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	}),
}

// withAlarm builds a RunE that resolves an *Alarm by ID across all
// alarms on the account and dispatches.
func withAlarm(action func(ctx context.Context, a *alexactl.Alarm, client *alexactl.Client, args []string) error) func(*cobra.Command, []string) error {
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
		alarms, err := client.Alarms.Get(ctx)
		if err != nil {
			return err
		}
		id := args[0]
		for _, a := range alarms {
			if a.ID == id {
				return action(ctx, a, client, args)
			}
		}
		return fmt.Errorf("alarm id %q not found", id)
	}
}

// parseAtFlag accepts "2026-05-29 07:00" (absolute) or "07:00" (today;
// rolls to tomorrow if already past). time.Local.
func parseAtFlag(raw string) (time.Time, error) {
	if strings.Contains(raw, " ") {
		return time.ParseInLocation("2006-01-02 15:04", raw, time.Local)
	}
	t, err := time.ParseInLocation("15:04", raw, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --at %q: %w", raw, err)
	}
	now := time.Now()
	candidate := time.Date(now.Year(), now.Month(), now.Day(),
		t.Hour(), t.Minute(), 0, 0, time.Local)
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate, nil
}

// parseRecurrenceFlag interprets the --recurrence CLI flag:
//
//	daily               → RecurrenceDaily()
//	weekdays            → RecurrenceWeekdays()
//	weekends            → RecurrenceWeekends()
//	weekly:MO,WE,FR     → RecurrenceWeekly(Mon, Wed, Fri)
//	rrule:"FREQ=..."    → RecurrenceRule("FREQ=...")
func parseRecurrenceFlag(raw string) (*alexactl.Recurrence, error) {
	switch raw {
	case "daily":
		return alexactl.RecurrenceDaily(), nil
	case "weekdays":
		return alexactl.RecurrenceWeekdays(), nil
	case "weekends":
		return alexactl.RecurrenceWeekends(), nil
	}
	if strings.HasPrefix(raw, "weekly:") {
		spec := strings.TrimPrefix(raw, "weekly:")
		codes := strings.Split(spec, ",")
		days := make([]time.Weekday, 0, len(codes))
		for _, c := range codes {
			d, err := codeToWeekday(strings.TrimSpace(c))
			if err != nil {
				return nil, err
			}
			days = append(days, d)
		}
		return alexactl.RecurrenceWeekly(days...), nil
	}
	if strings.HasPrefix(raw, "rrule:") {
		return alexactl.RecurrenceRule(strings.TrimPrefix(raw, "rrule:")), nil
	}
	return nil, fmt.Errorf("invalid --recurrence %q (use daily | weekdays | weekends | weekly:DAYS | rrule:RRULE)", raw)
}

func codeToWeekday(code string) (time.Weekday, error) {
	switch strings.ToUpper(code) {
	case "MO":
		return time.Monday, nil
	case "TU":
		return time.Tuesday, nil
	case "WE":
		return time.Wednesday, nil
	case "TH":
		return time.Thursday, nil
	case "FR":
		return time.Friday, nil
	case "SA":
		return time.Saturday, nil
	case "SU":
		return time.Sunday, nil
	}
	return time.Sunday, fmt.Errorf("invalid weekday %q (use MO/TU/WE/TH/FR/SA/SU)", code)
}

func init() {
	registerDeviceFlag(alarmsCreateCmd, &alarmsDeviceFlag)
	alarmsCreateCmd.Flags().StringVar(&alarmsLabelFlag, "label", "", "optional label")
	alarmsCreateCmd.Flags().StringVar(&alarmsSoundFlag, "sound", "", "optional sound id")
	alarmsCreateCmd.Flags().StringVar(&alarmsRecurrenceFlag, "recurrence", "",
		"recurrence: daily | weekdays | weekends | weekly:MO,WE,FR | rrule:\"FREQ=...\"")

	alarmsLsCmd.Flags().StringVar(&alarmsLsDeviceFlag, "device", "", "filter to one device (serial or name); omit for all devices")

	alarmsCmd.AddCommand(alarmsLsCmd)
	alarmsCmd.AddCommand(alarmsCreateCmd)
	alarmsCmd.AddCommand(alarmsEnableCmd)
	alarmsCmd.AddCommand(alarmsDisableCmd)
	alarmsCmd.AddCommand(alarmsSnoozeCmd)
	alarmsCmd.AddCommand(alarmsDeleteCmd)
	alarmsCmd.AddCommand(alarmsShowCmd)
	rootCmd.AddCommand(alarmsCmd)
}
