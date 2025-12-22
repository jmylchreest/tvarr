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
		TargetSegmentDuration: 6.0,
		MaxSegments:           7,
		PlaylistSegments:      3, // New clients start near live edge
		MinBufferTime:         6.0,
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
	*BaseProcessor

	config   DASHProcessorConfig
	esBuffer *SharedESBuffer
	variant  CodecVariant

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

	// ES reading state
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Reference to the ES variant for consumer tracking
	esVariant *ESVariant

	// fMP4 muxer using mediacommon
	writer  *FMP4Writer
	adapter *ESSampleAdapter

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
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

	base := NewBaseProcessor(id, OutputFormatDASH, nil)

	adapter := NewESSampleAdapter(DefaultESSampleAdapterConfig())
	// Set audio codec from variant so we correctly handle Opus, AC3, MP3, etc.
	// This is essential for non-AAC codecs since we can't detect them from ES samples.
	adapter.SetAudioCodecFromVariant(variant.AudioCodec())

	p := &DASHProcessor{
		BaseProcessor: base,
		config:        config,
		esBuffer:      esBuffer,
		variant:       variant,
		segments:      make([]*dashSegment, 0, config.MaxSegments),
		segmentNotify: make(chan struct{}, 1),
		writer:        NewFMP4Writer(),
		adapter:       adapter,
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
		case <-p.ctx.Done():
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

	// Initialize segment accumulator
	p.initNewSegment()

	p.config.Logger.Debug("Starting DASH processor",
		slog.String("id", p.id),
		slog.String("variant", p.variant.String()))

	// Start processing loop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runProcessingLoop(esVariant)
	}()

	return nil
}

// Stop stops the processor and cleans up resources.
func (p *DASHProcessor) Stop() {
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
func (p *DASHProcessor) RegisterClient(clientID string, w http.ResponseWriter, r *http.Request) error {
	_ = p.RegisterClientBase(clientID, w, r)
	return nil
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

	// Build MPD manifest
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `)
	buf.WriteString(`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" `)
	buf.WriteString(`xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" `)
	buf.WriteString(`type="dynamic" `)
	buf.WriteString(`profiles="urn:mpeg:dash:profile:isoff-live:2011" `)
	buf.WriteString(fmt.Sprintf(`minBufferTime="PT%.1fS" `, p.config.MinBufferTime))
	buf.WriteString(fmt.Sprintf(`minimumUpdatePeriod="PT%.1fS" `, p.config.TargetSegmentDuration))
	buf.WriteString(`availabilityStartTime="1970-01-01T00:00:00Z">`)
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
func (p *DASHProcessor) TargetDuration() int {
	return int(p.config.TargetSegmentDuration + 0.5)
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
	videoTrack := esVariant.VideoTrack()
	audioTrack := esVariant.AudioTrack()

	// Wait for initial video keyframe
	p.config.Logger.Debug("Waiting for initial keyframe")
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-videoTrack.NotifyChan():
		}

		samples := videoTrack.ReadFromKeyframe(p.lastVideoSeq, 1)
		if len(samples) > 0 {
			p.lastVideoSeq = samples[0].Sequence - 1
			break
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Accumulators for current segment
	var videoSamples []ESSample
	var audioSamples []ESSample

	for {
		select {
		case <-p.ctx.Done():
			// Flush any remaining segment
			if len(videoSamples) > 0 || len(audioSamples) > 0 {
				p.flushSegment(videoSamples, audioSamples)
			}
			return

		case <-ticker.C:
			// Read video samples
			newVideoSamples := videoTrack.ReadFrom(p.lastVideoSeq, 100)
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
				p.lastVideoSeq = sample.Sequence
				p.currentSegment.hasVideo = true
				if p.currentSegment.startPTS < 0 {
					p.currentSegment.startPTS = sample.PTS
				}
				p.currentSegment.endPTS = sample.PTS
			}

			// Read audio samples
			newAudioSamples := audioTrack.ReadFrom(p.lastAudioSeq, 200)
			for _, sample := range newAudioSamples {
				bytesRead += uint64(len(sample.Data))
				audioSamples = append(audioSamples, sample)
				p.lastAudioSeq = sample.Sequence
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
			if p.esVariant != nil && (len(newVideoSamples) > 0 || len(newAudioSamples) > 0) {
				p.esVariant.UpdateConsumerPosition(p.id, p.lastVideoSeq, p.lastAudioSeq)
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
		if err := p.generateInitSegment(len(videoSamples) > 0, len(audioSamples) > 0); err != nil {
			p.config.Logger.Error("Failed to generate DASH init segment",
				slog.String("error", err.Error()))
			return
		}
	}

	// Convert ES samples to fMP4 samples
	fmp4VideoSamples, videoBaseTime := p.adapter.ConvertVideoSamples(videoSamples)
	fmp4AudioSamples, audioBaseTime := p.adapter.ConvertAudioSamples(audioSamples)

	// Generate fragment using mediacommon
	fragmentData, err := p.writer.GeneratePart(fmp4VideoSamples, fmp4AudioSamples, videoBaseTime, audioBaseTime)
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

	p.config.Logger.Debug("Created DASH segment",
		slog.Uint64("sequence", seg.sequence),
		slog.Float64("duration", seg.duration),
		slog.Int("size", len(seg.data)))

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
