package ingestor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
)

func TestM3UHandler_Type(t *testing.T) {
	h := NewM3UHandler()
	if h.Type() != models.SourceTypeM3U {
		t.Errorf("expected type %s, got %s", models.SourceTypeM3U, h.Type())
	}
}

func TestM3UHandler_Validate(t *testing.T) {
	h := NewM3UHandler()

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
				Name: "Test",
				Type: models.SourceTypeXtream,
				URL:  "http://example.com/playlist.m3u",
			},
			wantErr: true,
			errMsg:  "source type must be m3u",
		},
		{
			name: "empty URL",
			source: &models.StreamSource{
				Name: "Test",
				Type: models.SourceTypeM3U,
				URL:  "",
			},
			wantErr: true,
			errMsg:  "URL is required",
		},
		{
			name: "invalid URL scheme",
			source: &models.StreamSource{
				Name: "Test",
				Type: models.SourceTypeM3U,
				URL:  "ftp://example.com/playlist.m3u",
			},
			wantErr: true,
			errMsg:  "HTTP, HTTPS, or file://",
		},
		{
			name: "valid HTTP source",
			source: &models.StreamSource{
				Name: "Test",
				Type: models.SourceTypeM3U,
				URL:  "http://example.com/playlist.m3u",
			},
			wantErr: false,
		},
		{
			name: "valid HTTPS source",
			source: &models.StreamSource{
				Name: "Test",
				Type: models.SourceTypeM3U,
				URL:  "https://example.com/playlist.m3u",
			},
			wantErr: false,
		},
		{
			name: "valid file URL",
			source: &models.StreamSource{
				Name: "Test",
				Type: models.SourceTypeM3U,
				URL:  "file:///path/to/playlist.m3u",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestM3UHandler_Ingest(t *testing.T) {
	m3uContent := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" tvg-name="Channel One" tvg-logo="http://logo.com/1.png" group-title="News",News Channel 1 HD
http://stream.example.com/news1.m3u8
#EXTINF:-1 tvg-id="ch2" tvg-name="Channel Two" group-title="Sports" tvg-chno="42",Sports Channel 2
http://stream.example.com/sports2.m3u8
`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegurl")
		w.Write([]byte(m3uContent))
	}))
	defer server.Close()

	h := NewM3UHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Source",
		Type:      models.SourceTypeM3U,
		URL:       server.URL,
	}

	var channels []*models.Channel
	err := h.Ingest(context.Background(), source, func(ch *models.Channel) error {
		channels = append(channels, ch)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}

	// Verify first channel
	ch1 := channels[0]
	if ch1.SourceID != sourceID {
		t.Errorf("expected SourceID %s, got %s", sourceID, ch1.SourceID)
	}
	if ch1.TvgID != "ch1" {
		t.Errorf("expected TvgID 'ch1', got %q", ch1.TvgID)
	}
	if ch1.TvgName != "Channel One" {
		t.Errorf("expected TvgName 'Channel One', got %q", ch1.TvgName)
	}
	if ch1.TvgLogo != "http://logo.com/1.png" {
		t.Errorf("expected TvgLogo 'http://logo.com/1.png', got %q", ch1.TvgLogo)
	}
	if ch1.GroupTitle != "News" {
		t.Errorf("expected GroupTitle 'News', got %q", ch1.GroupTitle)
	}
	if ch1.ChannelName != "News Channel 1 HD" {
		t.Errorf("expected ChannelName 'News Channel 1 HD', got %q", ch1.ChannelName)
	}
	if ch1.StreamURL != "http://stream.example.com/news1.m3u8" {
		t.Errorf("expected StreamURL, got %q", ch1.StreamURL)
	}
	if ch1.ExtID != "ch1" {
		t.Errorf("expected ExtID 'ch1' (from TvgID), got %q", ch1.ExtID)
	}

	// Verify second channel
	ch2 := channels[1]
	if ch2.ChannelNumber != 42 {
		t.Errorf("expected ChannelNumber 42, got %d", ch2.ChannelNumber)
	}
	if ch2.GroupTitle != "Sports" {
		t.Errorf("expected GroupTitle 'Sports', got %q", ch2.GroupTitle)
	}
}

func TestM3UHandler_Ingest_CallbackError(t *testing.T) {
	m3uContent := `#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1.m3u8
#EXTINF:-1,Channel 2
http://example.com/2.m3u8
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(m3uContent))
	}))
	defer server.Close()

	h := NewM3UHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Source",
		Type:      models.SourceTypeM3U,
		URL:       server.URL,
	}

	expectedErr := errors.New("callback error")
	callCount := 0
	err := h.Ingest(context.Background(), source, func(ch *models.Channel) error {
		callCount++
		if callCount == 1 {
			return expectedErr
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "callback error") {
		t.Errorf("expected callback error, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected callback to be called once, got %d", callCount)
	}
}

func TestM3UHandler_Ingest_ContextCancellation(t *testing.T) {
	m3uContent := `#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1.m3u8
#EXTINF:-1,Channel 2
http://example.com/2.m3u8
#EXTINF:-1,Channel 3
http://example.com/3.m3u8
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(m3uContent))
	}))
	defer server.Close()

	h := NewM3UHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Source",
		Type:      models.SourceTypeM3U,
		URL:       server.URL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	err := h.Ingest(ctx, source, func(ch *models.Channel) error {
		callCount++
		if callCount == 2 {
			cancel()
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

func TestM3UHandler_Ingest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	h := NewM3UHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Source",
		Type:      models.SourceTypeM3U,
		URL:       server.URL,
	}

	err := h.Ingest(context.Background(), source, func(ch *models.Channel) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error containing '404', got: %v", err)
	}
}

func TestM3UHandler_IngestFromReader(t *testing.T) {
	m3uContent := `#EXTM3U
#EXTINF:-1 tvg-id="test",Test Channel
http://example.com/test.m3u8
`

	h := NewM3UHandler()
	sourceID := models.NewULID()
	var channels []*models.Channel

	err := h.IngestFromReader(context.Background(), strings.NewReader(m3uContent), sourceID, func(ch *models.Channel) error {
		channels = append(channels, ch)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}

	if channels[0].SourceID != sourceID {
		t.Errorf("expected SourceID %s, got %s", sourceID, channels[0].SourceID)
	}
	if channels[0].TvgID != "test" {
		t.Errorf("expected TvgID 'test', got %q", channels[0].TvgID)
	}
}

func TestM3UHandler_ChannelNameFallback(t *testing.T) {
	tests := []struct {
		name         string
		m3u          string
		expectedName string
	}{
		{
			name: "title from EXTINF",
			m3u: `#EXTM3U
#EXTINF:-1 tvg-name="TVG Name",EXTINF Title
http://example.com/stream.m3u8
`,
			expectedName: "EXTINF Title",
		},
		{
			name: "fallback to tvg-name",
			m3u: `#EXTM3U
#EXTINF:-1 tvg-name="TVG Name",
http://example.com/stream.m3u8
`,
			expectedName: "TVG Name",
		},
		{
			name: "fallback to URL",
			m3u: `#EXTM3U
http://example.com/my-stream.m3u8
`,
			expectedName: "my-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewM3UHandler()
			sourceID := models.NewULID()
			var channels []*models.Channel

			err := h.IngestFromReader(context.Background(), strings.NewReader(tt.m3u), sourceID, func(ch *models.Channel) error {
				channels = append(channels, ch)
				return nil
			})

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(channels) != 1 {
				t.Fatalf("expected 1 channel, got %d", len(channels))
			}

			if channels[0].ChannelName != tt.expectedName {
				t.Errorf("expected ChannelName %q, got %q", tt.expectedName, channels[0].ChannelName)
			}
		})
	}
}

func TestM3UHandler_ExtIDGeneration(t *testing.T) {
	tests := []struct {
		name       string
		m3u        string
		expectedID string
	}{
		{
			name: "uses tvg-id when present",
			m3u: `#EXTM3U
#EXTINF:-1 tvg-id="unique-id",Channel
http://example.com/stream.m3u8
`,
			expectedID: "unique-id",
		},
		{
			name: "uses URL when no tvg-id",
			m3u: `#EXTM3U
#EXTINF:-1,Channel
http://example.com/stream.m3u8
`,
			expectedID: "http://example.com/stream.m3u8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewM3UHandler()
			sourceID := models.NewULID()
			var channels []*models.Channel

			err := h.IngestFromReader(context.Background(), strings.NewReader(tt.m3u), sourceID, func(ch *models.Channel) error {
				channels = append(channels, ch)
				return nil
			})

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(channels) != 1 {
				t.Fatalf("expected 1 channel, got %d", len(channels))
			}

			if channels[0].ExtID != tt.expectedID {
				t.Errorf("expected ExtID %q, got %q", tt.expectedID, channels[0].ExtID)
			}
		})
	}
}

func TestM3UHandler_ExtraAttributes(t *testing.T) {
	m3uContent := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" custom-attr="custom-value" another="test",Channel
http://example.com/stream.m3u8
`

	h := NewM3UHandler()
	sourceID := models.NewULID()
	var channels []*models.Channel

	err := h.IngestFromReader(context.Background(), strings.NewReader(m3uContent), sourceID, func(ch *models.Channel) error {
		channels = append(channels, ch)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}

	// Extra should contain JSON with custom attributes
	if channels[0].Extra == "" {
		t.Error("expected Extra to contain custom attributes")
	}
	if !strings.Contains(channels[0].Extra, "custom-attr") {
		t.Errorf("expected Extra to contain 'custom-attr', got %q", channels[0].Extra)
	}
}

func TestExtractNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/stream.m3u8", "stream"},
		{"http://example.com/path/to/channel.ts", "channel"},
		{"http://example.com/live?token=abc", "live"},
		{"http://example.com/", "Unknown"},
		{"http://example.com", "example"}, // Returns domain as last path segment
		{"http://example.com/my-cool-stream.m3u8?auth=xyz", "my-cool-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractNameFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("extractNameFromURL(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}
