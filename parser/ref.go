package parser

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var pointerUnescaper = strings.NewReplacer("~1", "/", "~0", "~")

// newRef records a $ref for later linking. Sibling keywords are ignored.
func (p *parser) newRef(raw any, path string) (Node, error) {
	ref, isString := raw.(string)
	if !isString {
		return nil, fmt.Errorf("%w at %s: $ref must be a string", ErrInvalidRef, path)
	}
	node := &RefNode{Common: Common{Path: path}, Ref: ref}
	p.refs = append(p.refs, node)
	return node, nil
}

// linkRefs resolves every pending ref. Parsing a target may discover new
// refs, so the slice is iterated by index while it grows.
func (p *parser) linkRefs() error {
	for i := 0; i < len(p.refs); i++ {
		ref := p.refs[i]
		target, err := p.resolve(ref)
		if err != nil {
			return err
		}
		ref.Target = target
	}
	for _, ref := range p.refs {
		if err := checkRefChain(ref); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) resolve(ref *RefNode) (Node, error) {
	tokens, err := parsePointer(ref.Ref)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %q: %v", ErrInvalidRef, ref.Path, ref.Ref, err)
	}
	path := childPath("#", tokens...)
	if node, seen := p.nodes[path]; seen {
		return node, nil
	}
	raw, err := lookupPointer(p.document, tokens)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %q: %v", ErrInvalidRef, ref.Path, ref.Ref, err)
	}
	return p.parseNode(raw, path)
}

// parsePointer turns a local URI fragment ("#", "#/$defs/a%20b") into
// unescaped JSON pointer tokens.
func parsePointer(ref string) ([]string, error) {
	fragment, isLocal := strings.CutPrefix(ref, "#")
	if !isLocal {
		return nil, errors.New("only local references starting with # are supported")
	}
	decoded, err := url.PathUnescape(fragment)
	if err != nil {
		return nil, err
	}
	if decoded == "" {
		return nil, nil
	}
	if !strings.HasPrefix(decoded, "/") {
		return nil, errors.New("plain-name fragments are not supported")
	}
	tokens := strings.Split(decoded[1:], "/")
	for i, token := range tokens {
		tokens[i] = pointerUnescaper.Replace(token)
	}
	return tokens, nil
}

func lookupPointer(document any, tokens []string) (any, error) {
	current := document
	for _, token := range tokens {
		switch value := current.(type) {
		case *orderedObject:
			member, ok := value.get(token)
			if !ok {
				return nil, fmt.Errorf("key %q not found", token)
			}
			current = member
		case []any:
			index, ok := arrayIndex(token, len(value))
			if !ok {
				return nil, fmt.Errorf("index %q out of range", token)
			}
			current = value[index]
		default:
			return nil, fmt.Errorf("cannot descend into %q", token)
		}
	}
	return current, nil
}

// arrayIndex parses an RFC 6901 array index: decimal digits, no leading zeros.
func arrayIndex(token string, length int) (int, bool) {
	if token == "" || (len(token) > 1 && token[0] == '0') || strings.TrimLeft(token, "0123456789") != "" {
		return 0, false
	}
	index, err := strconv.Atoi(token)
	if err != nil || index >= length {
		return 0, false
	}
	return index, true
}

// checkRefChain rejects refs that only point to other refs in a loop
// (A → B → A): there is no schema to generate from.
func checkRefChain(ref *RefNode) error {
	visited := map[*RefNode]bool{}
	for current := ref; ; {
		if visited[current] {
			return fmt.Errorf("%w at %s: %q is a circular reference with no schema", ErrInvalidRef, ref.Path, ref.Ref)
		}
		visited[current] = true
		next, isRef := current.Target.(*RefNode)
		if !isRef {
			return nil
		}
		current = next
	}
}
