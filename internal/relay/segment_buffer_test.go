package relay

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSegmentBuffer(t *testing.T) {
	tests := []struct {
		name   string
		config SegmentBufferConfig
		want   SegmentBufferConfig
	}{
		{
			name:   "default config",
			config: DefaultSegmentBufferConfig(),
			want: SegmentBufferConfig{
				MaxSegments:    DefaultPlaylistSize,
				TargetDuration: DefaultSegmentDuration,
				MaxBufferSize:  DefaultMaxBufferSize,
			},
		},
		{
			name:   "custom config",
			config: SegmentBufferConfig{MaxSegments: 10, TargetDuration: 4, MaxBufferSize: 50 * 1024 * 1024},
			want:   SegmentBufferConfig{MaxSegments: 10, TargetDuration: 4, MaxBufferSize: 50 * 1024 * 1024},
		},
		{
			name:   "zero values use defaults",
			config: SegmentBufferConfig{},
			want: SegmentBufferConfig{
				MaxSegments:    DefaultPlaylistSize,
				TargetDuration: DefaultSegmentDuration,
				MaxBufferSize:  DefaultMaxBufferSize,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewSegmentBuffer(tt.config)
			if buf == nil {
				t.Fatal("NewSegmentBuffer returned nil")
			}
			if buf.config.MaxSegments != tt.want.MaxSegments {
				t.Errorf("MaxSegments = %d, want %d", buf.config.MaxSegments, tt.want.MaxSegments)
			}
			if buf.config.TargetDuration != tt.want.TargetDuration {
				t.Errorf("TargetDuration = %d, want %d", buf.config.TargetDuration, tt.want.TargetDuration)
			}
			if buf.config.MaxBufferSize != tt.want.MaxBufferSize {
				t.Errorf("MaxBufferSize = %d, want %d", buf.config.MaxBufferSize, tt.want.MaxBufferSize)
			}
		})
	}
}

func TestSegmentBuffer_AddSegment(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    3,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Add segments
	for i := 0; i < 5; i++ {
		seg := Segment{
			Duration: 6.0,
			Data:     make([]byte, 100),
		}
		if err := buf.AddSegment(seg); err != nil {
			t.Fatalf("AddSegment failed: %v", err)
		}
	}

	// Should only have 3 segments (max)
	segments := buf.GetSegments()
	if len(segments) != 3 {
		t.Errorf("got %d segments, want 3", len(segments))
	}

	// Sequences should be 3, 4, 5 (oldest evicted)
	if segments[0].Sequence != 3 {
		t.Errorf("first segment sequence = %d, want 3", segments[0].Sequence)
	}
	if segments[2].Sequence != 5 {
		t.Errorf("last segment sequence = %d, want 5", segments[2].Sequence)
	}
}

func TestSegmentBuffer_AddSegmentEmpty(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())

	seg := Segment{
		Duration: 6.0,
		Data:     nil, // Empty
	}
	err := buf.AddSegment(seg)
	if err != ErrSegmentEmpty {
		t.Errorf("expected ErrSegmentEmpty, got %v", err)
	}
}

func TestSegmentBuffer_GetSegment(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Add 3 segments
	for i := 0; i < 3; i++ {
		seg := Segment{
			Duration: 6.0,
			Data:     []byte{byte(i)},
		}
		if err := buf.AddSegment(seg); err != nil {
			t.Fatalf("AddSegment failed: %v", err)
		}
	}

	// Get existing segment
	seg, err := buf.GetSegment(2)
	if err != nil {
		t.Fatalf("GetSegment failed: %v", err)
	}
	if seg.Data[0] != 1 {
		t.Errorf("segment data = %d, want 1", seg.Data[0])
	}

	// Get non-existent segment (too old)
	_, err = buf.GetSegment(0)
	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound for seq 0, got %v", err)
	}

	// Get non-existent segment (too new)
	_, err = buf.GetSegment(10)
	if err != ErrSegmentNotFound {
		t.Errorf("expected ErrSegmentNotFound for seq 10, got %v", err)
	}
}

func TestSegmentBuffer_GetLatestSegments(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    10,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Add 5 segments
	for i := 0; i < 5; i++ {
		seg := Segment{
			Duration: 6.0,
			Data:     []byte{byte(i)},
		}
		if err := buf.AddSegment(seg); err != nil {
			t.Fatalf("AddSegment failed: %v", err)
		}
	}

	// Get latest 3
	segments := buf.GetLatestSegments(3)
	if len(segments) != 3 {
		t.Errorf("got %d segments, want 3", len(segments))
	}
	if segments[0].Sequence != 3 {
		t.Errorf("first segment sequence = %d, want 3", segments[0].Sequence)
	}

	// Request more than available
	segments = buf.GetLatestSegments(10)
	if len(segments) != 5 {
		t.Errorf("got %d segments, want 5", len(segments))
	}
}

func TestSegmentBuffer_FirstLastSequence(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Empty buffer
	_, ok := buf.FirstSequence()
	if ok {
		t.Error("FirstSequence should return false for empty buffer")
	}
	_, ok = buf.LastSequence()
	if ok {
		t.Error("LastSequence should return false for empty buffer")
	}

	// Add segments
	for i := 0; i < 3; i++ {
		seg := Segment{Duration: 6.0, Data: []byte{byte(i)}}
		buf.AddSegment(seg)
	}

	first, ok := buf.FirstSequence()
	if !ok || first != 1 {
		t.Errorf("FirstSequence = %d, %v, want 1, true", first, ok)
	}

	last, ok := buf.LastSequence()
	if !ok || last != 3 {
		t.Errorf("LastSequence = %d, %v, want 3, true", last, ok)
	}
}

func TestSegmentBuffer_Close(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())

	// Add a segment
	seg := Segment{Duration: 6.0, Data: []byte{1}}
	buf.AddSegment(seg)

	// Close
	buf.Close()

	if !buf.IsClosed() {
		t.Error("buffer should be closed")
	}

	// Adding to closed buffer should fail
	err := buf.AddSegment(seg)
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}

	// Getting from closed buffer should fail
	_, err = buf.GetSegment(1)
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}
}

func TestSegmentBuffer_Stats(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	// Add segments
	for i := 0; i < 3; i++ {
		seg := Segment{Duration: 6.0, Data: make([]byte, 100)}
		buf.AddSegment(seg)
	}

	stats := buf.Stats()
	if stats.SegmentCount != 3 {
		t.Errorf("SegmentCount = %d, want 3", stats.SegmentCount)
	}
	if stats.TotalBytes != 300 {
		t.Errorf("TotalBytes = %d, want 300", stats.TotalBytes)
	}
	if stats.CurrentSize != 300 {
		t.Errorf("CurrentSize = %d, want 300", stats.CurrentSize)
	}
	if stats.FirstSequence != 1 {
		t.Errorf("FirstSequence = %d, want 1", stats.FirstSequence)
	}
	if stats.LastSequence != 3 {
		t.Errorf("LastSequence = %d, want 3", stats.LastSequence)
	}
}

func TestSegmentBuffer_MaxBufferSize(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    100, // High limit
		TargetDuration: 6,
		MaxBufferSize:  250, // Low limit - should evict when exceeded
	})

	// Add segments that will exceed buffer size
	for i := 0; i < 5; i++ {
		seg := Segment{Duration: 6.0, Data: make([]byte, 100)}
		buf.AddSegment(seg)
	}

	stats := buf.Stats()
	// Should have evicted to stay under 250 bytes
	if stats.CurrentSize > 250 {
		t.Errorf("CurrentSize = %d, should be <= 250", stats.CurrentSize)
	}
}

func TestSegmentBuffer_ClientTracking(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())

	// Add client
	client, err := buf.AddClient("TestAgent/1.0", "192.168.1.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	if client.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %s, want TestAgent/1.0", client.UserAgent)
	}
	if client.RemoteAddr != "192.168.1.1" {
		t.Errorf("RemoteAddr = %s, want 192.168.1.1", client.RemoteAddr)
	}

	// Get client
	retrieved, err := buf.GetClient(client.ID)
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}
	if retrieved.ID != client.ID {
		t.Error("retrieved client ID doesn't match")
	}

	// Client count
	if buf.ClientCount() != 1 {
		t.Errorf("ClientCount = %d, want 1", buf.ClientCount())
	}

	// Update client stats
	client.SetLastSegment(5)
	client.AddBytesServed(1000)
	if client.LastSegment() != 5 {
		t.Errorf("LastSegment = %d, want 5", client.LastSegment())
	}
	if client.BytesServed() != 1000 {
		t.Errorf("BytesServed = %d, want 1000", client.BytesServed())
	}

	// Remove client
	if !buf.RemoveClient(client.ID) {
		t.Error("RemoveClient should return true")
	}
	if buf.ClientCount() != 0 {
		t.Errorf("ClientCount = %d, want 0", buf.ClientCount())
	}

	// Remove non-existent client
	if buf.RemoveClient(uuid.New()) {
		t.Error("RemoveClient should return false for non-existent client")
	}

	// Get non-existent client
	_, err = buf.GetClient(uuid.New())
	if err != ErrClientNotFound {
		t.Errorf("expected ErrClientNotFound, got %v", err)
	}
}

func TestSegmentBuffer_ClientOnClosedBuffer(t *testing.T) {
	buf := NewSegmentBuffer(DefaultSegmentBufferConfig())
	buf.Close()

	_, err := buf.AddClient("TestAgent/1.0", "192.168.1.1")
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}
}

func TestSegmentBuffer_ConcurrentAccess(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    100,
		TargetDuration: 6,
		MaxBufferSize:  1024 * 1024,
	})

	var wg sync.WaitGroup
	numWriters := 5
	numReaders := 10
	numOps := 100

	// Writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				seg := Segment{Duration: 6.0, Data: make([]byte, 100)}
				buf.AddSegment(seg)
			}
		}()
	}

	// Readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				buf.GetSegments()
				buf.Stats()
				first, _ := buf.FirstSequence()
				buf.GetSegment(first)
			}
		}()
	}

	// Client operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < numOps; j++ {
			client, err := buf.AddClient("TestAgent", "127.0.0.1")
			if err == nil {
				buf.RemoveClient(client.ID)
			}
		}
	}()

	wg.Wait()

	// Verify buffer is still consistent
	stats := buf.Stats()
	if stats.SegmentCount > 100 {
		t.Errorf("SegmentCount = %d, should be <= 100", stats.SegmentCount)
	}
}

func TestSegment_Methods(t *testing.T) {
	seg := Segment{
		Sequence:   1,
		Duration:   6.0,
		Data:       []byte{1, 2, 3, 4, 5},
		Timestamp:  time.Now(),
		IsKeyframe: true,
		PTS:        90000,
		DTS:        90000,
	}

	if seg.Size() != 5 {
		t.Errorf("Size = %d, want 5", seg.Size())
	}

	if seg.IsEmpty() {
		t.Error("segment should not be empty")
	}

	// Test clone
	clone := seg.Clone()
	if clone.Sequence != seg.Sequence {
		t.Error("clone sequence doesn't match")
	}
	if clone.Duration != seg.Duration {
		t.Error("clone duration doesn't match")
	}
	if len(clone.Data) != len(seg.Data) {
		t.Error("clone data length doesn't match")
	}

	// Modify clone data - original should be unchanged
	clone.Data[0] = 99
	if seg.Data[0] == 99 {
		t.Error("modifying clone affected original")
	}

	// Empty segment
	empty := Segment{}
	if !empty.IsEmpty() {
		t.Error("empty segment should be empty")
	}

	// Clone of empty segment
	emptyClone := empty.Clone()
	if emptyClone.Data != nil {
		t.Error("clone of empty segment should have nil data")
	}
}

func TestSegmentBuffer_TargetDuration(t *testing.T) {
	buf := NewSegmentBuffer(SegmentBufferConfig{
		MaxSegments:    5,
		TargetDuration: 8,
		MaxBufferSize:  1024 * 1024,
	})

	if buf.TargetDuration() != 8 {
		t.Errorf("TargetDuration = %d, want 8", buf.TargetDuration())
	}
}
