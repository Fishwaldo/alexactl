// Package locale maps an Amazon country code (e.g., "US", "JP", "GB") to a
// BCP-47 language tag (e.g., "en-US", "ja-JP", "en-GB"). Replaces the
// Python langcodes call in aioamazondevices http_wrapper.
package locale

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
)

// ForCountry returns the most likely BCP-47 language tag for the given
// ISO-3166 country code. The country code is case-insensitive.
//
// Implementation note: x/text v0.37 does not expose Tag.Maximize publicly,
// but Tag.Base() and Tag.Region() each internally run CLDR's Add Likely
// Subtags algorithm, which is equivalent to maximization for our purposes.
func ForCountry(country string) (string, error) {
	if country == "" {
		return "", fmt.Errorf("locale: empty country code")
	}
	region, err := language.ParseRegion(strings.ToUpper(country))
	if err != nil {
		return "", fmt.Errorf("locale: parse region %q: %w", country, err)
	}
	tag, err := language.Compose(region)
	if err != nil {
		return "", fmt.Errorf("locale: compose %q: %w", country, err)
	}
	base, baseConf := tag.Base()
	reg, regConf := tag.Region()
	if baseConf == language.No || regConf == language.No {
		return "", fmt.Errorf("locale: cannot infer language/region for %q", country)
	}
	return fmt.Sprintf("%s-%s", base.String(), reg.String()), nil
}
