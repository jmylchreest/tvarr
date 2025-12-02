package expression

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer tokenizes an expression string.
type Lexer struct {
	input  string
	pos    int // current position in input
	start  int // start position of current token
	width  int // width of last rune read
	line   int
	column int
	tokens []Token
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		line:   1,
		column: 1,
	}
}

// Tokenize lexes the entire input and returns all tokens.
func (l *Lexer) Tokenize() ([]Token, error) {
	for {
		tok := l.nextToken()
		l.tokens = append(l.tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
		if tok.Type == TokenError {
			return l.tokens, &LexError{
				Message: tok.Value,
				Pos:     tok.Pos,
				Line:    tok.Line,
				Column:  tok.Column,
			}
		}
	}
	return l.tokens, nil
}

// LexError represents a lexer error.
type LexError struct {
	Message string
	Pos     int
	Line    int
	Column  int
}

func (e *LexError) Error() string {
	return e.Message
}

// nextToken returns the next token from the input.
func (l *Lexer) nextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return l.makeToken(TokenEOF, "")
	}

	l.start = l.pos
	ch := l.next()

	switch {
	case ch == '(':
		return l.makeToken(TokenLParen, "(")
	case ch == ')':
		return l.makeToken(TokenRParen, ")")
	case ch == ',':
		return l.makeToken(TokenComma, ",")
	case ch == '=':
		if l.peek() == '=' {
			l.next()
			return l.makeToken(TokenEquals, "==")
		}
		return l.makeToken(TokenEquals, "=")
	case ch == '?':
		if l.peek() == '=' {
			l.next()
			return l.makeToken(TokenSetIfEmpty, "?=")
		}
		return l.makeErrorToken("unexpected character '?'")
	case ch == '+':
		if l.peek() == '=' {
			l.next()
			return l.makeToken(TokenAppend, "+=")
		}
		return l.makeErrorToken("unexpected character '+'")
	case ch == '!':
		if l.peek() == '=' {
			l.next()
			return l.makeToken(TokenNotEquals, "!=")
		}
		return l.makeToken(TokenNot, "!")
	case ch == '&':
		if l.peek() == '&' {
			l.next()
			return l.makeToken(TokenAnd, "&&")
		}
		return l.makeErrorToken("unexpected character '&'")
	case ch == '|':
		if l.peek() == '|' {
			l.next()
			return l.makeToken(TokenOr, "||")
		}
		return l.makeErrorToken("unexpected character '|'")
	case ch == '-':
		if l.peek() == '=' {
			l.next()
			return l.makeToken(TokenRemove, "-=")
		}
		// Check for negative number
		if isDigit(l.peek()) {
			l.backup()
			return l.scanNumber()
		}
		return l.makeErrorToken("unexpected character '-'")
	case ch == '"' || ch == '\'':
		return l.scanString(ch)
	case isDigit(ch):
		l.backup()
		return l.scanNumber()
	case isIdentStart(ch):
		l.backup()
		return l.scanIdent()
	default:
		return l.makeErrorToken("unexpected character '" + string(ch) + "'")
	}
}

// scanString scans a quoted string.
func (l *Lexer) scanString(quote rune) Token {
	var sb strings.Builder

	for {
		ch := l.next()
		if ch == 0 {
			return l.makeErrorToken("unterminated string")
		}
		if ch == quote {
			break
		}
		if ch == '\\' {
			// Handle escape sequences
			escaped := l.next()
			switch escaped {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			default:
				sb.WriteRune(escaped)
			}
		} else {
			sb.WriteRune(ch)
		}
	}

	return l.makeToken(TokenString, sb.String())
}

// scanNumber scans a numeric literal.
func (l *Lexer) scanNumber() Token {
	// Handle negative sign
	if l.peek() == '-' {
		l.next()
	}

	// Scan integer part
	for isDigit(l.peek()) {
		l.next()
	}

	// Check for decimal part
	if l.peek() == '.' {
		l.next()
		for isDigit(l.peek()) {
			l.next()
		}
	}

	return l.makeToken(TokenNumber, l.input[l.start:l.pos])
}

// scanIdent scans an identifier or keyword.
func (l *Lexer) scanIdent() Token {
	for isIdentPart(l.peek()) {
		l.next()
	}

	value := l.input[l.start:l.pos]
	tokType := LookupKeyword(value)

	return l.makeToken(tokType, value)
}

// next returns the next rune and advances the position.
func (l *Lexer) next() rune {
	if l.pos >= len(l.input) {
		l.width = 0
		return 0
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = w
	l.pos += w

	if r == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}

	return r
}

// backup steps back one rune.
func (l *Lexer) backup() {
	l.pos -= l.width
	if l.width > 0 && l.input[l.pos] == '\n' {
		l.line--
	}
	l.column--
}

// peek returns the next rune without advancing.
func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.pos:])
	return r
}

// skipWhitespace skips whitespace characters.
func (l *Lexer) skipWhitespace() {
	for {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.next()
		} else {
			break
		}
	}
}

// makeToken creates a token with the current position info.
func (l *Lexer) makeToken(typ TokenType, value string) Token {
	return Token{
		Type:   typ,
		Value:  value,
		Pos:    l.start,
		Line:   l.line,
		Column: l.column - len(value),
	}
}

// makeErrorToken creates an error token.
func (l *Lexer) makeErrorToken(msg string) Token {
	return Token{
		Type:   TokenError,
		Value:  msg,
		Pos:    l.start,
		Line:   l.line,
		Column: l.column,
	}
}

// isDigit returns true if the rune is a digit.
func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// isIdentStart returns true if the rune can start an identifier.
func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == '@' || ch == '$'
}

// isIdentPart returns true if the rune can be part of an identifier.
func isIdentPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == ':' || ch == '-' || ch == '$' || ch == '@'
}
