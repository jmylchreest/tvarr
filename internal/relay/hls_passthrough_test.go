package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// parseURLForTest is a helper function for tests.
func parseURLForTest(s string) (*url.URL, error) {
	return url.Parse(s)
}

func TestNewHLSPassthroughHandler(t *testing.T) {
	handler := NewHLSPassthroughHandler(
		"http://upstream.example.com/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	if handler == nil {
		t.Fatal("NewHLSPassthroughHandler returned nil")
	}
	if handler.upstreamURL != "http://upstream.example.com/stream.m3u8" {
		t.Errorf("unexpected upstream URL: %s", handler.upstreamURL)
	}
	if handler.baseURL != "http://proxy.example.com/stream/123" {
		t.Errorf("unexpected base URL: %s", handler.baseURL)
	}
}

func TestHLSPassthroughHandler_RewritePlaylist(t *testing.T) {
	// Create mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:100
#EXTINF:6.000,
segment0.ts
#EXTINF:6.000,
segment1.ts
#EXTINF:6.000,
segment2.ts
`
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte(playlist))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()
	playlist, err := handler.getRewrittenPlaylist(ctx)
	if err != nil {
		t.Fatalf("getRewrittenPlaylist failed: %v", err)
	}

	// Verify playlist was rewritten
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("playlist missing #EXTM3U")
	}
	if !strings.Contains(playlist, "#EXT-X-TARGETDURATION:6") {
		t.Error("playlist missing #EXT-X-TARGETDURATION:6")
	}
	if !strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:100") {
		t.Error("playlist missing #EXT-X-MEDIA-SEQUENCE:100")
	}

	// Verify segment URLs were rewritten
	if !strings.Contains(playlist, "http://proxy.example.com/stream/123?format=hls&seg=0") {
		t.Errorf("playlist missing rewritten segment URL, got:\n%s", playlist)
	}
	if !strings.Contains(playlist, "http://proxy.example.com/stream/123?format=hls&seg=1") {
		t.Errorf("playlist missing rewritten segment URL for seg=1")
	}

	// Verify media sequence was parsed
	if handler.GetMediaSequence() != 100 {
		t.Errorf("media sequence = %d, want 100", handler.GetMediaSequence())
	}
	if handler.GetTargetDuration() != 6 {
		t.Errorf("target duration = %d, want 6", handler.GetTargetDuration())
	}
}

func TestHLSPassthroughHandler_RelativeURLs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/live/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:6.000,
../segments/seg0.ts
#EXTINF:6.000,
/absolute/seg1.ts
#EXTINF:6.000,
seg2.ts
`
			w.Write([]byte(playlist))
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/live/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()
	_, err := handler.getRewrittenPlaylist(ctx)
	if err != nil {
		t.Fatalf("getRewrittenPlaylist failed: %v", err)
	}

	// Verify segment URLs were resolved correctly
	handler.mu.RLock()
	defer handler.mu.RUnlock()

	if len(handler.segmentURLs) != 3 {
		t.Fatalf("expected 3 segment URLs, got %d", len(handler.segmentURLs))
	}

	// Relative URL: ../segments/seg0.ts should resolve to /segments/seg0.ts
	if !strings.Contains(handler.segmentURLs[0], "/segments/seg0.ts") {
		t.Errorf("segment 0 not resolved correctly: %s", handler.segmentURLs[0])
	}
	// Absolute URL: /absolute/seg1.ts
	if !strings.HasSuffix(handler.segmentURLs[1], "/absolute/seg1.ts") {
		t.Errorf("segment 1 not resolved correctly: %s", handler.segmentURLs[1])
	}
	// Same directory: seg2.ts
	if !strings.Contains(handler.segmentURLs[2], "/live/seg2.ts") {
		t.Errorf("segment 2 not resolved correctly: %s", handler.segmentURLs[2])
	}
}

func TestHLSPassthroughHandler_ServeSegment(t *testing.T) {
	segmentData := []byte("fake segment data for testing")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:6.000,
segment0.ts
`
			w.Write([]byte(playlist))
		} else if r.URL.Path == "/segment0.ts" {
			w.Header().Set("Content-Type", "video/MP2T")
			w.Write(segmentData)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()

	// First, fetch playlist to populate segment URLs
	_, err := handler.getRewrittenPlaylist(ctx)
	if err != nil {
		t.Fatalf("getRewrittenPlaylist failed: %v", err)
	}

	// Now serve segment
	w := httptest.NewRecorder()
	err = handler.ServeSegment(ctx, w, 0)
	if err != nil {
		t.Fatalf("ServeSegment failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != string(segmentData) {
		t.Errorf("segment data mismatch")
	}
}

func TestHLSPassthroughHandler_SegmentCaching(t *testing.T) {
	fetchCount := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:6.000,
segment0.ts
`
			w.Write([]byte(playlist))
		} else if r.URL.Path == "/segment0.ts" {
			fetchCount++
			w.Write([]byte("segment data"))
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()

	// Fetch playlist
	_, _ = handler.getRewrittenPlaylist(ctx)

	// Fetch segment multiple times
	for i := 0; i < 3; i++ {
		_, err := handler.getSegment(ctx, handler.segmentURLs[0])
		if err != nil {
			t.Fatalf("getSegment failed: %v", err)
		}
	}

	// Segment should only be fetched once
	if fetchCount != 1 {
		t.Errorf("segment fetched %d times, expected 1 (caching should prevent re-fetch)", fetchCount)
	}
}

func TestHLSPassthroughHandler_ServeSegment_NotFound(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:6.000,
segment0.ts
`
			w.Write([]byte(playlist))
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()
	_, _ = handler.getRewrittenPlaylist(ctx)

	// Request invalid segment index
	w := httptest.NewRecorder()
	err := handler.ServeSegment(ctx, w, 999)

	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound, got %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHLSPassthroughHandler_PlaylistCaching(t *testing.T) {
	fetchCount := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			fetchCount++
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:6.000,
segment0.ts
`
			w.Write([]byte(playlist))
		}
	}))
	defer upstream.Close()

	config := DefaultHLSPassthroughConfig()
	config.PlaylistRefreshInterval = 100 * time.Millisecond

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		config,
	)

	ctx := context.Background()

	// Fetch playlist twice in quick succession
	_, _ = handler.getRewrittenPlaylist(ctx)
	_, _ = handler.getRewrittenPlaylist(ctx)

	if fetchCount != 1 {
		t.Errorf("playlist fetched %d times, expected 1 (caching should prevent re-fetch)", fetchCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Fetch again
	_, _ = handler.getRewrittenPlaylist(ctx)

	if fetchCount != 2 {
		t.Errorf("playlist fetched %d times, expected 2 (cache should have expired)", fetchCount)
	}
}

func TestHLSPassthroughHandler_CacheStats(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream.m3u8" {
			playlist := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:42
#EXTINF:6.000,
segment0.ts
#EXTINF:6.000,
segment1.ts
`
			w.Write([]byte(playlist))
		} else {
			w.Write([]byte("segment data"))
		}
	}))
	defer upstream.Close()

	handler := NewHLSPassthroughHandler(
		upstream.URL+"/stream.m3u8",
		"http://proxy.example.com/stream/123",
		DefaultHLSPassthroughConfig(),
	)

	ctx := context.Background()
	_, _ = handler.getRewrittenPlaylist(ctx)

	// Cache a segment
	_, _ = handler.getSegment(ctx, handler.segmentURLs[0])

	stats := handler.CacheStats()

	if stats.CachedSegments != 1 {
		t.Errorf("CachedSegments = %d, want 1", stats.CachedSegments)
	}
	if stats.PlaylistSegments != 2 {
		t.Errorf("PlaylistSegments = %d, want 2", stats.PlaylistSegments)
	}
	if stats.MediaSequence != 42 {
		t.Errorf("MediaSequence = %d, want 42", stats.MediaSequence)
	}
	if stats.TargetDuration != 6 {
		t.Errorf("TargetDuration = %d, want 6", stats.TargetDuration)
	}
}

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		ref      string
		expected string
	}{
		{
			name:     "absolute URL",
			base:     "http://example.com/live/stream.m3u8",
			ref:      "http://other.com/segment.ts",
			expected: "http://other.com/segment.ts",
		},
		{
			name:     "relative same directory",
			base:     "http://example.com/live/stream.m3u8",
			ref:      "segment.ts",
			expected: "http://example.com/live/segment.ts",
		},
		{
			name:     "relative parent directory",
			base:     "http://example.com/live/stream.m3u8",
			ref:      "../segments/segment.ts",
			expected: "http://example.com/segments/segment.ts",
		},
		{
			name:     "absolute path",
			base:     "http://example.com/live/stream.m3u8",
			ref:      "/absolute/segment.ts",
			expected: "http://example.com/absolute/segment.ts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := parseURLForTest(tt.base)
			result := resolveURL(base, tt.ref)
			if result != tt.expected {
				t.Errorf("resolveURL(%q, %q) = %q, want %q", tt.base, tt.ref, result, tt.expected)
			}
		})
	}
}
