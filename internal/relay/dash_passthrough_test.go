package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewDASHPassthroughHandler(t *testing.T) {
	handler := NewDASHPassthroughHandler(
		"http://upstream.example.com/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	if handler == nil {
		t.Fatal("NewDASHPassthroughHandler returned nil")
	}
	if handler.upstreamURL != "http://upstream.example.com/manifest.mpd" {
		t.Errorf("unexpected upstream URL: %s", handler.upstreamURL)
	}
	if handler.baseURL != "http://proxy.example.com/stream/123" {
		t.Errorf("unexpected base URL: %s", handler.baseURL)
	}
}

func TestDASHPassthroughHandler_RewriteManifest(t *testing.T) {
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

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Header().Set("Content-Type", "application/dash+xml")
			w.Write([]byte(manifest))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()
	rewritten, err := handler.getRewrittenManifest(ctx)
	if err != nil {
		t.Fatalf("getRewrittenManifest failed: %v", err)
	}

	// Verify manifest structure preserved
	if !strings.Contains(rewritten, "<MPD") {
		t.Error("manifest missing MPD element")
	}
	if !strings.Contains(rewritten, "<AdaptationSet") {
		t.Error("manifest missing AdaptationSet element")
	}

	// Verify URLs were rewritten
	if !strings.Contains(rewritten, "http://proxy.example.com/stream/123?format=dash&init=") {
		t.Errorf("manifest missing rewritten init URL, got:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "http://proxy.example.com/stream/123?format=dash&seg=") {
		t.Errorf("manifest missing rewritten segment URL")
	}
}

func TestDASHPassthroughHandler_ServeManifest(t *testing.T) {
	manifest := `<?xml version="1.0"?><MPD><Period/></MPD>`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dash+xml")
		w.Write([]byte(manifest))
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()
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
	if !strings.Contains(w.Body.String(), "<MPD") {
		t.Error("response body doesn't contain manifest")
	}
}

func TestDASHPassthroughHandler_ServeSegment(t *testing.T) {
	segmentData := []byte("fake segment data")

	manifest := `<?xml version="1.0"?><MPD>
<Period><AdaptationSet>
<SegmentTemplate media="segment.m4s"/>
</AdaptationSet></Period>
</MPD>`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Write([]byte(manifest))
		} else if r.URL.Path == "/segment.m4s" {
			w.Header().Set("Content-Type", "video/iso.segment")
			w.Write(segmentData)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()

	// First fetch manifest to populate mappings
	_, err := handler.getRewrittenManifest(ctx)
	if err != nil {
		t.Fatalf("getRewrittenManifest failed: %v", err)
	}

	// Get the segment ID from the mapping
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

	// Serve the segment
	w := httptest.NewRecorder()
	err = handler.ServeSegment(ctx, w, segID)
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

func TestDASHPassthroughHandler_ServeInitSegment(t *testing.T) {
	initData := []byte("fake init segment")

	manifest := `<?xml version="1.0"?><MPD>
<Period><AdaptationSet>
<SegmentTemplate initialization="init.mp4" media="segment.m4s"/>
</AdaptationSet></Period>
</MPD>`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Write([]byte(manifest))
		} else if r.URL.Path == "/init.mp4" {
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(initData)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()

	// First fetch manifest
	_, err := handler.getRewrittenManifest(ctx)
	if err != nil {
		t.Fatalf("getRewrittenManifest failed: %v", err)
	}

	// Get init ID
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
	w := httptest.NewRecorder()
	err = handler.ServeInitSegment(ctx, w, initID)
	if err != nil {
		t.Fatalf("ServeInitSegment failed: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != string(initData) {
		t.Errorf("init segment data mismatch")
	}
}

func TestDASHPassthroughHandler_SegmentCaching(t *testing.T) {
	fetchCount := 0

	manifest := `<?xml version="1.0"?><MPD>
<Period><AdaptationSet>
<SegmentTemplate media="segment.m4s"/>
</AdaptationSet></Period>
</MPD>`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Write([]byte(manifest))
		} else if r.URL.Path == "/segment.m4s" {
			fetchCount++
			w.Write([]byte("segment data"))
		}
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()
	_, _ = handler.getRewrittenManifest(ctx)

	// Get segment URL
	handler.mu.RLock()
	var segmentURL string
	for _, url := range handler.segmentMapping {
		segmentURL = url
		break
	}
	handler.mu.RUnlock()

	// Fetch segment multiple times
	for i := 0; i < 3; i++ {
		_, err := handler.getSegment(ctx, segmentURL)
		if err != nil {
			t.Fatalf("getSegment failed: %v", err)
		}
	}

	if fetchCount != 1 {
		t.Errorf("segment fetched %d times, expected 1", fetchCount)
	}
}

func TestDASHPassthroughHandler_ManifestCaching(t *testing.T) {
	fetchCount := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Write([]byte(`<?xml version="1.0"?><MPD/>`))
	}))
	defer upstream.Close()

	config := DefaultDASHPassthroughConfig()
	config.ManifestRefreshInterval = 100 * time.Millisecond

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		config,
	)

	ctx := context.Background()

	// Fetch twice quickly
	_, _ = handler.getRewrittenManifest(ctx)
	_, _ = handler.getRewrittenManifest(ctx)

	if fetchCount != 1 {
		t.Errorf("manifest fetched %d times, expected 1", fetchCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	_, _ = handler.getRewrittenManifest(ctx)

	if fetchCount != 2 {
		t.Errorf("manifest fetched %d times, expected 2", fetchCount)
	}
}

func TestDASHPassthroughHandler_ServeSegment_NotFound(t *testing.T) {
	handler := NewDASHPassthroughHandler(
		"http://upstream.example.com/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()
	w := httptest.NewRecorder()
	err := handler.ServeSegment(ctx, w, "nonexistent")

	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound, got %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDASHPassthroughHandler_CacheStats(t *testing.T) {
	manifest := `<?xml version="1.0"?><MPD>
<Period><AdaptationSet>
<SegmentTemplate initialization="init.mp4" media="segment.m4s"/>
</AdaptationSet></Period>
</MPD>`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Write([]byte(manifest))
		} else {
			w.Write([]byte("data"))
		}
	}))
	defer upstream.Close()

	handler := NewDASHPassthroughHandler(
		upstream.URL+"/manifest.mpd",
		"http://proxy.example.com/stream/123",
		DefaultDASHPassthroughConfig(),
	)

	ctx := context.Background()
	_, _ = handler.getRewrittenManifest(ctx)

	// Cache a segment
	handler.mu.RLock()
	var segmentURL string
	for _, url := range handler.segmentMapping {
		segmentURL = url
		break
	}
	handler.mu.RUnlock()

	if segmentURL != "" {
		_, _ = handler.getSegment(ctx, segmentURL)
	}

	stats := handler.CacheStats()

	if stats.SegmentMappings == 0 {
		t.Error("expected segment mappings")
	}
	if stats.InitMappings == 0 {
		t.Error("expected init mappings")
	}
}

func TestResolveDASHURL(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		ref      string
		expected string
	}{
		{
			name:     "absolute URL",
			base:     "http://example.com/dash/manifest.mpd",
			ref:      "http://cdn.example.com/segment.m4s",
			expected: "http://cdn.example.com/segment.m4s",
		},
		{
			name:     "relative same directory",
			base:     "http://example.com/dash/manifest.mpd",
			ref:      "segment.m4s",
			expected: "http://example.com/dash/segment.m4s",
		},
		{
			name:     "relative parent directory",
			base:     "http://example.com/dash/live/manifest.mpd",
			ref:      "../segments/segment.m4s",
			expected: "http://example.com/dash/segments/segment.m4s",
		},
		{
			name:     "absolute path",
			base:     "http://example.com/dash/manifest.mpd",
			ref:      "/segments/segment.m4s",
			expected: "http://example.com/segments/segment.m4s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, _ := parseURLForTest(tt.base)
			result := resolveDASHURL(base, tt.ref)
			if result != tt.expected {
				t.Errorf("resolveDASHURL(%q, %q) = %q, want %q", tt.base, tt.ref, result, tt.expected)
			}
		})
	}
}
