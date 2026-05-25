package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var sensorsCmd = &cobra.Command{
	Use:   "sensors",
	Short: "Print per-device sensor state",
	Long: `Refreshes devices (including the sensor enrichment step) and
prints Device.Sensors for every device. Sensor keys depend on the device:
Echos report 'reachability' and optionally 'temperature'/'dnd'; Air
Quality Monitors report Humidity, VOC, PM25, PM10, CO, Air Quality.`,
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
			fmt.Fprintf(os.Stdout, "%s (%s, online=%v)\n",
				d.AccountName, d.SerialNumber, d.Online)
			if len(d.Sensors) == 0 {
				fmt.Fprintln(os.Stdout, "  (no sensors)")
				continue
			}
			names := make([]string, 0, len(d.Sensors))
			for n := range d.Sensors {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				s := d.Sensors[n]
				flags := ""
				if s.Error {
					flags = fmt.Sprintf(" [error: %s %s]", s.ErrorType, s.ErrorMsg)
				}
				scale := ""
				if s.Scale != "" {
					scale = " " + s.Scale
				}
				fmt.Fprintf(os.Stdout, "  %-20s  %v%s%s\n", n, s.Value, scale, flags)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sensorsCmd)
}
