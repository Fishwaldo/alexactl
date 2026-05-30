package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Interactively log in and save the session to disk",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		client, existing, err := newClient(true)
		if err != nil {
			return err
		}
		if existing != nil {
			fmt.Println("Restoring existing session...")
			s, err := client.Auth.Restore(ctx)
			if err == nil {
				return saveSession(s)
			}
			fmt.Printf("Restore failed: %v — falling back to interactive login\n", err)
		}

		email := viper.GetString("email")
		password := viper.GetString("password")
		if email == "" || password == "" {
			return fmt.Errorf("login: --email and --password required (or set in config / env AMZ_EMAIL, AMZ_PASSWORD)")
		}

		otpProvider := amazondevices.OTPProviderFunc(func(c context.Context) (string, error) {
			if v := viper.GetString("otp"); v != "" {
				return v, nil
			}
			fmt.Print("Enter Amazon OTP: ")
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
		return saveSession(sess)
	},
}

func saveSession(s *amazondevices.Session) error {
	if err := store.SaveSession(s); err != nil {
		return err
	}
	fmt.Printf("Session saved to %s\n", store.SessionPath())
	return nil
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
