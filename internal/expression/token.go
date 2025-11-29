package expression

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

// Token types.
const (
	TokenEOF TokenType = iota
	TokenError

	// Literals
	TokenIdent  // field names, keywords
	TokenString // "quoted string" or 'quoted string'
	TokenNumber // integer or float

	// Operators
	TokenEquals    // =
	TokenNotEquals // !=

	// Logical
	TokenAnd // AND, &&
	TokenOr  // OR, ||
	TokenNot // NOT, !

	// Grouping
	TokenLParen // (
	TokenRParen // )

	// Action
	TokenSet // SET keyword

	// Punctuation
	TokenComma // ,
)

// Token represents a lexical token.
type Token struct {
	Type   TokenType
	Value  string
	Pos    int // Position in input
	Line   int
	Column int
}

// String returns a string representation of the token.
func (t Token) String() string {
	switch t.Type {
	case TokenEOF:
		return "EOF"
	case TokenError:
		return fmt.Sprintf("ERROR(%s)", t.Value)
	default:
		if len(t.Value) > 20 {
			return fmt.Sprintf("%s(%.20s...)", t.Type, t.Value)
		}
		return fmt.Sprintf("%s(%s)", t.Type, t.Value)
	}
}

// String returns the name of the token type.
func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenError:
		return "Error"
	case TokenIdent:
		return "Ident"
	case TokenString:
		return "String"
	case TokenNumber:
		return "Number"
	case TokenEquals:
		return "Equals"
	case TokenNotEquals:
		return "NotEquals"
	case TokenAnd:
		return "And"
	case TokenOr:
		return "Or"
	case TokenNot:
		return "Not"
	case TokenLParen:
		return "LParen"
	case TokenRParen:
		return "RParen"
	case TokenSet:
		return "Set"
	case TokenComma:
		return "Comma"
	default:
		return fmt.Sprintf("Token(%d)", t)
	}
}

// Keywords that have special meaning.
var keywords = map[string]TokenType{
	"AND":          TokenAnd,
	"and":          TokenAnd,
	"OR":           TokenOr,
	"or":           TokenOr,
	"NOT":          TokenNot,
	"not":          TokenNot,
	"SET":          TokenSet,
	"set":          TokenSet,
	"SET_IF_EMPTY": TokenSet,
	"set_if_empty": TokenSet,
	"APPEND":       TokenSet,
	"append":       TokenSet,
	"REMOVE":       TokenSet,
	"remove":       TokenSet,
	"DELETE":       TokenSet,
	"delete":       TokenSet,
}

// LookupKeyword returns the token type for an identifier,
// checking if it's a keyword first.
func LookupKeyword(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TokenIdent
}
