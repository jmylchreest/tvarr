package generatexmltv

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *core.State {
	t.Helper()
	tempDir := t.TempDir()
	proxy := &models.StreamProxy{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "Test Proxy",
	}
	state := core.NewState(proxy)
	state.TempDir = tempDir
	return state
}

// T012-TEST: Test XMLTV generation produces valid output with <tv>, <channel>, and <programme> elements
func TestStage_Execute_ProducesValidXMLTV(t *testing.T) {
	t.Run("generates valid XMLTV with tv, channel, and programme elements", func(t *testing.T) {
		state := newTestState(t)

		// Add channels
		state.Channels = []*models.Channel{
			{
				TvgID:       "channel1",
				TvgName:     "Channel One",
				TvgLogo:     "http://example.com/logo1.png",
				ChannelName: "Channel One HD",
			},
			{
				TvgID:       "channel2",
				TvgName:     "Channel Two",
				ChannelName: "Channel Two HD",
			},
		}

		// Add programs
		now := time.Now()
		state.Programs = []*models.EpgProgram{
			{
				ChannelID: "channel1",
				Title:     "Morning Show",
				Start:     now,
				Stop:      now.Add(1 * time.Hour),
			},
			{
				ChannelID:   "channel1",
				Title:       "News at Noon",
				Description: "Daily news update",
				Start:       now.Add(1 * time.Hour),
				Stop:        now.Add(2 * time.Hour),
			},
			{
				ChannelID: "channel2",
				Title:     "Sports Hour",
				Start:     now,
				Stop:      now.Add(1 * time.Hour),
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify result
		assert.Equal(t, 3, result.RecordsProcessed)
		assert.Contains(t, result.Message, "2 channels")
		assert.Contains(t, result.Message, "3 programs")

		// Verify file was created
		xmltvPath, ok := state.GetMetadata(MetadataKeyTempPath)
		require.True(t, ok, "XMLTV path should be in metadata")
		pathStr, ok := xmltvPath.(string)
		require.True(t, ok, "XMLTV path should be a string")

		// Read and validate XMLTV content
		content, err := os.ReadFile(pathStr)
		require.NoError(t, err)

		contentStr := string(content)

		// Verify XML declaration and <tv> element
		assert.Contains(t, contentStr, `<?xml version="1.0" encoding="UTF-8"?>`)
		assert.Contains(t, contentStr, `<tv generator-info-name="tvarr"`)
		assert.Contains(t, contentStr, `</tv>`)

		// Verify <channel> elements
		assert.Contains(t, contentStr, `<channel id="channel1">`)
		assert.Contains(t, contentStr, `<display-name>Channel One</display-name>`)
		assert.Contains(t, contentStr, `<channel id="channel2">`)
		assert.Contains(t, contentStr, `</channel>`)

		// Verify <programme> elements
		assert.Contains(t, contentStr, `<programme start=`)
		assert.Contains(t, contentStr, `channel="channel1"`)
		assert.Contains(t, contentStr, `<title lang="en">Morning Show</title>`)
		assert.Contains(t, contentStr, `<title lang="en">News at Noon</title>`)
		assert.Contains(t, contentStr, `<desc lang="en">Daily news update</desc>`)
		assert.Contains(t, contentStr, `</programme>`)
	})

	t.Run("handles no channels gracefully", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{}
		state.Programs = []*models.EpgProgram{}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.RecordsProcessed)
	})

	t.Run("creates artifact with file info", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "test",
				ChannelName: "Test Channel",
			},
		}
		state.Programs = []*models.EpgProgram{
			{
				ChannelID: "test",
				Title:     "Test Show",
				Start:     time.Now(),
				Stop:      time.Now().Add(1 * time.Hour),
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)

		require.Len(t, result.Artifacts, 1)
		artifact := result.Artifacts[0]
		assert.Equal(t, core.ArtifactTypeXMLTV, artifact.Type)
		assert.Equal(t, 1, artifact.RecordCount)
		assert.Greater(t, artifact.FileSize, int64(0))
	})
}

// T013-TEST: Test skipping programs with missing required fields
func TestStage_Execute_SkipsMissingFields(t *testing.T) {
	t.Run("skips programs with missing Title", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "channel1",
				ChannelName: "Channel One",
			},
		}

		now := time.Now()
		state.Programs = []*models.EpgProgram{
			{
				ChannelID: "channel1",
				Title:     "Valid Show",
				Start:     now,
				Stop:      now.Add(1 * time.Hour),
			},
			{
				ChannelID: "channel1",
				Title:     "", // Empty title - should be skipped
				Start:     now.Add(1 * time.Hour),
				Stop:      now.Add(2 * time.Hour),
			},
			{
				ChannelID: "channel1",
				Title:     "Another Valid Show",
				Start:     now.Add(2 * time.Hour),
				Stop:      now.Add(3 * time.Hour),
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)

		// Should only process 2 programs (skipping the one with empty title)
		assert.Equal(t, 2, result.RecordsProcessed)
	})

	t.Run("skips channels without TvgID", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "valid_channel",
				ChannelName: "Valid Channel",
			},
			{
				TvgID:       "", // Empty TvgID - should be skipped
				ChannelName: "No ID Channel",
			},
		}

		now := time.Now()
		state.Programs = []*models.EpgProgram{
			{
				ChannelID: "valid_channel",
				Title:     "Show on Valid Channel",
				Start:     now,
				Stop:      now.Add(1 * time.Hour),
			},
		}

		stage := New()
		_, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)

		// Verify content only contains valid channel
		xmltvPath, _ := state.GetMetadata(MetadataKeyTempPath)
		content, err := os.ReadFile(xmltvPath.(string))
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, `<channel id="valid_channel">`)
		// Should not contain a channel with empty ID (it would be `channel id=""`)
		assert.False(t, strings.Contains(contentStr, `channel id=""`))
	})
}

func TestStage_Interface(t *testing.T) {
	stage := New()
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestStage_ContextCancellation(t *testing.T) {
	state := newTestState(t)
	// Create many channels and programs to increase chance of cancellation during iteration
	for range 100 {
		state.Channels = append(state.Channels, &models.Channel{
			TvgID:       "test",
			ChannelName: "Test",
		})
		state.Programs = append(state.Programs, &models.EpgProgram{
			ChannelID: "test",
			Title:     "Test",
			Start:     time.Now(),
			Stop:      time.Now().Add(1 * time.Hour),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	stage := New()
	_, err := stage.Execute(ctx, state)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewConstructor(t *testing.T) {
	constructor := NewConstructor()
	stage := constructor(nil)
	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}
