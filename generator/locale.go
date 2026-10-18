package generator

import (
	"fmt"
	"slices"
)

// Locale holds the word lists used by semantic generators (email, hostname,
// uri). Words are lowercase ASCII letters so they are valid hostname labels
// and email local parts as-is.
type Locale struct {
	Code string
	// Words feed user names, hostname labels and uri path segments.
	Words []string
	// TLDs complete "example.<tld>" domains; every entry must keep the
	// domain reserved by RFC 2606 so generated addresses never reach anyone.
	TLDs []string
}

// locales is keyed by Locale.Code. Only looked up, never iterated.
var locales = map[string]Locale{
	"en_US": {
		Code: "en_US",
		Words: []string{
			"alpha", "amber", "anchor", "apple", "arrow", "autumn", "badge", "banner",
			"basket", "beacon", "berry", "birch", "blossom", "bridge", "bright", "brook",
			"cabin", "candle", "canyon", "carbon", "castle", "cedar", "cherry", "cloud",
			"clover", "comet", "copper", "coral", "cotton", "crystal", "delta", "desert",
			"dragon", "eagle", "echo", "ember", "falcon", "feather", "field", "forest",
			"fossil", "garden", "glacier", "golden", "granite", "harbor", "hazel", "horizon",
			"island", "ivory", "jasper", "jungle", "lantern", "lemon", "lotus", "maple",
			"marble", "meadow", "mirror", "mountain", "nectar", "noble", "ocean", "olive",
			"orbit", "orchid", "pebble", "pepper", "pine", "planet", "prairie", "quartz",
			"rapid", "raven", "river", "rocket", "saddle", "silver", "spark", "spruce",
			"stone", "summit", "sunset", "thunder", "timber", "topaz", "tulip", "valley",
			"velvet", "violet", "willow", "winter", "zephyr",
		},
		TLDs: []string{"com", "net", "org"},
	},
}

// LookupLocale returns the locale registered under code. The returned slices
// are copies, so callers may modify them freely.
func LookupLocale(code string) (Locale, error) {
	locale, ok := locales[code]
	if !ok {
		return Locale{}, fmt.Errorf("%w: %q", ErrUnsupportedLocale, code)
	}
	locale.Words = slices.Clone(locale.Words)
	locale.TLDs = slices.Clone(locale.TLDs)
	return locale, nil
}
