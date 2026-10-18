package parser

import (
	"fmt"
	"math/big"
	"regexp"
	"regexp/syntax"
)

func parseString(obj *orderedObject, common Common) (Node, error) {
	path := common.Path
	node := &StringNode{Common: common}
	var err error
	if node.MinLength, err = readNonNegativeInt(obj, "minLength", path); err != nil {
		return nil, err
	}
	if node.MaxLength, err = readNonNegativeInt(obj, "maxLength", path); err != nil {
		return nil, err
	}
	if err = validateLengths(node.MinLength, node.MaxLength, "minLength", "maxLength", path); err != nil {
		return nil, err
	}
	if node.Format, _, err = readString(obj, "format", path); err != nil {
		return nil, err
	}
	source, hasPattern, err := readString(obj, "pattern", path)
	if err != nil {
		return nil, err
	}
	if !hasPattern {
		return node, nil
	}
	pattern, err := syntax.Parse(source, syntax.Perl)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %v", ErrInvalidPattern, path, err)
	}
	matcher, err := regexp.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %v", ErrInvalidPattern, path, err)
	}
	node.Pattern = pattern
	node.PatternSource = source
	node.Matcher = matcher
	return node, nil
}

func parseInteger(obj *orderedObject, common Common) (Node, error) {
	path := common.Path
	lower, exclusiveMin, err := readBound(obj, "minimum", "exclusiveMinimum", path, true)
	if err != nil {
		return nil, err
	}
	upper, exclusiveMax, err := readBound(obj, "maximum", "exclusiveMaximum", path, false)
	if err != nil {
		return nil, err
	}
	node := &IntegerNode{Common: common}
	if node.Minimum, err = integerBound(lower, exclusiveMin, true, "minimum", path); err != nil {
		return nil, err
	}
	if node.Maximum, err = integerBound(upper, exclusiveMax, false, "maximum", path); err != nil {
		return nil, err
	}
	if node.MultipleOf, err = integerMultipleOf(obj, path); err != nil {
		return nil, err
	}
	if err = validateInteger(node); err != nil {
		return nil, err
	}
	return node, nil
}

func parseNumber(obj *orderedObject, common Common) (Node, error) {
	path := common.Path
	lower, exclusiveMin, err := readBound(obj, "minimum", "exclusiveMinimum", path, true)
	if err != nil {
		return nil, err
	}
	upper, exclusiveMax, err := readBound(obj, "maximum", "exclusiveMaximum", path, false)
	if err != nil {
		return nil, err
	}
	multiple, err := readNumber(obj, "multipleOf", path)
	if err != nil {
		return nil, err
	}
	if multiple != nil && *multiple <= 0 {
		return nil, invalidKeyword(path, "multipleOf", "must be greater than 0")
	}
	node := &NumberNode{
		Common:       common,
		Minimum:      ratToFloat(lower),
		Maximum:      ratToFloat(upper),
		ExclusiveMin: exclusiveMin,
		ExclusiveMax: exclusiveMax,
		MultipleOf:   multiple,
	}
	if err = validateNumber(node); err != nil {
		return nil, err
	}
	return node, nil
}

// integerBound converts a rational bound into the closest inclusive int64.
// A bound beyond int64 on its open side (e.g. maximum 1e30) means unbounded;
// beyond int64 on its closed side it can never be satisfied.
func integerBound(value *big.Rat, exclusive, lower bool, key, path string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	floor := new(big.Int).Div(value.Num(), value.Denom())
	bound := floor
	switch {
	case lower && (exclusive || !value.IsInt()):
		bound = floor.Add(floor, big.NewInt(1))
	case !lower && exclusive && value.IsInt():
		bound = floor.Sub(floor, big.NewInt(1))
	}
	if bound.IsInt64() {
		result := bound.Int64()
		return &result, nil
	}
	if (bound.Sign() > 0) == lower {
		return nil, invalidKeyword(path, key, "is outside the int64 range")
	}
	return nil, nil
}

func integerMultipleOf(obj *orderedObject, path string) (*int64, error) {
	value, err := readRat(obj, "multipleOf", path)
	if err != nil || value == nil {
		return nil, err
	}
	if value.Sign() <= 0 || !value.IsInt() || !value.Num().IsInt64() {
		return nil, invalidKeyword(path, "multipleOf", "must be a positive integer for integer schemas")
	}
	multiple := value.Num().Int64()
	return &multiple, nil
}

func ratToFloat(value *big.Rat) *float64 {
	if value == nil {
		return nil
	}
	float, _ := value.Float64()
	return &float
}
