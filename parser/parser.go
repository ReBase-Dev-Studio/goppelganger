package parser

import "fmt"

// parser holds per-document state needed to resolve $ref.
type parser struct {
	document any
	// nodes caches every parsed node by JSON pointer, so each location is
	// parsed once and recursive refs link to existing nodes.
	nodes map[string]Node
	// refs collects RefNodes to link once the main tree is parsed.
	refs []*RefNode
}

// Parse converts a raw JSON Schema document into an AST.
func Parse(schema []byte) (Node, error) {
	document, err := decodeDocument(schema)
	if err != nil {
		return nil, fmt.Errorf("goppelganger: decode schema: %w", err)
	}
	p := &parser{document: document, nodes: map[string]Node{}}
	root, err := p.parseNode(document, "#")
	if err != nil {
		return nil, err
	}
	if err := p.linkRefs(); err != nil {
		return nil, err
	}
	return root, nil
}

func (p *parser) parseNode(raw any, path string) (Node, error) {
	if node, seen := p.nodes[path]; seen {
		return node, nil
	}
	node, err := p.buildNode(raw, path)
	if err != nil {
		return nil, err
	}
	p.nodes[path] = node
	return node, nil
}

func (p *parser) buildNode(raw any, path string) (Node, error) {
	obj, isObject := raw.(*orderedObject)
	if !isObject {
		return nil, fmt.Errorf("%w at %s: schema must be a JSON object", ErrInvalidConstraint, path)
	}
	if ref, hasRef := obj.get("$ref"); hasRef {
		return p.newRef(ref, path)
	}
	common, err := parseCommon(obj, path)
	if err != nil {
		return nil, err
	}
	typeName, hasType, err := parseType(obj, &common)
	if err != nil {
		return nil, err
	}
	if !hasType {
		return parseTypeless(common)
	}
	return p.parseTyped(obj, typeName, common)
}

func (p *parser) parseTyped(obj *orderedObject, typeName string, common Common) (Node, error) {
	switch typeName {
	case "object":
		return p.parseObject(obj, common)
	case "array":
		return p.parseArray(obj, common)
	case "string":
		return parseString(obj, common)
	case "integer":
		return parseInteger(obj, common)
	case "number":
		return parseNumber(obj, common)
	case "boolean":
		return &BooleanNode{Common: common}, nil
	case "null":
		return &NullNode{Common: common}, nil
	}
	return nil, fmt.Errorf("%w at %s: %q", ErrUnknownType, common.Path, typeName)
}

// parseTypeless accepts schemas without `type` only when enum or const
// fully determine the value.
func parseTypeless(common Common) (Node, error) {
	if common.HasConst || common.Enum != nil {
		return &LiteralNode{Common: common}, nil
	}
	return nil, fmt.Errorf("%w at %s", ErrMissingType, common.Path)
}

func parseCommon(obj *orderedObject, path string) (Common, error) {
	nullable, err := readBool(obj, "nullable", path)
	if err != nil {
		return Common{}, err
	}
	common := Common{Path: path, Nullable: nullable}
	if raw, ok := obj.get("const"); ok {
		common.Const = plainValue(raw)
		common.HasConst = true
	}
	raw, ok := obj.get("enum")
	if !ok {
		return common, nil
	}
	values, isArray := raw.([]any)
	if !isArray || len(values) == 0 {
		return Common{}, invalidKeyword(path, "enum", "must be a non-empty array")
	}
	common.Enum = plainValue(values).([]any)
	return common, nil
}

// parseType reads `type`, folding `[X, "null"]` into X plus Nullable.
func parseType(obj *orderedObject, common *Common) (string, bool, error) {
	raw, ok := obj.get("type")
	if !ok {
		return "", false, nil
	}
	switch value := raw.(type) {
	case string:
		return value, true, nil
	case []any:
		name, nullable, err := typeFromList(value, common.Path)
		common.Nullable = common.Nullable || nullable
		return name, true, err
	}
	return "", false, fmt.Errorf("%w at %s: type must be a string or an array of strings", ErrUnknownType, common.Path)
}

func typeFromList(list []any, path string) (string, bool, error) {
	var nonNull []string
	sawNull := false
	for _, item := range list {
		name, isString := item.(string)
		if !isString {
			return "", false, fmt.Errorf("%w at %s: type array must contain only strings", ErrUnknownType, path)
		}
		if name == "null" {
			sawNull = true
			continue
		}
		nonNull = append(nonNull, name)
	}
	switch {
	case len(nonNull) == 1:
		return nonNull[0], sawNull, nil
	case len(nonNull) == 0 && sawNull:
		return "null", false, nil
	case len(nonNull) == 0:
		return "", false, fmt.Errorf("%w at %s: type array is empty", ErrUnknownType, path)
	}
	return "", false, fmt.Errorf("%w at %s: multiple non-null types %v are not supported", ErrUnknownType, path, nonNull)
}
