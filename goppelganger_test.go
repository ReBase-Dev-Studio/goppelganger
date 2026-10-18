package goppelganger

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/generator"
)

func seeded(seed int64) *Faker {
	return New(Options{Seed: &seed})
}

func TestGenerateReturnsObject(t *testing.T) {
	object, err := seeded(1).Generate([]byte(`{
		"type": "object",
		"required": ["id", "name"],
		"properties": {"id": {"type": "integer"}, "name": {"type": "string"}}
	}`))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, ok := object["id"].(int64); !ok {
		t.Errorf("id = %#v, want int64", object["id"])
	}
	if _, ok := object["name"].(string); !ok {
		t.Errorf("name = %#v, want string", object["name"])
	}
}

func TestGenerateResolvesRootRef(t *testing.T) {
	object, err := seeded(1).Generate([]byte(`{
		"$ref": "#/$defs/user",
		"$defs": {"user": {"type": "object", "required": ["id"], "properties": {"id": {"type": "boolean"}}}}
	}`))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, ok := object["id"].(bool); !ok {
		t.Errorf("id = %#v, want bool", object["id"])
	}
}

func TestGenerateRejectsNonObjectRoot(t *testing.T) {
	for _, schema := range []string{
		`{"type":"string"}`,
		`{"type":"array"}`,
		`{"enum":[{"a":1}]}`,
		`{"$ref":"#/$defs/n","$defs":{"n":{"type":"integer"}}}`,
		`{"type":"object","const":5}`,
	} {
		if _, err := seeded(1).Generate([]byte(schema)); !errors.Is(err, ErrRootMustBeObject) {
			t.Errorf("%s: error = %v, want ErrRootMustBeObject", schema, err)
		}
	}
}

func TestGenerateNullableRoot(t *testing.T) {
	faker := seeded(3)
	sawNull, sawObject := false, false
	for range 100 {
		object, err := faker.Generate([]byte(`{"type":["object","null"]}`))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if object == nil {
			sawNull = true
		} else {
			sawObject = true
		}
	}
	if !sawNull || !sawObject {
		t.Errorf("sawNull=%v sawObject=%v, want both", sawNull, sawObject)
	}
}

func TestGenerateValueAnyRoot(t *testing.T) {
	value, err := seeded(1).GenerateValue([]byte(`{"type":"string","format":"uuid"}`))
	if err != nil {
		t.Fatalf("GenerateValue: %v", err)
	}
	if text, ok := value.(string); !ok || len(text) != 36 {
		t.Errorf("value = %#v, want uuid string", value)
	}
}

func TestUnsupportedLocaleFailsEveryCall(t *testing.T) {
	faker := New(Options{Locale: "xx_XX"})
	if _, err := faker.Generate([]byte(`{"type":"object"}`)); !errors.Is(err, ErrUnsupportedLocale) {
		t.Errorf("Generate error = %v, want ErrUnsupportedLocale", err)
	}
	if _, err := faker.GenerateValue([]byte(`{"type":"string"}`)); !errors.Is(err, ErrUnsupportedLocale) {
		t.Errorf("GenerateValue error = %v, want ErrUnsupportedLocale", err)
	}
}

func TestParseErrorsAreReExported(t *testing.T) {
	tests := []struct {
		schema string
		want   error
	}{
		{`{"type":"widget"}`, ErrUnknownType},
		{`{}`, ErrMissingType},
		{`{"type":"integer","minimum":5,"maximum":1}`, ErrInvalidConstraint},
		{`{"type":"string","pattern":"(?=x)"}`, ErrInvalidPattern},
		{`{"$ref":"https://example.com/s.json"}`, ErrInvalidRef},
		{`{"type":"array","minItems":10001}`, ErrSizeLimitExceeded},
		{`{"type":"string","format":"uuid","maxLength":3}`, ErrUnsatisfiable},
	}
	for _, tt := range tests {
		if _, err := seeded(1).GenerateValue([]byte(tt.schema)); !errors.Is(err, tt.want) {
			t.Errorf("%s: error = %v, want %v", tt.schema, err, tt.want)
		}
	}
}

func TestGenerateRejectsOversizedMandatoryOutput(t *testing.T) {
	schema := []byte(`{"type":"object","required":["grid"],"properties":{"grid":{"type":"array","minItems":10000,
		"items":{"type":"array","minItems":10000,"items":{"type":"integer"}}}}}`)
	for seed := range int64(5) {
		if _, err := seeded(seed).Generate(schema); !errors.Is(err, ErrSizeLimitExceeded) {
			t.Fatalf("seed %d: error = %v, want ErrSizeLimitExceeded", seed, err)
		}
	}
}

// Twelve levels of default-sized arrays would average ~3^11 objects; the
// budget keeps the output within MaxGenerationCost.
func TestGenerateBoundsExponentialOutput(t *testing.T) {
	var defs []string
	for level := range 12 {
		defs = append(defs, fmt.Sprintf(`"d%d":{"type":"object","required":["v","next"],"properties":{
			"v":{"type":"string"},"next":{"type":"array","items":{"$ref":"#/$defs/d%d"}}}}`, level, level+1))
	}
	defs = append(defs, `"d12":{"type":"integer"}`)
	schema := []byte(`{"$defs":{` + strings.Join(defs, ",") + `},"$ref":"#/$defs/d0"}`)
	value, err := seeded(3).Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cost := outputCost(value); cost > generator.MaxGenerationCost {
		t.Fatalf("output costs %d, above MaxGenerationCost %d", cost, generator.MaxGenerationCost)
	}
}

// outputCost mirrors the generator's cost model: 1 per value plus the code
// points of every string.
func outputCost(value any) int {
	switch typed := value.(type) {
	case string:
		return 1 + utf8.RuneCountInString(typed)
	case map[string]any:
		total := 1
		for _, member := range typed {
			total += outputCost(member)
		}
		return total
	case []any:
		total := 1
		for _, item := range typed {
			total += outputCost(item)
		}
		return total
	}
	return 1
}

func TestStaticAPI(t *testing.T) {
	if _, err := Generate([]byte(`{"type":"object"}`)); err != nil {
		t.Errorf("Generate: %v", err)
	}
	if _, err := GenerateValue([]byte(`{"type":"boolean"}`)); err != nil {
		t.Errorf("GenerateValue: %v", err)
	}
}

// userSchema exercises nested objects, bounded arrays, formats, enum,
// patterns, nullable values and a recursive $ref.
const userSchema = `{
	"type": "object",
	"required": ["id", "email", "role", "tags", "address", "code", "manager", "createdAt"],
	"properties": {
		"id": {"type": "string", "format": "uuid"},
		"email": {"type": "string", "format": "email"},
		"role": {"enum": ["admin", "editor", "viewer"]},
		"age": {"type": ["integer", "null"], "minimum": 18, "maximum": 99},
		"score": {"type": "number", "minimum": 0, "maximum": 5, "multipleOf": 0.5},
		"tags": {"type": "array", "minItems": 2, "maxItems": 4, "items": {"type": "string", "pattern": "^[a-z]{3,8}$"}},
		"address": {
			"type": "object",
			"required": ["zip", "city"],
			"properties": {
				"zip": {"type": "string", "pattern": "^\\d{5}$"},
				"city": {"type": "string", "minLength": 3, "maxLength": 12}
			}
		},
		"code": {"type": "string", "pattern": "^[A-Z]{2}-\\d{4}$"},
		"manager": {"$ref": "#/$defs/employee"},
		"createdAt": {"type": "string", "format": "date-time"},
		"active": {"type": "boolean"}
	},
	"$defs": {
		"employee": {
			"type": ["object", "null"],
			"required": ["name", "reports"],
			"properties": {
				"name": {"type": "string"},
				"reports": {"type": "array", "maxItems": 3, "items": {"$ref": "#/$defs/employee"}}
			}
		}
	}
}`

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	tagPattern  = regexp.MustCompile(`^[a-z]{3,8}$`)
	zipPattern  = regexp.MustCompile(`^\d{5}$`)
	codePattern = regexp.MustCompile(`^[A-Z]{2}-\d{4}$`)
)

func TestGenerateComplexSchema(t *testing.T) {
	faker := seeded(2024)
	seen := map[string]bool{}
	for range 300 {
		user, err := faker.Generate([]byte(userSchema))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if err := checkUser(user); err != nil {
			t.Fatalf("%v in %#v", err, user)
		}
		for key := range user {
			seen[key] = true
		}
	}
	if len(seen) != 11 {
		t.Errorf("observed properties %v, want all 11 at least once", seen)
	}
}

func checkUser(user map[string]any) error {
	checks := []error{
		checkString(user, "id", uuidPattern.MatchString),
		checkString(user, "email", isEmail),
		checkString(user, "code", codePattern.MatchString),
		checkString(user, "createdAt", isDateTime),
		checkRole(user["role"]),
		checkOptional(user, "age", checkAge),
		checkOptional(user, "score", checkScore),
		checkOptional(user, "active", checkBool),
		checkTags(user["tags"]),
		checkAddress(user["address"]),
		checkEmployee(user["manager"], 0),
	}
	return errors.Join(checks...)
}

func checkString(object map[string]any, key string, valid func(string) bool) error {
	text, ok := object[key].(string)
	if !ok || !valid(text) {
		return fmt.Errorf("%s = %#v is invalid", key, object[key])
	}
	return nil
}

func checkOptional(object map[string]any, key string, check func(any) error) error {
	value, present := object[key]
	if !present {
		return nil
	}
	return check(value)
}

func checkRole(value any) error {
	if !slices.Contains([]any{"admin", "editor", "viewer"}, value) {
		return fmt.Errorf("role = %#v not in enum", value)
	}
	return nil
}

func checkAge(value any) error {
	if value == nil {
		return nil
	}
	if age, ok := value.(int64); !ok || age < 18 || age > 99 {
		return fmt.Errorf("age = %#v, want nil or int64 in 18..99", value)
	}
	return nil
}

func checkScore(value any) error {
	score, ok := value.(float64)
	if !ok || score < 0 || score > 5 || math.Abs(score*2-math.Round(score*2)) > 1e-9 {
		return fmt.Errorf("score = %#v, want float64 multiple of 0.5 in 0..5", value)
	}
	return nil
}

func checkBool(value any) error {
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("active = %#v, want bool", value)
	}
	return nil
}

func checkTags(value any) error {
	tags, ok := value.([]any)
	if !ok || len(tags) < 2 || len(tags) > 4 {
		return fmt.Errorf("tags = %#v, want 2..4 items", value)
	}
	for _, tag := range tags {
		if text, ok := tag.(string); !ok || !tagPattern.MatchString(text) {
			return fmt.Errorf("tag = %#v does not match pattern", tag)
		}
	}
	return nil
}

func checkAddress(value any) error {
	address, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("address = %#v, want object", value)
	}
	city, ok := address["city"].(string)
	if !ok || len(city) < 3 || len(city) > 12 {
		return fmt.Errorf("city = %#v, want 3..12 chars", address["city"])
	}
	return checkString(address, "zip", zipPattern.MatchString)
}

// checkEmployee walks the recursive manager tree; recursion must stop
// within a couple of levels because each $ref expands once per branch.
func checkEmployee(value any, depth int) error {
	if value == nil {
		return nil
	}
	if depth > 3 {
		return fmt.Errorf("employee tree deeper than 3 levels")
	}
	employee, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("employee = %#v, want object or null", value)
	}
	if _, ok := employee["name"].(string); !ok {
		return fmt.Errorf("employee name = %#v, want string", employee["name"])
	}
	reports, ok := employee["reports"].([]any)
	if !ok || len(reports) > 3 {
		return fmt.Errorf("reports = %#v, want at most 3 items", employee["reports"])
	}
	for _, report := range reports {
		if err := checkEmployee(report, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func isEmail(text string) bool {
	address, err := mail.ParseAddress(text)
	return err == nil && address.Address == text
}

func isDateTime(text string) bool {
	_, err := time.Parse(time.RFC3339, text)
	return err == nil
}

// generateSequence runs count calls on faker, in order.
func generateSequence(t *testing.T, faker *Faker, count int) []map[string]any {
	t.Helper()
	results := make([]map[string]any, count)
	for i := range results {
		user, err := faker.Generate([]byte(userSchema))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		results[i] = user
	}
	return results
}

func TestGenerateIsDeterministic(t *testing.T) {
	first := generateSequence(t, seeded(42), 20)
	second := generateSequence(t, seeded(42), 20)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed produced different sequences")
	}
	if reflect.DeepEqual(first, generateSequence(t, seeded(43), 20)) {
		t.Error("different seeds produced identical sequences")
	}
	value, _ := seeded(7).GenerateValue([]byte(`{"type":"array","items":{"type":"string","format":"ipv6"}}`))
	again, _ := seeded(7).GenerateValue([]byte(`{"type":"array","items":{"type":"string","format":"ipv6"}}`))
	if !reflect.DeepEqual(value, again) {
		t.Error("GenerateValue is not deterministic for the same seed")
	}
}

// TestGenerateConcurrent shares one seeded Faker across goroutines. Because
// each call holds the lock for its whole generation, the results must be
// the same multiset as a sequential run with the same seed.
func TestGenerateConcurrent(t *testing.T) {
	const workers, callsPerWorker = 8, 25
	sequential := encodeSorted(t, generateSequence(t, seeded(99), workers*callsPerWorker))

	shared := seeded(99)
	var mu sync.Mutex
	var concurrent []map[string]any
	t.Run("workers", func(t *testing.T) {
		for worker := range workers {
			t.Run(fmt.Sprint(worker), func(t *testing.T) {
				t.Parallel()
				results := generateSequence(t, shared, callsPerWorker)
				for _, user := range results {
					if err := checkUser(user); err != nil {
						t.Error(err)
					}
				}
				mu.Lock()
				concurrent = append(concurrent, results...)
				mu.Unlock()
			})
		}
	})
	if !slices.Equal(sequential, encodeSorted(t, concurrent)) {
		t.Error("concurrent results differ from the sequential run with the same seed")
	}
}

func encodeSorted(t *testing.T, users []map[string]any) []string {
	t.Helper()
	encoded := make([]string, len(users))
	for i, user := range users {
		bytes, err := json.Marshal(user)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		encoded[i] = string(bytes)
	}
	slices.Sort(encoded)
	return encoded
}
