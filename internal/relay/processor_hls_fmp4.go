// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
)

// HLS-fMP4 Processor errors.
var (
	ErrHLSFMP4ProcessorClosed = errors.New("HLS-fMP4 processor closed")
	ErrInitSegmentNotReady    = errors.New("init segment not ready")
)

// HLSfMP4ProcessorConfig configures the HLS-fMP4 processor.
type HLSfMP4ProcessorConfig struct {
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

// DefaultHLSfMP4ProcessorConfig returns sensible defaults.
func DefaultHLSfMP4ProcessorConfig() HLSfMP4ProcessorConfig {
	return HLSfMP4ProcessorConfig{
		TargetSegmentDuration: 4.0,  // Cut on every keyframe for faster segment availability
		MaxSegments:           30,   // Keep ~2 minutes of segments for slow clients
		PlaylistSegments:      5,    // More segments in playlist = more buffer before live edge
		PlaylistType:          "",   // Live
		Logger:                slog.Default(),
	}
}

// hlsFMP4Segment represents a single HLS fMP4 segment.
type hlsFMP4Segment struct {
	sequence    uint64
	duration    float64 // Duration in seconds
	data        []byte  // fMP4 fragment data (moof+mdat)
	ptsStart    int64   // Start PTS (in 90kHz units)
	ptsEnd      int64   // End PTS (in 90kHz units)
	discontinue bool    // Discontinuity flag
	createdAt   time.Time
}

// HLSfMP4Processor reads from a SharedESBuffer variant and produces HLS with fMP4/CMAF segments.
// It implements the Processor and FMP4SegmentProvider interfaces.
type HLSfMP4Processor struct {
	*ESProcessorBase

	config HLSfMP4ProcessorConfig

	// Init segment
	initSegment   *InitSegment
	initSegmentMu sync.RWMutex

	// Segment management
	segments      []*hlsFMP4Segment
	segmentsMu    sync.RWMutex
	nextSequence  uint64
	segmentNotify chan struct{} // Notifies waiters when new segment is added

	// Playlist activity tracking - used to determine if clients are still watching.
	// HLS clients poll the playlist periodically; if no polls for a while, they've left.
	lastPlaylistRequest atomic.Value // time.Time

	// Current segment accumulator
	currentSegment struct {
		buf       bytes.Buffer
		startPTS  int64
		endPTS    int64
		hasVideo  bool
		hasAudio  bool
		startTime time.Time
		samples   int // Number of samples in current segment
	}

	// fMP4 muxer using mediacommon
	writer  *FMP4Writer
	adapter *ESSampleAdapter

	// Stream start time - set once when first segment is created
	// Used for availabilityStartTime in DASH manifests (must be constant)
	streamStartTime   time.Time
	streamStartTimeMu sync.RWMutex
}

// NewHLSfMP4Processor creates a new HLS-fMP4 processor.
func NewHLSfMP4Processor(
	id string,
	esBuffer *SharedESBuffer,
	variant CodecVariant,
	config HLSfMP4ProcessorConfig,
) *HLSfMP4Processor {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	esBase := NewESProcessorBase(id, OutputFormatHLSFMP4, esBuffer, variant, ESProcessorConfig{
		Logger: config.Logger,
	})

	adapter := NewESSampleAdapter(DefaultESSampleAdapterConfig())
	// Set audio codec from variant so we correctly handle Opus, AC3, MP3, etc.
	// This is essential for non-AAC codecs since we can't detect them from ES samples.
	adapter.SetAudioCodecFromVariant(variant.AudioCodec())

	p := &HLSfMP4Processor{
		ESProcessorBase: esBase,
		config:          config,
		segments:        make([]*hlsFMP4Segment, 0, config.MaxSegments),
		segmentNotify:   make(chan struct{}, 1),
		writer:          NewFMP4Writer(),
		adapter:         adapter,
	}

	// Initialize playlist request time to now (processor just created means someone wants it)
	p.lastPlaylistRequest.Store(time.Now())

	return p
}

// WaitForSegments waits until at least minSegments are available or context is cancelled.
// This is used to ensure clients don't get empty playlists when first connecting.
func (p *HLSfMP4Processor) WaitForSegments(ctx context.Context, minSegments int) error {
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
func (p *HLSfMP4Processor) SegmentCount() int {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()
	return len(p.segments)
}

// Start begins processing data from the shared buffer.
func (p *HLSfMP4Processor) Start(ctx context.Context) error {
	// Initialize ES processor base (get variant, register consumer)
	esVariant, err := p.InitES(ctx)
	if err != nil {
		return fmt.Errorf("initializing ES processor: %w", err)
	}

	// Update adapter's audio codec if using "copy" mode - we now know the actual codec
	// from the resolved ES variant. This ensures the init segment includes the audio track.
	currentAudioParams := p.adapter.AudioParams()
	if currentAudioParams == nil || currentAudioParams.Codec == "" || currentAudioParams.Codec == "copy" {
		resolvedAudioCodec := p.ResolvedAudioCodec()
		if resolvedAudioCodec != "" && resolvedAudioCodec != "copy" {
			p.adapter.SetAudioCodecFromVariant(resolvedAudioCodec)
			p.config.Logger.Debug("Updated audio codec from resolved variant",
				slog.String("id", p.id),
				slog.String("resolved_codec", resolvedAudioCodec))
		}
	}

	// Initialize segment accumulator
	p.initNewSegment()

	p.config.Logger.Info("Starting HLS-fMP4 processor",
		slog.String("id", p.id),
		slog.String("variant", p.Variant().String()),
		slog.String("es_variant_ptr", fmt.Sprintf("%p", esVariant)),
		slog.String("video_track_ptr", fmt.Sprintf("%p", esVariant.VideoTrack())))

	// Start processing loop
	p.WaitGroup().Add(1)
	go func() {
		defer p.WaitGroup().Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *HLSfMP4Processor) Stop() {
	p.StopES()
}

// RegisterClient adds a client to receive output from this processor.
func (p *HLSfMP4Processor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	_ = p.RegisterClientBase(clientID, w, r)
	return nil
}

// UnregisterClient removes a client.
func (p *HLSfMP4Processor) UnregisterClient(clientID string) {
	p.UnregisterClientBase(clientID)
}

// ServeManifest serves the HLS playlist.
// Returns only the latest PlaylistSegments segments so new clients start near live edge.
func (p *HLSfMP4Processor) ServeManifest(w http.ResponseWriter, r *http.Request) error {
	// Record playlist request - this is the heartbeat that indicates clients are watching
	p.RecordPlaylistRequest()

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
	segments := make([]*hlsFMP4Segment, playlistSize)
	copy(segments, allSegments[startIdx:])
	p.segmentsMu.RUnlock()

	// Build playlist
	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:7\n") // Version 7 for fMP4
	buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", int(p.config.TargetSegmentDuration+0.5)))
	buf.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", segments[0].sequence))

	if p.config.PlaylistType != "" {
		buf.WriteString(fmt.Sprintf("#EXT-X-PLAYLIST-TYPE:%s\n", p.config.PlaylistType))
	}

	// Add map for init segment
	buf.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")

	for _, seg := range segments {
		if seg.discontinue {
			buf.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		buf.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", seg.duration))
		buf.WriteString(fmt.Sprintf("segment%d.m4s\n", seg.sequence))
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
func (p *HLSfMP4Processor) ServeSegment(w http.ResponseWriter, r *http.Request, segmentName string) error {
	// Handle init segment
	if segmentName == "init.mp4" {
		p.initSegmentMu.RLock()
		initSeg := p.initSegment
		p.initSegmentMu.RUnlock()

		if initSeg == nil {
			http.Error(w, "Init segment not ready", http.StatusServiceUnavailable)
			return ErrInitSegmentNotReady
		}

		// Handle conditional request using standard HTTP caching
		if initSeg.ETag != "" {
			w.Header().Set("ETag", initSeg.ETag)

			// Honor standard If-None-Match header for HTTP caching
			if r.Header.Get("If-None-Match") == initSeg.ETag {
				p.config.Logger.Log(context.Background(), observability.LevelTrace, "Init segment: returning 304 (If-None-Match)",
					slog.String("processor_id", p.id),
					slog.String("etag", initSeg.ETag))
				w.WriteHeader(http.StatusNotModified)
				return nil
			}
		}

		p.config.Logger.Log(context.Background(), observability.LevelTrace, "Init segment: delivering to client",
			slog.String("processor_id", p.id),
			slog.String("etag", initSeg.ETag),
			slog.Int("data_size", len(initSeg.Data)))

		p.SetStreamHeaders(w)
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(initSeg.Data)))
		w.Header().Set("Cache-Control", "public, max-age=31536000") // Init segment is immutable
		_, err := w.Write(initSeg.Data)
		return err
	}

	// Parse segment sequence from name (e.g., "segment123.m4s")
	var seq uint64
	_, err := fmt.Sscanf(segmentName, "segment%d.m4s", &seq)
	if err != nil {
		http.Error(w, "Invalid segment name", http.StatusBadRequest)
		return fmt.Errorf("parsing segment name: %w", err)
	}

	p.segmentsMu.RLock()
	var segment *hlsFMP4Segment
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
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(segment.data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Segments are immutable
	_, err = w.Write(segment.data)
	return err
}

// IsFMP4Mode returns true since this processor uses fMP4.
func (p *HLSfMP4Processor) IsFMP4Mode() bool {
	return true
}

// GetInitSegment returns the initialization segment.
func (p *HLSfMP4Processor) GetInitSegment() *InitSegment {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()
	return p.initSegment
}

// HasInitSegment returns true if the init segment is available.
func (p *HLSfMP4Processor) HasInitSegment() bool {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()
	return p.initSegment != nil
}

// GetStreamStartTime returns the time when the first segment was created.
// This is used for availabilityStartTime in DASH manifests which must be constant.
func (p *HLSfMP4Processor) GetStreamStartTime() time.Time {
	p.streamStartTimeMu.RLock()
	defer p.streamStartTimeMu.RUnlock()
	return p.streamStartTime
}

// GetFilteredInitSegment returns the init segment filtered by track type.
// For HLS-fMP4, track filtering is not typically needed, so this returns the full
// init segment when trackType is empty, or an error for specific track requests.
// This method is required to implement FMP4SegmentProvider interface.
func (p *HLSfMP4Processor) GetFilteredInitSegment(trackType string) ([]byte, error) {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()

	if p.initSegment == nil {
		return nil, fmt.Errorf("no init segment available")
	}

	// HLS-fMP4 uses muxed init segments, return full init segment
	// Track filtering is primarily a DASH requirement
	if trackType == "" {
		return p.initSegment.Data, nil
	}

	// For HLS, we return the full muxed init segment even for specific track requests
	// The client will parse out what it needs
	return p.initSegment.Data, nil
}

// GetSegmentInfos implements SegmentProvider.
// Returns only the latest PlaylistSegments segments so new clients start near live edge.
func (p *HLSfMP4Processor) GetSegmentInfos() []SegmentInfo {
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
	var playlistSeqs []uint64
	for i, seg := range latestSegments {
		infos[i] = SegmentInfo{
			Sequence:  seg.sequence,
			Duration:  seg.duration,
			Timestamp: seg.createdAt,
		}
		playlistSeqs = append(playlistSeqs, seg.sequence)
	}

	// Log what sequences are being returned for playlist
	var bufferFirstSeq, bufferLastSeq uint64
	if len(p.segments) > 0 {
		bufferFirstSeq = p.segments[0].sequence
		bufferLastSeq = p.segments[len(p.segments)-1].sequence
	}
	p.config.Logger.Debug("GetSegmentInfos returning playlist segments",
		slog.String("processor_id", p.id),
		slog.Int("playlist_size", len(infos)),
		slog.Any("playlist_sequences", playlistSeqs),
		slog.Int("buffer_size", len(p.segments)),
		slog.Uint64("buffer_first_seq", bufferFirstSeq),
		slog.Uint64("buffer_last_seq", bufferLastSeq))

	return infos
}

// GetSegment implements SegmentProvider.
func (p *HLSfMP4Processor) GetSegment(sequence uint64) (*Segment, error) {
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

	// Log buffer state when segment not found for debugging
	var availableSeqs []uint64
	for _, seg := range p.segments {
		availableSeqs = append(availableSeqs, seg.sequence)
	}
	p.config.Logger.Warn("Segment not found in buffer",
		slog.String("processor_id", p.id),
		slog.Uint64("requested_sequence", sequence),
		slog.Int("buffer_size", len(p.segments)),
		slog.Any("available_sequences", availableSeqs))

	return nil, ErrSegmentNotFound
}

// TargetDuration implements SegmentProvider.
func (p *HLSfMP4Processor) TargetDuration() int {
	return int(p.config.TargetSegmentDuration + 0.5)
}

// initNewSegment initializes a new segment accumulator.
func (p *HLSfMP4Processor) initNewSegment() {
	p.currentSegment.buf.Reset()
	p.currentSegment.startPTS = -1
	p.currentSegment.endPTS = -1
	p.currentSegment.hasVideo = false
	p.currentSegment.hasAudio = false
	p.currentSegment.startTime = time.Now()
	p.currentSegment.samples = 0
}

// runProcessingLoop is the main processing loop.
func (p *HLSfMP4Processor) runProcessingLoop(esVariant *ESVariant) {
	ctx := p.Context()
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	p.config.Logger.Log(ctx, observability.LevelTrace, "HLS-fMP4 processor starting processing loop",
		slog.String("id", p.id),
		slog.String("variant", esVariant.Variant().String()),
		slog.String("variant_ptr", fmt.Sprintf("%p", esVariant)),
		slog.String("video_track_ptr", fmt.Sprintf("%p", videoTrack)),
		slog.Int("video_track_count", videoTrack.Count()),
		slog.Int("audio_track_count", audioTrack.Count()))

	// Wait for initial video keyframe
	// Check for existing samples first before waiting - handles case where
	// transcoder has stopped but buffer still has content (finite streams)
	p.config.Logger.Log(ctx, observability.LevelTrace, "HLS-fMP4 processor: waiting for initial keyframe",
		slog.String("id", p.id),
		slog.String("variant", esVariant.Variant().String()),
		slog.String("notify_chan_ptr", fmt.Sprintf("%p", videoTrack.NotifyChan())))
	notifyCount := 0
	for {
		// Try to read samples immediately (non-blocking check)
		trackCount := videoTrack.Count()
		samples := videoTrack.ReadFromKeyframe(p.LastVideoSeq(), 1)
		if len(samples) > 0 {
			p.config.Logger.Log(ctx, observability.LevelTrace, "HLS-fMP4 processor: found initial keyframe",
				slog.String("id", p.id),
				slog.Uint64("sequence", samples[0].Sequence),
				slog.Int("notify_count", notifyCount),
				slog.Int("data_len", len(samples[0].Data)))
			p.SetLastVideoSeq(samples[0].Sequence - 1)
			break
		}

		// Log periodically to help debug
		if notifyCount%50 == 1 || notifyCount == 1 {
			// Also check how many samples have IsKeyframe=true
			allSamples := videoTrack.ReadFrom(0, 100)
			keyframeCount := 0
			for _, s := range allSamples {
				if s.IsKeyframe {
					keyframeCount++
				}
			}
			p.config.Logger.Log(ctx, observability.LevelTrace, "HLS-fMP4 processor: still waiting for keyframe",
				slog.String("id", p.id),
				slog.Int("notify_count", notifyCount),
				slog.Int("track_sample_count", trackCount),
				slog.Int("samples_read", len(allSamples)),
				slog.Int("keyframes_found", keyframeCount),
				slog.Uint64("last_video_seq", p.LastVideoSeq()))
		}

		// No samples available, wait for notification or context cancellation
		select {
		case <-ctx.Done():
			p.config.Logger.Log(ctx, observability.LevelTrace, "HLS-fMP4 processor: context cancelled while waiting for keyframe",
				slog.String("id", p.id),
				slog.Int("notify_count", notifyCount))
			return
		case <-videoTrack.NotifyChan():
			notifyCount++
			// New sample notification received, loop back to read
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Accumulators for current segment
	var videoSamples []ESSample
	var audioSamples []ESSample

	for {
		select {
		case <-ctx.Done():
			// Flush any remaining segment
			if len(videoSamples) > 0 || len(audioSamples) > 0 {
				p.flushSegment(videoSamples, audioSamples)
			}
			return

		case <-ticker.C:
			// Read video samples
			newVideoSamples := videoTrack.ReadFrom(p.LastVideoSeq(), 100)
			var bytesRead uint64
			for _, sample := range newVideoSamples {
				bytesRead += uint64(len(sample.Data))
				// Check if this keyframe should trigger a new segment
				if sample.IsKeyframe && len(videoSamples) > 0 && p.hasEnoughContent() {
					// Only reset samples if flush succeeded; if deferred, keep accumulating
					if p.flushSegment(videoSamples, audioSamples) {
						p.initNewSegment()
						videoSamples = nil
						audioSamples = nil
					}
				}

				videoSamples = append(videoSamples, sample)
				p.SetLastVideoSeq(sample.Sequence)
				p.currentSegment.hasVideo = true
				if p.currentSegment.startPTS < 0 {
					p.currentSegment.startPTS = sample.PTS
				}
				p.currentSegment.endPTS = sample.PTS
			}

			// Read audio samples
			newAudioSamples := audioTrack.ReadFrom(p.LastAudioSeq(), 200)
			for _, sample := range newAudioSamples {
				bytesRead += uint64(len(sample.Data))
				audioSamples = append(audioSamples, sample)
				p.SetLastAudioSeq(sample.Sequence)
				p.currentSegment.hasAudio = true
				if p.currentSegment.startPTS < 0 {
					p.currentSegment.startPTS = sample.PTS
				}
			}

			// Track bytes read from buffer for bandwidth stats
			if bytesRead > 0 {
				p.TrackBytesFromBuffer(bytesRead)
			}

			// Update consumer position to allow eviction of samples we've processed
			if len(newVideoSamples) > 0 || len(newAudioSamples) > 0 {
				p.UpdateConsumerPosition()
			}

			// Check if we should finalize current segment (timeout)
			if p.shouldFinalizeSegment() {
				// Only reset samples if flush succeeded; if deferred, keep accumulating
				if p.flushSegment(videoSamples, audioSamples) {
					p.initNewSegment()
					videoSamples = nil
					audioSamples = nil
				}
			}
		}
	}
}

// hasEnoughContent returns true if we have enough content for a segment.
func (p *HLSfMP4Processor) hasEnoughContent() bool {
	if !p.currentSegment.hasVideo {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	return elapsed >= p.config.TargetSegmentDuration
}

// shouldFinalizeSegment returns true if current segment should be finalized.
func (p *HLSfMP4Processor) shouldFinalizeSegment() bool {
	if !p.currentSegment.hasVideo && !p.currentSegment.hasAudio {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	// Finalize if we've exceeded target duration by 50%
	return elapsed >= p.config.TargetSegmentDuration*1.5
}

// flushSegment finalizes the current segment and adds it to the list.
// Returns true if the segment was successfully flushed, false if deferred (e.g., waiting for codec params).
// Callers should only reset their accumulated samples if this returns true.
func (p *HLSfMP4Processor) flushSegment(videoSamples, audioSamples []ESSample) bool {
	if len(videoSamples) == 0 && len(audioSamples) == 0 {
		return true // Nothing to flush, consider it success
	}

	// Extract codec parameters if not already done
	if len(videoSamples) > 0 {
		updated := p.adapter.UpdateVideoParams(videoSamples)
		if updated {
			videoParams := p.adapter.VideoParams()
			p.config.Logger.Debug("Video params updated from samples",
				slog.String("id", p.id),
				slog.String("codec", videoParams.Codec),
				slog.Int("h265_vps_len", len(videoParams.H265VPS)),
				slog.Int("h265_sps_len", len(videoParams.H265SPS)),
				slog.Int("h265_pps_len", len(videoParams.H265PPS)),
				slog.Int("h264_sps_len", len(videoParams.H264SPS)),
				slog.Int("h264_pps_len", len(videoParams.H264PPS)),
				slog.Int("av1_seq_len", len(videoParams.AV1SequenceHeader)))
		}
	}
	if len(audioSamples) > 0 {
		p.adapter.UpdateAudioParams(audioSamples)
	}

	// Generate init segment if not yet created
	p.initSegmentMu.RLock()
	hasInit := p.initSegment != nil
	p.initSegmentMu.RUnlock()

	if !hasInit {
		// Determine audio presence from the adapter's preset audio codec, not just current samples.
		// This ensures audio track is included even if audio samples haven't arrived yet
		// (audio transcoding may be slower than video).
		hasAudio := p.adapter.AudioParams() != nil && p.adapter.AudioParams().Codec != ""

		// Log what params we have before attempting init segment generation
		videoParams := p.adapter.VideoParams()
		hasValidVideoParams := videoParams != nil && (
			(videoParams.Codec == "h264" && videoParams.H264SPS != nil && videoParams.H264PPS != nil) ||
			(videoParams.Codec == "h265" && videoParams.H265SPS != nil && videoParams.H265PPS != nil) ||
			(videoParams.Codec == "av1" && videoParams.AV1SequenceHeader != nil) ||
			(videoParams.Codec == "vp9" && videoParams.VP9Width > 0))

		p.config.Logger.Debug("Attempting init segment generation",
			slog.String("id", p.id),
			slog.Int("video_sample_count", len(videoSamples)),
			slog.Int("audio_sample_count", len(audioSamples)),
			slog.Bool("has_audio", hasAudio),
			slog.Bool("has_valid_video_params", hasValidVideoParams))

		if len(videoSamples) > 0 && !hasValidVideoParams {
			// Log a warning - we have video samples but no valid codec params yet
			p.config.Logger.Warn("Video samples present but codec params incomplete, deferring init segment",
				slog.String("id", p.id),
				slog.String("codec", func() string {
					if videoParams != nil {
						return videoParams.Codec
					}
					return "unknown"
				}()))
			return false // Don't try to generate init segment yet - wait for valid params
		}

		if err := p.generateInitSegment(len(videoSamples) > 0, hasAudio); err != nil {
			p.config.Logger.Error("Failed to generate init segment",
				slog.String("error", err.Error()))
			return false
		}
	}

	// Convert ES samples to fMP4 samples
	fmp4VideoSamples, videoBaseTime := p.adapter.ConvertVideoSamples(videoSamples)
	fmp4AudioSamples, audioBaseTime := p.adapter.ConvertAudioSamples(audioSamples)

	// Generate fragment using mediacommon
	fragmentData, err := p.writer.GeneratePart(fmp4VideoSamples, fmp4AudioSamples, videoBaseTime, audioBaseTime)
	if err != nil {
		p.config.Logger.Error("Failed to generate fragment",
			slog.String("error", err.Error()))
		return false
	}

	if len(fragmentData) == 0 {
		return true // Empty fragment is still considered success
	}

	// Calculate duration
	duration := p.calculateDuration(videoSamples, audioSamples)
	if duration < 0.1 {
		return true // Too short, skip but consider it success
	}

	// Create segment
	seg := &hlsFMP4Segment{
		sequence:  p.nextSequence,
		duration:  duration,
		data:      fragmentData,
		ptsStart:  p.currentSegment.startPTS,
		ptsEnd:    p.currentSegment.endPTS,
		createdAt: time.Now(),
	}
	p.nextSequence++

	p.segmentsMu.Lock()
	p.segments = append(p.segments, seg)

	// Set stream start time once (on first segment)
	// This is used for availabilityStartTime which must be constant throughout the stream
	p.streamStartTimeMu.Lock()
	if p.streamStartTime.IsZero() {
		p.streamStartTime = seg.createdAt
	}
	p.streamStartTimeMu.Unlock()

	// Trim to max segments, tracking evicted sequences for debug
	var evictedSeqs []uint64
	for len(p.segments) > p.config.MaxSegments {
		evictedSeqs = append(evictedSeqs, p.segments[0].sequence)
		p.segments = p.segments[1:]
	}

	// Capture buffer range for logging
	var firstSeq, lastSeq uint64
	if len(p.segments) > 0 {
		firstSeq = p.segments[0].sequence
		lastSeq = p.segments[len(p.segments)-1].sequence
	}
	bufferSize := len(p.segments)
	p.segmentsMu.Unlock()

	// Notify waiters that a new segment is available
	select {
	case p.segmentNotify <- struct{}{}:
	default:
		// Channel already has notification pending
	}

	p.config.Logger.Debug("Created HLS-fMP4 segment",
		slog.Uint64("sequence", seg.sequence),
		slog.Float64("duration", seg.duration),
		slog.Int("size", len(seg.data)),
		slog.Int("buffer_size", bufferSize),
		slog.Uint64("buffer_first_seq", firstSeq),
		slog.Uint64("buffer_last_seq", lastSeq),
		slog.Any("evicted_sequences", evictedSeqs))

	// Update stats
	p.RecordBytesWritten(uint64(len(seg.data)))
	return true
}

// generateInitSegment creates the initialization segment.
func (p *HLSfMP4Processor) generateInitSegment(hasVideo, hasAudio bool) error {
	// Log video codec parameters for debugging
	videoParams := p.adapter.VideoParams()
	if videoParams != nil {
		p.config.Logger.Debug("Video codec params for init segment",
			slog.String("id", p.id),
			slog.String("codec", videoParams.Codec),
			slog.Int("h264_sps_len", len(videoParams.H264SPS)),
			slog.Int("h264_pps_len", len(videoParams.H264PPS)),
			slog.Int("h265_vps_len", len(videoParams.H265VPS)),
			slog.Int("h265_sps_len", len(videoParams.H265SPS)),
			slog.Int("h265_pps_len", len(videoParams.H265PPS)),
			slog.Int("av1_seq_len", len(videoParams.AV1SequenceHeader)))
	} else {
		p.config.Logger.Warn("No video codec params detected for init segment",
			slog.String("id", p.id))
	}

	// Configure the writer with codec parameters
	if err := p.adapter.ConfigureWriter(p.writer); err != nil {
		return fmt.Errorf("configuring writer: %w", err)
	}

	// Generate init segment using mediacommon
	// Note: We generate BEFORE locking params so that if this fails,
	// future flushSegment calls can still update params and retry.
	initData, err := p.writer.GenerateInit(hasVideo, hasAudio, 90000, 90000)
	if err != nil {
		return fmt.Errorf("generating init: %w", err)
	}

	// Lock parameters only after successful init generation
	// This ensures we don't lock invalid params that would cause repeated failures
	p.adapter.LockParams()

	// Log track creation status for debugging
	p.config.Logger.Debug("Init segment generated",
		slog.String("id", p.id),
		slog.Int("init_size", len(initData)),
		slog.Int("video_track_id", p.writer.VideoTrackID()),
		slog.Int("audio_track_id", p.writer.AudioTrackID()))

	// Compute ETag from init segment hash to enable HTTP caching
	// This helps clients avoid refetching the init segment on reconnects,
	// which prevents "duplicate MOOV atom" warnings
	hash := sha256.Sum256(initData)
	etag := `"` + hex.EncodeToString(hash[:8]) + `"` // Use first 8 bytes (16 hex chars)

	p.initSegmentMu.Lock()
	p.initSegment = &InitSegment{
		Data: initData,
		ETag: etag,
	}
	p.initSegmentMu.Unlock()

	p.config.Logger.Debug("Generated init segment",
		slog.Int("size", len(initData)),
		slog.String("etag", etag),
		slog.Bool("has_video", hasVideo),
		slog.Bool("has_audio", hasAudio))

	return nil
}

// calculateDuration calculates segment duration from samples.
func (p *HLSfMP4Processor) calculateDuration(videoSamples, audioSamples []ESSample) float64 {
	// Use wall clock time as primary duration
	duration := time.Since(p.currentSegment.startTime).Seconds()

	// Verify with PTS if available
	if len(videoSamples) > 1 {
		firstPTS := videoSamples[0].PTS
		lastPTS := videoSamples[len(videoSamples)-1].PTS
		ptsDuration := float64(lastPTS-firstPTS) / 90000.0
		// Add approximate duration of last sample
		if len(videoSamples) > 1 {
			avgDuration := float64(lastPTS-firstPTS) / float64(len(videoSamples)-1)
			ptsDuration += avgDuration / 90000.0
		}

		// Use PTS duration if it's reasonable
		if ptsDuration > 0.1 && ptsDuration < duration*2 {
			duration = ptsDuration
		}
	} else if len(audioSamples) > 1 {
		firstPTS := audioSamples[0].PTS
		lastPTS := audioSamples[len(audioSamples)-1].PTS
		duration = float64(lastPTS-firstPTS) / 90000.0
		if len(audioSamples) > 1 {
			avgDuration := float64(lastPTS-firstPTS) / float64(len(audioSamples)-1)
			duration += avgDuration / 90000.0
		}
	}

	return duration
}

// RecordPlaylistRequest records that a playlist was requested.
// Implements PlaylistActivityRecorder interface.
func (p *HLSfMP4Processor) RecordPlaylistRequest() {
	p.lastPlaylistRequest.Store(time.Now())
}

// LastPlaylistRequest returns when the last playlist was requested.
// This is used to determine if clients are still watching.
func (p *HLSfMP4Processor) LastPlaylistRequest() time.Time {
	t, _ := p.lastPlaylistRequest.Load().(time.Time)
	return t
}

// PlaylistIdleTimeout returns how long the processor can go without playlist
// requests before it should be considered idle and stopped.
// Formula: playlist_segments * segment_duration * 2
// This gives clients time to buffer and consume content before re-polling.
func (p *HLSfMP4Processor) PlaylistIdleTimeout() time.Duration {
	segments := p.config.PlaylistSegments
	if segments <= 0 {
		segments = 3
	}
	duration := p.config.TargetSegmentDuration
	if duration <= 0 {
		duration = 4.0
	}
	return time.Duration(float64(segments)*duration*2) * time.Second
}

// IsPlaylistIdle returns true if no playlist has been requested for longer
// than the playlist idle timeout. This indicates clients have stopped watching.
func (p *HLSfMP4Processor) IsPlaylistIdle() bool {
	lastRequest := p.LastPlaylistRequest()
	if lastRequest.IsZero() {
		return false // Never had a request, not idle yet
	}
	return time.Since(lastRequest) > p.PlaylistIdleTimeout()
}

// IsIdle returns true if the processor should be stopped due to inactivity.
// For HLS-fMP4, we use playlist-based idle detection since clients poll for manifests.
// This overrides the base implementation which only checks client count.
func (p *HLSfMP4Processor) IsIdle() bool {
	return p.IsPlaylistIdle()
}
