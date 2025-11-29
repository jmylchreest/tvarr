package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleProcessor_ApplySet(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "UK",
	})

	processor := NewRuleProcessor()

	// Parse a rule with SET action
	rule, err := Parse(`channel_name contains "BBC" SET group_title = "UK Channels"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)

	// Check the modification
	mod := result.Modifications[0]
	assert.Equal(t, "group_title", mod.Field)
	assert.Equal(t, "UK", mod.OldValue)
	assert.Equal(t, "UK Channels", mod.NewValue)
	assert.Equal(t, ActionSet, mod.Action)

	// Verify the context was updated
	value, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK Channels", value)
}

func TestRuleProcessor_ApplySetIfEmpty(t *testing.T) {
	t.Run("empty field", func(t *testing.T) {
		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One",
			"tvg_logo":     "",
		})

		processor := NewRuleProcessor()

		rule, err := Parse(`channel_name contains "BBC" SET_IF_EMPTY tvg_logo = "bbc.png"`)
		require.NoError(t, err)

		result, err := processor.Apply(rule, ctx)
		require.NoError(t, err)

		assert.True(t, result.Matched)
		assert.Len(t, result.Modifications, 1)

		value, _ := ctx.GetFieldValue("tvg_logo")
		assert.Equal(t, "bbc.png", value)
	})

	t.Run("non-empty field", func(t *testing.T) {
		ctx := NewChannelEvalContext(map[string]string{
			"channel_name": "BBC One",
			"tvg_logo":     "existing.png",
		})

		processor := NewRuleProcessor()

		rule, err := Parse(`channel_name contains "BBC" SET_IF_EMPTY tvg_logo = "bbc.png"`)
		require.NoError(t, err)

		result, err := processor.Apply(rule, ctx)
		require.NoError(t, err)

		assert.True(t, result.Matched)
		assert.Len(t, result.Modifications, 0) // No modification because field wasn't empty

		value, _ := ctx.GetFieldValue("tvg_logo")
		assert.Equal(t, "existing.png", value)
	})
}

func TestRuleProcessor_ApplyAppend(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`channel_name contains "BBC" APPEND channel_name = " HD"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)

	value, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One HD", value)
}

func TestRuleProcessor_ApplyRemove(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD [UK]",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`channel_name contains "[" REMOVE channel_name = " [UK]"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)

	value, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One HD", value)
}

func TestRuleProcessor_ApplyDelete(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name":   "BBC One",
		"unwanted_field": "some value",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`channel_name contains "BBC" DELETE unwanted_field`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)

	// Field should be empty after delete
	value, ok := ctx.GetFieldValue("unwanted_field")
	assert.True(t, ok) // Field exists but is empty
	assert.Equal(t, "", value)
}

func TestRuleProcessor_NoMatch(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "ITV One",
		"group_title":  "UK",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`channel_name contains "BBC" SET group_title = "UK Channels"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.False(t, result.Matched)
	assert.Len(t, result.Modifications, 0)

	// Context should be unchanged
	value, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK", value)
}

func TestRuleProcessor_MultipleActions(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
		"group_title":  "",
		"tvg_logo":     "",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`channel_name contains "BBC" SET group_title = "UK", tvg_logo = "bbc.png"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 2)

	value, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK", value)

	value, _ = ctx.GetFieldValue("tvg_logo")
	assert.Equal(t, "bbc.png", value)
}

func TestRuleProcessor_RegexCapture(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
	})

	processor := NewRuleProcessor()

	// Use regex capture to extract "BBC One" without "HD"
	rule, err := Parse(`channel_name matches "(.+) HD" SET channel_name = $1`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)

	value, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One", value)
}

func TestRuleProcessor_RegexCaptureMultiple(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "UK: BBC One HD",
		"group_title":  "",
	})

	processor := NewRuleProcessor()

	// Extract country and name
	rule, err := Parse(`channel_name matches "(.+): (.+) HD" SET group_title = $1, channel_name = $2`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 2)

	groupTitle, _ := ctx.GetFieldValue("group_title")
	assert.Equal(t, "UK", groupTitle)

	channelName, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "BBC One", channelName)
}

func TestRuleProcessor_CaptureInString(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
	})

	processor := NewRuleProcessor()

	// Use capture reference in a string template
	rule, err := Parse(`channel_name matches "(.+) HD" SET channel_name = "Channel: $1"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)

	value, _ := ctx.GetFieldValue("channel_name")
	assert.Equal(t, "Channel: BBC One", value)
}

func TestRuleProcessor_FieldReference(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
		"tvg_name":     "",
	})

	processor := NewRuleProcessor()

	rule, err := Parse(`tvg_name equals "" SET tvg_name = $channel_name`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)

	value, _ := ctx.GetFieldValue("tvg_name")
	assert.Equal(t, "BBC One", value)
}

func TestRuleProcessor_ConditionOnlyNoAction(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	processor := NewRuleProcessor()

	// Condition-only expression (no SET)
	rule, err := Parse(`channel_name contains "BBC"`)
	require.NoError(t, err)

	result, err := processor.Apply(rule, ctx)
	require.NoError(t, err)

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 0)
}

func TestRuleResult(t *testing.T) {
	result := &RuleResult{
		Matched: true,
		Modifications: []FieldModification{
			{
				Field:    "group_title",
				OldValue: "old",
				NewValue: "new",
				Action:   ActionSet,
			},
		},
	}

	assert.True(t, result.Matched)
	assert.Len(t, result.Modifications, 1)
	assert.Equal(t, "group_title", result.Modifications[0].Field)
}
