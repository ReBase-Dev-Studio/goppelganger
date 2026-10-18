package goppelganger

// DefaultLocale is used when Options.Locale is empty.
const DefaultLocale = "en_US"

// Options configures a Faker instance.
type Options struct {
	// Seed makes generation deterministic. A nil Seed draws a random seed
	// from crypto/rand; any non-nil value (including 0) is used literally.
	Seed *int64

	// Locale selects the word lists used by semantic generators.
	// Empty means DefaultLocale.
	Locale string
}

func (opts Options) localeOrDefault() string {
	if opts.Locale == "" {
		return DefaultLocale
	}
	return opts.Locale
}
