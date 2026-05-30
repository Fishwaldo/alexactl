// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Fishwaldo/alexactl"
)

func newPushCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "push",
		Short: "Tail real-time push events from Amazon's AVS directives stream",
	}
	c.AddCommand(newPushTailCmd())
	return c
}

func newPushTailCmd() *cobra.Command {
	var capacity int
	var initialSync bool
	c := &cobra.Command{
		Use:   "tail",
		Short: "Subscribe to the push stream and print events to stdout until Ctrl+C",
		Long:  "Useful as a manual smoke test against real Amazon endpoints. Run after `login`.",
	}
	c.Flags().IntVar(&capacity, "capacity", 16,
		"per-subscriber channel buffer size (drop-oldest backpressure)")
	c.Flags().BoolVar(&initialSync, "initial-sync", false,
		"emit synthetic *VolumeChangeEvent per known device on first live event")
	c.RunE = func(_ *cobra.Command, _ []string) error {
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

		opts := []alexactl.SubscribeOption{
			alexactl.WithPushChannelCapacity(capacity),
		}
		if initialSync {
			opts = append(opts, alexactl.WithInitialSyncOnFirstEvent())
		}

		ch, err := client.Push.Subscribe(ctx, opts...)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "push tail: streaming events — Ctrl+C to stop")
		for evt := range ch {
			printEvent(evt)
		}
		return nil
	}
	return c
}

func printEvent(evt alexactl.PushEvent) {
	ts := time.Now().UTC().Format(time.RFC3339)
	if _, ok := evt.(*alexactl.ReauthRequiredEvent); ok {
		_, _ = fmt.Fprintf(os.Stdout, "%s REAUTH_REQUIRED — run `login` again\n", ts)
		return
	}
	rawBytes, _ := json.Marshal(evt.RawPayload())
	if u, ok := evt.(*alexactl.UnknownPushEvent); ok {
		_, _ = fmt.Fprintf(os.Stdout, "%s UNKNOWN[%s] serial=%s raw=%s\n",
			ts, u.WireType, u.SerialNumber(), rawBytes)
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s %s serial=%s raw=%s\n",
		ts, evt.Type(), evt.SerialNumber(), rawBytes)
}

func init() {
	rootCmd.AddCommand(newPushCmd())
}
