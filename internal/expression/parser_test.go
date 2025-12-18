package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_EmptyExpression(t *testing.T) {
	parsed, err := Parse("")
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)
	assert.Nil(t, condOnly.Condition)
}

func TestParser_SimpleCondition(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		field    string
		operator FilterOperator
		value    string
	}{
		{
			name:     "equals with quoted string",
			input:    `channel_name equals "BBC One"`,
			field:    "channel_name",
			operator: OpEquals,
			value:    "BBC One",
		},
		{
			name:     "contains",
			input:    `group_title contains "Sport"`,
			field:    "group_title",
			operator: OpContains,
			value:    "Sport",
		},
		{
			name:     "starts_with",
			input:    `channel_name starts_with "BBC"`,
			field:    "channel_name",
			operator: OpStartsWith,
			value:    "BBC",
		},
		{
			name:     "ends_with",
			input:    `stream_url ends_with ".m3u8"`,
			field:    "stream_url",
			operator: OpEndsWith,
			value:    ".m3u8",
		},
		{
			name:     "matches regex",
			input:    `channel_name matches "BBC.*"`,
			field:    "channel_name",
			operator: OpMatches,
			value:    "BBC.*",
		},
		{
			name:     "not_equals",
			input:    `group_title not_equals "Adult"`,
			field:    "group_title",
			operator: OpNotEquals,
			value:    "Adult",
		},
		{
			name:     "not_contains",
			input:    `channel_name not_contains "XXX"`,
			field:    "channel_name",
			operator: OpNotContains,
			value:    "XXX",
		},
		{
			name:     "numeric comparison",
			input:    `channel_number greater_than "100"`,
			field:    "channel_number",
			operator: OpGreaterThan,
			value:    "100",
		},
		{
			name:     "alias eq",
			input:    `channel_name eq "BBC"`,
			field:    "channel_name",
			operator: OpEquals,
			value:    "BBC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			require.NoError(t, err)

			condOnly, ok := parsed.Expression.(*ConditionOnly)
			require.True(t, ok)
			require.NotNil(t, condOnly.Condition)
			require.NotNil(t, condOnly.Condition.Root)

			cond, ok := condOnly.Condition.Root.(*Condition)
			require.True(t, ok)
			assert.Equal(t, tt.field, cond.Field)
			assert.Equal(t, tt.operator, cond.Operator)
			assert.Equal(t, tt.value, cond.Value)
		})
	}
}

func TestParser_AndCondition(t *testing.T) {
	input := `channel_name contains "BBC" AND group_title equals "UK"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	group, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalAnd, group.Operator)
	assert.Len(t, group.Children, 2)

	c1, ok := group.Children[0].(*Condition)
	require.True(t, ok)
	assert.Equal(t, "channel_name", c1.Field)
	assert.Equal(t, OpContains, c1.Operator)
	assert.Equal(t, "BBC", c1.Value)

	c2, ok := group.Children[1].(*Condition)
	require.True(t, ok)
	assert.Equal(t, "group_title", c2.Field)
	assert.Equal(t, OpEquals, c2.Operator)
	assert.Equal(t, "UK", c2.Value)
}

func TestParser_OrCondition(t *testing.T) {
	input := `channel_name contains "BBC" OR channel_name contains "ITV"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	group, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalOr, group.Operator)
	assert.Len(t, group.Children, 2)
}

func TestParser_NotCondition(t *testing.T) {
	input := `NOT channel_name contains "Adult"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	cond, ok := condOnly.Condition.Root.(*Condition)
	require.True(t, ok)
	assert.Equal(t, "channel_name", cond.Field)
	assert.Equal(t, OpNotContains, cond.Operator) // NOT contains = not_contains
	assert.Equal(t, "Adult", cond.Value)
}

func TestParser_ParenthesesGrouping(t *testing.T) {
	input := `(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title equals "UK"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	// Should be AND at the top level
	andGroup, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalAnd, andGroup.Operator)
	assert.Len(t, andGroup.Children, 2)

	// First child should be OR group
	orGroup, ok := andGroup.Children[0].(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalOr, orGroup.Operator)
	assert.Len(t, orGroup.Children, 2)

	// Second child should be simple condition
	cond, ok := andGroup.Children[1].(*Condition)
	require.True(t, ok)
	assert.Equal(t, "group_title", cond.Field)
}

func TestParser_OperatorPrecedence(t *testing.T) {
	// AND should bind tighter than OR
	// a OR b AND c should be a OR (b AND c)
	input := `a equals "1" OR b equals "2" AND c equals "3"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	// Top level should be OR
	orGroup, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalOr, orGroup.Operator)
	assert.Len(t, orGroup.Children, 2)

	// First child is simple condition
	c1, ok := orGroup.Children[0].(*Condition)
	require.True(t, ok)
	assert.Equal(t, "a", c1.Field)

	// Second child should be AND group
	andGroup, ok := orGroup.Children[1].(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalAnd, andGroup.Operator)
}

func TestParser_MultipleAnd(t *testing.T) {
	input := `a equals "1" AND b equals "2" AND c equals "3"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	// Should flatten into single AND group with 3 children
	andGroup, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalAnd, andGroup.Operator)
	assert.Len(t, andGroup.Children, 3)
}

func TestParser_MultipleOr(t *testing.T) {
	input := `a equals "1" OR b equals "2" OR c equals "3"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	// Should flatten into single OR group with 3 children
	orGroup, ok := condOnly.Condition.Root.(*ConditionGroup)
	require.True(t, ok)
	assert.Equal(t, LogicalOr, orGroup.Operator)
	assert.Len(t, orGroup.Children, 3)
}

func TestParser_ConditionWithActions(t *testing.T) {
	input := `channel_name contains "BBC" SET group_title = "UK Channels"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	// Check condition
	cond, ok := condActions.Condition.Root.(*Condition)
	require.True(t, ok)
	assert.Equal(t, "channel_name", cond.Field)
	assert.Equal(t, OpContains, cond.Operator)
	assert.Equal(t, "BBC", cond.Value)

	// Check actions
	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "group_title", condActions.Actions[0].Field)
	assert.Equal(t, ActionSet, condActions.Actions[0].Operator)

	lit, ok := condActions.Actions[0].Value.(*LiteralValue)
	require.True(t, ok)
	assert.Equal(t, "UK Channels", lit.Value)

	// Check metadata
	assert.True(t, parsed.HasActions)
	assert.Contains(t, parsed.ModifiedFields, "group_title")
}

func TestParser_MultipleActions(t *testing.T) {
	input := `channel_name contains "BBC" SET group_title = "UK", logo = "bbc.png"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	require.Len(t, condActions.Actions, 2)
	assert.Equal(t, "group_title", condActions.Actions[0].Field)
	assert.Equal(t, "logo", condActions.Actions[1].Field)
}

func TestParser_ActionOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		operator ActionOperator
	}{
		{
			name:     "SET",
			input:    `field equals "x" SET target = "value"`,
			operator: ActionSet,
		},
		{
			name:     "SET_IF_EMPTY",
			input:    `field equals "x" SET_IF_EMPTY target = "value"`,
			operator: ActionSetIfEmpty,
		},
		{
			name:     "APPEND",
			input:    `field equals "x" APPEND target = " suffix"`,
			operator: ActionAppend,
		},
		{
			name:     "REMOVE",
			input:    `field equals "x" REMOVE target = "unwanted"`,
			operator: ActionRemove,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			require.NoError(t, err)

			condActions, ok := parsed.Expression.(*ConditionWithActions)
			require.True(t, ok)

			require.Len(t, condActions.Actions, 1)
			assert.Equal(t, tt.operator, condActions.Actions[0].Operator)
		})
	}
}

func TestParser_DeleteAction(t *testing.T) {
	input := `channel_name contains "XXX" DELETE unwanted_field`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "unwanted_field", condActions.Actions[0].Field)
	assert.Equal(t, ActionDelete, condActions.Actions[0].Operator)
	assert.Nil(t, condActions.Actions[0].Value)
}

func TestParser_CaptureReference(t *testing.T) {
	input := `channel_name matches "(.+) HD" SET channel_name = $1`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	require.Len(t, condActions.Actions, 1)
	capture, ok := condActions.Actions[0].Value.(*CaptureReference)
	require.True(t, ok)
	assert.Equal(t, 1, capture.Index)
}

func TestParser_FieldReference(t *testing.T) {
	input := `field equals "x" SET target = $source_field`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	require.Len(t, condActions.Actions, 1)
	fieldRef, ok := condActions.Actions[0].Value.(*FieldReference)
	require.True(t, ok)
	assert.Equal(t, "source_field", fieldRef.Field)
}

func TestParser_Metadata(t *testing.T) {
	t.Run("referenced fields", func(t *testing.T) {
		input := `channel_name contains "BBC" AND group_title equals "UK"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.Contains(t, parsed.ReferencedFields, "channel_name")
		assert.Contains(t, parsed.ReferencedFields, "group_title")
	})

	t.Run("uses regex", func(t *testing.T) {
		input := `channel_name matches "BBC.*"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.True(t, parsed.UsesRegex)
	})

	t.Run("no regex", func(t *testing.T) {
		input := `channel_name contains "BBC"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.False(t, parsed.UsesRegex)
	})

	t.Run("has actions", func(t *testing.T) {
		input := `channel_name contains "BBC" SET group = "UK"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.True(t, parsed.HasActions)
	})

	t.Run("no actions", func(t *testing.T) {
		input := `channel_name contains "BBC"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.False(t, parsed.HasActions)
	})

	t.Run("original preserved", func(t *testing.T) {
		input := `channel_name contains "BBC"`
		parsed, err := Parse(input)
		require.NoError(t, err)

		assert.Equal(t, input, parsed.Original)
	})
}

func TestParser_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "missing value",
			input: `channel_name equals`,
		},
		{
			name:  "invalid operator",
			input: `channel_name invalid_op "value"`,
		},
		{
			name:  "missing operator",
			input: `channel_name "value"`,
		},
		{
			name:  "unclosed parenthesis",
			input: `(channel_name equals "x"`,
		},
		{
			name:  "missing SET value",
			input: `field equals "x" SET target =`,
		},
		{
			name:  "dangling AND",
			input: `field equals "x" AND`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestMustParse_Success(t *testing.T) {
	parsed := MustParse(`channel_name equals "BBC"`)
	assert.NotNil(t, parsed)
}

func TestMustParse_Panic(t *testing.T) {
	assert.Panics(t, func() {
		MustParse(`invalid expression (((`)
	})
}

func TestParser_NumberValue(t *testing.T) {
	input := `channel_number greater_than 100`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	cond, ok := condOnly.Condition.Root.(*Condition)
	require.True(t, ok)
	assert.Equal(t, "100", cond.Value)
}

func TestParser_UnquotedValue(t *testing.T) {
	// Allow unquoted simple values for compatibility
	input := `channel_name equals SimpleValue`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condOnly, ok := parsed.Expression.(*ConditionOnly)
	require.True(t, ok)

	cond, ok := condOnly.Condition.Root.(*Condition)
	require.True(t, ok)
	assert.Equal(t, "SimpleValue", cond.Value)
}

func TestParser_BooleanActionValues(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedValue string
	}{
		{
			name:          "true value",
			input:         `group_title matches "(?i)adult" SET is_adult = true`,
			expectedValue: "true",
		},
		{
			name:          "false value",
			input:         `channel_name contains "kids" SET is_adult = false`,
			expectedValue: "false",
		},
		{
			name:          "TRUE uppercase",
			input:         `group_title matches "adult" SET is_adult = TRUE`,
			expectedValue: "true",
		},
		{
			name:          "FALSE uppercase",
			input:         `channel_name contains "kids" SET is_adult = FALSE`,
			expectedValue: "false",
		},
		{
			name:          "multiple actions with boolean",
			input:         `group_title matches "adult" SET group_title = "Adult", is_adult = true`,
			expectedValue: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			require.NoError(t, err, "Failed to parse: %s", tt.input)

			condActions, ok := parsed.Expression.(*ConditionWithActions)
			require.True(t, ok, "Expected ConditionWithActions")
			require.NotEmpty(t, condActions.Actions)

			// Find the is_adult action
			var foundAction *Action
			for _, action := range condActions.Actions {
				if action.Field == "is_adult" {
					foundAction = action
					break
				}
			}
			require.NotNil(t, foundAction, "Expected is_adult action")

			litVal, ok := foundAction.Value.(*LiteralValue)
			require.True(t, ok, "Expected LiteralValue")
			assert.Equal(t, tt.expectedValue, litVal.Value)
		})
	}
}

func TestParser_ComplexExpression(t *testing.T) {
	// Test a complex real-world expression
	input := `(channel_name contains "BBC" OR channel_name contains "ITV") AND NOT group_title contains "Adult" SET group_title = "UK Channels", logo = "uk.png"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok)

	assert.NotNil(t, condActions.Condition)
	assert.Len(t, condActions.Actions, 2)
	assert.True(t, parsed.HasActions)
}

// Tests for action shorthand syntax (T490-T495)

func TestParser_ImplicitSetShorthand(t *testing.T) {
	// Implicit SET: field = "value" instead of SET field = "value"
	input := `channel_name contains "BBC" tvg_logo = "bbc_logo.png"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "tvg_logo", condActions.Actions[0].Field)
	assert.Equal(t, ActionSet, condActions.Actions[0].Operator)

	lit, ok := condActions.Actions[0].Value.(*LiteralValue)
	require.True(t, ok)
	assert.Equal(t, "bbc_logo.png", lit.Value)
}

func TestParser_SetIfEmptyShorthand(t *testing.T) {
	// SET_IF_EMPTY shorthand: field ?= "value"
	input := `channel_name contains "BBC" tvg_logo ?= "default.png"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "tvg_logo", condActions.Actions[0].Field)
	assert.Equal(t, ActionSetIfEmpty, condActions.Actions[0].Operator)

	lit, ok := condActions.Actions[0].Value.(*LiteralValue)
	require.True(t, ok)
	assert.Equal(t, "default.png", lit.Value)
}

func TestParser_AppendShorthand(t *testing.T) {
	// APPEND shorthand: field += "value"
	input := `channel_name contains "HD" tvg_name += " HD"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "tvg_name", condActions.Actions[0].Field)
	assert.Equal(t, ActionAppend, condActions.Actions[0].Operator)

	lit, ok := condActions.Actions[0].Value.(*LiteralValue)
	require.True(t, ok)
	assert.Equal(t, " HD", lit.Value)
}

func TestParser_RemoveShorthand(t *testing.T) {
	// REMOVE shorthand: field -= "value"
	input := `channel_name contains "UK" tvg_name -= "[UK]"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 1)
	assert.Equal(t, "tvg_name", condActions.Actions[0].Field)
	assert.Equal(t, ActionRemove, condActions.Actions[0].Operator)

	lit, ok := condActions.Actions[0].Value.(*LiteralValue)
	require.True(t, ok)
	assert.Equal(t, "[UK]", lit.Value)
}

func TestParser_MixedShorthandAndKeyword(t *testing.T) {
	// Mix of shorthand and keyword syntax
	input := `channel_name contains "BBC" SET group_title = "UK" tvg_logo ?= "default.png"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 2)

	// First action: SET group_title = "UK"
	assert.Equal(t, "group_title", condActions.Actions[0].Field)
	assert.Equal(t, ActionSet, condActions.Actions[0].Operator)

	// Second action: tvg_logo ?= "default.png"
	assert.Equal(t, "tvg_logo", condActions.Actions[1].Field)
	assert.Equal(t, ActionSetIfEmpty, condActions.Actions[1].Operator)
}

func TestParser_MultipleShorthandActions(t *testing.T) {
	// Multiple shorthand actions in a row
	input := `channel_name contains "BBC" tvg_logo = "logo.png" tvg_name += " HD" group_title ?= "Default"`

	parsed, err := Parse(input)
	require.NoError(t, err)

	condActions, ok := parsed.Expression.(*ConditionWithActions)
	require.True(t, ok, "expected ConditionWithActions")

	require.Len(t, condActions.Actions, 3)

	assert.Equal(t, "tvg_logo", condActions.Actions[0].Field)
	assert.Equal(t, ActionSet, condActions.Actions[0].Operator)

	assert.Equal(t, "tvg_name", condActions.Actions[1].Field)
	assert.Equal(t, ActionAppend, condActions.Actions[1].Operator)

	assert.Equal(t, "group_title", condActions.Actions[2].Field)
	assert.Equal(t, ActionSetIfEmpty, condActions.Actions[2].Operator)
}

func TestParser_SymbolicOperators(t *testing.T) {
	// Test symbolic operators == and != with boolean values
	tests := []struct {
		name     string
		input    string
		field    string
		operator FilterOperator
		value    string
	}{
		{
			name:     "equals with true",
			input:    `is_adult == true`,
			field:    "is_adult",
			operator: OpEquals,
			value:    "true",
		},
		{
			name:     "equals with false",
			input:    `is_adult == false`,
			field:    "is_adult",
			operator: OpEquals,
			value:    "false",
		},
		{
			name:     "not equals with true",
			input:    `is_adult != true`,
			field:    "is_adult",
			operator: OpNotEquals,
			value:    "true",
		},
		{
			name:     "not equals with false",
			input:    `is_adult != false`,
			field:    "is_adult",
			operator: OpNotEquals,
			value:    "false",
		},
		{
			name:     "equals with string",
			input:    `channel_name == "BBC One"`,
			field:    "channel_name",
			operator: OpEquals,
			value:    "BBC One",
		},
		{
			name:     "not equals with string",
			input:    `group_title != "Adult"`,
			field:    "group_title",
			operator: OpNotEquals,
			value:    "Adult",
		},
		{
			name:     "combined with OR",
			input:    `is_adult == true OR group_title matches "(?i)\\b(adult|xxx)"`,
			field:    "is_adult",
			operator: OpEquals,
			value:    "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			require.NoError(t, err)

			condOnly, ok := parsed.Expression.(*ConditionOnly)
			require.True(t, ok, "expected ConditionOnly")
			require.NotNil(t, condOnly.Condition)
			require.NotNil(t, condOnly.Condition.Root)

			// Handle the combined OR case
			if tt.name == "combined with OR" {
				group, ok := condOnly.Condition.Root.(*ConditionGroup)
				require.True(t, ok, "expected ConditionGroup for OR")
				require.Len(t, group.Children, 2)

				cond, ok := group.Children[0].(*Condition)
				require.True(t, ok)
				assert.Equal(t, tt.field, cond.Field)
				assert.Equal(t, tt.operator, cond.Operator)
				assert.Equal(t, tt.value, cond.Value)
			} else {
				cond, ok := condOnly.Condition.Root.(*Condition)
				require.True(t, ok)
				assert.Equal(t, tt.field, cond.Field)
				assert.Equal(t, tt.operator, cond.Operator)
				assert.Equal(t, tt.value, cond.Value)
			}
		})
	}
}

func TestParser_ShorthandBackwardCompatibility(t *testing.T) {
	// Ensure keyword syntax still works with shorthand enabled
	tests := []struct {
		name     string
		input    string
		field    string
		operator ActionOperator
	}{
		{
			name:     "SET keyword",
			input:    `channel_name contains "BBC" SET tvg_logo = "logo.png"`,
			field:    "tvg_logo",
			operator: ActionSet,
		},
		{
			name:     "SET_IF_EMPTY keyword",
			input:    `channel_name contains "BBC" SET_IF_EMPTY tvg_logo = "logo.png"`,
			field:    "tvg_logo",
			operator: ActionSetIfEmpty,
		},
		{
			name:     "APPEND keyword",
			input:    `channel_name contains "BBC" APPEND tvg_name = " HD"`,
			field:    "tvg_name",
			operator: ActionAppend,
		},
		{
			name:     "REMOVE keyword",
			input:    `channel_name contains "BBC" REMOVE tvg_name = "[UK]"`,
			field:    "tvg_name",
			operator: ActionRemove,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			require.NoError(t, err)

			condActions, ok := parsed.Expression.(*ConditionWithActions)
			require.True(t, ok, "expected ConditionWithActions")

			require.Len(t, condActions.Actions, 1)
			assert.Equal(t, tt.field, condActions.Actions[0].Field)
			assert.Equal(t, tt.operator, condActions.Actions[0].Operator)
		})
	}
}
