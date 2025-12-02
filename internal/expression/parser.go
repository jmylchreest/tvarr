package expression

import (
	"fmt"
	"strconv"
	"strings"
)

// Parser parses expression tokens into an AST.
type Parser struct {
	tokens  []Token
	pos     int
	current Token
}

// NewParser creates a new parser for the given tokens.
func NewParser(tokens []Token) *Parser {
	p := &Parser{
		tokens: tokens,
		pos:    0,
	}
	if len(tokens) > 0 {
		p.current = tokens[0]
	}
	return p
}

// Parse parses the tokens into a ParsedExpression.
func (p *Parser) Parse() (*ParsedExpression, error) {
	if len(p.tokens) == 0 || (len(p.tokens) == 1 && p.tokens[0].Type == TokenEOF) {
		// Empty expression - matches everything
		return &ParsedExpression{
			Expression: &ConditionOnly{Condition: nil},
		}, nil
	}

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Ensure we've consumed all tokens
	if p.current.Type != TokenEOF {
		return nil, p.errorf("unexpected token: %s", p.current.Value)
	}

	// Build metadata
	parsed := &ParsedExpression{
		Expression: expr,
	}
	p.extractMetadata(parsed)

	return parsed, nil
}

// parseExpression parses a full expression (conditions with optional actions).
func (p *Parser) parseExpression() (ExtendedExpression, error) {
	// Parse the condition tree first
	condition, err := p.parseOrCondition()
	if err != nil {
		return nil, err
	}

	// Check if there are actions (SET keyword or shorthand syntax)
	if p.hasActions() {
		actions, err := p.parseActions()
		if err != nil {
			return nil, err
		}
		return &ConditionWithActions{
			Condition: NewConditionTree(condition),
			Actions:   actions,
		}, nil
	}

	return &ConditionOnly{
		Condition: NewConditionTree(condition),
	}, nil
}

// hasActions checks if the current position starts action syntax.
// This includes:
// - SET/APPEND/etc keywords
// - Shorthand operators: field = value, field ?= value, field += value, field -= value
func (p *Parser) hasActions() bool {
	// Explicit action keyword
	if p.current.Type == TokenSet {
		return true
	}

	// Shorthand syntax: identifier followed by assignment operator
	if p.current.Type == TokenIdent {
		// Look ahead for shorthand operators
		if p.pos+1 < len(p.tokens) {
			next := p.tokens[p.pos+1]
			switch next.Type {
			case TokenEquals, TokenSetIfEmpty, TokenAppend, TokenRemove:
				return true
			}
		}
	}

	return false
}

// parseOrCondition parses OR-connected conditions.
func (p *Parser) parseOrCondition() (ConditionNode, error) {
	left, err := p.parseAndCondition()
	if err != nil {
		return nil, err
	}

	for p.current.Type == TokenOr {
		p.advance() // consume OR

		right, err := p.parseAndCondition()
		if err != nil {
			return nil, err
		}

		// Flatten consecutive OR operations
		if group, ok := left.(*ConditionGroup); ok && group.Operator == LogicalOr {
			group.Children = append(group.Children, right)
		} else {
			left = Or(left, right)
		}
	}

	return left, nil
}

// parseAndCondition parses AND-connected conditions.
func (p *Parser) parseAndCondition() (ConditionNode, error) {
	left, err := p.parseUnaryCondition()
	if err != nil {
		return nil, err
	}

	for p.current.Type == TokenAnd {
		p.advance() // consume AND

		right, err := p.parseUnaryCondition()
		if err != nil {
			return nil, err
		}

		// Flatten consecutive AND operations
		if group, ok := left.(*ConditionGroup); ok && group.Operator == LogicalAnd {
			group.Children = append(group.Children, right)
		} else {
			left = And(left, right)
		}
	}

	return left, nil
}

// parseUnaryCondition parses a possibly negated condition.
func (p *Parser) parseUnaryCondition() (ConditionNode, error) {
	// Handle NOT prefix
	if p.current.Type == TokenNot {
		p.advance() // consume NOT
		cond, err := p.parsePrimaryCondition()
		if err != nil {
			return nil, err
		}

		// Negate the condition
		if c, ok := cond.(*Condition); ok {
			c.Operator = negateOperator(c.Operator)
			return c, nil
		}

		return nil, p.errorf("NOT can only be applied to simple conditions")
	}

	return p.parsePrimaryCondition()
}

// parsePrimaryCondition parses a primary condition (grouped or simple).
func (p *Parser) parsePrimaryCondition() (ConditionNode, error) {
	// Handle parenthesized group
	if p.current.Type == TokenLParen {
		p.advance() // consume (

		cond, err := p.parseOrCondition()
		if err != nil {
			return nil, err
		}

		if p.current.Type != TokenRParen {
			return nil, p.errorf("expected ')' but got %s", p.current.Value)
		}
		p.advance() // consume )

		return cond, nil
	}

	// Parse simple condition: field operator value
	return p.parseSimpleCondition()
}

// parseSimpleCondition parses a simple field-operator-value condition.
// Supports both "field operator value" and "field not operator value" syntax.
func (p *Parser) parseSimpleCondition() (*Condition, error) {
	// Expect field name
	if p.current.Type != TokenIdent {
		return nil, p.errorf("expected field name but got %s", p.current.Value)
	}
	field := p.current.Value
	p.advance()

	// Check for mid-field "not" modifier (e.g., "field not contains value")
	negated := false
	if p.current.Type == TokenNot {
		negated = true
		p.advance()
	}

	// Expect operator (as identifier)
	if p.current.Type != TokenIdent {
		return nil, p.errorf("expected operator but got %s", p.current.Value)
	}
	opStr := p.current.Value
	op, ok := ParseFilterOperator(opStr)
	if !ok {
		return nil, p.errorf("unknown operator: %s", opStr)
	}
	// Apply negation if "not" modifier was present
	if negated {
		op = negateOperator(op)
	}
	p.advance()

	// Expect value (string or number)
	var value string
	switch p.current.Type {
	case TokenString:
		value = p.current.Value
	case TokenNumber:
		value = p.current.Value
	case TokenIdent:
		// Allow unquoted identifiers as values for backward compatibility
		value = p.current.Value
	default:
		return nil, p.errorf("expected value but got %s", p.current.Value)
	}
	p.advance()

	return NewCondition(field, op, value), nil
}

// parseActions parses one or more actions.
// Supports both keyword syntax (SET field = value) and shorthand syntax (field = value, field ?= value, etc).
func (p *Parser) parseActions() ([]*Action, error) {
	var actions []*Action

	for p.hasActions() {
		// Check for keyword-based action (SET, SET_IF_EMPTY, APPEND, etc)
		if p.current.Type == TokenSet {
			opStr := p.current.Value
			actionOp, ok := ParseActionOperator(opStr)
			if !ok {
				return nil, p.errorf("unknown action operator: %s", opStr)
			}
			p.advance()

			// Parse one or more field = value assignments for this action operator
			for {
				action, err := p.parseActionAssignment(actionOp)
				if err != nil {
					return nil, err
				}
				actions = append(actions, action)

				// Check for comma (multiple assignments in same SET)
				if p.current.Type == TokenComma {
					p.advance()
					// Continue with next field = value in same action operator
					continue
				}

				// No comma, break out of inner loop
				break
			}
		} else if p.current.Type == TokenIdent {
			// Shorthand syntax: field op= value
			action, err := p.parseShorthandAction()
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
		} else {
			break
		}
	}

	return actions, nil
}

// parseActionAssignment parses "field = value" or "field" (for DELETE).
func (p *Parser) parseActionAssignment(actionOp ActionOperator) (*Action, error) {
	// Parse field name
	if p.current.Type != TokenIdent {
		return nil, p.errorf("expected field name")
	}
	field := p.current.Value
	p.advance()

	// For DELETE, no value is needed
	if actionOp == ActionDelete {
		return NewAction(field, actionOp, nil), nil
	}

	// Expect = for assignment
	if p.current.Type != TokenEquals {
		return nil, p.errorf("expected '=' after field name")
	}
	p.advance()

	// Parse value
	value, err := p.parseActionValue()
	if err != nil {
		return nil, err
	}

	return NewAction(field, actionOp, value), nil
}

// parseShorthandAction parses shorthand action syntax: field = value, field ?= value, etc.
func (p *Parser) parseShorthandAction() (*Action, error) {
	// Parse field name
	if p.current.Type != TokenIdent {
		return nil, p.errorf("expected field name")
	}
	field := p.current.Value
	p.advance()

	// Determine action operator from shorthand token
	var actionOp ActionOperator
	switch p.current.Type {
	case TokenEquals:
		actionOp = ActionSet
	case TokenSetIfEmpty:
		actionOp = ActionSetIfEmpty
	case TokenAppend:
		actionOp = ActionAppend
	case TokenRemove:
		actionOp = ActionRemove
	default:
		return nil, p.errorf("expected assignment operator")
	}
	p.advance()

	// Parse value
	value, err := p.parseActionValue()
	if err != nil {
		return nil, err
	}

	return NewAction(field, actionOp, value), nil
}

// parseActionValue parses the value part of an action.
func (p *Parser) parseActionValue() (ActionValue, error) {
	switch p.current.Type {
	case TokenString:
		value := p.current.Value
		p.advance()
		// Check for capture references in the string
		if containsCaptureRef(value) {
			return &LiteralValue{Value: value}, nil
		}
		return NewLiteralValue(value), nil

	case TokenNumber:
		value := p.current.Value
		p.advance()
		return NewLiteralValue(value), nil

	case TokenIdent:
		value := p.current.Value
		p.advance()

		// Check if it's a field reference (starts with $)
		if strings.HasPrefix(value, "$") {
			// Check if it's a capture reference ($1, $2, etc.)
			if len(value) > 1 && isDigit(rune(value[1])) {
				idx, err := strconv.Atoi(value[1:])
				if err == nil {
					return &CaptureReference{Index: idx}, nil
				}
			}
			// Otherwise it's a field reference
			return &FieldReference{Field: value[1:]}, nil
		}

		return NewLiteralValue(value), nil

	default:
		return nil, p.errorf("expected value but got %s", p.current.Value)
	}
}

// advance moves to the next token.
func (p *Parser) advance() {
	p.pos++
	if p.pos < len(p.tokens) {
		p.current = p.tokens[p.pos]
	} else {
		p.current = Token{Type: TokenEOF}
	}
}

// errorf creates a parse error.
func (p *Parser) errorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return &ParseError{
		Message: msg,
		Pos:     p.current.Pos,
		Line:    p.current.Line,
		Column:  p.current.Column,
	}
}

// extractMetadata populates the ParsedExpression metadata.
func (p *Parser) extractMetadata(parsed *ParsedExpression) {
	switch expr := parsed.Expression.(type) {
	case *ConditionOnly:
		if expr.Condition != nil && expr.Condition.Root != nil {
			parsed.ReferencedFields = extractConditionFields(expr.Condition.Root)
			parsed.UsesRegex = conditionUsesRegex(expr.Condition.Root)
		}
		parsed.HasActions = false

	case *ConditionWithActions:
		if expr.Condition != nil && expr.Condition.Root != nil {
			parsed.ReferencedFields = extractConditionFields(expr.Condition.Root)
			parsed.UsesRegex = conditionUsesRegex(expr.Condition.Root)
		}
		parsed.HasActions = len(expr.Actions) > 0
		for _, action := range expr.Actions {
			parsed.ModifiedFields = append(parsed.ModifiedFields, action.Field)
		}
	}
}

// ParseError represents a parsing error.
type ParseError struct {
	Message string
	Pos     int
	Line    int
	Column  int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// negateOperator returns the negated form of an operator.
func negateOperator(op FilterOperator) FilterOperator {
	switch op {
	case OpEquals:
		return OpNotEquals
	case OpNotEquals:
		return OpEquals
	case OpContains:
		return OpNotContains
	case OpNotContains:
		return OpContains
	case OpStartsWith:
		return OpNotStartsWith
	case OpNotStartsWith:
		return OpStartsWith
	case OpEndsWith:
		return OpNotEndsWith
	case OpNotEndsWith:
		return OpEndsWith
	case OpMatches:
		return OpNotMatches
	case OpNotMatches:
		return OpMatches
	default:
		return op
	}
}

// containsCaptureRef checks if a string contains capture references like $1, $2.
func containsCaptureRef(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '$' && isDigit(rune(s[i+1])) {
			return true
		}
	}
	return false
}

// extractConditionFields extracts all field names from a condition tree.
func extractConditionFields(node ConditionNode) []string {
	var fields []string
	seen := make(map[string]bool)

	var extract func(n ConditionNode)
	extract = func(n ConditionNode) {
		switch c := n.(type) {
		case *Condition:
			if !seen[c.Field] {
				fields = append(fields, c.Field)
				seen[c.Field] = true
			}
		case *ConditionGroup:
			for _, child := range c.Children {
				extract(child)
			}
		}
	}

	extract(node)
	return fields
}

// conditionUsesRegex checks if any condition uses regex operators.
func conditionUsesRegex(node ConditionNode) bool {
	switch c := node.(type) {
	case *Condition:
		return c.Operator.IsRegex()
	case *ConditionGroup:
		for _, child := range c.Children {
			if conditionUsesRegex(child) {
				return true
			}
		}
	}
	return false
}

// Parse is a convenience function to parse an expression string.
func Parse(input string) (*ParsedExpression, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, fmt.Errorf("lexer error: %w", err)
	}

	parser := NewParser(tokens)
	parsed, err := parser.Parse()
	if err != nil {
		return nil, err
	}

	parsed.Original = input
	return parsed, nil
}

// MustParse parses an expression and panics on error.
// Useful for tests and static expressions.
func MustParse(input string) *ParsedExpression {
	parsed, err := Parse(input)
	if err != nil {
		panic(fmt.Sprintf("expression parse error: %v", err))
	}
	return parsed
}
