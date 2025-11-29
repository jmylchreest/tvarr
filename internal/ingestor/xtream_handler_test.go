package ingestor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/xtream"
)

func TestXtreamHandler_Type(t *testing.T) {
	h := NewXtreamHandler()
	if h.Type() != models.SourceTypeXtream {
		t.Errorf("expected type %s, got %s", models.SourceTypeXtream, h.Type())
	}
}

func TestXtreamHandler_Validate(t *testing.T) {
	h := NewXtreamHandler()

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
				Type: models.SourceTypeM3U,
				URL:  "http://example.com",
			},
			wantErr: true,
			errMsg:  "source type must be xtream",
		},
		{
			name: "empty URL",
			source: &models.StreamSource{
				Name:     "Test",
				Type:     models.SourceTypeXtream,
				URL:      "",
				Username: "user",
				Password: "pass",
			},
			wantErr: true,
			errMsg:  "URL is required",
		},
		{
			name: "missing username",
			source: &models.StreamSource{
				Name:     "Test",
				Type:     models.SourceTypeXtream,
				URL:      "http://example.com",
				Username: "",
				Password: "pass",
			},
			wantErr: true,
			errMsg:  "username is required",
		},
		{
			name: "missing password",
			source: &models.StreamSource{
				Name:     "Test",
				Type:     models.SourceTypeXtream,
				URL:      "http://example.com",
				Username: "user",
				Password: "",
			},
			wantErr: true,
			errMsg:  "password is required",
		},
		{
			name: "valid source",
			source: &models.StreamSource{
				Name:     "Test",
				Type:     models.SourceTypeXtream,
				URL:      "http://example.com",
				Username: "user",
				Password: "pass",
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

func TestXtreamHandler_Ingest(t *testing.T) {
	// Mock Xtream API responses
	liveCategories := []xtream.Category{
		{CategoryID: xtream.FlexString("1"), CategoryName: "News"},
		{CategoryID: xtream.FlexString("2"), CategoryName: "Sports"},
	}

	liveStreams := []xtream.Stream{
		{
			StreamID:     xtream.FlexInt(101),
			Name:         "News Channel 1",
			StreamIcon:   "http://logo.com/news1.png",
			EPGChannelID: "news1.epg",
			CategoryID:   xtream.FlexString("1"),
			Num:          xtream.FlexInt(1),
		},
		{
			StreamID:     xtream.FlexInt(102),
			Name:         "Sports Channel 1",
			StreamIcon:   "http://logo.com/sports1.png",
			EPGChannelID: "sports1.epg",
			CategoryID:   xtream.FlexString("2"),
			Num:          xtream.FlexInt(42),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication
		username := r.URL.Query().Get("username")
		password := r.URL.Query().Get("password")
		if username != "testuser" || password != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		action := r.URL.Query().Get("action")
		switch action {
		case "get_live_categories":
			json.NewEncoder(w).Encode(liveCategories)
		case "get_live_streams":
			json.NewEncoder(w).Encode(liveStreams)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	h := NewXtreamHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Xtream",
		Type:      models.SourceTypeXtream,
		URL:       server.URL,
		Username:  "testuser",
		Password:  "testpass",
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
	if ch1.ExtID != "101" {
		t.Errorf("expected ExtID '101', got %q", ch1.ExtID)
	}
	if ch1.ChannelName != "News Channel 1" {
		t.Errorf("expected ChannelName 'News Channel 1', got %q", ch1.ChannelName)
	}
	if ch1.TvgLogo != "http://logo.com/news1.png" {
		t.Errorf("expected TvgLogo, got %q", ch1.TvgLogo)
	}
	if ch1.TvgID != "news1.epg" {
		t.Errorf("expected TvgID 'news1.epg', got %q", ch1.TvgID)
	}
	if ch1.GroupTitle != "News" {
		t.Errorf("expected GroupTitle 'News', got %q", ch1.GroupTitle)
	}
	if ch1.ChannelNumber != 1 {
		t.Errorf("expected ChannelNumber 1, got %d", ch1.ChannelNumber)
	}

	// Verify second channel
	ch2 := channels[1]
	if ch2.GroupTitle != "Sports" {
		t.Errorf("expected GroupTitle 'Sports', got %q", ch2.GroupTitle)
	}
	if ch2.ChannelNumber != 42 {
		t.Errorf("expected ChannelNumber 42, got %d", ch2.ChannelNumber)
	}
}

func TestXtreamHandler_Ingest_CallbackError(t *testing.T) {
	liveCategories := []xtream.Category{}
	liveStreams := []xtream.Stream{
		{StreamID: xtream.FlexInt(1), Name: "Channel 1", CategoryID: xtream.FlexString("1")},
		{StreamID: xtream.FlexInt(2), Name: "Channel 2", CategoryID: xtream.FlexString("1")},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		switch action {
		case "get_live_categories":
			json.NewEncoder(w).Encode(liveCategories)
		case "get_live_streams":
			json.NewEncoder(w).Encode(liveStreams)
		}
	}))
	defer server.Close()

	h := NewXtreamHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test",
		Type:      models.SourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
	}

	expectedErr := errors.New("callback failed")
	err := h.Ingest(context.Background(), source, func(ch *models.Channel) error {
		return expectedErr
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "callback") {
		t.Errorf("expected callback error, got: %v", err)
	}
}

func TestXtreamHandler_Ingest_ContextCancellation(t *testing.T) {
	liveCategories := []xtream.Category{}
	liveStreams := []xtream.Stream{
		{StreamID: xtream.FlexInt(1), Name: "Channel 1"},
		{StreamID: xtream.FlexInt(2), Name: "Channel 2"},
		{StreamID: xtream.FlexInt(3), Name: "Channel 3"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		switch action {
		case "get_live_categories":
			json.NewEncoder(w).Encode(liveCategories)
		case "get_live_streams":
			json.NewEncoder(w).Encode(liveStreams)
		}
	}))
	defer server.Close()

	h := NewXtreamHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test",
		Type:      models.SourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
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
}

func TestXtreamHandler_Ingest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	h := NewXtreamHandler()
	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test",
		Type:      models.SourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
	}

	err := h.Ingest(context.Background(), source, func(ch *models.Channel) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
