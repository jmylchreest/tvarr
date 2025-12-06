package relay

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewCyclicBuffer(t *testing.T) {
	config := DefaultCyclicBufferConfig()
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	if cb == nil {
		t.Fatal("NewCyclicBuffer returned nil")
	}

	if cb.IsClosed() {
		t.Error("new buffer should not be closed")
	}

	if cb.ClientCount() != 0 {
		t.Errorf("new buffer should have 0 clients, got %d", cb.ClientCount())
	}

	stats := cb.Stats()
	if stats.TotalChunks != 0 {
		t.Errorf("new buffer should have 0 chunks, got %d", stats.TotalChunks)
	}
}

func TestCyclicBuffer_WriteChunk(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour, // Long interval to avoid cleanup during test
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Write some data
	data := []byte("test data chunk")
	err := cb.WriteChunk(data)
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	stats := cb.Stats()
	if stats.TotalChunks != 1 {
		t.Errorf("expected 1 chunk, got %d", stats.TotalChunks)
	}
	if stats.TotalBytesWritten != uint64(len(data)) {
		t.Errorf("expected %d bytes written, got %d", len(data), stats.TotalBytesWritten)
	}
	if stats.CurrentSequence != 1 {
		t.Errorf("expected sequence 1, got %d", stats.CurrentSequence)
	}
}

func TestCyclicBuffer_WriteEmptyChunk(t *testing.T) {
	config := DefaultCyclicBufferConfig()
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Empty chunks should be ignored
	err := cb.WriteChunk([]byte{})
	if err != nil {
		t.Fatalf("WriteChunk with empty data failed: %v", err)
	}

	stats := cb.Stats()
	if stats.TotalChunks != 0 {
		t.Errorf("empty chunk should not be added, got %d chunks", stats.TotalChunks)
	}
}

func TestCyclicBuffer_WriteToClosedBuffer(t *testing.T) {
	config := DefaultCyclicBufferConfig()
	cb := NewCyclicBuffer(config)
	cb.Close()

	err := cb.WriteChunk([]byte("test"))
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}
}

func TestCyclicBuffer_ClientManagement(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Add client
	client, err := cb.AddClient("TestAgent/1.0", "127.0.0.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("AddClient returned nil client")
	}

	if cb.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", cb.ClientCount())
	}

	// Get client
	retrieved, ok := cb.GetClient(client.ID)
	if !ok {
		t.Error("GetClient failed to find client")
	}
	if retrieved.ID != client.ID {
		t.Error("GetClient returned wrong client")
	}

	// Remove client
	removed := cb.RemoveClient(client.ID)
	if !removed {
		t.Error("RemoveClient should return true")
	}

	if cb.ClientCount() != 0 {
		t.Errorf("expected 0 clients after removal, got %d", cb.ClientCount())
	}

	// Remove non-existent client
	removed = cb.RemoveClient(client.ID)
	if removed {
		t.Error("RemoveClient should return false for non-existent client")
	}
}

func TestCyclicBuffer_AddClientToClosedBuffer(t *testing.T) {
	config := DefaultCyclicBufferConfig()
	cb := NewCyclicBuffer(config)
	cb.Close()

	_, err := cb.AddClient("TestAgent/1.0", "127.0.0.1")
	if err != ErrBufferClosed {
		t.Errorf("expected ErrBufferClosed, got %v", err)
	}
}

func TestCyclicBuffer_ReadChunksForClient(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Write data before adding client
	err := cb.WriteChunk([]byte("chunk1"))
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	// Add client - should start from current sequence
	client, err := cb.AddClient("TestAgent/1.0", "127.0.0.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	// Client should not see chunks written before it joined
	chunks := cb.ReadChunksForClient(client)
	if len(chunks) != 0 {
		t.Errorf("new client should not see old chunks, got %d", len(chunks))
	}

	// Write new data
	err = cb.WriteChunk([]byte("chunk2"))
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	// Now client should see the new chunk
	chunks = cb.ReadChunksForClient(client)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if string(chunks[0].Data) != "chunk2" {
		t.Errorf("expected 'chunk2', got '%s'", string(chunks[0].Data))
	}

	// Reading again should return no new chunks
	chunks = cb.ReadChunksForClient(client)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks on re-read, got %d", len(chunks))
	}
}

func TestCyclicBuffer_EnforceLimits_MaxChunks(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       3, // Only keep 3 chunks
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Write 5 chunks
	for i := 0; i < 5; i++ {
		err := cb.WriteChunk([]byte{byte(i)})
		if err != nil {
			t.Fatalf("WriteChunk failed: %v", err)
		}
	}

	stats := cb.Stats()
	if stats.TotalChunks != 3 {
		t.Errorf("expected 3 chunks (max), got %d", stats.TotalChunks)
	}
}

func TestCyclicBuffer_EnforceLimits_MaxSize(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   10, // Only 10 bytes max
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Write chunks that exceed buffer size
	err := cb.WriteChunk([]byte("12345")) // 5 bytes
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}
	err = cb.WriteChunk([]byte("67890")) // 5 bytes
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}
	err = cb.WriteChunk([]byte("abcde")) // 5 bytes - should trigger eviction
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	stats := cb.Stats()
	// Should have removed old chunks to stay within 10 bytes
	if stats.TotalBufferSize > 10 {
		t.Errorf("buffer size %d exceeds max %d", stats.TotalBufferSize, 10)
	}
}

func TestCyclicBuffer_ConcurrentWrites(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       1000,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	const numWriters = 10
	const chunksPerWriter = 100
	var wg sync.WaitGroup

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < chunksPerWriter; j++ {
				data := []byte{byte(writerID), byte(j)}
				if err := cb.WriteChunk(data); err != nil {
					t.Errorf("writer %d: WriteChunk failed: %v", writerID, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	stats := cb.Stats()
	expectedWrites := uint64(numWriters * chunksPerWriter)
	if stats.CurrentSequence != expectedWrites {
		t.Errorf("expected sequence %d, got %d", expectedWrites, stats.CurrentSequence)
	}
}

func TestCyclicBuffer_ConcurrentClientsAndWrites(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       1000,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	const numClients = 5
	const numWrites = 50
	var wg sync.WaitGroup

	// Start writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numWrites; i++ {
			if err := cb.WriteChunk([]byte{byte(i)}); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Start clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			client, err := cb.AddClient("TestAgent", "127.0.0.1")
			if err != nil {
				return
			}
			defer cb.RemoveClient(client.ID)

			// Read some chunks
			for j := 0; j < 10; j++ {
				cb.ReadChunksForClient(client)
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify no crashes or deadlocks occurred
	stats := cb.Stats()
	if stats.CurrentSequence == 0 {
		t.Error("expected some writes to complete")
	}
}

func TestStreamWriter(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	writer := NewStreamWriter(cb)

	data := []byte("test data via io.Writer")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, wrote %d", len(data), n)
	}

	stats := cb.Stats()
	if stats.TotalChunks != 1 {
		t.Errorf("expected 1 chunk, got %d", stats.TotalChunks)
	}
}

func TestStreamReader(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Add client first
	client, err := cb.AddClient("TestAgent", "127.0.0.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	reader := NewStreamReader(cb, client)

	// Write data after client is set up
	testData := []byte("hello world from stream reader test")
	if err := cb.WriteChunk(testData); err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	// Use a context with timeout for the read
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	buf := make([]byte, 1024)
	n, err := reader.ReadContext(ctx, buf)
	if err != nil {
		t.Fatalf("ReadContext failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("expected to read %d bytes, read %d", len(testData), n)
	}
	if string(buf[:n]) != string(testData) {
		t.Errorf("data mismatch: expected '%s', got '%s'", testData, buf[:n])
	}
}

func TestStreamReader_PartialReads(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	client, err := cb.AddClient("TestAgent", "127.0.0.1")
	if err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}

	reader := NewStreamReader(cb, client)

	// Write larger data
	testData := []byte("this is a longer piece of data that will require multiple reads")
	if err := cb.WriteChunk(testData); err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Read in small chunks
	var allData []byte
	buf := make([]byte, 10) // Small buffer
	for len(allData) < len(testData) {
		n, err := reader.ReadContext(ctx, buf)
		if err != nil {
			t.Fatalf("ReadContext failed: %v", err)
		}
		allData = append(allData, buf[:n]...)
	}

	if string(allData) != string(testData) {
		t.Errorf("data mismatch after partial reads")
	}
}

func TestCyclicBuffer_StatsWithClients(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: time.Hour,
	}
	cb := NewCyclicBuffer(config)
	defer cb.Close()

	// Add clients first (they only see data written after they join)
	client1, _ := cb.AddClient("Agent1", "192.168.1.1")
	client2, _ := cb.AddClient("Agent2", "192.168.1.2")

	// Write some data AFTER clients join
	cb.WriteChunk([]byte("chunk1"))
	cb.WriteChunk([]byte("chunk2"))
	cb.WriteChunk([]byte("chunk3"))

	// Have client1 read some data
	cb.ReadChunksForClient(client1)

	stats := cb.Stats()

	if stats.TotalChunks != 3 {
		t.Errorf("expected 3 chunks, got %d", stats.TotalChunks)
	}
	if stats.ClientCount != 2 {
		t.Errorf("expected 2 clients, got %d", stats.ClientCount)
	}
	if stats.CurrentSequence != 3 {
		t.Errorf("expected sequence 3, got %d", stats.CurrentSequence)
	}
	if len(stats.Clients) != 2 {
		t.Errorf("expected 2 client stats, got %d", len(stats.Clients))
	}

	// Verify client stats contain expected info
	found := false
	for _, cs := range stats.Clients {
		if cs.UserAgent == "Agent1" {
			found = true
			if cs.BytesRead == 0 {
				t.Error("client1 should have bytes read")
			}
		}
	}
	if !found {
		t.Error("client1 not found in stats")
	}

	// Cleanup
	cb.RemoveClient(client1.ID)
	cb.RemoveClient(client2.ID)
}

func TestCyclicBuffer_CloseWakesClients(t *testing.T) {
	config := CyclicBufferConfig{
		MaxBufferSize:   1024 * 1024,
		MaxChunks:       100,
		ChunkTimeout:    time.Minute,
		ClientTimeout:   time.Minute,
		CleanupInterval: 10 * time.Millisecond,
	}
	cb := NewCyclicBuffer(config)

	// Add a client
	client, _ := cb.AddClient("TestAgent", "127.0.0.1")

	// Start a read in a goroutine
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cb.ReadWithWait(ctx, client)
		close(done)
	}()

	// Give the goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Close should wake up waiting clients
	cb.Close()

	// Wait for goroutine to complete
	select {
	case <-done:
		// Good, the goroutine completed
	case <-time.After(time.Second):
		t.Error("Close did not wake up waiting client")
	}

	if !cb.IsClosed() {
		t.Error("buffer should be closed")
	}

	// Double close should be safe
	cb.Close()
}
