package generator

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

const (
	requiredSelfCycle = `{"$defs":{"n":{"type":"object","properties":{"next":{"$ref":"#/$defs/n"}},"required":["next"]}},
		"$ref":"#/$defs/n"}`
	nestedHugeArrays = `{"type":"array","minItems":10000,"items":{"type":"array","minItems":10000,
		"items":{"type":"string","minLength":10}}}`
)

func TestMinCost(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		want   int
	}{
		{"integer", `{"type":"integer"}`, 1},
		{"boolean", `{"type":"boolean"}`, 1},
		{"plain string", `{"type":"string"}`, 1},
		{"string minLength", `{"type":"string","minLength":7}`, 8},
		{"unknown format", `{"type":"string","format":"color","minLength":3}`, 4},
		{"uuid", `{"type":"string","format":"uuid"}`, 37},
		{"uuid capped by maxLength", `{"type":"string","format":"uuid","maxLength":10}`, 11},
		{"pattern bounded repeat", `{"type":"string","pattern":"^[a-z]{2,6}$"}`, 3},
		{"pattern open repeat", `{"type":"string","pattern":"^ab+$"}`, 3},
		{"pattern minLength", `{"type":"string","pattern":"^[a-z]+$","minLength":5}`, 6},
		{"pattern alternation", `{"type":"string","pattern":"^(ab|cde)$"}`, 3},
		{"const", `{"const":{"a":"xy","b":[1,2]}}`, 7},
		{"cheapest enum", `{"enum":["long string",1]}`, 1},
		{"const beats type", `{"type":"string","minLength":50,"const":"ab"}`, 3},
		{"nullable", `{"type":["string","null"],"minLength":50}`, 1},
		{"required only", `{"type":"object","properties":{"a":{"type":"string","minLength":4},"b":{"type":"integer"}},
			"required":["a"]}`, 6},
		{"array without minItems", `{"type":"array","items":{"type":"string","minLength":100}}`, 1},
		{"array minItems", `{"type":"array","minItems":3,"items":{"type":"string","minLength":2}}`, 10},
		{"array default items", `{"type":"array","minItems":2}`, 3},
		{"required self cycle", requiredSelfCycle, 3},
		{"array of itself", `{"$defs":{"t":{"type":"array","minItems":2,"items":{"$ref":"#/$defs/t"}}},
			"$ref":"#/$defs/t"}`, 7},
		{"optional recursion is free", `{"$defs":{"n":{"type":"object","properties":{"next":{"$ref":"#/$defs/n"}}}},
			"$ref":"#/$defs/n"}`, 1},
		{"saturates", nestedHugeArrays, MaxGenerationCost + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			walker := newTestWalker(t, 1)
			got := walker.minCost(mustParse(t, tc.schema), nil)
			if got.cut || got.cost != tc.want {
				t.Fatalf("minCost = %+v, want cost %d", got, tc.want)
			}
		})
	}
}

func TestMinCostSaturatesAtWalkerBudget(t *testing.T) {
	walker := newTestWalker(t, 1)
	walker.budget = 10
	got := walker.minCost(mustParse(t, `{"type":"string","minLength":50}`), nil)
	if got.cost != 11 {
		t.Fatalf("minCost = %d, want budget+1 = 11", got.cost)
	}
}

// Schemas whose output size is fixed must cost exactly their minimum.
func TestMinCostMatchesFixedSizeOutput(t *testing.T) {
	schemas := []string{
		requiredSelfCycle,
		`{"const":{"a":"xy","b":[1,"z"]}}`,
		`{"type":"object","properties":{"s":{"type":"string","minLength":4,"maxLength":4},"i":{"type":"integer"}},
			"required":["s","i"]}`,
		`{"type":"array","minItems":3,"maxItems":3,"items":{"type":"string","format":"uuid"}}`,
		`{"type":"string","pattern":"^[a-z]{3}-(ab|cd)$"}`,
	}
	for _, schema := range schemas {
		node := mustParse(t, schema)
		walker := newTestWalker(t, 3)
		value, err := walker.Walk(node)
		if err != nil {
			t.Fatalf("Walk(%s): %v", schema, err)
		}
		if got, want := valueCost(value), walker.minCost(node, nil).cost; got != want {
			t.Errorf("%s: output cost %d, minCost %d", schema, got, want)
		}
	}
}

func TestFormatMaxLengthBoundsOutput(t *testing.T) {
	walker := newTestWalker(t, 5)
	formats := make([]string, 0, len(formatGenerators))
	for format := range formatGenerators {
		formats = append(formats, format)
	}
	slices.Sort(formats)
	for _, format := range formats {
		bound, ok := walker.formatMaxLength(format)
		if !ok {
			t.Fatalf("formatMaxLength(%q) is missing", format)
		}
		for range 2000 {
			if value := formatGenerators[format](walker); utf8.RuneCountInString(value) > bound {
				t.Fatalf("%s: %q is longer than %d", format, value, bound)
			}
		}
	}
	if _, ok := walker.formatMaxLength("color"); ok {
		t.Fatal("unknown format must report ok = false")
	}
}

func TestPushRefInternsChains(t *testing.T) {
	walker := newTestWalker(t, 1)
	first, second := &parser.RefNode{}, &parser.RefNode{}
	chain := walker.pushRef(walker.pushRef(nil, first), second)
	if again := walker.pushRef(walker.pushRef(nil, first), second); again != chain {
		t.Fatal("equal chains must share a pointer")
	}
	if !chain.contains(first) || !chain.contains(second) || walker.pushRef(nil, second).contains(first) {
		t.Fatal("contains reports the wrong refs")
	}
}

func TestCapArithmetic(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"add within", capAdd(4, 6, 10), 10},
		{"add over", capAdd(5, 6, 10), 11},
		{"add saturated operand", capAdd(11, 0, 10), 11},
		{"mul within", capMul(2, 5, 10), 10},
		{"mul over", capMul(3, 4, 10), 11},
		{"mul zero", capMul(0, 11, 10), 0},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func newBudgetWalker(t *testing.T, seed int64, budget int) *Walker {
	t.Helper()
	walker := newTestWalker(t, seed)
	walker.budget = budget
	return walker
}

// walkWithBudget walks schema once per seed and fails on errors or on an
// output costing more than budget.
func walkWithBudget(t *testing.T, schema string, budget int, seeds int) []any {
	t.Helper()
	node := mustParse(t, schema)
	values := make([]any, seeds)
	for seed := range seeds {
		value, err := newBudgetWalker(t, int64(seed), budget).Walk(node)
		if err != nil {
			t.Fatalf("seed %d: Walk: %v", seed, err)
		}
		if cost := valueCost(value); cost > budget {
			t.Fatalf("seed %d: output costs %d, above the budget of %d", seed, cost, budget)
		}
		values[seed] = value
	}
	return values
}

func TestWalkRejectsMandatoryOutputOverBudget(t *testing.T) {
	walker, fresh := newTestWalker(t, 11), newTestWalker(t, 11)
	_, err := walker.Walk(mustParse(t, nestedHugeArrays))
	if !errors.Is(err, parser.ErrSizeLimitExceeded) {
		t.Fatalf("Walk error = %v, want ErrSizeLimitExceeded", err)
	}
	if walker.rng.Uint64() != fresh.rng.Uint64() {
		t.Fatal("rejection must happen before any random draw")
	}
}

func TestWalkAcceptsMandatoryOutputAtBudget(t *testing.T) {
	schema := `{"type":"array","minItems":3,"maxItems":3,"items":{"type":"string","minLength":2,"maxLength":2}}`
	walkWithBudget(t, schema, 10, 5)
	if _, err := newBudgetWalker(t, 1, 9).Walk(mustParse(t, schema)); !errors.Is(err, parser.ErrSizeLimitExceeded) {
		t.Fatalf("budget 9: Walk error = %v, want ErrSizeLimitExceeded", err)
	}
}

// exponentialSchema chains depth definitions through default-sized arrays:
// without a budget the output grows like 5^depth.
func exponentialSchema(depth int) string {
	var defs strings.Builder
	for level := range depth {
		if level > 0 {
			defs.WriteString(",")
		}
		next := `"next":{"type":"array","minItems":1,"items":{"$ref":"#/$defs/d` + strconv.Itoa(level+1) + `"}}`
		required := `["v","next"]`
		if level == depth-1 {
			next, required = `"next":{"type":"integer"}`, `["v"]`
		}
		defs.WriteString(`"d` + strconv.Itoa(level) + `":{"type":"object","properties":{"v":{"type":"string"},` +
			next + `},"required":` + required + `}`)
	}
	return `{"$defs":{` + defs.String() + `},"$ref":"#/$defs/d0"}`
}

func TestWalkExponentialSchemaStaysWithinBudget(t *testing.T) {
	for _, value := range walkWithBudget(t, exponentialSchema(12), 5_000, 20) {
		checkExponentialLevel(t, value, 0, 12)
	}
}

// checkExponentialLevel asserts required properties and minItems survive
// the degradation at every level.
func checkExponentialLevel(t *testing.T, value any, level, depth int) {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("level %d: got %T, want object", level, value)
	}
	if _, ok := object["v"].(string); !ok {
		t.Fatalf("level %d: missing required v", level)
	}
	if level == depth-1 {
		return
	}
	items, ok := object["next"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("level %d: next = %v, want at least one item", level, object["next"])
	}
	for _, item := range items {
		checkExponentialLevel(t, item, level+1, depth)
	}
}

func TestWalkBudgetDegradation(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		budget int
		check  func(value any) bool
	}{
		{
			"strings shrink but keep minLength",
			`{"type":"array","minItems":3,"maxItems":3,"items":{"type":"string","minLength":2,"maxLength":100}}`, 20,
			func(value any) bool {
				return everyItem(value, func(item any) bool { return utf8.RuneCountInString(item.(string)) >= 2 })
			},
		},
		{
			"enum keeps affordable values",
			`{"type":"array","minItems":4,"maxItems":4,"items":{"enum":["xxxxxxxxxx",1]}}`, 5,
			func(value any) bool { return everyItem(value, func(item any) bool { return item == int64(1) }) },
		},
		{
			"nullable falls back to null",
			`{"type":"array","minItems":3,"maxItems":3,"items":{"type":["string","null"],"minLength":10}}`, 4,
			func(value any) bool { return everyItem(value, func(item any) bool { return item == nil }) },
		},
		{
			"unaffordable optional is omitted",
			`{"type":"object","properties":{"a":{"type":"string","minLength":10},"b":{"type":"integer"}},"required":["b"]}`, 5,
			func(value any) bool {
				_, hasA := value.(map[string]any)["a"]
				_, hasB := value.(map[string]any)["b"]
				return !hasA && hasB
			},
		},
		{
			"open patterns shrink to allowance",
			`{"type":"array","minItems":3,"maxItems":3,"items":{"type":"string","pattern":"^[a-z]+$"}}`, 10,
			func(value any) bool {
				return everyItem(value, func(item any) bool { return utf8.RuneCountInString(item.(string)) >= 1 })
			},
		},
		{
			"stretched pattern fits its minimum",
			`{"type":"array","minItems":1,"maxItems":1,"items":{"type":"string","pattern":"^[a-z]+$","minLength":5}}`, 7,
			func(value any) bool {
				return everyItem(value, func(item any) bool { return utf8.RuneCountInString(item.(string)) == 5 })
			},
		},
		{
			"arrays shrink but keep minItems",
			`{"type":"array","minItems":2,"maxItems":50,"items":{"type":"integer"}}`, 4,
			func(value any) bool { return len(value.([]any)) >= 2 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for seed, value := range walkWithBudget(t, tc.schema, tc.budget, 50) {
				if !tc.check(value) {
					t.Fatalf("seed %d: unexpected value %v", seed, value)
				}
			}
		})
	}
}

// A target in a length gap overshoots; under a tight budget that is an
// ErrUnsatisfiable, never an output above the allowance.
func TestWalkGapPatternUnderTightBudget(t *testing.T) {
	node := mustParse(t, `{"type":"string","pattern":"^(ab)+$","minLength":3}`)
	if _, err := newBudgetWalker(t, 1, 4).Walk(node); !errors.Is(err, ErrUnsatisfiable) {
		t.Fatalf("Walk error = %v, want ErrUnsatisfiable", err)
	}
	value, err := newBudgetWalker(t, 1, 5).Walk(node)
	if err != nil || value != "abab" {
		t.Fatalf("Walk = %v, %v; want abab", value, err)
	}
}

func everyItem(value any, check func(any) bool) bool {
	for _, item := range value.([]any) {
		if !check(item) {
			return false
		}
	}
	return true
}
