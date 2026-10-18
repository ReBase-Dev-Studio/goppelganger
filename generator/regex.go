package generator

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

const (
	// maxPatternAttempts bounds retries when a candidate fails the final
	// regexp match or the explicit minLength/maxLength.
	maxPatternAttempts = 10
	// openRepeatSpan caps open-ended quantifiers (*, +, {n,}) at min+span.
	openRepeatSpan = 10
)

var (
	// printableASCII is tried first so classes like [^a-z] stay readable.
	printableASCII = []rune{0x20, 0x7e}
	// unicodeScalars excludes surrogates, which UTF-8 strings cannot hold.
	unicodeScalars = []rune{0, 0xd7ff, 0xe000, unicode.MaxRune}
)

// errNoMatch marks a regexp node that no string can satisfy.
var errNoMatch = errors.New("goppelganger: pattern matches nothing")

// generatePattern builds strings from the RE2 AST aimed at a random target
// length and keeps the first one that the compiled pattern matches within
// the explicit length bounds and allowance (one unit pays for the value
// itself). Default lengths never apply to patterns.
func (w *Walker) generatePattern(node *parser.StringNode, allowance int) (any, error) {
	lowest, highest := w.patternTargetBounds(node, allowance)
	if lowest > highest {
		return nil, fmt.Errorf("%w at %s: pattern %q cannot match a length within the length bounds",
			ErrUnsatisfiable, node.Path, node.PatternSource)
	}
	for range maxPatternAttempts {
		var builder strings.Builder
		target := lowest + w.rng.IntN(highest-lowest+1)
		if err := w.writeRegexp(&builder, node.Pattern, target); err != nil {
			return nil, fmt.Errorf("%w at %s: pattern %q: %v", ErrUnsatisfiable, node.Path, node.PatternSource, err)
		}
		value := builder.String()
		if node.Matcher.MatchString(value) && withinExplicitLength(node, value) && utf8.RuneCountInString(value) < allowance {
			return value, nil
		}
	}
	return nil, fmt.Errorf("%w at %s: pattern %q never produced a match within the length bounds",
		ErrUnsatisfiable, node.Path, node.PatternSource)
}

// patternTargetBounds is the inclusive range target lengths are drawn from:
// the pattern's natural range, whose open repeats stretch up to
// minLength+openRepeatSpan, clipped to the explicit bounds and allowance-1.
func (w *Walker) patternTargetBounds(node *parser.StringNode, allowance int) (int, int) {
	lengths := w.patternLengths(node.Pattern)
	lowest, highest := lengths.min, lengths.natural
	if node.MinLength != nil {
		lowest = max(lowest, *node.MinLength)
		highest = max(highest, *node.MinLength+openRepeatSpan)
	}
	if node.MaxLength != nil {
		highest = min(highest, *node.MaxLength)
	}
	return lowest, min(highest, lengths.max, allowance-1)
}

// writeRegexp appends one random string matching re, as close to target
// code points as re's length range allows (target is clamped into it).
// Anchors and word boundaries emit nothing; the final match check rejects
// misplaced ones.
func (w *Walker) writeRegexp(builder *strings.Builder, re *syntax.Regexp, target int) error {
	lengths := w.patternLengths(re)
	target = min(max(target, lengths.min), lengths.max)
	switch re.Op {
	case syntax.OpNoMatch:
		return errNoMatch
	case syntax.OpLiteral:
		for _, char := range re.Rune {
			builder.WriteRune(char)
		}
	case syntax.OpCharClass:
		char, err := w.pickRune(re.Rune)
		if err != nil {
			return err
		}
		builder.WriteRune(char)
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		builder.WriteByte(alphanumeric[w.rng.IntN(len(alphanumeric))])
	case syntax.OpCapture:
		return w.writeRegexp(builder, re.Sub[0], target)
	case syntax.OpConcat:
		return w.writeConcat(builder, re.Sub, target)
	case syntax.OpAlternate:
		return w.writeAlternate(builder, re.Sub, target)
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return w.writeRepeat(builder, re, target)
	}
	// OpEmptyMatch, line/text anchors and word boundaries emit nothing.
	return nil
}

// writeConcat splits target between subs in order (see drawShare).
func (w *Walker) writeConcat(builder *strings.Builder, subs []*syntax.Regexp, target int) error {
	// restMin[i] and restMax[i] bound the length of subs[i:].
	restMin, restMax := make([]int, len(subs)+1), make([]int, len(subs)+1)
	for i := len(subs) - 1; i >= 0; i-- {
		lengths := w.patternLengths(subs[i])
		restMin[i] = w.addCost(restMin[i+1], lengths.min)
		restMax[i] = w.addCost(restMax[i+1], lengths.max)
	}
	remaining := target
	for i, sub := range subs {
		share := w.drawShare(w.patternLengths(sub), remaining, restMin[i+1], restMax[i+1])
		if err := w.writeRegexp(builder, sub, share); err != nil {
			return err
		}
		remaining -= share
	}
	return nil
}

// writeAlternate draws uniformly among the branches whose length range holds
// target, falling back to the nearest branch (lowest index on ties).
func (w *Walker) writeAlternate(builder *strings.Builder, subs []*syntax.Regexp, target int) error {
	fitting, nearest, nearestDistance := 0, 0, -1
	for i, sub := range subs {
		distance := rangeDistance(w.patternLengths(sub), target)
		if distance == 0 {
			fitting++
		}
		if nearestDistance < 0 || distance < nearestDistance {
			nearest, nearestDistance = i, distance
		}
	}
	if fitting == 0 {
		return w.writeRegexp(builder, subs[nearest], target)
	}
	choice := w.rng.IntN(fitting)
	for _, sub := range subs {
		if rangeDistance(w.patternLengths(sub), target) != 0 {
			continue
		}
		if choice == 0 {
			return w.writeRegexp(builder, sub, target)
		}
		choice--
	}
	return nil
}

// writeRepeat draws a repetition count that can reach target, then splits
// target between the copies (see drawShare).
func (w *Walker) writeRepeat(builder *strings.Builder, re *syntax.Regexp, target int) error {
	sub := w.patternLengths(re.Sub[0])
	count := w.repeatCount(re, sub, target)
	remaining := target
	for copies := count - 1; copies >= 0; copies-- {
		share := w.drawShare(sub, remaining, w.mulCost(copies, sub.min), w.mulCost(copies, sub.max))
		if err := w.writeRegexp(builder, re.Sub[0], share); err != nil {
			return err
		}
		remaining -= share
	}
	return nil
}

// repeatCount draws uniformly among the counts in re's bounds whose copies
// can sum to target; when none can (a gap such as (ab)+ with an odd
// target), it overshoots with the smallest count reaching past target.
func (w *Walker) repeatCount(re *syntax.Regexp, sub lengthRange, target int) int {
	lowest, highest := repeatBounds(re)
	if openRepeat(re) {
		highest = w.budget + 1
	}
	if sub.max == 0 {
		return lowest
	}
	lowest = max(lowest, (target+sub.max-1)/sub.max)
	if sub.min > 0 {
		highest = min(highest, target/sub.min)
	} else {
		highest = min(highest, max(lowest, target))
	}
	if lowest > highest {
		return lowest
	}
	return lowest + w.rng.IntN(highest-lowest+1)
}

// drawShare picks the length of the next part of a sequence: uniform within
// part, leaving remaining-share reachable by the parts after it, whose
// lengths span [restMin, restMax]. Unreachable remainders get the shortest
// candidate.
func (w *Walker) drawShare(part lengthRange, remaining, restMin, restMax int) int {
	low := max(part.min, remaining-restMax)
	high := min(part.max, remaining-restMin)
	if low >= high {
		return min(low, part.max)
	}
	return low + w.rng.IntN(high-low+1)
}

// rangeDistance is how far target lies outside lengths (0 inside).
func rangeDistance(lengths lengthRange, target int) int {
	return max(lengths.min-target, target-lengths.max, 0)
}

// repeatBounds returns the inclusive repetition count range of a quantifier.
func repeatBounds(re *syntax.Regexp) (int, int) {
	switch re.Op {
	case syntax.OpStar:
		return 0, openRepeatSpan
	case syntax.OpPlus:
		return 1, 1 + openRepeatSpan
	case syntax.OpQuest:
		return 0, 1
	}
	if re.Max < 0 {
		return re.Min, re.Min + openRepeatSpan
	}
	return re.Min, re.Max
}

// openRepeat reports whether re has no upper repetition count.
func openRepeat(re *syntax.Regexp) bool {
	return re.Op == syntax.OpStar || re.Op == syntax.OpPlus || (re.Op == syntax.OpRepeat && re.Max < 0)
}

// lengthRange bounds, in code points, the strings a regexp node can match,
// saturated at the walker budget+1: natural caps open repeats at
// min+openRepeatSpan (see repeatBounds), max leaves them unbounded.
type lengthRange struct {
	min, natural, max int
}

// patternLengths returns the length range of re, memoized per Walker.
func (w *Walker) patternLengths(re *syntax.Regexp) lengthRange {
	if lengths, ok := w.lengths[re]; ok {
		return lengths
	}
	lengths := w.computeLengths(re)
	w.lengths[re] = lengths
	return lengths
}

func (w *Walker) computeLengths(re *syntax.Regexp) lengthRange {
	switch re.Op {
	case syntax.OpLiteral:
		length := min(len(re.Rune), w.budget+1)
		return lengthRange{min: length, natural: length, max: length}
	case syntax.OpCharClass, syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return lengthRange{min: 1, natural: 1, max: 1}
	case syntax.OpCapture:
		return w.patternLengths(re.Sub[0])
	case syntax.OpConcat:
		var total lengthRange
		for _, sub := range re.Sub {
			lengths := w.patternLengths(sub)
			total = lengthRange{
				min:     w.addCost(total.min, lengths.min),
				natural: w.addCost(total.natural, lengths.natural),
				max:     w.addCost(total.max, lengths.max),
			}
		}
		return total
	case syntax.OpAlternate:
		combined := w.patternLengths(re.Sub[0])
		for _, sub := range re.Sub[1:] {
			lengths := w.patternLengths(sub)
			combined = lengthRange{
				min:     min(combined.min, lengths.min),
				natural: max(combined.natural, lengths.natural),
				max:     max(combined.max, lengths.max),
			}
		}
		return combined
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return w.repeatLengths(re)
	}
	// OpNoMatch, OpEmptyMatch, anchors and word boundaries emit nothing.
	return lengthRange{}
}

func (w *Walker) repeatLengths(re *syntax.Regexp) lengthRange {
	minimum, maximum := repeatBounds(re)
	sub := w.patternLengths(re.Sub[0])
	unbounded := maximum
	if openRepeat(re) {
		unbounded = w.budget + 1
	}
	return lengthRange{
		min:     w.mulCost(minimum, sub.min),
		natural: w.mulCost(maximum, sub.natural),
		max:     w.mulCost(unbounded, sub.max),
	}
}

// pickRune draws from a character class, preferring printable ASCII.
func (w *Walker) pickRune(class []rune) (rune, error) {
	if candidates := intersectRanges(class, printableASCII); len(candidates) > 0 {
		return w.pickFromRanges(candidates), nil
	}
	if candidates := intersectRanges(class, unicodeScalars); len(candidates) > 0 {
		return w.pickFromRanges(candidates), nil
	}
	return 0, errNoMatch
}

// intersectRanges intersects two [lo, hi, lo, hi, ...] inclusive range lists.
func intersectRanges(left, right []rune) []rune {
	var result []rune
	for i := 0; i+1 < len(left); i += 2 {
		for j := 0; j+1 < len(right); j += 2 {
			low, high := max(left[i], right[j]), min(left[i+1], right[j+1])
			if low <= high {
				result = append(result, low, high)
			}
		}
	}
	return result
}

// pickFromRanges draws uniformly over every rune in the range list.
func (w *Walker) pickFromRanges(ranges []rune) rune {
	total := 0
	for i := 0; i < len(ranges); i += 2 {
		total += int(ranges[i+1]-ranges[i]) + 1
	}
	offset := w.rng.IntN(total)
	for i := 0; i < len(ranges); i += 2 {
		size := int(ranges[i+1]-ranges[i]) + 1
		if offset < size {
			return ranges[i] + rune(offset)
		}
		offset -= size
	}
	return ranges[len(ranges)-1]
}
