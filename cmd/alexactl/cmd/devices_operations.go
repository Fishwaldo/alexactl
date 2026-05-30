// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var devicesOperationsCmd = &cobra.Command{
	Use:   "operations",
	Short: "Dump per-endpoint and per-feature operation manifests",
	Long: `Lists every device's controllable directives — both top-level
endpoint operations (lifecycle: forget / deregister) and per-feature
operations from the deep-query data (turnOn, setBrightness, setMode,
adjustMode, ...). The per-feature view is the gold mine for picking
HAR-capture targets when wiring new typed accessors: any device with
a non-empty feature.operations list responds to setEndpointFeatures
calls with the listed operation names.

Endpoints whose ONLY operations are lifecycle stubs are still
controllable in theory (Amazon's web UI hides controls based on a
UI-side decision rather than missing capability), but in practice
the absence of feature.operations means we have no idea what payload
shape to send — capture a HAR first.`,
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
		if err := client.Devices.Refresh(ctx); err != nil {
			return err
		}
		devs, err := client.Devices.List(ctx)
		if err != nil {
			return err
		}

		for _, d := range devs {
			// Header line per device.
			_, _ = fmt.Fprintf(os.Stdout, "%s  (%s, %s)\n",
				d.AccountName, d.SerialNumber, kindOrUnknown(d.Kind))

			endpointOps := "(none)"
			if len(d.Operations) > 0 {
				endpointOps = fmt.Sprintf("%v", d.Operations)
			}
			_, _ = fmt.Fprintf(os.Stdout, "  endpoint:                 %s\n", endpointOps)

			// Per-feature operations from Sensors[].Operations. Sorted by
			// sensor key for stable output across runs.
			featureKeys := make([]string, 0, len(d.Sensors))
			for k, s := range d.Sensors {
				if s != nil && len(s.Operations) > 0 {
					featureKeys = append(featureKeys, k)
				}
			}
			sort.Strings(featureKeys)
			if len(featureKeys) == 0 {
				_, _ = fmt.Fprintln(os.Stdout, "  (no per-feature operations)")
			} else {
				for _, k := range featureKeys {
					s := d.Sensors[k]
					_, _ = fmt.Fprintf(os.Stdout, "  %-25s %v\n", k+":", s.Operations)
					if detailFlag && len(s.Configuration) > 0 {
						if line := formatSensorDetail(s); line != "" {
							_, _ = fmt.Fprintf(os.Stdout, "    %s\n", line)
						}
					}
				}
			}
			_, _ = fmt.Fprintln(os.Stdout)
		}
		return nil
	},
}

func init() {
	devicesCmd.AddCommand(devicesOperationsCmd)
}
