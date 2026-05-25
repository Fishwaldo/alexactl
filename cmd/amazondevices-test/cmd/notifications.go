package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var notificationsCmd = &cobra.Command{
	Use:   "notifications",
	Short: "List per-device alarms, reminders, and timers",
	Long: `Refreshes devices and prints Device.Notifications for every
device on the account. Notifications are populated by the sensors
enrichment step of Refresh (REMINDERS / TIMERS_AND_ALARMS capability
required on the device).`,
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

		devs, err := client.Devices.List(ctx)
		if err != nil {
			return err
		}

		for _, d := range devs {
			fmt.Fprintf(os.Stdout, "%s (%s)\n", d.AccountName, d.SerialNumber)
			if len(d.Notifications) == 0 {
				fmt.Fprintln(os.Stdout, "  (no notifications)")
				continue
			}
			types := make([]string, 0, len(d.Notifications))
			for t := range d.Notifications {
				types = append(types, t)
			}
			sort.Strings(types)
			for _, t := range types {
				n := d.Notifications[t]
				fmt.Fprintf(os.Stdout, "  %-10s  %s  next=%s\n",
					t, n.Label, n.NextOccurrence.Format("2006-01-02 15:04:05 MST"))
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(notificationsCmd)
}
