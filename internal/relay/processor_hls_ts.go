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
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
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
		MaxSegments:           7,
		PlaylistSegments:      3,  // New clients start near live edge
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
	*BaseProcessor

	config   HLSTSProcessorConfig
	esBuffer *SharedESBuffer
	variant  CodecVariant

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

	// ES reading state
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Reference to the ES variant for consumer tracking
	esVariant *ESVariant

	// Resolved codec names from the ES variant (handles VariantCopy → source codecs)
	resolvedVideoCodec string
	resolvedAudioCodec string

	// AAC config for proper sample rate/channel count in ADTS headers
	aacConfig *mpeg4audio.AudioSpecificConfig

	// Video parameter helper - persists across segments to retain SPS/PPS
	videoParams *VideoParamHelper

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
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

	base := NewBaseProcessor(id, OutputFormatHLSTS, nil) // nil buffer - we use esBuffer instead

	p := &HLSTSProcessor{
		BaseProcessor: base,
		config:        config,
		esBuffer:      esBuffer,
		variant:       variant,
		segments:      make([]*hlsTSSegment, 0, config.MaxSegments),
		segmentNotify: make(chan struct{}, 1),
		videoParams:   NewVideoParamHelper(), // Persists across segments
	}

	return p
}

// Start begins processing data from the shared buffer.
func (p *HLSTSProcessor) Start(ctx context.Context) error {
	if !p.started.CompareAndSwap(false, true) {
		return errors.New("processor already started")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.BaseProcessor.startedAt = time.Now()

	// Get or create the variant we'll read from
	// This will wait for the source variant to be ready if requesting VariantCopy
	esVariant, err := p.esBuffer.GetOrCreateVariantWithContext(p.ctx, p.variant)
	if err != nil {
		return fmt.Errorf("getting variant: %w", err)
	}

	// Register with buffer
	p.esBuffer.RegisterProcessor(p.id)

	// Store variant reference and register as a consumer to prevent eviction of unread samples
	p.esVariant = esVariant
	esVariant.RegisterConsumer(p.id)

	// Resolve actual codecs from the ES variant's tracks
	// The variant key (e.g., "h265/") may not include audio if it wasn't detected
	// when the source was created, but the track codec gets updated when audio arrives.
	p.resolvedVideoCodec = esVariant.VideoTrack().Codec()
	p.resolvedAudioCodec = esVariant.AudioTrack().Codec()

	// If audio codec is empty, wait briefly for audio detection
	if p.resolvedAudioCodec == "" {
		p.config.Logger.Debug("Waiting for audio codec detection")
		waitCtx, waitCancel := context.WithTimeout(p.ctx, 2*time.Second)
		ticker := time.NewTicker(50 * time.Millisecond)
		for p.resolvedAudioCodec == "" {
			select {
			case <-waitCtx.Done():
				p.config.Logger.Debug("Audio codec detection timeout, proceeding without audio")
				ticker.Stop()
				waitCancel()
				goto initMuxer
			case <-ticker.C:
				p.resolvedAudioCodec = esVariant.AudioTrack().Codec()
			}
		}
		ticker.Stop()
		waitCancel()
		p.config.Logger.Debug("Audio codec detected", slog.String("audio_codec", p.resolvedAudioCodec))
	}

initMuxer:
	// Get AAC config from initData if available
	// For AAC, we need the initData to get correct sample rate/channels
	// Wait briefly for it since the demuxer may not have parsed the first ADTS packet yet
	if p.resolvedAudioCodec == "aac" {
		initData := esVariant.AudioTrack().GetInitData()
		if initData == nil {
			p.config.Logger.Debug("Waiting for AAC initData from demuxer")
			waitCtx, waitCancel := context.WithTimeout(p.ctx, 2*time.Second)
			ticker := time.NewTicker(50 * time.Millisecond)
		waitLoop:
			for {
				select {
				case <-waitCtx.Done():
					p.config.Logger.Debug("AAC initData timeout, using defaults")
					break waitLoop
				case <-ticker.C:
					initData = esVariant.AudioTrack().GetInitData()
					if initData != nil {
						break waitLoop
					}
				}
			}
			ticker.Stop()
			waitCancel()
		}

		if initData != nil {
			p.aacConfig = &mpeg4audio.AudioSpecificConfig{}
			if err := p.aacConfig.Unmarshal(initData); err != nil {
				p.config.Logger.Debug("Failed to unmarshal AAC config from initData, using defaults",
					slog.String("error", err.Error()))
				p.aacConfig = nil
			} else {
				p.config.Logger.Debug("AAC config from initData",
					slog.Int("type", int(p.aacConfig.Type)),
					slog.Int("sample_rate", p.aacConfig.SampleRate),
					slog.Int("channel_count", p.aacConfig.ChannelCount))
			}
		}
	}

	// Initialize TS muxer for current segment
	p.initNewSegment()

	p.config.Logger.Debug("Starting HLS-TS processor",
		slog.String("id", p.id),
		slog.String("requested_variant", p.variant.String()),
		slog.String("resolved_variant", esVariant.Variant().String()),
		slog.String("video_codec", p.resolvedVideoCodec),
		slog.String("audio_codec", p.resolvedAudioCodec),
		slog.Bool("has_aac_config", p.aacConfig != nil))

	// Start processing loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *HLSTSProcessor) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()

	// Unregister as a consumer to allow eviction of our unread samples
	if p.esVariant != nil {
		p.esVariant.UnregisterConsumer(p.id)
	}

	p.esBuffer.UnregisterProcessor(p.id)
	p.BaseProcessor.Close()

	p.config.Logger.Debug("Processor stopped",
		slog.String("id", p.id))
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
		case <-p.ctx.Done():
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
// If no segments are available, it waits up to 15 seconds for the first segment.
func (p *HLSTSProcessor) ServeManifest(w http.ResponseWriter, r *http.Request) error {
	// Wait for at least 1 segment to be available (with timeout)
	waitCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
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
	if p.esBuffer != nil && p.esBuffer.IsSourceCompleted() {
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
			VideoCodec:  p.resolvedVideoCodec,
			AudioCodec:  p.resolvedAudioCodec,
			AACConfig:   p.aacConfig,
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

	// Wait for initial video keyframe
	// Check for existing samples first before waiting - handles case where
	// transcoder has stopped but buffer still has content (finite streams)
	p.config.Logger.Debug("Waiting for initial keyframe")
	for {
		// Try to read samples immediately (non-blocking check)
		samples := videoTrack.ReadFromKeyframe(p.lastVideoSeq, 1)
		if len(samples) > 0 {
			p.lastVideoSeq = samples[0].Sequence - 1 // Process this sample next
			break
		}

		// No samples available, wait for notification or context cancellation
		select {
		case <-p.ctx.Done():
			return
		case <-videoTrack.NotifyChan():
			// New sample notification received, loop back to read
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
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
	videoSamples := videoTrack.ReadFrom(p.lastVideoSeq, 100)
	var bytesRead uint64
	for _, sample := range videoSamples {
		bytesRead += uint64(len(sample.Data))
		p.processVideoSample(sample)
		p.lastVideoSeq = sample.Sequence
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(p.lastAudioSeq, 200)
	for _, sample := range audioSamples {
		bytesRead += uint64(len(sample.Data))
		p.processAudioSample(sample)
		p.lastAudioSeq = sample.Sequence
	}

	// Track bytes read from buffer for bandwidth stats
	if bytesRead > 0 {
		p.TrackBytesFromBuffer(bytesRead)
	}

	// Update consumer position to allow eviction of samples we've processed
	if p.esVariant != nil && (len(videoSamples) > 0 || len(audioSamples) > 0) {
		p.esVariant.UpdateConsumerPosition(p.id, p.lastVideoSeq, p.lastAudioSeq)
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
