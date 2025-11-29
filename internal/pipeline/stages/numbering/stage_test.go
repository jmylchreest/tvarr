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
		testChannel("Channel B", 5),  // Conflict!
		testChannel("Channel C", 5),  // Another conflict!
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
