// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// PlaceholderSegmentGenerator generates fMP4 segments on-demand from cached placeholder content.
// This allows serving placeholder content for any segment number until real content arrives.
type PlaceholderSegmentGenerator struct {
	variant CodecVariant
	logger  *slog.Logger

	// Cached placeholder data
	cached *CachedPlaceholder

	// Pre-generated init segment for this variant
	initSegment *InitSegment

	// fMP4 writer for generating segments
	writer  *FMP4Writer
	adapter *ESSampleAdapter

	// Target segment duration in seconds
	targetDuration float64

	mu sync.RWMutex
}

// NewPlaceholderSegmentGenerator creates a generator for the given codec variant.
func NewPlaceholderSegmentGenerator(variant CodecVariant, targetDuration float64, logger *slog.Logger) (*PlaceholderSegmentGenerator, error) {
	if logger == nil {
		logger = slog.Default()
	}

	injector := GetBufferInjector()
	if !injector.HasPlaceholder(variant) {
		return nil, fmt.Errorf("no placeholder available for variant %s", variant)
	}

	cached, err := injector.GetPlaceholder(variant)
	if err != nil {
		return nil, fmt.Errorf("loading placeholder: %w", err)
	}

	if len(cached.VideoSamples) == 0 {
		return nil, fmt.Errorf("placeholder has no video samples")
	}

	gen := &PlaceholderSegmentGenerator{
		variant:        variant,
		logger:         logger,
		cached:         cached,
		targetDuration: targetDuration,
		writer:         NewFMP4Writer(),
		adapter:        NewESSampleAdapter(DefaultESSampleAdapterConfig()),
	}

	// Configure adapter with placeholder codec params
	if err := gen.initAdapter(); err != nil {
		return nil, fmt.Errorf("initializing adapter: %w", err)
	}

	// Generate init segment
	if err := gen.generateInitSegment(); err != nil {
		return nil, fmt.Errorf("generating init segment: %w", err)
	}

	logger.Info("Placeholder segment generator initialized",
		slog.String("variant", string(variant)),
		slog.Int("video_samples", len(cached.VideoSamples)),
		slog.Int("audio_samples", len(cached.AudioSamples)),
		slog.Duration("placeholder_duration", cached.Duration),
		slog.Float64("target_segment_duration", targetDuration))

	return gen, nil
}

// initAdapter configures the ES sample adapter with placeholder codec parameters.
func (g *PlaceholderSegmentGenerator) initAdapter() error {
	videoCodec := g.variant.VideoCodec()
	audioCodec := g.variant.AudioCodec()

	// Set video params directly from the cached init data
	// The placeholder's VideoInitData contains pre-extracted SPS/PPS/VPS
	if len(g.cached.VideoInitData) > 0 {
		switch videoCodec {
		case "h264":
			sps, pps := parseH264InitData(g.cached.VideoInitData)
			if sps != nil && pps != nil {
				g.writer.SetH264Params(sps, pps)
			}
		case "h265", "hevc":
			vps, sps, pps := parseH265InitData(g.cached.VideoInitData)
			if sps != nil && pps != nil {
				g.writer.SetH265Params(vps, sps, pps)
			}
		case "vp9":
			// VP9 params come from frame header, extract from samples
			if len(g.cached.VideoSamples) > 0 {
				g.adapter.UpdateVideoParams(g.cached.VideoSamples)
			}
		case "av1":
			// AV1 sequence header is in the init data
			g.writer.SetAV1Params(g.cached.VideoInitData)
		}
	} else if len(g.cached.VideoSamples) > 0 {
		// Fallback: try to extract from samples (works for VP9)
		g.adapter.UpdateVideoParams(g.cached.VideoSamples)
	}

	// Set audio params from cached init data or variant
	if len(g.cached.AudioInitData) > 0 && audioCodec == "aac" {
		// Parse AAC AudioSpecificConfig
		g.adapter.SetAudioCodecFromVariant(audioCodec)
		// The cached AudioInitData is the AudioSpecificConfig
		// We can use it to configure AAC if needed
	} else {
		g.adapter.SetAudioCodecFromVariant(audioCodec)
	}

	// Configure writer with any params set via adapter
	if err := g.adapter.ConfigureWriter(g.writer); err != nil {
		return fmt.Errorf("configuring writer: %w", err)
	}

	g.adapter.LockParams()
	return nil
}

// parseH264InitData extracts SPS and PPS from the placeholder's init data.
// The format is: [4-byte length][SPS][4-byte length][PPS]
func parseH264InitData(data []byte) (sps, pps []byte) {
	if len(data) < 8 {
		return nil, nil
	}

	offset := 0

	// Read SPS
	if offset+4 > len(data) {
		return nil, nil
	}
	spsLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+spsLen > len(data) {
		return nil, nil
	}
	sps = data[offset : offset+spsLen]
	offset += spsLen

	// Read PPS
	if offset+4 > len(data) {
		return sps, nil
	}
	ppsLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+ppsLen > len(data) {
		return sps, nil
	}
	pps = data[offset : offset+ppsLen]

	return sps, pps
}

// parseH265InitData extracts VPS, SPS and PPS from the placeholder's init data.
// The format is: [4-byte length][VPS][4-byte length][SPS][4-byte length][PPS]
func parseH265InitData(data []byte) (vps, sps, pps []byte) {
	if len(data) < 12 {
		return nil, nil, nil
	}

	offset := 0

	// Read VPS
	if offset+4 > len(data) {
		return nil, nil, nil
	}
	vpsLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+vpsLen > len(data) {
		return nil, nil, nil
	}
	vps = data[offset : offset+vpsLen]
	offset += vpsLen

	// Read SPS
	if offset+4 > len(data) {
		return vps, nil, nil
	}
	spsLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+spsLen > len(data) {
		return vps, nil, nil
	}
	sps = data[offset : offset+spsLen]
	offset += spsLen

	// Read PPS
	if offset+4 > len(data) {
		return vps, sps, nil
	}
	ppsLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+ppsLen > len(data) {
		return vps, sps, nil
	}
	pps = data[offset : offset+ppsLen]

	return vps, sps, pps
}

// generateInitSegment creates the fMP4 init segment for placeholder content.
func (g *PlaceholderSegmentGenerator) generateInitSegment() error {
	hasVideo := len(g.cached.VideoSamples) > 0
	hasAudio := len(g.cached.AudioSamples) > 0

	initData, err := g.writer.GenerateInit(hasVideo, hasAudio, 90000, 90000)
	if err != nil {
		return fmt.Errorf("generating init: %w", err)
	}

	g.initSegment = &InitSegment{
		Data: initData,
	}

	g.logger.Debug("Generated placeholder init segment",
		slog.Int("size", len(initData)))

	return nil
}

// GetInitSegment returns the pre-generated init segment.
func (g *PlaceholderSegmentGenerator) GetInitSegment() *InitSegment {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.initSegment
}

// GenerateSegment generates a placeholder segment for the given sequence number.
// The segment will have proper timing based on the sequence and target duration.
func (g *PlaceholderSegmentGenerator) GenerateSegment(sequence uint64) (*Segment, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Calculate timing for this segment
	// Each segment represents targetDuration seconds of content
	// Timestamps are in 90kHz timescale
	segmentStartPTS := int64(float64(sequence) * g.targetDuration * 90000)

	// Calculate how many placeholder loops needed to fill target duration
	loopCount := int(g.targetDuration / g.cached.Duration.Seconds())
	if loopCount < 1 {
		loopCount = 1
	}

	loopDurationPTS := int64(g.cached.Duration.Seconds() * 90000)

	// Build samples with adjusted timestamps for this segment
	videoSamples := make([]ESSample, 0, len(g.cached.VideoSamples)*loopCount)
	audioSamples := make([]ESSample, 0, len(g.cached.AudioSamples)*loopCount)

	for loop := 0; loop < loopCount; loop++ {
		loopOffset := int64(loop) * loopDurationPTS

		for _, sample := range g.cached.VideoSamples {
			videoSamples = append(videoSamples, ESSample{
				PTS:        segmentStartPTS + sample.PTS + loopOffset,
				DTS:        segmentStartPTS + sample.DTS + loopOffset,
				Data:       sample.Data,
				IsKeyframe: sample.IsKeyframe,
			})
		}

		for _, sample := range g.cached.AudioSamples {
			audioSamples = append(audioSamples, ESSample{
				PTS:        segmentStartPTS + sample.PTS + loopOffset,
				DTS:        segmentStartPTS + sample.DTS + loopOffset,
				Data:       sample.Data,
				IsKeyframe: true,
			})
		}
	}

	// Convert to fMP4 samples
	fmp4VideoSamples, videoBaseTime := g.adapter.ConvertVideoSamples(videoSamples)
	fmp4AudioSamples, audioBaseTime := g.adapter.ConvertAudioSamples(audioSamples)

	// Generate the fMP4 fragment
	fragmentData, err := g.writer.GeneratePart(fmp4VideoSamples, fmp4AudioSamples, videoBaseTime, audioBaseTime)
	if err != nil {
		return nil, fmt.Errorf("generating fragment: %w", err)
	}

	// Calculate actual duration
	duration := float64(loopCount) * g.cached.Duration.Seconds()
	if duration > g.targetDuration {
		duration = g.targetDuration
	}

	g.logger.Debug("Generated placeholder segment",
		slog.Uint64("sequence", sequence),
		slog.Float64("duration", duration),
		slog.Int("size", len(fragmentData)),
		slog.Int64("start_pts", segmentStartPTS))

	return &Segment{
		Sequence:     sequence,
		Duration:     duration,
		Data:         fragmentData,
		Timestamp:    time.Now(),
		IsFragmented: true,
	}, nil
}

// TargetDuration returns the target segment duration.
func (g *PlaceholderSegmentGenerator) TargetDuration() float64 {
	return g.targetDuration
}
