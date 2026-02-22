package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
)

// mockManualChannelRepo is a mock implementation of ManualChannelRepository
type mockManualChannelRepo struct {
	channels map[models.ULID]*models.ManualStreamChannel
}

func newMockManualChannelRepo() *mockManualChannelRepo {
	return &mockManualChannelRepo{
		channels: make(map[models.ULID]*models.ManualStreamChannel),
	}
}

func (r *mockManualChannelRepo) Create(ctx context.Context, channel *models.ManualStreamChannel) error {
	channel.ID = models.NewULID()
	r.channels[channel.ID] = channel
	return nil
}

func (r *mockManualChannelRepo) GetByID(ctx context.Context, id models.ULID) (*models.ManualStreamChannel, error) {
	ch, exists := r.channels[id]
	if !exists {
		return nil, nil
	}
	return ch, nil
}

func (r *mockManualChannelRepo) GetAll(ctx context.Context) ([]*models.ManualStreamChannel, error) {
	channels := make([]*models.ManualStreamChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	return channels, nil
}

func (r *mockManualChannelRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	channels := make([]*models.ManualStreamChannel, 0)
	for _, ch := range r.channels {
		if ch.SourceID == sourceID {
			channels = append(channels, ch)
		}
	}
	return channels, nil
}

func (r *mockManualChannelRepo) GetEnabledBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	channels := make([]*models.ManualStreamChannel, 0)
	for _, ch := range r.channels {
		if ch.SourceID == sourceID && models.BoolVal(ch.Enabled) {
			channels = append(channels, ch)
		}
	}
	return channels, nil
}

func (r *mockManualChannelRepo) Update(ctx context.Context, channel *models.ManualStreamChannel) error {
	if _, exists := r.channels[channel.ID]; !exists {
		return errors.New("channel not found")
	}
	r.channels[channel.ID] = channel
	return nil
}

func (r *mockManualChannelRepo) Delete(ctx context.Context, id models.ULID) error {
	delete(r.channels, id)
	return nil
}

func (r *mockManualChannelRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	for id, ch := range r.channels {
		if ch.SourceID == sourceID {
			delete(r.channels, id)
		}
	}
	return nil
}

func (r *mockManualChannelRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	for _, ch := range r.channels {
		if ch.SourceID == sourceID {
			count++
		}
	}
	return count, nil
}

// mockSourceRepoForManual is a mock StreamSourceRepository for ManualChannelService tests
type mockSourceRepoForManual struct {
	sources map[models.ULID]*models.StreamSource
}

func newMockSourceRepoForManual() *mockSourceRepoForManual {
	return &mockSourceRepoForManual{
		sources: make(map[models.ULID]*models.StreamSource),
	}
}

func (r *mockSourceRepoForManual) Create(ctx context.Context, source *models.StreamSource) error {
	source.ID = models.NewULID()
	r.sources[source.ID] = source
	return nil
}

func (r *mockSourceRepoForManual) Update(ctx context.Context, source *models.StreamSource) error {
	if _, exists := r.sources[source.ID]; !exists {
		return errors.New("source not found")
	}
	r.sources[source.ID] = source
	return nil
}

func (r *mockSourceRepoForManual) Delete(ctx context.Context, id models.ULID) error {
	delete(r.sources, id)
	return nil
}

func (r *mockSourceRepoForManual) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	source, exists := r.sources[id]
	if !exists {
		return nil, nil
	}
	return source, nil
}

func (r *mockSourceRepoForManual) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	for _, s := range r.sources {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, nil
}

func (r *mockSourceRepoForManual) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	sources := make([]*models.StreamSource, 0, len(r.sources))
	for _, s := range r.sources {
		sources = append(sources, s)
	}
	return sources, nil
}

func (r *mockSourceRepoForManual) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	sources := make([]*models.StreamSource, 0)
	for _, s := range r.sources {
		if models.BoolVal(s.Enabled) {
			sources = append(sources, s)
		}
	}
	return sources, nil
}

func (r *mockSourceRepoForManual) GetByURL(ctx context.Context, url string) (*models.StreamSource, error) {
	for _, s := range r.sources {
		if s.URL == url {
			return s, nil
		}
	}
	return nil, nil
}

func (r *mockSourceRepoForManual) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	return nil
}

func TestManualChannelService_ListBySourceID(t *testing.T) {
	ctx := context.Background()
	channelRepo := newMockManualChannelRepo()
	sourceRepo := newMockSourceRepoForManual()

	// Create a manual source
	manualSource := &models.StreamSource{
		Name:    "Test Manual Source",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	if err := sourceRepo.Create(ctx, manualSource); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create some channels
	ch1 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Test Channel 1",
		StreamURL:   "http://example.com/stream1",
		Enabled:     new(true),
	}
	ch2 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Test Channel 2",
		StreamURL:   "http://example.com/stream2",
		Enabled:     new(true),
	}
	if err := channelRepo.Create(ctx, ch1); err != nil {
		t.Fatalf("failed to create channel 1: %v", err)
	}
	if err := channelRepo.Create(ctx, ch2); err != nil {
		t.Fatalf("failed to create channel 2: %v", err)
	}

	svc := NewManualChannelService(channelRepo, sourceRepo)

	t.Run("list channels for manual source", func(t *testing.T) {
		channels, err := svc.ListBySourceID(ctx, manualSource.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(channels) != 2 {
			t.Errorf("expected 2 channels, got %d", len(channels))
		}
	})

	t.Run("error for non-manual source", func(t *testing.T) {
		m3uSource := &models.StreamSource{
			Name:    "M3U Source",
			Type:    models.SourceTypeM3U,
			URL:     "http://example.com/playlist.m3u",
			Enabled: new(true),
		}
		if err := sourceRepo.Create(ctx, m3uSource); err != nil {
			t.Fatalf("failed to create m3u source: %v", err)
		}

		_, err := svc.ListBySourceID(ctx, m3uSource.ID)
		if err == nil {
			t.Error("expected error for non-manual source")
		}
	})

	t.Run("error for non-existent source", func(t *testing.T) {
		nonExistentID := models.NewULID()
		_, err := svc.ListBySourceID(ctx, nonExistentID)
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestManualChannelService_ReplaceChannels(t *testing.T) {
	ctx := context.Background()
	channelRepo := newMockManualChannelRepo()
	sourceRepo := newMockSourceRepoForManual()

	// Create a manual source
	manualSource := &models.StreamSource{
		Name:    "Test Manual Source",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	if err := sourceRepo.Create(ctx, manualSource); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	svc := NewManualChannelService(channelRepo, sourceRepo)

	t.Run("replace with valid channels", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{
				ChannelName: "New Channel 1",
				StreamURL:   "http://example.com/new1",
				Enabled:     new(true),
			},
			{
				ChannelName: "New Channel 2",
				StreamURL:   "https://example.com/new2",
				Enabled:     new(true),
			},
		}

		result, err := svc.ReplaceChannels(ctx, manualSource.ID, channels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 channels returned, got %d", len(result))
		}
	})

	t.Run("validation error for empty channel_name", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{
				ChannelName: "",
				StreamURL:   "http://example.com/stream",
				Enabled:     new(true),
			},
		}

		_, err := svc.ReplaceChannels(ctx, manualSource.ID, channels)
		if err == nil {
			t.Error("expected validation error for empty channel name")
		}
	})

	t.Run("validation error for invalid stream URL", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{
				ChannelName: "Test Channel",
				StreamURL:   "ftp://example.com/stream",
				Enabled:     new(true),
			},
		}

		_, err := svc.ReplaceChannels(ctx, manualSource.ID, channels)
		if err == nil {
			t.Error("expected validation error for invalid stream URL")
		}
	})

	t.Run("validation error for invalid logo format", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
				TvgLogo:     "invalid-logo",
				Enabled:     new(true),
			},
		}

		_, err := svc.ReplaceChannels(ctx, manualSource.ID, channels)
		if err == nil {
			t.Error("expected validation error for invalid logo format")
		}
	})

	t.Run("error for non-manual source", func(t *testing.T) {
		m3uSource := &models.StreamSource{
			Name:    "M3U Source",
			Type:    models.SourceTypeM3U,
			URL:     "http://example.com/playlist.m3u",
			Enabled: new(true),
		}
		if err := sourceRepo.Create(ctx, m3uSource); err != nil {
			t.Fatalf("failed to create m3u source: %v", err)
		}

		channels := []*models.ManualStreamChannel{
			{
				ChannelName: "Test",
				StreamURL:   "http://example.com/stream",
				Enabled:     new(true),
			},
		}

		_, err := svc.ReplaceChannels(ctx, m3uSource.ID, channels)
		if err == nil {
			t.Error("expected error for non-manual source")
		}
	})
}

func TestManualChannelService_ValidateChannel(t *testing.T) {
	ctx := context.Background()
	channelRepo := newMockManualChannelRepo()
	sourceRepo := newMockSourceRepoForManual()
	svc := NewManualChannelService(channelRepo, sourceRepo)

	testCases := []struct {
		name        string
		channel     *models.ManualStreamChannel
		expectError bool
	}{
		{
			name: "valid channel with http URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
			},
			expectError: false,
		},
		{
			name: "valid channel with https URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "https://example.com/stream",
			},
			expectError: false,
		},
		{
			name: "valid channel with rtsp URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "rtsp://192.168.1.1:554/stream",
			},
			expectError: false,
		},
		{
			name: "valid channel with @logo: token",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
				TvgLogo:     "@logo:mychannel",
			},
			expectError: false,
		},
		{
			name: "valid channel with http logo URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
				TvgLogo:     "https://example.com/logo.png",
			},
			expectError: false,
		},
		{
			name: "valid channel with empty logo",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
				TvgLogo:     "",
			},
			expectError: false,
		},
		{
			name: "invalid - empty channel name",
			channel: &models.ManualStreamChannel{
				ChannelName: "",
				StreamURL:   "http://example.com/stream",
			},
			expectError: true,
		},
		{
			name: "invalid - empty stream URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "",
			},
			expectError: true,
		},
		{
			name: "invalid - ftp URL",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "ftp://example.com/stream",
			},
			expectError: true,
		},
		{
			name: "invalid - malformed logo",
			channel: &models.ManualStreamChannel{
				ChannelName: "Test Channel",
				StreamURL:   "http://example.com/stream",
				TvgLogo:     "not-a-valid-logo-reference",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.ValidateChannel(ctx, tc.channel)
			if tc.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestManualChannelService_DetectDuplicateChannelNumbers(t *testing.T) {
	ctx := context.Background()
	channelRepo := newMockManualChannelRepo()
	sourceRepo := newMockSourceRepoForManual()
	svc := NewManualChannelService(channelRepo, sourceRepo)

	t.Run("no duplicates", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{ChannelName: "Ch1", StreamURL: "http://example.com/1", ChannelNumber: 1},
			{ChannelName: "Ch2", StreamURL: "http://example.com/2", ChannelNumber: 2},
			{ChannelName: "Ch3", StreamURL: "http://example.com/3", ChannelNumber: 3},
		}
		warnings := svc.DetectDuplicateChannelNumbers(ctx, channels)
		if len(warnings) != 0 {
			t.Errorf("expected no warnings, got %d", len(warnings))
		}
	})

	t.Run("with duplicates", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{ChannelName: "Ch1", StreamURL: "http://example.com/1", ChannelNumber: 1},
			{ChannelName: "Ch2", StreamURL: "http://example.com/2", ChannelNumber: 1},
			{ChannelName: "Ch3", StreamURL: "http://example.com/3", ChannelNumber: 2},
		}
		warnings := svc.DetectDuplicateChannelNumbers(ctx, channels)
		if len(warnings) != 1 {
			t.Errorf("expected 1 warning, got %d", len(warnings))
		}
	})

	t.Run("zero channel numbers ignored", func(t *testing.T) {
		channels := []*models.ManualStreamChannel{
			{ChannelName: "Ch1", StreamURL: "http://example.com/1", ChannelNumber: 0},
			{ChannelName: "Ch2", StreamURL: "http://example.com/2", ChannelNumber: 0},
			{ChannelName: "Ch3", StreamURL: "http://example.com/3", ChannelNumber: 0},
		}
		warnings := svc.DetectDuplicateChannelNumbers(ctx, channels)
		if len(warnings) != 0 {
			t.Errorf("expected no warnings for zero channel numbers, got %d", len(warnings))
		}
	})
}
