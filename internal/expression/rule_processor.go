package expression

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jmylchreest/tvarr/internal/expression/helpers"
)

// FieldModification records a modification made to a field.
type FieldModification struct {
	Field    string         // Field name that was modified
	OldValue string         // Previous value
	NewValue string         // New value
	Action   ActionOperator // Action that was performed
}

// RuleResult contains the result of applying a rule.
type RuleResult struct {
	// Matched indicates whether the rule's condition matched.
	Matched bool

	// Modifications lists all field modifications made.
	Modifications []FieldModification

	// Captures contains regex capture groups if any.
	Captures []string
}

// ModifiableContext extends FieldValueAccessor with the ability to set field values.
type ModifiableContext interface {
	FieldValueAccessor
	SetFieldValue(name, value string)
}

// RuleProcessor applies rules (conditions + actions) to records.
type RuleProcessor struct {
	evaluator *Evaluator
}

// NewRuleProcessor creates a new rule processor.
func NewRuleProcessor() *RuleProcessor {
	return &RuleProcessor{
		evaluator: NewEvaluator(),
	}
}

// Apply applies a parsed expression (rule) to a context.
// Returns the result including whether the condition matched and any modifications made.
func (p *RuleProcessor) Apply(parsed *ParsedExpression, ctx ModifiableContext) (*RuleResult, error) {
	if parsed == nil || parsed.Expression == nil {
		return &RuleResult{Matched: true}, nil
	}

	// Evaluate the condition
	evalResult, err := p.evaluator.Evaluate(parsed, ctx)
	if err != nil {
		return nil, fmt.Errorf("condition evaluation failed: %w", err)
	}

	result := &RuleResult{
		Matched:  evalResult.Matches,
		Captures: evalResult.Captures,
	}

	// If condition didn't match, no actions to apply
	if !evalResult.Matches {
		return result, nil
	}

	// Apply actions if present
	switch expr := parsed.Expression.(type) {
	case *ConditionWithActions:
		modifications, err := p.applyActions(expr.Actions, ctx, evalResult.Captures)
		if err != nil {
			return nil, err
		}
		result.Modifications = modifications

	case *ConditionOnly:
		// No actions to apply
	}

	return result, nil
}

// applyActions applies a list of actions to the context.
func (p *RuleProcessor) applyActions(actions []*Action, ctx ModifiableContext, captures []string) ([]FieldModification, error) {
	var modifications []FieldModification

	for _, action := range actions {
		mod, applied, err := p.applyAction(action, ctx, captures)
		if err != nil {
			return nil, err
		}
		if applied {
			modifications = append(modifications, mod)
		}
	}

	return modifications, nil
}

// applyAction applies a single action to the context.
// Returns the modification, whether it was applied, and any error.
func (p *RuleProcessor) applyAction(action *Action, ctx ModifiableContext, captures []string) (FieldModification, bool, error) {
	field := action.Field
	oldValue, _ := ctx.GetFieldValue(field)

	var newValue string
	var err error

	switch action.Operator {
	case ActionSet:
		newValue, err = p.resolveValue(action.Value, ctx, captures)
		if err != nil {
			return FieldModification{}, false, err
		}

	case ActionSetIfEmpty:
		if oldValue != "" {
			// Field is not empty, don't modify
			return FieldModification{}, false, nil
		}
		newValue, err = p.resolveValue(action.Value, ctx, captures)
		if err != nil {
			return FieldModification{}, false, err
		}

	case ActionAppend:
		appendValue, err := p.resolveValue(action.Value, ctx, captures)
		if err != nil {
			return FieldModification{}, false, err
		}
		newValue = oldValue + appendValue

	case ActionRemove:
		removeValue, err := p.resolveValue(action.Value, ctx, captures)
		if err != nil {
			return FieldModification{}, false, err
		}
		newValue = strings.ReplaceAll(oldValue, removeValue, "")

	case ActionDelete:
		newValue = ""

	default:
		return FieldModification{}, false, fmt.Errorf("unsupported action operator: %s", action.Operator)
	}

	// Apply the modification
	ctx.SetFieldValue(field, newValue)

	return FieldModification{
		Field:    field,
		OldValue: oldValue,
		NewValue: newValue,
		Action:   action.Operator,
	}, true, nil
}

// resolveValue resolves an action value to a string.
func (p *RuleProcessor) resolveValue(value ActionValue, ctx ModifiableContext, captures []string) (string, error) {
	if value == nil {
		return "", nil
	}

	switch v := value.(type) {
	case *LiteralValue:
		// Check if the literal contains capture references and substitute them
		result, err := p.substituteCaptureReferences(v.Value, captures)
		if err != nil {
			return "", err
		}

		// Process immediate helpers (e.g., @time:now) while leaving deferred helpers
		// (e.g., @logo:ULID) for later pipeline stages
		return helpers.ProcessImmediateHelpers(result)

	case *NullValue:
		return "", nil

	case *FieldReference:
		fieldValue, _ := ctx.GetFieldValue(v.Field)
		return fieldValue, nil

	case *CaptureReference:
		if v.Index < 0 || v.Index >= len(captures) {
			return "", nil
		}
		return captures[v.Index], nil

	default:
		return "", fmt.Errorf("unsupported value type: %T", value)
	}
}

// substituteCaptureReferences replaces $1, $2, etc. with capture group values.
func (p *RuleProcessor) substituteCaptureReferences(value string, captures []string) (string, error) {
	if len(captures) == 0 {
		return value, nil
	}

	// Pattern to match $1, $2, etc.
	re := regexp.MustCompile(`\$(\d+)`)

	result := re.ReplaceAllStringFunc(value, func(match string) string {
		idxStr := match[1:] // Remove $
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(captures) {
			return match // Keep original if invalid
		}
		return captures[idx]
	})

	return result, nil
}
