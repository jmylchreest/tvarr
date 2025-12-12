// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Errors for shared buffer operations.
var (
	ErrVariantNotFound = errors.New("codec variant not found")
	ErrVariantExists   = errors.New("codec variant already exists")
	// Note: ErrBufferClosed is defined in manager.go
	ErrNoSourceVariant = errors.New("no source variant available for transcoding")
)

// ESSample represents a single elementary stream sample (video NAL unit or audio frame).
type ESSample struct {
	PTS        int64     // Presentation timestamp (90kHz timescale)
	DTS        int64     // Decode timestamp (90kHz timescale, video only)
	Data       []byte    // Raw NAL unit or audio frame data
	IsKeyframe bool      // For video: IDR frame
	Sequence   uint64    // Monotonic sequence number for ordering
	Timestamp  time.Time // Wall clock time when sample was received
}

// ESTrack stores elementary stream samples for a single track (video or audio).
type ESTrack struct {
	codec    string // h264, h265, aac, ac3, mp3, etc.
	initData []byte // SPS/PPS for H.264, AudioSpecificConfig for AAC, etc.

	samples  []ESSample // Ring buffer of samples
	head     int        // Write position
	tail     int        // Read position (oldest sample)
	count    int        // Current number of samples
	capacity int        // Max samples in buffer

	lastSeq uint64 // Last sequence number assigned

	// Byte-based size tracking
	currentBytes uint64 // Current total bytes in buffer
	maxBytes     uint64 // Maximum bytes allowed (0 = unlimited, use sample count only)

	// Notification channel for new samples (non-blocking)
	notify chan struct{}

	mu sync.RWMutex
}

// DefaultMaxTrackBytes is the default maximum bytes per track (15MB per track = ~30MB per variant)
const DefaultMaxTrackBytes uint64 = 15 * 1024 * 1024

// NewESTrack creates a new elementary stream track with the specified capacity.
func NewESTrack(codec string, capacity int) *ESTrack {
	return NewESTrackWithMaxBytes(codec, capacity, DefaultMaxTrackBytes)
}

// NewESTrackWithMaxBytes creates a new elementary stream track with specified capacity and max bytes.
func NewESTrackWithMaxBytes(codec string, capacity int, maxBytes uint64) *ESTrack {
	if capacity <= 0 {
		capacity = 1000 // Default capacity
	}
	return &ESTrack{
		codec:    codec,
		samples:  make([]ESSample, capacity),
		capacity: capacity,
		maxBytes: maxBytes,
		notify:   make(chan struct{}, 1), // Buffered to avoid blocking writers
	}
}

// SetInitData sets the codec initialization data (SPS/PPS for H.264, etc.).
func (t *ESTrack) SetInitData(data []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.initData = make([]byte, len(data))
	copy(t.initData, data)
}

// GetInitData returns a copy of the codec initialization data.
func (t *ESTrack) GetInitData() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.initData == nil {
		return nil
	}
	data := make([]byte, len(t.initData))
	copy(data, t.initData)
	return data
}

// Codec returns the codec identifier for this track.
func (t *ESTrack) Codec() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.codec
}

// SetCodec sets the codec identifier for this track.
func (t *ESTrack) SetCodec(codec string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.codec = codec
}

// Write adds a new sample to the track.
func (t *ESTrack) Write(pts, dts int64, data []byte, isKeyframe bool) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastSeq++
	seq := t.lastSeq

	// Copy data to avoid holding references
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	sampleSize := uint64(len(dataCopy))

	sample := ESSample{
		PTS:        pts,
		DTS:        dts,
		Data:       dataCopy,
		IsKeyframe: isKeyframe,
		Sequence:   seq,
		Timestamp:  time.Now(),
	}

	// Check if we need to evict samples based on byte limit
	if t.maxBytes > 0 {
		// Evict oldest samples until we have room for the new sample
		for t.count > 0 && t.currentBytes+sampleSize > t.maxBytes {
			// Remove oldest sample
			oldSample := t.samples[t.tail]
			t.currentBytes -= uint64(len(oldSample.Data))
			t.samples[t.tail].Data = nil // Help GC
			t.tail = (t.tail + 1) % t.capacity
			t.count--
		}
	}

	// Write to ring buffer
	t.samples[t.head] = sample
	t.head = (t.head + 1) % t.capacity
	t.currentBytes += sampleSize

	if t.count < t.capacity {
		t.count++
	} else {
		// Buffer is full by sample count, advance tail (lose oldest sample)
		oldSample := t.samples[t.tail]
		t.currentBytes -= uint64(len(oldSample.Data))
		t.samples[t.tail].Data = nil // Help GC
		t.tail = (t.tail + 1) % t.capacity
	}

	// Notify waiters of new sample (non-blocking)
	select {
	case t.notify <- struct{}{}:
	default:
	}

	return seq
}

// NotifyChan returns a channel that receives notifications when new samples arrive.
func (t *ESTrack) NotifyChan() <-chan struct{} {
	return t.notify
}

// ReadFrom returns samples starting from the given sequence number.
// Returns up to maxSamples samples, or all available if maxSamples <= 0.
func (t *ESTrack) ReadFrom(afterSeq uint64, maxSamples int) []ESSample {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return nil
	}

	// Find starting position
	startIdx := -1
	for i := 0; i < t.count; i++ {
		idx := (t.tail + i) % t.capacity
		if t.samples[idx].Sequence > afterSeq {
			startIdx = i
			break
		}
	}

	if startIdx < 0 {
		return nil
	}

	// Collect samples
	available := t.count - startIdx
	if maxSamples > 0 && available > maxSamples {
		available = maxSamples
	}

	result := make([]ESSample, available)
	for i := 0; i < available; i++ {
		idx := (t.tail + startIdx + i) % t.capacity
		result[i] = t.samples[idx]
	}

	return result
}

// ReadFromKeyframe returns samples starting from the first keyframe after the given sequence.
// This is useful for clients joining mid-stream who need to start from a keyframe.
func (t *ESTrack) ReadFromKeyframe(afterSeq uint64, maxSamples int) []ESSample {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return nil
	}

	// Find first keyframe after afterSeq
	startIdx := -1
	for i := 0; i < t.count; i++ {
		idx := (t.tail + i) % t.capacity
		sample := t.samples[idx]
		if sample.Sequence > afterSeq && sample.IsKeyframe {
			startIdx = i
			break
		}
	}

	if startIdx < 0 {
		return nil
	}

	// Collect samples
	available := t.count - startIdx
	if maxSamples > 0 && available > maxSamples {
		available = maxSamples
	}

	result := make([]ESSample, available)
	for i := 0; i < available; i++ {
		idx := (t.tail + startIdx + i) % t.capacity
		result[i] = t.samples[idx]
	}

	return result
}

// LastSequence returns the sequence number of the most recent sample.
func (t *ESTrack) LastSequence() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastSeq
}

// FirstSequence returns the sequence number of the oldest sample in the buffer.
func (t *ESTrack) FirstSequence() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.count == 0 {
		return 0
	}
	return t.samples[t.tail].Sequence
}

// Count returns the number of samples currently in the track.
func (t *ESTrack) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count
}

// CurrentBytes returns the current total bytes in the track.
func (t *ESTrack) CurrentBytes() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentBytes
}

// MaxBytes returns the maximum bytes limit for the track.
func (t *ESTrack) MaxBytes() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.maxBytes
}

// ByteUtilization returns the percentage of max bytes used (0-100).
func (t *ESTrack) ByteUtilization() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.maxBytes == 0 {
		return 0
	}
	return float64(t.currentBytes) / float64(t.maxBytes) * 100
}

// CodecVariant identifies a specific video+audio codec combination.
// Format: "video/audio" e.g., "h264/aac", "vp9/opus", "hevc/ac3"
type CodecVariant string

// Common codec variants.
const (
	VariantH264AAC CodecVariant = "h264/aac"
	VariantH264AC3 CodecVariant = "h264/ac3"
	VariantH265AAC CodecVariant = "hevc/aac"
	VariantH265AC3 CodecVariant = "hevc/ac3"
	VariantVP9Opus CodecVariant = "vp9/opus"
	VariantAV1Opus CodecVariant = "av1/opus"
	VariantCopy    CodecVariant = "copy/copy" // Passthrough - use source codecs
)

// NewCodecVariant creates a CodecVariant from video and audio codec names.
// Codec names should be like "h264", "h265", "aac" - NOT encoder names like "libx265".
func NewCodecVariant(videoCodec, audioCodec string) CodecVariant {
	// Warn if encoder names are passed instead of codec names - this indicates a bug
	if IsEncoderName(videoCodec) {
		slog.Warn("NewCodecVariant called with encoder name instead of codec name",
			slog.String("video_codec", videoCodec),
			slog.String("expected", "codec name like h264, h265, vp9"),
			slog.String("caller", getSharedBufferCallerInfo()))
	}
	if IsEncoderName(audioCodec) {
		slog.Warn("NewCodecVariant called with encoder name instead of codec name",
			slog.String("audio_codec", audioCodec),
			slog.String("expected", "codec name like aac, opus, mp3"),
			slog.String("caller", getSharedBufferCallerInfo()))
	}
	return CodecVariant(fmt.Sprintf("%s/%s", videoCodec, audioCodec))
}

// getSharedBufferCallerInfo returns a string with caller information for debugging
func getSharedBufferCallerInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// VideoCodec returns the video codec part of the variant.
func (v CodecVariant) VideoCodec() string {
	for i, c := range v {
		if c == '/' {
			return string(v[:i])
		}
	}
	return string(v)
}

// AudioCodec returns the audio codec part of the variant.
func (v CodecVariant) AudioCodec() string {
	for i, c := range v {
		if c == '/' {
			return string(v[i+1:])
		}
	}
	return ""
}

// String returns the string representation of the variant.
func (v CodecVariant) String() string {
	return string(v)
}

// ESVariant holds the elementary stream tracks for a specific codec variant.
type ESVariant struct {
	variant    CodecVariant
	videoTrack *ESTrack
	audioTrack *ESTrack
	isSource   bool // True if this is the original source variant (from ingest)
	createdAt  time.Time
	lastAccess atomic.Value // time.Time - last time a processor read from this variant

	// Statistics
	bytesIngested atomic.Uint64
}

// NewESVariant creates a new elementary stream variant.
func NewESVariant(variant CodecVariant, videoCapacity, audioCapacity int, isSource bool) *ESVariant {
	return NewESVariantWithMaxBytes(variant, videoCapacity, audioCapacity, DefaultMaxTrackBytes, DefaultMaxTrackBytes, isSource)
}

// NewESVariantWithMaxBytes creates a new elementary stream variant with byte limits.
func NewESVariantWithMaxBytes(variant CodecVariant, videoCapacity, audioCapacity int, maxVideoBytes, maxAudioBytes uint64, isSource bool) *ESVariant {
	v := &ESVariant{
		variant:    variant,
		videoTrack: NewESTrackWithMaxBytes(variant.VideoCodec(), videoCapacity, maxVideoBytes),
		audioTrack: NewESTrackWithMaxBytes(variant.AudioCodec(), audioCapacity, maxAudioBytes),
		isSource:   isSource,
		createdAt:  time.Now(),
	}
	v.lastAccess.Store(time.Now())
	return v
}

// Variant returns the codec variant identifier.
func (v *ESVariant) Variant() CodecVariant {
	return v.variant
}

// VideoTrack returns the video elementary stream track.
func (v *ESVariant) VideoTrack() *ESTrack {
	return v.videoTrack
}

// AudioTrack returns the audio elementary stream track.
func (v *ESVariant) AudioTrack() *ESTrack {
	return v.audioTrack
}

// IsSource returns true if this is the original source variant.
func (v *ESVariant) IsSource() bool {
	return v.isSource
}

// WriteVideo writes a video sample to this variant.
func (v *ESVariant) WriteVideo(pts, dts int64, data []byte, isKeyframe bool) uint64 {
	v.bytesIngested.Add(uint64(len(data)))
	return v.videoTrack.Write(pts, dts, data, isKeyframe)
}

// WriteAudio writes an audio sample to this variant.
func (v *ESVariant) WriteAudio(pts int64, data []byte) uint64 {
	v.bytesIngested.Add(uint64(len(data)))
	return v.audioTrack.Write(pts, pts, data, false) // Audio has no keyframes
}

// RecordAccess updates the last access time.
func (v *ESVariant) RecordAccess() {
	v.lastAccess.Store(time.Now())
}

// LastAccess returns when this variant was last accessed.
func (v *ESVariant) LastAccess() time.Time {
	t, _ := v.lastAccess.Load().(time.Time)
	return t
}

// ESVariantStats contains statistics for a single variant.
type ESVariantStats struct {
	Variant         CodecVariant
	VideoCodec      string
	AudioCodec      string
	VideoSamples    int
	AudioSamples    int
	VideoInitData   bool
	AudioInitData   bool
	FirstVideoSeq   uint64
	LastVideoSeq    uint64
	FirstAudioSeq   uint64
	LastAudioSeq    uint64
	BytesIngested   uint64
	CurrentBytes    uint64  // Current bytes in buffer (resident)
	MaxBytes        uint64  // Maximum bytes allowed per variant
	ByteUtilization float64 // Percentage of max bytes used (0-100)
	IsSource        bool
	CreatedAt       time.Time
	LastAccess      time.Time
}

// Stats returns statistics for this variant.
func (v *ESVariant) Stats() ESVariantStats {
	videoCodec := v.videoTrack.Codec()
	audioCodec := v.audioTrack.Codec()

	// Use the original variant name - don't reconstruct from track codecs
	// The track codecs might be empty for transcoded variants since the codec
	// is set on creation from the variant name (e.g., "h265/aac")
	// If track codecs are empty, fall back to extracting from variant name
	if videoCodec == "" {
		videoCodec = v.variant.VideoCodec()
	}
	if audioCodec == "" {
		audioCodec = v.variant.AudioCodec()
	}

	// Calculate current bytes and max bytes
	currentBytes := v.videoTrack.CurrentBytes() + v.audioTrack.CurrentBytes()
	maxBytes := v.videoTrack.MaxBytes() + v.audioTrack.MaxBytes()
	var byteUtilization float64
	if maxBytes > 0 {
		byteUtilization = float64(currentBytes) / float64(maxBytes) * 100
	}

	return ESVariantStats{
		Variant:         v.variant,
		VideoCodec:      videoCodec,
		AudioCodec:      audioCodec,
		VideoSamples:    v.videoTrack.Count(),
		AudioSamples:    v.audioTrack.Count(),
		VideoInitData:   v.videoTrack.GetInitData() != nil,
		AudioInitData:   v.audioTrack.GetInitData() != nil,
		FirstVideoSeq:   v.videoTrack.FirstSequence(),
		LastVideoSeq:    v.videoTrack.LastSequence(),
		FirstAudioSeq:   v.audioTrack.FirstSequence(),
		LastAudioSeq:    v.audioTrack.LastSequence(),
		BytesIngested:   v.bytesIngested.Load(),
		CurrentBytes:    currentBytes,
		MaxBytes:        maxBytes,
		ByteUtilization: byteUtilization,
		IsSource:        v.isSource,
		CreatedAt:       v.createdAt,
		LastAccess:      v.LastAccess(),
	}
}

// ESBufferStats contains statistics about the shared elementary stream buffer.
type ESBufferStats struct {
	ChannelID      string
	VariantCount   int
	SourceVariant  CodecVariant
	Variants       []ESVariantStats
	ProcessorCount int
	TotalBytes     uint64
	Duration       time.Duration
}

// SharedESBufferConfig configures the shared ES buffer.
type SharedESBufferConfig struct {
	VideoCapacity   int    // Samples per variant video track
	AudioCapacity   int    // Samples per variant audio track
	MaxVideoBytes   uint64 // Maximum bytes per video track (0 = use default 15MB)
	MaxAudioBytes   uint64 // Maximum bytes per audio track (0 = use default 15MB)
	MaxVariantBytes uint64 // Maximum total bytes per variant (video + audio, 0 = use default 30MB)
	Logger          *slog.Logger
}

// DefaultMaxVariantBytes is the default maximum bytes per variant (30MB)
const DefaultMaxVariantBytes uint64 = 30 * 1024 * 1024

// DefaultSharedESBufferConfig returns sensible defaults.
func DefaultSharedESBufferConfig() SharedESBufferConfig {
	return SharedESBufferConfig{
		// With one sample per frame (not per NAL unit), these capacities provide:
		// Video: 3600 samples at 30fps = 2 minutes, at 60fps = 1 minute
		// Audio: 6000 samples at ~47fps (AAC) = ~2 minutes
		VideoCapacity:   3600,
		AudioCapacity:   6000,
		MaxVideoBytes:   DefaultMaxTrackBytes,   // 15MB per video track
		MaxAudioBytes:   DefaultMaxTrackBytes,   // 15MB per audio track
		MaxVariantBytes: DefaultMaxVariantBytes, // 30MB per variant total
		Logger:          slog.Default(),
	}
}

// SharedESBuffer stores elementary stream data for a channel with multi-variant support.
// Multiple codec variants can coexist, allowing different processors to read different formats.
type SharedESBuffer struct {
	channelID string
	proxyID   string
	config    SharedESBufferConfig

	// Multi-variant storage: map from codec variant to ES tracks
	variants      map[CodecVariant]*ESVariant
	sourceVariant CodecVariant // The original source codec variant
	variantsMu    sync.RWMutex

	// Source variant readiness signaling
	sourceReadyCh   chan struct{} // Closed when source variant is created
	sourceReadyOnce sync.Once     // Ensures sourceReadyCh is closed only once

	// Timing
	startTime time.Time

	// Processor tracking
	processors   map[string]struct{}
	processorsMu sync.RWMutex

	// Callback for requesting new variants (triggers FFmpeg transcoder)
	onVariantRequest func(source, target CodecVariant) error

	// Lifecycle
	closed   atomic.Bool
	closedCh chan struct{}
}

// NewSharedESBuffer creates a new shared elementary stream buffer.
func NewSharedESBuffer(channelID, proxyID string, config SharedESBufferConfig) *SharedESBuffer {
	if config.VideoCapacity <= 0 {
		config.VideoCapacity = DefaultSharedESBufferConfig().VideoCapacity
	}
	if config.AudioCapacity <= 0 {
		config.AudioCapacity = DefaultSharedESBufferConfig().AudioCapacity
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &SharedESBuffer{
		channelID:     channelID,
		proxyID:       proxyID,
		config:        config,
		variants:      make(map[CodecVariant]*ESVariant),
		sourceReadyCh: make(chan struct{}),
		startTime:     time.Now(),
		processors:    make(map[string]struct{}),
		closedCh:      make(chan struct{}),
	}
}

// ChannelID returns the channel ID this buffer is for.
func (b *SharedESBuffer) ChannelID() string {
	return b.channelID
}

// ProxyID returns the proxy ID this buffer is for.
func (b *SharedESBuffer) ProxyID() string {
	return b.proxyID
}

// SetVariantRequestCallback sets the callback for requesting new codec variants.
// This is called when a processor requests a variant that doesn't exist.
func (b *SharedESBuffer) SetVariantRequestCallback(cb func(source, target CodecVariant) error) {
	b.variantsMu.Lock()
	defer b.variantsMu.Unlock()
	b.onVariantRequest = cb
}

// CreateSourceVariant creates the source variant (from ingest).
// This should be called once when the source stream format is known.
// This signals any waiters that the source variant is ready.
func (b *SharedESBuffer) CreateSourceVariant(videoCodec, audioCodec string) *ESVariant {
	variant := NewCodecVariant(videoCodec, audioCodec)

	b.variantsMu.Lock()
	defer b.variantsMu.Unlock()

	// Create the source variant with configured byte limits
	maxVideoBytes := b.config.MaxVideoBytes
	if maxVideoBytes == 0 {
		maxVideoBytes = DefaultMaxTrackBytes
	}
	maxAudioBytes := b.config.MaxAudioBytes
	if maxAudioBytes == 0 {
		maxAudioBytes = DefaultMaxTrackBytes
	}
	v := NewESVariantWithMaxBytes(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVideoBytes, maxAudioBytes, true)
	b.variants[variant] = v
	b.sourceVariant = variant

	// Signal that source variant is ready
	b.sourceReadyOnce.Do(func() {
		close(b.sourceReadyCh)
	})

	b.config.Logger.Info("Created source variant",
		slog.String("channel_id", b.channelID),
		slog.String("variant", variant.String()))

	return v
}

// WaitSourceVariant blocks until the source variant is created or the context is canceled.
// Returns nil if source is ready, or the context error if canceled.
func (b *SharedESBuffer) WaitSourceVariant(ctx context.Context) error {
	select {
	case <-b.sourceReadyCh:
		return nil
	case <-b.closedCh:
		return ErrBufferClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsSourceReady returns true if the source variant has been created.
func (b *SharedESBuffer) IsSourceReady() bool {
	select {
	case <-b.sourceReadyCh:
		return true
	default:
		return false
	}
}

// GetSourceVariant returns the source variant (original codec from ingest).
func (b *SharedESBuffer) GetSourceVariant() *ESVariant {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()
	return b.variants[b.sourceVariant]
}

// SourceVariantKey returns the codec variant key for the source.
func (b *SharedESBuffer) SourceVariantKey() CodecVariant {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()
	return b.sourceVariant
}

// GetVariant returns a specific codec variant, or nil if it doesn't exist.
func (b *SharedESBuffer) GetVariant(variant CodecVariant) *ESVariant {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()
	v := b.variants[variant]
	if v != nil {
		v.RecordAccess()
	}
	return v
}

// GetOrCreateVariant returns an existing variant or creates a new one.
// If the variant doesn't exist and isn't the source, it triggers transcoding.
// For VariantCopy requests, this will wait for the source variant to be created.
func (b *SharedESBuffer) GetOrCreateVariant(variant CodecVariant) (*ESVariant, error) {
	return b.GetOrCreateVariantWithContext(context.Background(), variant)
}

// GetOrCreateVariantWithContext returns an existing variant or creates a new one.
// If the variant doesn't exist and isn't the source, it triggers transcoding.
// For VariantCopy requests, this will wait for the source variant to be created.
func (b *SharedESBuffer) GetOrCreateVariantWithContext(ctx context.Context, variant CodecVariant) (*ESVariant, error) {
	if b.closed.Load() {
		return nil, ErrBufferClosed
	}

	// Check if variant already exists
	b.variantsMu.RLock()
	v, exists := b.variants[variant]
	source := b.sourceVariant
	callback := b.onVariantRequest
	b.variantsMu.RUnlock()

	if exists {
		v.RecordAccess()
		return v, nil
	}

	// If requesting copy/copy variant, wait for source to be ready
	if variant == VariantCopy {
		if source == "" {
			// Source not ready yet, wait for it
			b.config.Logger.Debug("Waiting for source variant to be ready",
				slog.String("channel_id", b.channelID))
			if err := b.WaitSourceVariant(ctx); err != nil {
				return nil, fmt.Errorf("waiting for source variant: %w", err)
			}
			// Re-read source after waiting
			b.variantsMu.RLock()
			source = b.sourceVariant
			b.variantsMu.RUnlock()
		}
		return b.GetVariant(source), nil
	}

	// Check if source variant exists for non-copy requests
	if source == "" {
		return nil, ErrNoSourceVariant
	}

	// If variant matches source, return source
	if variant == source {
		return b.GetVariant(source), nil
	}

	// Create the new variant
	b.variantsMu.Lock()
	// Double-check after acquiring write lock
	if v, exists := b.variants[variant]; exists {
		b.variantsMu.Unlock()
		v.RecordAccess()
		return v, nil
	}

	maxVideoBytes := b.config.MaxVideoBytes
	if maxVideoBytes == 0 {
		maxVideoBytes = DefaultMaxTrackBytes
	}
	maxAudioBytes := b.config.MaxAudioBytes
	if maxAudioBytes == 0 {
		maxAudioBytes = DefaultMaxTrackBytes
	}
	v = NewESVariantWithMaxBytes(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVideoBytes, maxAudioBytes, false)
	b.variants[variant] = v
	b.variantsMu.Unlock()

	b.config.Logger.Info("Created new codec variant",
		slog.String("channel_id", b.channelID),
		slog.String("variant", variant.String()),
		slog.String("source", source.String()))

	// Trigger transcoding if callback is set
	if callback != nil {
		if err := callback(source, variant); err != nil {
			b.config.Logger.Error("Failed to start transcoder for variant",
				slog.String("variant", variant.String()),
				slog.String("error", err.Error()))
			// Don't remove the variant - let it exist but be empty until transcoder starts
		}
	}

	return v, nil
}

// ListVariants returns all available codec variants.
func (b *SharedESBuffer) ListVariants() []CodecVariant {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()

	variants := make([]CodecVariant, 0, len(b.variants))
	for v := range b.variants {
		variants = append(variants, v)
	}
	return variants
}

// HasVariant checks if a specific variant exists.
func (b *SharedESBuffer) HasVariant(variant CodecVariant) bool {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()
	_, exists := b.variants[variant]
	return exists
}

// RemoveVariant removes a non-source variant (for cleanup of unused transcoded variants).
func (b *SharedESBuffer) RemoveVariant(variant CodecVariant) error {
	b.variantsMu.Lock()
	defer b.variantsMu.Unlock()

	v, exists := b.variants[variant]
	if !exists {
		return ErrVariantNotFound
	}

	if v.isSource {
		return errors.New("cannot remove source variant")
	}

	delete(b.variants, variant)
	b.config.Logger.Info("Removed codec variant",
		slog.String("channel_id", b.channelID),
		slog.String("variant", variant.String()))

	return nil
}

// Legacy API compatibility - operates on source variant

// SetVideoCodec sets the video codec on the source variant.
// Creates source variant if it doesn't exist.
// Signals source readiness when the source variant is first created.
func (b *SharedESBuffer) SetVideoCodec(codec string, initData []byte) {
	b.variantsMu.Lock()
	source := b.variants[b.sourceVariant]
	created := false
	if source == nil {
		// Create a placeholder source variant - audio codec will be set later
		maxVideoBytes := b.config.MaxVideoBytes
		if maxVideoBytes == 0 {
			maxVideoBytes = DefaultMaxTrackBytes
		}
		maxAudioBytes := b.config.MaxAudioBytes
		if maxAudioBytes == 0 {
			maxAudioBytes = DefaultMaxTrackBytes
		}
		source = NewESVariantWithMaxBytes(NewCodecVariant(codec, ""), b.config.VideoCapacity, b.config.AudioCapacity, maxVideoBytes, maxAudioBytes, true)
		b.sourceVariant = source.variant
		b.variants[b.sourceVariant] = source
		created = true
	} else {
		source.videoTrack.SetCodec(codec)
	}
	b.variantsMu.Unlock()

	// Signal that source variant is ready (at least video detected)
	if created {
		b.sourceReadyOnce.Do(func() {
			close(b.sourceReadyCh)
			b.config.Logger.Info("Source variant ready (video detected)",
				slog.String("channel_id", b.channelID),
				slog.String("video_codec", codec))
		})
	}

	if initData != nil {
		source.videoTrack.SetInitData(initData)
	}
}

// SetAudioCodec sets the audio codec on the source variant.
// Creates source variant if it doesn't exist.
// Signals source readiness when the source variant is first created.
func (b *SharedESBuffer) SetAudioCodec(codec string, initData []byte) {
	b.variantsMu.Lock()
	source := b.variants[b.sourceVariant]
	created := false
	if source == nil {
		// Create a placeholder source variant - video codec will be set later
		maxVideoBytes := b.config.MaxVideoBytes
		if maxVideoBytes == 0 {
			maxVideoBytes = DefaultMaxTrackBytes
		}
		maxAudioBytes := b.config.MaxAudioBytes
		if maxAudioBytes == 0 {
			maxAudioBytes = DefaultMaxTrackBytes
		}
		source = NewESVariantWithMaxBytes(NewCodecVariant("", codec), b.config.VideoCapacity, b.config.AudioCapacity, maxVideoBytes, maxAudioBytes, true)
		b.sourceVariant = source.variant
		b.variants[b.sourceVariant] = source
		created = true
	} else {
		source.audioTrack.SetCodec(codec)
	}
	b.variantsMu.Unlock()

	// Signal that source variant is ready (at least audio detected)
	// This handles audio-only streams
	if created {
		b.sourceReadyOnce.Do(func() {
			close(b.sourceReadyCh)
			b.config.Logger.Info("Source variant ready (audio detected)",
				slog.String("channel_id", b.channelID),
				slog.String("audio_codec", codec))
		})
	}

	if initData != nil {
		source.audioTrack.SetInitData(initData)
	}
}

// WriteVideo writes a video sample to the source variant.
func (b *SharedESBuffer) WriteVideo(pts, dts int64, data []byte, isKeyframe bool) uint64 {
	source := b.GetSourceVariant()
	if source == nil {
		return 0
	}
	return source.WriteVideo(pts, dts, data, isKeyframe)
}

// WriteAudio writes an audio sample to the source variant.
func (b *SharedESBuffer) WriteAudio(pts int64, data []byte) uint64 {
	source := b.GetSourceVariant()
	if source == nil {
		return 0
	}
	return source.WriteAudio(pts, data)
}

// WriteVideoToVariant writes a video sample to a specific codec variant.
// If the variant doesn't exist, the sample is dropped.
func (b *SharedESBuffer) WriteVideoToVariant(variant CodecVariant, pts, dts int64, data []byte, isKeyframe bool) uint64 {
	v := b.GetVariant(variant)
	if v == nil {
		return 0
	}
	return v.WriteVideo(pts, dts, data, isKeyframe)
}

// WriteAudioToVariant writes an audio sample to a specific codec variant.
// If the variant doesn't exist, the sample is dropped.
func (b *SharedESBuffer) WriteAudioToVariant(variant CodecVariant, pts int64, data []byte) uint64 {
	v := b.GetVariant(variant)
	if v == nil {
		return 0
	}
	return v.WriteAudio(pts, data)
}

// CreateVariant creates a new codec variant if it doesn't exist.
// Returns the variant (existing or newly created) and an error if creation failed.
func (b *SharedESBuffer) CreateVariant(variant CodecVariant) (*ESVariant, error) {
	if b.closed.Load() {
		return nil, ErrBufferClosed
	}

	b.variantsMu.Lock()
	defer b.variantsMu.Unlock()

	// Check if variant already exists
	if v, exists := b.variants[variant]; exists {
		v.RecordAccess()
		return v, nil
	}

	// Create new variant with configured byte limits
	maxVideoBytes := b.config.MaxVideoBytes
	if maxVideoBytes == 0 {
		maxVideoBytes = DefaultMaxTrackBytes
	}
	maxAudioBytes := b.config.MaxAudioBytes
	if maxAudioBytes == 0 {
		maxAudioBytes = DefaultMaxTrackBytes
	}
	v := NewESVariantWithMaxBytes(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVideoBytes, maxAudioBytes, false)
	b.variants[variant] = v

	b.config.Logger.Info("Created codec variant",
		slog.String("channel_id", b.channelID),
		slog.String("variant", variant.String()))

	return v, nil
}

// VideoTrack returns the video track of the source variant.
func (b *SharedESBuffer) VideoTrack() *ESTrack {
	source := b.GetSourceVariant()
	if source == nil {
		return nil
	}
	return source.VideoTrack()
}

// AudioTrack returns the audio track of the source variant.
func (b *SharedESBuffer) AudioTrack() *ESTrack {
	source := b.GetSourceVariant()
	if source == nil {
		return nil
	}
	return source.AudioTrack()
}

// RegisterProcessor adds a processor to the buffer.
func (b *SharedESBuffer) RegisterProcessor(processorID string) {
	b.processorsMu.Lock()
	defer b.processorsMu.Unlock()
	b.processors[processorID] = struct{}{}
}

// UnregisterProcessor removes a processor from the buffer.
func (b *SharedESBuffer) UnregisterProcessor(processorID string) {
	b.processorsMu.Lock()
	defer b.processorsMu.Unlock()
	delete(b.processors, processorID)
}

// ProcessorCount returns the number of registered processors.
func (b *SharedESBuffer) ProcessorCount() int {
	b.processorsMu.RLock()
	defer b.processorsMu.RUnlock()
	return len(b.processors)
}

// HasProcessors returns true if any processors are registered.
func (b *SharedESBuffer) HasProcessors() bool {
	b.processorsMu.RLock()
	defer b.processorsMu.RUnlock()
	return len(b.processors) > 0
}

// Stats returns current buffer statistics.
func (b *SharedESBuffer) Stats() ESBufferStats {
	b.variantsMu.RLock()
	defer b.variantsMu.RUnlock()

	var totalBytes uint64
	variantStats := make([]ESVariantStats, 0, len(b.variants))
	for _, v := range b.variants {
		vs := v.Stats()
		variantStats = append(variantStats, vs)
		totalBytes += vs.BytesIngested
	}

	return ESBufferStats{
		ChannelID:      b.channelID,
		VariantCount:   len(b.variants),
		SourceVariant:  b.sourceVariant,
		Variants:       variantStats,
		ProcessorCount: b.ProcessorCount(),
		TotalBytes:     totalBytes,
		Duration:       time.Since(b.startTime),
	}
}

// Close closes the buffer and signals all readers.
func (b *SharedESBuffer) Close() {
	if b.closed.CompareAndSwap(false, true) {
		close(b.closedCh)
	}
}

// IsClosed returns true if the buffer has been closed.
func (b *SharedESBuffer) IsClosed() bool {
	return b.closed.Load()
}

// ClosedChan returns a channel that is closed when the buffer is closed.
func (b *SharedESBuffer) ClosedChan() <-chan struct{} {
	return b.closedCh
}

// Duration returns how long this buffer has been active.
func (b *SharedESBuffer) Duration() time.Duration {
	return time.Since(b.startTime)
}

// CleanupUnusedVariants removes transcoded variants that haven't been accessed recently.
func (b *SharedESBuffer) CleanupUnusedVariants(maxIdle time.Duration) int {
	b.variantsMu.Lock()
	defer b.variantsMu.Unlock()

	cutoff := time.Now().Add(-maxIdle)
	removed := 0

	for variant, v := range b.variants {
		// Never remove source variant
		if v.isSource {
			continue
		}

		if v.LastAccess().Before(cutoff) {
			delete(b.variants, variant)
			removed++
			b.config.Logger.Info("Cleaned up unused variant",
				slog.String("channel_id", b.channelID),
				slog.String("variant", variant.String()),
				slog.Duration("idle_time", time.Since(v.LastAccess())))
		}
	}

	return removed
}
