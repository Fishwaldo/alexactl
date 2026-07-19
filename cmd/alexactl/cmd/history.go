// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var historyDeviceFlag string

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Print the latest vocal record per device (last 7 days)",
	Long: `Fetches the most recent recorded utterance per device serial
from /alexa-privacy/apd/rah. Internally scrapes a CSRF token from the
alexa-privacy frontend page and waits 2s for backend propagation
before issuing the POST.`,
	Args: cobra.NoArgs,
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

		filter, err := resolveFilter(ctx, historyDeviceFlag, client)
		if err != nil {
			return err
		}
		records, err := client.History.Vocal(ctx)
		if err != nil {
			return err
		}

		serials := make([]string, 0, len(records))
		for s := range records {
			if filter != "" && s != filter {
				continue
			}
			serials = append(serials, s)
		}
		sort.Strings(serials)

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "%-32s  %-20s  %s\n",
			"SERIAL", "TIMESTAMP", "TITLE")
		for _, s := range serials {
			r := records[s]
			_, _ = fmt.Fprintf(out, "%-32s  %-20s  %s\n",
				s, r.Timestamp.Format("2006-01-02 15:04:05"), r.Title)
		}
		return nil
	},
}

func init() {
	registerDeviceFilterFlag(historyCmd, &historyDeviceFlag)
	rootCmd.AddCommand(historyCmd)
}
