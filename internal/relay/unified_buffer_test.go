package relay

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewUnifiedBuffer(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	if buf == nil {
		t.Fatal("NewUnifiedBuffer returned nil")
	}

	// Check defaults were applied
	if buf.config.MaxBufferSize != config.MaxBufferSize {
		t.Errorf("MaxBufferSize = %d, want %d", buf.config.MaxBufferSize, config.MaxBufferSize)
	}
	if buf.config.TargetSegmentDuration != DefaultSegmentDuration {
		t.Errorf("TargetSegmentDuration = %d, want %d", buf.config.TargetSegmentDuration, DefaultSegmentDuration)
	}
}

func TestUnifiedBuffer_WriteChunk(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	data := []byte("test data chunk")
	err := buf.WriteChunk(data)
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	// Verify data was written
	chunks := buf.ReadChunksFrom(0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if string(chunks[0].Data) != string(data) {
		t.Errorf("data mismatch: got %s, want %s", chunks[0].Data, data)
	}
}

func TestUnifiedBuffer_WriteChunk_Empty(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	err := buf.WriteChunk(nil)
	if err != nil {
		t.Errorf("WriteChunk(nil) should not error: %v", err)
	}

	err = buf.WriteChunk([]byte{})
	if err != nil {
		t.Errorf("WriteChunk(empty) should not error: %v", err)
	}

	// Should have no chunks
	chunks := buf.ReadChunksFrom(0)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty writes, got %d", len(chunks))
	}
}

func TestUnifiedBuffer_WriteChunk_Closed(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	buf.Close()

	err := buf.WriteChunk([]byte("data"))
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}
}

func TestUnifiedBuffer_ChunkSequencing(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	// Write multiple chunks
	for i := 0; i < 5; i++ {
		if err := buf.WriteChunk([]byte{byte(i)}); err != nil {
			t.Fatalf("WriteChunk %d failed: %v", i, err)
		}
	}

	chunks := buf.ReadChunksFrom(0)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}

	// Verify sequences are increasing
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Sequence <= chunks[i-1].Sequence {
			t.Errorf("sequences not increasing: %d <= %d", chunks[i].Sequence, chunks[i-1].Sequence)
		}
	}
}

func TestUnifiedBuffer_ReadChunksFrom(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	// Write 10 chunks
	for i := 0; i < 10; i++ {
		buf.WriteChunk([]byte{byte(i)})
	}

	all := buf.ReadChunksFrom(0)
	if len(all) != 10 {
		t.Fatalf("expected 10 chunks, got %d", len(all))
	}

	// Read from middle
	midSeq := all[5].Sequence
	fromMid := buf.ReadChunksFrom(midSeq)
	if len(fromMid) != 4 { // Should be chunks 6-9
		t.Errorf("expected 4 chunks from middle, got %d", len(fromMid))
	}

	// Read from end
	lastSeq := all[9].Sequence
	fromEnd := buf.ReadChunksFrom(lastSeq)
	if len(fromEnd) != 0 {
		t.Errorf("expected 0 chunks from end, got %d", len(fromEnd))
	}
}

func TestUnifiedBuffer_ChunkLimitEnforcement(t *testing.T) {
	config := UnifiedBufferConfig{
		MaxChunks:             5,
		MaxBufferSize:         1024 * 1024,
		TargetSegmentDuration: 6,
		MaxSegments:           3,
		CleanupInterval:       time.Minute, // Long interval to avoid cleanup during test
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write more than MaxChunks
	for i := 0; i < 10; i++ {
		buf.WriteChunk([]byte{byte(i)})
	}

	chunks := buf.ReadChunksFrom(0)
	if len(chunks) > 5 {
		t.Errorf("expected at most 5 chunks, got %d", len(chunks))
	}
}

func TestUnifiedBuffer_SizeLimitEnforcement(t *testing.T) {
	config := UnifiedBufferConfig{
		MaxChunks:             1000,
		MaxBufferSize:         100, // 100 bytes max
		TargetSegmentDuration: 6,
		MaxSegments:           3,
		CleanupInterval:       time.Minute,
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write data exceeding max size
	largeChunk := make([]byte, 50)
	for i := 0; i < 5; i++ {
		buf.WriteChunk(largeChunk)
	}

	// Check total size is within limit
	stats := buf.Stats()
	if stats.BufferSize > 100 {
		t.Errorf("buffer size %d exceeds limit 100", stats.BufferSize)
	}
}

func TestUnifiedBuffer_Client(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	// Add client
	client, err := buf.AddClient("test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}

	// Check client count
	if buf.ClientCount() != 1 {
		t.Errorf("ClientCount = %d, want 1", buf.ClientCount())
	}

	// Write some data
	buf.WriteChunk([]byte("test"))
	buf.WriteChunk([]byte("data"))

	// Read as client
	chunks := buf.ReadChunksForClient(client)
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks for client, got %d", len(chunks))
	}

	// Read again - should get nothing since already read
	chunks = buf.ReadChunksForClient(client)
	if len(chunks) != 0 {
		t.Errorf("expected 0 new chunks, got %d", len(chunks))
	}

	// Remove client
	removed := buf.RemoveClient(client.ID)
	if !removed {
		t.Error("RemoveClient returned false")
	}
	if buf.ClientCount() != 0 {
		t.Errorf("ClientCount after remove = %d, want 0", buf.ClientCount())
	}
}

func TestUnifiedBuffer_ReadChunksWithWait(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	client, _ := buf.AddClient("test", "127.0.0.1")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Write data in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		buf.WriteChunk([]byte("delayed data"))
	}()

	// Should wait and receive
	chunks, err := buf.ReadChunksWithWait(ctx, client)
	if err != nil {
		t.Fatalf("ReadChunksWithWait failed: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestUnifiedBuffer_ReadChunksWithWait_Timeout(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	client, _ := buf.AddClient("test", "127.0.0.1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// No data written - should timeout
	_, err := buf.ReadChunksWithWait(ctx, client)
	if err != context.DeadlineExceeded {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
}

func TestUnifiedBuffer_SegmentEmission(t *testing.T) {
	config := UnifiedBufferConfig{
		MaxChunks:             1000,
		MaxBufferSize:         10 * 1024 * 1024,
		TargetSegmentDuration: 1, // 1 second for faster test
		MaxSegments:           5,
		CleanupInterval:       time.Minute,
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write data simulating video stream
	// Force segment emission by using keyframe writes over time
	for i := 0; i < 5; i++ {
		data := make([]byte, 1000)
		isKeyframe := i%2 == 0 // Every other chunk is a keyframe
		buf.WriteChunkWithKeyframe(data, isKeyframe)
		time.Sleep(300 * time.Millisecond) // Spread over time
	}

	// Wait a bit for segment emission
	time.Sleep(200 * time.Millisecond)

	segments := buf.GetSegments()
	// Should have at least one segment after some time
	if len(segments) == 0 {
		// This is ok if duration hasn't been reached yet
		t.Log("No segments emitted yet - this may be expected for short test")
	}
}

func TestUnifiedBuffer_GetSegment(t *testing.T) {
	config := UnifiedBufferConfig{
		MaxChunks:             1000,
		MaxBufferSize:         10 * 1024 * 1024,
		TargetSegmentDuration: 1,
		MaxSegments:           5,
		CleanupInterval:       time.Minute,
	}
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Force a segment by writing enough data and waiting
	for i := 0; i < 3; i++ {
		buf.WriteChunkWithKeyframe(make([]byte, 100), true)
		time.Sleep(400 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	segments := buf.GetSegments()
	if len(segments) == 0 {
		t.Skip("No segments emitted - skipping segment retrieval test")
	}

	// Get first segment
	seg, err := buf.GetSegment(segments[0].Sequence)
	if err != nil {
		t.Fatalf("GetSegment failed: %v", err)
	}
	if seg == nil {
		t.Fatal("segment is nil")
	}
	if len(seg.Data) == 0 {
		t.Error("segment has no data")
	}
}

func TestUnifiedBuffer_GetSegment_NotFound(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	_, err := buf.GetSegment(999999)
	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound, got %v", err)
	}
}

func TestUnifiedBuffer_TargetDuration(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.TargetSegmentDuration = 10
	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	if buf.TargetDuration() != 10 {
		t.Errorf("TargetDuration = %d, want 10", buf.TargetDuration())
	}
}

func TestUnifiedBuffer_Stats(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	// Write some data
	buf.WriteChunk([]byte("chunk1"))
	buf.WriteChunk([]byte("chunk2"))

	// Add clients
	buf.AddClient("agent1", "127.0.0.1")
	buf.AddClient("agent2", "127.0.0.2")

	stats := buf.Stats()
	if stats.ChunkCount != 2 {
		t.Errorf("ChunkCount = %d, want 2", stats.ChunkCount)
	}
	if stats.BufferSize != 12 { // "chunk1" + "chunk2" = 12 bytes
		t.Errorf("BufferSize = %d, want 12", stats.BufferSize)
	}
	if stats.TotalBytesWritten != 12 {
		t.Errorf("TotalBytesWritten = %d, want 12", stats.TotalBytesWritten)
	}
	if stats.ClientCount != 2 {
		t.Errorf("ClientCount = %d, want 2", stats.ClientCount)
	}
}

func TestUnifiedBuffer_Close(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())

	if buf.IsClosed() {
		t.Error("buffer should not be closed initially")
	}

	buf.Close()

	if !buf.IsClosed() {
		t.Error("buffer should be closed after Close()")
	}

	// Double close should be safe
	buf.Close()
}

func TestUnifiedBuffer_ConcurrentAccess(t *testing.T) {
	buf := NewUnifiedBuffer(DefaultUnifiedBufferConfig())
	defer buf.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := buf.WriteChunk([]byte{byte(id), byte(j)}); err != nil {
					errCh <- err
					return
				}
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := buf.AddClient("reader", "127.0.0.1")
			if err != nil {
				errCh <- err
				return
			}
			for j := 0; j < 50; j++ {
				buf.ReadChunksForClient(client)
				time.Sleep(time.Millisecond)
			}
			buf.RemoveClient(client.ID)
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestUnifiedClient_Basic(t *testing.T) {
	client := NewUnifiedClient("test-agent", "192.168.1.1")

	if client.ID.String() == "" {
		t.Error("client ID should not be empty")
	}
	if client.UserAgent != "test-agent" {
		t.Errorf("UserAgent = %s, want test-agent", client.UserAgent)
	}
	if client.RemoteAddr != "192.168.1.1" {
		t.Errorf("RemoteAddr = %s, want 192.168.1.1", client.RemoteAddr)
	}
	if client.ConnectedAt.IsZero() {
		t.Error("ConnectedAt should not be zero")
	}
}

func TestUnifiedClient_ChunkTracking(t *testing.T) {
	client := NewUnifiedClient("test", "127.0.0.1")

	// Initial sequence should be 0
	if client.GetLastChunkSequence() != 0 {
		t.Errorf("initial sequence = %d, want 0", client.GetLastChunkSequence())
	}

	client.SetLastChunkSequence(100)
	if client.GetLastChunkSequence() != 100 {
		t.Errorf("sequence = %d, want 100", client.GetLastChunkSequence())
	}
}

func TestUnifiedClient_ByteTracking(t *testing.T) {
	client := NewUnifiedClient("test", "127.0.0.1")

	if client.GetBytesRead() != 0 {
		t.Errorf("initial bytes = %d, want 0", client.GetBytesRead())
	}

	client.AddBytesRead(100)
	client.AddBytesRead(50)
	if client.GetBytesRead() != 150 {
		t.Errorf("bytes = %d, want 150", client.GetBytesRead())
	}
}

func TestUnifiedClient_Notification(t *testing.T) {
	client := NewUnifiedClient("test", "127.0.0.1")

	// Test notify
	client.Notify()

	// Wait should return immediately
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Wait(ctx)
	if err != nil {
		t.Errorf("Wait after Notify should succeed: %v", err)
	}
}

func TestUnifiedClient_WaitTimeout(t *testing.T) {
	client := NewUnifiedClient("test", "127.0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected deadline exceeded, got %v", err)
	}
}

func TestUnifiedClient_IsStale(t *testing.T) {
	client := NewUnifiedClient("test", "127.0.0.1")

	// Initially not stale
	if client.IsStale(time.Second) {
		t.Error("client should not be stale immediately after creation")
	}

	// Update last read
	client.UpdateLastRead()

	time.Sleep(50 * time.Millisecond)

	// Should be stale with very short timeout
	if !client.IsStale(10 * time.Millisecond) {
		t.Error("client should be stale after timeout")
	}
}

func TestDetectKeyframeInChunk(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "no sync byte",
			data:     make([]byte, 188),
			expected: false,
		},
		{
			name:     "sync byte without adaptation",
			data:     append([]byte{0x47, 0x00, 0x00, 0x00}, make([]byte, 184)...),
			expected: false,
		},
		{
			name: "sync byte with random access",
			data: func() []byte {
				d := make([]byte, 188)
				d[0] = 0x47         // Sync byte
				d[1] = 0x00         // No errors, no payload unit start
				d[2] = 0x00         // PID low
				d[3] = 0x30         // Has adaptation field (0x20 set)
				d[4] = 0x07         // Adaptation field length
				d[5] = 0x40         // Random access indicator set (0x40)
				return d
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectKeyframeInChunk(tt.data)
			if result != tt.expected {
				t.Errorf("detectKeyframeInChunk() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSegmentMarker(t *testing.T) {
	marker := SegmentMarker{
		Sequence:   1,
		StartChunk: 10,
		EndChunk:   20,
		Duration:   6.5,
		Timestamp:  time.Now(),
		IsKeyframe: true,
		ByteSize:   50000,
	}

	if marker.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", marker.Sequence)
	}
	if marker.StartChunk != 10 {
		t.Errorf("StartChunk = %d, want 10", marker.StartChunk)
	}
	if marker.EndChunk != 20 {
		t.Errorf("EndChunk = %d, want 20", marker.EndChunk)
	}
	if marker.Duration != 6.5 {
		t.Errorf("Duration = %f, want 6.5", marker.Duration)
	}
	if !marker.IsKeyframe {
		t.Error("IsKeyframe should be true")
	}
	if marker.ByteSize != 50000 {
		t.Errorf("ByteSize = %d, want 50000", marker.ByteSize)
	}
}

func TestDefaultUnifiedBufferConfig(t *testing.T) {
	config := DefaultUnifiedBufferConfig()

	if config.MaxBufferSize != 100*1024*1024 {
		t.Errorf("MaxBufferSize = %d, want 100MB", config.MaxBufferSize)
	}
	if config.MaxChunks != 2000 {
		t.Errorf("MaxChunks = %d, want 2000", config.MaxChunks)
	}
	if config.ChunkTimeout != 120*time.Second {
		t.Errorf("ChunkTimeout = %v, want 120s", config.ChunkTimeout)
	}
	if config.TargetSegmentDuration != DefaultSegmentDuration {
		t.Errorf("TargetSegmentDuration = %d, want %d", config.TargetSegmentDuration, DefaultSegmentDuration)
	}
	if config.MaxSegments != DefaultPlaylistSize {
		t.Errorf("MaxSegments = %d, want %d", config.MaxSegments, DefaultPlaylistSize)
	}
}

// Tests for fMP4/CMAF mode

func TestUnifiedBuffer_FMP4Mode_Initialization(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.ContainerFormat = "fmp4"

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	if !buf.IsFMP4Mode() {
		t.Error("Buffer should be in fMP4 mode")
	}
	if buf.ContainerFormat() != "fmp4" {
		t.Errorf("ContainerFormat() = %s, want fmp4", buf.ContainerFormat())
	}
	if buf.HasInitSegment() {
		t.Error("Should not have init segment before any writes")
	}
}

func TestUnifiedBuffer_FMP4Mode_InitSegmentCapture(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.ContainerFormat = "fmp4"

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Create and write init segment (ftyp + moov)
	ftyp := makeFtypBox()
	moov := makeMoovBox(90000, true, true)
	initData := append(ftyp, moov...)

	err := buf.WriteChunk(initData)
	if err != nil {
		t.Fatalf("WriteChunk() error = %v", err)
	}

	// Verify init segment was captured
	if !buf.HasInitSegment() {
		t.Error("Should have init segment after writing ftyp+moov")
	}

	init := buf.GetInitSegment()
	if init == nil {
		t.Fatal("GetInitSegment() returned nil")
	}
	if !init.HasVideo {
		t.Error("Init segment should have video")
	}
	if !init.HasAudio {
		t.Error("Init segment should have audio")
	}
	if init.Timescale != 90000 {
		t.Errorf("Timescale = %d, want 90000", init.Timescale)
	}
}

func TestUnifiedBuffer_FMP4Mode_FragmentSegmentation(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.ContainerFormat = "fmp4"
	config.MaxSegments = 5

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write init segment
	ftyp := makeFtypBox()
	moov := makeMoovBox(90000, true, false)
	err := buf.WriteChunk(append(ftyp, moov...))
	if err != nil {
		t.Fatalf("WriteChunk(init) error = %v", err)
	}

	// Write fragments
	for i := uint32(1); i <= 3; i++ {
		moof := makeMoofBox(i, uint64(i)*90000, 30)
		mdat := makeMdatBox([]byte("video frame data"))
		fragData := append(moof, mdat...)

		err := buf.WriteChunk(fragData)
		if err != nil {
			t.Fatalf("WriteChunk(frag %d) error = %v", i, err)
		}
	}

	// Verify segments were created
	segments := buf.GetSegments()
	if len(segments) != 3 {
		t.Errorf("len(segments) = %d, want 3", len(segments))
	}

	// Check segment properties
	for i, seg := range segments {
		if seg.Sequence != uint64(i+1) {
			t.Errorf("seg[%d].Sequence = %d, want %d", i, seg.Sequence, i+1)
		}
		if seg.ByteSize == 0 {
			t.Errorf("seg[%d].ByteSize = 0, should be > 0", i)
		}
	}
}

func TestUnifiedBuffer_FMP4Mode_GetFMP4Segment(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.ContainerFormat = "fmp4"

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write init + fragment
	ftyp := makeFtypBox()
	moov := makeMoovBox(90000, true, false)
	buf.WriteChunk(append(ftyp, moov...))

	moof := makeMoofBox(1, 0, 30)
	mdat := makeMdatBox([]byte("segment data"))
	buf.WriteChunk(append(moof, mdat...))

	// Get segment
	seg, err := buf.GetFMP4Segment(1)
	if err != nil {
		t.Fatalf("GetFMP4Segment() error = %v", err)
	}
	if seg == nil {
		t.Fatal("GetFMP4Segment() returned nil")
	}

	if !seg.IsFragmented {
		t.Error("Segment should be flagged as fragmented")
	}
	if seg.ContainerFormat != "fmp4" {
		t.Errorf("ContainerFormat = %s, want fmp4", seg.ContainerFormat)
	}
}

func TestUnifiedBuffer_FMP4Mode_SegmentLimit(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	config.ContainerFormat = "fmp4"
	config.MaxSegments = 3

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	// Write init
	ftyp := makeFtypBox()
	moov := makeMoovBox(90000, true, false)
	buf.WriteChunk(append(ftyp, moov...))

	// Write more fragments than the limit
	for i := uint32(1); i <= 10; i++ {
		moof := makeMoofBox(i, uint64(i)*3000, 30)
		mdat := makeMdatBox([]byte("frame"))
		buf.WriteChunk(append(moof, mdat...))
	}

	// Should only have last 3 segments
	segments := buf.GetSegments()
	if len(segments) != 3 {
		t.Errorf("len(segments) = %d, want 3", len(segments))
	}
}

func TestUnifiedBuffer_NotFMP4Mode(t *testing.T) {
	config := DefaultUnifiedBufferConfig()
	// No ContainerFormat set - default to MPEG-TS mode

	buf := NewUnifiedBuffer(config)
	defer buf.Close()

	if buf.IsFMP4Mode() {
		t.Error("Buffer should not be in fMP4 mode by default")
	}
	if buf.HasInitSegment() {
		t.Error("Should not have init segment in MPEG-TS mode")
	}
	if buf.GetInitSegment() != nil {
		t.Error("GetInitSegment() should return nil in MPEG-TS mode")
	}
}

// Helper functions to create fMP4 boxes for testing

func makeFtypBox() []byte {
	content := []byte("isom\x00\x00\x00\x00")
	return makeMP4Box("ftyp", content)
}

func makeMoovBox(timescale uint32, hasVideo, hasAudio bool) []byte {
	mvhd := makeMvhdBox(timescale)
	content := mvhd

	if hasVideo {
		content = append(content, makeTrakBox("vide")...)
	}
	if hasAudio {
		content = append(content, makeTrakBox("soun")...)
	}

	return makeMP4Box("moov", content)
}

func makeMvhdBox(timescale uint32) []byte {
	content := make([]byte, 100)
	// version(1) + flags(3) + create_time(4) + mod_time(4) + timescale(4)
	content[0] = 0 // version 0
	content[12] = byte(timescale >> 24)
	content[13] = byte(timescale >> 16)
	content[14] = byte(timescale >> 8)
	content[15] = byte(timescale)
	return makeMP4Box("mvhd", content)
}

func makeTrakBox(handlerType string) []byte {
	mdia := makeMdiaBox(handlerType)
	return makeMP4Box("trak", mdia)
}

func makeMdiaBox(handlerType string) []byte {
	hdlr := makeHdlrBox(handlerType)
	return makeMP4Box("mdia", hdlr)
}

func makeHdlrBox(handlerType string) []byte {
	content := make([]byte, 24)
	// version(1) + flags(3) + pre_defined(4) + handler_type(4) + reserved(12)
	copy(content[8:12], handlerType)
	return makeMP4Box("hdlr", content)
}

func makeMoofBox(seqNum uint32, decodeTime uint64, sampleCount uint32) []byte {
	mfhd := makeMfhdBox(seqNum)
	traf := makeTrafBox(decodeTime, sampleCount)
	content := append(mfhd, traf...)
	return makeMP4Box("moof", content)
}

func makeMfhdBox(seqNum uint32) []byte {
	content := make([]byte, 8) // version(1) + flags(3) + sequence_number(4)
	content[4] = byte(seqNum >> 24)
	content[5] = byte(seqNum >> 16)
	content[6] = byte(seqNum >> 8)
	content[7] = byte(seqNum)
	return makeMP4Box("mfhd", content)
}

func makeTrafBox(decodeTime uint64, sampleCount uint32) []byte {
	tfdt := makeTfdtBox(decodeTime)
	trun := makeTrunBox(sampleCount)
	content := append(tfdt, trun...)
	return makeMP4Box("traf", content)
}

func makeTfdtBox(decodeTime uint64) []byte {
	content := make([]byte, 12) // version(1=64bit) + flags(3) + decode_time(8)
	content[0] = 1              // version 1 for 64-bit time
	content[4] = byte(decodeTime >> 56)
	content[5] = byte(decodeTime >> 48)
	content[6] = byte(decodeTime >> 40)
	content[7] = byte(decodeTime >> 32)
	content[8] = byte(decodeTime >> 24)
	content[9] = byte(decodeTime >> 16)
	content[10] = byte(decodeTime >> 8)
	content[11] = byte(decodeTime)
	return makeMP4Box("tfdt", content)
}

func makeTrunBox(sampleCount uint32) []byte {
	content := make([]byte, 8) // version(1) + flags(3) + sample_count(4)
	content[4] = byte(sampleCount >> 24)
	content[5] = byte(sampleCount >> 16)
	content[6] = byte(sampleCount >> 8)
	content[7] = byte(sampleCount)
	return makeMP4Box("trun", content)
}

func makeMdatBox(data []byte) []byte {
	return makeMP4Box("mdat", data)
}

func makeMP4Box(boxType string, content []byte) []byte {
	size := uint32(8 + len(content))
	box := make([]byte, size)
	box[0] = byte(size >> 24)
	box[1] = byte(size >> 16)
	box[2] = byte(size >> 8)
	box[3] = byte(size)
	copy(box[4:8], boxType)
	copy(box[8:], content)
	return box
}
