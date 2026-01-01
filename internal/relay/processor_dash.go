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
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
)

// DASH Processor errors.
var (
	ErrDASHProcessorClosed     = errors.New("DASH processor closed")
	ErrDASHInitSegmentNotReady = errors.New("DASH init segment not ready")
)

// DASHProcessorConfig configures the DASH processor.
type DASHProcessorConfig struct {
	// TargetSegmentDuration is the target duration for each segment in seconds.
	TargetSegmentDuration float64

	// MaxSegments is the maximum number of segments to keep in the sliding window.
	// This is the buffer size for resilience - supports slow clients who've already started.
	MaxSegments int

	// PlaylistSegments is the number of segments to include in the manifest for new clients.
	// New clients will start from the latest segments (near live edge).
	// If 0, defaults to min(3, MaxSegments).
	// Should be <= MaxSegments.
	PlaylistSegments int

	// MinBufferTime in seconds for the MPD.
	MinBufferTime float64

	// Logger for structured logging.
	Logger *slog.Logger
}

// DefaultDASHProcessorConfig returns sensible defaults.
func DefaultDASHProcessorConfig() DASHProcessorConfig {
	return DASHProcessorConfig{
		TargetSegmentDuration: 4.0, // Shorter segments for faster startup
		MaxSegments:           30,  // Keep ~2 minutes of segments for slow clients
		PlaylistSegments:      5,   // Segments in playlist
		MinBufferTime:         4.0, // Match target segment duration
		Logger:                slog.Default(),
	}
}

// dashSegment represents a single DASH segment.
type dashSegment struct {
	sequence  uint64
	duration  float64 // Duration in seconds
	data      []byte  // fMP4 fragment data (moof+mdat)
	ptsStart  int64   // Start PTS (in 90kHz units)
	ptsEnd    int64   // End PTS (in 90kHz units)
	createdAt time.Time
}

// DASHProcessor reads from a SharedESBuffer variant and produces DASH with fMP4 segments.
// It implements the Processor and FMP4SegmentProvider interfaces.
type DASHProcessor struct {
	*ESProcessorBase

	config DASHProcessorConfig

	// Init segment
	initSegment   *InitSegment
	initSegmentMu sync.RWMutex

	// Segment management
	segments      []*dashSegment
	segmentsMu    sync.RWMutex
	nextSequence  uint64
	segmentNotify chan struct{} // Notifies waiters when new segment is added

	// Current segment accumulator
	currentSegment struct {
		startPTS  int64
		endPTS    int64
		hasVideo  bool
		hasAudio  bool
		startTime time.Time
	}

	// fMP4 muxer using mediacommon
	writer  *FMP4Writer
	adapter *ESSampleAdapter

	// Timestamp offset for normalizing segment times to start from 0
	// These are set from the first segment and subtracted from all subsequent segments
	videoTimeOffset   uint64
	audioTimeOffset   uint64
	timeOffsetInitMu  sync.Mutex
	timeOffsetInitSet bool

	// Stream start time - set once when first segment is created
	// Used for availabilityStartTime in DASH manifest (must be constant)
	streamStartTime   time.Time
	streamStartTimeMu sync.RWMutex
}

// NewDASHProcessor creates a new DASH processor.
func NewDASHProcessor(
	id string,
	esBuffer *SharedESBuffer,
	variant CodecVariant,
	config DASHProcessorConfig,
) *DASHProcessor {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	esBase := NewESProcessorBase(id, OutputFormatDASH, esBuffer, variant, ESProcessorConfig{
		Logger: config.Logger,
	})

	adapter := NewESSampleAdapter(DefaultESSampleAdapterConfig())
	// Set audio codec from variant so we correctly handle Opus, AC3, MP3, etc.
	// This is essential for non-AAC codecs since we can't detect them from ES samples.
	adapter.SetAudioCodecFromVariant(variant.AudioCodec())

	p := &DASHProcessor{
		ESProcessorBase: esBase,
		config:          config,
		segments:        make([]*dashSegment, 0, config.MaxSegments),
		segmentNotify:   make(chan struct{}, 1),
		writer:          NewFMP4Writer(),
		adapter:         adapter,
	}

	return p
}

// WaitForSegments waits until at least minSegments are available or context is cancelled.
// This is used to ensure clients don't get empty manifests when first connecting.
func (p *DASHProcessor) WaitForSegments(ctx context.Context, minSegments int) error {
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
func (p *DASHProcessor) SegmentCount() int {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()
	return len(p.segments)
}

// Start begins processing data from the shared buffer.
func (p *DASHProcessor) Start(ctx context.Context) error {
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

	p.config.Logger.Debug("Starting DASH processor",
		slog.String("id", p.id),
		slog.String("variant", p.Variant().String()))

	// Start processing loop
	p.WaitGroup().Add(1)
	go func() {
		defer p.WaitGroup().Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *DASHProcessor) Stop() {
	p.StopES()
}

// RegisterClient adds a client to receive output from this processor.
// Returns ErrProcessorStopping if the processor is being shut down.
func (p *DASHProcessor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	_, err := p.RegisterClientBase(clientID, w, r)
	return err
}

// UnregisterClient removes a client.
func (p *DASHProcessor) UnregisterClient(clientID string) {
	p.UnregisterClientBase(clientID)
}

// ServeManifest serves the DASH MPD manifest.
// Returns only the latest PlaylistSegments segments so new clients start near live edge.
func (p *DASHProcessor) ServeManifest(w http.ResponseWriter, r *http.Request) error {
	p.segmentsMu.RLock()
	allSegments := p.segments
	if len(allSegments) == 0 {
		p.segmentsMu.RUnlock()
		http.Error(w, "No segments available", http.StatusServiceUnavailable)
		return errors.New("no segments available")
	}

	// Determine how many segments to include in manifest (latest segments for near-live)
	playlistSize := p.config.PlaylistSegments
	if playlistSize <= 0 {
		playlistSize = 3
	}
	if playlistSize > len(allSegments) {
		playlistSize = len(allSegments)
	}

	// Get only the latest segments
	startIdx := len(allSegments) - playlistSize
	segments := make([]*dashSegment, playlistSize)
	copy(segments, allSegments[startIdx:])
	p.segmentsMu.RUnlock()

	// Calculate total duration and first segment time
	var totalDuration float64
	for _, seg := range segments {
		totalDuration += seg.duration
	}

	// Check if source stream is complete (VOD mode)
	isSourceComplete := p.esBuffer != nil && p.esBuffer.IsSourceCompleted()

	// Build MPD manifest
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `)
	buf.WriteString(`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" `)
	buf.WriteString(`xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" `)

	if isSourceComplete {
		// VOD mode: static MPD with known duration
		buf.WriteString(`type="static" `)
		buf.WriteString(`profiles="urn:mpeg:dash:profile:isoff-on-demand:2011" `)
		buf.WriteString(fmt.Sprintf(`mediaPresentationDuration="PT%.3fS" `, totalDuration))
	} else {
		// Live mode: dynamic MPD with periodic updates
		buf.WriteString(`type="dynamic" `)
		buf.WriteString(`profiles="urn:mpeg:dash:profile:isoff-live:2011" `)
		buf.WriteString(fmt.Sprintf(`minimumUpdatePeriod="PT%.1fS" `, p.config.TargetSegmentDuration))
		buf.WriteString(`availabilityStartTime="1970-01-01T00:00:00Z" `)
	}

	buf.WriteString(fmt.Sprintf(`minBufferTime="PT%.1fS">`, p.config.MinBufferTime))
	buf.WriteString("\n")

	// Period
	buf.WriteString("  <Period id=\"0\" start=\"PT0S\">\n")

	// AdaptationSet for video
	buf.WriteString("    <AdaptationSet id=\"0\" contentType=\"video\" segmentAlignment=\"true\" startWithSAP=\"1\">\n")
	buf.WriteString("      <SegmentTemplate ")
	buf.WriteString(`media="segment$Number$.m4s" `)
	buf.WriteString(`initialization="init.mp4" `)
	buf.WriteString(`timescale="90000" `)
	buf.WriteString(fmt.Sprintf(`startNumber="%d"`, segments[0].sequence))
	buf.WriteString(">\n")
	buf.WriteString("        <SegmentTimeline>\n")

	// Write segment timeline
	for _, seg := range segments {
		// Duration in timescale units (90kHz)
		durationUnits := int64(seg.duration * 90000)
		buf.WriteString(fmt.Sprintf("          <S d=\"%d\"/>\n", durationUnits))
	}

	buf.WriteString("        </SegmentTimeline>\n")
	buf.WriteString("      </SegmentTemplate>\n")

	// Representation
	buf.WriteString("      <Representation id=\"v0\" mimeType=\"video/mp4\" codecs=\"avc1.42c01e\" ")
	buf.WriteString(`bandwidth="2000000" width="1920" height="1080" frameRate="30"/>`)
	buf.WriteString("\n")

	buf.WriteString("    </AdaptationSet>\n")

	// AdaptationSet for audio
	buf.WriteString("    <AdaptationSet id=\"1\" contentType=\"audio\" segmentAlignment=\"true\">\n")
	buf.WriteString("      <SegmentTemplate ")
	buf.WriteString(`media="segment$Number$.m4s" `)
	buf.WriteString(`initialization="init.mp4" `)
	buf.WriteString(`timescale="90000" `)
	buf.WriteString(fmt.Sprintf(`startNumber="%d"`, segments[0].sequence))
	buf.WriteString("/>\n")

	// Audio representation
	buf.WriteString("      <Representation id=\"a0\" mimeType=\"audio/mp4\" codecs=\"mp4a.40.2\" ")
	buf.WriteString(`bandwidth="128000" audioSamplingRate="48000"/>`)
	buf.WriteString("\n")

	buf.WriteString("    </AdaptationSet>\n")
	buf.WriteString("  </Period>\n")
	buf.WriteString("</MPD>\n")

	p.SetStreamHeaders(w)
	w.Header().Set("Content-Type", "application/dash+xml")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, err := w.Write(buf.Bytes())
	return err
}

// ServeSegment serves a specific segment by name.
func (p *DASHProcessor) ServeSegment(w http.ResponseWriter, r *http.Request, segmentName string) error {
	// Handle init segment
	if segmentName == "init.mp4" {
		p.initSegmentMu.RLock()
		initSeg := p.initSegment
		p.initSegmentMu.RUnlock()

		if initSeg == nil {
			http.Error(w, "Init segment not ready", http.StatusServiceUnavailable)
			return ErrDASHInitSegmentNotReady
		}

		// Handle conditional request to avoid duplicate MOOV atoms on client reconnects
		if initSeg.ETag != "" {
			w.Header().Set("ETag", initSeg.ETag)
			if r.Header.Get("If-None-Match") == initSeg.ETag {
				w.WriteHeader(http.StatusNotModified)
				return nil
			}
		}

		p.SetStreamHeaders(w)
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(initSeg.Data)))
		w.Header().Set("Cache-Control", "public, max-age=31536000")
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
	var segment *dashSegment
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
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	_, err = w.Write(segment.data)
	return err
}

// IsFMP4Mode returns true since DASH uses fMP4.
func (p *DASHProcessor) IsFMP4Mode() bool {
	return true
}

// GetInitSegment returns the initialization segment.
func (p *DASHProcessor) GetInitSegment() *InitSegment {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()
	return p.initSegment
}

// HasInitSegment returns true if the init segment is available.
func (p *DASHProcessor) HasInitSegment() bool {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()
	return p.initSegment != nil
}

// GetFilteredInitSegment returns an init segment containing only the specified track type.
// trackType should be "video" or "audio". Returns the full init segment if trackType is empty.
// This method parses the existing muxed init segment and filters out tracks, preserving
// the original track IDs so that segments (which contain both tracks) can be properly
// demuxed by the client.
func (p *DASHProcessor) GetFilteredInitSegment(trackType string) ([]byte, error) {
	p.initSegmentMu.RLock()
	defer p.initSegmentMu.RUnlock()

	if p.initSegment == nil {
		p.config.Logger.Debug("GetFilteredInitSegment: no init segment available",
			slog.String("track_type", trackType))
		return nil, fmt.Errorf("no init segment available")
	}

	p.config.Logger.Debug("GetFilteredInitSegment: processing request",
		slog.String("track_type", trackType),
		slog.Int("init_data_len", len(p.initSegment.Data)))

	// If no filter, return full init segment
	if trackType == "" {
		return p.initSegment.Data, nil
	}

	// Parse the existing muxed init segment to preserve track IDs
	var parsedInit fmp4.Init
	if err := parsedInit.Unmarshal(bytes.NewReader(p.initSegment.Data)); err != nil {
		p.config.Logger.Debug("GetFilteredInitSegment: failed to parse init segment",
			slog.String("track_type", trackType),
			slog.String("error", err.Error()),
			slog.Int("init_data_len", len(p.initSegment.Data)))
		return nil, fmt.Errorf("parsing init segment: %w", err)
	}

	p.config.Logger.Debug("GetFilteredInitSegment: parsed init segment",
		slog.String("track_type", trackType),
		slog.Int("track_count", len(parsedInit.Tracks)))

	// Filter tracks based on type, preserving original track IDs
	filteredInit := fmp4.Init{
		Tracks: make([]*fmp4.InitTrack, 0),
	}

	for _, track := range parsedInit.Tracks {
		isVideo := false
		isAudio := false

		switch track.Codec.(type) {
		case *mp4.CodecH264, *mp4.CodecH265, *mp4.CodecAV1, *mp4.CodecVP9:
			isVideo = true
		case *mp4.CodecMPEG4Audio, *mp4.CodecAC3, *mp4.CodecEAC3, *mp4.CodecOpus, *mp4.CodecMPEG1Audio:
			isAudio = true
		}

		if (trackType == "video" && isVideo) || (trackType == "audio" && isAudio) {
			filteredInit.Tracks = append(filteredInit.Tracks, track)
		}
	}

	if len(filteredInit.Tracks) == 0 {
		return nil, fmt.Errorf("no %s track found in init segment", trackType)
	}

	// Marshal the filtered init segment (track IDs are preserved)
	var buf seekablebuffer.Buffer
	if err := filteredInit.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling filtered init: %w", err)
	}

	return buf.Bytes(), nil
}

// GetSegmentInfos implements SegmentProvider.
// Returns only the latest PlaylistSegments segments so new clients start near live edge.
func (p *DASHProcessor) GetSegmentInfos() []SegmentInfo {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()

	if len(p.segments) == 0 {
		return nil
	}

	// Determine how many segments to include in manifest
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
func (p *DASHProcessor) GetSegment(sequence uint64) (*Segment, error) {
	p.segmentsMu.RLock()
	defer p.segmentsMu.RUnlock()

	for _, seg := range p.segments {
		if seg.sequence == sequence {
			return &Segment{
				Sequence:     seg.sequence,
				Duration:     seg.duration,
				Data:         seg.data,
				Timestamp:    seg.createdAt,
				IsFragmented: true, // DASH always produces fMP4 segments
			}, nil
		}
	}

	// Log available segments when requested segment is not found
	if len(p.segments) > 0 {
		minSeq := p.segments[0].sequence
		maxSeq := p.segments[len(p.segments)-1].sequence
		p.config.Logger.Info("DASH segment not found - buffer state",
			slog.String("processor_id", p.id),
			slog.Uint64("requested_seq", sequence),
			slog.Int("buffer_count", len(p.segments)),
			slog.Uint64("buffer_min_seq", minSeq),
			slog.Uint64("buffer_max_seq", maxSeq),
			slog.Uint64("next_sequence", p.nextSequence))
	} else {
		p.config.Logger.Info("DASH segment not found - buffer empty",
			slog.String("processor_id", p.id),
			slog.Uint64("requested_seq", sequence),
			slog.Uint64("next_sequence", p.nextSequence))
	}

	return nil, ErrSegmentNotFound
}

// TargetDuration implements SegmentProvider.
func (p *DASHProcessor) TargetDuration() int {
	return int(p.config.TargetSegmentDuration + 0.5)
}

// GetStreamStartTime returns the time when the first segment was created.
// This is used for availabilityStartTime in DASH manifests which must be constant.
func (p *DASHProcessor) GetStreamStartTime() time.Time {
	p.streamStartTimeMu.RLock()
	defer p.streamStartTimeMu.RUnlock()
	return p.streamStartTime
}

// RecordPlaylistRequest implements PlaylistActivityRecorder interface.
// Records that a manifest was requested, updating the processor's last activity time.
func (p *DASHProcessor) RecordPlaylistRequest() {
	p.lastActivity.Store(time.Now())
}

// RecordSegmentRequest implements SegmentActivityRecorder interface.
// Records that a segment was requested, updating the processor's last activity time.
// This is critical for DASH clients that may buffer segments and not request manifests frequently.
func (p *DASHProcessor) RecordSegmentRequest() {
	p.lastActivity.Store(time.Now())
}

// initNewSegment initializes a new segment accumulator.
func (p *DASHProcessor) initNewSegment() {
	p.currentSegment.startPTS = -1
	p.currentSegment.endPTS = -1
	p.currentSegment.hasVideo = false
	p.currentSegment.hasAudio = false
	p.currentSegment.startTime = time.Now()
}

// runProcessingLoop is the main processing loop.
func (p *DASHProcessor) runProcessingLoop(esVariant *ESVariant) {
	ctx := p.Context()
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	p.config.Logger.Info("DASH processor loop started, waiting for keyframe",
		slog.String("id", p.id),
		slog.String("variant", p.variant.String()),
		slog.Bool("has_video_track", videoTrack != nil),
		slog.Bool("has_audio_track", audioTrack != nil))

	// Wait for initial video keyframe using base class helper
	if _, ok := p.WaitForKeyframe(videoTrack); !ok {
		p.config.Logger.Warn("DASH processor: keyframe wait failed, exiting loop",
			slog.String("id", p.id))
		return
	}

	p.config.Logger.Info("DASH processor: received first keyframe, starting segment accumulation",
		slog.String("id", p.id),
		slog.String("variant", p.variant.String()))

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
					p.flushSegment(videoSamples, audioSamples)
					p.initNewSegment()
					videoSamples = nil
					audioSamples = nil
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
			var newAudioSamples []ESSample
			if audioTrack != nil {
				newAudioSamples = audioTrack.ReadFrom(p.LastAudioSeq(), 200)
				for _, sample := range newAudioSamples {
					bytesRead += uint64(len(sample.Data))
					audioSamples = append(audioSamples, sample)
					p.SetLastAudioSeq(sample.Sequence)
					p.currentSegment.hasAudio = true
					if p.currentSegment.startPTS < 0 {
						p.currentSegment.startPTS = sample.PTS
					}
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
				p.flushSegment(videoSamples, audioSamples)
				p.initNewSegment()
				videoSamples = nil
				audioSamples = nil
			}
		}
	}
}

// hasEnoughContent returns true if we have enough content for a segment.
func (p *DASHProcessor) hasEnoughContent() bool {
	if !p.currentSegment.hasVideo {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	return elapsed >= p.config.TargetSegmentDuration
}

// shouldFinalizeSegment returns true if current segment should be finalized.
func (p *DASHProcessor) shouldFinalizeSegment() bool {
	if !p.currentSegment.hasVideo && !p.currentSegment.hasAudio {
		return false
	}

	elapsed := time.Since(p.currentSegment.startTime).Seconds()
	// Finalize if we've exceeded target duration by 50%
	return elapsed >= p.config.TargetSegmentDuration*1.5
}

// flushSegment finalizes the current segment and adds it to the list.
func (p *DASHProcessor) flushSegment(videoSamples, audioSamples []ESSample) {
	if len(videoSamples) == 0 && len(audioSamples) == 0 {
		return
	}

	// Extract codec parameters if not already done
	if len(videoSamples) > 0 {
		p.adapter.UpdateVideoParams(videoSamples)
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
		if err := p.generateInitSegment(len(videoSamples) > 0, hasAudio); err != nil {
			p.config.Logger.Error("Failed to generate DASH init segment",
				slog.String("error", err.Error()))
			return
		}
	}

	// Convert ES samples to fMP4 samples
	fmp4VideoSamples, videoBaseTime := p.adapter.ConvertVideoSamples(videoSamples)
	fmp4AudioSamples, audioBaseTime := p.adapter.ConvertAudioSamples(audioSamples)

	// Initialize time offsets on first segment to normalize timestamps to start from 0
	// This is critical for DASH because the manifest's SegmentTemplate assumes segments
	// start at time 0, but source streams may have arbitrary starting timestamps.
	p.timeOffsetInitMu.Lock()
	if !p.timeOffsetInitSet {
		p.videoTimeOffset = videoBaseTime
		p.audioTimeOffset = audioBaseTime
		p.timeOffsetInitSet = true
		p.config.Logger.Debug("DASH timestamp offset initialized",
			slog.Uint64("video_offset", p.videoTimeOffset),
			slog.Uint64("audio_offset", p.audioTimeOffset))
	}
	// Apply offset to normalize timestamps
	normalizedVideoTime := videoBaseTime - p.videoTimeOffset
	normalizedAudioTime := audioBaseTime - p.audioTimeOffset
	p.timeOffsetInitMu.Unlock()

	// Log sample PTS/DTS for timestamp debugging
	var firstVideoPTS, firstVideoDTS, lastVideoPTS int64
	if len(videoSamples) > 0 {
		firstVideoPTS = videoSamples[0].PTS
		firstVideoDTS = videoSamples[0].DTS
		lastVideoPTS = videoSamples[len(videoSamples)-1].PTS
	}
	var firstAudioPTS, lastAudioPTS int64
	if len(audioSamples) > 0 {
		firstAudioPTS = audioSamples[0].PTS
		lastAudioPTS = audioSamples[len(audioSamples)-1].PTS
	}

	// Detect potential timestamp discontinuity (base time less than offset)
	var timestampDiscontinuity bool
	if p.timeOffsetInitSet && (videoBaseTime < p.videoTimeOffset || audioBaseTime < p.audioTimeOffset) {
		timestampDiscontinuity = true
		p.config.Logger.Warn("DASH timestamp discontinuity detected",
			slog.String("processor_id", p.id),
			slog.Uint64("video_base_time", videoBaseTime),
			slog.Uint64("video_offset", p.videoTimeOffset),
			slog.Uint64("audio_base_time", audioBaseTime),
			slog.Uint64("audio_offset", p.audioTimeOffset),
			slog.Uint64("next_sequence", p.nextSequence))
	}

	// Debug: Log sample counts for each segment
	var videoFormat string
	if len(videoSamples) > 0 && len(videoSamples[0].Data) >= 4 {
		d := videoSamples[0].Data
		if d[0] == 0 && d[1] == 0 && (d[2] == 1 || (d[2] == 0 && d[3] == 1)) {
			videoFormat = "annexb"
		} else {
			videoFormat = "avcc"
		}
	}

	// INFO-level timestamp tracking for debugging
	p.config.Logger.Info("DASH segment timestamp info",
		slog.String("processor_id", p.id),
		slog.Uint64("sequence", p.nextSequence),
		slog.Int64("video_first_pts", firstVideoPTS),
		slog.Int64("video_first_dts", firstVideoDTS),
		slog.Int64("video_last_pts", lastVideoPTS),
		slog.Int64("audio_first_pts", firstAudioPTS),
		slog.Int64("audio_last_pts", lastAudioPTS),
		slog.Uint64("video_base_time", videoBaseTime),
		slog.Uint64("audio_base_time", audioBaseTime),
		slog.Uint64("video_offset", p.videoTimeOffset),
		slog.Uint64("audio_offset", p.audioTimeOffset),
		slog.Uint64("normalized_video_time", normalizedVideoTime),
		slog.Uint64("normalized_audio_time", normalizedAudioTime),
		slog.Bool("discontinuity", timestampDiscontinuity),
		slog.Int("video_samples", len(videoSamples)),
		slog.Int("audio_samples", len(audioSamples)))

	p.config.Logger.Debug("DASH segment samples",
		slog.Int("video_es_samples", len(videoSamples)),
		slog.Int("audio_es_samples", len(audioSamples)),
		slog.Int("video_fmp4_samples", len(fmp4VideoSamples)),
		slog.Int("audio_fmp4_samples", len(fmp4AudioSamples)),
		slog.Uint64("video_base_time", normalizedVideoTime),
		slog.Uint64("audio_base_time", normalizedAudioTime),
		slog.String("video_format", videoFormat))

	// Generate fragment using mediacommon with normalized timestamps
	fragmentData, err := p.writer.GeneratePart(fmp4VideoSamples, fmp4AudioSamples, normalizedVideoTime, normalizedAudioTime)
	if err != nil {
		p.config.Logger.Error("Failed to generate DASH fragment",
			slog.String("error", err.Error()))
		return
	}

	if len(fragmentData) == 0 {
		return
	}

	// Calculate duration
	duration := p.calculateDuration(videoSamples, audioSamples)
	if duration < 0.1 {
		return // Too short, skip
	}

	// Create segment
	seg := &dashSegment{
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

	p.config.Logger.Info("Created DASH segment",
		slog.String("processor_id", p.id),
		slog.String("variant", p.variant.String()),
		slog.Uint64("sequence", seg.sequence),
		slog.Float64("duration", seg.duration),
		slog.Int("size", len(seg.data)),
		slog.Int("buffer_count", len(p.segments)))

	// Update stats
	p.RecordBytesWritten(uint64(len(seg.data)))
}

// generateInitSegment creates the initialization segment.
func (p *DASHProcessor) generateInitSegment(hasVideo, hasAudio bool) error {
	// Configure the writer with codec parameters
	if err := p.adapter.ConfigureWriter(p.writer); err != nil {
		return fmt.Errorf("configuring writer: %w", err)
	}

	// Lock parameters after first init generation
	p.adapter.LockParams()

	// Generate init segment using mediacommon
	initData, err := p.writer.GenerateInit(hasVideo, hasAudio, 90000, 90000)
	if err != nil {
		return fmt.Errorf("generating init: %w", err)
	}

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

	p.config.Logger.Debug("Generated DASH init segment",
		slog.Int("size", len(initData)),
		slog.String("etag", etag))

	return nil
}

// calculateDuration calculates segment duration from samples.
func (p *DASHProcessor) calculateDuration(videoSamples, audioSamples []ESSample) float64 {
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
