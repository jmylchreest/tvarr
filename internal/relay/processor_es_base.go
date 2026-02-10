// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
)

// ESProcessorConfig contains common configuration for ES-based processors.
type ESProcessorConfig struct {
	// Logger for structured logging.
	Logger *slog.Logger

	// AudioDetectTimeout is how long to wait for audio codec detection.
	// Default: 2 seconds.
	AudioDetectTimeout time.Duration

	// AudioDetectPollInterval is how often to poll for audio codec.
	// Default: 50ms.
	AudioDetectPollInterval time.Duration
}

// ESProcessorBase provides common functionality for processors that read from SharedESBuffer.
// It extracts duplicated code from HLS-TS, HLS-fMP4, DASH, and MPEG-TS processors.
type ESProcessorBase struct {
	*BaseProcessor

	// Configuration
	esConfig ESProcessorConfig

	// Buffer and variant
	esBuffer *SharedESBuffer
	variant  CodecVariant

	// Reference to the ES variant for consumer tracking
	esVariant *ESVariant

	// ES reading state
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Resolved codec names from the ES variant's tracks
	resolvedVideoCodec string
	resolvedAudioCodec string

	// AAC config for proper sample rate/channel count
	aacConfig *mpeg4audio.AudioSpecificConfig

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
}

// NewESProcessorBase creates a new ES processor base.
func NewESProcessorBase(
	id string,
	format OutputFormat,
	esBuffer *SharedESBuffer,
	variant CodecVariant,
	config ESProcessorConfig,
) *ESProcessorBase {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.AudioDetectTimeout == 0 {
		config.AudioDetectTimeout = 2 * time.Second
	}
	if config.AudioDetectPollInterval == 0 {
		config.AudioDetectPollInterval = 50 * time.Millisecond
	}

	return &ESProcessorBase{
		BaseProcessor: NewBaseProcessor(id, format),
		esConfig:      config,
		esBuffer:      esBuffer,
		variant:       variant,
	}
}

// InitES initializes the ES processor - gets variant, registers consumer, detects codecs.
// Returns the ESVariant to read from, or an error.
// This should be called at the start of Start() in concrete processors.
func (p *ESProcessorBase) InitES(ctx context.Context) (*ESVariant, error) {
	if !p.started.CompareAndSwap(false, true) {
		return nil, ErrProcessorAlreadyStarted
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.BaseProcessor.startedAt = time.Now()

	// Get or create the variant we'll read from
	esVariant, err := p.esBuffer.GetOrCreateVariantWithContext(p.ctx, p.variant)
	if err != nil {
		return nil, err
	}

	// Register with buffer
	p.esBuffer.RegisterProcessor(p.id)

	// Store variant reference and register as consumer
	p.esVariant = esVariant
	esVariant.RegisterConsumer(p.id)

	// Resolve codecs from the ES variant's tracks
	p.resolvedVideoCodec = esVariant.VideoTrack().Codec()
	p.resolvedAudioCodec = esVariant.AudioTrack().Codec()

	return esVariant, nil
}

// WaitForAudioCodec waits for audio codec to be detected on the variant.
// Returns the detected codec name, or empty string on timeout.
func (p *ESProcessorBase) WaitForAudioCodec() string {
	if p.resolvedAudioCodec != "" {
		return p.resolvedAudioCodec
	}

	p.esConfig.Logger.Debug("Waiting for audio codec detection")

	waitCtx, waitCancel := context.WithTimeout(p.ctx, p.esConfig.AudioDetectTimeout)
	defer waitCancel()

	ticker := time.NewTicker(p.esConfig.AudioDetectPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			p.esConfig.Logger.Debug("Audio codec detection timeout, proceeding without audio")
			return ""
		case <-ticker.C:
			codec := p.esVariant.AudioTrack().Codec()
			if codec != "" {
				p.resolvedAudioCodec = codec
				p.esConfig.Logger.Debug("Audio codec detected", slog.String("audio_codec", codec))
				return codec
			}
		}
	}
}

// WaitForAACInitData waits for AAC initialization data (AudioSpecificConfig).
// Returns the AAC config, or nil on timeout.
func (p *ESProcessorBase) WaitForAACInitData() *mpeg4audio.AudioSpecificConfig {
	if p.resolvedAudioCodec != "aac" {
		return nil
	}

	// Check if already available
	initData := p.esVariant.AudioTrack().GetInitData()
	if initData != nil {
		return p.parseAACConfig(initData)
	}

	p.esConfig.Logger.Debug("Waiting for AAC initData from demuxer")

	waitCtx, waitCancel := context.WithTimeout(p.ctx, p.esConfig.AudioDetectTimeout)
	defer waitCancel()

	ticker := time.NewTicker(p.esConfig.AudioDetectPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			p.esConfig.Logger.Debug("AAC initData timeout, using defaults")
			return nil
		case <-ticker.C:
			initData := p.esVariant.AudioTrack().GetInitData()
			if initData != nil {
				return p.parseAACConfig(initData)
			}
		}
	}
}

// parseAACConfig parses AAC initialization data.
func (p *ESProcessorBase) parseAACConfig(initData []byte) *mpeg4audio.AudioSpecificConfig {
	config := &mpeg4audio.AudioSpecificConfig{}
	if err := config.Unmarshal(initData); err != nil {
		p.esConfig.Logger.Debug("Failed to unmarshal AAC config from initData, using defaults",
			slog.String("error", err.Error()))
		return nil
	}
	p.esConfig.Logger.Debug("AAC config from initData",
		slog.Int("type", int(config.Type)),
		slog.Int("sample_rate", config.SampleRate),
		slog.Int("channel_count", config.ChannelCount))
	p.aacConfig = config
	return config
}

// WaitForKeyframe waits for the first keyframe on the video track.
// Returns the sequence number to start reading from (one before the keyframe).
// This should be called at the start of runProcessingLoop in concrete processors.
func (p *ESProcessorBase) WaitForKeyframe(videoTrack *ESTrack) (uint64, bool) {
	p.esConfig.Logger.Debug("Waiting for initial keyframe")

	for {
		// Try to read samples immediately (non-blocking check)
		samples := videoTrack.ReadFromKeyframe(p.lastVideoSeq, 1)
		if len(samples) > 0 {
			startSeq := samples[0].Sequence - 1
			p.lastVideoSeq = startSeq
			return startSeq, true
		}

		// No samples available, wait for notification or context cancellation
		select {
		case <-p.ctx.Done():
			return 0, false
		case <-videoTrack.NotifyChan():
			// New sample notification received, loop back to read
		}
	}
}

// ReadSamples reads available video and audio samples from the tracks.
// Returns video samples, audio samples, and total bytes read.
func (p *ESProcessorBase) ReadSamples(videoTrack, audioTrack *ESTrack, maxVideo, maxAudio int) ([]ESSample, []ESSample, uint64) {
	var bytesRead uint64

	// Read video samples
	videoSamples := videoTrack.ReadFrom(p.lastVideoSeq, maxVideo)
	for _, sample := range videoSamples {
		bytesRead += uint64(len(sample.Data))
		p.lastVideoSeq = sample.Sequence
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(p.lastAudioSeq, maxAudio)
	for _, sample := range audioSamples {
		bytesRead += uint64(len(sample.Data))
		p.lastAudioSeq = sample.Sequence
	}

	// Record access on the variant if we read any samples
	// This is used for variant cleanup - variants that aren't being read
	// from can be cleaned up along with their transcoders
	if len(videoSamples) > 0 || len(audioSamples) > 0 {
		if p.esVariant != nil {
			p.esVariant.RecordAccess()
		}
	}

	return videoSamples, audioSamples, bytesRead
}

// UpdateConsumerPosition updates the consumer position after reading samples.
// This allows the buffer to evict samples that have been read.
func (p *ESProcessorBase) UpdateConsumerPosition() {
	if p.esVariant != nil {
		p.esVariant.UpdateConsumerPosition(p.id, p.lastVideoSeq, p.lastAudioSeq)
	}
}

// StopES cleans up the ES processor - unregisters consumer, closes base.
// This should be called in Stop() of concrete processors.
func (p *ESProcessorBase) StopES() {
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

	p.esConfig.Logger.Debug("Processor stopped", slog.String("id", p.id))
}

// Context returns the processor's context.
func (p *ESProcessorBase) Context() context.Context {
	return p.ctx
}

// WaitGroup returns the processor's wait group for goroutine tracking.
func (p *ESProcessorBase) WaitGroup() *sync.WaitGroup {
	return &p.wg
}

// ESBuffer returns the shared ES buffer.
func (p *ESProcessorBase) ESBuffer() *SharedESBuffer {
	return p.esBuffer
}

// Variant returns the codec variant this processor reads from.
func (p *ESProcessorBase) Variant() CodecVariant {
	return p.variant
}

// ESVariant returns the ES variant being read from.
func (p *ESProcessorBase) ESVariant() *ESVariant {
	return p.esVariant
}

// ResolvedVideoCodec returns the resolved video codec name.
func (p *ESProcessorBase) ResolvedVideoCodec() string {
	return p.resolvedVideoCodec
}

// ResolvedAudioCodec returns the resolved audio codec name.
func (p *ESProcessorBase) ResolvedAudioCodec() string {
	return p.resolvedAudioCodec
}

// AACConfig returns the parsed AAC configuration, if available.
func (p *ESProcessorBase) AACConfig() *mpeg4audio.AudioSpecificConfig {
	return p.aacConfig
}

// LastVideoSeq returns the last video sequence number read.
func (p *ESProcessorBase) LastVideoSeq() uint64 {
	return p.lastVideoSeq
}

// LastAudioSeq returns the last audio sequence number read.
func (p *ESProcessorBase) LastAudioSeq() uint64 {
	return p.lastAudioSeq
}

// SetLastVideoSeq sets the last video sequence number.
func (p *ESProcessorBase) SetLastVideoSeq(seq uint64) {
	p.lastVideoSeq = seq
}

// SetLastAudioSeq sets the last audio sequence number.
func (p *ESProcessorBase) SetLastAudioSeq(seq uint64) {
	p.lastAudioSeq = seq
}

// ErrProcessorAlreadyStarted is returned when Start is called on an already started processor.
var ErrProcessorAlreadyStarted = stringError("processor already started")

// stringError is a simple error type.
type stringError string

func (e stringError) Error() string { return string(e) }
