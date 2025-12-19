// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

// GRPCTranscoder transcodes ES samples using a ffmpegd subprocess via gRPC.
// It reads from a source variant in SharedESBuffer and writes to a target variant.
type GRPCTranscoder struct {
	id     string
	config GRPCTranscoderConfig
	buffer *SharedESBuffer
	logger *slog.Logger

	// Spawner for subprocess management
	spawner *FFmpegDSpawner

	// gRPC client and cleanup
	client  proto.FFmpegDaemonClient
	cleanup func()

	// gRPC stream for bidirectional transcoding
	stream proto.FFmpegDaemon_TranscodeClient

	// ES reading state for source variant
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Reference to the source ES variant for consumer tracking
	sourceESVariant *ESVariant

	// Video parameter helper for keyframe handling
	videoParams *VideoParamHelper

	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  atomic.Bool
	closed   atomic.Bool
	closedCh chan struct{}

	// Stats
	samplesIn     atomic.Uint64
	samplesOut    atomic.Uint64
	bytesIn       atomic.Uint64
	bytesOut      atomic.Uint64
	errorCount    atomic.Uint64
	startedAt     time.Time
	lastActivity  atomic.Value // time.Time
	encodingSpeed atomic.Value // float64

	// Actual encoder (may differ from requested if fallback occurred)
	actualVideoEncoder string
	actualAudioEncoder string
}

// GRPCTranscoderConfig configures the gRPC transcoder.
type GRPCTranscoderConfig struct {
	// SourceVariant is the source codec variant to read from.
	SourceVariant CodecVariant

	// TargetVariant is the target codec variant to produce.
	TargetVariant CodecVariant

	// VideoEncoder is the target video encoder (e.g., "libx264", "h264_nvenc").
	VideoEncoder string

	// AudioEncoder is the target audio encoder (e.g., "aac", "libopus").
	AudioEncoder string

	// VideoBitrate in kbps (0 for default).
	VideoBitrate int

	// AudioBitrate in kbps (0 for default).
	AudioBitrate int

	// VideoPreset for encoding speed/quality tradeoff.
	VideoPreset string

	// HWAccel hardware acceleration type (empty for software).
	HWAccel string

	// HWAccelDevice hardware acceleration device path.
	HWAccelDevice string

	// SourceURL for direct input mode (bypasses ES demux/mux).
	SourceURL string

	// UseDirectInput enables direct URL input mode.
	UseDirectInput bool

	// ChannelName for job identification.
	ChannelName string

	// Logger for structured logging.
	Logger *slog.Logger
}

// NewGRPCTranscoder creates a new gRPC transcoder.
func NewGRPCTranscoder(
	id string,
	buffer *SharedESBuffer,
	spawner *FFmpegDSpawner,
	config GRPCTranscoderConfig,
) *GRPCTranscoder {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	t := &GRPCTranscoder{
		id:          id,
		config:      config,
		buffer:      buffer,
		spawner:     spawner,
		logger:      config.Logger,
		closedCh:    make(chan struct{}),
		videoParams: NewVideoParamHelper(),
	}
	t.lastActivity.Store(time.Now())

	return t
}

// Start begins the transcoding process.
func (t *GRPCTranscoder) Start(ctx context.Context) error {
	if !t.started.CompareAndSwap(false, true) {
		return errors.New("transcoder already started")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.startedAt = time.Now()

	// Spawn ffmpegd subprocess
	client, cleanup, err := t.spawner.SpawnForJob(t.ctx, t.id)
	if err != nil {
		return fmt.Errorf("spawning ffmpegd subprocess: %w", err)
	}
	t.client = client
	t.cleanup = cleanup

	// Get source variant to read from
	currentSourceKey := t.buffer.SourceVariantKey()
	sourceVariant := t.buffer.GetVariant(currentSourceKey)
	if sourceVariant == nil {
		t.cleanup()
		return fmt.Errorf("source variant %s not found", currentSourceKey)
	}

	// Create or get target variant
	targetVariant, err := t.buffer.CreateVariant(t.config.TargetVariant)
	if err != nil {
		t.cleanup()
		return fmt.Errorf("creating target variant: %w", err)
	}

	// Store source variant reference and register as consumer
	t.sourceESVariant = sourceVariant
	sourceVariant.RegisterConsumer(t.id)

	// Create transcode stream
	t.stream, err = t.client.Transcode(t.ctx)
	if err != nil {
		sourceVariant.UnregisterConsumer(t.id)
		t.cleanup()
		return fmt.Errorf("creating transcode stream: %w", err)
	}

	// Send transcode start message
	startMsg := &proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_Start{
			Start: &proto.TranscodeStart{
				JobId:            t.id,
				ChannelName:      t.config.ChannelName,
				SourceVideoCodec: currentSourceKey.VideoCodec(),
				SourceAudioCodec: currentSourceKey.AudioCodec(),
				TargetVideoCodec: t.config.TargetVariant.VideoCodec(),
				TargetAudioCodec: t.config.TargetVariant.AudioCodec(),
				VideoEncoder:     t.config.VideoEncoder,
				AudioEncoder:     t.config.AudioEncoder,
				VideoBitrateKbps: int32(t.config.VideoBitrate),
				AudioBitrateKbps: int32(t.config.AudioBitrate),
				VideoPreset:      t.config.VideoPreset,
				PreferredHwAccel: t.config.HWAccel,
				HwDevice:         t.config.HWAccelDevice,
			},
		},
	}

	if err := t.stream.Send(startMsg); err != nil {
		sourceVariant.UnregisterConsumer(t.id)
		t.cleanup()
		return fmt.Errorf("sending start message: %w", err)
	}

	// Wait for ack
	ackMsg, err := t.stream.Recv()
	if err != nil {
		sourceVariant.UnregisterConsumer(t.id)
		t.cleanup()
		return fmt.Errorf("receiving ack: %w", err)
	}

	ack := ackMsg.GetAck()
	if ack == nil {
		sourceVariant.UnregisterConsumer(t.id)
		t.cleanup()
		return errors.New("expected ack message, got different type")
	}

	if !ack.Success {
		sourceVariant.UnregisterConsumer(t.id)
		t.cleanup()
		return fmt.Errorf("transcode start failed: %s", ack.Error)
	}

	// Store actual encoders used
	t.actualVideoEncoder = ack.ActualVideoEncoder
	t.actualAudioEncoder = ack.ActualAudioEncoder

	t.logger.Debug("gRPC transcoder started",
		slog.String("id", t.id),
		slog.String("source", string(currentSourceKey)),
		slog.String("target", string(t.config.TargetVariant)),
		slog.String("video_encoder", t.actualVideoEncoder),
		slog.String("audio_encoder", t.actualAudioEncoder))

	// Start input goroutine (sends samples to ffmpegd)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runInputLoop(sourceVariant)
	}()

	// Start output goroutine (receives transcoded samples)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runOutputLoop(targetVariant)
	}()

	return nil
}

// Stop stops the transcoder.
func (t *GRPCTranscoder) Stop() {
	if t.closed.CompareAndSwap(false, true) {
		// Send stop message
		if t.stream != nil {
			stopMsg := &proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Stop{
					Stop: &proto.TranscodeStop{
						Reason: "stop requested",
					},
				},
			}
			_ = t.stream.Send(stopMsg)
			_ = t.stream.CloseSend()
		}

		if t.cancel != nil {
			t.cancel()
		}

		// Unregister as consumer
		if t.sourceESVariant != nil {
			t.sourceESVariant.UnregisterConsumer(t.id)
		}

		close(t.closedCh)

		// Wait for goroutines with timeout
		done := make(chan struct{})
		go func() {
			t.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.logger.Warn("gRPC transcoder goroutines did not finish in time",
				slog.String("id", t.id))
		}

		// Cleanup subprocess
		if t.cleanup != nil {
			t.cleanup()
		}

		t.logger.Debug("gRPC transcoder stopped",
			slog.String("id", t.id))
	}
}

// Stats returns current transcoder statistics.
func (t *GRPCTranscoder) Stats() TranscoderStats {
	lastActivity, _ := t.lastActivity.Load().(time.Time)
	encodingSpeed, _ := t.encodingSpeed.Load().(float64)

	return TranscoderStats{
		ID:            t.id,
		SourceVariant: t.config.SourceVariant,
		TargetVariant: t.config.TargetVariant,
		StartedAt:     t.startedAt,
		LastActivity:  lastActivity,
		SamplesIn:     t.samplesIn.Load(),
		SamplesOut:    t.samplesOut.Load(),
		BytesIn:       t.bytesIn.Load(),
		BytesOut:      t.bytesOut.Load(),
		Errors:        t.errorCount.Load(),
		VideoCodec:    t.config.TargetVariant.VideoCodec(),
		AudioCodec:    t.config.TargetVariant.AudioCodec(),
		VideoEncoder:  t.actualVideoEncoder,
		AudioEncoder:  t.actualAudioEncoder,
		HWAccel:       t.config.HWAccel,
		HWAccelDevice: t.config.HWAccelDevice,
		EncodingSpeed: encodingSpeed,
	}
}

// ProcessStats returns process-level statistics.
// For gRPC transcoder, this is not available (subprocess stats would require IPC).
func (t *GRPCTranscoder) ProcessStats() *TranscoderProcessStats {
	return nil
}

// IsClosed returns true if the transcoder is closed.
func (t *GRPCTranscoder) IsClosed() bool {
	return t.closed.Load()
}

// ClosedChan returns a channel that is closed when transcoder stops.
func (t *GRPCTranscoder) ClosedChan() <-chan struct{} {
	return t.closedCh
}

// runInputLoop reads ES samples from source variant and sends them to ffmpegd.
func (t *GRPCTranscoder) runInputLoop(source *ESVariant) {
	videoTrack := source.VideoTrack()
	audioTrack := source.AudioTrack()

	// Wait for initial keyframe
	t.logger.Debug("Waiting for initial keyframe from source",
		slog.String("id", t.id))

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-videoTrack.NotifyChan():
		}

		samples := videoTrack.ReadFromKeyframe(t.lastVideoSeq, 1)
		if len(samples) > 0 {
			t.lastVideoSeq = samples[0].Sequence - 1
			break
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			if err := t.processSourceSamples(videoTrack, audioTrack); err != nil {
				if !errors.Is(err, context.Canceled) {
					t.errorCount.Add(1)
					t.logger.Warn("Error processing source samples",
						slog.String("id", t.id),
						slog.String("error", err.Error()))
				}
				return
			}
		}
	}
}

// processSourceSamples reads samples from source and sends them to ffmpegd.
func (t *GRPCTranscoder) processSourceSamples(videoTrack, audioTrack *ESTrack) error {
	var protoVideoSamples []*proto.ESSample
	var protoAudioSamples []*proto.ESSample

	// Read video samples
	videoSamples := videoTrack.ReadFrom(t.lastVideoSeq, 100)
	for _, sample := range videoSamples {
		// Extract video params from keyframes
		if sample.IsKeyframe {
			isH265 := t.config.SourceVariant.VideoCodec() == "h265" ||
				t.config.SourceVariant.VideoCodec() == "hevc"
			t.videoParams.ExtractFromAnnexB(sample.Data, isH265)
		}

		protoVideoSamples = append(protoVideoSamples, &proto.ESSample{
			Pts:        sample.PTS,
			Dts:        sample.DTS,
			Data:       sample.Data,
			IsKeyframe: sample.IsKeyframe,
			Sequence:   sample.Sequence,
		})
		t.lastVideoSeq = sample.Sequence
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(t.lastAudioSeq, 200)
	for _, sample := range audioSamples {
		protoAudioSamples = append(protoAudioSamples, &proto.ESSample{
			Pts:      sample.PTS,
			Data:     sample.Data,
			Sequence: sample.Sequence,
		})
		t.lastAudioSeq = sample.Sequence
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Update consumer position
	if t.sourceESVariant != nil && (len(videoSamples) > 0 || len(audioSamples) > 0) {
		t.sourceESVariant.UpdateConsumerPosition(t.id, t.lastVideoSeq, t.lastAudioSeq)
	}

	// Send samples to ffmpegd
	if len(protoVideoSamples) > 0 || len(protoAudioSamples) > 0 {
		msg := &proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Samples{
				Samples: &proto.ESSampleBatch{
					VideoSamples: protoVideoSamples,
					AudioSamples: protoAudioSamples,
					IsSource:     true,
				},
			},
		}
		if err := t.stream.Send(msg); err != nil {
			return fmt.Errorf("sending samples: %w", err)
		}
		t.recordActivity()
	}

	return nil
}

// runOutputLoop receives transcoded samples from ffmpegd and writes to target variant.
func (t *GRPCTranscoder) runOutputLoop(target *ESVariant) {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		msg, err := t.stream.Recv()
		if err != nil {
			if err == io.EOF || errors.Is(err, context.Canceled) {
				return
			}
			t.errorCount.Add(1)
			t.logger.Warn("Error receiving from ffmpegd",
				slog.String("id", t.id),
				slog.String("error", err.Error()))
			return
		}

		switch payload := msg.Payload.(type) {
		case *proto.TranscodeMessage_Samples:
			if err := t.handleOutputSamples(target, payload.Samples); err != nil {
				t.errorCount.Add(1)
				t.logger.Warn("Error handling output samples",
					slog.String("id", t.id),
					slog.String("error", err.Error()))
			}

		case *proto.TranscodeMessage_Stats:
			t.handleStats(payload.Stats)

		case *proto.TranscodeMessage_Error:
			t.logger.Warn("Transcode error from ffmpegd",
				slog.String("id", t.id),
				slog.String("error", payload.Error.Message),
				slog.String("code", payload.Error.Code.String()))
			t.errorCount.Add(1)

		case *proto.TranscodeMessage_Ack:
			// Ignore late acks

		case *proto.TranscodeMessage_Stop:
			t.logger.Debug("Received stop from ffmpegd",
				slog.String("id", t.id))
			return
		}
	}
}

// handleOutputSamples writes transcoded samples to target variant.
func (t *GRPCTranscoder) handleOutputSamples(target *ESVariant, batch *proto.ESSampleBatch) error {
	// Process video samples
	for _, sample := range batch.VideoSamples {
		target.VideoTrack().Write(sample.Pts, sample.Dts, sample.Data, sample.IsKeyframe)
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(sample.Data)))
	}

	// Process audio samples
	for _, sample := range batch.AudioSamples {
		target.AudioTrack().Write(sample.Pts, sample.Pts, sample.Data, false)
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(sample.Data)))
	}

	t.recordActivity()
	return nil
}

// handleStats updates transcoder stats from ffmpegd.
func (t *GRPCTranscoder) handleStats(stats *proto.TranscodeStats) {
	if stats == nil {
		return
	}
	t.encodingSpeed.Store(stats.EncodingSpeed)
}

// recordActivity updates last activity time.
func (t *GRPCTranscoder) recordActivity() {
	t.lastActivity.Store(time.Now())
}

// Verify GRPCTranscoder implements Transcoder interface.
var _ Transcoder = (*GRPCTranscoder)(nil)
