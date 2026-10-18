package generator

import (
	"reflect"
	"testing"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

func newTestWalker(t *testing.T, seed int64) *Walker {
	t.Helper()
	rng, err := NewRand(&seed)
	if err != nil {
		t.Fatalf("NewRand: %v", err)
	}
	locale, err := LookupLocale("en_US")
	if err != nil {
		t.Fatalf("LookupLocale: %v", err)
	}
	return NewWalker(rng, locale)
}

func mustParse(t *testing.T, schema string) parser.Node {
	t.Helper()
	node, err := parser.Parse([]byte(schema))
	if err != nil {
		t.Fatalf("Parse(%s): %v", schema, err)
	}
	return node
}

// sample walks schema n times with one seeded walker.
func sample(t *testing.T, schema string, n int) []any {
	t.Helper()
	node := mustParse(t, schema)
	walker := newTestWalker(t, 7)
	values := make([]any, n)
	for i := range values {
		value, err := walker.Walk(node)
		if err != nil {
			t.Fatalf("Walk(%s): %v", schema, err)
		}
		values[i] = value
	}
	return values
}

func TestWalkConst(t *testing.T) {
	values := sample(t, `{"const":{"tags":["a"]}}`, 2)
	want := map[string]any{"tags": []any{"a"}}
	if !reflect.DeepEqual(values[0], want) {
		t.Fatalf("const = %#v, want %#v", values[0], want)
	}
	values[0].(map[string]any)["tags"].([]any)[0] = "mutated"
	if !reflect.DeepEqual(values[1], want) {
		t.Error("mutating one output leaked into another: const values must be cloned")
	}
	if got := sample(t, `{"type":"string","const":null}`, 1)[0]; got != nil {
		t.Errorf("const null = %#v, want nil", got)
	}
}

func TestWalkEnum(t *testing.T) {
	seen := map[any]int{}
	for _, value := range sample(t, `{"enum":["red", "green", 3, null]}`, 300) {
		seen[value]++
	}
	if len(seen) != 4 {
		t.Errorf("enum produced %v, want all of red/green/3/nil", seen)
	}
	for value := range seen {
		switch value {
		case "red", "green", int64(3), nil:
		default:
			t.Errorf("enum produced unexpected %#v", value)
		}
	}
}

func TestWalkNullable(t *testing.T) {
	nulls := 0
	const runs = 2000
	for _, value := range sample(t, `{"type":["object","null"]}`, runs) {
		if value == nil {
			nulls++
		}
	}
	if rate := float64(nulls) / runs; rate < 0.20 || rate > 0.30 {
		t.Errorf("null rate = %.3f, want ≈ %.2f", rate, nullProbability)
	}
}

func TestWalkObjectRequiredAndOptional(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["id"],
		"properties": {"id": {"const": 1}, "nickname": {"const": "x"}}
	}`
	present := 0
	const runs = 1000
	for _, value := range sample(t, schema, runs) {
		object := value.(map[string]any)
		if object["id"] != int64(1) {
			t.Fatalf("required id missing: %#v", object)
		}
		if _, ok := object["nickname"]; ok {
			present++
		}
	}
	if rate := float64(present) / runs; rate < 0.42 || rate > 0.58 {
		t.Errorf("optional inclusion rate = %.3f, want ≈ %.2f", rate, optionalPropertyProbability)
	}
}

func TestWalkArrayBounds(t *testing.T) {
	tests := []struct {
		schema   string
		min, max int
	}{
		{`{"type":"array","items":{"const":0}}`, defaultMinItems, defaultMaxItems},
		{`{"type":"array","minItems":2,"maxItems":3,"items":{"const":0}}`, 2, 3},
		{`{"type":"array","minItems":3,"items":{"const":0}}`, 3, 3 + itemsRangeSpan},
		{`{"type":"array","maxItems":2,"items":{"const":0}}`, 1, 2},
		{`{"type":"array","maxItems":0,"items":{"const":0}}`, 0, 0},
		{`{"type":"array","minItems":0,"maxItems":0}`, 0, 0},
	}
	for _, tt := range tests {
		lengths := map[int]bool{}
		for _, value := range sample(t, tt.schema, 300) {
			items := value.([]any)
			if len(items) < tt.min || len(items) > tt.max {
				t.Fatalf("%s: len = %d, want %d..%d", tt.schema, len(items), tt.min, tt.max)
			}
			lengths[len(items)] = true
		}
		if len(lengths) != tt.max-tt.min+1 {
			t.Errorf("%s: observed lengths %v, want every length in %d..%d", tt.schema, lengths, tt.min, tt.max)
		}
	}
}

func TestWalkRef(t *testing.T) {
	value := sample(t, `{"$ref":"#/$defs/a","$defs":{"a":{"type":"array","minItems":1,"maxItems":1,"items":{"const":"x"}}}}`, 1)[0]
	if !reflect.DeepEqual(value, []any{"x"}) {
		t.Errorf("ref value = %#v, want [x]", value)
	}
}

func TestWalkRecursiveArrayTerminates(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["children"],
		"properties": {"children": {"type": "array", "minItems": 1, "maxItems": 2, "items": {"$ref": "#"}}}
	}`
	for _, value := range sample(t, schema, 50) {
		for _, child := range value.(map[string]any)["children"].([]any) {
			grandchildren := child.(map[string]any)["children"].([]any)
			if !reflect.DeepEqual(grandchildren, []any{nil}) {
				t.Fatalf("cycle with minItems 1 = %#v, want [nil]", grandchildren)
			}
		}
	}
	unbounded := `{"type":"object","required":["kids"],"properties":{"kids":{"type":"array","items":{"$ref":"#"}}}}`
	child := sample(t, unbounded, 1)[0].(map[string]any)["kids"].([]any)[0].(map[string]any)
	if got := child["kids"]; !reflect.DeepEqual(got, []any{}) {
		t.Errorf("cycle without minItems = %#v, want []", got)
	}
}

func TestWalkRecursiveProperties(t *testing.T) {
	required := sample(t, `{"type":"object","required":["self"],"properties":{"self":{"$ref":"#"}}}`, 1)[0]
	if want := map[string]any{"self": map[string]any{"self": nil}}; !reflect.DeepEqual(required, want) {
		t.Errorf("required cycle = %#v, want %#v", required, want)
	}
	for _, value := range sample(t, `{"type":"object","properties":{"self":{"$ref":"#"}}}`, 100) {
		if nested, ok := value.(map[string]any)["self"]; ok {
			if _, deeper := nested.(map[string]any)["self"]; deeper {
				t.Fatalf("optional cycle must be omitted, got %#v", value)
			}
		}
	}
}

func TestWalkMutualRecursionTerminates(t *testing.T) {
	schema := `{
		"$ref": "#/$defs/a",
		"$defs": {
			"a": {"type": "object", "required": ["b"], "properties": {"b": {"$ref": "#/$defs/b"}}},
			"b": {"type": "object", "required": ["a"], "properties": {"a": {"$ref": "#/$defs/a"}}}
		}
	}`
	want := map[string]any{"b": map[string]any{"a": map[string]any{"b": nil}}}
	if got := sample(t, schema, 1)[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("mutual recursion = %#v, want %#v", got, want)
	}
}

func TestWalkIsDeterministic(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"a": {"enum": [1, 2, 3]},
			"b": {"type": "array", "items": {"enum": ["x", "y"]}},
			"c": {"type": ["object", "null"], "properties": {"d": {"enum": [true, false]}}}
		}
	}`
	node := mustParse(t, schema)
	first, _ := newTestWalker(t, 42).Walk(node)
	second, _ := newTestWalker(t, 42).Walk(node)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("same seed produced %#v and %#v", first, second)
	}
}

func TestBoundsClampToSizeLimit(t *testing.T) {
	huge, nearCap := 1_000_000_000, parser.MaxGeneratedSize-1
	limit := parser.MaxGeneratedSize
	arrays := []struct {
		node     *parser.ArrayNode
		min, max int
	}{
		{&parser.ArrayNode{MaxItems: &huge}, defaultMinItems, limit},
		{&parser.ArrayNode{MinItems: &nearCap}, nearCap, limit},
		{&parser.ArrayNode{MinItems: &nearCap, MaxItems: &huge}, nearCap, limit},
	}
	for _, tt := range arrays {
		if low, high := itemBounds(tt.node); low != tt.min || high != tt.max {
			t.Errorf("itemBounds = %d..%d, want %d..%d", low, high, tt.min, tt.max)
		}
	}
	strings := []struct {
		node     *parser.StringNode
		min, max int
	}{
		{&parser.StringNode{MaxLength: &huge}, defaultMinLength, limit},
		{&parser.StringNode{MinLength: &nearCap}, nearCap, limit},
		{&parser.StringNode{MinLength: &nearCap, MaxLength: &huge}, nearCap, limit},
	}
	for _, tt := range strings {
		if low, high := lengthBounds(tt.node); low != tt.min || high != tt.max {
			t.Errorf("lengthBounds = %d..%d, want %d..%d", low, high, tt.min, tt.max)
		}
	}
}

func TestWalkHugeMaximumsStayWithinLimit(t *testing.T) {
	for _, value := range sample(t, `{"type":"array","minItems":9999,"maxItems":1000000000,"items":{"type":"boolean"}}`, 5) {
		if count := len(value.([]any)); count > parser.MaxGeneratedSize {
			t.Fatalf("array has %d items, above the limit", count)
		}
	}
	for _, value := range sample(t, `{"type":"string","minLength":9990,"maxLength":1000000000}`, 20) {
		if length := len(value.(string)); length > parser.MaxGeneratedSize {
			t.Fatalf("string has %d chars, above the limit", length)
		}
	}
}
