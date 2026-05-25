package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

// newSoundCmd builds the `sound` parent command with `list` and `play`
// subcommands.
func newSoundCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sound",
		Short: "List or play Alexa sound effects",
	}
	c.AddCommand(newSoundListCmd())
	c.AddCommand(newSoundPlayCmd())
	return c
}

func newSoundListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available sound effect IDs and labels",
		RunE: func(_ *cobra.Command, _ []string) error {
			client, _, err := newClient(false)
			if err != nil {
				return err
			}
			sounds := client.Media.Sounds()
			ids := make([]string, 0, len(sounds))
			for id := range sounds {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			fmt.Fprintf(os.Stdout, "%-46s  %s\n", "ID", "LABEL")
			for _, id := range ids {
				fmt.Fprintf(os.Stdout, "%-46s  %s\n", id, sounds[id])
			}
			return nil
		},
	}
}

func newSoundPlayCmd() *cobra.Command {
	var serial, soundID string
	c := &cobra.Command{
		Use:   "play",
		Short: "Play a sound effect on a device (cluster fanout)",
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	c.Flags().StringVar(&soundID, "sound-id", "", "sound effect id (e.g. amzn_sfx_doorbell_01) (required)")
	_ = c.MarkFlagRequired("serial")
	_ = c.MarkFlagRequired("sound-id")
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		return client.Media.PlaySound(ctx, dvc, soundID)
	}
	return c
}

// newMusicCmd builds `music` — query is a positional argument.
//
// Usage: amazondevices-test music --serial S --provider amzn-music "avici"
func newMusicCmd() *cobra.Command {
	var serial, provider string
	c := &cobra.Command{
		Use:   "music <query>",
		Short: "Play music on a device via the named provider",
		Args:  cobra.ExactArgs(1),
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	c.Flags().StringVar(&provider, "provider", "", "music provider id (required; see `music-providers`)")
	_ = c.MarkFlagRequired("serial")
	_ = c.MarkFlagRequired("provider")
	c.RunE = func(_ *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()
		query := args[0]
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		return client.Media.PlayMusic(ctx, dvc, query, provider)
	}
	return c
}

func newTextCmd() *cobra.Command {
	var serial, text string
	c := &cobra.Command{
		Use:   "text",
		Short: "Send an Alexa.TextCommand to a device",
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	c.Flags().StringVar(&text, "text", "", "command text (required)")
	_ = c.MarkFlagRequired("serial")
	_ = c.MarkFlagRequired("text")
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		return client.Media.TextCommand(ctx, dvc, text)
	}
	return c
}

// newSkillCmd builds the `skill` parent with `list` and `launch`
// subcommands.
func newSkillCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "skill",
		Short: "List or launch Alexa info skills",
	}
	c.AddCommand(newSkillListCmd())
	c.AddCommand(newSkillLaunchCmd())
	return c
}

func newSkillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the canonical Alexa info-skill names",
		RunE: func(_ *cobra.Command, _ []string) error {
			client, _, err := newClient(false)
			if err != nil {
				return err
			}
			for _, s := range client.Media.InfoSkills() {
				fmt.Fprintln(os.Stdout, s)
			}
			return nil
		},
	}
}

func newSkillLaunchCmd() *cobra.Command {
	var serial, skill string
	c := &cobra.Command{
		Use:   "launch",
		Short: "Launch a skill on a device",
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	c.Flags().StringVar(&skill, "skill", "", "skill name (required)")
	_ = c.MarkFlagRequired("serial")
	_ = c.MarkFlagRequired("skill")
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		return client.Media.LaunchSkill(ctx, dvc, skill)
	}
	return c
}

// newControlCmd builds `control` — the command keyword is a positional
// argument.
//
// Usage: amazondevices-test control --serial S play
func newControlCmd() *cobra.Command {
	var serial string
	c := &cobra.Command{
		Use:   "control <play|pause|next|prev|stop|rewind|forward>",
		Short: "Send a media control",
		Args:  cobra.ExactArgs(1),
	}
	c.Flags().StringVar(&serial, "serial", "", "device serial number (required)")
	_ = c.MarkFlagRequired("serial")
	c.RunE = func(_ *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()
		mc, err := parseMediaCommand(args[0])
		if err != nil {
			return err
		}
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
		dvc, err := client.Devices.Get(ctx, serial)
		if err != nil {
			return err
		}
		return client.Media.Control(ctx, dvc, mc)
	}
	return c
}

// parseMediaCommand maps user-friendly strings ("play", "stop", ...) to
// the typed amazondevices.MediaCommand.
func parseMediaCommand(s string) (amazondevices.MediaCommand, error) {
	switch strings.ToLower(s) {
	case "play":
		return amazondevices.CmdPlay, nil
	case "pause":
		return amazondevices.CmdPause, nil
	case "next":
		return amazondevices.CmdNext, nil
	case "prev", "previous":
		return amazondevices.CmdPrev, nil
	case "stop":
		return amazondevices.CmdStop, nil
	case "rewind":
		return amazondevices.CmdRewind, nil
	case "forward", "fastforward", "ff":
		return amazondevices.CmdForward, nil
	}
	return "", fmt.Errorf("unknown media command %q", s)
}

func init() {
	rootCmd.AddCommand(newSoundCmd())
	rootCmd.AddCommand(newMusicCmd())
	rootCmd.AddCommand(newTextCmd())
	rootCmd.AddCommand(newSkillCmd())
	rootCmd.AddCommand(newControlCmd())
}
