package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannel_TableName(t *testing.T) {
	c := Channel{}
	assert.Equal(t, "channels", c.TableName())
}

func TestChannel_GetSourceID(t *testing.T) {
	sourceID := NewULID()
	c := Channel{SourceID: sourceID}
	assert.Equal(t, sourceID, c.GetSourceID())
}

func TestChannel_Validate(t *testing.T) {
	sourceID := NewULID()

	tests := []struct {
		name    string
		channel Channel
		wantErr error
	}{
		{
			name: "valid channel",
			channel: Channel{
				SourceID:    sourceID,
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
			},
			wantErr: nil,
		},
		{
			name: "missing source ID",
			channel: Channel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
			},
			wantErr: ErrSourceIDRequired,
		},
		{
			name: "missing channel name",
			channel: Channel{
				SourceID:  sourceID,
				StreamURL: "http://example.com/stream",
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "missing stream URL",
			channel: Channel{
				SourceID:    sourceID,
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

func TestChannel_GenerateExtID(t *testing.T) {
	tests := []struct {
		name     string
		channel  Channel
		expected string
	}{
		{
			name: "uses existing ExtID",
			channel: Channel{
				ExtID: "existing-id",
				TvgID: "tvg-id",
			},
			expected: "existing-id",
		},
		{
			name: "uses TvgID when ExtID is empty",
			channel: Channel{
				TvgID:     "tvg-id",
				StreamURL: "http://example.com/stream",
			},
			expected: "tvg-id",
		},
		{
			name: "hashes StreamURL when both empty",
			channel: Channel{
				StreamURL: "http://example.com/stream",
			},
			expected: hashString("http://example.com/stream"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.channel.GenerateExtID()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChannel_FullModel(t *testing.T) {
	// Test creating a fully populated channel
	id := NewULID()
	sourceID := NewULID()

	c := Channel{
		BaseModel:     BaseModel{ID: id},
		SourceID:      sourceID,
		ExtID:         "ext-123",
		TvgID:         "tvg-456",
		TvgName:       "Test Channel Name",
		TvgLogo:       "http://example.com/logo.png",
		GroupTitle:    "Sports",
		ChannelName:   "ESPN",
		ChannelNumber: 100,
		StreamURL:     "http://stream.example.com/live/espn",
		StreamType:    "live",
		Language:      "en",
		Country:       "US",
		IsAdult:       false,
		Extra:         `{"quality": "HD"}`,
	}

	assert.Equal(t, id, c.GetID())
	assert.Equal(t, sourceID, c.GetSourceID())
	assert.NoError(t, c.Validate())
}
