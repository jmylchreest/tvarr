package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataMappingEngine_SingleRule(t *testing.T) {
	engine := NewDataMappingEngine()

	// Add a rule
	rule, err := Parse(`channel_name contains "BBC" SET group_title = "UK Channels"`)
	require.NoError(t, err)
	engine.AddRule(rule)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, result.RulesMatched)
	assert.Equal(t, 1, result.TotalModifications)

	value, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK Channels", value)
}

func TestDataMappingEngine_MultipleRules(t *testing.T) {
	engine := NewDataMappingEngine()

	// Add multiple rules
	rule1, _ := Parse(`channel_name contains "BBC" SET group_title = "UK"`)
	rule2, _ := Parse(`channel_name ends_with "HD" APPEND channel_name = " [HD]"`)
	engine.AddRule(rule1)
	engine.AddRule(rule2)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, result.RulesMatched)
	assert.Equal(t, 2, result.TotalModifications)

	// Both rules should have applied
	group, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK", group)

	name, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One HD [HD]", name)
}

func TestDataMappingEngine_RuleOrder(t *testing.T) {
	engine := NewDataMappingEngine()

	// Rules should apply in order
	rule1, _ := Parse(`channel_name contains "BBC" SET channel_name = "Modified BBC"`)
	rule2, _ := Parse(`channel_name contains "Modified" SET group_title = "Was Modified"`)
	engine.AddRule(rule1)
	engine.AddRule(rule2)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, result.RulesMatched)

	// Second rule should match because first rule modified the channel_name
	group, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "Was Modified", group)
}

func TestDataMappingEngine_NoMatch(t *testing.T) {
	engine := NewDataMappingEngine()

	rule, _ := Parse(`channel_name contains "BBC" SET group_title = "UK"`)
	engine.AddRule(rule)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "ITV One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, result.RulesMatched)
	assert.Equal(t, 0, result.TotalModifications)

	// group_title should be empty
	group, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "", group)
}

func TestDataMappingEngine_StopOnFirstMatch(t *testing.T) {
	engine := NewDataMappingEngine()
	engine.SetStopOnFirstMatch(true)

	rule1, _ := Parse(`channel_name contains "BBC" SET group_title = "Rule 1"`)
	rule2, _ := Parse(`channel_name contains "BBC" SET group_title = "Rule 2"`)
	engine.AddRule(rule1)
	engine.AddRule(rule2)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, result.RulesMatched) // Only first rule matched
	assert.Equal(t, 1, result.TotalModifications)

	// Only first rule's action should apply
	group, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "Rule 1", group)
}

func TestDataMappingEngine_ModificationTracking(t *testing.T) {
	engine := NewDataMappingEngine()

	rule, _ := Parse(`channel_name contains "BBC" SET group_title = "UK", tvg_logo = "bbc.png"`)
	engine.AddRule(rule)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
		"group_title":  "Old",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, result.TotalModifications)
	require.Len(t, result.AllModifications, 2)

	// Check modifications are tracked
	var groupMod, logoMod *FieldModification
	for i := range result.AllModifications {
		switch result.AllModifications[i].Field {
		case "group_title":
			groupMod = &result.AllModifications[i]
		case "tvg_logo":
			logoMod = &result.AllModifications[i]
		}
	}

	require.NotNil(t, groupMod)
	assert.Equal(t, "Old", groupMod.OldValue)
	assert.Equal(t, "UK", groupMod.NewValue)

	require.NotNil(t, logoMod)
	assert.Equal(t, "", logoMod.OldValue)
	assert.Equal(t, "bbc.png", logoMod.NewValue)
}

func TestDataMappingEngine_RuleChaining(t *testing.T) {
	engine := NewDataMappingEngine()

	// Chain of transformations
	rule1, _ := Parse(`channel_name matches "(.+) HD$" SET channel_name = $1`)
	rule2, _ := Parse(`channel_name matches "UK: (.+)" SET channel_name = $1, group_title = "UK"`)
	rule3, _ := Parse(`channel_name not_equals "" SET_IF_EMPTY tvg_name = $channel_name`)

	engine.AddRule(rule1)
	engine.AddRule(rule2)
	engine.AddRule(rule3)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "UK: BBC One HD",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 3, result.RulesMatched)

	// After rule 1: "UK: BBC One"
	// After rule 2: "BBC One", group = "UK"
	// After rule 3: tvg_name = "BBC One"
	name, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One", name)

	group, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK", group)

	tvgName, _ := ctx.GetFieldValue("tvg_name")
	assert.Equal(t, "BBC One", tvgName)
}

func TestDataMappingEngine_EmptyRules(t *testing.T) {
	engine := NewDataMappingEngine()

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, result.RulesMatched)
	assert.Equal(t, 0, result.TotalModifications)
}

func TestDataMappingEngine_FilterRulesOnly(t *testing.T) {
	engine := NewDataMappingEngine()

	// Condition-only rules (filters) should match but not modify
	rule, _ := Parse(`channel_name contains "BBC"`)
	engine.AddRule(rule)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, result.RulesMatched)
	assert.Equal(t, 0, result.TotalModifications)
}

func TestMappingResult(t *testing.T) {
	result := &MappingResult{
		RulesMatched:       2,
		TotalModifications: 3,
		AllModifications: []FieldModification{
			{Field: "field1", OldValue: "a", NewValue: "b", Action: ActionSet},
			{Field: "field2", OldValue: "", NewValue: "c", Action: ActionSet},
			{Field: "field3", OldValue: "d", NewValue: "de", Action: ActionAppend},
		},
	}

	assert.Equal(t, 2, result.RulesMatched)
	assert.Equal(t, 3, result.TotalModifications)
	assert.Len(t, result.AllModifications, 3)
}

func TestDataMappingEngine_AddRuleString(t *testing.T) {
	engine := NewDataMappingEngine()

	err := engine.AddRuleString(`channel_name contains "BBC" SET group_title = "UK"`)
	require.NoError(t, err)

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, result.RulesMatched)
}

func TestDataMappingEngine_AddRuleStringError(t *testing.T) {
	engine := NewDataMappingEngine()

	err := engine.AddRuleString(`invalid expression (((`)
	assert.Error(t, err)
}

func TestDataMappingEngine_ClearRules(t *testing.T) {
	engine := NewDataMappingEngine()

	rule, _ := Parse(`channel_name contains "BBC" SET group_title = "UK"`)
	engine.AddRule(rule)

	// Clear all rules
	engine.ClearRules()

	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	result, err := engine.Process(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, result.RulesMatched)
}
