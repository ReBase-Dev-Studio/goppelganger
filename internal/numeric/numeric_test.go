package numeric

import (
	"math"
	"testing"
)

func TestIntegerDivision(t *testing.T) {
	tests := []struct {
		dividend, divisor, floor, ceil int64
	}{
		{7, 2, 3, 4},
		{-7, 2, -4, -3},
		{6, 3, 2, 2},
		{-6, 3, -2, -2},
		{0, 5, 0, 0},
		{math.MinInt64, 10, -922337203685477581, -922337203685477580},
		{math.MaxInt64, 10, 922337203685477580, 922337203685477581},
	}
	for _, tt := range tests {
		if got := FloorDiv(tt.dividend, tt.divisor); got != tt.floor {
			t.Errorf("FloorDiv(%d, %d) = %d, want %d", tt.dividend, tt.divisor, got, tt.floor)
		}
		if got := CeilDiv(tt.dividend, tt.divisor); got != tt.ceil {
			t.Errorf("CeilDiv(%d, %d) = %d, want %d", tt.dividend, tt.divisor, got, tt.ceil)
		}
	}
}

func TestMultipleRange(t *testing.T) {
	tests := []struct {
		lower, upper               float64
		exclusiveMin, exclusiveMax bool
		multiple                   float64
		first, last                float64
	}{
		{0.3, 0.7, false, false, 0.1, 3, 7},
		{0.5, 1, true, true, 0.5, 2, 1},
		{0.5, 1, true, false, 0.5, 2, 2},
		{0.6, 0.9, false, false, 0.5, 2, 1},
		{-1, 1, false, false, 0.25, -4, 4},
	}
	for _, tt := range tests {
		first, last := MultipleRange(tt.lower, tt.upper, tt.exclusiveMin, tt.exclusiveMax, tt.multiple)
		if first != tt.first || last != tt.last {
			t.Errorf("MultipleRange(%v, %v, %v, %v, %v) = %v..%v, want %v..%v",
				tt.lower, tt.upper, tt.exclusiveMin, tt.exclusiveMax, tt.multiple, first, last, tt.first, tt.last)
		}
	}
}
