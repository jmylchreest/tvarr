package numbering

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testChannel creates a minimal channel for testing.
func testChannel(name string, number int) *models.Channel {
	return &models.Channel{
		ChannelName:   name,
		ChannelNumber: number,
		StreamURL:     "http://example.com/" + name,
	}
}

func TestStage_Sequential(t *testing.T) {
	stage := New().WithMode(NumberingModeSequential)

	channels := []*models.Channel{
		testChannel("Channel 1", 0),
		testChannel("Channel 2", 0),
		testChannel("Channel 3", 0),
	}

	proxy := &models.StreamProxy{StartingChannelNumber: 100}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 100, channels[0].ChannelNumber)
	assert.Equal(t, 101, channels[1].ChannelNumber)
	assert.Equal(t, 102, channels[2].ChannelNumber)
	assert.Equal(t, 3, result.RecordsProcessed)
	assert.Equal(t, 3, result.RecordsModified)
}

func TestStage_Preserve_NoConflicts(t *testing.T) {
	stage := New().WithMode(NumberingModePreserve)

	channels := []*models.Channel{
		testChannel("Channel 1", 5),
		testChannel("Channel 2", 10),
		testChannel("Channel 3", 0), // No number, should get assigned
	}

	proxy := &models.StreamProxy{StartingChannelNumber: 1}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Existing numbers preserved
	assert.Equal(t, 5, channels[0].ChannelNumber)
	assert.Equal(t, 10, channels[1].ChannelNumber)
	// New number assigned starting from 1
	assert.Equal(t, 1, channels[2].ChannelNumber)
	assert.Equal(t, 3, result.RecordsProcessed)
	assert.Equal(t, 1, result.RecordsModified) // Only 1 was modified
	assert.Empty(t, stage.GetConflicts())
}

func TestStage_Preserve_WithConflicts(t *testing.T) {
	stage := New().WithMode(NumberingModePreserve)

	channels := []*models.Channel{
		testChannel("Channel A", 5),
		testChannel("Channel B", 5), // Conflict!
		testChannel("Channel C", 5), // Another conflict!
		testChannel("Channel D", 10),
	}

	proxy := &models.StreamProxy{StartingChannelNumber: 1}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// First channel keeps 5
	assert.Equal(t, 5, channels[0].ChannelNumber)
	// Second channel gets incremented to 6
	assert.Equal(t, 6, channels[1].ChannelNumber)
	// Third channel gets incremented to 7
	assert.Equal(t, 7, channels[2].ChannelNumber)
	// Fourth channel keeps 10
	assert.Equal(t, 10, channels[3].ChannelNumber)

	assert.Equal(t, 4, result.RecordsProcessed)
	assert.Equal(t, 2, result.RecordsModified) // 2 conflicts resolved

	// Verify conflicts were tracked
	conflicts := stage.GetConflicts()
	assert.Len(t, conflicts, 2)

	// Verify message includes conflict count
	assert.Contains(t, result.Message, "2 conflicts resolved")
}

func TestStage_Preserve_MixedConflictsAndNew(t *testing.T) {
	stage := New().WithMode(NumberingModePreserve)

	channels := []*models.Channel{
		testChannel("Has Number", 100),
		testChannel("Conflict", 100),
		testChannel("No Number", 0),
		testChannel("Another No Number", 0),
	}

	proxy := &models.StreamProxy{StartingChannelNumber: 1}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 100, channels[0].ChannelNumber) // Kept
	assert.Equal(t, 101, channels[1].ChannelNumber) // Conflict resolved
	assert.Equal(t, 1, channels[2].ChannelNumber)   // New, starting from 1
	assert.Equal(t, 2, channels[3].ChannelNumber)   // New, sequential

	assert.Equal(t, 3, result.RecordsModified)
	assert.Len(t, stage.GetConflicts(), 1)
}

func TestStage_Group(t *testing.T) {
	stage := New().WithMode(NumberingModeGroup)

	// Create channels with group titles
	ch1 := testChannel("Sports 1", 0)
	ch1.GroupTitle = "Sports"
	ch2 := testChannel("Sports 2", 0)
	ch2.GroupTitle = "Sports"
	ch3 := testChannel("News 1", 0)
	ch3.GroupTitle = "News"
	ch4 := testChannel("Movie 1", 0)
	ch4.GroupTitle = "Movies"

	channels := []*models.Channel{ch1, ch2, ch3, ch4}

	proxy := &models.StreamProxy{StartingChannelNumber: 100}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Groups are sorted alphabetically: Movies, News, Sports
	// Movies starts at 100
	assert.Equal(t, 100, ch4.ChannelNumber)
	// News starts at 200
	assert.Equal(t, 200, ch3.ChannelNumber)
	// Sports starts at 300
	assert.Equal(t, 300, ch1.ChannelNumber)
	assert.Equal(t, 301, ch2.ChannelNumber)

	assert.Equal(t, 4, result.RecordsModified)
}

func TestStage_EmptyChannels(t *testing.T) {
	stage := New()

	proxy := &models.StreamProxy{StartingChannelNumber: 1}
	state := core.NewState(proxy)
	state.Channels = []*models.Channel{}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, "No channels to number", result.Message)
	assert.Equal(t, 0, result.RecordsProcessed)
}

func TestStage_DefaultStartingNumber(t *testing.T) {
	stage := New()

	channels := []*models.Channel{
		testChannel("Channel 1", 0),
	}

	// StartingChannelNumber is 0, should default to 1
	proxy := &models.StreamProxy{StartingChannelNumber: 0}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 1, channels[0].ChannelNumber)
}

func TestStage_ConflictsResetBetweenExecutions(t *testing.T) {
	stage := New().WithMode(NumberingModePreserve)

	// First execution with conflicts
	channels1 := []*models.Channel{
		testChannel("A", 1),
		testChannel("B", 1),
	}

	proxy := &models.StreamProxy{StartingChannelNumber: 1}
	state1 := core.NewState(proxy)
	state1.Channels = channels1

	_, err := stage.Execute(context.Background(), state1)
	require.NoError(t, err)
	assert.Len(t, stage.GetConflicts(), 1)

	// Second execution without conflicts
	channels2 := []*models.Channel{
		testChannel("C", 1),
		testChannel("D", 2),
	}

	state2 := core.NewState(proxy)
	state2.Channels = channels2

	_, err = stage.Execute(context.Background(), state2)
	require.NoError(t, err)
	// Conflicts should be reset
	assert.Empty(t, stage.GetConflicts())
}

// Integration tests for proxy configuration

func TestStage_ProxyConfigOverridesStageDefaults(t *testing.T) {
	// Stage defaults to sequential mode
	stage := New()

	channels := []*models.Channel{
		testChannel("Channel A", 5),
		testChannel("Channel B", 5), // Conflict
		testChannel("Channel C", 0),
	}

	// Proxy specifies preserve mode
	proxy := &models.StreamProxy{
		StartingChannelNumber: 1,
		NumberingMode:         models.NumberingModePreserve,
	}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Preserve mode should keep existing numbers and resolve conflicts
	assert.Equal(t, 5, channels[0].ChannelNumber) // Kept
	assert.Equal(t, 6, channels[1].ChannelNumber) // Conflict resolved
	assert.Equal(t, 1, channels[2].ChannelNumber) // Assigned starting number
}

func TestStage_ProxyGroupSizeConfiguration(t *testing.T) {
	stage := New()

	// Create channels with group titles
	ch1 := testChannel("Sports 1", 0)
	ch1.GroupTitle = "Sports"
	ch2 := testChannel("News 1", 0)
	ch2.GroupTitle = "News"

	channels := []*models.Channel{ch1, ch2}

	// Proxy specifies group mode with custom group size
	proxy := &models.StreamProxy{
		StartingChannelNumber: 1,
		NumberingMode:         models.NumberingModeGroup,
		GroupNumberingSize:    50, // Custom: 50 instead of default 100
	}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Groups are sorted alphabetically: News, Sports
	// With groupSize=50: News starts at 1, Sports starts at 51
	assert.Equal(t, 1, ch2.ChannelNumber)  // News at 1
	assert.Equal(t, 51, ch1.ChannelNumber) // Sports at 51
}

func TestStage_ProxyGroupSizeZeroUsesDefault(t *testing.T) {
	stage := New()

	ch1 := testChannel("A Channel", 0)
	ch1.GroupTitle = "Group A"
	ch2 := testChannel("B Channel", 0)
	ch2.GroupTitle = "Group B"

	channels := []*models.Channel{ch1, ch2}

	// Proxy specifies group mode but GroupNumberingSize is 0 (default)
	proxy := &models.StreamProxy{
		StartingChannelNumber: 1,
		NumberingMode:         models.NumberingModeGroup,
		GroupNumberingSize:    0, // Zero means use stage default (100)
	}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// With default groupSize=100: Group A at 1, Group B at 101
	assert.Equal(t, 1, ch1.ChannelNumber)   // Group A at 1
	assert.Equal(t, 101, ch2.ChannelNumber) // Group B at 101
}

func TestStage_StageMethodConfigurationStillWorks(t *testing.T) {
	// Configure stage via methods
	stage := New().WithMode(NumberingModeGroup).WithGroupSize(25)

	ch1 := testChannel("X", 0)
	ch1.GroupTitle = "First"
	ch2 := testChannel("Y", 0)
	ch2.GroupTitle = "Second"

	channels := []*models.Channel{ch1, ch2}

	// Proxy with empty NumberingMode - should use stage default
	proxy := &models.StreamProxy{
		StartingChannelNumber: 10,
		NumberingMode:         "", // Empty - use stage default
		GroupNumberingSize:    0,  // Zero - use stage default
	}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Stage configured with groupSize=25: First at 10, Second at 35
	assert.Equal(t, 10, ch1.ChannelNumber) // First at 10
	assert.Equal(t, 35, ch2.ChannelNumber) // Second at 35
}

func TestStage_ArtifactMetadataContainsConfiguration(t *testing.T) {
	stage := New()

	channels := []*models.Channel{
		testChannel("Channel 1", 0),
	}

	proxy := &models.StreamProxy{
		StartingChannelNumber: 100,
		NumberingMode:         models.NumberingModeSequential,
	}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, result.Artifacts, 1)
	artifact := result.Artifacts[0]

	assert.Equal(t, 100, artifact.Metadata["starting_number"])
	assert.Equal(t, 0, artifact.Metadata["conflicts_resolved"])
}

func TestStage_UncategorizedGroupHandling(t *testing.T) {
	stage := New()

	// Mix of channels with and without group titles
	ch1 := testChannel("Sports", 0)
	ch1.GroupTitle = "Sports"
	ch2 := testChannel("No Group", 0)
	ch2.GroupTitle = "" // Will be "Uncategorized"
	ch3 := testChannel("News", 0)
	ch3.GroupTitle = "News"

	channels := []*models.Channel{ch1, ch2, ch3}

	proxy := &models.StreamProxy{
		StartingChannelNumber: 1,
		NumberingMode:         models.NumberingModeGroup,
		GroupNumberingSize:    100,
	}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Groups sorted: News, Sports, Uncategorized
	// Group 0 (News): starts at 1
	// Group 1 (Sports): starts at 101
	// Group 2 (Uncategorized): starts at 201
	assert.Equal(t, 101, ch1.ChannelNumber) // Sports at 101
	assert.Equal(t, 201, ch2.ChannelNumber) // Uncategorized at 201
	assert.Equal(t, 1, ch3.ChannelNumber)   // News at 1
}
