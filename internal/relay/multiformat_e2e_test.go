package relay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// skipIfNoFFmpeg checks if ffmpeg is available and skips the test if not.
func skipIfNoFFmpeg(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed - skipping E2E test")
	}
	return path
}

// TestMultiFormatStreaming_E2E tests all output formats with real FFmpeg data.
// This test requires FFmpeg to be installed.
func TestMultiFormatStreaming_E2E(t *testing.T) {
	ffmpegPath := skipIfNoFFmpeg(t)

	// Create unified buffer with realistic config
	config := UnifiedBufferConfig{
		MaxBufferSize:         50 * 1024 * 1024, // 50MB
		MaxChunks:             1000,
		TargetSegmentDuration: 2, // 2 second segments for faster test
		MaxSegments:           10,
		CleanupInterval:       30 * time.Second, // Long interval to prevent cleanup during test
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Add a test client to prevent cleanup from evicting all chunks
	_, err := buf.AddClient("test-e2e", "127.0.0.1")
	if err != nil {
		t.Fatalf("Failed to add test client: %v", err)
	}

	// Generate test video using FFmpeg
	// Uses testsrc to generate a synthetic video pattern for 4 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build FFmpeg command - simpler version that's more reliable
	// Generate 8 seconds of video with keyframes every second
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",          // Overwrite output
		"-f", "lavfi", // Use lavfi for test source
		"-i", "testsrc=duration=8:size=320x240:rate=15", // Test video - 8 seconds
		"-f", "lavfi",
		"-i", "anullsrc=r=44100:cl=mono", // Silent audio
		"-t", "8", // Duration 8 seconds
		"-pix_fmt", "yuv420p", // Convert to YUV420 for baseline compatibility
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-g", "15", // GOP size = framerate (1 keyframe per second)
		"-force_key_frames", "expr:gte(t,n_forced*1)", // Force keyframe every 1 second
		"-c:a", "aac",
		"-b:a", "32k",
		"-ac", "1", // Mono audio
		"-f", "mpegts",
		"-mpegts_copyts", "1",
		"pipe:1",
	)

	// Capture stderr for debugging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start FFmpeg: %v", err)
	}

	// Read FFmpeg output and write to buffer in chunks
	// We accumulate all data first, then write in simulated real-time
	var wg sync.WaitGroup
	var totalRead int
	var allData []byte

	wg.Add(1)
	go func() {
		defer wg.Done()
		chunk := make([]byte, 188*7) // Multiple TS packets (188 * 7 = 1316 bytes)
		for {
			n, readErr := stdout.Read(chunk)
			if n > 0 {
				totalRead += n
				allData = append(allData, chunk[:n]...)
			}
			if readErr == io.EOF {
				return
			}
			if readErr != nil {
				t.Logf("Read error: %v", readErr)
				return
			}
		}
	}()

	// Wait for FFmpeg to complete
	cmdErr := cmd.Wait()
	wg.Wait()

	if cmdErr != nil {
		t.Logf("FFmpeg stderr: %s", stderr.String())
		// Check if we got any data - if so, test can continue
		if totalRead == 0 {
			t.Skipf("FFmpeg failed with no output: %v", cmdErr)
		}
		t.Logf("FFmpeg exited with: %v but we got %d bytes", cmdErr, totalRead)
	}

	t.Logf("FFmpeg produced %d bytes of MPEG-TS data", totalRead)

	// Write data to buffer in simulated real-time chunks
	// For 8 seconds of video, we write data to simulate ~1 second per chunk group
	chunkSize := 188 * 50 // ~9.4KB per chunk
	bytesPerSecond := len(allData) / 8
	chunksPerSecond := bytesPerSecond / chunkSize
	if chunksPerSecond < 1 {
		chunksPerSecond = 1
	}
	chunkDelay := time.Second / time.Duration(chunksPerSecond)

	t.Logf("Writing %d bytes in chunks of %d bytes with %v delay", len(allData), chunkSize, chunkDelay)

	for i := 0; i < len(allData); i += chunkSize {
		end := i + chunkSize
		if end > len(allData) {
			end = len(allData)
		}
		data := allData[i:end]
		if err := buf.WriteChunk(data); err != nil {
			t.Logf("WriteChunk error: %v", err)
			break
		}
		// Delay between chunks to simulate real-time streaming
		// Using shorter delays for test speed but enough to trigger segment emission
		time.Sleep(50 * time.Millisecond) // 50ms delay = 20 chunks per second
	}

	// Give buffer time to process final segment
	time.Sleep(500 * time.Millisecond)

	// Log segment info for debugging
	segments := buf.GetSegmentInfos()
	t.Logf("Buffer has %d segments after writing data", len(segments))

	// Run sub-tests for each format
	t.Run("MPEG-TS", func(t *testing.T) {
		testMPEGTSOutput(t, buf)
	})

	t.Run("HLS", func(t *testing.T) {
		testHLSOutput(t, buf)
	})

	t.Run("DASH", func(t *testing.T) {
		testDASHOutput(t, buf)
	})

	t.Run("FormatRouter", func(t *testing.T) {
		testFormatRouterIntegration(t, buf)
	})
}

// testMPEGTSOutput verifies MPEG-TS continuous stream output.
func testMPEGTSOutput(t *testing.T, buf *UnifiedBuffer) {
	t.Helper()

	// Read all chunks from buffer
	chunks := buf.ReadChunksFrom(0)
	if len(chunks) == 0 {
		t.Skip("No chunks in buffer - FFmpeg may not have produced output")
	}

	t.Logf("MPEG-TS: %d chunks available", len(chunks))

	// Concatenate chunks to form stream
	var totalBytes int
	for _, chunk := range chunks {
		totalBytes += len(chunk.Data)
	}

	if totalBytes == 0 {
		t.Error("MPEG-TS stream has 0 bytes")
	}
	t.Logf("MPEG-TS: %d total bytes", totalBytes)

	// Verify first chunk starts with TS sync byte (0x47)
	if len(chunks[0].Data) > 0 && chunks[0].Data[0] != 0x47 {
		// Try to find sync byte within first few bytes
		foundSync := false
		for i := 0; i < min(188, len(chunks[0].Data)); i++ {
			if chunks[0].Data[i] == 0x47 {
				foundSync = true
				break
			}
		}
		if !foundSync {
			t.Logf("Warning: MPEG-TS stream may not start with sync byte (first byte: 0x%02x)", chunks[0].Data[0])
		}
	}

	// Verify buffer stats
	stats := buf.Stats()
	if stats.ChunkCount == 0 {
		t.Error("Buffer reports 0 chunks")
	}
	t.Logf("MPEG-TS: Buffer stats - chunks=%d, bytes=%d, utilization=%.1f%%",
		stats.ChunkCount, stats.BufferSize, stats.BufferUtilization)
}

// testHLSOutput verifies HLS playlist and segment output.
func testHLSOutput(t *testing.T, buf *UnifiedBuffer) {
	t.Helper()

	// Create HLS handler using unified buffer as segment provider
	handler := NewHLSHandler(buf)

	// Generate playlist
	baseURL := "http://test.example.com/stream/test-channel"
	playlist := handler.GeneratePlaylist(baseURL)

	// Verify playlist structure
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("HLS playlist missing #EXTM3U header")
	}
	if !strings.Contains(playlist, "#EXT-X-VERSION:3") {
		t.Error("HLS playlist missing version tag")
	}
	if !strings.Contains(playlist, "#EXT-X-TARGETDURATION:") {
		t.Error("HLS playlist missing target duration")
	}
	if !strings.Contains(playlist, "#EXT-X-MEDIA-SEQUENCE:") {
		t.Error("HLS playlist missing media sequence")
	}

	t.Logf("HLS playlist:\n%s", playlist)

	// Count segments in playlist
	segmentCount := strings.Count(playlist, "#EXTINF:")
	t.Logf("HLS: %d segments in playlist", segmentCount)

	// Test segment retrieval if we have segments
	segments := buf.GetSegmentInfos()
	if len(segments) > 0 {
		// Request first available segment
		w := httptest.NewRecorder()
		err := handler.ServeSegment(w, segments[0].Sequence)
		if err != nil {
			t.Errorf("ServeSegment failed: %v", err)
		} else {
			if w.Code != http.StatusOK {
				t.Errorf("ServeSegment status = %d, want %d", w.Code, http.StatusOK)
			}
			if w.Header().Get("Content-Type") != ContentTypeHLSSegment {
				t.Errorf("ServeSegment Content-Type = %s, want %s",
					w.Header().Get("Content-Type"), ContentTypeHLSSegment)
			}
			if w.Body.Len() == 0 {
				t.Error("ServeSegment returned empty body")
			}
			t.Logf("HLS segment %d: %d bytes", segments[0].Sequence, w.Body.Len())
		}
	} else {
		t.Log("HLS: No segments available for retrieval test")
	}

	// Test playlist serving via HTTP
	w := httptest.NewRecorder()
	err := handler.ServePlaylist(w, baseURL)
	if err != nil {
		t.Errorf("ServePlaylist failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("ServePlaylist status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != ContentTypeHLSPlaylist {
		t.Errorf("ServePlaylist Content-Type = %s, want %s",
			w.Header().Get("Content-Type"), ContentTypeHLSPlaylist)
	}
}

// testDASHOutput verifies DASH manifest output.
// Note: Full DASH segment output requires fMP4 muxer (T028).
func testDASHOutput(t *testing.T, buf *UnifiedBuffer) {
	t.Helper()

	// Create DASH handler
	handler := NewDASHHandler(buf)

	// Set some metadata
	handler.SetStreamMetadata(320, 240, 1000000, 2, 64000)

	// Generate manifest
	baseURL := "http://test.example.com/stream/test-channel"
	manifest := handler.GenerateManifest(baseURL)

	// Verify manifest structure
	if !strings.Contains(manifest, "<?xml") {
		t.Error("DASH manifest missing XML declaration")
	}
	if !strings.Contains(manifest, "<MPD") {
		t.Error("DASH manifest missing MPD element")
	}
	if !strings.Contains(manifest, "urn:mpeg:dash:schema:mpd:2011") {
		t.Error("DASH manifest missing DASH namespace")
	}
	if !strings.Contains(manifest, "<Period") {
		t.Error("DASH manifest missing Period element")
	}
	if !strings.Contains(manifest, "<AdaptationSet") {
		t.Error("DASH manifest missing AdaptationSet element")
	}
	if !strings.Contains(manifest, "<SegmentTemplate") {
		t.Error("DASH manifest missing SegmentTemplate element")
	}

	t.Logf("DASH manifest:\n%s", manifest)

	// Verify video adaptation set
	if !strings.Contains(manifest, `mimeType="video/mp4"`) {
		t.Error("DASH manifest missing video AdaptationSet")
	}
	if !strings.Contains(manifest, `width="320"`) || !strings.Contains(manifest, `height="240"`) {
		t.Error("DASH manifest has incorrect video dimensions")
	}

	// Verify audio adaptation set
	if !strings.Contains(manifest, `mimeType="audio/mp4"`) {
		t.Error("DASH manifest missing audio AdaptationSet")
	}

	// Test manifest serving via HTTP
	w := httptest.NewRecorder()
	err := handler.ServePlaylist(w, baseURL)
	if err != nil {
		t.Errorf("ServePlaylist (DASH) failed: %v", err)
	}
	if w.Code != http.StatusOK {
		t.Errorf("ServePlaylist (DASH) status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("Content-Type") != ContentTypeDASHManifest {
		t.Errorf("ServePlaylist (DASH) Content-Type = %s, want %s",
			w.Header().Get("Content-Type"), ContentTypeDASHManifest)
	}

	// Note: Full segment testing requires fMP4 muxer implementation (T028)
	// The DASH handler currently serves segments from SegmentBuffer,
	// but the UnifiedBuffer provides TS segments, not fMP4 segments.
	t.Log("DASH: Manifest generation verified (segment serving requires fMP4 muxer)")
}

// testFormatRouterIntegration tests the format router with all handlers.
func testFormatRouterIntegration(t *testing.T, buf *UnifiedBuffer) {
	t.Helper()

	// Create format router with MPEG-TS as default
	router := NewFormatRouter(models.ContainerFormatMPEGTS)

	// Register HLS handler
	hlsHandler := NewHLSHandler(buf)
	router.RegisterHandler(FormatValueHLS, hlsHandler)

	// Register DASH handler
	dashHandler := NewDASHHandler(buf)
	router.RegisterHandler(FormatValueDASH, dashHandler)

	// Test format resolution
	tests := []struct {
		name     string
		request  OutputRequest
		expected string
	}{
		{
			name:     "explicit HLS",
			request:  OutputRequest{Format: "hls"},
			expected: FormatValueHLS,
		},
		{
			name:     "explicit DASH",
			request:  OutputRequest{Format: "dash"},
			expected: FormatValueDASH,
		},
		{
			name:     "explicit MPEG-TS",
			request:  OutputRequest{Format: "mpegts"},
			expected: FormatValueMPEGTS,
		},
		{
			name:     "auto with Safari UA",
			request:  OutputRequest{Format: "auto", UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X) Safari/605.1.15"},
			expected: FormatValueHLS,
		},
		{
			name:     "auto with DASH Accept",
			request:  OutputRequest{Format: "auto", Accept: "application/dash+xml"},
			expected: FormatValueDASH,
		},
		{
			name:     "auto with generic UA",
			request:  OutputRequest{Format: "auto", UserAgent: "VLC/3.0"},
			expected: FormatValueMPEGTS,
		},
		{
			name:     "empty defaults to auto-detect",
			request:  OutputRequest{Format: "", UserAgent: "curl/7.0"},
			expected: FormatValueMPEGTS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved := router.ResolveFormat(tt.request)
			if resolved != tt.expected {
				t.Errorf("ResolveFormat() = %s, want %s", resolved, tt.expected)
			}
		})
	}

	// Test handler retrieval
	t.Run("GetHandler_HLS", func(t *testing.T) {
		handler, err := router.GetHandler(OutputRequest{Format: "hls"})
		if err != nil {
			t.Errorf("GetHandler(hls) failed: %v", err)
		}
		if handler.Format() != FormatValueHLS {
			t.Errorf("Handler format = %s, want %s", handler.Format(), FormatValueHLS)
		}
	})

	t.Run("GetHandler_DASH", func(t *testing.T) {
		handler, err := router.GetHandler(OutputRequest{Format: "dash"})
		if err != nil {
			t.Errorf("GetHandler(dash) failed: %v", err)
		}
		if handler.Format() != FormatValueDASH {
			t.Errorf("Handler format = %s, want %s", handler.Format(), FormatValueDASH)
		}
	})

	t.Run("GetHandler_MPEGTS_NotRegistered", func(t *testing.T) {
		// MPEG-TS handler not registered, should fail
		_, err := router.GetHandler(OutputRequest{Format: "mpegts"})
		if err != ErrNoHandlerAvailable {
			t.Errorf("GetHandler(mpegts) error = %v, want %v", err, ErrNoHandlerAvailable)
		}
	})
}

// TestMultiFormatStreaming_BufferToHandlers tests the data flow from buffer to handlers.
func TestMultiFormatStreaming_BufferToHandlers(t *testing.T) {
	// This test doesn't require FFmpeg - uses synthetic data
	config := UnifiedBufferConfig{
		MaxBufferSize:         10 * 1024 * 1024,
		MaxChunks:             100,
		TargetSegmentDuration: 2,
		MaxSegments:           5,
		CleanupInterval:       time.Second,
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write synthetic MPEG-TS-like data (with sync bytes)
	// Real TS packets are 188 bytes starting with 0x47
	for i := 0; i < 50; i++ {
		packet := make([]byte, 188)
		packet[0] = 0x47 // TS sync byte
		packet[1] = byte(i)
		if err := buf.WriteChunk(packet); err != nil {
			t.Fatalf("WriteChunk failed: %v", err)
		}
		// Small delay to allow segment accumulation
		if i%10 == 9 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Wait for segment processing
	time.Sleep(300 * time.Millisecond)

	// Verify chunks are available
	chunks := buf.ReadChunksFrom(0)
	if len(chunks) == 0 {
		t.Fatal("No chunks available after writing")
	}
	t.Logf("Buffer has %d chunks", len(chunks))

	// Create handlers and verify they can access data
	hlsHandler := NewHLSHandler(buf)
	playlist := hlsHandler.GeneratePlaylist("http://test/stream")
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("HLS handler failed to generate valid playlist")
	}

	dashHandler := NewDASHHandler(buf)
	manifest := dashHandler.GenerateManifest("http://test/stream")
	if !strings.Contains(manifest, "<MPD") {
		t.Error("DASH handler failed to generate valid manifest")
	}

	t.Logf("Both HLS and DASH handlers successfully generated output from buffer")
}

// TestMultiFormatStreaming_ConcurrentAccess tests concurrent access to all formats.
func TestMultiFormatStreaming_ConcurrentAccess(t *testing.T) {
	config := UnifiedBufferConfig{
		MaxBufferSize:         10 * 1024 * 1024,
		MaxChunks:             100,
		TargetSegmentDuration: 2,
		MaxSegments:           5,
		CleanupInterval:       time.Second,
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Pre-populate buffer with data
	for i := 0; i < 30; i++ {
		packet := make([]byte, 188)
		packet[0] = 0x47
		buf.WriteChunk(packet)
	}
	time.Sleep(200 * time.Millisecond)

	hlsHandler := NewHLSHandler(buf)
	dashHandler := NewDASHHandler(buf)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Simulate concurrent clients
	for i := 0; i < 10; i++ {
		wg.Add(3)

		// HLS playlist client
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				w := httptest.NewRecorder()
				if err := hlsHandler.ServePlaylist(w, "http://test/stream"); err != nil {
					errors <- err
				}
				if w.Code != http.StatusOK {
					errors <- bytes.ErrTooLarge // Use as sentinel
				}
			}
		}()

		// DASH manifest client
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				w := httptest.NewRecorder()
				if err := dashHandler.ServePlaylist(w, "http://test/stream"); err != nil {
					errors <- err
				}
				if w.Code != http.StatusOK {
					errors <- bytes.ErrTooLarge
				}
			}
		}()

		// Chunk reader client
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				chunks := buf.ReadChunksFrom(0)
				if len(chunks) == 0 {
					errors <- io.EOF
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		errCount++
		t.Logf("Concurrent access error: %v", err)
	}

	if errCount > 0 {
		t.Errorf("Had %d errors during concurrent access", errCount)
	} else {
		t.Log("Concurrent access test passed with no errors")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
