package expression

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationErrorCategory represents the category of a validation error.
type ValidationErrorCategory string

const (
	// ErrorCategorySyntax for general syntax issues.
	ErrorCategorySyntax ValidationErrorCategory = "syntax"
	// ErrorCategoryField for invalid field names.
	ErrorCategoryField ValidationErrorCategory = "field"
	// ErrorCategoryOperator for invalid operators.
	ErrorCategoryOperator ValidationErrorCategory = "operator"
	// ErrorCategoryValue for invalid values.
	ErrorCategoryValue ValidationErrorCategory = "value"
)

// ValidationError represents a single validation error with details.
type ValidationError struct {
	Category   ValidationErrorCategory `json:"category"`
	ErrorType  string                  `json:"error_type"`
	Message    string                  `json:"message"`
	Details    string                  `json:"details,omitempty"`
	Position   *int                    `json:"position,omitempty"`
	Context    string                  `json:"context,omitempty"`
	Suggestion string                  `json:"suggestion,omitempty"`
}

// ValidationResult contains the result of validating an expression.
type ValidationResult struct {
	IsValid             bool              `json:"is_valid"`
	CanonicalExpression string            `json:"canonical_expression,omitempty"`
	Errors              []ValidationError `json:"errors"`
	ExpressionTree      json.RawMessage   `json:"expression_tree,omitempty"`
}

// Validator validates expressions against a set of valid fields.
type Validator struct {
	registry *FieldRegistry
}

// NewValidator creates a new validator with the given field registry.
func NewValidator(registry *FieldRegistry) *Validator {
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &Validator{
		registry: registry,
	}
}

// Validate validates an expression string for the given domains.
// If no domains are specified, defaults to stream_filter and epg_filter.
func (v *Validator) Validate(expression string, domains ...ExpressionDomain) *ValidationResult {
	result := &ValidationResult{
		IsValid: true,
		Errors:  make([]ValidationError, 0),
	}

	// Default to stream and EPG filter domains if none specified
	if len(domains) == 0 {
		domains = []ExpressionDomain{DomainStreamFilter, DomainEPGFilter}
	}

	// Build union of valid fields across all domains
	validFields := v.getValidFieldsForDomains(domains)

	// Empty expression is valid
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return result
	}

	// Preprocess the expression
	preprocessed := Preprocess(expression)

	// Try to parse the expression
	parsed, err := Parse(preprocessed)
	if err != nil {
		result.IsValid = false

		// Extract position info if available
		var position *int
		var context string
		if pe, ok := err.(*ParseError); ok {
			position = &pe.Column
			// Extract context around the error
			if pe.Column > 0 && pe.Column <= len(preprocessed) {
				start := pe.Column - 1
				if start > 10 {
					start = pe.Column - 10
				}
				end := pe.Column + 20
				if end > len(preprocessed) {
					end = len(preprocessed)
				}
				context = preprocessed[start:end]
			}
		}

		result.Errors = append(result.Errors, ValidationError{
			Category:  ErrorCategorySyntax,
			ErrorType: "parse_error",
			Message:   err.Error(),
			Position:  position,
			Context:   context,
		})
		return result
	}

	// Validate field names used in the expression
	v.validateFields(parsed, validFields, result)

	// If valid, include the expression tree and canonical form
	if result.IsValid && parsed != nil {
		result.CanonicalExpression = preprocessed

		// Convert expression tree to JSON
		if treeJSON, err := json.Marshal(expressionToMap(parsed)); err == nil {
			result.ExpressionTree = treeJSON
		}
	}

	return result
}

// getValidFieldsForDomains returns the union of valid fields for the given domains.
func (v *Validator) getValidFieldsForDomains(domains []ExpressionDomain) map[string]bool {
	validFields := make(map[string]bool)

	for _, domain := range domains {
		// Get field domain types based on expression domain
		var fieldDomains []FieldDomain
		switch domain {
		case DomainStreamFilter, DomainStreamMapping:
			fieldDomains = []FieldDomain{DomainStream, DomainFilter, DomainRule}
		case DomainEPGFilter, DomainEPGMapping:
			fieldDomains = []FieldDomain{DomainEPG, DomainFilter, DomainRule}
		default:
			fieldDomains = []FieldDomain{DomainStream, DomainEPG, DomainFilter, DomainRule}
		}

		// Add all fields that belong to any of the allowed field domains
		for _, fieldDomain := range fieldDomains {
			for _, def := range v.registry.ListByDomain(fieldDomain) {
				validFields[def.Name] = true
				// Also add aliases
				for _, alias := range def.Aliases {
					validFields[alias] = true
				}
			}
		}
	}

	return validFields
}

// validateFields checks that all field names in the expression are valid.
func (v *Validator) validateFields(parsed *ParsedExpression, validFields map[string]bool, result *ValidationResult) {
	if parsed == nil || parsed.Expression == nil {
		return
	}

	// Extract fields from the expression
	var fields []string
	switch expr := parsed.Expression.(type) {
	case *ConditionOnly:
		if expr.Condition != nil && expr.Condition.Root != nil {
			fields = extractConditionFields(expr.Condition.Root)
		}
	case *ConditionWithActions:
		if expr.Condition != nil && expr.Condition.Root != nil {
			fields = extractConditionFields(expr.Condition.Root)
		}
		// Also check action target fields
		for _, action := range expr.Actions {
			fields = append(fields, action.Field)
		}
	}

	// Check each field
	for _, field := range fields {
		if !validFields[field] {
			result.IsValid = false

			// Find suggestion
			suggestion := v.findFieldSuggestion(field, validFields)
			details := fmt.Sprintf("Field '%s' is not available.", field)
			if suggestion != "" {
				details += fmt.Sprintf(" Did you mean '%s'?", suggestion)
			}

			result.Errors = append(result.Errors, ValidationError{
				Category:   ErrorCategoryField,
				ErrorType:  "unknown_field",
				Message:    fmt.Sprintf("Unknown field '%s'", field),
				Details:    details,
				Suggestion: v.getAvailableFieldsSuggestion(validFields),
			})
		}
	}
}

// findFieldSuggestion finds a similar field name if one exists.
func (v *Validator) findFieldSuggestion(field string, validFields map[string]bool) string {
	var best string
	bestScore := 0

	for valid := range validFields {
		score := similarity(field, valid)
		if score > bestScore && score >= 55 {
			bestScore = score
			best = valid
		}
	}

	return best
}

// similarity calculates a simple similarity score between two strings.
func similarity(a, b string) int {
	if a == b {
		return 100
	}

	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)

	// Character overlap
	aChars := make(map[rune]bool)
	for _, ch := range aLower {
		aChars[ch] = true
	}
	bChars := make(map[rune]bool)
	for _, ch := range bLower {
		bChars[ch] = true
	}

	common := 0
	for ch := range aChars {
		if bChars[ch] {
			common++
		}
	}

	maxLen := len(aLower)
	if len(bLower) > maxLen {
		maxLen = len(bLower)
	}
	if maxLen == 0 {
		return 0
	}

	return (common * 100) / maxLen
}

// getAvailableFieldsSuggestion returns a string listing available fields.
func (v *Validator) getAvailableFieldsSuggestion(validFields map[string]bool) string {
	var fields []string
	for field := range validFields {
		fields = append(fields, field)
	}
	return fmt.Sprintf("Available fields: %s", strings.Join(fields, ", "))
}

// expressionToMap converts a parsed expression to a map for JSON serialization.
func expressionToMap(parsed *ParsedExpression) map[string]any {
	if parsed == nil || parsed.Expression == nil {
		return nil
	}

	result := make(map[string]any)

	switch expr := parsed.Expression.(type) {
	case *ConditionOnly:
		if expr.Condition != nil && expr.Condition.Root != nil {
			result["type"] = "condition_only"
			result["condition"] = conditionNodeToMap(expr.Condition.Root)
		}
	case *ConditionWithActions:
		result["type"] = "condition_with_actions"
		if expr.Condition != nil && expr.Condition.Root != nil {
			result["condition"] = conditionNodeToMap(expr.Condition.Root)
		}
		var actions []map[string]any
		for _, action := range expr.Actions {
			actions = append(actions, map[string]any{
				"field":    action.Field,
				"operator": string(action.Operator),
				"value":    actionValueToString(action.Value),
			})
		}
		result["actions"] = actions
	}

	return result
}

// conditionNodeToMap converts a condition node to a map.
func conditionNodeToMap(node ConditionNode) map[string]any {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *Condition:
		return map[string]any{
			"type":           "condition",
			"field":          n.Field,
			"operator":       string(n.Operator),
			"value":          n.Value,
			"case_sensitive": n.CaseSensitive,
		}
	case *ConditionGroup:
		var children []map[string]any
		for _, child := range n.Children {
			children = append(children, conditionNodeToMap(child))
		}
		return map[string]any{
			"type":     "group",
			"operator": string(n.Operator),
			"children": children,
		}
	}

	return nil
}

// actionValueToString converts an action value to string representation.
func actionValueToString(val ActionValue) string {
	if val == nil {
		return ""
	}

	switch v := val.(type) {
	case *LiteralValue:
		return v.Value
	case *FieldReference:
		return "$" + v.Field
	case *CaptureReference:
		return fmt.Sprintf("$%d", v.Index)
	case *NullValue:
		return "null"
	}

	return ""
}
