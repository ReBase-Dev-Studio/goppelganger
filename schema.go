package goppelganger

import (
	"fmt"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

// Schema is a parsed JSON Schema, ready to generate from many times. It is
// immutable and safe for concurrent use by any number of Fakers. Build it
// with Compile or MustCompile; a nil *Schema panics.
type Schema struct {
	root parser.Node
	// rootKind is the kind behind any root $ref, checked by GenerateSchema.
	rootKind parser.Kind
}

// Compile parses schema once. Any root type is accepted: GenerateSchema
// reports ErrRootMustBeObject, GenerateSchemaValue takes every root.
func Compile(schema []byte) (*Schema, error) {
	root, err := parser.Parse(schema)
	if err != nil {
		return nil, err
	}
	return &Schema{root: root, rootKind: resolveRef(root).Kind()}, nil
}

// MustCompile is like Compile but panics with the wrapped error, for
// schemas known to be valid such as package-level variables.
func MustCompile(schema []byte) *Schema {
	compiled, err := Compile(schema)
	if err != nil {
		panic(fmt.Errorf("goppelganger: Compile: %w", err))
	}
	return compiled
}

// GenerateSchema returns a random object that satisfies schema. The root
// must be of type object (directly or through $ref); a nullable root yields
// a nil map when null is drawn.
func (f *Faker) GenerateSchema(schema *Schema) (map[string]any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	if schema.rootKind != parser.KindObject {
		return nil, fmt.Errorf("%w: got %v", ErrRootMustBeObject, schema.rootKind)
	}
	value, err := f.walk(schema.root)
	if err != nil || value == nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: enum/const produced %T", ErrRootMustBeObject, value)
	}
	return object, nil
}

// GenerateSchemaValue returns a random value of any JSON type that
// satisfies schema.
func (f *Faker) GenerateSchemaValue(schema *Schema) (any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	return f.walk(schema.root)
}

// GenerateSchema uses a shared, randomly seeded Faker.
func GenerateSchema(schema *Schema) (map[string]any, error) {
	return defaultFaker.GenerateSchema(schema)
}

// GenerateSchemaValue uses a shared, randomly seeded Faker.
func GenerateSchemaValue(schema *Schema) (any, error) {
	return defaultFaker.GenerateSchemaValue(schema)
}
