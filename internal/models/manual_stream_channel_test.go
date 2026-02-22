package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualStreamChannel_TableName(t *testing.T) {
	c := ManualStreamChannel{}
	assert.Equal(t, "manual_stream_channels", c.TableName())
}

func TestManualStreamChannel_GetID(t *testing.T) {
	id := NewULID()
	c := ManualStreamChannel{BaseModel: BaseModel{ID: id}}
	assert.Equal(t, id, c.GetID())
}

func TestManualStreamChannel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		channel ManualStreamChannel
		wantErr error
	}{
		{
			name: "valid manual channel",
			channel: ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
			},
			wantErr: nil,
		},
		{
			name: "missing channel name",
			channel: ManualStreamChannel{
				StreamURL: "http://example.com/stream",
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "missing stream URL",
			channel: ManualStreamChannel{
				ChannelName: "Test Channel",
			},
			wantErr: ErrStreamURLRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.channel.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManualStreamChannel_ToChannel(t *testing.T) {
	id := NewULID()
	mc := ManualStreamChannel{
		BaseModel:     BaseModel{ID: id},
		TvgID:         "tvg-123",
		TvgName:       "Test Name",
		TvgLogo:       "http://example.com/logo.png",
		GroupTitle:    "Movies",
		ChannelName:   "Movie Channel",
		ChannelNumber: 50,
		StreamURL:     "http://stream.example.com/movie",
		StreamType:    "movie",
		Language:      "en",
		Country:       "US",
		IsAdult:       true,
		Extra:         `{"rating": "R"}`,
	}

	c := mc.ToChannel()

	// Verify all fields are copied correctly
	assert.Equal(t, mc.TvgID, c.TvgID)
	assert.Equal(t, mc.TvgName, c.TvgName)
	assert.Equal(t, mc.TvgLogo, c.TvgLogo)
	assert.Equal(t, mc.GroupTitle, c.GroupTitle)
	assert.Equal(t, mc.ChannelName, c.ChannelName)
	assert.Equal(t, mc.ChannelNumber, c.ChannelNumber)
	assert.Equal(t, mc.StreamURL, c.StreamURL)
	assert.Equal(t, mc.StreamType, c.StreamType)
	assert.Equal(t, mc.Language, c.Language)
	assert.Equal(t, mc.Country, c.Country)
	assert.Equal(t, mc.IsAdult, c.IsAdult)
	assert.Equal(t, mc.Extra, c.Extra)

	// Note: ID and SourceID are not copied (different entity)
	assert.True(t, c.ID.IsZero())
	assert.True(t, c.SourceID.IsZero())
}

func TestManualStreamChannel_FullModel(t *testing.T) {
	id := NewULID()
	mc := ManualStreamChannel{
		BaseModel:     BaseModel{ID: id},
		TvgID:         "tvg-123",
		TvgName:       "Test Name",
		TvgLogo:       "http://example.com/logo.png",
		GroupTitle:    "Movies",
		ChannelName:   "Movie Channel",
		ChannelNumber: 50,
		StreamURL:     "http://stream.example.com/movie",
		StreamType:    "movie",
		Language:      "en",
		Country:       "US",
		IsAdult:       true,
		Enabled:       new(true),
		Priority:      10,
		Extra:         `{"rating": "R"}`,
	}

	assert.Equal(t, id, mc.GetID())
	assert.NoError(t, mc.Validate())
}
