package generator

import (
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	labelPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	earliest      = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	latest        = time.Date(2030, time.December, 31, 23, 59, 59, 0, time.UTC)
)

// formatChecks validate each format by parsing it back with the stdlib.
var formatChecks = map[string]func(string) error{
	"uuid": func(value string) error {
		if !uuidV4Pattern.MatchString(value) {
			return errors.New("not a lowercase RFC 4122 v4 UUID")
		}
		return nil
	},
	"email":     checkEmail,
	"date-time": func(value string) error { return checkInstant(time.RFC3339, value) },
	"date":      func(value string) error { return checkInstant(time.DateOnly, value) },
	"time": func(value string) error {
		_, err := time.Parse("15:04:05Z07:00", value)
		return err
	},
	"uri":      checkURI,
	"ipv4":     checkIPv4,
	"ipv6":     checkIPv6,
	"hostname": checkHostname,
}

func TestGenerateFormatsAreValid(t *testing.T) {
	for _, format := range []string{"uuid", "email", "date-time", "date", "time", "uri", "ipv4", "ipv6", "hostname"} {
		schema := fmt.Sprintf(`{"type":"string","format":%q}`, format)
		distinct := map[string]bool{}
		for _, value := range sample(t, schema, 300) {
			text, ok := value.(string)
			if !ok {
				t.Fatalf("%s: value %#v is %T, want string", format, value, value)
			}
			if err := formatChecks[format](text); err != nil {
				t.Fatalf("%s: %q: %v", format, text, err)
			}
			distinct[text] = true
		}
		if len(distinct) < 250 {
			t.Errorf("%s: only %d distinct values out of 300", format, len(distinct))
		}
	}
}

func TestGenerateFormatIgnoresDefaultLength(t *testing.T) {
	for _, value := range sample(t, `{"type":"string","format":"uuid"}`, 5) {
		if len(value.(string)) != 36 {
			t.Fatalf("uuid %q clipped to default length", value)
		}
	}
}

func TestGenerateFormatHonorsExplicitLength(t *testing.T) {
	for _, value := range sample(t, `{"type":"string","format":"ipv4","maxLength":14}`, 200) {
		if len(value.(string)) > 14 {
			t.Fatalf("ipv4 %q exceeds maxLength 14", value)
		}
	}
}

func TestGenerateFormatUnsatisfiableLength(t *testing.T) {
	for _, schema := range []string{
		`{"type":"string","format":"uuid","maxLength":35}`,
		`{"type":"string","format":"date","minLength":11}`,
	} {
		_, err := newTestWalker(t, 1).Walk(mustParse(t, schema))
		if !errors.Is(err, ErrUnsatisfiable) || !strings.Contains(err.Error(), "at #") {
			t.Errorf("%s: error = %v, want ErrUnsatisfiable with path", schema, err)
		}
	}
}

func TestGenerateUnknownFormatFallsBack(t *testing.T) {
	for _, value := range sample(t, `{"type":"string","format":"credit-card","minLength":8,"maxLength":8}`, 20) {
		text := value.(string)
		if len(text) != 8 || strings.Trim(text, alphanumeric) != "" {
			t.Fatalf("unknown format produced %q, want 8 alphanumeric chars", text)
		}
	}
}

func checkEmail(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return err
	}
	if address.Address != value || address.Name != "" {
		return fmt.Errorf("parsed as %q", address.String())
	}
	_, domain, _ := strings.Cut(value, "@")
	return checkHostname(domain)
}

func checkInstant(layout, value string) error {
	instant, err := time.Parse(layout, value)
	if err != nil {
		return err
	}
	if instant.Before(earliest) || instant.After(latest) {
		return fmt.Errorf("outside %v..%v", earliest, latest)
	}
	return nil
}

func checkURI(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || !parsed.IsAbs() || parsed.String() != value {
		return fmt.Errorf("not an absolute https URI: %#v", parsed)
	}
	return checkHostname(parsed.Host)
}

func checkIPv4(value string) error {
	address, err := netip.ParseAddr(value)
	if err != nil {
		return err
	}
	if !address.Is4() || address.String() != value {
		return errors.New("not a canonical IPv4 address")
	}
	if first := address.As4()[0]; first < 1 || first > 223 {
		return fmt.Errorf("first octet %d outside 1..223", first)
	}
	return nil
}

func checkIPv6(value string) error {
	address, err := netip.ParseAddr(value)
	if err != nil {
		return err
	}
	if !address.Is6() || address.Zone() != "" {
		return errors.New("not an IPv6 address")
	}
	return nil
}

// checkHostname requires an RFC 1123 name under an RFC 2606 reserved domain.
func checkHostname(value string) error {
	if len(value) > 253 {
		return errors.New("longer than 253 characters")
	}
	for _, label := range strings.Split(value, ".") {
		if !labelPattern.MatchString(label) {
			return fmt.Errorf("invalid label %q", label)
		}
	}
	for _, reserved := range []string{".example.com", ".example.net", ".example.org"} {
		if strings.HasSuffix(value, reserved) {
			return nil
		}
	}
	return errors.New("not under an RFC 2606 reserved domain")
}
