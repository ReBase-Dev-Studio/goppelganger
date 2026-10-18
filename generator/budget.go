package generator

import (
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

// MaxGenerationCost caps the total size of one generated value: every value
// costs 1 and every string also costs its length in code points (object keys
// are free). Schemas whose mandatory output costs more are rejected.
const MaxGenerationCost = 1_000_000

// refLink is one $ref on the current branch, linked to the refs above it.
// Links are interned per Walker (see pushRef), so equal branches share a
// pointer and can key the cost memo.
type refLink struct {
	parent *refLink
	ref    *parser.RefNode
}

func (link *refLink) contains(ref *parser.RefNode) bool {
	for current := link; current != nil; current = current.parent {
		if current.ref == ref {
			return true
		}
	}
	return false
}

// pushRef returns the interned chain of parent followed by ref.
func (w *Walker) pushRef(parent *refLink, ref *parser.RefNode) *refLink {
	key := refLink{parent: parent, ref: ref}
	if link, ok := w.links[key]; ok {
		return link
	}
	link := &key
	w.links[key] = link
	return link
}

type costKey struct {
	node  parser.Node
	chain *refLink
}

// costResult is the minimum cost of a node; cut means the node re-enters a
// $ref already on the chain, which the walker reports as errCycle.
type costResult struct {
	cost int
	cut  bool
}

// minCost returns the smallest cost the walker can spend on node when the
// refs on chain are open, saturated at w.budget+1. It mirrors the walker's
// degradation rules, so a branch given at least this much always fits.
func (w *Walker) minCost(node parser.Node, chain *refLink) costResult {
	key := costKey{node: node}
	switch node.(type) {
	case *parser.ObjectNode, *parser.ArrayNode, *parser.RefNode:
		key.chain = chain
	}
	if result, ok := w.costs[key]; ok {
		return result
	}
	result := w.computeCost(node, chain)
	w.costs[key] = result
	return result
}

// computeCost follows the walker's precedence: const > enum > nullable > type.
func (w *Walker) computeCost(node parser.Node, chain *refLink) costResult {
	meta := node.Meta()
	switch {
	case meta.HasConst:
		return costResult{cost: min(valueCost(meta.Const), w.budget+1)}
	case meta.Enum != nil:
		cheapest := valueCost(meta.Enum[0])
		for _, value := range meta.Enum[1:] {
			cheapest = min(cheapest, valueCost(value))
		}
		return costResult{cost: min(cheapest, w.budget+1)}
	case meta.Nullable:
		return costResult{cost: 1}
	}
	return w.typeCost(node, chain)
}

// typeCost is the minimum cost of node's type-specific generator.
func (w *Walker) typeCost(node parser.Node, chain *refLink) costResult {
	switch typed := node.(type) {
	case *parser.ObjectNode:
		return costResult{cost: w.objectCost(typed, chain)}
	case *parser.ArrayNode:
		return costResult{cost: w.arrayCost(typed, chain)}
	case *parser.RefNode:
		if chain.contains(typed) {
			return costResult{cut: true}
		}
		return w.minCost(typed.Target, w.pushRef(chain, typed))
	case *parser.StringNode:
		return costResult{cost: w.stringCost(typed)}
	}
	return costResult{cost: 1}
}

// objectCost counts the object plus its required properties; a required
// property that re-enters the chain becomes nil (cost 1).
func (w *Walker) objectCost(node *parser.ObjectNode, chain *refLink) int {
	total := 1
	for _, property := range node.Properties {
		if total > w.budget {
			break
		}
		if !node.Required[property.Name] {
			continue
		}
		total = w.addCost(total, w.slotCost(property.Schema, chain))
	}
	return total
}

// arrayCost counts the array plus minItems items; items that re-enter the
// chain become minItems nils (see cycleArray).
func (w *Walker) arrayCost(node *parser.ArrayNode, chain *refLink) int {
	if node.MinItems == nil || *node.MinItems == 0 {
		return 1
	}
	count := *node.MinItems
	itemSchema := node.Items
	if itemSchema == nil {
		itemSchema = defaultItems
	}
	item := w.minCost(itemSchema, chain)
	if item.cut {
		return w.addCost(1, count)
	}
	return w.addCost(1, w.mulCost(count, item.cost))
}

// slotCost is the minimum cost of a value its container must emit: a
// re-entered $ref is absorbed as nil.
func (w *Walker) slotCost(node parser.Node, chain *refLink) int {
	result := w.minCost(node, chain)
	if result.cut {
		return 1
	}
	return result.cost
}

// stringCost uses the shortest reachable length for patterns, the upper
// bound for formats, whose length the walker cannot choose, and the explicit
// minLength otherwise.
func (w *Walker) stringCost(node *parser.StringNode) int {
	if node.Pattern != nil {
		lowest, _ := w.patternTargetBounds(node, w.budget+1)
		return w.addCost(1, lowest)
	}
	if length, ok := w.formatMaxLength(node.Format); ok {
		return w.addCost(1, capToMaxLength(node, length))
	}
	if node.MinLength != nil {
		return w.addCost(1, *node.MinLength)
	}
	return 1
}

func capToMaxLength(node *parser.StringNode, length int) int {
	if node.MaxLength != nil {
		return min(length, *node.MaxLength)
	}
	return length
}

// formatMaxLength is the longest value (in code points) each entry of
// formatGenerators can produce; ok is false for unknown formats.
func (w *Walker) formatMaxLength(format string) (int, bool) {
	word, tld := longestLength(w.locale.Words), longestLength(w.locale.TLDs)
	hostname := 2*word + len("-.example.") + tld
	switch format {
	case "uuid":
		return len("xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"), true
	case "date-time":
		return len("2006-01-02T15:04:05Z"), true
	case "date":
		return len(time.DateOnly), true
	case "time":
		return len("15:04:05Z"), true
	case "ipv4":
		return len("255.255.255.255"), true
	case "ipv6":
		return len("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"), true
	case "hostname":
		return hostname, true
	case "email":
		return 3*word + len(".@.example.") + len(strconv.Itoa(maxEmailSuffix)) + tld, true
	case "uri":
		return len("https://") + hostname + len("//") + 2*word, true
	}
	return 0, false
}

func longestLength(words []string) int {
	longest := 0
	for _, word := range words {
		longest = max(longest, utf8.RuneCountInString(word))
	}
	return longest
}

// valueCost is the cost of an emitted value: 1 per value plus the code
// points of every string.
func valueCost(value any) int {
	switch typed := value.(type) {
	case string:
		return 1 + utf8.RuneCountInString(typed)
	case map[string]any:
		total := 1
		for _, member := range typed {
			total += valueCost(member)
		}
		return total
	case []any:
		total := 1
		for _, item := range typed {
			total += valueCost(item)
		}
		return total
	}
	return 1
}

func (w *Walker) addCost(left, right int) int { return capAdd(left, right, w.budget) }
func (w *Walker) mulCost(left, right int) int { return capMul(left, right, w.budget) }

// capAdd and capMul take non-negative operands and cap the
// result at limit+1, which callers read as "over the limit".
func capAdd(left, right, limit int) int {
	if left > limit || right > limit-left {
		return limit + 1
	}
	return left + right
}

func capMul(left, right, limit int) int {
	if left != 0 && right > limit/left {
		return limit + 1
	}
	return left * right
}
