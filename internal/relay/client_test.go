package relay

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewBufferClient(t *testing.T) {
	userAgent := "TestAgent/1.0"
	remoteAddr := "192.168.1.100"

	client := NewBufferClient(userAgent, remoteAddr)

	if client == nil {
		t.Fatal("NewBufferClient returned nil")
	}

	if client.UserAgent != userAgent {
		t.Errorf("expected UserAgent %s, got %s", userAgent, client.UserAgent)
	}

	if client.RemoteAddr != remoteAddr {
		t.Errorf("expected RemoteAddr %s, got %s", remoteAddr, client.RemoteAddr)
	}

	if client.ID.String() == "" {
		t.Error("client should have a valid UUID")
	}

	if client.ConnectedAt.IsZero() {
		t.Error("ConnectedAt should be set")
	}

	if client.GetBytesRead() != 0 {
		t.Errorf("new client should have 0 bytes read, got %d", client.GetBytesRead())
	}

	if client.GetLastSequence() != 0 {
		t.Errorf("new client should have sequence 0, got %d", client.GetLastSequence())
	}
}

func TestBufferClient_SequenceTracking(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Initial sequence should be 0
	if seq := client.GetLastSequence(); seq != 0 {
		t.Errorf("expected initial sequence 0, got %d", seq)
	}

	// Set sequence
	client.SetLastSequence(42)
	if seq := client.GetLastSequence(); seq != 42 {
		t.Errorf("expected sequence 42, got %d", seq)
	}

	// Set to a higher value
	client.SetLastSequence(100)
	if seq := client.GetLastSequence(); seq != 100 {
		t.Errorf("expected sequence 100, got %d", seq)
	}

	// Can also set to lower (no enforcement of monotonicity in client)
	client.SetLastSequence(50)
	if seq := client.GetLastSequence(); seq != 50 {
		t.Errorf("expected sequence 50, got %d", seq)
	}
}

func TestBufferClient_BytesTracking(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Initial bytes should be 0
	if bytes := client.GetBytesRead(); bytes != 0 {
		t.Errorf("expected initial bytes 0, got %d", bytes)
	}

	// Add bytes
	client.AddBytesRead(1000)
	if bytes := client.GetBytesRead(); bytes != 1000 {
		t.Errorf("expected 1000 bytes, got %d", bytes)
	}

	// Add more bytes
	client.AddBytesRead(500)
	if bytes := client.GetBytesRead(); bytes != 1500 {
		t.Errorf("expected 1500 bytes, got %d", bytes)
	}
}

func TestBufferClient_LastReadTracking(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Get initial time
	initialTime := client.GetLastReadTime()
	if initialTime.IsZero() {
		t.Error("lastRead should be initialized")
	}

	// Wait a bit and update
	time.Sleep(10 * time.Millisecond)
	client.UpdateLastRead()

	newTime := client.GetLastReadTime()
	if !newTime.After(initialTime) {
		t.Error("lastRead should be updated to a later time")
	}
}

func TestBufferClient_IsStale(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Fresh client should not be stale
	if client.IsStale(time.Second) {
		t.Error("fresh client should not be stale")
	}

	// With very short timeout, should be stale after sleep
	time.Sleep(15 * time.Millisecond)
	if !client.IsStale(10 * time.Millisecond) {
		t.Error("client should be stale with 10ms timeout after 15ms")
	}

	// Update last read and check again
	client.UpdateLastRead()
	if client.IsStale(time.Second) {
		t.Error("client should not be stale after UpdateLastRead")
	}
}

func TestBufferClient_NotifyAndWait(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Notify before wait - notification should be pending
	client.Notify()

	// Wait should return immediately since notification is pending
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- client.Wait(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Wait should have returned immediately")
	}
}

func TestBufferClient_WaitWithContextCancellation(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- client.Wait(ctx)
	}()

	// Give the goroutine time to start waiting
	time.Sleep(10 * time.Millisecond)

	// Cancel the context
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Wait should have returned after context cancellation")
	}
}

func TestBufferClient_WaitWithTimeout(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := client.Wait(ctx)
	elapsed := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	if elapsed < 40*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("Wait should have taken ~50ms, took %v", elapsed)
	}
}

func TestBufferClient_NotifyWakesWaiter(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- client.Wait(ctx)
	}()

	// Give the goroutine time to start waiting
	time.Sleep(20 * time.Millisecond)

	// Notify should wake up the waiter
	client.Notify()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Notify should have woken up Wait")
	}
}

func TestBufferClient_MultipleNotifications(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	// Multiple notifications should not block
	for i := 0; i < 10; i++ {
		client.Notify()
	}

	// Should still have exactly one pending notification
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// First wait should succeed
	if err := client.Wait(ctx); err != nil {
		t.Errorf("first Wait failed: %v", err)
	}

	// Second wait should block (no more pending notifications)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()

	err := client.Wait(ctx2)
	if err != context.DeadlineExceeded {
		t.Errorf("expected second Wait to timeout, got %v", err)
	}
}

func TestBufferClient_ConcurrentOperations(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	var wg sync.WaitGroup
	const goroutines = 10

	// Concurrent sequence updates
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				client.SetLastSequence(uint64(id*100 + j))
				_ = client.GetLastSequence()
			}
		}(i)
	}
	wg.Wait()

	// Concurrent bytes tracking
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				client.AddBytesRead(10)
				_ = client.GetBytesRead()
			}
		}()
	}
	wg.Wait()

	// Should have 10 * 100 * 10 = 10000 bytes
	expectedBytes := uint64(goroutines * 100 * 10)
	if bytes := client.GetBytesRead(); bytes != expectedBytes {
		t.Errorf("expected %d bytes, got %d", expectedBytes, bytes)
	}

	// Concurrent lastRead updates
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				client.UpdateLastRead()
				_ = client.GetLastReadTime()
				_ = client.IsStale(time.Second)
			}
		}()
	}
	wg.Wait()

	// Concurrent notify
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				client.Notify()
			}
		}()
	}
	wg.Wait()

	// Should not have deadlocked or panicked
}

func TestBufferClient_WaitWithMultipleWaiters(t *testing.T) {
	client := NewBufferClient("TestAgent", "127.0.0.1")

	const numWaiters = 5
	results := make(chan error, numWaiters)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Start multiple waiters
	for i := 0; i < numWaiters; i++ {
		go func() {
			results <- client.Wait(ctx)
		}()
	}

	// Give goroutines time to start
	time.Sleep(20 * time.Millisecond)

	// One notify
	client.Notify()

	// Only one waiter should wake up immediately
	select {
	case err := <-results:
		if err != nil {
			t.Errorf("first waiter failed: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("one waiter should have been woken")
	}

	// Send more notifications for remaining waiters
	for i := 0; i < numWaiters-1; i++ {
		client.Notify()
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("waiter %d failed: %v", i+2, err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("waiter %d should have been woken", i+2)
		}
	}
}

func TestBufferClient_ConnectedAt(t *testing.T) {
	before := time.Now()
	client := NewBufferClient("TestAgent", "127.0.0.1")
	after := time.Now()

	if client.ConnectedAt.Before(before) {
		t.Error("ConnectedAt should be after test start")
	}
	if client.ConnectedAt.After(after) {
		t.Error("ConnectedAt should be before now")
	}
}
