package parser

import (
	"errors"
	"testing"
)

func TestRefToDefinitions(t *testing.T) {
	root := mustParse[*ObjectNode](t, `{
		"type": "object",
		"properties": {
			"home": {"$ref": "#/$defs/address"},
			"work": {"$ref": "#/$defs/address", "type": "integer"},
			"legacy": {"$ref": "#/definitions/zip"}
		},
		"$defs": {"address": {"type": "object", "properties": {"zip": {"$ref": "#/definitions/zip"}}}},
		"definitions": {"zip": {"type": "string", "pattern": "^[0-9]{5}$"}}
	}`)
	home := root.Properties[0].Schema.(*RefNode)
	work := root.Properties[1].Schema.(*RefNode)
	legacy := root.Properties[2].Schema.(*RefNode)
	address, ok := home.Target.(*ObjectNode)
	if !ok || address.Path != "#/$defs/address" {
		t.Fatalf("home.Target = %#v, want ObjectNode at #/$defs/address", home.Target)
	}
	if work.Target != home.Target {
		t.Error("refs to the same pointer must share one target node")
	}
	nestedZip := address.Properties[0].Schema.(*RefNode)
	if nestedZip.Target != legacy.Target || nestedZip.Target.Kind() != KindString {
		t.Errorf("nested ref target = %#v, want shared StringNode", nestedZip.Target)
	}
}

func TestRefRecursive(t *testing.T) {
	root := mustParse[*ObjectNode](t, `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"children": {"type": "array", "items": {"$ref": "#"}},
			"alias": {"$ref": "#/properties/name"}
		}
	}`)
	children := root.Properties[1].Schema.(*ArrayNode)
	if children.Items.(*RefNode).Target != Node(root) {
		t.Error(`"#" ref must link back to the root node`)
	}
	if root.Properties[2].Schema.(*RefNode).Target != root.Properties[0].Schema {
		t.Error("ref to an already parsed path must reuse that node")
	}
}

func TestRefMutualRecursion(t *testing.T) {
	root := mustParse[*RefNode](t, `{
		"$ref": "#/$defs/a",
		"$defs": {
			"a": {"type": "object", "properties": {"b": {"$ref": "#/$defs/b"}}},
			"b": {"type": "object", "properties": {"a": {"$ref": "#/$defs/a"}}}
		}
	}`)
	a := root.Target.(*ObjectNode)
	b := a.Properties[0].Schema.(*RefNode).Target.(*ObjectNode)
	if b.Properties[0].Schema.(*RefNode).Target != Node(a) {
		t.Error("a → b → a cycle must link back to the same node")
	}
}

func TestRefPointerEscaping(t *testing.T) {
	root := mustParse[*ObjectNode](t, `{
		"type": "object",
		"properties": {
			"slash": {"$ref": "#/$defs/a~1b"},
			"tilde": {"$ref": "#/$defs/c~0d"},
			"space": {"$ref": "#/$defs/with%20space"},
			"index": {"$ref": "#/$defs/list/1"}
		},
		"$defs": {
			"a/b": {"type": "string"},
			"c~d": {"type": "integer"},
			"with space": {"type": "boolean"},
			"list": [{"type": "null"}, {"type": "number"}]
		}
	}`)
	want := []Kind{KindString, KindInteger, KindBoolean, KindNumber}
	for i, property := range root.Properties {
		if got := property.Schema.(*RefNode).Target.Kind(); got != want[i] {
			t.Errorf("%s target = %v, want %v", property.Name, got, want[i])
		}
	}
}

func TestRefErrors(t *testing.T) {
	tests := []struct {
		schema string
		want   error
	}{
		{`{"$ref": 5}`, ErrInvalidRef},
		{`{"$ref": "other.json#/a"}`, ErrInvalidRef},
		{`{"$ref": "https://example.com/schema"}`, ErrInvalidRef},
		{`{"$ref": "#anchor"}`, ErrInvalidRef},
		{`{"$ref": "#/%zz"}`, ErrInvalidRef},
		{`{"$ref": "#/$defs/missing", "$defs": {}}`, ErrInvalidRef},
		{`{"$ref": "#/list/2", "list": [{"type":"string"}]}`, ErrInvalidRef},
		{`{"$ref": "#/list/01", "list": [{"type":"string"}, {"type":"string"}]}`, ErrInvalidRef},
		{`{"$ref": "#/list/-1", "list": [{"type":"string"}]}`, ErrInvalidRef},
		{`{"$ref": "#/$defs/s/type/x", "$defs": {"s": {"type": "string"}}}`, ErrInvalidRef},
		{`{"$ref": "#"}`, ErrInvalidRef},
		{`{"$ref": "#/$defs/a", "$defs": {"a": {"$ref": "#/$defs/b"}, "b": {"$ref": "#/$defs/a"}}}`, ErrInvalidRef},
		{`{"$ref": "#/$defs/bad", "$defs": {"bad": {"type": "magia"}}}`, ErrUnknownType},
		{`{"$ref": "#/required", "required": ["x"]}`, ErrInvalidConstraint},
	}
	for _, tt := range tests {
		if _, err := Parse([]byte(tt.schema)); !errors.Is(err, tt.want) {
			t.Errorf("Parse(%s) error = %v, want %v", tt.schema, err, tt.want)
		}
	}
}

func TestRefToNestedPathBeforeParent(t *testing.T) {
	root := mustParse[*ObjectNode](t, `{
		"type": "object",
		"properties": {
			"inner": {"$ref": "#/$defs/outer/properties/b"},
			"outer": {"$ref": "#/$defs/outer"}
		},
		"$defs": {"outer": {"type": "object", "properties": {"b": {"type": "string"}}}}
	}`)
	inner := root.Properties[0].Schema.(*RefNode).Target
	outer := root.Properties[1].Schema.(*RefNode).Target.(*ObjectNode)
	if outer.Properties[0].Schema != inner {
		t.Error("parsing a parent after its child must reuse the cached child node")
	}
}
