package expression

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// FieldValueAccessor provides access to field values for evaluation.
type FieldValueAccessor interface {
	// GetFieldValue returns the value of a field by name.
	// Returns the value and true if found, or empty string and false if not found.
	GetFieldValue(name string) (string, bool)
}

// EvaluationResult contains the result of evaluating an expression.
type EvaluationResult struct {
	// Matches indicates whether the expression matched.
	Matches bool

	// Captures contains regex capture groups if the expression used regex.
	// Index 0 is the full match, subsequent indices are capture groups.
	Captures []string
}

// Evaluator evaluates parsed expressions against field values.
type Evaluator struct {
	mu            sync.RWMutex
	caseSensitive bool
	regexCache    map[string]*regexp.Regexp
}

// NewEvaluator creates a new expression evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{
		caseSensitive: true,
		regexCache:    make(map[string]*regexp.Regexp),
	}
}

// SetCaseSensitive sets whether string comparisons are case-sensitive.
func (e *Evaluator) SetCaseSensitive(sensitive bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.caseSensitive = sensitive
}

// Evaluate evaluates a parsed expression against the given field accessor.
func (e *Evaluator) Evaluate(parsed *ParsedExpression, accessor FieldValueAccessor) (*EvaluationResult, error) {
	if parsed == nil || parsed.Expression == nil {
		return &EvaluationResult{Matches: true}, nil
	}

	switch expr := parsed.Expression.(type) {
	case *ConditionOnly:
		return e.evaluateConditionTree(expr.Condition, accessor)
	case *ConditionWithActions:
		return e.evaluateConditionTree(expr.Condition, accessor)
	default:
		return nil, fmt.Errorf("unsupported expression type: %T", parsed.Expression)
	}
}

// evaluateConditionTree evaluates a condition tree.
func (e *Evaluator) evaluateConditionTree(tree *ConditionTree, accessor FieldValueAccessor) (*EvaluationResult, error) {
	if tree == nil || tree.Root == nil {
		return &EvaluationResult{Matches: true}, nil
	}
	return e.evaluateNode(tree.Root, accessor)
}

// evaluateNode evaluates a condition node.
func (e *Evaluator) evaluateNode(node ConditionNode, accessor FieldValueAccessor) (*EvaluationResult, error) {
	switch n := node.(type) {
	case *Condition:
		return e.evaluateCondition(n, accessor)
	case *ConditionGroup:
		return e.evaluateGroup(n, accessor)
	default:
		return nil, fmt.Errorf("unsupported node type: %T", node)
	}
}

// evaluateGroup evaluates a condition group (AND/OR).
func (e *Evaluator) evaluateGroup(group *ConditionGroup, accessor FieldValueAccessor) (*EvaluationResult, error) {
	if len(group.Children) == 0 {
		return &EvaluationResult{Matches: true}, nil
	}

	var lastCaptures []string

	switch group.Operator {
	case LogicalAnd:
		// All conditions must match (short-circuit on first false)
		for _, child := range group.Children {
			result, err := e.evaluateNode(child, accessor)
			if err != nil {
				return nil, err
			}
			if !result.Matches {
				return &EvaluationResult{Matches: false}, nil
			}
			if len(result.Captures) > 0 {
				lastCaptures = result.Captures
			}
		}
		return &EvaluationResult{Matches: true, Captures: lastCaptures}, nil

	case LogicalOr:
		// Any condition must match (short-circuit on first true)
		for _, child := range group.Children {
			result, err := e.evaluateNode(child, accessor)
			if err != nil {
				return nil, err
			}
			if result.Matches {
				return result, nil
			}
		}
		return &EvaluationResult{Matches: false}, nil

	default:
		return nil, fmt.Errorf("unsupported logical operator: %s", group.Operator)
	}
}

// evaluateCondition evaluates a single condition.
func (e *Evaluator) evaluateCondition(cond *Condition, accessor FieldValueAccessor) (*EvaluationResult, error) {
	fieldValue, _ := accessor.GetFieldValue(cond.Field)
	compareValue := cond.Value

	e.mu.RLock()
	globalCaseSensitive := e.caseSensitive
	e.mu.RUnlock()

	// Per-condition CaseSensitive takes precedence if set to true.
	// Otherwise, use the global evaluator setting.
	// This allows expressions like `channel_name case_sensitive contains "BBC"` to
	// force case-sensitive matching even when the evaluator default is case-insensitive.
	caseSensitive := globalCaseSensitive || cond.CaseSensitive

	// Apply case sensitivity
	if !caseSensitive && !cond.Operator.IsRegex() {
		fieldValue = strings.ToLower(fieldValue)
		compareValue = strings.ToLower(compareValue)
	}

	var matches bool
	var captures []string
	var err error

	switch cond.Operator {
	case OpEquals:
		matches = fieldValue == compareValue
	case OpNotEquals:
		matches = fieldValue != compareValue
	case OpContains:
		matches = strings.Contains(fieldValue, compareValue)
	case OpNotContains:
		matches = !strings.Contains(fieldValue, compareValue)
	case OpStartsWith:
		matches = strings.HasPrefix(fieldValue, compareValue)
	case OpNotStartsWith:
		matches = !strings.HasPrefix(fieldValue, compareValue)
	case OpEndsWith:
		matches = strings.HasSuffix(fieldValue, compareValue)
	case OpNotEndsWith:
		matches = !strings.HasSuffix(fieldValue, compareValue)
	case OpMatches:
		matches, captures, err = e.matchRegex(fieldValue, cond.Value, caseSensitive)
	case OpNotMatches:
		matches, _, err = e.matchRegex(fieldValue, cond.Value, caseSensitive)
		matches = !matches
	case OpGreaterThan:
		matches, err = compareNumeric(fieldValue, compareValue, func(a, b float64) bool { return a > b })
	case OpGreaterThanOrEqual:
		matches, err = compareNumeric(fieldValue, compareValue, func(a, b float64) bool { return a >= b })
	case OpLessThan:
		matches, err = compareNumeric(fieldValue, compareValue, func(a, b float64) bool { return a < b })
	case OpLessThanOrEqual:
		matches, err = compareNumeric(fieldValue, compareValue, func(a, b float64) bool { return a <= b })
	default:
		return nil, fmt.Errorf("unsupported operator: %s", cond.Operator)
	}

	if err != nil {
		return nil, err
	}

	return &EvaluationResult{
		Matches:  matches,
		Captures: captures,
	}, nil
}

// matchRegex performs regex matching with capture group extraction.
func (e *Evaluator) matchRegex(value, pattern string, caseSensitive bool) (bool, []string, error) {
	// Prepend case-insensitive flag if needed
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}

	re, err := e.getOrCompileRegex(pattern)
	if err != nil {
		return false, nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
	}

	matches := re.FindStringSubmatch(value)
	if matches == nil {
		return false, nil, nil
	}

	return true, matches, nil
}

// getOrCompileRegex returns a cached compiled regex or compiles and caches it.
func (e *Evaluator) getOrCompileRegex(pattern string) (*regexp.Regexp, error) {
	e.mu.RLock()
	re, ok := e.regexCache[pattern]
	e.mu.RUnlock()

	if ok {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.regexCache[pattern] = re
	e.mu.Unlock()

	return re, nil
}

// compareNumeric compares two values as numbers.
func compareNumeric(a, b string, cmp func(float64, float64) bool) (bool, error) {
	aNum, err := strconv.ParseFloat(a, 64)
	if err != nil {
		return false, fmt.Errorf("cannot parse %q as number: %w", a, err)
	}

	bNum, err := strconv.ParseFloat(b, 64)
	if err != nil {
		return false, fmt.Errorf("cannot parse %q as number: %w", b, err)
	}

	return cmp(aNum, bNum), nil
}

// DynamicFieldAccessor wraps a base FieldValueAccessor and adds dynamic field resolution.
// It first checks the dynamic registry for @prefix:param fields, then falls back to the base accessor.
type DynamicFieldAccessor struct {
	base     FieldValueAccessor
	registry *DynamicFieldRegistry
}

// NewDynamicFieldAccessor creates an accessor that combines base field access with dynamic resolution.
func NewDynamicFieldAccessor(base FieldValueAccessor, registry *DynamicFieldRegistry) *DynamicFieldAccessor {
	return &DynamicFieldAccessor{
		base:     base,
		registry: registry,
	}
}

// GetFieldValue returns the value of a field, checking dynamic fields first.
func (a *DynamicFieldAccessor) GetFieldValue(name string) (string, bool) {
	// Check dynamic fields first (e.g., @dynamic(request.headers):x-custom-player)
	if a.registry != nil && IsDynamicField(name) {
		if value, ok := a.registry.Resolve(name); ok {
			return value, true
		}
	}

	// Fall back to base accessor
	if a.base != nil {
		return a.base.GetFieldValue(name)
	}

	return "", false
}

// EvaluateWithDynamicFields evaluates an expression with dynamic field support.
// This is a convenience method that wraps the accessor with dynamic field resolution.
func (e *Evaluator) EvaluateWithDynamicFields(parsed *ParsedExpression, accessor FieldValueAccessor, registry *DynamicFieldRegistry) (*EvaluationResult, error) {
	dynamicAccessor := NewDynamicFieldAccessor(accessor, registry)
	return e.Evaluate(parsed, dynamicAccessor)
}
