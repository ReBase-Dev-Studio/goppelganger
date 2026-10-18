package generator

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

func patternSchema(pattern, extra string) string {
	quoted, _ := json.Marshal(pattern)
	return fmt.Sprintf(`{"type":"string","pattern":%s%s}`, quoted, extra)
}

func TestGeneratePatternMatches(t *testing.T) {
	patterns := []string{
		`^[a-z]{3}-[0-9]{4}$`,
		`^\d{5}(-\d{4})?$`,
		`^(foo|bar|baz)+$`,
		`abc`,
		`^[^a-z]{4}$`,
		`^\w+@\w+\.com$`,
		`(?i)^hello$`,
		`^.{3}$`,
		`(?s)^.{3}$`,
		`^\p{Greek}{3}$`,
		`^[\x{1F600}-\x{1F64F}]{2}$`,
		`^$`,
		`^a*b+c?$`,
		`^x{2,}y{0,3}$`,
		`\bword\b`,
		`^[A-Z][a-z]+(?: [A-Z][a-z]+)*$`,
	}
	for _, pattern := range patterns {
		compiled := regexp.MustCompile(pattern)
		for _, value := range sample(t, patternSchema(pattern, ""), 200) {
			text, ok := value.(string)
			if !ok || !utf8.ValidString(text) || !compiled.MatchString(text) {
				t.Fatalf("%s: generated %#v, want a matching UTF-8 string", pattern, value)
			}
		}
	}
}

func TestGeneratePatternVaries(t *testing.T) {
	distinct := map[string]bool{}
	for _, value := range sample(t, patternSchema(`^[a-z]{8}$`, ""), 100) {
		distinct[value.(string)] = true
	}
	if len(distinct) < 95 {
		t.Errorf("only %d distinct values out of 100", len(distinct))
	}
}

func TestGeneratePatternPrefersPrintableASCII(t *testing.T) {
	for _, value := range sample(t, patternSchema(`^[^0-9]{20}$`, ""), 50) {
		for _, char := range value.(string) {
			if char < 0x20 || char > 0x7e {
				t.Fatalf("negated class produced %q, want printable ASCII", char)
			}
		}
	}
}

func TestGeneratePatternOpenRepeatIsBounded(t *testing.T) {
	for _, value := range sample(t, patternSchema(`^a+$`, ""), 300) {
		if length := len(value.(string)); length < 1 || length > 1+openRepeatSpan {
			t.Fatalf("a+ produced length %d, want 1..%d", length, 1+openRepeatSpan)
		}
	}
}

func TestGeneratePatternIgnoresDefaultLength(t *testing.T) {
	for _, value := range sample(t, patternSchema(`^[a-z]{30}$`, ""), 5) {
		if len(value.(string)) != 30 {
			t.Fatalf("pattern value %q clipped to default length", value)
		}
	}
}

func TestGeneratePatternHonorsExplicitLength(t *testing.T) {
	for _, value := range sample(t, patternSchema(`^[a-z]{2,6}$`, `,"maxLength":4`), 200) {
		if length := len(value.(string)); length > 4 {
			t.Fatalf("length %d exceeds maxLength 4", length)
		}
	}
}

func TestGeneratePatternBeatsFormat(t *testing.T) {
	for _, value := range sample(t, patternSchema(`^[0-9]{3}$`, `,"format":"email"`), 20) {
		if text := value.(string); len(text) != 3 || strings.Trim(text, "0123456789") != "" {
			t.Fatalf("pattern+format produced %q, want 3 digits", text)
		}
	}
}

func TestGeneratePatternUnsatisfiable(t *testing.T) {
	for _, schema := range []string{
		patternSchema(`^[a-z]{5}$`, `,"maxLength":4`),
		patternSchema(`^[a-z]{3}$`, `,"minLength":4`),
		patternSchema(`a^b`, ""),
		patternSchema(`^(ab)+$`, `,"minLength":3,"maxLength":3`),
		patternSchema(`[^\x00-\x{10FFFF}]`, ""),
	} {
		_, err := newTestWalker(t, 1).Walk(mustParse(t, schema))
		if !errors.Is(err, ErrUnsatisfiable) || !strings.Contains(err.Error(), "at #") {
			t.Errorf("%s: error = %v, want ErrUnsatisfiable with path", schema, err)
		}
	}
}

func TestPatternLengths(t *testing.T) {
	cases := []struct {
		pattern string
		want    lengthRange
	}{
		{`abc`, lengthRange{min: 3, natural: 3, max: 3}},
		{`^[a-z].$`, lengthRange{min: 2, natural: 2, max: 2}},
		{`\bx\b`, lengthRange{min: 1, natural: 1, max: 1}},
		{`(ab|cde)`, lengthRange{min: 2, natural: 3, max: 3}},
		{`a?`, lengthRange{min: 0, natural: 1, max: 1}},
		{`[a-z]{2,6}`, lengthRange{min: 2, natural: 6, max: 6}},
		{`a*`, lengthRange{min: 0, natural: openRepeatSpan, max: MaxGenerationCost + 1}},
		{`(ab)+`, lengthRange{min: 2, natural: 2 * (1 + openRepeatSpan), max: MaxGenerationCost + 1}},
		{`a{3,}`, lengthRange{min: 3, natural: 3 + openRepeatSpan, max: MaxGenerationCost + 1}},
		{`(?:\b)*x`, lengthRange{min: 1, natural: 1, max: 1}},
	}
	for _, tc := range cases {
		re, err := syntax.Parse(tc.pattern, syntax.Perl)
		if err != nil {
			t.Fatalf("Parse(%s): %v", tc.pattern, err)
		}
		if got := newTestWalker(t, 1).patternLengths(re); got != tc.want {
			t.Errorf("patternLengths(%s) = %+v, want %+v", tc.pattern, got, tc.want)
		}
	}
	saturated := lengthRange{min: 11, natural: 11, max: 11}
	re, _ := syntax.Parse(`(a{6}){2}b`, syntax.Perl)
	if got := newBudgetWalker(t, 1, 10).patternLengths(re); got != saturated {
		t.Errorf("patternLengths under budget 10 = %+v, want %+v", got, saturated)
	}
}

func TestWriteRegexpHitsTarget(t *testing.T) {
	patterns := []string{`^[a-z]{2,6}$`, `^(ab|cde)+x?$`, `^\d{3}-\d{4}$`, `.*`, `^(a{2,}b?){1,3}$`, `^a*b+c?$`}
	walker := newTestWalker(t, 9)
	for _, pattern := range patterns {
		node := mustParse(t, patternSchema(pattern, "")).(*parser.StringNode)
		lengths := walker.patternLengths(node.Pattern)
		for target := lengths.min; target <= lengths.natural+openRepeatSpan; target++ {
			var builder strings.Builder
			if err := walker.writeRegexp(&builder, node.Pattern, target); err != nil {
				t.Fatalf("%s: %v", pattern, err)
			}
			want := min(target, lengths.max)
			if length := utf8.RuneCountInString(builder.String()); length != want {
				t.Fatalf("%s: target %d produced %q (length %d), want length %d", pattern, target, builder.String(), length, want)
			}
		}
	}
}

func TestGeneratePatternMeetsLengthBounds(t *testing.T) {
	cases := []struct {
		pattern, extra  string
		lowest, highest int
	}{
		{`^[a-z]{2,6}$`, `,"minLength":3`, 3, 6},
		{`^[a-z]+$`, `,"minLength":15`, 15, 15 + openRepeatSpan},
		{`^[a-z]+$`, `,"minLength":40,"maxLength":40`, 40, 40},
		{`^(ab|cde)+$`, `,"minLength":7,"maxLength":9`, 7, 9},
		{`^[A-Z][a-z]*$`, `,"maxLength":3`, 1, 3},
	}
	for _, tc := range cases {
		compiled := regexp.MustCompile(tc.pattern)
		for _, value := range sample(t, patternSchema(tc.pattern, tc.extra), 500) {
			text := value.(string)
			if length := utf8.RuneCountInString(text); length < tc.lowest || length > tc.highest || !compiled.MatchString(text) {
				t.Fatalf("%s%s: generated %q, want a match of length %d..%d", tc.pattern, tc.extra, text, tc.lowest, tc.highest)
			}
		}
	}
}

func TestGeneratePatternLengthIsUniform(t *testing.T) {
	counts := map[int]int{}
	for _, value := range sample(t, patternSchema(`^[a-z]{3,8}$`, ""), 600) {
		counts[len(value.(string))]++
	}
	for length := 3; length <= 8; length++ {
		if counts[length] < 60 {
			t.Errorf("length %d drawn %d times out of 600, want about 100", length, counts[length])
		}
	}
}

func TestGeneratePatternEmptyRangeDrawsNothing(t *testing.T) {
	walker, fresh := newTestWalker(t, 3), newTestWalker(t, 3)
	_, err := walker.Walk(mustParse(t, patternSchema(`^[a-z]{3}$`, `,"minLength":4`)))
	if !errors.Is(err, ErrUnsatisfiable) {
		t.Fatalf("error = %v, want ErrUnsatisfiable", err)
	}
	if walker.rng.Uint64() != fresh.rng.Uint64() {
		t.Fatal("an empty length range must fail before any random draw")
	}
}

func TestIntersectRanges(t *testing.T) {
	got := intersectRanges([]rune{'a', 'z', 0x100, 0x200}, printableASCII)
	if want := []rune{'a', 'z'}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("intersectRanges = %v, want %v", got, want)
	}
	if got := intersectRanges([]rune{0x100, 0x200}, printableASCII); got != nil {
		t.Errorf("disjoint ranges = %v, want nil", got)
	}
}
