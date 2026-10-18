package parser

import "strings"

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// childPath appends reference tokens to a JSON pointer, escaping per RFC 6901.
func childPath(path string, tokens ...string) string {
	var builder strings.Builder
	builder.WriteString(path)
	for _, token := range tokens {
		builder.WriteByte('/')
		builder.WriteString(pointerEscaper.Replace(token))
	}
	return builder.String()
}

func (p *parser) parseObject(obj *orderedObject, common Common) (Node, error) {
	path := common.Path
	properties, err := p.parseProperties(obj, path)
	if err != nil {
		return nil, err
	}
	required, err := parseRequired(obj, path)
	if err != nil {
		return nil, err
	}
	return &ObjectNode{Common: common, Properties: properties, Required: required}, nil
}

func (p *parser) parseProperties(obj *orderedObject, path string) ([]Property, error) {
	raw, ok := obj.get("properties")
	if !ok {
		return nil, nil
	}
	members, isObject := raw.(*orderedObject)
	if !isObject {
		return nil, invalidKeyword(path, "properties", "must be an object")
	}
	properties := make([]Property, 0, len(members.keys))
	for _, name := range members.keys {
		schema, err := p.parseNode(members.values[name], childPath(path, "properties", name))
		if err != nil {
			return nil, err
		}
		properties = append(properties, Property{Name: name, Schema: schema})
	}
	return properties, nil
}

// parseRequired returns the set of required names. Names missing from
// `properties` are kept; the walker simply never encounters them.
func parseRequired(obj *orderedObject, path string) (map[string]bool, error) {
	required := map[string]bool{}
	raw, ok := obj.get("required")
	if !ok {
		return required, nil
	}
	names, isArray := raw.([]any)
	if !isArray {
		return nil, invalidKeyword(path, "required", "must be an array of strings")
	}
	for _, item := range names {
		name, isString := item.(string)
		if !isString {
			return nil, invalidKeyword(path, "required", "must be an array of strings")
		}
		required[name] = true
	}
	return required, nil
}

func (p *parser) parseArray(obj *orderedObject, common Common) (Node, error) {
	path := common.Path
	node := &ArrayNode{Common: common}
	var err error
	if node.MinItems, err = readNonNegativeInt(obj, "minItems", path); err != nil {
		return nil, err
	}
	if node.MaxItems, err = readNonNegativeInt(obj, "maxItems", path); err != nil {
		return nil, err
	}
	if err = validateLengths(node.MinItems, node.MaxItems, "minItems", "maxItems", path); err != nil {
		return nil, err
	}
	raw, ok := obj.get("items")
	if !ok {
		return node, nil
	}
	if _, isTuple := raw.([]any); isTuple {
		return nil, invalidKeyword(path, "items", "tuple form is not supported; use a single schema")
	}
	if node.Items, err = p.parseNode(raw, childPath(path, "items")); err != nil {
		return nil, err
	}
	return node, nil
}
