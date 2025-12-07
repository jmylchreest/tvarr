package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHLSHandler(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	if handler == nil {
		t.Fatal("NewHLSHandler returned nil")
	}
	if handler.Provider() != buf {
		t.Error("handler provider doesn't match")
	}
}

func TestHLSHandler_Format(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	if handler.Format() != FormatValueHLS {
		t.Errorf("Format() = %s, want %s", handler.Format(), FormatValueHLS)
	}
}

func TestHLSHandler_ContentType(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	if handler.ContentType() != ContentTypeHLSPlaylist {
		t.Errorf("ContentType() = %s, want %s", handler.ContentType(), ContentTypeHLSPlaylist)
	}
}

func TestHLSHandler_SegmentContentType(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	if handler.SegmentContentType() != ContentTypeHLSSegment {
		t.Errorf("SegmentContentType() = %s, want %s", handler.SegmentContentType(), ContentTypeHLSSegment)
	}
}

func TestHLSHandler_SupportsStreaming(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	if handler.SupportsStreaming() {
		t.Error("HLS handler should not support streaming")
	}
}

func TestHLSHandler_GeneratePlaylist_Empty(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})
	handler := NewHLSHandler(buf)

	playlist := handler.GeneratePlaylist("http://example.com/stream")

	// Should be a valid minimal playlist
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("playlist missing #EXTM3U")
	}
	if !strings.Contains(playlist, "#EXT-X-VERSION:3") {
		t.Error("playlist missing #EXT-X-VERSION:3")
	}
	if !strings.Contains(playlist, "#EXT-X-TARGETDURATION:6") {
		t.Error("playlist missing #EXT-X-TARGETDURATION:6")
	}
	if !strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:0") {
		t.Error("playlist missing #EXT-X-MEDIA-SEQUENCE:0")
	}
}

func TestHLSHandler_GeneratePlaylist_WithSegments(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Add some segments
	for i := 0; i < 3; i++ {
		seg := Segment{
			Duration: 6.0,
			Data:     make([]byte, 100),
		}
		buf.AddSegment(seg)
	}

	handler := NewHLSHandler(buf)
	playlist := handler.GeneratePlaylist("http://example.com/stream")

	// Verify playlist structure
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("playlist missing #EXTM3U")
	}
	if !strings.Contains(playlist, "#EXT-X-VERSION:3") {
		t.Error("playlist missing #EXT-X-VERSION:3")
	}
	if !strings.Contains(playlist, "#EXT-X-TARGETDURATION:6") {
		t.Error("playlist missing #EXT-X-TARGETDURATION:6")
	}
	if !strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:1") {
		t.Error("playlist missing #EXT-X-MEDIA-SEQUENCE:1")
	}

	// Should have 3 segments
	if strings.Count(playlist, "#EXTINF:") != 3 {
		t.Errorf("expected 3 #EXTINF entries, got %d", strings.Count(playlist, "#EXTINF:"))
	}

	// Should have proper segment URLs
	if !strings.Contains(playlist, "format=hls") {
		t.Error("segment URLs missing format=hls parameter")
	}
	if !strings.Contains(playlist, "seg=1") {
		t.Error("segment URLs missing seg parameter")
	}
}

func TestHLSHandler_GeneratePlaylist_DurationRounding(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 4,
		MaxBufferSize:  1024 * 1024,
	})

	// Add segment with longer duration than target
	seg := Segment{
		Duration: 8.5,
		Data:     make([]byte, 100),
	}
	buf.AddSegment(seg)

	handler := NewHLSHandler(buf)
	playlist := handler.GeneratePlaylist("http://example.com/stream")

	// Target duration should be rounded up to 9
	if !strings.Contains(playlist, "#EXT-X-TARGETDURATION:9") {
		t.Errorf("expected #EXT-X-TARGETDURATION:9 for segment with duration 8.5, got playlist:\n%s", playlist)
	}
}

func TestHLSHandler_ServePlaylist(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	// Add a segment
	buf.AddSegment(Segment{Duration: 6.0, Data: make([]byte, 100)})

	// Create test request/response
	w := httptest.NewRecorder()
	err := handler.ServePlaylist(w, "http://example.com/stream")
	if err != nil {
		t.Fatalf("ServePlaylist failed: %v", err)
	}

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != ContentTypeHLSPlaylist {
		t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeHLSPlaylist)
	}
	if w.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Errorf("unexpected Cache-Control header: %s", w.Header().Get("Cache-Control"))
	}

	// Should contain playlist content
	if !strings.Contains(w.Body.String(), "#EXTM3U") {
		t.Error("response body doesn't contain playlist")
	}
}

func TestHLSHandler_ServeSegment(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	// Add segments
	for i := 0; i < 3; i++ {
		buf.AddSegment(Segment{Duration: 6.0, Data: []byte{byte(i), byte(i), byte(i)}})
	}

	// Request segment 2
	w := httptest.NewRecorder()
	err := handler.ServeSegment(w, 2)
	if err != nil {
		t.Fatalf("ServeSegment failed: %v", err)
	}

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != ContentTypeHLSSegment {
		t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeHLSSegment)
	}
	if w.Header().Get("Content-Length") != "3" {
		t.Errorf("Content-Length = %s, want 3", w.Header().Get("Content-Length"))
	}

	// Check segment data (segment 2 has data [1,1,1])
	if w.Body.Len() != 3 {
		t.Errorf("body length = %d, want 3", w.Body.Len())
	}
	data := w.Body.Bytes()
	if data[0] != 1 || data[1] != 1 || data[2] != 1 {
		t.Errorf("unexpected segment data: %v", data)
	}
}

func TestHLSHandler_ServeSegment_NotFound(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	// Request non-existent segment
	w := httptest.NewRecorder()
	err := handler.ServeSegment(w, 999)

	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound, got %v", err)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHLSHandler_ServeStream(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewHLSHandler(buf)

	w := httptest.NewRecorder()
	err := handler.ServeStream(context.Background(), w)

	if err != ErrUnsupportedOperation {
		t.Errorf("expected ErrUnsupportedOperation, got %v", err)
	}
}

func TestHLSHandler_TrailingSlashHandling(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	buf.AddSegment(Segment{Duration: 6.0, Data: make([]byte, 100)})
	handler := NewHLSHandler(buf)

	// Test with trailing slash
	playlist := handler.GeneratePlaylist("http://example.com/stream/")

	// Should not have double slashes in segment URLs
	if strings.Contains(playlist, "stream/?") || strings.Contains(playlist, "stream//") {
		t.Error("playlist contains malformed URL with trailing slash")
	}
}
