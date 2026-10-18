// Package goppelganger is a Schema-First Faker: it reads a JSON Schema and
// generates random data that satisfies every constraint in the contract.
package goppelganger

import (
	"errors"
	"math/rand/v2"
	"sync"

	"github.com/ReBase-Dev-Studio/goppelganger/generator"
	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

// ErrRootMustBeObject is returned by Generate when the schema root is not
// of type "object". Use GenerateValue for non-object roots.
var ErrRootMustBeObject = errors.New("goppelganger: schema root must be of type object")

// Re-exported sentinel errors so callers only need to import this package.
var (
	ErrUnknownType       = parser.ErrUnknownType
	ErrMissingType       = parser.ErrMissingType
	ErrInvalidConstraint = parser.ErrInvalidConstraint
	ErrInvalidPattern    = parser.ErrInvalidPattern
	ErrInvalidRef        = parser.ErrInvalidRef
	ErrSizeLimitExceeded = parser.ErrSizeLimitExceeded
	ErrUnsatisfiable     = generator.ErrUnsatisfiable
	ErrUnsupportedLocale = generator.ErrUnsupportedLocale
)

// Faker generates schema-compliant data. It is safe for concurrent use.
type Faker struct {
	mu     sync.Mutex
	rng    *rand.Rand
	locale generator.Locale
	// initErr is reported by every Generate call (e.g. unsupported locale).
	initErr error
}

// New returns a Faker configured with opts. An unsupported locale does not
// fail here; it is reported as ErrUnsupportedLocale by every Generate call.
func New(opts Options) *Faker {
	locale, err := generator.LookupLocale(opts.localeOrDefault())
	if err != nil {
		return &Faker{initErr: err}
	}
	rng, err := generator.NewRand(opts.Seed)
	if err != nil {
		return &Faker{initErr: err}
	}
	return &Faker{rng: rng, locale: locale}
}

// Generate parses schema and returns a random object that satisfies it.
// It is Compile followed by GenerateSchema; compile once to reuse a schema.
func (f *Faker) Generate(schema []byte) (map[string]any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	compiled, err := Compile(schema)
	if err != nil {
		return nil, err
	}
	return f.GenerateSchema(compiled)
}

// GenerateValue parses schema and returns a random value of any JSON type.
// It is Compile followed by GenerateSchemaValue.
func (f *Faker) GenerateValue(schema []byte) (any, error) {
	if f.initErr != nil {
		return nil, f.initErr
	}
	compiled, err := Compile(schema)
	if err != nil {
		return nil, err
	}
	return f.GenerateSchemaValue(compiled)
}

// walk holds the lock for the whole generation so seeded output does not
// depend on how concurrent calls interleave.
func (f *Faker) walk(root parser.Node) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return generator.NewWalker(f.rng, f.locale).Walk(root)
}

// resolveRef follows $ref links to the schema they point at. The parser
// rejects pure ref→ref loops, so this terminates.
func resolveRef(node parser.Node) parser.Node {
	for {
		ref, ok := node.(*parser.RefNode)
		if !ok {
			return node
		}
		node = ref.Target
	}
}

var defaultFaker = New(Options{})

// Generate uses a shared, randomly seeded Faker.
func Generate(schema []byte) (map[string]any, error) {
	return defaultFaker.Generate(schema)
}

// GenerateValue uses a shared, randomly seeded Faker.
func GenerateValue(schema []byte) (any, error) {
	return defaultFaker.GenerateValue(schema)
}
