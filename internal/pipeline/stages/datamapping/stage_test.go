package datamapping

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *core.State {
	t.Helper()
	proxy := &models.StreamProxy{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "Test Proxy",
	}
	state := core.NewState(proxy)
	state.TempDir = t.TempDir()
	return state
}

func TestStage_New(t *testing.T) {
	stage := New()
	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestStage_Execute_NoRules(t *testing.T) {
	stage := New()
	state := newTestState(t)

	// Add a channel
	state.Channels = []*models.Channel{
		{ChannelName: "Test Channel", TvgLogo: "http://example.com/logo.png"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.Message, "No data mapping rules configured")
}

func TestStage_Execute_ChannelLogoRule(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "test-rule-1",
			Name:       "Replace channel logos",
			Enabled:    true,
			Target:     RuleTargetChannel,
			Priority:   100,
			Expression: `tvg_logo starts_with "http" SET tvg_logo = "@logo:01TESTULID123456789AB"`,
		},
	})

	state := newTestState(t)
	state.Channels = []*models.Channel{
		{ChannelName: "Channel 1", TvgLogo: "http://example.com/logo1.png"},
		{ChannelName: "Channel 2", TvgLogo: "http://example.com/logo2.png"},
		{ChannelName: "Channel 3", TvgLogo: ""}, // No logo, should not match
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check that logos were replaced
	assert.Equal(t, "@logo:01TESTULID123456789AB", state.Channels[0].TvgLogo)
	assert.Equal(t, "@logo:01TESTULID123456789AB", state.Channels[1].TvgLogo)
	assert.Equal(t, "", state.Channels[2].TvgLogo) // Should remain empty
}

func TestStage_Execute_ProgramIconRule(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "test-rule-2",
			Name:       "Replace program icons",
			Enabled:    true,
			Target:     RuleTargetProgram,
			Priority:   100,
			Expression: `programme_icon starts_with "http" SET programme_icon = "@logo:01TESTULID987654321XY"`,
		},
	})

	state := newTestState(t)
	state.Programs = []*models.EpgProgram{
		{Title: "Program 1", Icon: "http://example.com/icon1.jpg"},
		{Title: "Program 2", Icon: "http://example.com/icon2.jpg"},
		{Title: "Program 3", Icon: ""},                // No icon, should not match
		{Title: "Program 4", Icon: "/local/path.jpg"}, // Not http, should not match
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check that icons were replaced
	assert.Equal(t, "@logo:01TESTULID987654321XY", state.Programs[0].Icon)
	assert.Equal(t, "@logo:01TESTULID987654321XY", state.Programs[1].Icon)
	assert.Equal(t, "", state.Programs[2].Icon)                // Should remain empty
	assert.Equal(t, "/local/path.jpg", state.Programs[3].Icon) // Should remain unchanged
}

func TestStage_Execute_ProgramIconAlias(t *testing.T) {
	// Test using the "icon" alias instead of "programme_icon"
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "test-rule-3",
			Name:       "Replace program icons using alias",
			Enabled:    true,
			Target:     RuleTargetProgram,
			Priority:   100,
			Expression: `icon starts_with "http" SET icon = "@logo:01ALIASULID12345678ZZ"`,
		},
	})

	state := newTestState(t)
	state.Programs = []*models.EpgProgram{
		{Title: "Program 1", Icon: "http://example.com/icon.jpg"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check that icon was replaced using alias
	assert.Equal(t, "@logo:01ALIASULID12345678ZZ", state.Programs[0].Icon)
}

func TestStage_Execute_MixedRules(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "channel-rule",
			Name:       "Replace channel logos",
			Enabled:    true,
			Target:     RuleTargetChannel,
			Priority:   100,
			Expression: `tvg_logo starts_with "http" SET tvg_logo = "@logo:CHANNEL123"`,
		},
		{
			ID:         "program-rule",
			Name:       "Replace program icons",
			Enabled:    true,
			Target:     RuleTargetProgram,
			Priority:   100,
			Expression: `programme_icon starts_with "http" SET programme_icon = "@logo:PROGRAM456"`,
		},
	})

	state := newTestState(t)
	state.Channels = []*models.Channel{
		{ChannelName: "Channel 1", TvgLogo: "http://example.com/channel.png"},
	}
	state.Programs = []*models.EpgProgram{
		{Title: "Program 1", Icon: "http://example.com/program.jpg"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check both were replaced
	assert.Equal(t, "@logo:CHANNEL123", state.Channels[0].TvgLogo)
	assert.Equal(t, "@logo:PROGRAM456", state.Programs[0].Icon)
}

func TestStage_Execute_DisabledRule(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "disabled-rule",
			Name:       "Disabled rule",
			Enabled:    false, // Disabled
			Target:     RuleTargetChannel,
			Priority:   100,
			Expression: `tvg_logo starts_with "http" SET tvg_logo = "@logo:SHOULDNOTAPPLY"`,
		},
	})

	state := newTestState(t)
	state.Channels = []*models.Channel{
		{ChannelName: "Channel 1", TvgLogo: "http://example.com/logo.png"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Logo should remain unchanged since rule is disabled
	assert.Equal(t, "http://example.com/logo.png", state.Channels[0].TvgLogo)
}

func TestStage_Execute_ChannelNameRule(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "name-rule",
			Name:       "Append HD to channel names",
			Enabled:    true,
			Target:     RuleTargetChannel,
			Priority:   100,
			Expression: `channel_name contains "News" SET channel_name = "HD News Channel"`,
		},
	})

	state := newTestState(t)
	state.Channels = []*models.Channel{
		{ChannelName: "Breaking News"},
		{ChannelName: "Sports Channel"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Only the News channel should be modified
	assert.Equal(t, "HD News Channel", state.Channels[0].ChannelName)
	assert.Equal(t, "Sports Channel", state.Channels[1].ChannelName)
}

func TestStage_Execute_ProgramTitleRule(t *testing.T) {
	stage := New()
	stage.WithRules([]DataMappingRule{
		{
			ID:         "title-rule",
			Name:       "Clean program titles",
			Enabled:    true,
			Target:     RuleTargetProgram,
			Priority:   100,
			Expression: `programme_title contains "RERUN" SET programme_title = "Classic Show"`,
		},
	})

	state := newTestState(t)
	state.Programs = []*models.EpgProgram{
		{Title: "RERUN: Old Movie"},
		{Title: "New Show"},
	}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Only the RERUN program should be modified
	assert.Equal(t, "Classic Show", state.Programs[0].Title)
	assert.Equal(t, "New Show", state.Programs[1].Title)
}

func TestStage_CreateProgramContext_IncludesIcon(t *testing.T) {
	stage := New()
	prog := &models.EpgProgram{
		Title:       "Test Program",
		Description: "Test Description",
		Category:    "Drama",
		Icon:        "http://example.com/icon.png",
	}

	ctx := stage.createProgramContext(prog)

	// Verify programme_icon is accessible
	iconVal, ok := ctx.GetFieldValue("programme_icon")
	assert.True(t, ok, "programme_icon should be accessible")
	assert.Equal(t, "http://example.com/icon.png", iconVal)

	// Verify icon alias works
	iconAliasVal, ok := ctx.GetFieldValue("icon")
	assert.True(t, ok, "icon alias should be accessible")
	assert.Equal(t, "http://example.com/icon.png", iconAliasVal)
}

func TestStage_CreateChannelContext_IncludesLogo(t *testing.T) {
	stage := New()
	ch := &models.Channel{
		ChannelName: "Test Channel",
		TvgID:       "test-id",
		TvgName:     "Test Name",
		TvgLogo:     "http://example.com/logo.png",
		GroupTitle:  "Entertainment",
		StreamURL:   "http://example.com/stream.m3u8",
	}

	ctx := stage.createChannelContext(ch)

	// Verify tvg_logo is accessible
	logoVal, ok := ctx.GetFieldValue("tvg_logo")
	assert.True(t, ok, "tvg_logo should be accessible")
	assert.Equal(t, "http://example.com/logo.png", logoVal)

	// Verify logo alias works
	logoAliasVal, ok := ctx.GetFieldValue("logo")
	assert.True(t, ok, "logo alias should be accessible")
	assert.Equal(t, "http://example.com/logo.png", logoAliasVal)
}

func TestNewConstructor(t *testing.T) {
	constructor := NewConstructor()
	// Pass empty dependencies instead of nil to avoid nil pointer
	deps := &core.Dependencies{}
	stage := constructor(deps)
	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}
