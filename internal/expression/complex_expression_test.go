package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComplexNestedExpressions tests deeply nested condition groups.
func TestComplexNestedExpressions(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		description string
	}{
		{
			name:        "deeply nested OR within AND",
			expression:  `((channel_name contains "BBC" OR channel_name contains "ITV") AND (group_title equals "UK" OR group_title equals "Entertainment"))`,
			description: "Nested OR groups combined with AND",
		},
		{
			name:        "three-level nesting",
			expression:  `(((channel_name contains "News" AND language equals "en") OR channel_name contains "Sport") AND group_title not_equals "Adult")`,
			description: "Three levels of nested conditions",
		},
		{
			name:        "complex premium UK channels",
			expression:  `((channel_name matches "^(BBC|ITV|Channel [45])" AND tvg_id not_equals "") OR (channel_name matches "Sky (Sports|Movies|News)" AND group_title equals ""))`,
			description: "Complex regex with nested groups",
		},
		{
			name:        "multiple OR and AND mixed",
			expression:  `(channel_name contains "HD" OR channel_name contains "4K") AND (group_title equals "Movies" OR group_title equals "Sports") OR (stream_url starts_with "https" AND tvg_id matches "^[0-9]+$")`,
			description: "Complex mixed logical operators",
		},
		{
			name:        "four conditions with mixed operators",
			expression:  `channel_name contains "BBC" AND group_title equals "UK" OR channel_name contains "ITV" AND group_title equals "Entertainment"`,
			description: "Four conditions with AND/OR precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expression)
			require.NoError(t, err, "Failed to parse: %s", tt.description)
			require.NotNil(t, parsed)
			require.NotNil(t, parsed.Expression)
		})
	}
}

// TestComplexConditionsWithActions tests complex conditions combined with actions.
func TestComplexConditionsWithActions(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		wantFields []string
		wantOps    []ActionOperator
	}{
		{
			name:       "complex condition with single SET",
			expression: `(channel_name contains "sport" OR channel_name contains "football") AND language equals "en" SET group_title = "English Sports"`,
			wantFields: []string{"group_title"},
			wantOps:    []ActionOperator{ActionSet},
		},
		{
			name:       "nested conditions with multiple actions",
			expression: `(channel_name matches "BBC" AND group_title equals "") SET group_title = "BBC Channels", tvg_logo = "bbc.png"`,
			wantFields: []string{"group_title", "tvg_logo"},
			wantOps:    []ActionOperator{ActionSet, ActionSet},
		},
		{
			name:       "regex capture with transform",
			expression: `channel_name matches "(.+) \\+([0-9]+)" SET tvg_shift = "$2", channel_name = "$1"`,
			wantFields: []string{"tvg_shift", "channel_name"},
			wantOps:    []ActionOperator{ActionSet, ActionSet},
		},
		{
			name:       "complex condition with APPEND",
			expression: `(channel_name ends_with "HD" OR channel_name ends_with "FHD") AND group_title not_equals "" APPEND group_title = " [HD]"`,
			wantFields: []string{"group_title"},
			wantOps:    []ActionOperator{ActionAppend},
		},
		{
			name:       "pattern extraction with SET_IF_EMPTY",
			expression: `channel_name matches "\\[([A-Z]{2,3})\\] (.+)" SET_IF_EMPTY tvg_id = "$1"`,
			wantFields: []string{"tvg_id"},
			wantOps:    []ActionOperator{ActionSetIfEmpty},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expression)
			require.NoError(t, err)

			condActions, ok := parsed.Expression.(*ConditionWithActions)
			require.True(t, ok, "Expected ConditionWithActions")
			require.NotNil(t, condActions.Condition)
			require.Len(t, condActions.Actions, len(tt.wantFields))

			for i, action := range condActions.Actions {
				assert.Equal(t, tt.wantFields[i], action.Field)
				assert.Equal(t, tt.wantOps[i], action.Operator)
			}
		})
	}
}

// TestFilterExpressionEvaluation tests filter expressions with complex conditions.
func TestFilterExpressionEvaluation(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name     string
		expr     string
		data     map[string]string
		expected bool
	}{
		{
			name: "nested OR/AND matches",
			expr: `(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title equals "UK"`,
			data: map[string]string{
				"channel_name": "BBC One HD",
				"group_title":  "UK",
			},
			expected: true,
		},
		{
			name: "nested OR/AND fails on AND",
			expr: `(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title equals "UK"`,
			data: map[string]string{
				"channel_name": "BBC One HD",
				"group_title":  "Entertainment",
			},
			expected: false,
		},
		{
			name: "complex OR chain with single AND",
			expr: `channel_name contains "News" AND (group_title contains "UK" OR group_title contains "International" OR group_title contains "World")`,
			data: map[string]string{
				"channel_name": "BBC News",
				"group_title":  "International News",
			},
			expected: true,
		},
		{
			name: "three-level nesting evaluates correctly",
			expr: `((channel_name starts_with "BBC" AND language equals "en") OR channel_name starts_with "CNN") AND group_title not_equals "Adult"`,
			data: map[string]string{
				"channel_name": "BBC World News",
				"language":     "en",
				"group_title":  "News",
			},
			expected: true,
		},
		{
			name: "complex regex with AND",
			expr: `channel_name matches "^(BBC|ITV).*HD$" AND group_title equals "UK Entertainment"`,
			data: map[string]string{
				"channel_name": "BBC One HD",
				"group_title":  "UK Entertainment",
			},
			expected: true,
		},
		{
			name: "NOT on single condition",
			expr: `NOT channel_name contains "Adult"`,
			data: map[string]string{
				"channel_name": "BBC One",
				"group_title":  "Entertainment",
			},
			expected: true,
		},
		{
			name: "multiple NOT conditions with AND",
			expr: `NOT channel_name contains "Adult" AND NOT group_title contains "XXX"`,
			data: map[string]string{
				"channel_name": "BBC One",
				"group_title":  "Entertainment",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			accessor := newMockAccessor(tt.data)
			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches, "Expression: %s", tt.expr)
		})
	}
}

// TestDataMappingWithComplexConditions tests data mapping rules with complex conditions.
func TestDataMappingWithComplexConditions(t *testing.T) {
	t.Run("complex condition modifies field", func(t *testing.T) {
		engine := NewDataMappingEngine()

		rule, err := Parse(`(channel_name contains "BBC" AND group_title equals "") OR channel_name contains "ITV" SET group_title = "UK TV"`)
		require.NoError(t, err)
		engine.AddRule(rule)

		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One",
			"group_title":  "",
		})

		result, err := engine.Process(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, result.RulesMatched)

		value, _ := ctx.GetFieldValue("group_title")
		assert.Equal(t, "UK TV", value)
	})

	t.Run("regex capture with nested conditions", func(t *testing.T) {
		engine := NewDataMappingEngine()

		rule, err := Parse(`channel_name matches "(.+) HD$" AND group_title equals "" SET channel_name = "$1", group_title = "HD Channels"`)
		require.NoError(t, err)
		engine.AddRule(rule)

		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One HD",
			"group_title":  "",
		})

		result, err := engine.Process(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, result.RulesMatched)

		name, _ := ctx.GetFieldValue("channel_name")
		group, _ := ctx.GetFieldValue("group_title")
		assert.Equal(t, "BBC One", name)
		assert.Equal(t, "HD Channels", group)
	})

	t.Run("multiple complex rules in sequence", func(t *testing.T) {
		engine := NewDataMappingEngine()

		// Rule 1: Classify UK channels
		rule1, _ := Parse(`(channel_name starts_with "BBC" OR channel_name starts_with "ITV" OR channel_name starts_with "Channel") SET group_title = "UK Terrestrial"`)
		// Rule 2: Add quality suffix for HD
		rule2, _ := Parse(`channel_name ends_with "HD" APPEND group_title = " [HD]"`)
		// Rule 3: Set logo based on channel prefix
		rule3, _ := Parse(`channel_name starts_with "BBC" SET_IF_EMPTY tvg_logo = "bbc.png"`)

		engine.AddRule(rule1)
		engine.AddRule(rule2)
		engine.AddRule(rule3)

		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One HD",
			"group_title":  "",
			"tvg_logo":     "",
		})

		result, err := engine.Process(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, result.RulesMatched)

		group, _ := ctx.GetFieldValue("group_title")
		logo, _ := ctx.GetFieldValue("tvg_logo")
		assert.Equal(t, "UK Terrestrial [HD]", group)
		assert.Equal(t, "bbc.png", logo)
	})

	t.Run("exclusion with multiple NOT conditions", func(t *testing.T) {
		engine := NewDataMappingEngine()

		// Only apply to non-adult, non-PPV channels using multiple NOT conditions
		rule, err := Parse(`NOT channel_name contains "Adult" AND NOT channel_name contains "PPV" AND NOT group_title contains "XXX" SET approved = "true"`)
		require.NoError(t, err)
		engine.AddRule(rule)

		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One",
			"group_title":  "Entertainment",
			"approved":     "",
		})

		result, err := engine.Process(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, result.RulesMatched)

		approved, _ := ctx.GetFieldValue("approved")
		assert.Equal(t, "true", approved)
	})
}

// TestFilterEvaluation tests filter expressions using the evaluator directly.
func TestFilterEvaluation(t *testing.T) {
	t.Run("complex filter includes matching channels", func(t *testing.T) {
		filter, err := Parse(`(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title not_equals "Adult"`)
		require.NoError(t, err)

		evaluator := NewEvaluator()

		channels := []map[string]string{
			{"channel_name": "BBC One", "group_title": "UK"},
			{"channel_name": "ITV", "group_title": "UK"},
			{"channel_name": "CNN", "group_title": "News"},
			{"channel_name": "BBC Adult", "group_title": "Adult"}, // Should be excluded
		}

		var included []string
		for _, ch := range channels {
			ctx := NewChannelEvalContext(ch)
			result, err := evaluator.Evaluate(filter, ctx)
			require.NoError(t, err)

			if result.Matches {
				included = append(included, ch["channel_name"])
			}
		}

		assert.ElementsMatch(t, []string{"BBC One", "ITV"}, included)
	})

	t.Run("multi-condition filter evaluation", func(t *testing.T) {
		// Filter must be HD AND UK content
		filter, _ := Parse(`channel_name ends_with "HD" AND group_title contains "UK"`)

		evaluator := NewEvaluator()

		channels := []map[string]string{
			{"channel_name": "BBC One HD", "group_title": "UK Entertainment"},
			{"channel_name": "BBC One", "group_title": "UK Entertainment"},   // No HD
			{"channel_name": "CNN HD", "group_title": "US News"},             // Not UK
			{"channel_name": "ITV HD", "group_title": "UK Sports"},           // Both match
		}

		var included []string
		for _, ch := range channels {
			ctx := NewChannelEvalContext(ch)
			result, err := evaluator.Evaluate(filter, ctx)
			require.NoError(t, err)

			if result.Matches {
				included = append(included, ch["channel_name"])
			}
		}

		assert.ElementsMatch(t, []string{"BBC One HD", "ITV HD"}, included)
	})
}

// TestEdgeCases tests edge cases in expression parsing and evaluation.
func TestEdgeCases(t *testing.T) {
	t.Run("empty string matches empty value", func(t *testing.T) {
		parsed, err := Parse(`group_title equals ""`)
		require.NoError(t, err)

		evaluator := NewEvaluator()
		accessor := newMockAccessor(map[string]string{
			"group_title": "",
		})

		result, err := evaluator.Evaluate(parsed, accessor)
		require.NoError(t, err)
		assert.True(t, result.Matches)
	})

	t.Run("regex with special characters", func(t *testing.T) {
		parsed, err := Parse(`channel_name matches "\\[([A-Z]+)\\] (.+)"`)
		require.NoError(t, err)

		evaluator := NewEvaluator()
		accessor := newMockAccessor(map[string]string{
			"channel_name": "[UK] BBC One",
		})

		result, err := evaluator.Evaluate(parsed, accessor)
		require.NoError(t, err)
		assert.True(t, result.Matches)
	})

	t.Run("missing field evaluates to empty string", func(t *testing.T) {
		parsed, err := Parse(`nonexistent_field equals ""`)
		require.NoError(t, err)

		evaluator := NewEvaluator()
		accessor := newMockAccessor(map[string]string{
			"channel_name": "BBC One",
		})

		result, err := evaluator.Evaluate(parsed, accessor)
		require.NoError(t, err)
		assert.True(t, result.Matches) // Missing field treated as empty
	})

	t.Run("case sensitivity in matches", func(t *testing.T) {
		evaluator := NewEvaluator()

		// Default case-sensitive
		parsed, _ := Parse(`channel_name contains "bbc"`)
		accessor := newMockAccessor(map[string]string{"channel_name": "BBC One"})

		result, _ := evaluator.Evaluate(parsed, accessor)
		assert.False(t, result.Matches, "Should be case-sensitive by default")

		// With case-insensitive flag
		evaluator = NewEvaluator()
		evaluator.SetCaseSensitive(false)
		result, _ = evaluator.Evaluate(parsed, accessor)
		assert.True(t, result.Matches, "Should match with case-insensitive mode")
	})
}

// TestRealWorldExpressions tests expressions that might be used in real deployments.
func TestRealWorldExpressions(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name        string
		expression  string
		description string
		data        map[string]string
		expected    bool
	}{
		{
			name:        "UK terrestrial channels filter",
			expression:  `channel_name matches "^(BBC|ITV|Channel [45]|More4|E4|Film4)" AND group_title not_contains "Adult"`,
			description: "Match UK terrestrial channels excluding adult",
			data:        map[string]string{"channel_name": "BBC One HD", "group_title": "UK Entertainment"},
			expected:    true,
		},
		{
			name:        "sports channels across groups",
			expression:  `group_title matches "(?i)sport" OR channel_name matches "(?i)(sky sports|bt sport|eurosport|espn)"`,
			description: "Match sports channels by name or group",
			data:        map[string]string{"channel_name": "Sky Sports Main Event", "group_title": "Premium"},
			expected:    true,
		},
		{
			name:        "HD channels only",
			expression:  `channel_name matches "(HD|FHD|4K|UHD|2160p|1080p)$" OR channel_name matches " (HD|FHD|4K|UHD) "`,
			description: "Match high-definition channels",
			data:        map[string]string{"channel_name": "BBC One HD"},
			expected:    true,
		},
		{
			name:        "exclude unwanted channels using NOT conditions",
			expression:  `NOT channel_name matches "(?i)(test|demo|sample|backup|offline)" AND NOT group_title contains "XXX" AND NOT stream_url contains "localhost"`,
			description: "Exclude test, demo, and adult channels using multiple NOT",
			data:        map[string]string{"channel_name": "BBC One", "group_title": "UK", "stream_url": "http://example.com/live"},
			expected:    true,
		},
		{
			name:        "language-specific content",
			expression:  `(language equals "en" OR language equals "") AND (channel_name matches "^(BBC|ITV|Sky)" OR group_title contains "UK")`,
			description: "Match English-language UK content",
			data:        map[string]string{"channel_name": "BBC One", "language": "en", "group_title": "Entertainment"},
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expression)
			require.NoError(t, err, "Failed to parse: %s", tt.description)

			accessor := newMockAccessor(tt.data)
			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches, "%s: Expression: %s", tt.description, tt.expression)
		})
	}
}
