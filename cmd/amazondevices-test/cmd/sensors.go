package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
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
			_, _ = fmt.Fprintf(os.Stdout, "%s (%s, online=%v)\n",
				d.AccountName, d.SerialNumber, d.Online)
			if len(d.Sensors) == 0 {
				_, _ = fmt.Fprintln(os.Stdout, "  (no sensors)")
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
				_, _ = fmt.Fprintf(os.Stdout, "  %-20s  %v%s%s\n", n, s.Value, scale, flags)
				if detailFlag {
					if line := formatSensorDetail(s); line != "" {
						_, _ = fmt.Fprintf(os.Stdout, "    %s\n", line)
					}
				}
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sensorsCmd)
}

// formatSensorDetail returns a single-line summary of the
// non-state fields on a *DeviceSensor that are interesting under
// --detail (Operations + Configuration). Returns "" when there's
// nothing extra to show.
func formatSensorDetail(s *amazondevices.DeviceSensor) string {
	if s == nil {
		return ""
	}
	parts := []string{}
	if len(s.Operations) > 0 {
		parts = append(parts, fmt.Sprintf("ops=%v", s.Operations))
	}
	if len(s.Configuration) > 0 {
		// Compact one-line JSON with sorted keys for stable output.
		keys := make([]string, 0, len(s.Configuration))
		for k := range s.Configuration {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf []byte
		buf = append(buf, '{')
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, _ := json.Marshal(k)
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, _ := json.Marshal(s.Configuration[k])
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		parts = append(parts, "cfg="+string(buf))
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "  " + p
	}
	return out
}
