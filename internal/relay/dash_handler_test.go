package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDASHHandler(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	if handler == nil {
		t.Fatal("NewDASHHandler returned nil")
	}
	if handler.Provider() != buf {
		t.Error("handler provider doesn't match")
	}
}

func TestDASHHandler_Format(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	if handler.Format() != FormatValueDASH {
		t.Errorf("Format() = %s, want %s", handler.Format(), FormatValueDASH)
	}
}

func TestDASHHandler_ContentType(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	if handler.ContentType() != ContentTypeDASHManifest {
		t.Errorf("ContentType() = %s, want %s", handler.ContentType(), ContentTypeDASHManifest)
	}
}

func TestDASHHandler_SegmentContentType(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	if handler.SegmentContentType() != ContentTypeDASHSegment {
		t.Errorf("SegmentContentType() = %s, want %s", handler.SegmentContentType(), ContentTypeDASHSegment)
	}
}

func TestDASHHandler_SupportsStreaming(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	if handler.SupportsStreaming() {
		t.Error("DASH handler should not support streaming")
	}
}

func TestDASHHandler_GenerateManifest_Empty(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})
	handler := NewDASHHandler(buf)

	manifest := handler.GenerateManifest("http://example.com/stream")

	// Should be a valid minimal MPD
	if !strings.Contains(manifest, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Error("manifest missing XML declaration")
	}
	if !strings.Contains(manifest, "<MPD") {
		t.Error("manifest missing MPD element")
	}
	if !strings.Contains(manifest, `type="dynamic"`) {
		t.Error("manifest should be dynamic type")
	}
	if !strings.Contains(manifest, "urn:mpeg:dash:profile:isoff-live:2011") {
		t.Error("manifest missing live profile")
	}
	if !strings.Contains(manifest, "<Period") {
		t.Error("manifest missing Period element")
	}
	if !strings.Contains(manifest, "<AdaptationSet") {
		t.Error("manifest missing AdaptationSet element")
	}
}

func TestDASHHandler_GenerateManifest_WithSegments(t *testing.T) {
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

	handler := NewDASHHandler(buf)
	manifest := handler.GenerateManifest("http://example.com/stream")

	// Verify manifest structure
	if !strings.Contains(manifest, "<MPD") {
		t.Error("manifest missing MPD element")
	}
	if !strings.Contains(manifest, "minimumUpdatePeriod") {
		t.Error("manifest missing minimumUpdatePeriod")
	}
	if !strings.Contains(manifest, "<SegmentTemplate") {
		t.Error("manifest missing SegmentTemplate")
	}
	if !strings.Contains(manifest, `startNumber="1"`) {
		t.Error("manifest should start at segment 1")
	}

	// Should have proper segment URLs with format parameter
	if !strings.Contains(manifest, "format=dash") {
		t.Error("segment URLs missing format=dash parameter")
	}
	if !strings.Contains(manifest, "seg=$Number$") {
		t.Error("segment URLs missing seg=$Number$ template")
	}
}

func TestDASHHandler_GenerateManifest_WithMetadata(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	// Set stream metadata
	handler.SetStreamMetadata(1280, 720, 3000000, 2, 128000)

	manifest := handler.GenerateManifest("http://example.com/stream")

	if !strings.Contains(manifest, `width="1280"`) {
		t.Error("manifest missing correct width")
	}
	if !strings.Contains(manifest, `height="720"`) {
		t.Error("manifest missing correct height")
	}
	if !strings.Contains(manifest, `bandwidth="3000000"`) {
		t.Error("manifest missing correct video bandwidth")
	}
	if !strings.Contains(manifest, `bandwidth="128000"`) {
		t.Error("manifest missing correct audio bandwidth")
	}
}

func TestDASHHandler_GenerateManifest_WithInitSegments(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	buf.AddSegment(Segment{Duration: 6.0, Data: make([]byte, 100)})

	handler := NewDASHHandler(buf)

	// Set init segments
	handler.SetInitSegments([]byte{1, 2, 3}, []byte{4, 5, 6})

	manifest := handler.GenerateManifest("http://example.com/stream")

	// Should have initialization URLs
	if !strings.Contains(manifest, "init=v") {
		t.Error("manifest missing video initialization URL")
	}
	if !strings.Contains(manifest, "init=a") {
		t.Error("manifest missing audio initialization URL")
	}
}

func TestDASHHandler_ServePlaylist(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

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
	if w.Header().Get("Content-Type") != ContentTypeDASHManifest {
		t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeDASHManifest)
	}
	if w.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Errorf("unexpected Cache-Control header: %s", w.Header().Get("Cache-Control"))
	}

	// Should contain manifest content
	if !strings.Contains(w.Body.String(), "<MPD") {
		t.Error("response body doesn't contain manifest")
	}
}

func TestDASHHandler_ServeSegment(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

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
	if w.Header().Get("Content-Type") != ContentTypeDASHSegment {
		t.Errorf("Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeDASHSegment)
	}
	if w.Header().Get("Content-Length") != "3" {
		t.Errorf("Content-Length = %s, want 3", w.Header().Get("Content-Length"))
	}

	// Check segment data
	data := w.Body.Bytes()
	if data[0] != 1 || data[1] != 1 || data[2] != 1 {
		t.Errorf("unexpected segment data: %v", data)
	}
}

func TestDASHHandler_ServeSegment_NotFound(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

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

func TestDASHHandler_ServeInitSegment(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	// Set init segments
	videoInit := []byte{1, 2, 3, 4, 5}
	audioInit := []byte{6, 7, 8}
	handler.SetInitSegments(videoInit, audioInit)

	// Test video init
	w := httptest.NewRecorder()
	err := handler.ServeInitSegment(w, "v")
	if err != nil {
		t.Fatalf("ServeInitSegment(v) failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("video init status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != ContentTypeDASHInit {
		t.Errorf("video init Content-Type = %s, want %s", w.Header().Get("Content-Type"), ContentTypeDASHInit)
	}
	if w.Body.Len() != 5 {
		t.Errorf("video init length = %d, want 5", w.Body.Len())
	}

	// Test audio init
	w = httptest.NewRecorder()
	err = handler.ServeInitSegment(w, "a")
	if err != nil {
		t.Fatalf("ServeInitSegment(a) failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("audio init status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() != 3 {
		t.Errorf("audio init length = %d, want 3", w.Body.Len())
	}
}

func TestDASHHandler_ServeInitSegment_NotAvailable(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	// Request without setting init segments
	w := httptest.NewRecorder()
	err := handler.ServeInitSegment(w, "v")

	if err == nil {
		t.Error("expected error for unavailable init segment")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDASHHandler_ServeInitSegment_InvalidType(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)
	handler.SetInitSegments([]byte{1}, []byte{2})

	w := httptest.NewRecorder()
	err := handler.ServeInitSegment(w, "x")

	if err == nil {
		t.Error("expected error for invalid stream type")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDASHHandler_ServeStream(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	handler := NewDASHHandler(buf)

	w := httptest.NewRecorder()
	err := handler.ServeStream(context.Background(), w)

	if err != ErrUnsupportedOperation {
		t.Errorf("expected ErrUnsupportedOperation, got %v", err)
	}
}

func TestDASHHandler_TrailingSlashHandling(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	buf.AddSegment(Segment{Duration: 6.0, Data: make([]byte, 100)})
	handler := NewDASHHandler(buf)

	// Test with trailing slash
	manifest := handler.GenerateManifest("http://example.com/stream/")

	// Should not have double slashes in segment URLs
	if strings.Contains(manifest, "stream/?") || strings.Contains(manifest, "stream//") {
		t.Error("manifest contains malformed URL with trailing slash")
	}
}
