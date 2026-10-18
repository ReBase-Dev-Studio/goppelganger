package generator

import "github.com/ReBase-Dev-Studio/goppelganger/parser"

const (
	defaultMinLength = 5
	defaultMaxLength = 20
	// lengthRangeSpan widens [minLength, minLength+span] when only minLength is set.
	lengthRangeSpan = 15
	// alphanumeric is the default alphabet: safe for URLs, logs and frontends.
	alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// generateString applies `pattern` first (an assertion, while `format` is
// only an annotation in 2020-12), then a known `format`, otherwise an
// alphanumeric string within the (defaulted) length bounds, shortened to
// fit allowance (one unit pays for the value itself).
func (w *Walker) generateString(node *parser.StringNode, allowance int) (any, error) {
	if node.Pattern != nil {
		return w.generatePattern(node, allowance)
	}
	if generate, ok := formatGenerators[node.Format]; ok {
		return w.generateFormat(node, generate)
	}
	minimum, maximum := lengthBounds(node)
	maximum = min(maximum, allowance-1)
	minimum = min(minimum, maximum)
	return w.randomString(minimum+w.rng.IntN(maximum-minimum+1), alphanumeric), nil
}

// lengthBounds applies defaults to absent minLength/maxLength and clamps the
// upper bound to parser.MaxGeneratedSize (the parser caps minLength).
func lengthBounds(node *parser.StringNode) (int, int) {
	switch {
	case node.MinLength != nil && node.MaxLength != nil:
		return *node.MinLength, min(*node.MaxLength, parser.MaxGeneratedSize)
	case node.MinLength != nil:
		return *node.MinLength, min(*node.MinLength+lengthRangeSpan, parser.MaxGeneratedSize)
	case node.MaxLength != nil:
		return min(defaultMinLength, *node.MaxLength), min(*node.MaxLength, parser.MaxGeneratedSize)
	}
	return defaultMinLength, defaultMaxLength
}

// randomString draws length bytes from an ASCII alphabet.
func (w *Walker) randomString(length int, alphabet string) string {
	buffer := make([]byte, length)
	for i := range buffer {
		buffer[i] = alphabet[w.rng.IntN(len(alphabet))]
	}
	return string(buffer)
}

func (w *Walker) generateBoolean() bool {
	return w.rng.IntN(2) == 1
}
