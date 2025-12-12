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
	capacity int        // Max samples in buffer (safety bound, not primary limit)

	lastSeq uint64 // Last sequence number assigned

	// Byte-based size tracking
	currentBytes uint64 // Current total bytes in buffer
	// Note: Per-track byte limits are deprecated - use variant-level limits instead

	// Time-based duration limit (in 90kHz PTS ticks)
	// Buffer will evict samples older than (newest PTS - maxDurationPTS)
	maxDurationPTS int64 // Maximum duration in PTS ticks (0 = unlimited)

	// Eviction tracking
	evictedSamples uint64 // Total number of samples evicted
	evictedBytes   uint64 // Total bytes evicted

	// Notification channel for new samples (non-blocking)
	notify chan struct{}

	mu sync.RWMutex
}

// DefaultMaxTrackBytes is the default maximum bytes per track (deprecated - use variant limits)
// Kept for backwards compatibility but not used for eviction decisions.
const DefaultMaxTrackBytes uint64 = 15 * 1024 * 1024

// DefaultMaxTrackDuration is the default maximum duration per track (2 minutes)
// This is stored in 90kHz PTS ticks for direct comparison with sample timestamps
const DefaultMaxTrackDuration = 2 * 60 * time.Second
const DefaultMaxTrackDurationPTS int64 = 2 * 60 * 90000 // 2 minutes in 90kHz ticks

// DefaultVideoCapacity is the max video samples (2 min at 60fps = 7200, add headroom)
const DefaultVideoCapacity = 8000

// DefaultAudioCapacity is the max audio samples (2 min at ~47fps AAC, add headroom)
const DefaultAudioCapacity = 8000

// NewESTrack creates a new elementary stream track with the specified capacity.
// Note: Per-track byte limits are no longer used - eviction is controlled at the variant level.
func NewESTrack(codec string, capacity int) *ESTrack {
	return NewESTrackWithDuration(codec, capacity, DefaultMaxTrackDurationPTS)
}

// NewESTrackWithMaxBytes creates a new elementary stream track with specified capacity and max bytes.
// Deprecated: Per-track byte limits are no longer used. Use variant-level limits instead.
func NewESTrackWithMaxBytes(codec string, capacity int, _ uint64) *ESTrack {
	return NewESTrackWithDuration(codec, capacity, DefaultMaxTrackDurationPTS)
}

// NewESTrackWithLimits creates a new elementary stream track.
// Deprecated: Per-track byte limits are no longer used. Use variant-level limits instead.
func NewESTrackWithLimits(codec string, capacity int, _ uint64, maxDurationPTS int64) *ESTrack {
	return NewESTrackWithDuration(codec, capacity, maxDurationPTS)
}

// NewESTrackWithDuration creates a new elementary stream track with specified capacity and max duration.
// The sample capacity is a safety bound - actual eviction is controlled at the variant level
// using byte limits and time limits with paired video/audio eviction.
func NewESTrackWithDuration(codec string, capacity int, maxDurationPTS int64) *ESTrack {
	if capacity <= 0 {
		capacity = 1000 // Default capacity
	}
	return &ESTrack{
		codec:          codec,
		samples:        make([]ESSample, capacity),
		capacity:       capacity,
		maxDurationPTS: maxDurationPTS,
		notify:         make(chan struct{}, 1), // Buffered to avoid blocking writers
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
// Note: Byte-based eviction is handled at the variant level for paired video/audio eviction.
// Track-level eviction only handles time limits and sample capacity overflow.
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

	// Write to ring buffer
	t.samples[t.head] = sample
	t.head = (t.head + 1) % t.capacity
	t.currentBytes += sampleSize

	if t.count < t.capacity {
		t.count++
	} else {
		// Buffer is full by sample count (safety bound), advance tail
		oldSample := t.samples[t.tail]
		evictedSize := uint64(len(oldSample.Data))
		t.currentBytes -= evictedSize
		t.evictedSamples++
		t.evictedBytes += evictedSize
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

// EvictOldestSample removes the oldest sample from the track.
// Returns the PTS of the evicted sample, the bytes freed, and whether eviction occurred.
// This is called by the variant for coordinated eviction.
func (t *ESTrack) EvictOldestSample() (pts int64, bytesFreed uint64, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return 0, 0, false
	}

	oldSample := t.samples[t.tail]
	evictedPTS := oldSample.PTS
	evictedSize := uint64(len(oldSample.Data))

	t.currentBytes -= evictedSize
	t.evictedSamples++
	t.evictedBytes += evictedSize
	t.samples[t.tail].Data = nil // Help GC
	t.tail = (t.tail + 1) % t.capacity
	t.count--

	return evictedPTS, evictedSize, true
}

// OldestPTS returns the PTS of the oldest sample in the track.
// Returns 0 if the track is empty.
func (t *ESTrack) OldestPTS() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.count == 0 {
		return 0
	}
	return t.samples[t.tail].PTS
}

// evictByPTS removes all samples with PTS older than the specified threshold.
// Used for time-based eviction. Must be called with appropriate locking.
func (t *ESTrack) evictByPTS(oldestAllowedPTS int64) (samplesEvicted int, bytesEvicted uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for t.count > 0 {
		oldSample := t.samples[t.tail]
		if oldSample.PTS >= oldestAllowedPTS {
			break
		}

		evictedSize := uint64(len(oldSample.Data))
		t.currentBytes -= evictedSize
		t.evictedSamples++
		t.evictedBytes += evictedSize
		t.samples[t.tail].Data = nil // Help GC
		t.tail = (t.tail + 1) % t.capacity
		t.count--

		samplesEvicted++
		bytesEvicted += evictedSize
	}

	return samplesEvicted, bytesEvicted
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

// MaxDuration returns the maximum duration limit for the track.
func (t *ESTrack) MaxDuration() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	// Convert from 90kHz PTS ticks to time.Duration
	return time.Duration(t.maxDurationPTS) * time.Second / 90000
}

// EvictionStats returns the total number of samples and bytes evicted from this track.
func (t *ESTrack) EvictionStats() (samples uint64, bytes uint64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.evictedSamples, t.evictedBytes
}

// BufferDuration returns the approximate duration of content in the buffer based on timestamps.
// Returns the time difference between the oldest and newest samples.
func (t *ESTrack) BufferDuration() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.count < 2 {
		return 0
	}

	// Get oldest and newest sample timestamps
	oldestIdx := t.tail
	newestIdx := (t.head - 1 + t.capacity) % t.capacity

	oldestPTS := t.samples[oldestIdx].PTS
	newestPTS := t.samples[newestIdx].PTS

	// PTS is in 90kHz timescale
	if newestPTS <= oldestPTS {
		return 0
	}
	ptsDiff := newestPTS - oldestPTS
	return time.Duration(ptsDiff) * time.Second / 90000
}

// IsAtCapacity returns true if the track is at sample capacity.
func (t *ESTrack) IsAtCapacity() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.count >= t.capacity
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

	// Variant-level byte limit (combined video + audio)
	maxBytes       uint64 // Maximum total bytes for this variant (0 = unlimited)
	maxDurationPTS int64  // Maximum duration in PTS ticks

	// Statistics
	bytesIngested atomic.Uint64

	// Mutex for coordinated eviction
	evictMu sync.Mutex
}

// NewESVariant creates a new elementary stream variant with default limits.
func NewESVariant(variant CodecVariant, videoCapacity, audioCapacity int, isSource bool) *ESVariant {
	return NewESVariantWithConfig(variant, videoCapacity, audioCapacity, DefaultMaxVariantBytes, DefaultMaxTrackDurationPTS, isSource)
}

// NewESVariantWithMaxBytes creates a new elementary stream variant with byte limits.
// Deprecated: Use NewESVariantWithConfig instead. The per-track byte limits are combined into a single variant limit.
func NewESVariantWithMaxBytes(variant CodecVariant, videoCapacity, audioCapacity int, maxVideoBytes, maxAudioBytes uint64, isSource bool) *ESVariant {
	// Combine per-track limits into variant-level limit
	maxVariantBytes := maxVideoBytes + maxAudioBytes
	return NewESVariantWithConfig(variant, videoCapacity, audioCapacity, maxVariantBytes, DefaultMaxTrackDurationPTS, isSource)
}

// NewESVariantWithLimits creates a new elementary stream variant.
// Deprecated: Use NewESVariantWithConfig instead. The per-track byte limits are combined into a single variant limit.
func NewESVariantWithLimits(variant CodecVariant, videoCapacity, audioCapacity int, maxVideoBytes, maxAudioBytes uint64, maxDurationPTS int64, isSource bool) *ESVariant {
	// Combine per-track limits into variant-level limit
	maxVariantBytes := maxVideoBytes + maxAudioBytes
	return NewESVariantWithConfig(variant, videoCapacity, audioCapacity, maxVariantBytes, maxDurationPTS, isSource)
}

// NewESVariantWithConfig creates a new elementary stream variant with specified configuration.
// The maxBytes limit applies to the combined video + audio data in the variant.
// When the limit is exceeded, video and audio samples are evicted together (paired eviction)
// to maintain A/V sync.
func NewESVariantWithConfig(variant CodecVariant, videoCapacity, audioCapacity int, maxBytes uint64, maxDurationPTS int64, isSource bool) *ESVariant {
	v := &ESVariant{
		variant:        variant,
		videoTrack:     NewESTrackWithDuration(variant.VideoCodec(), videoCapacity, maxDurationPTS),
		audioTrack:     NewESTrackWithDuration(variant.AudioCodec(), audioCapacity, maxDurationPTS),
		isSource:       isSource,
		createdAt:      time.Now(),
		maxBytes:       maxBytes,
		maxDurationPTS: maxDurationPTS,
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
// Performs paired eviction if the variant exceeds its byte limit.
func (v *ESVariant) WriteVideo(pts, dts int64, data []byte, isKeyframe bool) uint64 {
	// Perform paired eviction before writing
	v.evictIfNeeded(uint64(len(data)))

	v.bytesIngested.Add(uint64(len(data)))
	return v.videoTrack.Write(pts, dts, data, isKeyframe)
}

// WriteAudio writes an audio sample to this variant.
// Performs paired eviction if the variant exceeds its byte limit.
func (v *ESVariant) WriteAudio(pts int64, data []byte) uint64 {
	// Perform paired eviction before writing
	v.evictIfNeeded(uint64(len(data)))

	v.bytesIngested.Add(uint64(len(data)))
	return v.audioTrack.Write(pts, pts, data, false) // Audio has no keyframes
}

// CurrentBytes returns the current total bytes across both tracks.
func (v *ESVariant) CurrentBytes() uint64 {
	return v.videoTrack.CurrentBytes() + v.audioTrack.CurrentBytes()
}

// MaxBytes returns the maximum bytes limit for this variant.
func (v *ESVariant) MaxBytes() uint64 {
	return v.maxBytes
}

// evictIfNeeded performs paired video/audio eviction if the variant exceeds its byte limit.
// This evicts from whichever track has the older PTS, maintaining A/V sync.
func (v *ESVariant) evictIfNeeded(incomingBytes uint64) {
	if v.maxBytes == 0 {
		return // No byte limit
	}

	v.evictMu.Lock()
	defer v.evictMu.Unlock()

	// Keep evicting until we have room for the incoming data
	for v.CurrentBytes()+incomingBytes > v.maxBytes {
		// Get oldest PTS from each track
		videoPTS := v.videoTrack.OldestPTS()
		audioPTS := v.audioTrack.OldestPTS()

		// Evict from the track with the older sample
		// This keeps video and audio roughly in sync
		if videoPTS > 0 && (audioPTS == 0 || videoPTS <= audioPTS) {
			// Video is older or audio is empty - evict video
			_, _, ok := v.videoTrack.EvictOldestSample()
			if !ok {
				break // No more samples to evict
			}
		} else if audioPTS > 0 {
			// Audio is older - evict audio
			_, _, ok := v.audioTrack.EvictOldestSample()
			if !ok {
				break // No more samples to evict
			}
		} else {
			// Both tracks empty
			break
		}
	}
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

	// Eviction tracking
	IsEvicting       bool          // True if buffer is at capacity and evicting
	BufferDuration   time.Duration // Duration of content in buffer (based on PTS)
	EvictedSamples   uint64        // Total samples evicted since start
	EvictedBytes     uint64        // Total bytes evicted since start
	VideoEvictedSamp uint64        // Video samples evicted
	VideoEvictedByte uint64        // Video bytes evicted
	AudioEvictedSamp uint64        // Audio samples evicted
	AudioEvictedByte uint64        // Audio bytes evicted
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

	// Calculate current bytes and max bytes (using variant-level values)
	currentBytes := v.CurrentBytes()
	maxBytes := v.MaxBytes()
	var byteUtilization float64
	if maxBytes > 0 {
		byteUtilization = float64(currentBytes) / float64(maxBytes) * 100
	}

	// Get eviction stats from both tracks
	videoEvictedSamp, videoEvictedByte := v.videoTrack.EvictionStats()
	audioEvictedSamp, audioEvictedByte := v.audioTrack.EvictionStats()

	// Calculate buffer duration from video track (primary timing source)
	bufferDuration := v.videoTrack.BufferDuration()
	if bufferDuration == 0 {
		// Fall back to audio track if video has no samples
		bufferDuration = v.audioTrack.BufferDuration()
	}

	// Consider evicting if we're close to the byte limit (>95%)
	var isEvicting bool
	if maxBytes > 0 {
		isEvicting = float64(currentBytes)/float64(maxBytes) > 0.95
	} else {
		// No byte limit - check sample capacity
		isEvicting = v.videoTrack.IsAtCapacity() || v.audioTrack.IsAtCapacity()
	}

	return ESVariantStats{
		Variant:          v.variant,
		VideoCodec:       videoCodec,
		AudioCodec:       audioCodec,
		VideoSamples:     v.videoTrack.Count(),
		AudioSamples:     v.audioTrack.Count(),
		VideoInitData:    v.videoTrack.GetInitData() != nil,
		AudioInitData:    v.audioTrack.GetInitData() != nil,
		FirstVideoSeq:    v.videoTrack.FirstSequence(),
		LastVideoSeq:     v.videoTrack.LastSequence(),
		FirstAudioSeq:    v.audioTrack.FirstSequence(),
		LastAudioSeq:     v.audioTrack.LastSequence(),
		BytesIngested:    v.bytesIngested.Load(),
		CurrentBytes:     currentBytes,
		MaxBytes:         maxBytes,
		ByteUtilization:  byteUtilization,
		IsSource:         v.isSource,
		CreatedAt:        v.createdAt,
		LastAccess:       v.LastAccess(),
		IsEvicting:       isEvicting,
		BufferDuration:   bufferDuration,
		EvictedSamples:   videoEvictedSamp + audioEvictedSamp,
		EvictedBytes:     videoEvictedByte + audioEvictedByte,
		VideoEvictedSamp: videoEvictedSamp,
		VideoEvictedByte: videoEvictedByte,
		AudioEvictedSamp: audioEvictedSamp,
		AudioEvictedByte: audioEvictedByte,
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
	VideoCapacity   int           // Samples per variant video track (safety bound)
	AudioCapacity   int           // Samples per variant audio track (safety bound)
	MaxVariantBytes uint64        // Maximum total bytes per variant (combined video + audio, 0 = use default 30MB)
	MaxDuration     time.Duration // Maximum buffer duration (0 = use default 2 minutes)
	Logger          *slog.Logger

	// Deprecated: Per-track byte limits are no longer used. Use MaxVariantBytes instead.
	MaxVideoBytes uint64
	MaxAudioBytes uint64
}

// DefaultMaxVariantBytes is the default maximum bytes per variant (30MB)
const DefaultMaxVariantBytes uint64 = 30 * 1024 * 1024

// DurationToPTS converts a time.Duration to 90kHz PTS ticks.
func DurationToPTS(d time.Duration) int64 {
	return int64(d.Seconds() * 90000)
}

// getMaxDurationPTS returns the max duration in PTS ticks from config, or default if not set.
func (c SharedESBufferConfig) getMaxDurationPTS() int64 {
	if c.MaxDuration > 0 {
		return DurationToPTS(c.MaxDuration)
	}
	return DefaultMaxTrackDurationPTS
}

// getMaxVariantBytes returns the variant byte limit from config, or default if not set.
func (c SharedESBufferConfig) getMaxVariantBytes() uint64 {
	if c.MaxVariantBytes > 0 {
		return c.MaxVariantBytes
	}
	// Fall back to sum of deprecated per-track limits if set
	if c.MaxVideoBytes > 0 || c.MaxAudioBytes > 0 {
		return c.MaxVideoBytes + c.MaxAudioBytes
	}
	return DefaultMaxVariantBytes
}

// DefaultSharedESBufferConfig returns sensible defaults.
func DefaultSharedESBufferConfig() SharedESBufferConfig {
	return SharedESBufferConfig{
		// Sample capacity is a safety bound - actual eviction is controlled
		// by the variant byte limit with paired video/audio eviction.
		// 8000 samples supports 2+ minutes at 60fps.
		VideoCapacity:   DefaultVideoCapacity, // 8000 samples (~2+ min at 60fps)
		AudioCapacity:   DefaultAudioCapacity, // 8000 samples (~2+ min at ~47fps AAC)
		MaxVariantBytes: DefaultMaxVariantBytes, // 30MB per variant (combined video + audio)
		MaxDuration:     DefaultMaxTrackDuration, // 2 minutes max buffer duration
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

	// Create the source variant with configured limits
	maxVariantBytes := b.config.getMaxVariantBytes()
	maxDurationPTS := b.config.getMaxDurationPTS()
	v := NewESVariantWithConfig(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVariantBytes, maxDurationPTS, true)
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

	maxVariantBytes := b.config.getMaxVariantBytes()
	maxDurationPTS := b.config.getMaxDurationPTS()
	v = NewESVariantWithConfig(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVariantBytes, maxDurationPTS, false)
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
		maxVariantBytes := b.config.getMaxVariantBytes()
		maxDurationPTS := b.config.getMaxDurationPTS()
		source = NewESVariantWithConfig(NewCodecVariant(codec, ""), b.config.VideoCapacity, b.config.AudioCapacity, maxVariantBytes, maxDurationPTS, true)
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
		maxVariantBytes := b.config.getMaxVariantBytes()
		maxDurationPTS := b.config.getMaxDurationPTS()
		source = NewESVariantWithConfig(NewCodecVariant("", codec), b.config.VideoCapacity, b.config.AudioCapacity, maxVariantBytes, maxDurationPTS, true)
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

	// Create new variant with configured limits
	maxVariantBytes := b.config.getMaxVariantBytes()
	maxDurationPTS := b.config.getMaxDurationPTS()
	v := NewESVariantWithConfig(variant, b.config.VideoCapacity, b.config.AudioCapacity, maxVariantBytes, maxDurationPTS, false)
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
// This method is optimized to minimize lock hold time by copying variant pointers
// first, then releasing the variants lock before collecting stats from each variant.
// This prevents blocking other operations during stats collection.
func (b *SharedESBuffer) Stats() ESBufferStats {
	// Copy variant pointers and immutable fields while holding the lock briefly
	b.variantsMu.RLock()
	variantCount := len(b.variants)
	channelID := b.channelID
	sourceVariant := b.sourceVariant
	startTime := b.startTime
	variantList := make([]*ESVariant, 0, variantCount)
	for _, v := range b.variants {
		variantList = append(variantList, v)
	}
	b.variantsMu.RUnlock()

	// Collect stats from each variant WITHOUT holding the variants lock.
	// Each variant.Stats() call acquires its own locks internally.
	var totalBytes uint64
	variantStats := make([]ESVariantStats, 0, variantCount)
	for _, v := range variantList {
		vs := v.Stats()
		variantStats = append(variantStats, vs)
		totalBytes += vs.BytesIngested
	}

	// ProcessorCount() acquires its own lock - safe to call outside variantsMu
	return ESBufferStats{
		ChannelID:      channelID,
		VariantCount:   variantCount,
		SourceVariant:  sourceVariant,
		Variants:       variantStats,
		ProcessorCount: b.ProcessorCount(),
		TotalBytes:     totalBytes,
		Duration:       time.Since(startTime),
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
