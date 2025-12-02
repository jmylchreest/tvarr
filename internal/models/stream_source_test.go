package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamSource_TableName(t *testing.T) {
	s := StreamSource{}
	assert.Equal(t, "stream_sources", s.TableName())
}

func TestStreamSource_GetID(t *testing.T) {
	id := NewULID()
	s := StreamSource{BaseModel: BaseModel{ID: id}}
	assert.Equal(t, id, s.GetID())
}

func TestStreamSource_IsM3U(t *testing.T) {
	tests := []struct {
		name     string
		source   StreamSource
		expected bool
	}{
		{"M3U source", StreamSource{Type: SourceTypeM3U}, true},
		{"Xtream source", StreamSource{Type: SourceTypeXtream}, false},
		{"Empty type", StreamSource{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.source.IsM3U())
		})
	}
}

func TestStreamSource_IsXtream(t *testing.T) {
	tests := []struct {
		name     string
		source   StreamSource
		expected bool
	}{
		{"Xtream source", StreamSource{Type: SourceTypeXtream}, true},
		{"M3U source", StreamSource{Type: SourceTypeM3U}, false},
		{"Empty type", StreamSource{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.source.IsXtream())
		})
	}
}

func TestStreamSource_MarkIngesting(t *testing.T) {
	s := StreamSource{
		Status:    SourceStatusSuccess,
		LastError: "previous error",
	}

	s.MarkIngesting()

	assert.Equal(t, SourceStatusIngesting, s.Status)
	assert.Empty(t, s.LastError)
}

func TestStreamSource_MarkSuccess(t *testing.T) {
	s := StreamSource{
		Status:    SourceStatusIngesting,
		LastError: "previous error",
	}

	s.MarkSuccess(100)

	assert.Equal(t, SourceStatusSuccess, s.Status)
	assert.Equal(t, 100, s.ChannelCount)
	assert.NotNil(t, s.LastIngestionAt)
	assert.Empty(t, s.LastError)

	// Verify time is recent
	assert.WithinDuration(t, time.Now(), *s.LastIngestionAt, time.Second)
}

func TestStreamSource_MarkFailed(t *testing.T) {
	s := StreamSource{
		Status: SourceStatusIngesting,
	}

	testErr := assert.AnError
	s.MarkFailed(testErr)

	assert.Equal(t, SourceStatusFailed, s.Status)
	assert.Equal(t, testErr.Error(), s.LastError)
}

func TestStreamSource_MarkFailed_NilError(t *testing.T) {
	s := StreamSource{
		Status: SourceStatusIngesting,
	}

	s.MarkFailed(nil)

	assert.Equal(t, SourceStatusFailed, s.Status)
	assert.Empty(t, s.LastError)
}

func TestStreamSource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		source  StreamSource
		wantErr error
	}{
		{
			name: "valid M3U source",
			source: StreamSource{
				Name: "Test M3U",
				Type: SourceTypeM3U,
				URL:  "http://example.com/playlist.m3u",
			},
			wantErr: nil,
		},
		{
			name: "valid Xtream source",
			source: StreamSource{
				Name:     "Test Xtream",
				Type:     SourceTypeXtream,
				URL:      "http://xtream.example.com",
				Username: "user",
				Password: "pass",
			},
			wantErr: nil,
		},
		{
			name: "missing name",
			source: StreamSource{
				Type: SourceTypeM3U,
				URL:  "http://example.com/playlist.m3u",
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "missing URL",
			source: StreamSource{
				Name: "Test",
				Type: SourceTypeM3U,
			},
			wantErr: ErrURLRequired,
		},
		{
			name: "invalid type",
			source: StreamSource{
				Name: "Test",
				Type: "invalid",
				URL:  "http://example.com",
			},
			wantErr: ErrInvalidSourceType,
		},
		{
			name: "Xtream missing credentials",
			source: StreamSource{
				Name: "Test Xtream",
				Type: SourceTypeXtream,
				URL:  "http://xtream.example.com",
			},
			wantErr: ErrXtreamCredentialsRequired,
		},
		{
			name: "Xtream missing password",
			source: StreamSource{
				Name:     "Test Xtream",
				Type:     SourceTypeXtream,
				URL:      "http://xtream.example.com",
				Username: "user",
			},
			wantErr: ErrXtreamCredentialsRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourceType_Constants(t *testing.T) {
	assert.Equal(t, SourceType("m3u"), SourceTypeM3U)
	assert.Equal(t, SourceType("xtream"), SourceTypeXtream)
}

func TestSourceStatus_Constants(t *testing.T) {
	assert.Equal(t, SourceStatus("pending"), SourceStatusPending)
	assert.Equal(t, SourceStatus("ingesting"), SourceStatusIngesting)
	assert.Equal(t, SourceStatus("success"), SourceStatusSuccess)
	assert.Equal(t, SourceStatus("failed"), SourceStatusFailed)
}
