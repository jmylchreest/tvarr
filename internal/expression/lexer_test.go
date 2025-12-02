package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLexer_SimpleTokens(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []TokenType
	}{
		{
			name:     "empty",
			input:    "",
			expected: []TokenType{TokenEOF},
		},
		{
			name:     "parentheses",
			input:    "()",
			expected: []TokenType{TokenLParen, TokenRParen, TokenEOF},
		},
		{
			name:     "equals",
			input:    "= ==",
			expected: []TokenType{TokenEquals, TokenEquals, TokenEOF},
		},
		{
			name:     "not equals",
			input:    "!=",
			expected: []TokenType{TokenNotEquals, TokenEOF},
		},
		{
			name:     "logical operators",
			input:    "AND OR && ||",
			expected: []TokenType{TokenAnd, TokenOr, TokenAnd, TokenOr, TokenEOF},
		},
		{
			name:     "not",
			input:    "NOT !",
			expected: []TokenType{TokenNot, TokenNot, TokenEOF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			require.NoError(t, err)

			types := make([]TokenType, len(tokens))
			for i, tok := range tokens {
				types[i] = tok.Type
			}
			assert.Equal(t, tt.expected, types)
		})
	}
}

func TestLexer_Identifiers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple identifier",
			input:    "channel_name",
			expected: []string{"channel_name"},
		},
		{
			name:     "multiple identifiers",
			input:    "field1 field2 field_three",
			expected: []string{"field1", "field2", "field_three"},
		},
		{
			name:     "helper syntax",
			input:    "@time:now @logo:uuid",
			expected: []string{"@time:now", "@logo:uuid"},
		},
		{
			name:     "operators as identifiers",
			input:    "contains equals starts_with",
			expected: []string{"contains", "equals", "starts_with"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			require.NoError(t, err)

			var values []string
			for _, tok := range tokens {
				if tok.Type == TokenIdent {
					values = append(values, tok.Value)
				}
			}
			assert.Equal(t, tt.expected, values)
		})
	}
}

func TestLexer_Strings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "double quoted",
			input:    `"hello world"`,
			expected: "hello world",
		},
		{
			name:     "single quoted",
			input:    `'hello world'`,
			expected: "hello world",
		},
		{
			name:     "escaped quotes",
			input:    `"hello \"world\""`,
			expected: `hello "world"`,
		},
		{
			name:     "escape sequences",
			input:    `"line1\nline2\ttab"`,
			expected: "line1\nline2\ttab",
		},
		{
			name:     "empty string",
			input:    `""`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			require.NoError(t, err)
			require.Len(t, tokens, 2) // string + EOF

			assert.Equal(t, TokenString, tokens[0].Type)
			assert.Equal(t, tt.expected, tokens[0].Value)
		})
	}
}

func TestLexer_Numbers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "integer",
			input:    "42",
			expected: "42",
		},
		{
			name:     "negative integer",
			input:    "-42",
			expected: "-42",
		},
		{
			name:     "decimal",
			input:    "3.14",
			expected: "3.14",
		},
		{
			name:     "negative decimal",
			input:    "-3.14",
			expected: "-3.14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			require.NoError(t, err)
			require.Len(t, tokens, 2) // number + EOF

			assert.Equal(t, TokenNumber, tokens[0].Type)
			assert.Equal(t, tt.expected, tokens[0].Value)
		})
	}
}

func TestLexer_CompleteExpression(t *testing.T) {
	input := `channel_name contains "Sport" AND group_title equals "Live TV"`

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	require.NoError(t, err)

	expected := []struct {
		typ   TokenType
		value string
	}{
		{TokenIdent, "channel_name"},
		{TokenIdent, "contains"},
		{TokenString, "Sport"},
		{TokenAnd, "AND"},
		{TokenIdent, "group_title"},
		{TokenIdent, "equals"},
		{TokenString, "Live TV"},
		{TokenEOF, ""},
	}

	require.Len(t, tokens, len(expected))
	for i, exp := range expected {
		assert.Equal(t, exp.typ, tokens[i].Type, "token %d type", i)
		assert.Equal(t, exp.value, tokens[i].Value, "token %d value", i)
	}
}

func TestLexer_ExpressionWithActions(t *testing.T) {
	input := `channel_name contains "BBC" SET group_title = "UK Channels"`

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	require.NoError(t, err)

	expected := []struct {
		typ   TokenType
		value string
	}{
		{TokenIdent, "channel_name"},
		{TokenIdent, "contains"},
		{TokenString, "BBC"},
		{TokenSet, "SET"},
		{TokenIdent, "group_title"},
		{TokenEquals, "="},
		{TokenString, "UK Channels"},
		{TokenEOF, ""},
	}

	require.Len(t, tokens, len(expected))
	for i, exp := range expected {
		assert.Equal(t, exp.typ, tokens[i].Type, "token %d type", i)
		assert.Equal(t, exp.value, tokens[i].Value, "token %d value", i)
	}
}

func TestLexer_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unterminated string",
			input: `"hello`,
		},
		{
			name:  "unexpected character",
			input: `field # value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			_, err := lexer.Tokenize()
			assert.Error(t, err)
		})
	}
}

func TestLexer_Whitespace(t *testing.T) {
	// Whitespace should be ignored
	input := "  field1   \n\t  field2  \r\n  "

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	require.NoError(t, err)

	assert.Len(t, tokens, 3) // field1, field2, EOF
	assert.Equal(t, "field1", tokens[0].Value)
	assert.Equal(t, "field2", tokens[1].Value)
}
