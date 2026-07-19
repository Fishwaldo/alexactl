// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

// versionCmd prints the build version (from alexactl.Version, the single
// source of truth) and the upstream aioamazondevices version it mirrors.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the alexactl version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "alexactl %s (upstream aioamazondevices %s)\n",
			alexactl.Version, alexactl.UpstreamVersion)
		return err
	},
}

func init() {
	// Wire the library version into cobra so `alexactl --version` works too.
	rootCmd.Version = alexactl.Version
	rootCmd.AddCommand(versionCmd)
}
