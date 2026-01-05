package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBandwidthTracker_NewWithDefaults(t *testing.T) {
	tracker := NewBandwidthTracker()

	assert.NotNil(t, tracker)
	assert.Equal(t, DefaultBandwidthWindowSize, tracker.WindowSize())
	assert.Equal(t, DefaultBandwidthSamplePeriod, tracker.SamplePeriod())
	assert.Equal(t, uint64(0), tracker.TotalBytes())
	assert.Equal(t, 0, tracker.SampleCount())
}

func TestBandwidthTracker_NewWithConfig(t *testing.T) {
	windowSize := 60
	samplePeriod := 500 * time.Millisecond

	tracker := NewBandwidthTrackerWithConfig(windowSize, samplePeriod)

	assert.NotNil(t, tracker)
	assert.Equal(t, windowSize, tracker.WindowSize())
	assert.Equal(t, samplePeriod, tracker.SamplePeriod())
}

func TestBandwidthTracker_NewWithInvalidConfig(t *testing.T) {
	// Should use defaults for invalid values
	tracker := NewBandwidthTrackerWithConfig(0, 0)

	assert.Equal(t, DefaultBandwidthWindowSize, tracker.WindowSize())
	assert.Equal(t, DefaultBandwidthSamplePeriod, tracker.SamplePeriod())
}

func TestBandwidthTracker_Add(t *testing.T) {
	tracker := NewBandwidthTracker()

	tracker.Add(100)
	assert.Equal(t, uint64(100), tracker.TotalBytes())

	tracker.Add(50)
	assert.Equal(t, uint64(150), tracker.TotalBytes())

	tracker.Add(0)
	assert.Equal(t, uint64(150), tracker.TotalBytes())
}

func TestBandwidthTracker_Sample(t *testing.T) {
	tracker := NewBandwidthTrackerWithConfig(5, time.Second)

	// Add some bytes
	tracker.Add(1000)
	tracker.Sample()

	assert.Equal(t, 1, tracker.SampleCount())

	// Add more bytes
	tracker.Add(2000)
	tracker.Sample()

	assert.Equal(t, 2, tracker.SampleCount())
}

func TestBandwidthTracker_SampleWindowLimit(t *testing.T) {
	windowSize := 3
	tracker := NewBandwidthTrackerWithConfig(windowSize, time.Second)

	// Add samples beyond window size
	for range 5 {
		tracker.Add(1000)
		tracker.Sample()
	}

	// Should be limited to window size
	assert.Equal(t, windowSize, tracker.SampleCount())
}

func TestBandwidthTracker_CurrentBps(t *testing.T) {
	tracker := NewBandwidthTrackerWithConfig(5, time.Second)

	// Empty tracker should return 0
	assert.Equal(t, uint64(0), tracker.CurrentBps())

	// Add 1000 bytes and sample
	tracker.Add(1000)
	tracker.Sample()

	// With one sample at 1 second period, should be 1000 bps
	bps := tracker.CurrentBps()
	assert.Equal(t, uint64(1000), bps)

	// Add another 2000 bytes and sample
	tracker.Add(2000)
	tracker.Sample()

	// Average of 1000 + 2000 = 3000 over 2 seconds = 1500 bps
	bps = tracker.CurrentBps()
	assert.Equal(t, uint64(1500), bps)
}

func TestBandwidthTracker_History(t *testing.T) {
	tracker := NewBandwidthTrackerWithConfig(5, time.Second)

	// Empty tracker should return nil
	assert.Nil(t, tracker.History())

	// Add samples
	tracker.Add(1000)
	tracker.Sample()
	tracker.Add(2000)
	tracker.Sample()
	tracker.Add(500)
	tracker.Sample()

	history := tracker.History()
	assert.Len(t, history, 3)

	// History should be in order: 1000, 2000, 500 (bytes per second)
	assert.Equal(t, uint64(1000), history[0])
	assert.Equal(t, uint64(2000), history[1])
	assert.Equal(t, uint64(500), history[2])
}

func TestBandwidthTracker_Reset(t *testing.T) {
	tracker := NewBandwidthTracker()

	// Add some data
	tracker.Add(1000)
	tracker.Sample()
	tracker.Add(2000)
	tracker.Sample()

	assert.Equal(t, uint64(3000), tracker.TotalBytes())
	assert.Equal(t, 2, tracker.SampleCount())

	// Reset
	tracker.Reset()

	assert.Equal(t, uint64(0), tracker.TotalBytes())
	assert.Equal(t, 0, tracker.SampleCount())
	assert.Equal(t, uint64(0), tracker.CurrentBps())
	assert.Nil(t, tracker.History())
}

func TestBandwidthTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewBandwidthTracker()
	done := make(chan bool)

	// Concurrent writes
	for range 10 {
		go func() {
			for range 100 {
				tracker.Add(100)
			}
			done <- true
		}()
	}

	// Concurrent reads
	for range 5 {
		go func() {
			for range 100 {
				_ = tracker.TotalBytes()
				_ = tracker.CurrentBps()
				_ = tracker.History()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for range 15 {
		<-done
	}

	// Should have recorded all bytes
	assert.Equal(t, uint64(100000), tracker.TotalBytes())
}

func TestBandwidthTracker_SubSecondSamplePeriod(t *testing.T) {
	tracker := NewBandwidthTrackerWithConfig(5, 100*time.Millisecond)

	// Add 100 bytes per 100ms sample = 1000 bytes per second
	tracker.Add(100)
	tracker.Sample()
	tracker.Add(100)
	tracker.Sample()
	tracker.Add(100)
	tracker.Sample()

	bps := tracker.CurrentBps()
	// Should be approximately 1000 bps (100 bytes / 0.1 seconds)
	assert.Equal(t, uint64(1000), bps)
}
