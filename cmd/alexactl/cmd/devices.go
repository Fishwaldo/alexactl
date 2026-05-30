package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "Device discovery commands",
}

var devicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Amazon devices on the account",
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

		devs, err := client.Devices.List(ctx)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(os.Stdout, "%-32s  %-20s  %-16s  %-7s  %s\n",
			"SERIAL", "KIND", "TYPE", "ONLINE", "NAME")
		for _, d := range devs {
			_, _ = fmt.Fprintf(os.Stdout, "%-32s  %-20s  %-16s  %-7v  %s\n",
				d.SerialNumber, kindOrUnknown(d.Kind), d.DeviceType, d.Online, d.AccountName)
			if hw := summarizeHardware(d); hw != "" {
				_, _ = fmt.Fprintf(os.Stdout, "  %s\n", hw)
			}
			if summary := summarizeDevice(d); summary != "" {
				_, _ = fmt.Fprintf(os.Stdout, "  %s\n", summary)
			}
			if detailFlag {
				if len(d.DisplayCategories) > 0 {
					_, _ = fmt.Fprintf(os.Stdout, "    categories=%v\n", d.DisplayCategories)
				}
				if len(d.AlexaInterfaces) > 0 {
					_, _ = fmt.Fprintf(os.Stdout, "    interfaces=%v\n", d.AlexaInterfaces)
				}
				if d.SoftwareVersion != "" {
					_, _ = fmt.Fprintf(os.Stdout, "    software=%q\n", d.SoftwareVersion)
				}
			}
		}
		return nil
	},
}

// sessionFilePath returns the resolved session file path.
func sessionFilePath() string {
	return store.SessionPath()
}

func init() {
	devicesCmd.AddCommand(devicesListCmd)
	rootCmd.AddCommand(devicesCmd)
}

// kindOrUnknown returns the device's Kind string, or "?" if empty.
// Compact for the table column.
func kindOrUnknown(k alexactl.Kind) string {
	if k == "" {
		return "?"
	}
	return string(k)
}

// summarizeHardware returns a short manufacturer/model identifier
// line, or "" if neither is set. Hardware info is on a separate
// indented line so it doesn't compete with state in the same column.
// Examples:
//
//	"TP-LINK HS100"
//	"Signify Netherlands B.V. LCT024"
//	"Amazon Echo Show 8 (gen 2)"
func summarizeHardware(d *alexactl.Device) string {
	mfg := d.Manufacturer
	model := d.Model
	switch {
	case mfg != "" && model != "":
		return mfg + " " + model
	case mfg != "":
		return mfg
	case model != "":
		return model
	default:
		return ""
	}
}

// summarizeDevice returns a short per-Kind status line, or "" if the
// device's Sensors don't carry enough state to make one. Examples:
//
//	light:      "ON, 80%, 2700K"
//	thermostat: "COOL, target=21.5°C"
//	lock:       "LOCKED"
//	switch:     "OFF"
func summarizeDevice(d *alexactl.Device) string {
	var parts []string
	if p, ok := d.Sensors["powerState"]; ok && p != nil {
		if s, _ := p.Value.(string); s != "" {
			parts = append(parts, s)
		}
	}
	if p, ok := d.Sensors["brightness"]; ok && p != nil {
		switch v := p.Value.(type) {
		case int:
			parts = append(parts, fmt.Sprintf("%d%%", v))
		case float64:
			parts = append(parts, fmt.Sprintf("%.0f%%", v))
		}
	}
	if p, ok := d.Sensors["colorTemperature"]; ok && p != nil {
		switch v := p.Value.(type) {
		case int:
			parts = append(parts, fmt.Sprintf("%dK", v))
		case float64:
			parts = append(parts, fmt.Sprintf("%.0fK", v))
		}
	}
	if p, ok := d.Sensors["lockState"]; ok && p != nil {
		if s, _ := p.Value.(string); s != "" {
			parts = []string{s} // lock state is the whole summary
		}
	}
	if p, ok := d.Sensors["thermostatMode"]; ok && p != nil {
		if s, _ := p.Value.(string); s != "" {
			parts = append(parts, s)
		}
	}
	if p, ok := d.Sensors["targetSetpoint"]; ok && p != nil {
		if f, _ := p.Value.(float64); f > 0 {
			scale := "°"
			switch p.Scale {
			case "CELSIUS":
				scale = "°C"
			case "FAHRENHEIT":
				scale = "°F"
			}
			parts = append(parts, fmt.Sprintf("target=%.1f%s", f, scale))
		}
	}
	return strings.Join(parts, ", ")
}
