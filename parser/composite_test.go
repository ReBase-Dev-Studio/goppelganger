package parser

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestChildPath(t *testing.T) {
	tests := []struct {
		tokens []string
		want   string
	}{
		{[]string{"items"}, "#/items"},
		{[]string{"properties", "name"}, "#/properties/name"},
		{[]string{"properties", "a/b~c"}, "#/properties/a~1b~0c"},
	}
	for _, tt := range tests {
		if got := childPath("#", tt.tokens...); got != tt.want {
			t.Errorf("childPath(#, %v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestParseObject(t *testing.T) {
	node := mustParse[*ObjectNode](t, `{
		"type": "object",
		"required": ["id", "ghost"],
		"properties": {
			"id": {"type": "string", "format": "uuid"},
			"a/b": {"type": "integer"},
			"profile": {
				"type": "object",
				"properties": {"age": {"type": "integer", "minimum": 0}}
			}
		}
	}`)
	var names, paths []string
	for _, property := range node.Properties {
		names = append(names, property.Name)
		paths = append(paths, property.Schema.Meta().Path)
	}
	if want := []string{"id", "a/b", "profile"}; !reflect.DeepEqual(names, want) {
		t.Errorf("property order = %v, want %v", names, want)
	}
	if want := []string{"#/properties/id", "#/properties/a~1b", "#/properties/profile"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("property paths = %v, want %v", paths, want)
	}
	if want := map[string]bool{"id": true, "ghost": true}; !reflect.DeepEqual(node.Required, want) {
		t.Errorf("Required = %v, want %v", node.Required, want)
	}
	profile := node.Properties[2].Schema.(*ObjectNode)
	age := profile.Properties[0].Schema.(*IntegerNode)
	if age.Path != "#/properties/profile/properties/age" || *age.Minimum != 0 {
		t.Errorf("nested age = %q/%v", age.Path, *age.Minimum)
	}
}

func TestParseEmptyObject(t *testing.T) {
	node := mustParse[*ObjectNode](t, `{"type":"object"}`)
	if node.Properties != nil || node.Required == nil || len(node.Required) != 0 {
		t.Errorf("empty object = %+v, want nil Properties and empty Required", node)
	}
}

func TestParseArray(t *testing.T) {
	node := mustParse[*ArrayNode](t, `{"type":"array","minItems":2,"maxItems":4,"items":{"type":"string","format":"email"}}`)
	if *node.MinItems != 2 || *node.MaxItems != 4 {
		t.Errorf("items range = %d..%d, want 2..4", *node.MinItems, *node.MaxItems)
	}
	items, ok := node.Items.(*StringNode)
	if !ok || items.Format != "email" || items.Path != "#/items" {
		t.Errorf("Items = %#v", node.Items)
	}
	bare := mustParse[*ArrayNode](t, `{"type":["array","null"]}`)
	if bare.Items != nil || bare.MinItems != nil || bare.MaxItems != nil || !bare.Nullable {
		t.Errorf("bare array = %+v", bare)
	}
}

func TestParseCompositeErrors(t *testing.T) {
	tests := []struct {
		schema   string
		want     error
		wantPath string
	}{
		{`{"type":"object","properties":[]}`, ErrInvalidConstraint, "at #:"},
		{`{"type":"object","required":"id"}`, ErrInvalidConstraint, "at #:"},
		{`{"type":"object","required":[1]}`, ErrInvalidConstraint, "at #:"},
		{`{"type":"object","properties":{"x":{"type":"magia"}}}`, ErrUnknownType, "#/properties/x"},
		{`{"type":"object","properties":{"x":true}}`, ErrInvalidConstraint, "#/properties/x"},
		{`{"type":"array","items":[{"type":"string"}]}`, ErrInvalidConstraint, "at #:"},
		{`{"type":"array","items":{}}`, ErrMissingType, "#/items"},
		{`{"type":"array","minItems":-1}`, ErrInvalidConstraint, "at #:"},
		{`{"type":"array","maxItems":"5"}`, ErrInvalidConstraint, "at #:"},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.schema))
		if !errors.Is(err, tt.want) {
			t.Errorf("Parse(%s) error = %v, want %v", tt.schema, err, tt.want)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantPath) {
			t.Errorf("Parse(%s) error = %q, want path %q", tt.schema, err, tt.wantPath)
		}
	}
}
