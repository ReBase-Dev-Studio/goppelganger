package generator

import (
	"errors"
	"math"
	"strconv"
	"testing"
)

func TestGenerateInteger(t *testing.T) {
	tests := []struct {
		schema       string
		lower, upper int64
		multiple     int64
	}{
		{`{"type":"integer"}`, 0, 1000, 1},
		{`{"type":"integer","minimum":1,"maximum":3}`, 1, 3, 1},
		{`{"type":"integer","exclusiveMinimum":1,"exclusiveMaximum":4}`, 2, 3, 1},
		{`{"type":"integer","minimum":5000}`, 5000, 6000, 1},
		{`{"type":"integer","maximum":-50}`, -1050, -50, 1},
		{`{"type":"integer","minimum":10,"maximum":100,"multipleOf":7}`, 14, 98, 7},
		{`{"type":"integer","minimum":-100,"maximum":-10,"multipleOf":7}`, -98, -14, 7},
		{`{"type":"integer","minimum":1,"multipleOf":5000}`, 5000, 5000, 5000},
		{`{"type":"integer","maximum":-1,"multipleOf":5000}`, -5000, -5000, 5000},
		{`{"type":"integer","multipleOf":5000}`, 0, 0, 5000},
		{`{"type":"integer","minimum":9223372036854775800}`, 9223372036854775800, math.MaxInt64, 1},
		{`{"type":"integer","maximum":-9223372036854775800}`, math.MinInt64, -9223372036854775800, 1},
		{`{"type":"integer","minimum":-9223372036854775808,"maximum":9223372036854775807}`, math.MinInt64, math.MaxInt64, 1},
	}
	for _, tt := range tests {
		for _, value := range sample(t, tt.schema, 300) {
			integer, ok := value.(int64)
			if !ok {
				t.Fatalf("%s: value %#v is %T, want int64", tt.schema, value, value)
			}
			if integer < tt.lower || integer > tt.upper || integer%tt.multiple != 0 {
				t.Fatalf("%s: %d outside %d..%d or not a multiple of %d", tt.schema, integer, tt.lower, tt.upper, tt.multiple)
			}
		}
	}
}

func TestGenerateIntegerCoversRange(t *testing.T) {
	seen := map[any]bool{}
	for _, value := range sample(t, `{"type":"integer","minimum":-2,"maximum":2}`, 200) {
		seen[value] = true
	}
	if len(seen) != 5 {
		t.Errorf("observed %v, want every integer in -2..2", seen)
	}
}

func TestGenerateNumber(t *testing.T) {
	tests := []struct {
		schema                     string
		lower, upper               float64
		exclusiveMin, exclusiveMax bool
	}{
		{`{"type":"number"}`, 0, 1000, false, false},
		{`{"type":"number","minimum":-1.5,"maximum":2.5}`, -1.5, 2.5, false, false},
		{`{"type":"number","exclusiveMinimum":0,"exclusiveMaximum":1}`, 0, 1, true, true},
		{`{"type":"number","minimum":2,"maximum":2}`, 2, 2, false, false},
		{`{"type":"number","minimum":1e20}`, 1e20, 2e20, false, false},
		{`{"type":"number","maximum":-10}`, -1010, -10, false, false},
		{`{"type":"number","minimum":-1e308,"maximum":1e308}`, -1e308, 1e308, false, false},
	}
	for _, tt := range tests {
		for _, value := range sample(t, tt.schema, 300) {
			number, ok := value.(float64)
			if !ok {
				t.Fatalf("%s: value %#v is %T, want float64", tt.schema, value, value)
			}
			below := number < tt.lower || (tt.exclusiveMin && number == tt.lower)
			above := number > tt.upper || (tt.exclusiveMax && number == tt.upper)
			if below || above || math.IsInf(number, 0) || math.IsNaN(number) {
				t.Fatalf("%s: %v outside bounds", tt.schema, number)
			}
		}
	}
}

func TestGenerateNumberMultipleOf(t *testing.T) {
	tests := []struct {
		schema string
		want   map[float64]bool
	}{
		{`{"type":"number","minimum":0.3,"maximum":0.7,"multipleOf":0.1}`, set(0.3, 0.4, 0.5, 0.6, 0.7)},
		{`{"type":"number","exclusiveMinimum":0,"exclusiveMaximum":1,"multipleOf":0.25}`, set(0.25, 0.5, 0.75)},
		{`{"type":"number","minimum":10,"multipleOf":1000}`, set(1000)},
		{`{"type":"number","maximum":-10,"multipleOf":1000}`, set(-1000)},
		{`{"type":"number","minimum":10,"multipleOf":5000}`, set(5000)},
		{`{"type":"number","maximum":-10,"multipleOf":5000}`, set(-5000)},
		{`{"type":"number","minimum":-1,"maximum":1,"multipleOf":1}`, set(-1, 0, 1)},
	}
	for _, tt := range tests {
		seen := map[float64]bool{}
		for _, value := range sample(t, tt.schema, 300) {
			number := value.(float64)
			if !tt.want[number] {
				t.Fatalf("%s: %v (%s) not in %v", tt.schema, number, strconv.FormatFloat(number, 'g', -1, 64), tt.want)
			}
			seen[number] = true
		}
		if len(seen) != len(tt.want) {
			t.Errorf("%s: observed %v, want all of %v", tt.schema, seen, tt.want)
		}
	}
}

func TestGenerateNumberUnsatisfiable(t *testing.T) {
	// No float64 lies strictly between 1 and its successor.
	schema := `{"type":"number","exclusiveMinimum":1,"exclusiveMaximum":1.0000000000000002}`
	_, err := newTestWalker(t, 1).Walk(mustParse(t, schema))
	if !errors.Is(err, ErrUnsatisfiable) {
		t.Errorf("error = %v, want ErrUnsatisfiable", err)
	}
}

func TestWalkPropagatesGeneratorErrors(t *testing.T) {
	unsatisfiable := `{"type":"number","exclusiveMinimum":1,"exclusiveMaximum":1.0000000000000002}`
	for _, schema := range []string{
		`{"type":"object","required":["n"],"properties":{"n":` + unsatisfiable + `}}`,
		`{"type":"array","minItems":1,"items":` + unsatisfiable + `}`,
	} {
		if _, err := newTestWalker(t, 1).Walk(mustParse(t, schema)); !errors.Is(err, ErrUnsatisfiable) {
			t.Errorf("%s: error = %v, want ErrUnsatisfiable", schema, err)
		}
	}
}

func TestRoundToMultiple(t *testing.T) {
	tests := []struct{ value, multiple, want float64 }{
		{3 * 0.1, 0.1, 0.3},
		{7 * 0.01, 0.01, 0.07},
		{5, 1, 5},
		{3e-20, 1e-20, 3e-20},
		{1e308, 0.5, 1e308},
	}
	for _, tt := range tests {
		if got := roundToMultiple(tt.value, tt.multiple); got != tt.want {
			t.Errorf("roundToMultiple(%v, %v) = %v, want %v", tt.value, tt.multiple, got, tt.want)
		}
	}
}

func set(values ...float64) map[float64]bool {
	result := make(map[float64]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
