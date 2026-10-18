package parser

import (
	"errors"
	"reflect"
	"regexp/syntax"
	"testing"
)

func ptr[T any](value T) *T { return &value }

func mustParse[T Node](t *testing.T, schema string) T {
	t.Helper()
	node, err := Parse([]byte(schema))
	if err != nil {
		t.Fatalf("Parse(%s): %v", schema, err)
	}
	typed, ok := node.(T)
	if !ok {
		t.Fatalf("Parse(%s) = %T, want %T", schema, node, *new(T))
	}
	return typed
}

func TestParseString(t *testing.T) {
	node := mustParse[*StringNode](t, `{"type":"string","minLength":2,"maxLength":8.0,"format":"email","pattern":"^[a-z]+$"}`)
	if !reflect.DeepEqual(node.MinLength, ptr(2)) || !reflect.DeepEqual(node.MaxLength, ptr(8)) {
		t.Errorf("lengths = %v/%v, want 2/8", node.MinLength, node.MaxLength)
	}
	if node.Format != "email" || node.PatternSource != "^[a-z]+$" || node.Pattern == nil {
		t.Errorf("format/pattern = %q/%q/%v", node.Format, node.PatternSource, node.Pattern)
	}
	if node.Matcher == nil || node.Matcher.String() != node.PatternSource {
		t.Errorf("Matcher = %v, want %q compiled", node.Matcher, node.PatternSource)
	}
	bounded := mustParse[*StringNode](t, `{"type":"string","pattern":"a{2,6}"}`)
	if re := bounded.Pattern; re.Op != syntax.OpRepeat || re.Min != 2 || re.Max != 6 {
		t.Errorf("pattern AST = %v, want an unsimplified a{2,6} repeat", re)
	}
	if node.Path != "#" {
		t.Errorf("Path = %q, want #", node.Path)
	}
}

func TestParseIntegerBounds(t *testing.T) {
	tests := []struct {
		schema           string
		minimum, maximum *int64
	}{
		{`{"type":"integer"}`, nil, nil},
		{`{"type":"integer","minimum":1.5,"maximum":10.9}`, ptr[int64](2), ptr[int64](10)},
		{`{"type":"integer","minimum":-1.5,"maximum":-0.5}`, ptr[int64](-1), ptr[int64](-1)},
		{`{"type":"integer","exclusiveMinimum":5,"exclusiveMaximum":10}`, ptr[int64](6), ptr[int64](9)},
		{`{"type":"integer","exclusiveMinimum":1.5,"exclusiveMaximum":10.5}`, ptr[int64](2), ptr[int64](10)},
		{`{"type":"integer","minimum":5,"exclusiveMinimum":true,"maximum":10,"exclusiveMaximum":true}`, ptr[int64](6), ptr[int64](9)},
		{`{"type":"integer","minimum":3,"exclusiveMinimum":false}`, ptr[int64](3), nil},
		{`{"type":"integer","exclusiveMinimum":true}`, nil, nil},
		{`{"type":"integer","minimum":7,"exclusiveMinimum":5}`, ptr[int64](7), nil},
		{`{"type":"integer","minimum":5,"exclusiveMinimum":5}`, ptr[int64](6), nil},
		{`{"type":"integer","maximum":3,"exclusiveMaximum":8}`, nil, ptr[int64](3)},
		{`{"type":"integer","maximum":9007199254740993}`, nil, ptr[int64](9007199254740993)},
		{`{"type":"integer","minimum":-1e30,"maximum":1e30}`, nil, nil},
	}
	for _, tt := range tests {
		node := mustParse[*IntegerNode](t, tt.schema)
		if !reflect.DeepEqual(node.Minimum, tt.minimum) || !reflect.DeepEqual(node.Maximum, tt.maximum) {
			t.Errorf("%s: bounds = %v/%v, want %v/%v", tt.schema, deref(node.Minimum), deref(node.Maximum), deref(tt.minimum), deref(tt.maximum))
		}
	}
}

func TestParseIntegerMultipleOf(t *testing.T) {
	node := mustParse[*IntegerNode](t, `{"type":"integer","multipleOf":3.0}`)
	if !reflect.DeepEqual(node.MultipleOf, ptr[int64](3)) {
		t.Errorf("MultipleOf = %v, want 3", deref(node.MultipleOf))
	}
}

func TestParseNumber(t *testing.T) {
	node := mustParse[*NumberNode](t, `{"type":"number","minimum":0.5,"exclusiveMaximum":2.5,"multipleOf":0.25}`)
	want := &NumberNode{
		Common:       Common{Path: "#"},
		Minimum:      ptr(0.5),
		Maximum:      ptr(2.5),
		ExclusiveMax: true,
		MultipleOf:   ptr(0.25),
	}
	if !reflect.DeepEqual(node, want) {
		t.Errorf("node = %+v, want %+v", node, want)
	}
	draft4 := mustParse[*NumberNode](t, `{"type":"number","maximum":3,"exclusiveMaximum":true}`)
	if !draft4.ExclusiveMax || *draft4.Maximum != 3 {
		t.Errorf("draft-04 exclusiveMaximum = %v/%v, want 3/true", *draft4.Maximum, draft4.ExclusiveMax)
	}
}

func TestParseBooleanNullAndUnknownKeywords(t *testing.T) {
	mustParse[*BooleanNode](t, `{"type":"boolean","soy_un_metadato":true}`)
	mustParse[*NullNode](t, `{"type":"null"}`)
	mustParse[*NullNode](t, `{"type":["null"]}`)
}

func TestParseNullable(t *testing.T) {
	for _, schema := range []string{
		`{"type":["string","null"]}`,
		`{"type":["null","string"]}`,
		`{"type":"string","nullable":true}`,
	} {
		if node := mustParse[*StringNode](t, schema); !node.Nullable {
			t.Errorf("%s: Nullable = false, want true", schema)
		}
	}
	if node := mustParse[*StringNode](t, `{"type":["string"]}`); node.Nullable {
		t.Errorf(`["string"]: Nullable = true, want false`)
	}
}

func TestParseEnumAndConst(t *testing.T) {
	node := mustParse[*StringNode](t, `{"type":"string","enum":["a","b"]}`)
	if want := []any{"a", "b"}; !reflect.DeepEqual(node.Enum, want) {
		t.Errorf("Enum = %v, want %v", node.Enum, want)
	}
	literal := mustParse[*LiteralNode](t, `{"enum":["a", 1, 2.5, null, {"k":[true]}]}`)
	want := []any{"a", int64(1), 2.5, nil, map[string]any{"k": []any{true}}}
	if !reflect.DeepEqual(literal.Enum, want) {
		t.Errorf("Enum = %#v, want %#v", literal.Enum, want)
	}
	constNull := mustParse[*LiteralNode](t, `{"const":null}`)
	if !constNull.HasConst || constNull.Const != nil {
		t.Errorf("const null = %v/%v, want true/nil", constNull.HasConst, constNull.Const)
	}
	constInt := mustParse[*IntegerNode](t, `{"type":"integer","const":3}`)
	if !constInt.HasConst || constInt.Const != int64(3) {
		t.Errorf("const 3 = %v/%#v, want true/int64(3)", constInt.HasConst, constInt.Const)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		schema string
		want   error
	}{
		{`[]`, ErrInvalidConstraint},
		{`true`, ErrInvalidConstraint},
		{`{}`, ErrMissingType},
		{`{"minimum":1}`, ErrMissingType},
		{`{"type":"magia"}`, ErrUnknownType},
		{`{"type":5}`, ErrUnknownType},
		{`{"type":[]}`, ErrUnknownType},
		{`{"type":[1]}`, ErrUnknownType},
		{`{"type":["string","integer"]}`, ErrUnknownType},
		{`{"type":"string","nullable":"yes"}`, ErrInvalidConstraint},
		{`{"enum":[]}`, ErrInvalidConstraint},
		{`{"enum":"x"}`, ErrInvalidConstraint},
		{`{"type":"string","minLength":-1}`, ErrInvalidConstraint},
		{`{"type":"string","minLength":1.5}`, ErrInvalidConstraint},
		{`{"type":"string","maxLength":"3"}`, ErrInvalidConstraint},
		{`{"type":"string","maxLength":1e12}`, ErrInvalidConstraint},
		{`{"type":"string","format":5}`, ErrInvalidConstraint},
		{`{"type":"string","pattern":5}`, ErrInvalidConstraint},
		{`{"type":"string","pattern":"(?=x)"}`, ErrInvalidPattern},
		{`{"type":"string","pattern":"[a-"}`, ErrInvalidPattern},
		{`{"type":"integer","minimum":"1"}`, ErrInvalidConstraint},
		{`{"type":"integer","exclusiveMinimum":"1"}`, ErrInvalidConstraint},
		{`{"type":"integer","minimum":1e30}`, ErrInvalidConstraint},
		{`{"type":"integer","maximum":-1e30}`, ErrInvalidConstraint},
		{`{"type":"integer","maximum":1e400}`, ErrInvalidConstraint},
		{`{"type":"integer","multipleOf":1.5}`, ErrInvalidConstraint},
		{`{"type":"integer","multipleOf":0}`, ErrInvalidConstraint},
		{`{"type":"number","multipleOf":-1}`, ErrInvalidConstraint},
		{`{"type":"number","multipleOf":"2"}`, ErrInvalidConstraint},
		{`{"type":"number","maximum":true}`, ErrInvalidConstraint},
		{`{"type":"number","minimum":"0"}`, ErrInvalidConstraint},
		{`{"type":"string","minLength":1e400}`, ErrInvalidConstraint},
	}
	for _, tt := range tests {
		if _, err := Parse([]byte(tt.schema)); !errors.Is(err, tt.want) {
			t.Errorf("Parse(%s) error = %v, want %v", tt.schema, err, tt.want)
		}
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"type":`)); err == nil {
		t.Error("Parse of truncated JSON succeeded, want error")
	}
}

func deref[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
