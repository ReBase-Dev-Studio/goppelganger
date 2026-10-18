package generator

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
)

// NewRand returns a PCG-backed RNG. A nil seed draws 128 bits from
// crypto/rand; any other value (including 0) yields a reproducible stream.
// The returned *rand.Rand is not safe for concurrent use.
func NewRand(seed *int64) (*rand.Rand, error) {
	if seed != nil {
		state := uint64(*seed)
		return rand.New(rand.NewPCG(state, splitMix64(state))), nil
	}
	var entropy [16]byte
	if _, err := cryptorand.Read(entropy[:]); err != nil {
		return nil, fmt.Errorf("goppelganger: read random seed: %w", err)
	}
	high := binary.LittleEndian.Uint64(entropy[:8])
	low := binary.LittleEndian.Uint64(entropy[8:])
	return rand.New(rand.NewPCG(high, low)), nil
}

// splitMix64 derives PCG's second seed word from the first, so nearby user
// seeds (1, 2, 3...) still start from well-separated states.
func splitMix64(state uint64) uint64 {
	state += 0x9e3779b97f4a7c15
	state = (state ^ (state >> 30)) * 0xbf58476d1ce4e5b9
	state = (state ^ (state >> 27)) * 0x94d049bb133111eb
	return state ^ (state >> 31)
}
