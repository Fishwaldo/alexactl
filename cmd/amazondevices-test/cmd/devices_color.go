package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	amazondevices "github.com/Fishwaldo/goamazondevices"
)

var colorDevice string

var devicesColorCmd = &cobra.Command{
	Use:   "color <hex | hsv h s b>",
	Short: "Set an RGB device's color from a hex code or HSV triple",
	Long: `Issues an Alexa.ColorController setColor directive against the
given device. Color can be specified either as a 6-digit RGB hex
(optionally with leading '#') or as 'hsv <hue> <saturation> <brightness>'
with hue in degrees [0..360) and saturation/brightness as floats [0..1].

Examples:
  amazondevices-test devices color #FF5733 --device "Bedroom"   # red-orange via hex
  amazondevices-test devices color FF5733 --device AB1234567890 # leading # optional
  amazondevices-test devices color hsv 348 0.91 1.0`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		color, err := parseColorArgs(args)
		if err != nil {
			return err
		}

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

		d, err := resolveDevice(ctx, colorDevice, client, store)
		if err != nil {
			return err
		}
		c := d.Color()
		if c == nil {
			return fmt.Errorf("device %q does not expose Alexa.ColorController", d.SerialNumber)
		}
		return c.Set(ctx, color)
	},
}

// parseColorArgs accepts either:
//   - one hex string ("#FF5733" or "FF5733") → RGB → Color via ColorFromRGB
//   - "hsv <h> <s> <b>" (4 args) → Color literal
func parseColorArgs(args []string) (amazondevices.Color, error) {
	if len(args) == 1 {
		hex := strings.TrimPrefix(args[0], "#")
		if len(hex) != 6 {
			return amazondevices.Color{}, fmt.Errorf("invalid hex %q: want 6 hex digits (e.g. FF5733)", args[0])
		}
		rgb, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return amazondevices.Color{}, fmt.Errorf("invalid hex %q: %w", args[0], err)
		}
		r := uint8((rgb >> 16) & 0xFF)
		g := uint8((rgb >> 8) & 0xFF)
		b := uint8(rgb & 0xFF)
		return amazondevices.ColorFromRGB(r, g, b), nil
	}
	if len(args) == 4 && args[0] == "hsv" {
		h, herr := strconv.ParseFloat(args[1], 64)
		s, serr := strconv.ParseFloat(args[2], 64)
		br, berr := strconv.ParseFloat(args[3], 64)
		if herr != nil || serr != nil || berr != nil {
			return amazondevices.Color{}, fmt.Errorf("hsv args must be 3 floats; got %v %v %v", args[1], args[2], args[3])
		}
		return amazondevices.Color{Hue: h, Saturation: s, Brightness: br}, nil
	}
	return amazondevices.Color{}, fmt.Errorf("color: want either <hex> or 'hsv <h> <s> <b>', got %v", args)
}

func init() {
	devicesCmd.AddCommand(devicesColorCmd)
	registerDeviceFlag(devicesColorCmd, &colorDevice)
}
