package expression

// Node is the interface implemented by all AST nodes.
type Node interface {
	node()
}

// ConditionNode represents a node in a condition tree.
// It can be either a single condition or a group of conditions.
type ConditionNode interface {
	Node
	conditionNode()
}

// Condition represents a single field comparison condition.
type Condition struct {
	Field         string         // Field name to compare
	Operator      FilterOperator // Comparison operator
	Value         string         // Value to compare against
	CaseSensitive bool           // If true, comparison is case-sensitive
}

func (c *Condition) node()          {}
func (c *Condition) conditionNode() {}

// ConditionGroup represents a group of conditions joined by a logical operator.
type ConditionGroup struct {
	Operator LogicalOperator // AND or OR
	Children []ConditionNode // Child conditions
}

func (g *ConditionGroup) node()          {}
func (g *ConditionGroup) conditionNode() {}

// ConditionTree wraps the root of a condition tree.
type ConditionTree struct {
	Root ConditionNode
}

func (t *ConditionTree) node() {}

// IsEmpty returns true if the condition tree has no conditions.
func (t *ConditionTree) IsEmpty() bool {
	return t == nil || t.Root == nil
}

// Action represents a field modification action.
type Action struct {
	Field    string         // Field to modify
	Operator ActionOperator // Action to perform
	Value    ActionValue    // Value for the action (may be nil for DELETE)
}

func (a *Action) node() {}

// ActionValue represents the value in an action.
type ActionValue interface {
	Node
	actionValue()
}

// LiteralValue represents a literal string value.
type LiteralValue struct {
	Value string
}

func (v *LiteralValue) node()        {}
func (v *LiteralValue) actionValue() {}

// NullValue represents a null/empty value.
type NullValue struct{}

func (v *NullValue) node()        {}
func (v *NullValue) actionValue() {}

// FieldReference represents a reference to another field's value.
type FieldReference struct {
	Field string
}

func (v *FieldReference) node()        {}
func (v *FieldReference) actionValue() {}

// CaptureReference represents a regex capture group reference ($1, $2, etc.).
type CaptureReference struct {
	Index int // 1-based index
}

func (v *CaptureReference) node()        {}
func (v *CaptureReference) actionValue() {}

// DynamicFieldReference represents a reference to a dynamically resolved field.
// Supports two syntaxes:
//   - Legacy: @prefix:parameter (e.g., @header_req:x-video-codec)
//   - Unified: @dynamic(path):key (e.g., @dynamic(request.headers):x-video-codec)
type DynamicFieldReference struct {
	// For legacy syntax (@prefix:parameter)
	Prefix    string // e.g., "header_req"
	Parameter string // e.g., "x-video-codec"

	// For unified syntax (@dynamic(path):key)
	Path string // e.g., "request.headers" (empty for legacy syntax)
	Key  string // e.g., "x-video-codec" (empty for legacy syntax)
}

// IsUnified returns true if this uses the unified @dynamic(path):key syntax.
func (v *DynamicFieldReference) IsUnified() bool {
	return v.Path != ""
}

// String returns the string representation of the dynamic reference.
func (v *DynamicFieldReference) String() string {
	if v.IsUnified() {
		return "@dynamic(" + v.Path + "):" + v.Key
	}
	return "@" + v.Prefix + ":" + v.Parameter
}

func (v *DynamicFieldReference) node()        {}
func (v *DynamicFieldReference) actionValue() {}

// ExtendedExpression represents a complete expression that may include
// both conditions and actions.
type ExtendedExpression interface {
	Node
	extendedExpression()
}

// ConditionOnly represents an expression with only conditions (for filtering).
type ConditionOnly struct {
	Condition *ConditionTree
}

func (e *ConditionOnly) node()               {}
func (e *ConditionOnly) extendedExpression() {}

// ConditionWithActions represents an expression with conditions and actions (for data mapping).
// Format: condition SET field = value, field2 = value2
type ConditionWithActions struct {
	Condition *ConditionTree
	Actions   []*Action
}

func (e *ConditionWithActions) node()               {}
func (e *ConditionWithActions) extendedExpression() {}

// ParsedExpression wraps a parsed expression with metadata.
type ParsedExpression struct {
	// Original is the original expression text.
	Original string

	// Expression is the parsed AST.
	Expression ExtendedExpression

	// HasActions indicates if the expression contains any actions.
	HasActions bool

	// UsesRegex indicates if any condition uses regex operators.
	UsesRegex bool

	// ReferencedFields lists all field names referenced in conditions.
	ReferencedFields []string

	// ModifiedFields lists all field names that may be modified by actions.
	ModifiedFields []string
}

// NewCondition creates a new condition.
func NewCondition(field string, op FilterOperator, value string) *Condition {
	return &Condition{
		Field:         field,
		Operator:      op,
		Value:         value,
		CaseSensitive: false,
	}
}

// NewConditionGroup creates a new condition group.
func NewConditionGroup(op LogicalOperator, children ...ConditionNode) *ConditionGroup {
	return &ConditionGroup{
		Operator: op,
		Children: children,
	}
}

// NewConditionTree creates a new condition tree.
func NewConditionTree(root ConditionNode) *ConditionTree {
	return &ConditionTree{Root: root}
}

// NewAction creates a new action.
func NewAction(field string, op ActionOperator, value ActionValue) *Action {
	return &Action{
		Field:    field,
		Operator: op,
		Value:    value,
	}
}

// NewLiteralValue creates a new literal value.
func NewLiteralValue(value string) *LiteralValue {
	return &LiteralValue{Value: value}
}

// And creates an AND group with the given conditions.
func And(conditions ...ConditionNode) *ConditionGroup {
	return NewConditionGroup(LogicalAnd, conditions...)
}

// Or creates an OR group with the given conditions.
func Or(conditions ...ConditionNode) *ConditionGroup {
	return NewConditionGroup(LogicalOr, conditions...)
}
