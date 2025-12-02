package xtream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://example.com:8080", "user", "pass")

	if client.BaseURL != "http://example.com:8080" {
		t.Errorf("expected BaseURL 'http://example.com:8080', got %q", client.BaseURL)
	}
	if client.Username != "user" {
		t.Errorf("expected Username 'user', got %q", client.Username)
	}
	if client.Password != "pass" {
		t.Errorf("expected Password 'pass', got %q", client.Password)
	}
	if client.HTTPClient == nil {
		t.Error("expected HTTPClient to be set")
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	client := NewClient("http://example.com:8080/", "user", "pass")

	if client.BaseURL != "http://example.com:8080" {
		t.Errorf("expected trailing slash to be removed, got %q", client.BaseURL)
	}
}

func TestClient_GetAuthInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("username") != "user" {
			t.Errorf("unexpected username: %s", r.URL.Query().Get("username"))
		}
		if r.URL.Query().Get("password") != "pass" {
			t.Errorf("unexpected password: %s", r.URL.Query().Get("password"))
		}

		response := AuthInfo{
			UserInfo: UserInfo{
				Username:       "user",
				Status:         "Active",
				Auth:           1,
				ExpDate:        FlexInt(time.Now().Add(30 * 24 * time.Hour).Unix()),
				MaxConnections: 1,
			},
			ServerInfo: ServerInfo{
				URL:            "example.com",
				Port:           8080,
				ServerProtocol: "http",
				Timezone:       "UTC",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	info, err := client.GetAuthInfo(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UserInfo.Username != "user" {
		t.Errorf("expected username 'user', got %q", info.UserInfo.Username)
	}
	if !info.UserInfo.IsAuthenticated() {
		t.Error("expected user to be authenticated")
	}
	if info.ServerInfo.Port.Int() != 8080 {
		t.Errorf("expected port 8080, got %d", info.ServerInfo.Port.Int())
	}
}

func TestClient_GetLiveCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "get_live_categories" {
			t.Errorf("unexpected action: %s", r.URL.Query().Get("action"))
		}

		categories := []Category{
			{CategoryID: "1", CategoryName: "News"},
			{CategoryID: "2", CategoryName: "Sports"},
		}
		json.NewEncoder(w).Encode(categories)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	categories, err := client.GetLiveCategories(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].CategoryName != "News" {
		t.Errorf("expected first category 'News', got %q", categories[0].CategoryName)
	}
}

func TestClient_GetLiveStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "get_live_streams" {
			t.Errorf("unexpected action: %s", r.URL.Query().Get("action"))
		}

		streams := []Stream{
			{
				StreamID:     123,
				Name:         "CNN",
				StreamIcon:   "http://example.com/cnn.png",
				EPGChannelID: "CNN.us",
				CategoryID:   "1",
			},
		}
		json.NewEncoder(w).Encode(streams)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	streams, err := client.GetLiveStreams(context.Background(), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(streams) != 1 {
		t.Errorf("expected 1 stream, got %d", len(streams))
	}
	if streams[0].Name != "CNN" {
		t.Errorf("expected stream name 'CNN', got %q", streams[0].Name)
	}
	if streams[0].StreamID.Int() != 123 {
		t.Errorf("expected stream ID 123, got %d", streams[0].StreamID.Int())
	}
}

func TestClient_GetLiveStreams_WithCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("category_id") != "5" {
			t.Errorf("unexpected category_id: %s", r.URL.Query().Get("category_id"))
		}
		json.NewEncoder(w).Encode([]Stream{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	_, err := client.GetLiveStreams(context.Background(), &StreamsOptions{CategoryID: "5"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_GetShortEPG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "get_short_epg" {
			t.Errorf("unexpected action: %s", r.URL.Query().Get("action"))
		}
		if r.URL.Query().Get("stream_id") != "123" {
			t.Errorf("unexpected stream_id: %s", r.URL.Query().Get("stream_id"))
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("unexpected limit: %s", r.URL.Query().Get("limit"))
		}

		response := EPGResponse{
			EPGListings: []EPGListing{
				{
					Title:          "Morning News",
					Description:    "The latest news",
					StartTimestamp: FlexInt(time.Now().Unix()),
					StopTimestamp:  FlexInt(time.Now().Add(time.Hour).Unix()),
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	epg, err := client.GetShortEPG(context.Background(), 123, 10)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(epg) != 1 {
		t.Errorf("expected 1 EPG entry, got %d", len(epg))
	}
	if epg[0].Title != "Morning News" {
		t.Errorf("expected title 'Morning News', got %q", epg[0].Title)
	}
}

func TestClient_StreamURLs(t *testing.T) {
	client := NewClient("http://example.com:8080", "user", "pass")

	tests := []struct {
		name     string
		method   func() string
		expected string
	}{
		{
			name:     "live stream",
			method:   func() string { return client.GetLiveStreamURL(123, "ts") },
			expected: "http://example.com:8080/live/user/pass/123.ts",
		},
		{
			name:     "live stream m3u8",
			method:   func() string { return client.GetLiveStreamURL(123, "m3u8") },
			expected: "http://example.com:8080/live/user/pass/123.m3u8",
		},
		{
			name:     "live stream default ext",
			method:   func() string { return client.GetLiveStreamURL(123, "") },
			expected: "http://example.com:8080/live/user/pass/123.ts",
		},
		{
			name:     "VOD stream",
			method:   func() string { return client.GetVODStreamURL(456, "mp4") },
			expected: "http://example.com:8080/movie/user/pass/456.mp4",
		},
		{
			name:     "series stream",
			method:   func() string { return client.GetSeriesStreamURL(789, "mkv") },
			expected: "http://example.com:8080/series/user/pass/789.mkv",
		},
		{
			name:     "XMLTV URL",
			method:   client.GetXMLTVURL,
			expected: "http://example.com:8080/xmltv.php?username=user&password=pass",
		},
		{
			name:     "M3U playlist",
			method:   func() string { return client.GetM3UPlaylistURL("m3u_plus") },
			expected: "http://example.com:8080/get.php?username=user&password=pass&type=m3u_plus&output=ts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid credentials"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")
	_, err := client.GetAuthInfo(context.Background())

	if err == nil {
		t.Error("expected error for unauthorized response")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(AuthInfo{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetAuthInfo(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
