package ingestor

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockManualChannelRepository is a mock implementation for testing.
type MockManualChannelRepository struct {
	channels           []*models.ManualStreamChannel
	enabledChannels    []*models.ManualStreamChannel
	getEnabledErr      error
	getBySourceIDCalls int
}

func (m *MockManualChannelRepository) Create(ctx context.Context, channel *models.ManualStreamChannel) error {
	return nil
}

func (m *MockManualChannelRepository) GetByID(ctx context.Context, id models.ULID) (*models.ManualStreamChannel, error) {
	return nil, nil
}

func (m *MockManualChannelRepository) GetAll(ctx context.Context) ([]*models.ManualStreamChannel, error) {
	return m.channels, nil
}

func (m *MockManualChannelRepository) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	m.getBySourceIDCalls++
	return m.channels, nil
}

func (m *MockManualChannelRepository) GetEnabledBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	m.getBySourceIDCalls++
	if m.getEnabledErr != nil {
		return nil, m.getEnabledErr
	}
	if m.enabledChannels != nil {
		return m.enabledChannels, nil
	}
	// Filter enabled from all channels
	var enabled []*models.ManualStreamChannel
	for _, ch := range m.channels {
		if ch.Enabled {
			enabled = append(enabled, ch)
		}
	}
	return enabled, nil
}

func (m *MockManualChannelRepository) Update(ctx context.Context, channel *models.ManualStreamChannel) error {
	return nil
}

func (m *MockManualChannelRepository) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *MockManualChannelRepository) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	return nil
}

func (m *MockManualChannelRepository) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	for _, ch := range m.channels {
		if ch.SourceID == sourceID {
			count++
		}
	}
	return count, nil
}

func TestManualHandler_Type(t *testing.T) {
	handler := NewManualHandler(nil)
	assert.Equal(t, models.SourceTypeManual, handler.Type())
}

func TestManualHandler_Validate(t *testing.T) {
	handler := NewManualHandler(nil)

	tests := []struct {
		name    string
		source  *models.StreamSource
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil source",
			source:  nil,
			wantErr: true,
			errMsg:  "source is nil",
		},
		{
			name: "wrong source type",
			source: &models.StreamSource{
				Type: models.SourceTypeM3U,
			},
			wantErr: true,
			errMsg:  "source type must be manual",
		},
		{
			name: "valid manual source without URL",
			source: &models.StreamSource{
				Type: models.SourceTypeManual,
				Name: "Manual Source",
			},
			wantErr: false,
		},
		{
			name: "valid manual source with URL",
			source: &models.StreamSource{
				Type: models.SourceTypeManual,
				Name: "Manual Source",
				URL:  "http://example.com", // Optional for manual
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.source)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestManualHandler_Ingest(t *testing.T) {
	sourceID := models.NewULID()
	ch1ID := models.NewULID()
	ch2ID := models.NewULID()
	ch3ID := models.NewULID()

	t.Run("successfully materializes enabled channels", func(t *testing.T) {
		mockRepo := &MockManualChannelRepository{
			channels: []*models.ManualStreamChannel{
				{
					BaseModel:   models.BaseModel{ID: ch1ID},
					SourceID:    sourceID,
					ChannelName: "Channel 1",
					StreamURL:   "http://stream1.com/live",
					GroupTitle:  "Movies",
					Enabled:     true,
				},
				{
					BaseModel:   models.BaseModel{ID: ch2ID},
					SourceID:    sourceID,
					ChannelName: "Channel 2",
					StreamURL:   "http://stream2.com/live",
					GroupTitle:  "Sports",
					Enabled:     true,
				},
				{
					BaseModel:   models.BaseModel{ID: ch3ID},
					SourceID:    sourceID,
					ChannelName: "Disabled Channel",
					StreamURL:   "http://stream3.com/live",
					Enabled:     false, // Should be filtered out
				},
			},
		}

		handler := NewManualHandler(mockRepo)
		source := &models.StreamSource{
			BaseModel: models.BaseModel{ID: sourceID},
			Type:      models.SourceTypeManual,
			Name:      "Test Manual Source",
		}

		var channels []*models.Channel
		err := handler.Ingest(context.Background(), source, func(ch *models.Channel) error {
			channels = append(channels, ch)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, channels, 2) // Only enabled channels
		assert.Equal(t, "Channel 1", channels[0].ChannelName)
		assert.Equal(t, "Channel 2", channels[1].ChannelName)
		assert.Equal(t, sourceID, channels[0].SourceID)
		assert.Equal(t, ch1ID.String(), channels[0].ExtID)
	})

	t.Run("handles empty channel list", func(t *testing.T) {
		mockRepo := &MockManualChannelRepository{
			channels: []*models.ManualStreamChannel{},
		}

		handler := NewManualHandler(mockRepo)
		source := &models.StreamSource{
			BaseModel: models.BaseModel{ID: sourceID},
			Type:      models.SourceTypeManual,
			Name:      "Empty Manual Source",
		}

		var channels []*models.Channel
		err := handler.Ingest(context.Background(), source, func(ch *models.Channel) error {
			channels = append(channels, ch)
			return nil
		})

		require.NoError(t, err)
		assert.Empty(t, channels)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		mockRepo := &MockManualChannelRepository{
			channels: []*models.ManualStreamChannel{
				{
					BaseModel:   models.BaseModel{ID: ch1ID},
					SourceID:    sourceID,
					ChannelName: "Channel 1",
					StreamURL:   "http://stream1.com/live",
					Enabled:     true,
				},
			},
		}

		handler := NewManualHandler(mockRepo)
		source := &models.StreamSource{
			BaseModel: models.BaseModel{ID: sourceID},
			Type:      models.SourceTypeManual,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel before starting

		err := handler.Ingest(ctx, source, func(ch *models.Channel) error {
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when repository not configured", func(t *testing.T) {
		handler := NewManualHandler(nil)
		source := &models.StreamSource{
			BaseModel: models.BaseModel{ID: sourceID},
			Type:      models.SourceTypeManual,
		}

		err := handler.Ingest(context.Background(), source, func(ch *models.Channel) error {
			return nil
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "manual channel repository not configured")
	})
}

func TestManualHandler_ChannelConversion(t *testing.T) {
	sourceID := models.NewULID()
	channelID := models.NewULID()

	mockRepo := &MockManualChannelRepository{
		channels: []*models.ManualStreamChannel{
			{
				BaseModel:     models.BaseModel{ID: channelID},
				SourceID:      sourceID,
				TvgID:         "tvg123",
				TvgName:       "TVG Name",
				TvgLogo:       "http://logo.com/logo.png",
				GroupTitle:    "Entertainment",
				ChannelName:   "Test Channel",
				ChannelNumber: 42,
				StreamURL:     "http://stream.com/live.m3u8",
				StreamType:    "live",
				Language:      "en",
				Country:       "US",
				IsAdult:       false,
				Enabled:       true,
				Priority:      10,
				Extra:         `{"custom":"value"}`,
			},
		},
	}

	handler := NewManualHandler(mockRepo)
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.SourceTypeManual,
	}

	var channels []*models.Channel
	err := handler.Ingest(context.Background(), source, func(ch *models.Channel) error {
		channels = append(channels, ch)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, channels, 1)

	ch := channels[0]
	assert.Equal(t, sourceID, ch.SourceID)
	assert.Equal(t, channelID.String(), ch.ExtID) // Uses manual channel ID as ExtID
	assert.Equal(t, "tvg123", ch.TvgID)
	assert.Equal(t, "TVG Name", ch.TvgName)
	assert.Equal(t, "http://logo.com/logo.png", ch.TvgLogo)
	assert.Equal(t, "Entertainment", ch.GroupTitle)
	assert.Equal(t, "Test Channel", ch.ChannelName)
	assert.Equal(t, 42, ch.ChannelNumber)
	assert.Equal(t, "http://stream.com/live.m3u8", ch.StreamURL)
	assert.Equal(t, "live", ch.StreamType)
	assert.Equal(t, "en", ch.Language)
	assert.Equal(t, "US", ch.Country)
	assert.False(t, ch.IsAdult)
	assert.Equal(t, `{"custom":"value"}`, ch.Extra)
}
