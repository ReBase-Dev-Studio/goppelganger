package generator

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ReBase-Dev-Studio/goppelganger/internal/numeric"
	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

const (
	defaultNumberMin = 0
	defaultNumberMax = 1000
	// numberRangeSpan widens a one-sided bound into [min, min+span] or [max-span, max].
	numberRangeSpan   = 1000
	maxNumberAttempts = 10
)

func (w *Walker) generateInteger(node *parser.IntegerNode) (any, error) {
	lower, upper := integerBounds(node)
	if node.MultipleOf == nil {
		return w.int64Between(lower, upper), nil
	}
	multiple := *node.MultipleOf
	first, last := numeric.CeilDiv(lower, multiple), numeric.FloorDiv(upper, multiple)
	if first > last {
		first, last = collapseFactors(first, last, node.Maximum == nil)
	}
	return w.int64Between(first, last) * multiple, nil
}

// integerBounds applies defaults to absent bounds. Defaults never override
// explicit bounds; one-sided ranges saturate at the int64 limits.
func integerBounds(node *parser.IntegerNode) (int64, int64) {
	switch {
	case node.Minimum != nil && node.Maximum != nil:
		return *node.Minimum, *node.Maximum
	case node.Minimum != nil:
		return *node.Minimum, saturatingAdd(*node.Minimum, numberRangeSpan)
	case node.Maximum != nil:
		return saturatingAdd(*node.Maximum, -numberRangeSpan), *node.Maximum
	}
	return defaultNumberMin, defaultNumberMax
}

// collapseFactors handles defaulted ranges that contain no multiple (e.g.
// minimum 1, multipleOf 5000): a defaulted bound is a suggestion, so collapse
// onto the nearest multiple on the explicit side. The parser already proved
// that side has a multiple within int64.
func collapseFactors[T int64 | float64](first, last T, maximumDefaulted bool) (T, T) {
	if maximumDefaulted {
		return first, first
	}
	return last, last
}

func saturatingAdd(value, delta int64) int64 {
	if delta > 0 && value > math.MaxInt64-delta {
		return math.MaxInt64
	}
	if delta < 0 && value < math.MinInt64-delta {
		return math.MinInt64
	}
	return value + delta
}

// int64Between returns a uniform value in [lower, upper]. Unsigned arithmetic
// keeps the span correct even for [MinInt64, MaxInt64].
func (w *Walker) int64Between(lower, upper int64) int64 {
	span := uint64(upper) - uint64(lower)
	if span == math.MaxUint64 {
		return int64(w.rng.Uint64())
	}
	return int64(uint64(lower) + w.rng.Uint64N(span+1))
}

// generateNumber draws candidates from the (possibly defaulted) range and
// keeps the first that satisfies the explicit bounds. Rejections only happen
// on float edge cases (exclusive bounds hit exactly, rounding).
func (w *Walker) generateNumber(node *parser.NumberNode) (any, error) {
	lower, upper := numberBounds(node)
	for range maxNumberAttempts {
		candidate := w.numberCandidate(node, lower, upper)
		if numberInRange(node, candidate) {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("%w at %s: no number found within bounds", ErrUnsatisfiable, node.Path)
}

func (w *Walker) numberCandidate(node *parser.NumberNode, lower, upper float64) float64 {
	if node.MultipleOf == nil {
		if lower == upper {
			return lower
		}
		fraction := w.rng.Float64()
		return lower*(1-fraction) + upper*fraction
	}
	multiple := *node.MultipleOf
	first, last := numeric.MultipleRange(lower, upper, node.ExclusiveMin, node.ExclusiveMax, multiple)
	if first > last {
		first, last = collapseFactors(first, last, node.Maximum == nil)
	}
	factor := math.Min(first+math.Floor(w.rng.Float64()*(last-first+1)), last)
	return roundToMultiple(factor*multiple, multiple)
}

// numberBounds applies defaults to absent bounds. The span scales with the
// bound's magnitude so that e.g. minimum 1e20 still yields a real range.
func numberBounds(node *parser.NumberNode) (float64, float64) {
	switch {
	case node.Minimum != nil && node.Maximum != nil:
		return *node.Minimum, *node.Maximum
	case node.Minimum != nil:
		return *node.Minimum, *node.Minimum + math.Max(numberRangeSpan, math.Abs(*node.Minimum))
	case node.Maximum != nil:
		return *node.Maximum - math.Max(numberRangeSpan, math.Abs(*node.Maximum)), *node.Maximum
	}
	return defaultNumberMin, defaultNumberMax
}

// numberInRange checks only explicit bounds; defaults are not constraints.
func numberInRange(node *parser.NumberNode, value float64) bool {
	if node.Minimum != nil && (value < *node.Minimum || (node.ExclusiveMin && value == *node.Minimum)) {
		return false
	}
	if node.Maximum != nil && (value > *node.Maximum || (node.ExclusiveMax && value == *node.Maximum)) {
		return false
	}
	return true
}

// roundToMultiple trims float noise from factor*multiple
// (3*0.1 = 0.30000000000000004) by rounding to multiple's decimal places.
func roundToMultiple(value, multiple float64) float64 {
	places := decimalPlaces(multiple)
	if places == 0 || places > 15 {
		return value
	}
	scale := math.Pow10(places)
	rounded := math.Round(value*scale) / scale
	if math.IsInf(rounded, 0) || math.IsNaN(rounded) {
		return value
	}
	return rounded
}

func decimalPlaces(value float64) int {
	_, fraction, found := strings.Cut(strconv.FormatFloat(value, 'f', -1, 64), ".")
	if !found {
		return 0
	}
	return len(fraction)
}
