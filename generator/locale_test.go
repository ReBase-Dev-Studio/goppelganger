package generator

import (
	"errors"
	"testing"
)

func TestLookupLocaleEnUS(t *testing.T) {
	locale, err := LookupLocale("en_US")
	if err != nil {
		t.Fatalf("LookupLocale: %v", err)
	}
	if locale.Code != "en_US" {
		t.Errorf("Code = %q, want en_US", locale.Code)
	}
	if len(locale.Words) == 0 || len(locale.TLDs) == 0 {
		t.Fatalf("empty word lists: %d words, %d TLDs", len(locale.Words), len(locale.TLDs))
	}
	seen := map[string]bool{}
	for _, word := range locale.Words {
		if !isLowerASCIILabel(word) {
			t.Errorf("word %q is not a 1–63 char lowercase ASCII label", word)
		}
		if seen[word] {
			t.Errorf("duplicate word %q", word)
		}
		seen[word] = true
	}
	reserved := map[string]bool{"com": true, "net": true, "org": true}
	for _, tld := range locale.TLDs {
		if !reserved[tld] {
			t.Errorf("TLD %q does not keep example.<tld> reserved by RFC 2606", tld)
		}
	}
}

func TestLookupLocaleReturnsCopies(t *testing.T) {
	first, _ := LookupLocale("en_US")
	original := first.Words[0]
	first.Words[0] = "mutated"
	first.TLDs[0] = "mutated"
	second, _ := LookupLocale("en_US")
	if second.Words[0] != original || second.TLDs[0] == "mutated" {
		t.Error("mutating a returned Locale changed the registry")
	}
}

func TestLookupLocaleUnsupported(t *testing.T) {
	for _, code := range []string{"", "es_MX", "en-US", "EN_US"} {
		if _, err := LookupLocale(code); !errors.Is(err, ErrUnsupportedLocale) {
			t.Errorf("LookupLocale(%q) error = %v, want ErrUnsupportedLocale", code, err)
		}
	}
}

func isLowerASCIILabel(word string) bool {
	if len(word) == 0 || len(word) > 63 {
		return false
	}
	for _, char := range word {
		if char < 'a' || char > 'z' {
			return false
		}
	}
	return true
}
