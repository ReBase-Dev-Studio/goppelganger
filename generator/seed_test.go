package generator

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func draw(t *testing.T, seed *int64) []uint64 {
	t.Helper()
	rng, err := NewRand(seed)
	if err != nil {
		t.Fatalf("NewRand: %v", err)
	}
	return drawFrom(rng)
}

func drawFrom(rng *rand.Rand) []uint64 {
	values := make([]uint64, 8)
	for i := range values {
		values[i] = rng.Uint64()
	}
	return values
}

func TestNewRandSeedIsReproducible(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, -1} {
		first, second := draw(t, &seed), draw(t, &seed)
		if !slices.Equal(first, second) {
			t.Errorf("seed %d produced different streams", seed)
		}
	}
}

func TestNewRandDifferentSeedsDiverge(t *testing.T) {
	zero, one := int64(0), int64(1)
	if slices.Equal(draw(t, &zero), draw(t, &one)) {
		t.Error("seeds 0 and 1 produced identical streams")
	}
}

func TestNewRandNilSeedIsRandom(t *testing.T) {
	if slices.Equal(draw(t, nil), draw(t, nil)) {
		t.Error("two nil-seeded RNGs produced identical streams")
	}
}

// TestNewRandGoldenStream locks the seed → stream mapping. If this fails,
// every user's seeded fixtures would silently change: treat it as breaking.
func TestNewRandGoldenStream(t *testing.T) {
	seed := int64(42)
	want := []uint64{8049799869446681793, 5158490231983756222, 13245621452906053900}
	if got := draw(t, &seed)[:3]; !slices.Equal(got, want) {
		t.Errorf("seed 42 stream = %v, want %v", got, want)
	}
	if got, want := splitMix64(0), uint64(0xe220a8397b1dcdaf); got != want {
		t.Errorf("splitMix64(0) = %#x, want %#x", got, want)
	}
}
