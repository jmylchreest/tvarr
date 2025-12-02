package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCondition_Creation(t *testing.T) {
	cond := NewCondition("channel_name", OpEquals, "BBC One")

	assert.Equal(t, "channel_name", cond.Field)
	assert.Equal(t, OpEquals, cond.Operator)
	assert.Equal(t, "BBC One", cond.Value)
	assert.False(t, cond.CaseSensitive)
}

func TestConditionGroup_And(t *testing.T) {
	c1 := NewCondition("field1", OpEquals, "value1")
	c2 := NewCondition("field2", OpContains, "value2")

	group := And(c1, c2)

	assert.Equal(t, LogicalAnd, group.Operator)
	assert.Len(t, group.Children, 2)
	assert.Equal(t, c1, group.Children[0])
	assert.Equal(t, c2, group.Children[1])
}

func TestConditionGroup_Or(t *testing.T) {
	c1 := NewCondition("field1", OpEquals, "value1")
	c2 := NewCondition("field2", OpContains, "value2")

	group := Or(c1, c2)

	assert.Equal(t, LogicalOr, group.Operator)
	assert.Len(t, group.Children, 2)
	assert.Equal(t, c1, group.Children[0])
	assert.Equal(t, c2, group.Children[1])
}

func TestConditionGroup_Nested(t *testing.T) {
	// (A AND B) OR (C AND D)
	a := NewCondition("a", OpEquals, "1")
	b := NewCondition("b", OpEquals, "2")
	c := NewCondition("c", OpEquals, "3")
	d := NewCondition("d", OpEquals, "4")

	ab := And(a, b)
	cd := And(c, d)
	combined := Or(ab, cd)

	assert.Equal(t, LogicalOr, combined.Operator)
	assert.Len(t, combined.Children, 2)

	left, ok := combined.Children[0].(*ConditionGroup)
	assert.True(t, ok)
	assert.Equal(t, LogicalAnd, left.Operator)

	right, ok := combined.Children[1].(*ConditionGroup)
	assert.True(t, ok)
	assert.Equal(t, LogicalAnd, right.Operator)
}

func TestConditionTree_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		tree     *ConditionTree
		expected bool
	}{
		{
			name:     "nil tree",
			tree:     nil,
			expected: true,
		},
		{
			name:     "nil root",
			tree:     &ConditionTree{Root: nil},
			expected: true,
		},
		{
			name:     "with condition",
			tree:     NewConditionTree(NewCondition("field", OpEquals, "value")),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.tree.IsEmpty())
		})
	}
}

func TestAction_Creation(t *testing.T) {
	action := NewAction("group_title", ActionSet, NewLiteralValue("Sports"))

	assert.Equal(t, "group_title", action.Field)
	assert.Equal(t, ActionSet, action.Operator)

	lit, ok := action.Value.(*LiteralValue)
	assert.True(t, ok)
	assert.Equal(t, "Sports", lit.Value)
}

func TestAction_Delete(t *testing.T) {
	action := NewAction("unwanted_field", ActionDelete, nil)

	assert.Equal(t, "unwanted_field", action.Field)
	assert.Equal(t, ActionDelete, action.Operator)
	assert.Nil(t, action.Value)
}

func TestActionValue_Types(t *testing.T) {
	t.Run("LiteralValue", func(t *testing.T) {
		val := NewLiteralValue("hello")
		assert.Equal(t, "hello", val.Value)

		// Verify it implements ActionValue interface
		var _ ActionValue = val
	})

	t.Run("NullValue", func(t *testing.T) {
		val := &NullValue{}

		// Verify it implements ActionValue interface
		var _ ActionValue = val
	})

	t.Run("FieldReference", func(t *testing.T) {
		val := &FieldReference{Field: "source_field"}
		assert.Equal(t, "source_field", val.Field)

		// Verify it implements ActionValue interface
		var _ ActionValue = val
	})

	t.Run("CaptureReference", func(t *testing.T) {
		val := &CaptureReference{Index: 1}
		assert.Equal(t, 1, val.Index)

		// Verify it implements ActionValue interface
		var _ ActionValue = val
	})
}

func TestConditionOnly(t *testing.T) {
	cond := NewCondition("channel_name", OpContains, "BBC")
	tree := NewConditionTree(cond)

	expr := &ConditionOnly{Condition: tree}

	// Verify it implements ExtendedExpression interface
	var _ ExtendedExpression = expr

	assert.False(t, expr.Condition.IsEmpty())
}

func TestConditionWithActions(t *testing.T) {
	cond := NewCondition("channel_name", OpContains, "BBC")
	tree := NewConditionTree(cond)

	actions := []*Action{
		NewAction("group_title", ActionSet, NewLiteralValue("UK Channels")),
		NewAction("logo", ActionSetIfEmpty, NewLiteralValue("bbc.png")),
	}

	expr := &ConditionWithActions{
		Condition: tree,
		Actions:   actions,
	}

	// Verify it implements ExtendedExpression interface
	var _ ExtendedExpression = expr

	assert.False(t, expr.Condition.IsEmpty())
	assert.Len(t, expr.Actions, 2)
}

func TestParsedExpression_Metadata(t *testing.T) {
	parsed := &ParsedExpression{
		Original:         `channel_name contains "BBC" SET group_title = "UK"`,
		HasActions:       true,
		UsesRegex:        false,
		ReferencedFields: []string{"channel_name"},
		ModifiedFields:   []string{"group_title"},
	}

	assert.Equal(t, `channel_name contains "BBC" SET group_title = "UK"`, parsed.Original)
	assert.True(t, parsed.HasActions)
	assert.False(t, parsed.UsesRegex)
	assert.Contains(t, parsed.ReferencedFields, "channel_name")
	assert.Contains(t, parsed.ModifiedFields, "group_title")
}

func TestNode_Interfaces(t *testing.T) {
	// Ensure all types implement their respective interfaces

	t.Run("ConditionNode implementations", func(t *testing.T) {
		var _ ConditionNode = &Condition{}
		var _ ConditionNode = &ConditionGroup{}
	})

	t.Run("ActionValue implementations", func(t *testing.T) {
		var _ ActionValue = &LiteralValue{}
		var _ ActionValue = &NullValue{}
		var _ ActionValue = &FieldReference{}
		var _ ActionValue = &CaptureReference{}
	})

	t.Run("ExtendedExpression implementations", func(t *testing.T) {
		var _ ExtendedExpression = &ConditionOnly{}
		var _ ExtendedExpression = &ConditionWithActions{}
		var _ ExtendedExpression = &ConditionalActionGroup{}
	})

	t.Run("Node implementations", func(t *testing.T) {
		var _ Node = &Condition{}
		var _ Node = &ConditionGroup{}
		var _ Node = &ConditionTree{}
		var _ Node = &Action{}
		var _ Node = &LiteralValue{}
		var _ Node = &NullValue{}
		var _ Node = &FieldReference{}
		var _ Node = &CaptureReference{}
		var _ Node = &ConditionOnly{}
		var _ Node = &ConditionWithActions{}
		var _ Node = &ConditionalActionGroup{}
	})
}
