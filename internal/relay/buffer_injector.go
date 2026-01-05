// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/jmylchreest/tvarr/internal/relay/placeholders"
)

// PlaceholderType identifies the type of placeholder content.
type PlaceholderType string

const (
	// PlaceholderStarting is shown while the stream is starting up.
	PlaceholderStarting PlaceholderType = "starting"
	// PlaceholderUnavailable is shown when the origin stream is unavailable.
	PlaceholderUnavailable PlaceholderType = "unavailable"
	// PlaceholderLimitReached is shown when concurrent stream limits are hit.
	PlaceholderLimitReached PlaceholderType = "limit_reached"
	// PlaceholderEnded is shown when the stream has ended.
	PlaceholderEnded PlaceholderType = "ended"
)

// CachedPlaceholder holds pre-parsed ES samples for a codec variant.
type CachedPlaceholder struct {
	VideoSamples  []ESSample
	AudioSamples  []ESSample
	VideoInitData []byte
	AudioInitData []byte
	Duration      time.Duration // Actual duration of the placeholder
}

// BufferInjector injects placeholder ES samples into variant buffers.
// It caches demuxed samples from embedded fMP4 files for efficient injection.
// Supported codecs: H.264/AAC, H.265/AAC, VP9/Opus.
// Note: AV1 placeholders are embedded but fMP4 parsing has limited support in mediacommon.
type BufferInjector struct {
	mu     sync.RWMutex
	cache  map[CodecVariant]*CachedPlaceholder
	logger *slog.Logger
}

// NewBufferInjector creates a new buffer injector.
func NewBufferInjector(logger *slog.Logger) *BufferInjector {
	if logger == nil {
		logger = slog.Default()
	}
	return &BufferInjector{
		cache:  make(map[CodecVariant]*CachedPlaceholder),
		logger: logger,
	}
}

// GetPlaceholder returns cached placeholder samples for the variant, parsing on first access.
func (b *BufferInjector) GetPlaceholder(variant CodecVariant) (*CachedPlaceholder, error) {
	b.mu.RLock()
	cached, ok := b.cache[variant]
	b.mu.RUnlock()

	if ok {
		return cached, nil
	}

	// Parse and cache
	return b.loadPlaceholder(variant)
}

// loadPlaceholder parses the embedded fMP4 and caches the ES samples.
func (b *BufferInjector) loadPlaceholder(variant CodecVariant) (*CachedPlaceholder, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check under write lock
	if cached, ok := b.cache[variant]; ok {
		return cached, nil
	}

	videoCodec := variant.VideoCodec()
	audioCodec := variant.AudioCodec()

	// Get embedded placeholder data
	data := placeholders.GetPlaceholder(videoCodec, audioCodec)
	if data == nil {
		return nil, fmt.Errorf("no placeholder available for variant %s", variant)
	}

	b.logger.Debug("Loading placeholder for variant",
		slog.String("variant", string(variant)),
		slog.Int("data_size", len(data)))

	// Demux the fMP4 to extract ES samples
	cached, err := b.demuxFMP4(data, variant)
	if err != nil {
		return nil, fmt.Errorf("failed to demux placeholder: %w", err)
	}

	b.cache[variant] = cached

	b.logger.Info("Loaded placeholder content",
		slog.String("variant", string(variant)),
		slog.Int("video_samples", len(cached.VideoSamples)),
		slog.Int("audio_samples", len(cached.AudioSamples)),
		slog.Duration("duration", cached.Duration))

	return cached, nil
}

// demuxFMP4 extracts ES samples from an fMP4 container using mediacommon.
func (b *BufferInjector) demuxFMP4(data []byte, variant CodecVariant) (*CachedPlaceholder, error) {
	cached := &CachedPlaceholder{
		VideoSamples: make([]ESSample, 0, 30), // ~1 second at 25fps
		AudioSamples: make([]ESSample, 0, 50), // ~1 second at 48kHz
	}

	// Parse the init segment first
	reader := bytes.NewReader(data)
	var init fmp4.Init
	if err := init.Unmarshal(reader); err != nil {
		return nil, fmt.Errorf("failed to parse init segment: %w", err)
	}

	// Find video and audio tracks and extract init data
	var videoTrackID, audioTrackID int
	var videoTimescale, audioTimescale uint32

	for _, track := range init.Tracks {
		switch track.Codec.(type) {
		case *fmp4.CodecH264:
			videoTrackID = track.ID
			videoTimescale = track.TimeScale
			h264 := track.Codec.(*fmp4.CodecH264)
			// Build SPS/PPS init data
			cached.VideoInitData = buildH264InitData(h264.SPS, h264.PPS)
		case *fmp4.CodecH265:
			videoTrackID = track.ID
			videoTimescale = track.TimeScale
			h265 := track.Codec.(*fmp4.CodecH265)
			cached.VideoInitData = buildH265InitData(h265.VPS, h265.SPS, h265.PPS)
		case *fmp4.CodecVP9:
			videoTrackID = track.ID
			videoTimescale = track.TimeScale
			// VP9 doesn't have separate init data
		case *fmp4.CodecAV1:
			videoTrackID = track.ID
			videoTimescale = track.TimeScale
			av1 := track.Codec.(*fmp4.CodecAV1)
			cached.VideoInitData = av1.SequenceHeader
		case *fmp4.CodecMPEG4Audio:
			audioTrackID = track.ID
			audioTimescale = track.TimeScale
			mpeg4 := track.Codec.(*fmp4.CodecMPEG4Audio)
			cached.AudioInitData, _ = mpeg4.Config.Marshal()
		case *fmp4.CodecOpus:
			audioTrackID = track.ID
			audioTimescale = track.TimeScale
			// Opus init data is in the Opus header within the track
		}
	}

	// Default timescales
	if videoTimescale == 0 {
		videoTimescale = 90000
	}
	if audioTimescale == 0 {
		audioTimescale = 48000
	}

	b.logger.Debug("Found tracks in fMP4",
		slog.Int("video_track", videoTrackID),
		slog.Int("audio_track", audioTrackID),
		slog.Uint64("video_timescale", uint64(videoTimescale)),
		slog.Uint64("audio_timescale", uint64(audioTimescale)))

	// Parse the media parts (moof+mdat fragments)
	var parts fmp4.Parts
	if err := parts.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("failed to parse parts: %w", err)
	}

	// Extract samples from parts
	var maxVideoPTS, maxAudioPTS int64

	for _, part := range parts {
		for _, track := range part.Tracks {
			isVideo := track.ID == videoTrackID
			isAudio := track.ID == audioTrackID

			if !isVideo && !isAudio {
				continue
			}

			baseTime := track.BaseTime
			currentTime := baseTime

			for _, sample := range track.Samples {
				var pts, dts int64

				if isVideo {
					// Convert to 90kHz timescale
					dts = int64(currentTime * 90000 / uint64(videoTimescale))
					pts = dts + int64(sample.PTSOffset)*90000/int64(videoTimescale)

					isKeyframe := !sample.IsNonSyncSample

					cached.VideoSamples = append(cached.VideoSamples, ESSample{
						PTS:        pts,
						DTS:        dts,
						Data:       sample.Payload,
						IsKeyframe: isKeyframe,
					})
					if pts > maxVideoPTS {
						maxVideoPTS = pts
					}
				} else if isAudio {
					// Convert to 90kHz timescale
					pts = int64(currentTime * 90000 / uint64(audioTimescale))
					dts = pts

					cached.AudioSamples = append(cached.AudioSamples, ESSample{
						PTS:        pts,
						DTS:        dts,
						Data:       sample.Payload,
						IsKeyframe: true,
					})
					if pts > maxAudioPTS {
						maxAudioPTS = pts
					}
				}

				currentTime += uint64(sample.Duration)
			}
		}
	}

	// Calculate duration from max PTS (90kHz timescale)
	maxPTS := max(maxAudioPTS, maxVideoPTS)
	cached.Duration = time.Duration(maxPTS) * time.Second / 90000

	return cached, nil
}

// buildH264InitData creates SPS/PPS init data for H.264.
func buildH264InitData(sps, pps []byte) []byte {
	// Format: NAL length prefix (4 bytes) + NAL unit for each
	if len(sps) == 0 || len(pps) == 0 {
		return nil
	}
	result := make([]byte, 0, 8+len(sps)+len(pps))
	// SPS
	result = append(result, byte(len(sps)>>24), byte(len(sps)>>16), byte(len(sps)>>8), byte(len(sps)))
	result = append(result, sps...)
	// PPS
	result = append(result, byte(len(pps)>>24), byte(len(pps)>>16), byte(len(pps)>>8), byte(len(pps)))
	result = append(result, pps...)
	return result
}

// buildH265InitData creates VPS/SPS/PPS init data for H.265.
func buildH265InitData(vps, sps, pps []byte) []byte {
	if len(vps) == 0 || len(sps) == 0 || len(pps) == 0 {
		return nil
	}
	result := make([]byte, 0, 12+len(vps)+len(sps)+len(pps))
	// VPS
	result = append(result, byte(len(vps)>>24), byte(len(vps)>>16), byte(len(vps)>>8), byte(len(vps)))
	result = append(result, vps...)
	// SPS
	result = append(result, byte(len(sps)>>24), byte(len(sps)>>16), byte(len(sps)>>8), byte(len(sps)))
	result = append(result, sps...)
	// PPS
	result = append(result, byte(len(pps)>>24), byte(len(pps)>>16), byte(len(pps)>>8), byte(len(pps)))
	result = append(result, pps...)
	return result
}

// InjectStartupPlaceholder injects placeholder content into the variant buffer
// to provide immediate playback while the transcoder starts.
// The samples are looped to fill the target duration.
func (b *BufferInjector) InjectStartupPlaceholder(variant *ESVariant, targetDuration time.Duration) error {
	variantKey := variant.Variant()

	cached, err := b.GetPlaceholder(variantKey)
	if err != nil {
		return err
	}

	if len(cached.VideoSamples) == 0 {
		return errors.New("placeholder has no video samples")
	}

	// Set init data if not already set
	if variant.VideoTrack().GetInitData() == nil && len(cached.VideoInitData) > 0 {
		variant.VideoTrack().SetInitData(cached.VideoInitData)
	}
	if variant.AudioTrack().GetInitData() == nil && len(cached.AudioInitData) > 0 {
		variant.AudioTrack().SetInitData(cached.AudioInitData)
	}

	// Calculate how many loops needed to fill target duration
	loopCount := max(int(targetDuration/cached.Duration), 1)

	// Calculate PTS increment per loop (in 90kHz timescale)
	loopDurationPTS := int64(cached.Duration.Seconds() * 90000)

	b.logger.Debug("Injecting placeholder content",
		slog.String("variant", string(variantKey)),
		slog.Duration("target_duration", targetDuration),
		slog.Duration("placeholder_duration", cached.Duration),
		slog.Int("loop_count", loopCount))

	// Inject video samples
	videoTrack := variant.VideoTrack()
	for loop := 0; loop < loopCount; loop++ {
		ptsOffset := int64(loop) * loopDurationPTS
		for _, sample := range cached.VideoSamples {
			videoTrack.Write(
				sample.PTS+ptsOffset,
				sample.DTS+ptsOffset,
				sample.Data,
				sample.IsKeyframe,
			)
		}
	}

	// Inject audio samples
	audioTrack := variant.AudioTrack()
	for loop := 0; loop < loopCount; loop++ {
		ptsOffset := int64(loop) * loopDurationPTS
		for _, sample := range cached.AudioSamples {
			audioTrack.Write(
				sample.PTS+ptsOffset,
				sample.DTS+ptsOffset,
				sample.Data,
				sample.IsKeyframe,
			)
		}
	}

	b.logger.Info("Injected placeholder content",
		slog.String("variant", string(variantKey)),
		slog.Int("video_samples", len(cached.VideoSamples)*loopCount),
		slog.Int("audio_samples", len(cached.AudioSamples)*loopCount),
		slog.Duration("total_duration", cached.Duration*time.Duration(loopCount)))

	return nil
}

// HasPlaceholder returns true if a placeholder is available for the variant.
func (b *BufferInjector) HasPlaceholder(variant CodecVariant) bool {
	return placeholders.HasPlaceholder(variant.VideoCodec(), variant.AudioCodec())
}

// AvailableVariants returns codec variants that have placeholder content.
func (b *BufferInjector) AvailableVariants() []CodecVariant {
	variants := placeholders.AvailableVariants()
	result := make([]CodecVariant, len(variants))
	for i, v := range variants {
		result[i] = CodecVariant(v)
	}
	return result
}

// Global injector instance (lazy-initialized)
var (
	globalInjector     *BufferInjector
	globalInjectorOnce sync.Once
)

// GetBufferInjector returns the global buffer injector instance.
func GetBufferInjector() *BufferInjector {
	globalInjectorOnce.Do(func() {
		globalInjector = NewBufferInjector(slog.Default())
	})
	return globalInjector
}
