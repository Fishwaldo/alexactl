package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Fishwaldo/goamazondevices"
)

var (
	cfgFile      string
	emailFlag    string
	passwordFlag string
	otpFlag      string
	sessionFile  string
	siteFlag     string
	debugFlag    bool
)

// signalContext returns a context that's canceled on SIGINT / SIGTERM.
// Uses stdlib signal.NotifyContext, which cleans up the signal handler
// goroutine when the returned cancel func runs.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// makeLogger returns the *slog.Logger to inject into the client.
func makeLogger() *slog.Logger {
	level := slog.LevelInfo
	if debugFlag {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

var rootCmd = &cobra.Command{
	Use:   "amazondevices-test",
	Short: "Manual CLI harness for the goamazondevices library",
	Long: `Manual smoke-test CLI for goamazondevices. Logs in, optionally
persists the session, lists devices, and exercises media commands.

Reads configuration from --config (default: ./config.yaml). Flags override
config; environment variables override config too (prefix AMZ_).`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		viper.SetConfigType("yaml")
		viper.SetEnvPrefix("AMZ")
		viper.AutomaticEnv()
		if cfgFile != "" {
			viper.SetConfigFile(cfgFile)
		} else {
			viper.AddConfigPath(".")
			viper.SetConfigName("config")
		}
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return fmt.Errorf("read config: %w", err)
			}
		}
		_ = viper.BindPFlag("email", cmd.Flags().Lookup("email"))
		_ = viper.BindPFlag("password", cmd.Flags().Lookup("password"))
		_ = viper.BindPFlag("otp", cmd.Flags().Lookup("otp"))
		_ = viper.BindPFlag("session_file", cmd.Flags().Lookup("session-file"))
		_ = viper.BindPFlag("site", cmd.Flags().Lookup("site"))
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&emailFlag, "email", "", "Amazon account email")
	rootCmd.PersistentFlags().StringVar(&passwordFlag, "password", "", "Amazon account password")
	rootCmd.PersistentFlags().StringVar(&otpFlag, "otp", "", "OTP code (skip the interactive prompt)")
	rootCmd.PersistentFlags().StringVar(&sessionFile, "session-file", "session.json", "path to saved session")
	rootCmd.PersistentFlags().StringVar(&siteFlag, "site", "https://www.amazon.com", "Amazon site URL")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug-level logging (default is info)")
}

// Execute is the cobra entry point.
func Execute() error {
	return rootCmd.Execute()
}

// helper: build a configured Client from the resolved viper config.
func newClient(loadSession bool) (*amazondevices.Client, *amazondevices.Session, error) {
	site := viper.GetString("site")
	if site == "" {
		site = "https://www.amazon.com"
	}

	var sess *amazondevices.Session
	if loadSession {
		path := viper.GetString("session_file")
		if path != "" {
			if b, err := os.ReadFile(path); err == nil {
				var s amazondevices.Session
				if err := jsonUnmarshalSession(b, &s); err == nil {
					sess = &s
				}
			}
		}
	}

	opts := []amazondevices.Option{
		amazondevices.WithLogger(makeLogger()),
		amazondevices.WithSite(site),
	}
	if sess != nil {
		opts = append(opts, amazondevices.WithSession(sess))
	}
	c, err := amazondevices.NewClient(opts...)
	if err != nil {
		return nil, nil, err
	}
	return c, sess, nil
}

// jsonUnmarshalSession is a tiny indirection so we don't import encoding/json
// in five places.
func jsonUnmarshalSession(b []byte, s *amazondevices.Session) error {
	return jsonUnmarshal(b, s)
}
