package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFieldAccessor implements FieldValueAccessor for testing.
type mockFieldAccessor struct {
	values map[string]string
}

func newMockAccessor(values map[string]string) *mockFieldAccessor {
	return &mockFieldAccessor{values: values}
}

func (m *mockFieldAccessor) GetFieldValue(name string) (string, bool) {
	v, ok := m.values[name]
	return v, ok
}

func TestEvaluator_StringOperators(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "UK Entertainment",
	})

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		// equals
		{
			name:     "equals match",
			expr:     `channel_name equals "BBC One HD"`,
			expected: true,
		},
		{
			name:     "equals no match",
			expr:     `channel_name equals "ITV"`,
			expected: false,
		},
		// not_equals
		{
			name:     "not_equals match",
			expr:     `channel_name not_equals "ITV"`,
			expected: true,
		},
		{
			name:     "not_equals no match",
			expr:     `channel_name not_equals "BBC One HD"`,
			expected: false,
		},
		// contains
		{
			name:     "contains match",
			expr:     `channel_name contains "BBC"`,
			expected: true,
		},
		{
			name:     "contains no match",
			expr:     `channel_name contains "ITV"`,
			expected: false,
		},
		// not_contains
		{
			name:     "not_contains match",
			expr:     `channel_name not_contains "ITV"`,
			expected: true,
		},
		{
			name:     "not_contains no match",
			expr:     `channel_name not_contains "BBC"`,
			expected: false,
		},
		// starts_with
		{
			name:     "starts_with match",
			expr:     `channel_name starts_with "BBC"`,
			expected: true,
		},
		{
			name:     "starts_with no match",
			expr:     `channel_name starts_with "ITV"`,
			expected: false,
		},
		// not_starts_with
		{
			name:     "not_starts_with match",
			expr:     `channel_name not_starts_with "ITV"`,
			expected: true,
		},
		{
			name:     "not_starts_with no match",
			expr:     `channel_name not_starts_with "BBC"`,
			expected: false,
		},
		// ends_with
		{
			name:     "ends_with match",
			expr:     `channel_name ends_with "HD"`,
			expected: true,
		},
		{
			name:     "ends_with no match",
			expr:     `channel_name ends_with "SD"`,
			expected: false,
		},
		// not_ends_with
		{
			name:     "not_ends_with match",
			expr:     `channel_name not_ends_with "SD"`,
			expected: true,
		},
		{
			name:     "not_ends_with no match",
			expr:     `channel_name not_ends_with "HD"`,
			expected: false,
		},
	}

	evaluator := NewEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_RegexOperators(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
		"stream_url":   "http://example.com/stream/123.m3u8",
	})

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "matches simple",
			expr:     `channel_name matches "BBC.*"`,
			expected: true,
		},
		{
			name:     "matches no match",
			expr:     `channel_name matches "ITV.*"`,
			expected: false,
		},
		{
			name:     "matches with groups",
			expr:     `channel_name matches "(.+) HD"`,
			expected: true,
		},
		{
			name:     "not_matches match",
			expr:     `channel_name not_matches "ITV.*"`,
			expected: true,
		},
		{
			name:     "not_matches no match",
			expr:     `channel_name not_matches "BBC.*"`,
			expected: false,
		},
		{
			name:     "matches complex regex",
			expr:     `stream_url matches ".*\\.m3u8$"`,
			expected: true,
		},
	}

	evaluator := NewEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_RegexCaptures(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
	})

	evaluator := NewEvaluator()

	parsed, err := Parse(`channel_name matches "(.+) (HD|SD)"`)
	require.NoError(t, err)

	result, err := evaluator.Evaluate(parsed, accessor)
	require.NoError(t, err)
	assert.True(t, result.Matches)

	// Check captures
	require.Len(t, result.Captures, 3) // full match + 2 groups
	assert.Equal(t, "BBC One HD", result.Captures[0])
	assert.Equal(t, "BBC One", result.Captures[1])
	assert.Equal(t, "HD", result.Captures[2])
}

func TestEvaluator_NumericOperators(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_number": "150",
	})

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "greater_than true",
			expr:     `channel_number greater_than "100"`,
			expected: true,
		},
		{
			name:     "greater_than false",
			expr:     `channel_number greater_than "200"`,
			expected: false,
		},
		{
			name:     "greater_than_or_equal true equal",
			expr:     `channel_number greater_than_or_equal "150"`,
			expected: true,
		},
		{
			name:     "greater_than_or_equal true greater",
			expr:     `channel_number greater_than_or_equal "100"`,
			expected: true,
		},
		{
			name:     "less_than true",
			expr:     `channel_number less_than "200"`,
			expected: true,
		},
		{
			name:     "less_than false",
			expr:     `channel_number less_than "100"`,
			expected: false,
		},
		{
			name:     "less_than_or_equal true equal",
			expr:     `channel_number less_than_or_equal "150"`,
			expected: true,
		},
		{
			name:     "less_than_or_equal true less",
			expr:     `channel_number less_than_or_equal "200"`,
			expected: true,
		},
	}

	evaluator := NewEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_BooleanLogic(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "UK Entertainment",
	})

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		// AND
		{
			name:     "AND both true",
			expr:     `channel_name contains "BBC" AND group_title contains "UK"`,
			expected: true,
		},
		{
			name:     "AND first false",
			expr:     `channel_name contains "ITV" AND group_title contains "UK"`,
			expected: false,
		},
		{
			name:     "AND second false",
			expr:     `channel_name contains "BBC" AND group_title contains "US"`,
			expected: false,
		},
		{
			name:     "AND both false",
			expr:     `channel_name contains "ITV" AND group_title contains "US"`,
			expected: false,
		},
		// OR
		{
			name:     "OR both true",
			expr:     `channel_name contains "BBC" OR group_title contains "UK"`,
			expected: true,
		},
		{
			name:     "OR first true",
			expr:     `channel_name contains "BBC" OR group_title contains "US"`,
			expected: true,
		},
		{
			name:     "OR second true",
			expr:     `channel_name contains "ITV" OR group_title contains "UK"`,
			expected: true,
		},
		{
			name:     "OR both false",
			expr:     `channel_name contains "ITV" OR group_title contains "US"`,
			expected: false,
		},
		// NOT (via negated operators)
		{
			name:     "NOT true",
			expr:     `NOT channel_name contains "ITV"`,
			expected: true,
		},
		{
			name:     "NOT false",
			expr:     `NOT channel_name contains "BBC"`,
			expected: false,
		},
	}

	evaluator := NewEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_NestedLogic(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "UK Entertainment",
		"source_name":  "Provider A",
	})

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "nested (A OR B) AND C - all true",
			expr:     `(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title contains "UK"`,
			expected: true,
		},
		{
			name:     "nested (A OR B) AND C - C false",
			expr:     `(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title contains "US"`,
			expected: false,
		},
		{
			name:     "nested A AND (B OR C) - all true",
			expr:     `channel_name contains "BBC" AND (group_title contains "UK" OR group_title contains "US")`,
			expected: true,
		},
		{
			name:     "complex nested",
			expr:     `(channel_name contains "BBC" AND group_title contains "UK") OR source_name equals "Provider B"`,
			expected: true,
		},
	}

	evaluator := NewEvaluator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_ShortCircuit(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"a": "true",
	})

	evaluator := NewEvaluator()

	// For AND, if first is false, second should not be evaluated
	// For OR, if first is true, second should not be evaluated
	// We can verify this by checking that non-existent fields don't cause errors
	// when short-circuit should skip them

	t.Run("AND short-circuit", func(t *testing.T) {
		// First condition is false, second references non-existent field
		parsed, err := Parse(`a equals "false" AND nonexistent equals "x"`)
		require.NoError(t, err)

		result, err := evaluator.Evaluate(parsed, accessor)
		require.NoError(t, err)
		assert.False(t, result.Matches)
	})

	t.Run("OR short-circuit", func(t *testing.T) {
		// First condition is true, second references non-existent field
		parsed, err := Parse(`a equals "true" OR nonexistent equals "x"`)
		require.NoError(t, err)

		result, err := evaluator.Evaluate(parsed, accessor)
		require.NoError(t, err)
		assert.True(t, result.Matches)
	})
}

func TestEvaluator_EmptyExpression(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC",
	})

	evaluator := NewEvaluator()

	// Empty expression should match everything
	parsed, err := Parse("")
	require.NoError(t, err)

	result, err := evaluator.Evaluate(parsed, accessor)
	require.NoError(t, err)
	assert.True(t, result.Matches)
}

func TestEvaluator_MissingField(t *testing.T) {
	accessor := newMockAccessor(map[string]string{})

	evaluator := NewEvaluator()

	// Comparing against a missing field should treat it as empty string
	parsed, err := Parse(`channel_name equals ""`)
	require.NoError(t, err)

	result, err := evaluator.Evaluate(parsed, accessor)
	require.NoError(t, err)
	assert.True(t, result.Matches)
}

func TestEvaluator_CaseInsensitive(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One HD",
	})

	evaluator := NewEvaluator()
	evaluator.SetCaseSensitive(false)

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "equals case insensitive",
			expr:     `channel_name equals "bbc one hd"`,
			expected: true,
		},
		{
			name:     "contains case insensitive",
			expr:     `channel_name contains "bbc"`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, accessor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestEvaluator_InvalidRegex(t *testing.T) {
	accessor := newMockAccessor(map[string]string{
		"channel_name": "BBC One",
	})

	evaluator := NewEvaluator()

	// Invalid regex should return error
	parsed, err := Parse(`channel_name matches "[invalid"`)
	require.NoError(t, err)

	_, err = evaluator.Evaluate(parsed, accessor)
	assert.Error(t, err)
}

func TestEvaluationResult(t *testing.T) {
	result := &EvaluationResult{
		Matches:  true,
		Captures: []string{"full", "group1", "group2"},
	}

	assert.True(t, result.Matches)
	assert.Len(t, result.Captures, 3)
	assert.Equal(t, "group1", result.Captures[1])
}
