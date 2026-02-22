package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/pkg/m3u"
)

// mockManualChannelService is a mock implementation of ManualChannelServiceInterface
type mockManualChannelService struct {
	channels map[models.ULID][]*models.ManualStreamChannel
	sources  map[models.ULID]*models.StreamSource
}

func newMockManualChannelService() *mockManualChannelService {
	return &mockManualChannelService{
		channels: make(map[models.ULID][]*models.ManualStreamChannel),
		sources:  make(map[models.ULID]*models.StreamSource),
	}
}

func (s *mockManualChannelService) AddSource(source *models.StreamSource) {
	source.ID = models.NewULID()
	s.sources[source.ID] = source
	s.channels[source.ID] = []*models.ManualStreamChannel{}
}

func (s *mockManualChannelService) ListBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	source, exists := s.sources[sourceID]
	if !exists {
		return nil, errors.New("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, errors.New("operation only valid for manual sources")
	}
	return s.channels[sourceID], nil
}

func (s *mockManualChannelService) ReplaceChannels(ctx context.Context, sourceID models.ULID, channels []*models.ManualStreamChannel) ([]*models.ManualStreamChannel, error) {
	source, exists := s.sources[sourceID]
	if !exists {
		return nil, errors.New("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, errors.New("operation only valid for manual sources")
	}

	// Assign IDs to channels
	result := make([]*models.ManualStreamChannel, len(channels))
	for i, ch := range channels {
		ch.ID = models.NewULID()
		ch.SourceID = sourceID
		result[i] = ch
	}
	s.channels[sourceID] = result
	return result, nil
}

func (s *mockManualChannelService) ValidateChannel(ctx context.Context, channel *models.ManualStreamChannel) error {
	if channel.ChannelName == "" {
		return errors.New("channel name is required")
	}
	if channel.StreamURL == "" {
		return errors.New("stream URL is required")
	}
	return nil
}

func (s *mockManualChannelService) DetectDuplicateChannelNumbers(ctx context.Context, channels []*models.ManualStreamChannel) []string {
	return nil
}

func (s *mockManualChannelService) ParseM3U(ctx context.Context, sourceID models.ULID, m3uContent string) (*service.M3UImportResult, error) {
	source, exists := s.sources[sourceID]
	if !exists {
		return nil, errors.New("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, errors.New("operation only valid for manual sources")
	}

	result := &service.M3UImportResult{
		Channels: make([]*models.ManualStreamChannel, 0),
		Errors:   make([]string, 0),
	}

	parser := &m3u.Parser{
		OnEntry: func(entry *m3u.Entry) error {
			ch := &models.ManualStreamChannel{
				SourceID:      sourceID,
				TvgID:         entry.TvgID,
				TvgName:       entry.TvgName,
				TvgLogo:       entry.TvgLogo,
				GroupTitle:    entry.GroupTitle,
				ChannelName:   entry.Title,
				ChannelNumber: entry.ChannelNumber,
				StreamURL:     entry.URL,
				Enabled:       new(true),
			}
			result.Channels = append(result.Channels, ch)
			result.ParsedCount++
			return nil
		},
	}

	if err := parser.Parse(strings.NewReader(m3uContent)); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *mockManualChannelService) ImportM3U(ctx context.Context, sourceID models.ULID, m3uContent string, apply bool) (*service.M3UImportResult, error) {
	result, err := s.ParseM3U(ctx, sourceID, m3uContent)
	if err != nil {
		return nil, err
	}

	if !apply {
		return result, nil
	}

	if len(result.Channels) == 0 {
		return nil, errors.New("no valid channels to import")
	}

	replaced, err := s.ReplaceChannels(ctx, sourceID, result.Channels)
	if err != nil {
		return nil, err
	}

	result.Channels = replaced
	result.Applied = true
	return result, nil
}

func (s *mockManualChannelService) ExportM3U(ctx context.Context, sourceID models.ULID) (string, error) {
	channels, err := s.ListBySourceID(ctx, sourceID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	for _, ch := range channels {
		sb.WriteString("#EXTINF:-1")
		if ch.TvgID != "" {
			sb.WriteString(` tvg-id="` + ch.TvgID + `"`)
		}
		if ch.TvgName != "" {
			sb.WriteString(` tvg-name="` + ch.TvgName + `"`)
		}
		if ch.TvgLogo != "" {
			sb.WriteString(` tvg-logo="` + ch.TvgLogo + `"`)
		}
		if ch.GroupTitle != "" {
			sb.WriteString(` group-title="` + ch.GroupTitle + `"`)
		}
		sb.WriteString("," + ch.ChannelName + "\n")
		sb.WriteString(ch.StreamURL + "\n")
	}

	return sb.String(), nil
}

func TestManualChannelHandler_List(t *testing.T) {
	ctx := context.Background()
	svc := newMockManualChannelService()

	// Add a manual source
	manualSource := &models.StreamSource{
		Name:    "Test Manual",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	svc.AddSource(manualSource)

	// Add some channels
	ch1 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Channel 1",
		StreamURL:   "http://example.com/1",
		Enabled:     new(true),
	}
	ch1.ID = models.NewULID()
	ch2 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Channel 2",
		StreamURL:   "http://example.com/2",
		Enabled:     new(true),
	}
	ch2.ID = models.NewULID()
	svc.channels[manualSource.ID] = []*models.ManualStreamChannel{ch1, ch2}

	handler := NewManualChannelHandler(svc)

	t.Run("list channels for valid manual source", func(t *testing.T) {
		input := &ListManualChannelsInput{
			SourceID: manualSource.ID.String(),
		}

		output, err := handler.List(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if len(output.Body.Items) != 2 {
			t.Errorf("expected 2 channels, got %d", len(output.Body.Items))
		}

		if output.Body.Total != 2 {
			t.Errorf("expected total of 2, got %d", output.Body.Total)
		}
	})

	t.Run("error for invalid source ID format", func(t *testing.T) {
		input := &ListManualChannelsInput{
			SourceID: "invalid-id",
		}

		_, err := handler.List(ctx, input)
		if err == nil {
			t.Error("expected error for invalid source ID")
		}
	})

	t.Run("error for non-existent source", func(t *testing.T) {
		input := &ListManualChannelsInput{
			SourceID: models.NewULID().String(),
		}

		_, err := handler.List(ctx, input)
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestManualChannelHandler_Replace(t *testing.T) {
	ctx := context.Background()
	svc := newMockManualChannelService()

	// Add a manual source
	manualSource := &models.StreamSource{
		Name:    "Test Manual",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	svc.AddSource(manualSource)

	handler := NewManualChannelHandler(svc)

	t.Run("replace channels successfully", func(t *testing.T) {
		input := &ReplaceManualChannelsInput{
			SourceID: manualSource.ID.String(),
			Body: ReplaceManualChannelsRequest{
				Channels: []ManualChannelInput{
					{
						ChannelName: "New Channel 1",
						StreamURL:   "http://example.com/new1",
					},
					{
						ChannelName: "New Channel 2",
						StreamURL:   "http://example.com/new2",
					},
				},
			},
		}

		output, err := handler.Replace(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if len(output.Body.Items) != 2 {
			t.Errorf("expected 2 channels returned, got %d", len(output.Body.Items))
		}
	})

	t.Run("error for invalid source ID", func(t *testing.T) {
		input := &ReplaceManualChannelsInput{
			SourceID: "invalid-id",
			Body: ReplaceManualChannelsRequest{
				Channels: []ManualChannelInput{
					{
						ChannelName: "Test",
						StreamURL:   "http://example.com/test",
					},
				},
			},
		}

		_, err := handler.Replace(ctx, input)
		if err == nil {
			t.Error("expected error for invalid source ID")
		}
	})

	t.Run("error for non-manual source", func(t *testing.T) {
		m3uSource := &models.StreamSource{
			Name:    "M3U Source",
			Type:    models.SourceTypeM3U,
			URL:     "http://example.com/playlist.m3u",
			Enabled: new(true),
		}
		svc.AddSource(m3uSource)

		input := &ReplaceManualChannelsInput{
			SourceID: m3uSource.ID.String(),
			Body: ReplaceManualChannelsRequest{
				Channels: []ManualChannelInput{
					{
						ChannelName: "Test",
						StreamURL:   "http://example.com/test",
					},
				},
			},
		}

		_, err := handler.Replace(ctx, input)
		if err == nil {
			t.Error("expected error for non-manual source")
		}
	})
}

func TestManualChannelHandler_ImportM3U(t *testing.T) {
	ctx := context.Background()
	svc := newMockManualChannelService()

	// Add a manual source
	manualSource := &models.StreamSource{
		Name:    "Test Manual",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	svc.AddSource(manualSource)

	handler := NewManualChannelHandler(svc)

	validM3U := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" tvg-name="Channel One" group-title="Group A",Channel 1
http://example.com/stream1.m3u8
#EXTINF:-1 tvg-id="ch2" tvg-name="Channel Two" group-title="Group B",Channel 2
http://example.com/stream2.m3u8
`

	t.Run("preview M3U without applying", func(t *testing.T) {
		input := &ImportM3UInput{
			SourceID: manualSource.ID.String(),
			Apply:    false,
			RawBody:  []byte(validM3U),
		}

		output, err := handler.ImportM3U(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if output.Body.ParsedCount != 2 {
			t.Errorf("expected 2 parsed channels, got %d", output.Body.ParsedCount)
		}

		if output.Body.Applied {
			t.Error("expected Applied to be false for preview mode")
		}

		if len(output.Body.Channels) != 2 {
			t.Errorf("expected 2 channels in response, got %d", len(output.Body.Channels))
		}
	})

	t.Run("apply M3U import", func(t *testing.T) {
		input := &ImportM3UInput{
			SourceID: manualSource.ID.String(),
			Apply:    true,
			RawBody:  []byte(validM3U),
		}

		output, err := handler.ImportM3U(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if !output.Body.Applied {
			t.Error("expected Applied to be true after applying")
		}

		if len(output.Body.Channels) != 2 {
			t.Errorf("expected 2 channels after apply, got %d", len(output.Body.Channels))
		}

		// Verify channels were persisted
		channels := svc.channels[manualSource.ID]
		if len(channels) != 2 {
			t.Errorf("expected 2 channels in storage, got %d", len(channels))
		}
	})

	t.Run("error for empty M3U content", func(t *testing.T) {
		input := &ImportM3UInput{
			SourceID: manualSource.ID.String(),
			Apply:    false,
			RawBody:  []byte("   "),
		}

		_, err := handler.ImportM3U(ctx, input)
		if err == nil {
			t.Error("expected error for empty M3U content")
		}
	})

	t.Run("error for invalid source ID", func(t *testing.T) {
		input := &ImportM3UInput{
			SourceID: "invalid-id",
			Apply:    false,
			RawBody:  []byte(validM3U),
		}

		_, err := handler.ImportM3U(ctx, input)
		if err == nil {
			t.Error("expected error for invalid source ID")
		}
	})

	t.Run("error for non-existent source", func(t *testing.T) {
		input := &ImportM3UInput{
			SourceID: models.NewULID().String(),
			Apply:    false,
			RawBody:  []byte(validM3U),
		}

		_, err := handler.ImportM3U(ctx, input)
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestManualChannelHandler_ExportM3U(t *testing.T) {
	ctx := context.Background()
	svc := newMockManualChannelService()

	// Add a manual source with channels
	manualSource := &models.StreamSource{
		Name:    "Test Manual",
		Type:    models.SourceTypeManual,
		Enabled: new(true),
	}
	svc.AddSource(manualSource)

	// Add some channels
	ch1 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Channel 1",
		StreamURL:   "http://example.com/1",
		TvgID:       "ch1",
		TvgName:     "Channel One",
		GroupTitle:  "Group A",
		Enabled:     new(true),
	}
	ch1.ID = models.NewULID()
	ch2 := &models.ManualStreamChannel{
		SourceID:    manualSource.ID,
		ChannelName: "Channel 2",
		StreamURL:   "http://example.com/2",
		TvgID:       "ch2",
		TvgName:     "Channel Two",
		GroupTitle:  "Group B",
		Enabled:     new(true),
	}
	ch2.ID = models.NewULID()
	svc.channels[manualSource.ID] = []*models.ManualStreamChannel{ch1, ch2}

	handler := NewManualChannelHandler(svc)

	t.Run("export channels as M3U", func(t *testing.T) {
		input := &ExportM3UInput{
			SourceID: manualSource.ID.String(),
		}

		output, err := handler.ExportM3U(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if output.ContentType != "audio/x-mpegurl" {
			t.Errorf("expected content type 'audio/x-mpegurl', got '%s'", output.ContentType)
		}

		// Read the body to verify content
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, output.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		m3uContent := buf.String()
		if !strings.HasPrefix(m3uContent, "#EXTM3U") {
			t.Error("exported M3U should start with #EXTM3U header")
		}

		if !strings.Contains(m3uContent, "Channel 1") {
			t.Error("exported M3U should contain Channel 1")
		}

		if !strings.Contains(m3uContent, "http://example.com/1") {
			t.Error("exported M3U should contain stream URL")
		}
	})

	t.Run("error for invalid source ID", func(t *testing.T) {
		input := &ExportM3UInput{
			SourceID: "invalid-id",
		}

		_, err := handler.ExportM3U(ctx, input)
		if err == nil {
			t.Error("expected error for invalid source ID")
		}
	})

	t.Run("error for non-existent source", func(t *testing.T) {
		input := &ExportM3UInput{
			SourceID: models.NewULID().String(),
		}

		_, err := handler.ExportM3U(ctx, input)
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}
