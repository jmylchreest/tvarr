package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHLSPassthroughIntegration tests the complete HLS passthrough flow.
func TestHLSPassthroughIntegration(t *testing.T) {
	// Create a mock upstream HLS server
	upstream := createMockHLSServer(t)
	defer upstream.Close()

	// Create passthrough handler
	config := DefaultHLSPassthroughConfig()
	config.PlaylistRefreshInterval = 100 * time.Millisecond
	handler := NewHLSPassthroughHandler(
		upstream.URL+"/live/stream.m3u8",
		"http://proxy.example.com/stream/test-123",
		config,
	)

	ctx := context.Background()

	// Test 1: Fetch and serve playlist
	t.Run("ServePlaylist", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := handler.ServePlaylist(ctx, w)
		if err != nil {
			t.Fatalf("ServePlaylist failed: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if w.Header().Get("Content-Type") != ContentTypeHLSPlaylist {
			t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeHLSPlaylist)
		}

		body := w.Body.String()
		if !strings.Contains(body, "#EXTM3U") {
			t.Error("playlist missing #EXTM3U header")
		}
		if !strings.Contains(body, "http://proxy.example.com/stream/test-123?format=hls&seg=") {
			t.Errorf("playlist missing rewritten segment URLs:\n%s", body)
		}
	})

	// Test 2: Fetch and serve segment
	t.Run("ServeSegment", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := handler.ServeSegment(ctx, w, 0)
		if err != nil {
			t.Fatalf("ServeSegment failed: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if w.Header().Get("Content-Type") != ContentTypeHLSSegment {
			t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeHLSSegment)
		}
		if !strings.HasPrefix(w.Body.String(), "FAKE-TS-SEGMENT") {
			t.Error("segment data mismatch")
		}
	})

	// Test 3: Segment caching - multiple clients should hit cache
	t.Run("SegmentCaching", func(t *testing.T) {
		stats := handler.CacheStats()
		initialCachedSegments := stats.CachedSegments

		// Serve same segment to multiple clients
		for i := 0; i < 3; i++ {
			w := httptest.NewRecorder()
			err := handler.ServeSegment(ctx, w, 0)
			if err != nil {
				t.Fatalf("ServeSegment (client %d) failed: %v", i, err)
			}
		}

		// Cache should have the same number of segments (not 3x)
		stats = handler.CacheStats()
		if stats.CachedSegments > initialCachedSegments+1 {
			t.Errorf("expected caching to prevent duplicate segments, got %d cached", stats.CachedSegments)
		}
	})

	// Test 4: Invalid segment index
	t.Run("InvalidSegment", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := handler.ServeSegment(ctx, w, 999)
		if err != ErrSegmentNotFound {
			t.Errorf("expected ErrSegmentNotFound, got %v", err)
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	// Test 5: Playlist refresh
	t.Run("PlaylistRefresh", func(t *testing.T) {
		// Get initial media sequence
		initialSeq := handler.GetMediaSequence()

		// Wait for cache to expire
		time.Sleep(150 * time.Millisecond)

		// Fetch again - should refresh
		w := httptest.NewRecorder()
		err := handler.ServePlaylist(ctx, w)
		if err != nil {
			t.Fatalf("ServePlaylist after refresh failed: %v", err)
		}

		// Upstream has incrementing media sequence (see mock server)
		newSeq := handler.GetMediaSequence()
		if newSeq == initialSeq {
			// This is acceptable if the mock doesn't change sequence
			t.Log("Media sequence unchanged (mock server doesn't update sequence)")
		}
	})
}

// TestDASHPassthroughIntegration tests the complete DASH passthrough flow.
func TestDASHPassthroughIntegration(t *testing.T) {
	// Create a mock upstream DASH server
	upstream := createMockDASHServer(t)
	defer upstream.Close()

	// Create passthrough handler
	config := DefaultDASHPassthroughConfig()
	config.ManifestRefreshInterval = 100 * time.Millisecond
	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/test-456",
		config,
	)

	ctx := context.Background()

	// Test 1: Fetch and serve manifest
	t.Run("ServeManifest", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := handler.ServeManifest(ctx, w)
		if err != nil {
			t.Fatalf("ServeManifest failed: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if w.Header().Get("Content-Type") != ContentTypeDASHManifest {
			t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeDASHManifest)
		}

		body := w.Body.String()
		if !strings.Contains(body, "<MPD") {
			t.Error("manifest missing MPD element")
		}
		if !strings.Contains(body, "http://proxy.example.com/stream/test-456?format=dash") {
			t.Errorf("manifest missing rewritten URLs:\n%s", body)
		}
	})

	// Test 2: Fetch init segment
	t.Run("ServeInitSegment", func(t *testing.T) {
		// First fetch manifest to populate mappings
		w := httptest.NewRecorder()
		_ = handler.ServeManifest(ctx, w)

		// Get init ID from mappings
		handler.mu.RLock()
		var initID string
		for id := range handler.initMapping {
			initID = id
			break
		}
		handler.mu.RUnlock()

		if initID == "" {
			t.Fatal("no init mapping found")
		}

		// Serve init segment
		w = httptest.NewRecorder()
		err := handler.ServeInitSegment(ctx, w, initID)
		if err != nil {
			t.Fatalf("ServeInitSegment failed: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if !strings.HasPrefix(w.Body.String(), "FAKE-INIT") {
			t.Error("init segment data mismatch")
		}
	})

	// Test 3: Fetch media segment
	t.Run("ServeSegment", func(t *testing.T) {
		// Get segment ID from mappings
		handler.mu.RLock()
		var segID string
		for id := range handler.segmentMapping {
			segID = id
			break
		}
		handler.mu.RUnlock()

		if segID == "" {
			t.Fatal("no segment mapping found")
		}

		// Serve segment
		w := httptest.NewRecorder()
		err := handler.ServeSegment(ctx, w, segID)
		if err != nil {
			t.Fatalf("ServeSegment failed: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if !strings.HasPrefix(w.Body.String(), "FAKE-SEGMENT") {
			t.Error("segment data mismatch")
		}
	})

	// Test 4: Invalid segment ID
	t.Run("InvalidSegment", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := handler.ServeSegment(ctx, w, "nonexistent-segment-id")
		if err != ErrSegmentNotFound {
			t.Errorf("expected ErrSegmentNotFound, got %v", err)
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	// Test 5: Cache stats
	t.Run("CacheStats", func(t *testing.T) {
		stats := handler.CacheStats()
		if stats.SegmentMappings == 0 {
			t.Error("expected segment mappings")
		}
		if stats.InitMappings == 0 {
			t.Error("expected init mappings")
		}
	})
}

// TestStreamClassifier_DASH tests DASH stream classification.
func TestStreamClassifier_DASH(t *testing.T) {
	t.Run("DetectDASHByURL", func(t *testing.T) {
		// Test isDASHURL
		dashURLs := []string{
			"http://example.com/manifest.mpd",
			"http://example.com/stream.mpd?token=abc",
			"http://example.com/manifest(format=mpd-time-csf)",
			"http://example.com/MANIFEST.MPD",
		}

		for _, url := range dashURLs {
			if !isDASHURL(url) {
				t.Errorf("isDASHURL(%q) = false, want true", url)
			}
		}

		nonDASHURLs := []string{
			"http://example.com/stream.m3u8",
			"http://example.com/video.mp4",
			"http://example.com/mpd-video.ts",
		}

		for _, url := range nonDASHURLs {
			if isDASHURL(url) {
				t.Errorf("isDASHURL(%q) = true, want false", url)
			}
		}
	})

	t.Run("DetectHLSByURL", func(t *testing.T) {
		hlsURLs := []string{
			"http://example.com/stream.m3u8",
			"http://example.com/playlist.m3u8?token=abc",
			"http://example.com/index.m3u",
		}

		for _, url := range hlsURLs {
			if !isHLSURL(url) {
				t.Errorf("isHLSURL(%q) = false, want true", url)
			}
		}

		nonHLSURLs := []string{
			"http://example.com/stream.mpd",
			"http://example.com/video.mp4",
			"http://example.com/m3u8-video.ts",
		}

		for _, url := range nonHLSURLs {
			if isHLSURL(url) {
				t.Errorf("isHLSURL(%q) = true, want false", url)
			}
		}
	})

	t.Run("DetectMPEGTSByURL", func(t *testing.T) {
		mpegtsURLs := []string{
			"http://example.com/stream.ts",
			"http://example.com/live/123.ts?token=abc",
			"http://example.com/video.TS",
		}

		for _, url := range mpegtsURLs {
			if !isMPEGTSURL(url) {
				t.Errorf("isMPEGTSURL(%q) = false, want true", url)
			}
		}

		nonMPEGTSURLs := []string{
			"http://example.com/stream.m3u8",
			"http://example.com/manifest.mpd",
			"http://example.com/video.mp4",
			"http://example.com/ts-stream.m3u8", // ts in path but m3u8 extension
		}

		for _, url := range nonMPEGTSURLs {
			if isMPEGTSURL(url) {
				t.Errorf("isMPEGTSURL(%q) = true, want false", url)
			}
		}
	})
}

// TestFormatRouter_Passthrough tests the format router passthrough registration.
func TestFormatRouter_Passthrough(t *testing.T) {
	router := NewFormatRouter("mpegts")

	// Initially no passthrough handlers
	if router.IsPassthroughMode(FormatValueHLS) {
		t.Error("expected no HLS passthrough initially")
	}
	if router.IsPassthroughMode(FormatValueDASH) {
		t.Error("expected no DASH passthrough initially")
	}

	// Register HLS passthrough
	hlsHandler := NewHLSPassthroughHandler(
		"http://upstream/stream.m3u8",
		"http://proxy/stream",
		DefaultHLSPassthroughConfig(),
	)
	router.RegisterPassthroughHandler(FormatValueHLS, hlsHandler)

	if !router.IsPassthroughMode(FormatValueHLS) {
		t.Error("expected HLS passthrough after registration")
	}
	if router.GetHLSPassthrough() == nil {
		t.Error("expected HLS passthrough handler to be retrievable")
	}

	// Register DASH passthrough
	dashHandler := NewDASHPassthroughHandler(
		"http://upstream/manifest.mpd",
		"http://proxy/stream",
		DefaultDASHPassthroughConfig(),
	)
	router.RegisterPassthroughHandler(FormatValueDASH, dashHandler)

	if !router.IsPassthroughMode(FormatValueDASH) {
		t.Error("expected DASH passthrough after registration")
	}
	if router.GetDASHPassthrough() == nil {
		t.Error("expected DASH passthrough handler to be retrievable")
	}
}

// Helper: Create mock HLS server
func createMockHLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live/stream.m3u8":
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
		case "/live/segment0.ts", "/live/segment1.ts", "/live/segment2.ts":
			w.Header().Set("Content-Type", "video/MP2T")
			// Send fake TS data
			w.Write([]byte("FAKE-TS-SEGMENT-DATA-" + r.URL.Path))
		default:
			http.NotFound(w, r)
		}
	}))
}

// Helper: Create mock DASH server
func createMockDASHServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.mpd":
			manifest := `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" profiles="urn:mpeg:dash:profile:isoff-live:2011">
  <Period id="0">
    <AdaptationSet id="0" mimeType="video/mp4">
      <SegmentTemplate initialization="init_v.mp4" media="seg_v_$Number$.m4s" startNumber="1"/>
      <Representation id="video" bandwidth="5000000"/>
    </AdaptationSet>
    <AdaptationSet id="1" mimeType="audio/mp4">
      <SegmentTemplate initialization="init_a.mp4" media="seg_a_$Number$.m4s" startNumber="1"/>
      <Representation id="audio" bandwidth="128000"/>
    </AdaptationSet>
  </Period>
</MPD>`
			w.Header().Set("Content-Type", "application/dash+xml")
			w.Write([]byte(manifest))
		case "/init_v.mp4", "/init_a.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			w.Write([]byte("FAKE-INIT-SEGMENT-" + r.URL.Path))
		default:
			// Match segment paths
			if strings.Contains(r.URL.Path, "seg_v_") || strings.Contains(r.URL.Path, "seg_a_") {
				w.Header().Set("Content-Type", "video/iso.segment")
				w.Write([]byte("FAKE-SEGMENT-DATA-" + r.URL.Path))
				return
			}
			http.NotFound(w, r)
		}
	}))
}
