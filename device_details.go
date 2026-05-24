package amazondevices

import (
	"regexp"
	"strings"
)

var (
	reGen     = regexp.MustCompile(`(?i)\(\s*(\d+(?:st|nd|rd|th))\s*gen\s*\)`)
	reYear    = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)
	reSpaces  = regexp.MustCompile(`\s+`)
	reGenWord = regexp.MustCompile(`(?i)\bgeneration\b`)
)

// parseDeviceDetails extracts a normalized model name and a hardware
// generation/year from a friendly model string.
//
// Examples:
//
//	"Echo Dot (5th Gen) with Clock" -> ("Echo Dot with Clock", "5th Gen")
//	"Fire TV Stick 4K (2nd Generation)" -> ("Fire TV Stick 4K", "2nd Gen")
//	"2021 Samsung UHD TV" -> ("Samsung UHD TV", "2021")
//
// Ported from aioamazondevices utils.parse_device_details.
func parseDeviceDetails(model string) (string, string) {
	if model == "" {
		return "", ""
	}
	model = reGenWord.ReplaceAllString(model, "Gen")
	model = strings.TrimSpace(model)

	if m := reGen.FindStringSubmatchIndex(model); m != nil {
		ordinal := strings.ToLower(model[m[2]:m[3]])
		stripped := model[:m[0]] + model[m[1]:]
		stripped = reSpaces.ReplaceAllString(stripped, " ")
		return strings.TrimSpace(stripped), ordinal + " Gen"
	}

	if m := reYear.FindStringIndex(model); m != nil {
		year := model[m[0]:m[1]]
		paren := "(" + year + ")"
		var stripped string
		switch {
		case strings.Contains(model, paren):
			stripped = strings.Replace(model, paren, "", 1)
		default:
			stripped = strings.Replace(model, year, "", 1)
		}
		stripped = reSpaces.ReplaceAllString(stripped, " ")
		return strings.TrimSpace(stripped), year
	}

	return model, ""
}
