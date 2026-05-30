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
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
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
			sounds, err := client.Media.Sounds(ctx)
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(sounds))
			for id := range sounds {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			_, _ = fmt.Fprintf(os.Stdout, "%-46s  %-26s  %s\n", "ID", "CATEGORY", "LABEL")
			for _, id := range ids {
				s := sounds[id]
				_, _ = fmt.Fprintf(os.Stdout, "%-46s  %-26s  %s\n", id, s.Category, s.DisplayName)
			}
			return nil
		},
	}
}

func newSoundPlayCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "play <sound-id>",
		Short: "Play a sound effect on a device (cluster fanout)",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(c, &device)
	c.RunE = func(_ *cobra.Command, args []string) error {
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.PlaySound(ctx, dvc, args[0])
	}
	return c
}

// newMusicCmd builds `music` — provider and query are positional
// arguments (provider first, then the search query).
//
// Usage: amazondevices-test music amzn-music "avici" --device "Bedroom"
func newMusicCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "music <provider> <query>",
		Short: "Play music on a device via the named provider",
		Args:  cobra.ExactArgs(2),
	}
	registerDeviceFlag(c, &device)
	c.RunE = func(_ *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()
		provider, query := args[0], args[1]
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.PlayMusic(ctx, dvc, query, provider)
	}
	return c
}

func newTextCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "text <command-text>",
		Short: "Send an Alexa.TextCommand to a device",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(c, &device)
	c.RunE = func(_ *cobra.Command, args []string) error {
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.TextCommand(ctx, dvc, args[0])
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
				_, _ = fmt.Fprintln(os.Stdout, s)
			}
			return nil
		},
	}
}

func newSkillLaunchCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "launch <skill>",
		Short: "Launch a skill on a device",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(c, &device)
	c.RunE = func(_ *cobra.Command, args []string) error {
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
		dvc, err := resolveDevice(ctx, device, client, store)
		if err != nil {
			return err
		}
		return client.Media.LaunchSkill(ctx, dvc, args[0])
	}
	return c
}

// newControlCmd builds `control` — the command keyword is a positional
// argument.
//
// Usage: amazondevices-test control --device "Bedroom" play
func newControlCmd() *cobra.Command {
	var device string
	c := &cobra.Command{
		Use:   "control <play|pause|next|prev|stop|rewind|forward>",
		Short: "Send a media control",
		Args:  cobra.ExactArgs(1),
	}
	registerDeviceFlag(c, &device)
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
		dvc, err := resolveDevice(ctx, device, client, store)
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
