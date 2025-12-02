// Package expression provides an expression engine for parsing and evaluating
// filter conditions and data mapping rules.
package expression

// FilterOperator represents a comparison operator in a filter condition.
type FilterOperator string

// Filter operators for condition matching.
const (
	// String comparison operators
	OpEquals        FilterOperator = "equals"
	OpNotEquals     FilterOperator = "not_equals"
	OpContains      FilterOperator = "contains"
	OpNotContains   FilterOperator = "not_contains"
	OpStartsWith    FilterOperator = "starts_with"
	OpNotStartsWith FilterOperator = "not_starts_with"
	OpEndsWith      FilterOperator = "ends_with"
	OpNotEndsWith   FilterOperator = "not_ends_with"

	// Regex operators
	OpMatches    FilterOperator = "matches"
	OpNotMatches FilterOperator = "not_matches"

	// Numeric comparison operators
	OpGreaterThan        FilterOperator = "greater_than"
	OpGreaterThanOrEqual FilterOperator = "greater_than_or_equal"
	OpLessThan           FilterOperator = "less_than"
	OpLessThanOrEqual    FilterOperator = "less_than_or_equal"
)

// IsNegated returns true if the operator is a negated form.
func (op FilterOperator) IsNegated() bool {
	switch op {
	case OpNotEquals, OpNotContains, OpNotStartsWith, OpNotEndsWith, OpNotMatches:
		return true
	default:
		return false
	}
}

// Base returns the non-negated form of the operator.
func (op FilterOperator) Base() FilterOperator {
	switch op {
	case OpNotEquals:
		return OpEquals
	case OpNotContains:
		return OpContains
	case OpNotStartsWith:
		return OpStartsWith
	case OpNotEndsWith:
		return OpEndsWith
	case OpNotMatches:
		return OpMatches
	default:
		return op
	}
}

// IsRegex returns true if the operator uses regex matching.
func (op FilterOperator) IsRegex() bool {
	return op == OpMatches || op == OpNotMatches
}

// IsNumeric returns true if the operator is for numeric comparison.
func (op FilterOperator) IsNumeric() bool {
	switch op {
	case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual:
		return true
	default:
		return false
	}
}

// LogicalOperator represents a boolean operator for combining conditions.
type LogicalOperator string

// Logical operators for combining conditions.
const (
	LogicalAnd LogicalOperator = "AND"
	LogicalOr  LogicalOperator = "OR"
)

// ActionOperator represents an operation to perform on a field.
type ActionOperator string

// Action operators for field modifications.
const (
	// ActionSet replaces the field value.
	ActionSet ActionOperator = "SET"

	// ActionSetIfEmpty sets the field only if it's currently empty/null.
	ActionSetIfEmpty ActionOperator = "SET_IF_EMPTY"

	// ActionAppend appends to the existing field value.
	ActionAppend ActionOperator = "APPEND"

	// ActionRemove removes a substring from the field value.
	ActionRemove ActionOperator = "REMOVE"

	// ActionDelete removes the field entirely (sets to null).
	ActionDelete ActionOperator = "DELETE"
)

// operatorKeywords maps keyword strings to FilterOperator values.
var operatorKeywords = map[string]FilterOperator{
	"equals":                OpEquals,
	"not_equals":            OpNotEquals,
	"contains":              OpContains,
	"not_contains":          OpNotContains,
	"starts_with":           OpStartsWith,
	"not_starts_with":       OpNotStartsWith,
	"ends_with":             OpEndsWith,
	"not_ends_with":         OpNotEndsWith,
	"matches":               OpMatches,
	"not_matches":           OpNotMatches,
	"greater_than":          OpGreaterThan,
	"greater_than_or_equal": OpGreaterThanOrEqual,
	"less_than":             OpLessThan,
	"less_than_or_equal":    OpLessThanOrEqual,
	// Aliases
	"eq":  OpEquals,
	"neq": OpNotEquals,
	"gt":  OpGreaterThan,
	"gte": OpGreaterThanOrEqual,
	"lt":  OpLessThan,
	"lte": OpLessThanOrEqual,
}

// ParseFilterOperator parses a string into a FilterOperator.
// Returns the operator and true if valid, or empty and false if invalid.
func ParseFilterOperator(s string) (FilterOperator, bool) {
	op, ok := operatorKeywords[s]
	return op, ok
}

// actionKeywords maps keyword strings to ActionOperator values.
var actionKeywords = map[string]ActionOperator{
	"SET":          ActionSet,
	"SET_IF_EMPTY": ActionSetIfEmpty,
	"APPEND":       ActionAppend,
	"REMOVE":       ActionRemove,
	"DELETE":       ActionDelete,
	// Lowercase versions
	"set":          ActionSet,
	"set_if_empty": ActionSetIfEmpty,
	"append":       ActionAppend,
	"remove":       ActionRemove,
	"delete":       ActionDelete,
}

// ParseActionOperator parses a string into an ActionOperator.
// Returns the operator and true if valid, or empty and false if invalid.
func ParseActionOperator(s string) (ActionOperator, bool) {
	op, ok := actionKeywords[s]
	return op, ok
}
