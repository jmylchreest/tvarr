// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultBandwidthWindowSize is the default number of samples to keep for rolling average.
	DefaultBandwidthWindowSize = 30

	// DefaultBandwidthSamplePeriod is the default sampling period.
	DefaultBandwidthSamplePeriod = time.Second
)

// bandwidthSample represents a single bandwidth measurement.
type bandwidthSample struct {
	bytes     uint64
	timestamp time.Time
}

// BandwidthTracker tracks bytes transferred and calculates rolling bandwidth.
// It maintains a sliding window of samples for real-time rate calculation.
type BandwidthTracker struct {
	totalBytes atomic.Uint64

	// Rolling window for real-time rate calculation
	mu           sync.RWMutex
	samples      []bandwidthSample
	windowSize   int
	samplePeriod time.Duration
	lastSample   time.Time
	lastBytes    uint64 // Bytes at last sample time
}

// NewBandwidthTracker creates a new bandwidth tracker with default settings.
func NewBandwidthTracker() *BandwidthTracker {
	return NewBandwidthTrackerWithConfig(DefaultBandwidthWindowSize, DefaultBandwidthSamplePeriod)
}

// NewBandwidthTrackerWithConfig creates a new bandwidth tracker with custom settings.
func NewBandwidthTrackerWithConfig(windowSize int, samplePeriod time.Duration) *BandwidthTracker {
	if windowSize <= 0 {
		windowSize = DefaultBandwidthWindowSize
	}
	if samplePeriod <= 0 {
		samplePeriod = DefaultBandwidthSamplePeriod
	}
	return &BandwidthTracker{
		samples:      make([]bandwidthSample, 0, windowSize),
		windowSize:   windowSize,
		samplePeriod: samplePeriod,
		lastSample:   time.Now(),
	}
}

// Add records bytes transferred.
func (t *BandwidthTracker) Add(bytes uint64) {
	t.totalBytes.Add(bytes)
}

// TotalBytes returns the cumulative bytes transferred.
func (t *BandwidthTracker) TotalBytes() uint64 {
	return t.totalBytes.Load()
}

// Sample records the current state for bandwidth calculation.
// This should be called periodically (e.g., once per second).
func (t *BandwidthTracker) Sample() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	currentBytes := t.totalBytes.Load()

	// Calculate bytes since last sample
	bytesDelta := currentBytes - t.lastBytes

	// Add new sample
	sample := bandwidthSample{
		bytes:     bytesDelta,
		timestamp: now,
	}
	t.samples = append(t.samples, sample)

	// Trim to window size
	if len(t.samples) > t.windowSize {
		t.samples = t.samples[len(t.samples)-t.windowSize:]
	}

	t.lastBytes = currentBytes
	t.lastSample = now
}

// CurrentBps returns the current bandwidth in bytes per second.
// This is calculated as a rolling average over the sample window.
func (t *BandwidthTracker) CurrentBps() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.samples) == 0 {
		return 0
	}

	// Sum all samples in the window
	var totalBytes uint64
	for _, s := range t.samples {
		totalBytes += s.bytes
	}

	// Calculate time span
	duration := time.Duration(len(t.samples)) * t.samplePeriod
	if duration == 0 {
		return 0
	}

	// Return bytes per second
	return uint64(float64(totalBytes) / duration.Seconds())
}

// History returns the bandwidth history for sparkline visualization.
// Returns up to windowSize values, each representing bytes per sample period.
func (t *BandwidthTracker) History() []uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.samples) == 0 {
		return nil
	}

	// Convert samples to bytes per second for each sample
	history := make([]uint64, len(t.samples))
	for i, s := range t.samples {
		// Each sample represents bytes transferred during one sample period
		history[i] = uint64(float64(s.bytes) / t.samplePeriod.Seconds())
	}

	return history
}

// Reset clears all tracking data.
func (t *BandwidthTracker) Reset() {
	t.totalBytes.Store(0)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.samples = t.samples[:0]
	t.lastBytes = 0
	t.lastSample = time.Now()
}

// WindowSize returns the configured window size.
func (t *BandwidthTracker) WindowSize() int {
	return t.windowSize
}

// SamplePeriod returns the configured sample period.
func (t *BandwidthTracker) SamplePeriod() time.Duration {
	return t.samplePeriod
}

// SampleCount returns the current number of samples in the window.
func (t *BandwidthTracker) SampleCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.samples)
}

// EdgeBandwidthTrackers holds bandwidth trackers for each edge in the relay pipeline.
type EdgeBandwidthTrackers struct {
	// Origin to buffer edge (bytes from upstream)
	OriginToBuffer *BandwidthTracker

	// Buffer to transcoder edge (ES samples sent to transcoder)
	BufferToTranscoder *BandwidthTracker

	// Transcoder to buffer edge (transcoded samples back to buffer)
	TranscoderToBuffer *BandwidthTracker

	// Buffer to processor edges (keyed by processor format, e.g., "hls", "mpegts")
	BufferToProcessor map[string]*BandwidthTracker
	bufferToProcessorMu sync.RWMutex

	// Processor to client edges (keyed by client ID)
	ProcessorToClient map[string]*BandwidthTracker
	processorToClientMu sync.RWMutex
}

// NewEdgeBandwidthTrackers creates a new set of edge bandwidth trackers.
func NewEdgeBandwidthTrackers() *EdgeBandwidthTrackers {
	return &EdgeBandwidthTrackers{
		OriginToBuffer:     NewBandwidthTracker(),
		BufferToTranscoder: NewBandwidthTracker(),
		TranscoderToBuffer: NewBandwidthTracker(),
		BufferToProcessor:  make(map[string]*BandwidthTracker),
		ProcessorToClient:  make(map[string]*BandwidthTracker),
	}
}

// GetOrCreateProcessorTracker returns the tracker for a processor, creating it if needed.
func (e *EdgeBandwidthTrackers) GetOrCreateProcessorTracker(format string) *BandwidthTracker {
	e.bufferToProcessorMu.RLock()
	tracker, ok := e.BufferToProcessor[format]
	e.bufferToProcessorMu.RUnlock()

	if ok {
		return tracker
	}

	e.bufferToProcessorMu.Lock()
	defer e.bufferToProcessorMu.Unlock()

	// Double-check after acquiring write lock
	if tracker, ok := e.BufferToProcessor[format]; ok {
		return tracker
	}

	tracker = NewBandwidthTracker()
	e.BufferToProcessor[format] = tracker
	return tracker
}

// GetOrCreateClientTracker returns the tracker for a client, creating it if needed.
func (e *EdgeBandwidthTrackers) GetOrCreateClientTracker(clientID string) *BandwidthTracker {
	e.processorToClientMu.RLock()
	tracker, ok := e.ProcessorToClient[clientID]
	e.processorToClientMu.RUnlock()

	if ok {
		return tracker
	}

	e.processorToClientMu.Lock()
	defer e.processorToClientMu.Unlock()

	// Double-check after acquiring write lock
	if tracker, ok := e.ProcessorToClient[clientID]; ok {
		return tracker
	}

	tracker = NewBandwidthTracker()
	e.ProcessorToClient[clientID] = tracker
	return tracker
}

// RemoveClientTracker removes a client tracker when the client disconnects.
func (e *EdgeBandwidthTrackers) RemoveClientTracker(clientID string) {
	e.processorToClientMu.Lock()
	defer e.processorToClientMu.Unlock()
	delete(e.ProcessorToClient, clientID)
}

// SampleAll takes samples from all trackers.
// This should be called periodically (e.g., once per second).
func (e *EdgeBandwidthTrackers) SampleAll() {
	e.OriginToBuffer.Sample()
	e.BufferToTranscoder.Sample()
	e.TranscoderToBuffer.Sample()

	e.bufferToProcessorMu.RLock()
	for _, tracker := range e.BufferToProcessor {
		tracker.Sample()
	}
	e.bufferToProcessorMu.RUnlock()

	e.processorToClientMu.RLock()
	for _, tracker := range e.ProcessorToClient {
		tracker.Sample()
	}
	e.processorToClientMu.RUnlock()
}

// EdgeBandwidthInfo contains bandwidth info for a single edge.
type EdgeBandwidthInfo struct {
	TotalBytes uint64   `json:"total_bytes"`
	CurrentBps uint64   `json:"current_bps"`
	History    []uint64 `json:"history,omitempty"`
}

// EdgeBandwidthStats contains bandwidth stats for all edges in the pipeline.
type EdgeBandwidthStats struct {
	OriginToBuffer     EdgeBandwidthInfo            `json:"origin_to_buffer"`
	BufferToTranscoder EdgeBandwidthInfo            `json:"buffer_to_transcoder,omitempty"`
	TranscoderToBuffer EdgeBandwidthInfo            `json:"transcoder_to_buffer,omitempty"`
	BufferToProcessor  map[string]EdgeBandwidthInfo `json:"buffer_to_processor,omitempty"`
	ProcessorToClient  map[string]EdgeBandwidthInfo `json:"processor_to_client,omitempty"`
}

// Stats returns the current edge bandwidth statistics.
func (e *EdgeBandwidthTrackers) Stats() *EdgeBandwidthStats {
	stats := &EdgeBandwidthStats{
		OriginToBuffer: EdgeBandwidthInfo{
			TotalBytes: e.OriginToBuffer.TotalBytes(),
			CurrentBps: e.OriginToBuffer.CurrentBps(),
			History:    e.OriginToBuffer.History(),
		},
	}

	// Only include transcoder stats if there's been activity
	if e.BufferToTranscoder.TotalBytes() > 0 {
		stats.BufferToTranscoder = EdgeBandwidthInfo{
			TotalBytes: e.BufferToTranscoder.TotalBytes(),
			CurrentBps: e.BufferToTranscoder.CurrentBps(),
			History:    e.BufferToTranscoder.History(),
		}
	}

	if e.TranscoderToBuffer.TotalBytes() > 0 {
		stats.TranscoderToBuffer = EdgeBandwidthInfo{
			TotalBytes: e.TranscoderToBuffer.TotalBytes(),
			CurrentBps: e.TranscoderToBuffer.CurrentBps(),
			History:    e.TranscoderToBuffer.History(),
		}
	}

	// Processor stats
	e.bufferToProcessorMu.RLock()
	if len(e.BufferToProcessor) > 0 {
		stats.BufferToProcessor = make(map[string]EdgeBandwidthInfo, len(e.BufferToProcessor))
		for format, tracker := range e.BufferToProcessor {
			stats.BufferToProcessor[format] = EdgeBandwidthInfo{
				TotalBytes: tracker.TotalBytes(),
				CurrentBps: tracker.CurrentBps(),
				History:    tracker.History(),
			}
		}
	}
	e.bufferToProcessorMu.RUnlock()

	// Client stats
	e.processorToClientMu.RLock()
	if len(e.ProcessorToClient) > 0 {
		stats.ProcessorToClient = make(map[string]EdgeBandwidthInfo, len(e.ProcessorToClient))
		for clientID, tracker := range e.ProcessorToClient {
			stats.ProcessorToClient[clientID] = EdgeBandwidthInfo{
				TotalBytes: tracker.TotalBytes(),
				CurrentBps: tracker.CurrentBps(),
				History:    tracker.History(),
			}
		}
	}
	e.processorToClientMu.RUnlock()

	return stats
}
