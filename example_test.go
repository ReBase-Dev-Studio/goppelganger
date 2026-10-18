package goppelganger_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/ReBase-Dev-Studio/goppelganger"
)

var userSchema = []byte(`{
	"type": "object",
	"required": ["id", "email", "role", "tags"],
	"properties": {
		"id":    {"type": "string", "format": "uuid"},
		"email": {"type": "string", "format": "email"},
		"role":  {"enum": ["admin", "editor", "viewer"]},
		"age":   {"type": ["integer", "null"], "minimum": 18, "maximum": 99},
		"tags":  {"type": "array", "minItems": 1, "maxItems": 3, "items": {"type": "string", "pattern": "^[a-z]{3,8}$"}}
	}
}`)

// compiledUser is parsed once at package initialization; MustCompile panics
// on an invalid schema, so mistakes surface at startup.
var compiledUser = goppelganger.MustCompile(userSchema)

func seededFaker(seed int64) *goppelganger.Faker {
	return goppelganger.New(goppelganger.Options{Seed: &seed})
}

func printJSON(value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(encoded))
}

// The package-level functions use a randomly seeded Faker, so only the
// constrained parts of the output are predictable.
func ExampleGenerate() {
	order, err := goppelganger.Generate([]byte(`{
		"type": "object",
		"required": ["id", "status"],
		"properties": {
			"id":     {"type": "string", "format": "uuid"},
			"status": {"const": "paid"}
		}
	}`))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(order["status"], len(order["id"].(string)))
	// Output: paid 36
}

// A seeded Faker returns the same sequence of values for the same sequence
// of calls, which makes generated fixtures reproducible in tests.
func ExampleNew() {
	user, err := seededFaker(42).Generate(userSchema)
	if err != nil {
		log.Fatal(err)
	}
	printJSON(user)
	// Output:
	// {
	//   "email": "winter.banner616@mountain.example.org",
	//   "id": "c1be0cb7-e57d-4160-b718-3a6e539fecae",
	//   "role": "admin",
	//   "tags": [
	//     "hfj",
	//     "dztbzyna"
	//   ]
	// }
}

// GenerateValue accepts any root type, not only objects.
func ExampleFaker_GenerateValue() {
	addresses, err := seededFaker(7).GenerateValue([]byte(`{
		"type": "array",
		"minItems": 2,
		"maxItems": 2,
		"items": {"type": "string", "format": "ipv4"}
	}`))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(addresses)
	// Output:
	// [146.181.107.237 184.47.140.171]
}

// Compile parses a schema once; the result is immutable and can be shared
// by any number of Fakers and goroutines.
func ExampleCompile() {
	counter, err := goppelganger.Compile([]byte(`{"type": "integer", "minimum": 1, "maximum": 6}`))
	if err != nil {
		log.Fatal(err)
	}
	faker := seededFaker(3)
	rolls := make([]any, 0, 5)
	for range 5 {
		roll, err := faker.GenerateSchemaValue(counter)
		if err != nil {
			log.Fatal(err)
		}
		rolls = append(rolls, roll)
	}
	fmt.Println(rolls...)
	// Output:
	// 1 1 4 6 2
}

// Recursive schemas expand once per branch; deeper levels are cut off
// (an optional property is omitted, an array becomes empty).
func ExampleFaker_GenerateSchema() {
	category := goppelganger.MustCompile([]byte(`{
		"$ref": "#/$defs/category",
		"$defs": {
			"category": {
				"type": "object",
				"required": ["name", "children"],
				"properties": {
					"name":     {"type": "string", "pattern": "^[A-Z][a-z]{3,6}$"},
					"children": {"type": "array", "maxItems": 2, "items": {"$ref": "#/$defs/category"}}
				}
			}
		}
	}`))
	tree, err := seededFaker(5).GenerateSchema(category)
	if err != nil {
		log.Fatal(err)
	}
	printJSON(tree)
	// Output:
	// {
	//   "children": [
	//     {
	//       "children": [],
	//       "name": "Djlw"
	//     },
	//     {
	//       "children": [],
	//       "name": "Lsvfj"
	//     }
	//   ],
	//   "name": "Popt"
	// }
}

func ExampleMustCompile() {
	faker := seededFaker(1)
	for range 2 {
		user, err := faker.GenerateSchema(compiledUser)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(user["role"], user["tags"])
	}
	// Output:
	// editor [zcq]
	// viewer [xjfmaz ayluds ezbcv]
}

// Every error wraps a sentinel re-exported by this package; match it with
// errors.Is instead of comparing messages.
func Example_errors() {
	_, err := goppelganger.Generate([]byte(`{"type": "integer", "minimum": 10, "maximum": 1}`))
	fmt.Println(errors.Is(err, goppelganger.ErrInvalidConstraint))

	_, err = goppelganger.Generate([]byte(`{"type": "string"}`))
	fmt.Println(errors.Is(err, goppelganger.ErrRootMustBeObject))

	_, err = goppelganger.New(goppelganger.Options{Locale: "xx_XX"}).Generate(userSchema)
	fmt.Println(errors.Is(err, goppelganger.ErrUnsupportedLocale))
	// Output:
	// true
	// true
	// true
}
