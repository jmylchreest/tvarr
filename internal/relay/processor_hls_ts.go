// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// HLS-TS Processor errors.
var (
	ErrHLSTSProcessorClosed = errors.New("HLS-TS processor closed")
	ErrSegmentNotReady      = errors.New("segment not ready")
)

// HLSTSProcessorConfig configures the HLS-TS processor.
type HLSTSProcessorConfig struct {
	// TargetSegmentDuration is the target duration for each segment in seconds.
	TargetSegmentDuration float64

	// MaxSegments is the maximum number of segments to keep in the sliding window.
	// This is the buffer size for resilience - supports slow clients who've already started.
	MaxSegments int

	// PlaylistSegments is the number of segments to include in the playlist for new clients.
	// New clients will start from the latest segments (near live edge).
	// If 0, defaults to min(3, MaxSegments).
	// Should be <= MaxSegments.
	PlaylistSegments int

	// PlaylistType is the HLS playlist type (EVENT or VOD, empty for live).
	PlaylistType string

	// Logger for structured logging.
	Logger *slog.Logger
}

// DefaultHLSTSProcessorConfig returns sensible defaults.
func DefaultHLSTSProcessorConfig() HLSTSProcessorConfig {
	return HLSTSProcessorConfig{
		TargetSegmentDuration: 6.0,
		MaxSegments:           30, // Keep ~3 minutes of segments for slow clients
		PlaylistSegments:      5,  // Segments in playlist
		PlaylistType:          "", // Live
		Logger:                slog.Default(),
	}
}

// hlsTSSegment represents a single HLS segment.
type hlsTSSegment struct {
	sequence    uint64
	duration    float64 // Duration in seconds
	data        []byte  // MPEG-TS data
	ptsStart    int64   // Start PTS
	discontinue bool    // Discontinuity flag
	createdAt   time.Time
}

// HLSTSProcessor reads from a SharedESBuffer variant and produces HLS with MPEG-TS segments.
// It implements the Processor interface.
type HLSTSProcessor struct {
	*ESProcessorBase

	config HLSTSProcessorConfig

	// Segment management
	segments      []*hlsTSSegment
	segmentsMu    sync.RWMutex
	nextSequence  uint64
	segmentNotify chan struct{} // Notifies waiters when new segment is added

	// Persistent muxer - shared across all segments to maintain continuity counters
	muxer           *TSMuxer
	swappableWriter *SwappableWriter

	// Current segment accumulator
	currentSegment struct {
		buf       bytes.Buffer
		startPTS  int64
		hasVideo  bool
		hasAudio  bool
		startTime time.Time
	}

	// Video parameter helper - persists across segments to retain SPS/PPS
	videoParams *VideoParamHelper
}

// NewHLSTSProcessor creates a new HLS-TS processor.
func NewHLSTSProcessor(
	id string,
	esBuffer *SharedESBuffer,
	variant CodecVariant,
	config HLSTSProcessorConfig,
) *HLSTSProcessor {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	esBase := NewESProcessorBase(id, OutputFormatHLSTS, esBuffer, variant, ESProcessorConfig{
		Logger: config.Logger,
	})

	p := &HLSTSProcessor{
		ESProcessorBase: esBase,
		config:          config,
		segments:        make([]*hlsTSSegment, 0, config.MaxSegments),
		segmentNotify:   make(chan struct{}, 1),
		videoParams:     NewVideoParamHelper(), // Persists across segments
	}

	return p
}

// Start begins processing data from the shared buffer.
func (p *HLSTSProcessor) Start(ctx context.Context) error {
	// Initialize ES processor base (get variant, register consumer, detect codecs)
	esVariant, err := p.InitES(ctx)
	if err != nil {
		return fmt.Errorf("initializing ES processor: %w", err)
	}

	// Wait for audio codec detection if not already available
	p.WaitForAudioCodec()

	// Wait for AAC init data if using AAC
	p.WaitForAACInitData()

	// Initialize TS muxer for current segment
	p.initNewSegment()

	p.config.Logger.Debug("Starting HLS-TS processor",
		slog.String("id", p.id),
		slog.String("requested_variant", p.Variant().String()),
		slog.String("resolved_variant", esVariant.Variant().String()),
		slog.String("video_codec", p.ResolvedVideoCodec()),
		slog.String("audio_codec", p.ResolvedAudioCodec()),
		slog.Bool("has_aac_config", p.AACConfig() != nil))

	// Start processing loop
	p.WaitGroup().Add(1)
	go func() {
		defer p.WaitGroup().Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *HLSTSProcessor) Stop() {
	p.StopES()
}

// RegisterClient adds a client to receive output from this processor.
func (p *HLSTSProcessor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	_ = p.RegisterClientBase(clientID, w, r)
	return nil
}

// UnregisterClient removes a client.
func (p *HLSTSProcessor) UnregisterClient(clientID string) {
	p.UnregisterClientBase(clientID)
}

// WaitForSegments waits until at least minSegments are available or context is cancelled.
// This is used to ensure clients don't get empty playlists when first connecting.
func (p *HLSTSProcessor) WaitForSegments(ctx context.Context, minSegments int) error {
	for {
		p.segmentsMu.RLock()
		count := len(p.segments)
		p.segmentsMu.RUnlock()

		if count >= minSegments {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.Context().Done():
			return errors.New("processor stopped")
		case <-p.segmentNotify:
			// New segment added, check again
		}
	}
}

// SegmentCount returns the current number of segments.
func (p *HLSTSProcessor) SegmentCount() int {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()
	return len(p.segments)
}

// ServeManifest serves the HLS playlist.
// If no segments are available, it waits up to SegmentWaitTimeout for the first segment.
func (p *HLSTSProcessor) ServeManifest(w http.ResponseWriter, r *http.Request) error {
	// Wait for at least 1 segment to be available (timeout matching HTTP WriteTimeout)
	waitCtx, cancel := context.WithTimeout(r.Context(), SegmentWaitTimeout)
	defer cancel()

	if err := p.WaitForSegments(waitCtx, 1); err != nil {
		p.config.Logger.Debug("Timeout waiting for HLS segments",
			slog.String("id", p.id),
			slog.String("error", err.Error()))
		http.Error(w, "No segments available yet, please retry", http.StatusServiceUnavailable)
		return fmt.Errorf("waiting for segments: %w", err)
	}

	p.segmentsMu.RLock()
	allSegments := p.segments
	if len(allSegments) == 0 {
		p.segmentsMu.RUnlock()
		http.Error(w, "No segments available", http.StatusServiceUnavailable)
		return errors.New("no segments available")
	}

	// Determine how many segments to include in playlist (latest segments for near-live)
	playlistSize := p.config.PlaylistSegments
	if playlistSize <= 0 {
		playlistSize = 3
	}
	if playlistSize > len(allSegments) {
		playlistSize = len(allSegments)
	}

	// Get only the latest segments
	startIdx := len(allSegments) - playlistSize
	segments := make([]*hlsTSSegment, playlistSize)
	copy(segments, allSegments[startIdx:])
	p.segmentsMu.RUnlock()

	// Build playlist
	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", int(p.config.TargetSegmentDuration+0.5)))
	buf.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", segments[0].sequence))

	if p.config.PlaylistType != "" {
		buf.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%s\n", p.config.PlaylistType))
	}

	for _, seg := range segments {
		if seg.discontinue {
			buf.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		buf.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", seg.duration))
		buf.WriteString(fmt.Sprintf("segment%d.ts\n", seg.sequence))
	}

	// Add #EXT-X-ENDLIST if source stream has completed (VOD mode)
	// This signals to players that no more segments will be added
	if p.ESBuffer() != nil && p.ESBuffer().IsSourceCompleted() {
		buf.WriteString("#EXT-X-ENDLIST\n")
	}

	p.SetStreamHeaders(w)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, err := w.Write(buf.Bytes())
	return err
}

// ServeSegment serves a specific segment by name.
func (p *HLSTSProcessor) ServeSegment(w http.ResponseWriter, r *http.Request, segmentName string) error {
	// Parse segment sequence from name (e.g., "segment123.ts")
	var seq uint64
	_, err := fmt.Sscanf(segmentName, "segment%d.ts", &seq)
	if err != nil {
		http.Error(w, "Invalid segment name", http.StatusBadRequest)
		return fmt.Errorf("parsing segment name: %w", err)
	}

	p.segmentsMu.RLock()
	var segment *hlsTSSegment
	for _, s := range p.segments {
		if s.sequence == seq {
			segment = s
			break
		}
	}
	p.segmentsMu.RUnlock()

	if segment == nil {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return fmt.Errorf("segment %d not found", seq)
	}

	p.SetStreamHeaders(w)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(segment.data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Segments are immutable
	_, err = w.Write(segment.data)
	return err
}

// GetSegmentInfos implements SegmentProvider.
// Returns only the latest PlaylistSegments segments so new clients start near live edge.
func (p *HLSTSProcessor) GetSegmentInfos() []SegmentInfo {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()

	if len(p.segments) == 0 {
		return nil
	}

	// Determine how many segments to include in playlist
	playlistSize := p.config.PlaylistSegments
	if playlistSize <= 0 {
		playlistSize = 3 // Default to 3 segments for near-live playback
	}
	if playlistSize > len(p.segments) {
		playlistSize = len(p.segments)
	}

	// Return only the latest segments (from the end of the buffer)
	startIdx := len(p.segments) - playlistSize
	latestSegments := p.segments[startIdx:]

	infos := make([]SegmentInfo, len(latestSegments))
	for i, seg := range latestSegments {
		infos[i] = SegmentInfo{
			Sequence:  seg.sequence,
			Duration:  seg.duration,
			Timestamp: seg.createdAt,
		}
	}
	return infos
}

// GetSegment implements SegmentProvider.
func (p *HLSTSProcessor) GetSegment(sequence uint64) (*Segment, error) {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()

	for _, seg := range p.segments {
		if seg.sequence == sequence {
			return &Segment{
				Sequence:  seg.sequence,
				Duration:  seg.duration,
				Data:      seg.data,
				Timestamp: seg.createdAt,
			}, nil
		}
	}

	return nil, ErrSegmentNotFound
}

// TargetDuration implements SegmentProvider.
func (p *HLSTSProcessor) TargetDuration() int {
	return int(p.config.TargetSegmentDuration + 0.5)
}

// initNewSegment initializes a new segment accumulator.
func (p *HLSTSProcessor) initNewSegment() {
	p.currentSegment.buf.Reset()
	p.currentSegment.startPTS = -1
	p.currentSegment.hasVideo = false
	p.currentSegment.hasAudio = false
	p.currentSegment.startTime = time.Now()

	// Create the persistent muxer on first call, reuse thereafter
	// This maintains continuity counters across segments
	if p.muxer == nil {
		p.swappableWriter = NewSwappableWriter(&p.currentSegment.buf)
		// Use resolved codecs (handles VariantCopy → source codecs like "h265/eac3")
		p.muxer = NewTSMuxer(p.swappableWriter, TSMuxerConfig{
			Logger:      p.config.Logger,
			VideoCodec:  p.ResolvedVideoCodec(),
			AudioCodec:  p.ResolvedAudioCodec(),
			AACConfig:   p.AACConfig(),
			VideoParams: p.videoParams,
		})
	} else {
		// Just redirect the muxer to the new segment buffer
		p.swappableWriter.SetBuffer(&p.currentSegment.buf)
	}
}

// runProcessingLoop is the main processing loop.
func (p *HLSTSProcessor) runProcessingLoop(esVariant *ESVariant) {
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	// Wait for initial video keyframe using base class method
	if _, ok := p.WaitForKeyframe(videoTrack); !ok {
		return // Context cancelled
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.Context().Done():
			// Flush any remaining segment
			p.flushSegment()
			return

		case <-ticker.C:
			// Process available samples
			p.processAvailableSamples(videoTrack, audioTrack)
		}
	}
}

// processAvailableSamples reads and processes available ES samples.
func (p *HLSTSProcessor) processAvailableSamples(videoTrack, audioTrack *ESTrack) {
	// Read video samples
	videoSamples := videoTrack.ReadFrom(p.LastVideoSeq(), 100)
	var bytesRead uint64
	for _, sample := range videoSamples {
		bytesRead += uint64(len(sample.Data))
		p.processVideoSample(sample)
		p.SetLastVideoSeq(sample.Sequence)
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(p.LastAudioSeq(), 200)
	for _, sample := range audioSamples {
		bytesRead += uint64(len(sample.Data))
		p.processAudioSample(sample)
		p.SetLastAudioSeq(sample.Sequence)
	}

	// Track bytes read from buffer for bandwidth stats
	if bytesRead > 0 {
		p.TrackBytesFromBuffer(bytesRead)
	}

	// Update consumer position to allow eviction of samples we've processed
	if len(videoSamples) > 0 || len(audioSamples) > 0 {
		p.UpdateConsumerPosition()
	}

	// Check if we should finalize current segment
	if p.shouldFinalizeSegment() {
		p.flushSegment()
		p.initNewSegment()
	}
}

// processVideoSample processes a single video sample.
func (p *HLSTSProcessor) processVideoSample(sample ESSample) {
	if p.currentSegment.startPTS < 0 {
		p.currentSegment.startPTS = sample.PTS
	}

	// If this is a keyframe and we have enough content, finalize previous segment
	if sample.IsKeyframe && p.hasEnoughContent() {
		p.flushSegment()
		p.initNewSegment()
		p.currentSegment.startPTS = sample.PTS
	}

	// Write to muxer
	if p.muxer != nil {
		// Ignore muxer errors - continue streaming on failures
		_ = p.muxer.WriteVideo(sample.PTS, sample.DTS, sample.Data, sample.IsKeyframe)
		p.currentSegment.hasVideo = true
	}
}

// processAudioSample processes a single audio sample.
func (p *HLSTSProcessor) processAudioSample(sample ESSample) {
	if p.currentSegment.startPTS < 0 {
		p.currentSegment.startPTS = sample.PTS
	}

	if p.muxer != nil {
		// Ignore muxer errors - continue streaming on failures
		_ = p.muxer.WriteAudio(sample.PTS, sample.Data)
		p.currentSegment.hasAudio = true
	}
}

// hasEnoughContent returns true if we have enough content for a segment.
func (p *HLSTSProcessor) hasEnoughContent() bool {
	if !p.currentSegment.hasVideo {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	return elapsed >= p.config.TargetSegmentDuration
}

// shouldFinalizeSegment returns true if current segment should be finalized.
func (p *HLSTSProcessor) shouldFinalizeSegment() bool {
	if !p.currentSegment.hasVideo && !p.currentSegment.hasAudio {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	// Finalize if we've exceeded target duration by 50%
	return elapsed >= p.config.TargetSegmentDuration*1.5
}

// flushSegment finalizes the current segment and adds it to the list.
func (p *HLSTSProcessor) flushSegment() {
	if p.currentSegment.buf.Len() == 0 {
		return
	}

	// Flush muxer
	if p.muxer != nil {
		p.muxer.Flush()
	}

	duration := time.Since(p.currentSegment.startTime).Seconds()
	if duration < 0.1 {
		return // Too short, skip
	}

	// Create segment
	seg := &hlsTSSegment{
		sequence:  p.nextSequence,
		duration:  duration,
		data:      append([]byte(nil), p.currentSegment.buf.Bytes()...), // Copy data
		ptsStart:  p.currentSegment.startPTS,
		createdAt: time.Now(),
	}
	p.nextSequence++

	p.segmentsMu.Lock()
	p.segments = append(p.segments, seg)

	// Trim to max segments
	for len(p.segments) > p.config.MaxSegments {
		p.segments = p.segments[1:]
	}
	p.segmentsMu.Unlock()

	// Notify waiters that a new segment is available
	select {
	case p.segmentNotify <- struct{}{}:
	default:
		// Channel already has notification pending
	}

	p.config.Logger.Debug("Created HLS segment",
		slog.Uint64("sequence", seg.sequence),
		slog.Float64("duration", seg.duration),
		slog.Int("size", len(seg.data)))

	// Update stats
	p.RecordBytesWritten(uint64(len(seg.data)))
}
