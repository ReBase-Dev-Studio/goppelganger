package parser

import "testing"

func TestKindString(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindObject, "object"},
		{KindArray, "array"},
		{KindString, "string"},
		{KindInteger, "integer"},
		{KindNumber, "number"},
		{KindBoolean, "boolean"},
		{KindNull, "null"},
		{KindRef, "$ref"},
		{KindLiteral, "literal"},
		{Kind(0), "unknown"},
		{Kind(255), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestNodeKindAndMeta(t *testing.T) {
	nodes := []struct {
		node Node
		want Kind
	}{
		{&ObjectNode{}, KindObject},
		{&ArrayNode{}, KindArray},
		{&StringNode{}, KindString},
		{&IntegerNode{}, KindInteger},
		{&NumberNode{}, KindNumber},
		{&BooleanNode{}, KindBoolean},
		{&NullNode{}, KindNull},
		{&LiteralNode{}, KindLiteral},
		{&RefNode{}, KindRef},
	}
	for _, tt := range nodes {
		if got := tt.node.Kind(); got != tt.want {
			t.Errorf("%T.Kind() = %v, want %v", tt.node, got, tt.want)
		}
		tt.node.Meta().Nullable = true
		if !tt.node.Meta().Nullable {
			t.Errorf("%T.Meta() does not return a pointer to the embedded Common", tt.node)
		}
	}
}
