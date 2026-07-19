// SPDX-FileCopyrightText: 2026 Justin Hammond
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Fishwaldo/alexactl"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Interactively log in and save the session to disk",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		out := cmd.OutOrStdout()
		client, existing, err := newClient(true)
		if err != nil {
			return err
		}
		if existing != nil {
			_, _ = fmt.Fprintln(out, "Restoring existing session...")
			s, err := client.Auth.Restore(ctx)
			if err == nil {
				return saveSession(out, s)
			}
			_, _ = fmt.Fprintf(out, "Restore failed: %v — falling back to interactive login\n", err)
		}

		email := viper.GetString("email")
		password := viper.GetString("password")
		if email == "" || password == "" {
			return fmt.Errorf("login: --email and --password required (or set in config / env AMZ_EMAIL, AMZ_PASSWORD)")
		}

		otpProvider := alexactl.OTPProviderFunc(func(c context.Context) (string, error) {
			if v := viper.GetString("otp"); v != "" {
				return v, nil
			}
			_, _ = fmt.Fprint(out, "Enter Amazon OTP: ")
			r := bufio.NewReader(os.Stdin)
			line, _, err := r.ReadLine()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(line)), nil
		})

		sess, err := client.Auth.Login(ctx, email, password, otpProvider)
		if err != nil {
			return err
		}
		return saveSession(out, sess)
	},
}

func saveSession(w io.Writer, s *alexactl.Session) error {
	if err := store.SaveSession(s); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "Session saved to %s\n", store.SessionPath())
	return nil
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
