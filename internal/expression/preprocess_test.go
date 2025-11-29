package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSymbolicOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "equals symbol",
			input:    `field == "value"`,
			expected: `field  equals  "value"`,
		},
		{
			name:     "not equals symbol",
			input:    `field != "value"`,
			expected: `field  not equals  "value"`,
		},
		{
			name:     "matches symbol",
			input:    `field =~ "pattern"`,
			expected: `field  matches  "pattern"`,
		},
		{
			name:     "not matches symbol",
			input:    `field !~ "pattern"`,
			expected: `field  not matches  "pattern"`,
		},
		{
			name:     "greater than",
			input:    `field > 100`,
			expected: `field  greater_than  100`,
		},
		{
			name:     "less than",
			input:    `field < 100`,
			expected: `field  less_than  100`,
		},
		{
			name:     "greater than or equal",
			input:    `field >= 100`,
			expected: `field  greater_than_or_equal  100`,
		},
		{
			name:     "less than or equal",
			input:    `field <= 100`,
			expected: `field  less_than_or_equal  100`,
		},
		{
			name:     "logical AND symbol",
			input:    `a == "1" && b == "2"`,
			expected: `a  equals  "1"  AND  b  equals  "2"`,
		},
		{
			name:     "logical OR symbol",
			input:    `a == "1" || b == "2"`,
			expected: `a  equals  "1"  OR  b  equals  "2"`,
		},
		{
			name:     "lowercase and",
			input:    `a equals "1" and b equals "2"`,
			expected: `a equals "1" AND b equals "2"`,
		},
		{
			name:     "lowercase or",
			input:    `a equals "1" or b equals "2"`,
			expected: `a equals "1" OR b equals "2"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSymbolicOperators(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanonicalizeLegacyFusedNegations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "not_equals",
			input:    `field not_equals "value"`,
			expected: `field not equals "value"`,
		},
		{
			name:     "not_matches",
			input:    `field not_matches "pattern"`,
			expected: `field not matches "pattern"`,
		},
		{
			name:     "not_contains",
			input:    `field not_contains "text"`,
			expected: `field not contains "text"`,
		},
		{
			name:     "not_starts_with",
			input:    `field not_starts_with "prefix"`,
			expected: `field not starts_with "prefix"`,
		},
		{
			name:     "not_ends_with",
			input:    `field not_ends_with "suffix"`,
			expected: `field not ends_with "suffix"`,
		},
		{
			name:     "no change needed",
			input:    `field contains "value"`,
			expected: `field contains "value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := canonicalizeLegacyFusedNegations(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRelocatePreFieldModifiers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "not before field",
			input:    `not field contains "value"`,
			expected: `field not contains "value"`,
		},
		{
			name:     "case_sensitive before field",
			input:    `case_sensitive field equals "Value"`,
			expected: `field case_sensitive equals "Value"`,
		},
		{
			name:     "no modifier",
			input:    `field contains "value"`,
			expected: `field contains "value"`,
		},
		{
			name:     "preserve leading whitespace",
			input:    `  not field contains "value"`,
			expected: `  field not contains "value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := relocatePreFieldModifiers(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollapseWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    `field   contains    "value"`,
			expected: `field contains "value"`,
		},
		{
			name:     "tabs and spaces",
			input:    "field\t\t contains \t \"value\"",
			expected: `field contains "value"`,
		},
		{
			name:     "leading and trailing",
			input:    `   field contains "value"   `,
			expected: `field contains "value"`,
		},
		{
			name:     "no change needed",
			input:    `field contains "value"`,
			expected: `field contains "value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapseWhitespace(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPreprocess(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "symbolic operators with whitespace",
			input:    `field == "value"  &&  other != "x"`,
			expected: `field equals "value" AND other not equals "x"`,
		},
		{
			name:     "fused negations",
			input:    `field not_contains "test"`,
			expected: `field not contains "test"`,
		},
		{
			name:     "complex expression",
			input:    `field == "a" && other !~ "b" || third >= 10`,
			expected: `field equals "a" AND other not matches "b" OR third greater_than_or_equal 10`,
		},
		{
			name:     "pre-field modifier relocation",
			input:    `not field contains "value"`,
			expected: `field not contains "value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Preprocess(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPreprocessAndParse(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		result, err := PreprocessAndParse("")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("whitespace only returns nil", func(t *testing.T) {
		result, err := PreprocessAndParse("   ")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("symbolic equals parses correctly", func(t *testing.T) {
		result, err := PreprocessAndParse(`channel_name == "BBC"`)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("symbolic not equals parses correctly", func(t *testing.T) {
		result, err := PreprocessAndParse(`channel_name != "BBC"`)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("symbolic and/or parses correctly", func(t *testing.T) {
		result, err := PreprocessAndParse(`channel_name == "BBC" && group_title != ""`)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}
