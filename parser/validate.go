package parser

import (
	"fmt"
	"math"

	"github.com/ReBase-Dev-Studio/goppelganger/internal/numeric"
)

// Cross-keyword validation: rejects schemas whose constraints can never be
// satisfied, so the generator never has to deal with empty ranges.

// MaxGeneratedSize caps array items and string length. Larger minimums are
// rejected at parse time; the generator clamps larger maximums to it.
const MaxGeneratedSize = 10_000

func validateLengths(minimum, maximum *int, minKey, maxKey, path string) error {
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return invalidKeyword(path, minKey, "is greater than "+maxKey)
	}
	if minimum != nil && *minimum > MaxGeneratedSize {
		return fmt.Errorf("%w at %s: %s %d is above the limit of %d",
			ErrSizeLimitExceeded, path, minKey, *minimum, MaxGeneratedSize)
	}
	return nil
}

func validateInteger(node *IntegerNode) error {
	lower, upper := int64(math.MinInt64), int64(math.MaxInt64)
	if node.Minimum != nil {
		lower = *node.Minimum
	}
	if node.Maximum != nil {
		upper = *node.Maximum
	}
	if lower > upper {
		return invalidKeyword(node.Path, "minimum", "leaves no integer within maximum")
	}
	if node.MultipleOf == nil {
		return nil
	}
	multiple := *node.MultipleOf
	if numeric.CeilDiv(lower, multiple) > numeric.FloorDiv(upper, multiple) {
		return invalidKeyword(node.Path, "multipleOf", "has no multiple within the allowed range")
	}
	return nil
}

func validateNumber(node *NumberNode) error {
	if node.Minimum == nil || node.Maximum == nil {
		return nil
	}
	lower, upper := *node.Minimum, *node.Maximum
	if lower > upper || (lower == upper && (node.ExclusiveMin || node.ExclusiveMax)) {
		return invalidKeyword(node.Path, "minimum", "leaves no number within maximum")
	}
	if node.MultipleOf == nil {
		return nil
	}
	first, last := numeric.MultipleRange(lower, upper, node.ExclusiveMin, node.ExclusiveMax, *node.MultipleOf)
	if first > last {
		return invalidKeyword(node.Path, "multipleOf", "has no multiple within the allowed range")
	}
	return nil
}
