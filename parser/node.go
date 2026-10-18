package parser

import (
	"regexp"
	"regexp/syntax"
)

// Kind identifies the concrete type of a Node.
type Kind uint8

const (
	KindObject Kind = iota + 1
	KindArray
	KindString
	KindInteger
	KindNumber
	KindBoolean
	KindNull
	KindRef
	KindLiteral
)

var kindNames = [...]string{
	KindObject:  "object",
	KindArray:   "array",
	KindString:  "string",
	KindInteger: "integer",
	KindNumber:  "number",
	KindBoolean: "boolean",
	KindNull:    "null",
	KindRef:     "$ref",
	KindLiteral: "literal",
}

func (k Kind) String() string {
	if int(k) >= len(kindNames) || kindNames[k] == "" {
		return "unknown"
	}
	return kindNames[k]
}

// Node is a parsed schema node.
type Node interface {
	Kind() Kind
	Meta() *Common
}

// Common holds keywords shared by every node type.
type Common struct {
	// Nullable is set by `type: [X, "null"]` or OpenAPI's `nullable: true`.
	Nullable bool
	// Enum is nil when the keyword is absent; it is never empty when present.
	Enum []any
	// Const is only meaningful when HasConst is true (const may be null).
	Const    any
	HasConst bool
	// Path is the JSON pointer of this node, used in error messages.
	Path string
}

// Meta returns the shared keywords; promoted to every node via embedding.
func (c *Common) Meta() *Common { return c }

// ObjectNode is `type: object`.
type ObjectNode struct {
	Common
	// Properties keeps declaration order for deterministic generation.
	Properties []Property
	Required   map[string]bool
}

// Property is a named member of an ObjectNode.
type Property struct {
	Name   string
	Schema Node
}

// ArrayNode is `type: array`.
type ArrayNode struct {
	Common
	// Items is nil when the keyword is absent.
	Items Node
	// MinItems and MaxItems are nil when absent; the walker applies defaults.
	MinItems *int
	MaxItems *int
}

// StringNode is `type: string`.
type StringNode struct {
	Common
	MinLength *int
	MaxLength *int
	Format    string
	// Pattern is the unsimplified RE2 AST; nil when the keyword is absent.
	Pattern       *syntax.Regexp
	PatternSource string
	// Matcher is PatternSource compiled once at parse time; safe for
	// concurrent use. Nil exactly when Pattern is nil.
	Matcher *regexp.Regexp
}

// IntegerNode is `type: integer`. Exclusive bounds and fractional bounds are
// normalized by the parser into inclusive int64 bounds.
type IntegerNode struct {
	Common
	Minimum    *int64
	Maximum    *int64
	MultipleOf *int64
}

// NumberNode is `type: number`.
type NumberNode struct {
	Common
	Minimum      *float64
	Maximum      *float64
	ExclusiveMin bool
	ExclusiveMax bool
	MultipleOf   *float64
}

// BooleanNode is `type: boolean`.
type BooleanNode struct{ Common }

// NullNode is `type: null`.
type NullNode struct{ Common }

// LiteralNode is a typeless schema whose value comes from enum or const.
type LiteralNode struct{ Common }

// RefNode is a local `$ref`. Target is linked after parsing so that
// recursive schemas form a graph instead of infinite recursion.
type RefNode struct {
	Common
	Ref    string
	Target Node
}

func (*ObjectNode) Kind() Kind  { return KindObject }
func (*ArrayNode) Kind() Kind   { return KindArray }
func (*StringNode) Kind() Kind  { return KindString }
func (*IntegerNode) Kind() Kind { return KindInteger }
func (*NumberNode) Kind() Kind  { return KindNumber }
func (*BooleanNode) Kind() Kind { return KindBoolean }
func (*NullNode) Kind() Kind    { return KindNull }
func (*LiteralNode) Kind() Kind { return KindLiteral }
func (*RefNode) Kind() Kind     { return KindRef }

var (
	_ Node = (*ObjectNode)(nil)
	_ Node = (*ArrayNode)(nil)
	_ Node = (*StringNode)(nil)
	_ Node = (*IntegerNode)(nil)
	_ Node = (*NumberNode)(nil)
	_ Node = (*BooleanNode)(nil)
	_ Node = (*NullNode)(nil)
	_ Node = (*LiteralNode)(nil)
	_ Node = (*RefNode)(nil)
)
