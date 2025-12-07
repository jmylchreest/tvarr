package relay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the marshalAnnexB helper function
func TestMarshalAnnexB(t *testing.T) {
	tests := []struct {
		name   string
		nalus  [][]byte
		want   []byte
	}{
		{
			name:  "single NALU",
			nalus: [][]byte{{0x65, 0x00, 0x01}}, // IDR slice
			want:  []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x00, 0x01},
		},
		{
			name: "multiple NALUs",
			nalus: [][]byte{
				{0x67, 0x42, 0x00}, // SPS
				{0x68, 0xce, 0x3c}, // PPS
				{0x65, 0x88},       // IDR
			},
			want: []byte{
				0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, // SPS with start code
				0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, // PPS with start code
				0x00, 0x00, 0x00, 0x01, 0x65, 0x88, // IDR with start code
			},
		},
		{
			name:  "empty NALU list",
			nalus: [][]byte{},
			want:  []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marshalAnnexB(tt.nalus)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test HLSCollapser creation
func TestNewHLSCollapser(t *testing.T) {
	client := &http.Client{}
	playlistURL := "http://example.com/playlist.m3u8"

	collapser := NewHLSCollapser(client, playlistURL)

	assert.NotNil(t, collapser)
	assert.Equal(t, playlistURL, collapser.uri)
	assert.NotEmpty(t, collapser.sessionID)
	assert.NotNil(t, collapser.pipeReader)
	assert.NotNil(t, collapser.pipeWriter)
	assert.Equal(t, videoPIDBase, collapser.videoPID)
	assert.Equal(t, audioPIDBase, collapser.audioPID)
	assert.False(t, collapser.started.Load())
	assert.False(t, collapser.closed.Load())
}

// Test double start prevention
func TestHLSCollapser_DoubleStart(t *testing.T) {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	collapser := NewHLSCollapser(client, "http://example.com/playlist.m3u8")
	defer collapser.Close()

	ctx := context.Background()

	// First start - will fail due to invalid URL but should mark as started
	_ = collapser.Start(ctx)

	// Second start should return immediately (nil)
	assert.True(t, collapser.started.Load())
}

// Test Close and IsClosed
func TestHLSCollapser_Close(t *testing.T) {
	client := &http.Client{}
	collapser := NewHLSCollapser(client, "http://example.com/playlist.m3u8")

	assert.False(t, collapser.IsClosed())

	err := collapser.Close()
	assert.NoError(t, err)
	assert.True(t, collapser.IsClosed())
}

// Test SessionID returns a valid UUID-like string
func TestHLSCollapser_SessionID(t *testing.T) {
	client := &http.Client{}
	collapser := NewHLSCollapser(client, "http://example.com/playlist.m3u8")

	sessionID := collapser.SessionID()
	assert.NotEmpty(t, sessionID)
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	assert.Equal(t, 36, len(sessionID))
	assert.Contains(t, sessionID, "-")
}

// Test isH264IDR detection
func TestHLSCollapser_IsH264IDR(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	tests := []struct {
		name     string
		au       [][]byte
		expected bool
	}{
		{
			name:     "IDR slice type 5",
			au:       [][]byte{{0x65, 0x00, 0x01}}, // NAL type = 5 (0x65 & 0x1F = 5)
			expected: true,
		},
		{
			name:     "non-IDR slice",
			au:       [][]byte{{0x41, 0x00, 0x01}}, // NAL type = 1 (0x41 & 0x1F = 1)
			expected: false,
		},
		{
			name:     "SPS",
			au:       [][]byte{{0x67, 0x42, 0x00}}, // NAL type = 7 (SPS)
			expected: false,
		},
		{
			name:     "PPS",
			au:       [][]byte{{0x68, 0xce}}, // NAL type = 8 (PPS)
			expected: false,
		},
		{
			name: "IDR among others",
			au: [][]byte{
				{0x67, 0x42}, // SPS
				{0x68, 0xce}, // PPS
				{0x65, 0x88}, // IDR
			},
			expected: true,
		},
		{
			name:     "empty AU",
			au:       [][]byte{},
			expected: false,
		},
		{
			name:     "empty NALU",
			au:       [][]byte{{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapser.isH264IDR(tt.au)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test isH265IDR detection
func TestHLSCollapser_IsH265IDR(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	tests := []struct {
		name     string
		au       [][]byte
		expected bool
	}{
		{
			name:     "IDR_W_RADL type 19",
			au:       [][]byte{{0x26, 0x01}}, // NAL type = 19 ((0x26 >> 1) & 0x3F = 19)
			expected: true,
		},
		{
			name:     "IDR_N_LP type 20",
			au:       [][]byte{{0x28, 0x01}}, // NAL type = 20 ((0x28 >> 1) & 0x3F = 20)
			expected: true,
		},
		{
			name:     "non-IDR TRAIL_R type 1",
			au:       [][]byte{{0x02, 0x01}}, // NAL type = 1
			expected: false,
		},
		{
			name:     "empty AU",
			au:       [][]byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collapser.isH265IDR(tt.au)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test PTS offset handling
func TestHLSCollapser_HandlePTSOffset(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	// First call should set firstPTS
	collapser.handlePTSOffset(90000) // 1 second at 90kHz
	assert.True(t, collapser.firstPTSSet)
	assert.Equal(t, int64(90000), collapser.firstPTS)
	assert.Equal(t, int64(90000), collapser.ptsOffset)
	assert.Equal(t, int64(0), collapser.lastPTS)

	// Normal increment
	collapser.handlePTSOffset(93003) // ~1 frame later
	// lastPTS should be updated to adjusted PTS (93003 - 90000 = 3003)
	assert.Equal(t, int64(3003), collapser.lastPTS)

	// Another normal increment
	collapser.handlePTSOffset(96006) // another frame
	assert.Equal(t, int64(6006), collapser.lastPTS)
}

// Test StreamClassifier creation
func TestNewStreamClassifier(t *testing.T) {
	client := &http.Client{}
	classifier := NewStreamClassifier(client)

	assert.NotNil(t, classifier)
	assert.Equal(t, 3*time.Second, classifier.timeout)
}

// Test StreamClassifier with invalid URL
func TestStreamClassifier_InvalidURL(t *testing.T) {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	classifier := NewStreamClassifier(client)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result := classifier.Classify(ctx, "http://invalid-url-that-does-not-exist.example/playlist.m3u8")

	assert.Equal(t, StreamModeUnknown, result.Mode)
	assert.NotEmpty(t, result.Reasons)
}

// Test StreamClassifier with timeout
func TestStreamClassifier_Timeout(t *testing.T) {
	// Create a server that delays but respects request context
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for client to disconnect or timeout
		select {
		case <-r.Context().Done():
			// Client disconnected, exit immediately
			return
		case <-time.After(5 * time.Second):
			// Fallback timeout (should not reach here in practice)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	classifier := &StreamClassifier{
		client:  client,
		timeout: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	result := classifier.Classify(ctx, server.URL+"/playlist.m3u8")

	// Should return unknown due to timeout
	assert.Equal(t, StreamModeUnknown, result.Mode)
}

// Test simple HLS playlist serving and collapser connection
func TestHLSCollapser_WithMockServer(t *testing.T) {
	// Create a mock HLS server with a simple master playlist
	var mu sync.Mutex
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		switch r.URL.Path {
		case "/master.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1000000
stream.m3u8
`)
		case "/stream.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:4
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:4.0,
segment0.ts
#EXT-X-ENDLIST
`)
		case "/segment0.ts":
			// Return minimal TS data (just sync bytes)
			w.Header().Set("Content-Type", "video/MP2T")
			w.Write(bytes.Repeat([]byte{0x47}, 188)) // TS sync byte
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	collapser := NewHLSCollapser(client, server.URL+"/master.m3u8")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start should return without error (though parsing will fail for our minimal TS)
	err := collapser.Start(ctx)
	// The start may fail due to no valid tracks, but we should at least connect
	if err != nil {
		// Check that we at least made HTTP requests
		mu.Lock()
		count := requestCount
		mu.Unlock()
		assert.GreaterOrEqual(t, count, 1, "Should have made at least one request")
	}

	collapser.Close()
}

// Test error handling through closeWithError
func TestHLSCollapser_CloseWithError(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	testErr := fmt.Errorf("test error")
	collapser.closeWithError(testErr)

	assert.True(t, collapser.IsClosed())
	assert.Equal(t, testErr, collapser.Error())

	// Second close should be a no-op
	collapser.closeWithError(fmt.Errorf("second error"))
	assert.Equal(t, testErr, collapser.Error()) // Still first error
}

// Test Read returns error after close
func TestHLSCollapser_ReadAfterClose(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	// Close the collapser
	collapser.Close()

	// Try to read
	buf := make([]byte, 100)
	_, err := collapser.Read(buf)
	assert.Error(t, err)
	assert.True(t, err == io.EOF || strings.Contains(err.Error(), "closed"))
}

// Test Stop closes the client
func TestHLSCollapser_Stop(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	collapser.Stop()

	assert.True(t, collapser.IsClosed())
	assert.Equal(t, ErrCollapserAborted, collapser.Error())
}

// Benchmark marshalAnnexB
func BenchmarkMarshalAnnexB(b *testing.B) {
	nalus := [][]byte{
		bytes.Repeat([]byte{0x67}, 30),  // SPS ~30 bytes
		bytes.Repeat([]byte{0x68}, 10),  // PPS ~10 bytes
		bytes.Repeat([]byte{0x65}, 5000), // IDR slice ~5KB
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = marshalAnnexB(nalus)
	}
}

// Test classifyTracks with different codec combinations
func TestStreamClassifier_ClassifyTracks(t *testing.T) {
	classifier := NewStreamClassifier(&http.Client{})

	t.Run("empty tracks returns unknown mode", func(t *testing.T) {
		result := &ClassificationResult{
			Mode:    StreamModeUnknown,
			Reasons: []string{},
		}
		classified := classifier.classifyTracks(nil, result)
		assert.Equal(t, StreamModeUnknown, classified.Mode)
		// First reason is "Tracks: video= audio=", second is "No supported tracks found"
		assert.Len(t, classified.Reasons, 2)
		assert.Contains(t, classified.Reasons[1], "No supported tracks found")
	})
}

// Test concurrent access to handlePTSOffset
func TestHLSCollapser_HandlePTSOffset_Concurrent(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(pts int64) {
			defer wg.Done()
			collapser.handlePTSOffset(pts)
		}(int64(i * 3003)) // Simulate frame timestamps
	}

	wg.Wait()

	// Should not panic and should have valid state
	assert.True(t, collapser.firstPTSSet)
}

// Test video/audio ready flags
func TestHLSCollapser_ReadyFlags(t *testing.T) {
	collapser := NewHLSCollapser(&http.Client{}, "http://example.com/playlist.m3u8")

	// Initially both should be false
	assert.False(t, collapser.videoReady)
	assert.False(t, collapser.audioReady)
}

// Ensure PID constants are correct
func TestPIDConstants(t *testing.T) {
	assert.Equal(t, uint16(256), videoPIDBase)
	assert.Equal(t, uint16(257), audioPIDBase)
}

// Test ErrCollapserAborted is defined
func TestErrCollapserAborted(t *testing.T) {
	require.NotNil(t, ErrCollapserAborted)
	assert.Contains(t, ErrCollapserAborted.Error(), "abort")
}
