package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateContradictions(t *testing.T) {
	invalid := []string{
		`{"type":"string","minLength":5,"maxLength":3}`,
		`{"type":"array","minItems":4,"maxItems":2}`,
		`{"type":"integer","minimum":10,"maximum":1}`,
		`{"type":"integer","exclusiveMinimum":5,"exclusiveMaximum":6}`,
		`{"type":"integer","minimum":1.2,"maximum":1.8}`,
		`{"type":"integer","minimum":11,"maximum":19,"multipleOf":10}`,
		`{"type":"integer","minimum":-19,"maximum":-11,"multipleOf":10}`,
		`{"type":"integer","minimum":9223372036854775801,"multipleOf":10}`,
		`{"type":"integer","maximum":-9223372036854775801,"multipleOf":10}`,
		`{"type":"number","minimum":2,"maximum":1}`,
		`{"type":"number","minimum":1,"exclusiveMaximum":1}`,
		`{"type":"number","exclusiveMinimum":1,"maximum":1}`,
		`{"type":"number","minimum":0.6,"maximum":0.9,"multipleOf":0.5}`,
		`{"type":"number","exclusiveMinimum":0.5,"exclusiveMaximum":1,"multipleOf":0.5}`,
		`{"type":"number","exclusiveMinimum":0.3,"exclusiveMaximum":0.4,"multipleOf":0.1}`,
	}
	for _, schema := range invalid {
		if _, err := Parse([]byte(schema)); !errors.Is(err, ErrInvalidConstraint) {
			t.Errorf("Parse(%s) error = %v, want ErrInvalidConstraint", schema, err)
		}
	}
}

func TestValidateSizeLimit(t *testing.T) {
	for _, schema := range []string{
		`{"type":"array","minItems":10001}`,
		`{"type":"string","minLength":10001,"maxLength":20000}`,
		`{"type":"object","properties":{"deep":{"type":"array","items":{"type":"string","minLength":1000000}}}}`,
	} {
		_, err := Parse([]byte(schema))
		if !errors.Is(err, ErrSizeLimitExceeded) || !strings.Contains(err.Error(), " at #") {
			t.Errorf("Parse(%s) error = %v, want ErrSizeLimitExceeded with path", schema, err)
		}
	}
	for _, schema := range []string{
		`{"type":"array","minItems":10000}`,
		`{"type":"string","minLength":10000}`,
		`{"type":"string","maxLength":1000000000}`,
	} {
		if _, err := Parse([]byte(schema)); err != nil {
			t.Errorf("Parse(%s) error = %v, want nil", schema, err)
		}
	}
}

func TestValidateAcceptsTightRanges(t *testing.T) {
	valid := []string{
		`{"type":"string","minLength":3,"maxLength":3}`,
		`{"type":"array","minItems":2,"maxItems":2}`,
		`{"type":"integer","minimum":5,"maximum":5}`,
		`{"type":"integer","minimum":11,"maximum":20,"multipleOf":10}`,
		`{"type":"integer","minimum":-20,"maximum":-11,"multipleOf":10}`,
		`{"type":"integer","minimum":9223372036854775800,"multipleOf":10}`,
		`{"type":"integer","multipleOf":7}`,
		`{"type":"number","minimum":1,"maximum":1}`,
		`{"type":"number","minimum":0.3,"maximum":0.3,"multipleOf":0.1}`,
		`{"type":"number","minimum":0.7,"maximum":0.7,"multipleOf":0.1}`,
		`{"type":"number","exclusiveMinimum":0.5,"maximum":1,"multipleOf":0.5}`,
		`{"type":"number","minimum":0.5,"exclusiveMaximum":1,"multipleOf":0.5}`,
		`{"type":"number","minimum":10,"multipleOf":1000}`,
	}
	for _, schema := range valid {
		if _, err := Parse([]byte(schema)); err != nil {
			t.Errorf("Parse(%s) error = %v, want nil", schema, err)
		}
	}
}
