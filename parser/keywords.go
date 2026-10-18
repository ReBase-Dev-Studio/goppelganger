package parser

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
)

// maxLimit caps size keywords (minLength, maxItems, ...) to a sane int range.
const maxLimit = math.MaxInt32

func invalidKeyword(path, key, reason string) error {
	return fmt.Errorf("%w at %s: %s %s", ErrInvalidConstraint, path, key, reason)
}

// readNumber returns a numeric keyword as float64, or nil when absent.
func readNumber(obj *orderedObject, key, path string) (*float64, error) {
	raw, ok := obj.get(key)
	if !ok {
		return nil, nil
	}
	number, isNumber := raw.(json.Number)
	if !isNumber {
		return nil, invalidKeyword(path, key, "must be a number")
	}
	value, err := number.Float64()
	if err != nil {
		return nil, invalidKeyword(path, key, "is out of range")
	}
	return &value, nil
}

// readRat returns a numeric keyword as an exact rational, or nil when absent.
// Integer literals stay exact beyond 2^53; other values go through float64,
// which avoids big.Rat allocating huge numbers for literals like 1e-99999999.
func readRat(obj *orderedObject, key, path string) (*big.Rat, error) {
	raw, ok := obj.get(key)
	if !ok {
		return nil, nil
	}
	number, isNumber := raw.(json.Number)
	if !isNumber {
		return nil, invalidKeyword(path, key, "must be a number")
	}
	if integer, err := number.Int64(); err == nil {
		return new(big.Rat).SetInt64(integer), nil
	}
	float, err := number.Float64()
	if err != nil {
		return nil, invalidKeyword(path, key, "is out of range")
	}
	return new(big.Rat).SetFloat64(float), nil
}

// readNonNegativeInt returns a size keyword, or nil when absent.
func readNonNegativeInt(obj *orderedObject, key, path string) (*int, error) {
	value, err := readNumber(obj, key, path)
	if err != nil || value == nil {
		return nil, err
	}
	if *value < 0 || *value != math.Trunc(*value) || *value > maxLimit {
		return nil, invalidKeyword(path, key, "must be a non-negative integer")
	}
	limit := int(*value)
	return &limit, nil
}

// readString returns a string keyword and whether it was present.
func readString(obj *orderedObject, key, path string) (string, bool, error) {
	raw, ok := obj.get(key)
	if !ok {
		return "", false, nil
	}
	value, isString := raw.(string)
	if !isString {
		return "", false, invalidKeyword(path, key, "must be a string")
	}
	return value, true, nil
}

// readBool returns a boolean keyword, false when absent.
func readBool(obj *orderedObject, key, path string) (bool, error) {
	raw, ok := obj.get(key)
	if !ok {
		return false, nil
	}
	value, isBool := raw.(bool)
	if !isBool {
		return false, invalidKeyword(path, key, "must be a boolean")
	}
	return value, nil
}

// readBound merges an inclusive keyword (minimum/maximum) with its exclusive
// counterpart, supporting both draft-04 booleans and draft-06+ numbers.
// When both are numeric the tighter one wins; on a tie, exclusive wins.
func readBound(obj *orderedObject, inclusiveKey, exclusiveKey, path string, lower bool) (*big.Rat, bool, error) {
	inclusive, err := readRat(obj, inclusiveKey, path)
	if err != nil {
		return nil, false, err
	}
	raw, ok := obj.get(exclusiveKey)
	if !ok {
		return inclusive, false, nil
	}
	if flag, isBool := raw.(bool); isBool {
		return inclusive, flag && inclusive != nil, nil
	}
	exclusive, err := readRat(obj, exclusiveKey, path)
	if err != nil {
		return nil, false, err
	}
	if inclusive == nil {
		return exclusive, true, nil
	}
	comparison := exclusive.Cmp(inclusive)
	if !lower {
		comparison = -comparison
	}
	if comparison >= 0 {
		return exclusive, true, nil
	}
	return inclusive, false, nil
}
