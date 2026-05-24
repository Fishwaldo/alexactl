package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		fmt.Fprintf(os.Stdout, "%-32s  %-16s  %-7s  %s\n", "SERIAL", "TYPE", "ONLINE", "NAME")
		for _, d := range devs {
			fmt.Fprintf(os.Stdout, "%-32s  %-16s  %-7v  %s\n",
				d.SerialNumber, d.DeviceType, d.Online, d.AccountName)
		}
		return nil
	},
}

// sessionFilePath returns the resolved session file path (or default).
func sessionFilePath() string {
	if v := viper.GetString("session_file"); v != "" {
		return v
	}
	return "session.json"
}

func init() {
	devicesCmd.AddCommand(devicesListCmd)
	rootCmd.AddCommand(devicesCmd)
}
