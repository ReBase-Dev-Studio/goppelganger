package generator

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp/syntax"

	"github.com/ReBase-Dev-Studio/goppelganger/parser"
)

const (
	optionalPropertyProbability = 0.5
	nullProbability             = 0.25
	defaultMinItems             = 1
	defaultMaxItems             = 5
	// itemsRangeSpan widens [minItems, minItems+span] when only minItems is set.
	itemsRangeSpan = 4
)

// errCycle signals that a $ref re-entered itself; the enclosing object or
// array decides how to degrade (omit, null or empty).
var errCycle = errors.New("goppelganger: recursive $ref")

// defaultItems generates values for arrays without an `items` keyword.
// Shared and read-only, so safe across goroutines.
var defaultItems parser.Node = &parser.StringNode{}

// Walker turns an AST into random data. It is not safe for concurrent use;
// callers must serialize access to the underlying *rand.Rand.
type Walker struct {
	rng    *rand.Rand
	locale Locale
	// chain holds the refs on the current branch, for cycle detection.
	chain *refLink
	// links interns chains so they can key costs.
	links map[refLink]*refLink
	// costs memoizes minCost per node and chain.
	costs map[costKey]costResult
	// enums memoizes the cost of every enum value per node.
	enums map[*parser.Common][]int
	// budget caps the cost of one Walk; MaxGenerationCost outside tests.
	budget int
	// lengths memoizes patternLengths per regexp node.
	lengths map[*syntax.Regexp]lengthRange
}

// NewWalker returns a Walker drawing randomness from rng and words from
// locale, which must come from LookupLocale.
func NewWalker(rng *rand.Rand, locale Locale) *Walker {
	return &Walker{
		rng:     rng,
		locale:  locale,
		links:   map[refLink]*refLink{},
		costs:   map[costKey]costResult{},
		enums:   map[*parser.Common][]int{},
		budget:  MaxGenerationCost,
		lengths: map[*syntax.Regexp]lengthRange{},
	}
}

// Walk generates a value satisfying node. Schemas whose mandatory output
// costs more than the budget are rejected before any random draw.
func (w *Walker) Walk(node parser.Node) (any, error) {
	if minimum := w.minCost(node, nil); minimum.cost > w.budget {
		return nil, fmt.Errorf("%w at %s: minimum output costs more than the limit of %d",
			parser.ErrSizeLimitExceeded, node.Meta().Path, w.budget)
	}
	value, _, err := w.walk(node, w.budget)
	if errors.Is(err, errCycle) {
		return nil, nil
	}
	return value, err
}

// walk applies precedence const > enum > nullable > type-specific generator.
// Callers pass allowance >= minCost(node, w.chain); walk returns the cost of
// the value, which never exceeds allowance.
func (w *Walker) walk(node parser.Node, allowance int) (any, int, error) {
	meta := node.Meta()
	if meta.HasConst {
		value := cloneValue(meta.Const)
		return value, valueCost(value), nil
	}
	if meta.Enum != nil {
		return w.pickEnum(meta, allowance)
	}
	if meta.Nullable && (w.rng.Float64() < nullProbability || w.typeCost(node, w.chain).cost > allowance) {
		return nil, 1, nil
	}
	switch typed := node.(type) {
	case *parser.ObjectNode:
		return w.walkObject(typed, allowance)
	case *parser.ArrayNode:
		return w.walkArray(typed, allowance)
	case *parser.RefNode:
		return w.walkRef(typed, allowance)
	case *parser.StringNode:
		value, err := w.generateString(typed, allowance)
		if err != nil {
			return nil, 0, err
		}
		return value, valueCost(value), nil
	}
	value, err := w.generateScalar(node)
	return value, 1, err
}

func (w *Walker) generateScalar(node parser.Node) (any, error) {
	switch typed := node.(type) {
	case *parser.IntegerNode:
		return w.generateInteger(typed)
	case *parser.NumberNode:
		return w.generateNumber(typed)
	case *parser.BooleanNode:
		return w.generateBoolean(), nil
	case *parser.NullNode, *parser.LiteralNode:
		return nil, nil
	}
	return nil, fmt.Errorf("goppelganger: no generator for %v at %s", node.Kind(), node.Meta().Path)
}

// pickEnum draws uniformly among the values that fit allowance; when all of
// them fit, the draw is the same as without a budget.
func (w *Walker) pickEnum(meta *parser.Common, allowance int) (any, int, error) {
	costs := w.enumCosts(meta)
	affordable := make([]int, 0, len(costs))
	for index, cost := range costs {
		if cost <= allowance {
			affordable = append(affordable, index)
		}
	}
	index := affordable[w.rng.IntN(len(affordable))]
	return cloneValue(meta.Enum[index]), costs[index], nil
}

func (w *Walker) enumCosts(meta *parser.Common) []int {
	if costs, ok := w.enums[meta]; ok {
		return costs
	}
	costs := make([]int, len(meta.Enum))
	for index, value := range meta.Enum {
		costs[index] = valueCost(value)
	}
	w.enums[meta] = costs
	return costs
}

// walkObject reserves the minimum cost of the required properties still
// pending; optional properties only get what is left.
func (w *Walker) walkObject(node *parser.ObjectNode, allowance int) (any, int, error) {
	result := make(map[string]any, len(node.Properties))
	spent, reserved := 1, w.objectCost(node, w.chain)-1
	for _, property := range node.Properties {
		required := node.Required[property.Name]
		if required {
			reserved -= w.slotCost(property.Schema, w.chain)
		} else if w.rng.Float64() >= optionalPropertyProbability {
			continue
		}
		available := allowance - spent - reserved
		if !required && w.minCost(property.Schema, w.chain).cost > available {
			continue
		}
		value, cost, err := w.walk(property.Schema, available)
		switch {
		case errors.Is(err, errCycle) && !required:
			continue
		case errors.Is(err, errCycle):
			value, cost = nil, 1
		case err != nil:
			return nil, 0, err
		}
		result[property.Name] = value
		spent += cost
	}
	return result, spent, nil
}

// walkArray trims the drawn item count to what allowance can hold and
// reserves the minimum cost of every item still pending.
func (w *Walker) walkArray(node *parser.ArrayNode, allowance int) (any, int, error) {
	itemSchema := node.Items
	if itemSchema == nil {
		itemSchema = defaultItems
	}
	minimum, maximum := itemBounds(node)
	count := minimum + w.rng.IntN(maximum-minimum+1)
	itemCost := w.minCost(itemSchema, w.chain)
	if itemCost.cut && count > 0 {
		return cycleArray(node)
	}
	count = min(count, (allowance-1)/max(itemCost.cost, 1))
	items := make([]any, 0, count)
	spent := 1
	for index := range count {
		item, cost, err := w.walk(itemSchema, allowance-spent-(count-index-1)*itemCost.cost)
		if errors.Is(err, errCycle) {
			return cycleArray(node)
		}
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
		spent += cost
	}
	return items, spent, nil
}

func (w *Walker) walkRef(node *parser.RefNode, allowance int) (any, int, error) {
	if w.chain.contains(node) {
		return nil, 0, errCycle
	}
	parent := w.chain
	w.chain = w.pushRef(parent, node)
	defer func() { w.chain = parent }()
	return w.walk(node.Target, allowance)
}

// itemBounds applies defaults to absent minItems/maxItems and clamps the
// upper bound to parser.MaxGeneratedSize (the parser caps minItems).
func itemBounds(node *parser.ArrayNode) (int, int) {
	switch {
	case node.MinItems != nil && node.MaxItems != nil:
		return *node.MinItems, min(*node.MaxItems, parser.MaxGeneratedSize)
	case node.MinItems != nil:
		return *node.MinItems, min(*node.MinItems+itemsRangeSpan, parser.MaxGeneratedSize)
	case node.MaxItems != nil:
		return min(defaultMinItems, *node.MaxItems), min(*node.MaxItems, parser.MaxGeneratedSize)
	}
	return defaultMinItems, defaultMaxItems
}

// cycleArray is the value for an array whose items recurse into themselves:
// empty when the schema allows it, otherwise minItems nulls.
func cycleArray(node *parser.ArrayNode) (any, int, error) {
	if node.MinItems == nil {
		return []any{}, 1, nil
	}
	return make([]any, *node.MinItems), 1 + *node.MinItems, nil
}

// cloneValue deep-copies enum/const values so callers mutating the output
// never alias each other or the AST.
func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, member := range typed {
			clone[key] = cloneValue(member)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for i, item := range typed {
			clone[i] = cloneValue(item)
		}
		return clone
	}
	return value
}
