package goppelganger

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const arraySchema = `{"type":"array","items":{"type":"string","pattern":"^[a-z]{2,6}$"}}`

func TestGenerateSchemaMatchesGenerate(t *testing.T) {
	compiled := MustCompile([]byte(userSchema))
	viaBytes, viaSchema := seeded(42), seeded(42)
	for call := range 20 {
		want, err := viaBytes.Generate([]byte(userSchema))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		got, err := viaSchema.GenerateSchema(compiled)
		if err != nil {
			t.Fatalf("GenerateSchema: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d: GenerateSchema = %v, Generate = %v", call, got, want)
		}
	}
	want, _ := seeded(5).GenerateValue([]byte(arraySchema))
	got, _ := seeded(5).GenerateSchemaValue(MustCompile([]byte(arraySchema)))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GenerateSchemaValue = %v, GenerateValue = %v", got, want)
	}
}

func TestGenerateSchemaRejectsNonObjectRoot(t *testing.T) {
	compiled := MustCompile([]byte(arraySchema))
	if _, err := seeded(1).GenerateSchema(compiled); !errors.Is(err, ErrRootMustBeObject) {
		t.Errorf("GenerateSchema error = %v, want ErrRootMustBeObject", err)
	}
	if value, err := seeded(1).GenerateSchemaValue(compiled); err != nil || value == nil {
		t.Errorf("GenerateSchemaValue = %v, %v; want an array", value, err)
	}
}

func TestGenerateSchemaReportsInitErr(t *testing.T) {
	faker := New(Options{Locale: "xx_XX"})
	compiled := MustCompile([]byte(arraySchema))
	if _, err := faker.GenerateSchema(compiled); !errors.Is(err, ErrUnsupportedLocale) {
		t.Errorf("GenerateSchema error = %v, want ErrUnsupportedLocale", err)
	}
	if _, err := faker.GenerateSchemaValue(compiled); !errors.Is(err, ErrUnsupportedLocale) {
		t.Errorf("GenerateSchemaValue error = %v, want ErrUnsupportedLocale", err)
	}
}

func TestCompileErrors(t *testing.T) {
	compiled, err := Compile([]byte(`{"type":"string","pattern":"(?=x)"}`))
	if compiled != nil || !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("Compile = %v, %v; want nil, ErrInvalidPattern", compiled, err)
	}
	if _, err := Compile([]byte(`{`)); err == nil {
		t.Error("Compile accepted malformed JSON")
	}
}

func TestMustCompilePanicsWithWrappedError(t *testing.T) {
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, ErrMissingType) || !strings.HasPrefix(err.Error(), "goppelganger: Compile: ") {
			t.Errorf("recovered %v, want a wrapped ErrMissingType", err)
		}
	}()
	MustCompile([]byte(`{}`))
}

// TestSchemaSharedAcrossFakers shares one Schema between goroutines with
// their own Fakers (-race catches AST mutation); each must match the
// []byte path with the same seed.
func TestSchemaSharedAcrossFakers(t *testing.T) {
	const workers, callsPerWorker = 8, 25
	compiled := MustCompile([]byte(userSchema))
	t.Run("workers", func(t *testing.T) {
		for worker := range workers {
			t.Run(fmt.Sprint(worker), func(t *testing.T) {
				t.Parallel()
				want := generateSequence(t, seeded(int64(worker)), callsPerWorker)
				faker := seeded(int64(worker))
				for call := range callsPerWorker {
					got, err := faker.GenerateSchema(compiled)
					if err != nil {
						t.Fatalf("GenerateSchema: %v", err)
					}
					if !reflect.DeepEqual(got, want[call]) {
						t.Fatalf("call %d differs from the []byte path", call)
					}
				}
			})
		}
	})
}

func TestStaticSchemaAPI(t *testing.T) {
	user, err := GenerateSchema(MustCompile([]byte(userSchema)))
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	if err := checkUser(user); err != nil {
		t.Error(err)
	}
	if value, err := GenerateSchemaValue(MustCompile([]byte(arraySchema))); err != nil || value == nil {
		t.Errorf("GenerateSchemaValue = %v, %v", value, err)
	}
}

func BenchmarkGenerate(b *testing.B) {
	faker, schema := seeded(1), []byte(userSchema)
	for range b.N {
		if _, err := faker.Generate(schema); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateSchema(b *testing.B) {
	faker, compiled := seeded(1), MustCompile([]byte(userSchema))
	for range b.N {
		if _, err := faker.GenerateSchema(compiled); err != nil {
			b.Fatal(err)
		}
	}
}
