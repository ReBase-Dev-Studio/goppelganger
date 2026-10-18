package generator

import (
	"fmt"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

const (
	// maxFormatAttempts bounds retries when a format value violates
	// minLength/maxLength.
	maxFormatAttempts = 10
	// maxEmailSuffix caps the numeric suffix of email local parts.
	maxEmailSuffix = 999
)

// Fixed instants instead of time.Now keep seeded output reproducible.
var (
	earliestInstant = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	latestInstant   = time.Date(2030, time.December, 31, 23, 59, 59, 0, time.UTC).Unix()
)

// formatGenerator produces one value of a `format`.
type formatGenerator func(w *Walker) string

// formatGenerators is keyed by the `format` keyword. Only looked up, never
// iterated; unknown formats fall back to alphanumeric strings.
var formatGenerators = map[string]formatGenerator{
	"uuid":      (*Walker).generateUUID,
	"email":     (*Walker).generateEmail,
	"date-time": (*Walker).generateDateTime,
	"date":      (*Walker).generateDate,
	"time":      (*Walker).generateTime,
	"uri":       (*Walker).generateURI,
	"ipv4":      (*Walker).generateIPv4,
	"ipv6":      (*Walker).generateIPv6,
	"hostname":  (*Walker).generateHostname,
}

// generateFormat retries generate until the value honors the explicit
// minLength/maxLength; defaulted lengths never apply to formats.
func (w *Walker) generateFormat(node *parser.StringNode, generate formatGenerator) (any, error) {
	for range maxFormatAttempts {
		value := generate(w)
		if withinExplicitLength(node, value) {
			return value, nil
		}
	}
	return nil, fmt.Errorf("%w at %s: format %q never fit the length bounds", ErrUnsatisfiable, node.Path, node.Format)
}

// withinExplicitLength counts code points, as JSON Schema does.
func withinExplicitLength(node *parser.StringNode, value string) bool {
	length := utf8.RuneCountInString(value)
	if node.MinLength != nil && length < *node.MinLength {
		return false
	}
	return node.MaxLength == nil || length <= *node.MaxLength
}

// generateUUID returns an RFC 4122 version 4 UUID.
func (w *Walker) generateUUID() string {
	bytes := w.randomBytes(16)
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// generateEmail returns word.word<N>@word.example.<tld> (RFC 2606 reserved).
func (w *Walker) generateEmail() string {
	return fmt.Sprintf("%s.%s%d@%s.example.%s",
		w.pickWord(), w.pickWord(), w.rng.IntN(maxEmailSuffix+1), w.pickWord(), w.pickTLD())
}

func (w *Walker) generateDateTime() string {
	return w.randomInstant().Format(time.RFC3339)
}

func (w *Walker) generateDate() string {
	return w.randomInstant().Format(time.DateOnly)
}

// generateTime returns an RFC 3339 full-time in UTC (HH:MM:SSZ).
func (w *Walker) generateTime() string {
	return w.randomInstant().Format("15:04:05Z07:00")
}

func (w *Walker) generateURI() string {
	return fmt.Sprintf("https://%s/%s/%s", w.generateHostname(), w.pickWord(), w.pickWord())
}

// generateIPv4 keeps the first octet in 1–223 (no "this network",
// multicast or reserved class E addresses).
func (w *Walker) generateIPv4() string {
	octets := w.randomBytes(4)
	octets[0] = byte(1 + w.rng.IntN(223))
	return netip.AddrFrom4([4]byte(octets)).String()
}

func (w *Walker) generateIPv6() string {
	return netip.AddrFrom16([16]byte(w.randomBytes(16))).String()
}

// generateHostname returns word-word.example.<tld> (RFC 2606 reserved).
func (w *Walker) generateHostname() string {
	return fmt.Sprintf("%s-%s.example.%s", w.pickWord(), w.pickWord(), w.pickTLD())
}

func (w *Walker) randomInstant() time.Time {
	return time.Unix(earliestInstant+w.rng.Int64N(latestInstant-earliestInstant+1), 0).UTC()
}

func (w *Walker) randomBytes(count int) []byte {
	bytes := make([]byte, count)
	for i := range bytes {
		bytes[i] = byte(w.rng.UintN(256))
	}
	return bytes
}

func (w *Walker) pickWord() string {
	return w.locale.Words[w.rng.IntN(len(w.locale.Words))]
}

func (w *Walker) pickTLD() string {
	return w.locale.TLDs[w.rng.IntN(len(w.locale.TLDs))]
}
