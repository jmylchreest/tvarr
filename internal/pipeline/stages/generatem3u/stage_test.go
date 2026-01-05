package generatem3u

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *core.State {
	t.Helper()
	tempDir := t.TempDir()
	proxy := &models.StreamProxy{
		BaseModel:             models.BaseModel{ID: models.NewULID()},
		Name:                  "Test Proxy",
		StartingChannelNumber: 1,
	}
	state := core.NewState(proxy)
	state.TempDir = tempDir
	return state
}

// T010-TEST: Test M3U generation produces valid output with #EXTM3U header and #EXTINF entries
func TestStage_Execute_ProducesValidM3U(t *testing.T) {
	t.Run("generates valid M3U with header and entries", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "channel1",
				TvgName:     "Channel One",
				TvgLogo:     "http://example.com/logo1.png",
				GroupTitle:  "News",
				ChannelName: "Channel One HD",
				StreamURL:   "http://example.com/stream1",
			},
			{
				TvgID:       "channel2",
				TvgName:     "Channel Two",
				TvgLogo:     "http://example.com/logo2.png",
				GroupTitle:  "Sports",
				ChannelName: "Channel Two HD",
				StreamURL:   "http://example.com/stream2",
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify result
		assert.Equal(t, 2, result.RecordsProcessed)
		assert.Contains(t, result.Message, "2 channels")

		// Verify file was created
		m3uPath, ok := state.GetMetadata(MetadataKeyTempPath)
		require.True(t, ok, "M3U path should be in metadata")
		pathStr, ok := m3uPath.(string)
		require.True(t, ok, "M3U path should be a string")

		// Read and validate M3U content
		content, err := os.ReadFile(pathStr)
		require.NoError(t, err)

		contentStr := string(content)

		// Verify #EXTM3U header
		assert.True(t, strings.HasPrefix(contentStr, "#EXTM3U"), "M3U should start with #EXTM3U header")

		// Verify #EXTINF entries
		assert.Contains(t, contentStr, "#EXTINF:", "M3U should contain #EXTINF entries")
		assert.Contains(t, contentStr, `tvg-id="channel1"`)
		assert.Contains(t, contentStr, `tvg-name="Channel One"`)
		assert.Contains(t, contentStr, `tvg-logo="http://example.com/logo1.png"`)
		assert.Contains(t, contentStr, `group-title="News"`)
		assert.Contains(t, contentStr, `tvg-chno="1"`)
		assert.Contains(t, contentStr, "Channel One HD")
		assert.Contains(t, contentStr, "http://example.com/stream1")

		// Verify second channel
		assert.Contains(t, contentStr, `tvg-id="channel2"`)
		assert.Contains(t, contentStr, `tvg-chno="2"`)
		assert.Contains(t, contentStr, "http://example.com/stream2")
	})

	t.Run("handles no channels gracefully", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, result.RecordsProcessed)
		assert.Equal(t, "No channels to write", result.Message)
	})

	t.Run("creates artifact with file info", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "test",
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)

		require.Len(t, result.Artifacts, 1)
		artifact := result.Artifacts[0]
		assert.Equal(t, core.ArtifactTypeM3U, artifact.Type)
		assert.Equal(t, 1, artifact.RecordCount)
		assert.Greater(t, artifact.FileSize, int64(0))
	})
}

// T011-TEST: Test skipping channels with empty StreamURL
func TestStage_Execute_SkipsEmptyStreamURL(t *testing.T) {
	t.Run("skips channels with empty StreamURL and logs warning", func(t *testing.T) {
		state := newTestState(t)
		state.Channels = []*models.Channel{
			{
				TvgID:       "valid",
				ChannelName: "Valid Channel",
				StreamURL:   "http://example.com/stream",
			},
			{
				TvgID:       "empty_url",
				ChannelName: "Empty URL Channel",
				StreamURL:   "", // Empty - should be skipped
			},
			{
				TvgID:       "another_valid",
				ChannelName: "Another Valid Channel",
				StreamURL:   "http://example.com/stream2",
			},
		}

		stage := New()
		result, err := stage.Execute(context.Background(), state)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should only process 2 channels (skipping the one with empty URL)
		assert.Equal(t, 2, result.RecordsProcessed)

		// Verify file content doesn't have empty URL channel
		m3uPath, _ := state.GetMetadata(MetadataKeyTempPath)
		content, err := os.ReadFile(m3uPath.(string))
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, `tvg-id="valid"`)
		assert.Contains(t, contentStr, `tvg-id="another_valid"`)
		assert.NotContains(t, contentStr, `tvg-id="empty_url"`)
	})
}

func TestStage_Interface(t *testing.T) {
	stage := New()
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestStage_ContextCancellation(t *testing.T) {
	state := newTestState(t)
	// Create many channels to increase chance of cancellation during iteration
	for range 100 {
		state.Channels = append(state.Channels, &models.Channel{
			TvgID:       "test",
			ChannelName: "Test",
			StreamURL:   "http://example.com/stream",
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
