// Package cmd implements the cobra commands for the amazondevices-test CLI,
// a manual exercise harness for the goamazondevices library.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

var (
	cfgFile      string
	emailFlag    string
	passwordFlag string
	otpFlag      string
	sessionFile  string
	siteFlag     string
	debugFlag    bool
	detailFlag   bool
	cacheFile    string
	store        *Store
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
config; environment variables override config too (prefix AMZ_).

Device targeting: commands that act on a single device take an optional
--device flag accepting either a serial number or a friendly name (names
are resolved against the live device list; ambiguous names error with the
candidates listed). When --device is omitted, the device configured via
'config set default-device <serial-or-name>' is used. List/filter commands
(e.g. alarms ls, media-state) also take --device as a filter but never fall
back to the default — omitting it lists all devices.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		s, err := Load(cfgFile, sessionFile, cacheFile)
		if err != nil {
			return err
		}
		store = s

		// The global viper resolves credentials (email/password/otp) and the
		// AMZ_SITE env fallback from flags + AMZ_ env + the same config file
		// the Store resolved. The Store owns writes and the new keys.
		viper.SetConfigType("yaml")
		viper.SetEnvPrefix("AMZ")
		viper.AutomaticEnv()
		viper.SetConfigFile(store.ConfigPath())
		if err := viper.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) && !os.IsNotExist(err) {
				return fmt.Errorf("read config: %w", err)
			}
		}
		_ = viper.BindPFlag("email", cmd.Flags().Lookup("email"))
		_ = viper.BindPFlag("password", cmd.Flags().Lookup("password"))
		_ = viper.BindPFlag("otp", cmd.Flags().Lookup("otp"))
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file (default ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&emailFlag, "email", "", "Amazon account email")
	rootCmd.PersistentFlags().StringVar(&passwordFlag, "password", "", "Amazon account password")
	rootCmd.PersistentFlags().StringVar(&otpFlag, "otp", "", "OTP code (skip the interactive prompt)")
	rootCmd.PersistentFlags().StringVar(&sessionFile, "session-file", "", "path to saved session (default: OS config dir)")
	rootCmd.PersistentFlags().StringVar(&cacheFile, "cache-file", "", "path to cache file (default: OS cache dir)")
	rootCmd.PersistentFlags().StringVar(&siteFlag, "site", "", "Amazon site URL (default https://www.amazon.com)")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug-level logging (default is info)")
	rootCmd.PersistentFlags().BoolVar(&detailFlag, "detail", false,
		"include extra per-item detail in read-only commands (Configuration / supported modes / presets / interfaces / categories)")
}

// Execute is the cobra entry point.
func Execute() error {
	return rootCmd.Execute()
}

// newClient builds a configured Client from the Store. When loadSession is
// true, a persisted session is supplied via WithSession. Site precedence:
// --site flag > config site: key > AMZ_SITE env > default.
func newClient(loadSession bool) (*amazondevices.Client, *amazondevices.Session, error) {
	site := siteFlag
	if site == "" {
		if v, ok := store.Get("site"); ok {
			site = v
		}
	}
	if site == "" {
		site = viper.GetString("site") // AMZ_SITE via AutomaticEnv
	}
	if site == "" {
		site = "https://www.amazon.com"
	}

	var sess *amazondevices.Session
	if loadSession {
		sess = store.LoadSession()
	}

	opts := []amazondevices.Option{
		amazondevices.WithLogger(makeLogger()),
		amazondevices.WithSite(site),
		amazondevices.WithCachePersistence(store.CachePersistence()),
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
