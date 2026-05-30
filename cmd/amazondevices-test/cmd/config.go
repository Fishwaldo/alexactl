package cmd

import (
	"fmt"
	"strings"

	amazondevices "github.com/Fishwaldo/goamazondevices"
	"github.com/spf13/cobra"
)

// resolveDeviceRef maps a serial-or-name reference to a serial. Serial match
// wins; otherwise an exact (case-sensitive) AccountName match is used. Zero
// matches is an error; multiple name matches is an error listing candidates.
func resolveDeviceRef(devices []*amazondevices.Device, ref string) (string, error) {
	for _, d := range devices {
		if d.SerialNumber == ref {
			return d.SerialNumber, nil
		}
	}
	var matches []*amazondevices.Device
	for _, d := range devices {
		if d.AccountName == ref {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no device with serial or name %q", ref)
	case 1:
		return matches[0].SerialNumber, nil
	default:
		var b strings.Builder
		for _, d := range matches {
			fmt.Fprintf(&b, "\n  %s  %s", d.SerialNumber, d.AccountName)
		}
		return "", fmt.Errorf("name %q is ambiguous; matches:%s\nre-run with an explicit serial", ref, b.String())
	}
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Get and set persisted CLI settings",
}

var configGetCmd = &cobra.Command{
	Use:   "get [option]",
	Short: "Print a persisted setting (or all settings)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			for key := range configKeys {
				if v, ok := store.Get(key); ok {
					fmt.Printf("%s = %s\n", key, v)
				}
			}
			return nil
		}
		key := args[0]
		v, ok := store.Get(key)
		if !ok {
			fmt.Printf("%s is not set\n", key)
			return nil
		}
		fmt.Println(v)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <option> <value>",
	Short: "Persist a setting (default-device accepts a serial or name)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		if key == "default-device" && val != "" {
			serial, err := resolveDefaultDeviceValue(val)
			if err != nil {
				return err
			}
			val = serial
		}
		if err := store.Set(key, val); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n", key, val)
		return nil
	},
}

// resolveDefaultDeviceValue logs in, lists devices, and resolves a
// serial-or-name reference to a serial. Requires a session.
func resolveDefaultDeviceValue(ref string) (string, error) {
	ctx, cancel := signalContext()
	defer cancel()
	client, sess, err := newClient(true)
	if err != nil {
		return "", err
	}
	if sess == nil {
		return "", fmt.Errorf("config set default-device needs a session at %s — run `login` first", sessionFilePath())
	}
	if _, err := client.Auth.Restore(ctx); err != nil {
		return "", fmt.Errorf("restore session: %w", err)
	}
	devices, err := client.Devices.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list devices: %w", err)
	}
	return resolveDeviceRef(devices, ref)
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
