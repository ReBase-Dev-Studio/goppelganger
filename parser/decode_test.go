package parser

import (
	"reflect"
	"testing"
)

func TestDecodeDocumentPreservesKeyOrder(t *testing.T) {
	document, err := decodeDocument([]byte(`{"b":1,"a":2,"c":{"z":1,"y":2},"a":3}`))
	if err != nil {
		t.Fatalf("decodeDocument: %v", err)
	}
	root := document.(*orderedObject)
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(root.keys, want) {
		t.Errorf("root keys = %v, want %v", root.keys, want)
	}
	if got := plainValue(root.values["a"]); got != int64(3) {
		t.Errorf("duplicate key value = %v, want last value 3", got)
	}
	nested := root.values["c"].(*orderedObject)
	if want := []string{"z", "y"}; !reflect.DeepEqual(nested.keys, want) {
		t.Errorf("nested keys = %v, want %v", nested.keys, want)
	}
}

func TestDecodeDocumentErrors(t *testing.T) {
	for _, input := range []string{``, `{`, `{"a":}`, `{"type":"string"} {}`, `[1,`} {
		if _, err := decodeDocument([]byte(input)); err == nil {
			t.Errorf("decodeDocument(%q) succeeded, want error", input)
		}
	}
}

func TestPlainValue(t *testing.T) {
	document, err := decodeDocument([]byte(`["a", 1, 2.5, 1e3, null, true, {"k": [false]}]`))
	if err != nil {
		t.Fatalf("decodeDocument: %v", err)
	}
	want := []any{"a", int64(1), 2.5, 1000.0, nil, true, map[string]any{"k": []any{false}}}
	if got := plainValue(document); !reflect.DeepEqual(got, want) {
		t.Errorf("plainValue = %#v, want %#v", got, want)
	}
}
