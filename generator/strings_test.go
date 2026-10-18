package generator

import (
	"strings"
	"testing"
)

func TestGenerateStringLengths(t *testing.T) {
	tests := []struct {
		schema   string
		min, max int
	}{
		{`{"type":"string"}`, defaultMinLength, defaultMaxLength},
		{`{"type":"string","minLength":30}`, 30, 30 + lengthRangeSpan},
		{`{"type":"string","maxLength":10}`, defaultMinLength, 10},
		{`{"type":"string","maxLength":3}`, 3, 3},
		{`{"type":"string","maxLength":0}`, 0, 0},
		{`{"type":"string","minLength":8,"maxLength":8}`, 8, 8},
		{`{"type":"string","minLength":0,"maxLength":2}`, 0, 2},
	}
	for _, tt := range tests {
		lengths := map[int]bool{}
		for _, value := range sample(t, tt.schema, 500) {
			text, ok := value.(string)
			if !ok {
				t.Fatalf("%s: value %#v is %T, want string", tt.schema, value, value)
			}
			if len(text) < tt.min || len(text) > tt.max {
				t.Fatalf("%s: len(%q) = %d, want %d..%d", tt.schema, text, len(text), tt.min, tt.max)
			}
			lengths[len(text)] = true
		}
		if len(lengths) != tt.max-tt.min+1 {
			t.Errorf("%s: observed lengths %v, want every length in %d..%d", tt.schema, lengths, tt.min, tt.max)
		}
	}
}

func TestGenerateStringAlphabet(t *testing.T) {
	seen := map[rune]bool{}
	for _, value := range sample(t, `{"type":"string","minLength":50,"maxLength":50}`, 200) {
		for _, char := range value.(string) {
			if !strings.ContainsRune(alphanumeric, char) {
				t.Fatalf("unexpected character %q", char)
			}
			seen[char] = true
		}
	}
	if len(seen) != len(alphanumeric) {
		t.Errorf("observed %d distinct characters, want %d", len(seen), len(alphanumeric))
	}
}

func TestArrayWithoutItemsHoldsStrings(t *testing.T) {
	for _, value := range sample(t, `{"type":"array","minItems":3,"maxItems":3}`, 20) {
		for _, item := range value.([]any) {
			text, ok := item.(string)
			if !ok || len(text) < defaultMinLength || len(text) > defaultMaxLength {
				t.Fatalf("default item = %#v, want alphanumeric string of default length", item)
			}
		}
	}
}

func TestGenerateBoolean(t *testing.T) {
	trues := 0
	const runs = 2000
	for _, value := range sample(t, `{"type":"boolean"}`, runs) {
		flag, ok := value.(bool)
		if !ok {
			t.Fatalf("value %#v is %T, want bool", value, value)
		}
		if flag {
			trues++
		}
	}
	if rate := float64(trues) / runs; rate < 0.45 || rate > 0.55 {
		t.Errorf("true rate = %.3f, want ≈ 0.5", rate)
	}
}
