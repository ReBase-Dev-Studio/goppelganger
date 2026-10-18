// Package numeric holds integer and float range math shared by the parser
// (to reject unsatisfiable schemas) and the generator (to pick values).
package numeric

import "math"

// MultipleRange returns the factors k such that k*multiple lies within the
// bounds. A relative tolerance absorbs float error (0.3/0.1 = 2.9999999999999996),
// so callers that need strict compliance must re-check k*multiple.
func MultipleRange(lower, upper float64, exclusiveMin, exclusiveMax bool, multiple float64) (float64, float64) {
	lowFactor, highFactor := lower/multiple, upper/multiple
	first := math.Ceil(lowFactor - tolerance(lowFactor))
	if exclusiveMin && math.Abs(first-lowFactor) <= tolerance(lowFactor) {
		first++
	}
	last := math.Floor(highFactor + tolerance(highFactor))
	if exclusiveMax && math.Abs(last-highFactor) <= tolerance(highFactor) {
		last--
	}
	return first, last
}

func tolerance(value float64) float64 {
	return 1e-9 * math.Max(1, math.Abs(value))
}

// FloorDiv divides rounding toward negative infinity.
func FloorDiv(dividend, divisor int64) int64 {
	quotient := dividend / divisor
	if dividend%divisor != 0 && (dividend < 0) != (divisor < 0) {
		quotient--
	}
	return quotient
}

// CeilDiv divides rounding toward positive infinity.
func CeilDiv(dividend, divisor int64) int64 {
	quotient := dividend / divisor
	if dividend%divisor != 0 && (dividend < 0) == (divisor < 0) {
		quotient++
	}
	return quotient
}
