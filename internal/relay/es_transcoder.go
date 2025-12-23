// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// ESTranscoderMode indicates whether the transcoder spawns a local subprocess or uses a remote daemon.
type ESTranscoderMode int

const (
	// ESTranscoderModeLocal spawns a local ffmpegd subprocess.
	ESTranscoderModeLocal ESTranscoderMode = iota
	// ESTranscoderModeRemote uses an already-connected remote daemon.
	ESTranscoderModeRemote
)

// ESTranscoderConfig configures the ES transcoder.
type ESTranscoderConfig struct {
	// SourceVariant is the source codec variant to read from.
	SourceVariant CodecVariant

	// TargetVariant is the target codec variant to produce.
	TargetVariant CodecVariant

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

	// ChannelID for job identification (used in remote mode).
	ChannelID string

	// ChannelName for job identification.
	ChannelName string

	// SessionID for session tracking (used in remote mode).
	SessionID string

	// SourceURL for direct input mode (bypasses ES demux/mux).
	SourceURL string

	// UseDirectInput enables direct URL input mode.
	UseDirectInput bool

	// Custom FFmpeg flags from encoding profile.
	GlobalFlags string // Flags placed at the start of the command
	InputFlags  string // Flags placed before -i input
	OutputFlags string // Flags placed after -i input

	// EncoderOverrides are passed to the daemon to override encoder selection.
	// These come from the encoder_overrides database table and allow working
	// around hardware encoder bugs.
	EncoderOverrides []*proto.EncoderOverride

	// OutputFormat is the container format for daemon FFmpeg output.
	// Values: "fmp4" (fragmented MP4), "mpegts" (MPEG Transport Stream)
	// Default: auto-select based on target codec (fmp4 for av1/vp9, mpegts for h264/h265)
	OutputFormat string

	// Logger for structured logging.
	Logger *slog.Logger
}

// ESTranscoder transcodes ES samples using ffmpegd (either local subprocess or remote daemon).
// It reads from a source variant in SharedESBuffer and writes to a target variant.
type ESTranscoder struct {
	id     string
	mode   ESTranscoderMode
	config ESTranscoderConfig
	buffer *SharedESBuffer
	logger *slog.Logger

	// Local mode: spawner for subprocess management
	spawner *FFmpegDSpawner
	cleanup func() // Cleanup function for local subprocess

	// Remote mode: daemon reference
	daemon *types.Daemon

	// Stream and job management
	streamMgr *DaemonStreamManager
	jobMgr    *ActiveJobManager

	// Active job and daemon tracking
	daemonID  types.DaemonID
	activeJob *ActiveJob

	// ES reading state for source variant
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Reference to the source ES variant for consumer tracking
	sourceESVariant *ESVariant

	// Video parameter helper for keyframe handling
	videoParams *VideoParamHelper

	// Lifecycle
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	started        atomic.Bool
	closed         atomic.Bool
	closedCh       chan struct{}
	inputExhausted atomic.Bool // Set when input loop finishes naturally (source EOF)

	// Stats
	samplesIn     atomic.Uint64
	samplesOut    atomic.Uint64
	bytesIn       atomic.Uint64
	bytesOut      atomic.Uint64
	errorCount    atomic.Uint64
	startedAt     time.Time
	lastActivity  atomic.Value // time.Time
	encodingSpeed atomic.Value // float64

	// Remote process stats (received from daemon)
	cpuPercent atomic.Value // float64
	memoryMB   atomic.Value // float64
	ffmpegPID  atomic.Int32

	// Resource history for sparkline graphs
	resourceHistory *ResourceHistory

	// Actual encoder (may differ from requested if fallback occurred)
	actualVideoEncoder string
	actualAudioEncoder string
}

// NewLocalESTranscoder creates an ES transcoder that spawns a local ffmpegd subprocess.
func NewLocalESTranscoder(
	id string,
	buffer *SharedESBuffer,
	spawner *FFmpegDSpawner,
	streamMgr *DaemonStreamManager,
	jobMgr *ActiveJobManager,
	config ESTranscoderConfig,
) *ESTranscoder {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	t := &ESTranscoder{
		id:              id,
		mode:            ESTranscoderModeLocal,
		config:          config,
		buffer:          buffer,
		spawner:         spawner,
		streamMgr:       streamMgr,
		jobMgr:          jobMgr,
		logger:          config.Logger,
		closedCh:        make(chan struct{}),
		videoParams:     NewVideoParamHelper(),
		resourceHistory: NewResourceHistory(),
	}
	t.lastActivity.Store(time.Now())

	return t
}

// NewRemoteESTranscoder creates an ES transcoder that uses an already-connected remote daemon.
func NewRemoteESTranscoder(
	id string,
	buffer *SharedESBuffer,
	daemon *types.Daemon,
	streamMgr *DaemonStreamManager,
	jobMgr *ActiveJobManager,
	config ESTranscoderConfig,
) *ESTranscoder {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	t := &ESTranscoder{
		id:              id,
		mode:            ESTranscoderModeRemote,
		config:          config,
		buffer:          buffer,
		daemon:          daemon,
		daemonID:        daemon.ID,
		streamMgr:       streamMgr,
		jobMgr:          jobMgr,
		logger:          config.Logger,
		closedCh:        make(chan struct{}),
		videoParams:     NewVideoParamHelper(),
		resourceHistory: NewResourceHistory(),
	}
	t.lastActivity.Store(time.Now())

	return t
}

// Start begins the transcoding process.
func (t *ESTranscoder) Start(ctx context.Context) error {
	if !t.started.CompareAndSwap(false, true) {
		return errors.New("transcoder already started")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.startedAt = time.Now()

	// Get source variant and register as consumer BEFORE spawning subprocess
	// This is critical to prevent keyframe eviction during subprocess startup
	currentSourceKey := t.buffer.SourceVariantKey()
	sourceVariant := t.buffer.GetVariant(currentSourceKey)
	if sourceVariant == nil {
		return fmt.Errorf("source variant %s not found", currentSourceKey)
	}

	// Register as consumer EARLY to protect samples from eviction
	t.sourceESVariant = sourceVariant
	sourceVariant.RegisterConsumer(t.id)

	// Create or get target variant
	targetVariant, err := t.buffer.CreateVariant(t.config.TargetVariant)
	if err != nil {
		sourceVariant.UnregisterConsumer(t.id)
		return fmt.Errorf("creating target variant: %w", err)
	}

	// Mode-specific startup
	var stream *DaemonStream
	if t.mode == ESTranscoderModeLocal {
		stream, err = t.startLocal(currentSourceKey)
	} else {
		stream, err = t.startRemote(currentSourceKey)
	}
	if err != nil {
		sourceVariant.UnregisterConsumer(t.id)
		return err
	}

	t.logger.Debug("ES transcoder started",
		slog.String("id", t.id),
		slog.String("mode", t.modeString()),
		slog.String("daemon_id", string(t.daemonID)),
		slog.String("source", string(currentSourceKey)),
		slog.String("target", string(t.config.TargetVariant)),
		slog.Bool("direct_input", t.config.UseDirectInput))

	// Start input goroutine (sends samples to ffmpegd via stream)
	// Only needed when NOT using direct input mode
	if !t.config.UseDirectInput {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runInputLoop(sourceVariant, stream)
		}()
	}

	// Start output goroutine (receives transcoded samples from job)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runOutputLoop(targetVariant)
	}()

	return nil
}

// startLocal spawns a local ffmpegd subprocess and starts a transcode job.
func (t *ESTranscoder) startLocal(sourceKey CodecVariant) (*DaemonStream, error) {
	// Spawn ffmpegd subprocess
	daemonID, cleanup, err := t.spawner.SpawnForJob(t.ctx, t.id)
	if err != nil {
		return nil, fmt.Errorf("spawning ffmpegd subprocess: %w", err)
	}
	t.daemonID = daemonID
	t.cleanup = cleanup

	// Wait for daemon's transcode stream to become available
	streamCtx, streamCancel := context.WithTimeout(t.ctx, 30*time.Second)
	defer streamCancel()

	stream, err := t.streamMgr.WaitForStream(streamCtx, daemonID)
	if err != nil {
		t.cleanup()
		return nil, fmt.Errorf("waiting for daemon stream: %w", err)
	}

	// Wait for audio init data (AudioSpecificConfig for AAC) from demuxer
	// This ensures we get the correct sample rate and channel count for ADTS headers
	var audioInitData []byte
	if t.sourceESVariant != nil {
		audioTrack := t.sourceESVariant.AudioTrack()
		audioCodec := sourceKey.AudioCodec()
		t.logger.Info("Checking for audio init data",
			slog.String("audio_codec", audioCodec))

		audioInitData = audioTrack.GetInitData()
		if audioInitData == nil {
			t.logger.Info("Waiting for audio initData from demuxer")
			waitCtx, waitCancel := context.WithTimeout(t.ctx, 2*time.Second)
			ticker := time.NewTicker(50 * time.Millisecond)
		waitLoop:
			for {
				select {
				case <-waitCtx.Done():
					t.logger.Warn("Audio initData timeout, using defaults")
					break waitLoop
				case <-ticker.C:
					audioInitData = audioTrack.GetInitData()
					if audioInitData != nil {
						break waitLoop
					}
				}
			}
			ticker.Stop()
			waitCancel()
		}
		if audioInitData != nil {
			t.logger.Info("Got audio initData for daemon",
				slog.Int("init_data_len", len(audioInitData)))
		} else {
			t.logger.Warn("No audio initData available, ADTS headers will use defaults")
		}
	} else {
		t.logger.Warn("sourceESVariant is nil, cannot get audio initData")
	}

	// Build TranscodeStart config
	startConfig := &proto.TranscodeStart{
		JobId:                 t.id,
		ChannelName:           t.config.ChannelName,
		SourceVideoCodec:      sourceKey.VideoCodec(),
		SourceAudioCodec:      sourceKey.AudioCodec(),
		AudioInitData:         audioInitData,
		TargetVideoCodec:      t.config.TargetVariant.VideoCodec(),
		TargetAudioCodec:      t.config.TargetVariant.AudioCodec(),
		VideoBitrateKbps:      int32(t.config.VideoBitrate),
		AudioBitrateKbps:      int32(t.config.AudioBitrate),
		VideoPreset:           t.config.VideoPreset,
		PreferredHwAccel:      t.config.HWAccel,
		HwDevice:              t.config.HWAccelDevice,
		GlobalFlags:           t.config.GlobalFlags,
		InputFlags:            t.config.InputFlags,
		OutputFlags:           t.config.OutputFlags,
		EncoderOverrides:      t.config.EncoderOverrides,
		OutputContainerFormat: t.config.OutputFormat,
	}

	// Log encoder overrides being sent to daemon
	if len(t.config.EncoderOverrides) > 0 {
		t.logger.Debug("sending encoder overrides to daemon",
			slog.String("daemon_id", string(daemonID)),
			slog.Int("override_count", len(t.config.EncoderOverrides)),
		)
		for i, override := range t.config.EncoderOverrides {
			t.logger.Debug("encoder override",
				slog.Int("index", i),
				slog.String("codec_type", override.CodecType),
				slog.String("source_codec", override.SourceCodec),
				slog.String("target_encoder", override.TargetEncoder),
				slog.String("hw_accel_match", override.HwAccelMatch),
				slog.String("cpu_match", override.CpuMatch),
				slog.Int("priority", int(override.Priority)),
			)
		}
	}

	// Start the transcode job on the daemon's stream
	t.activeJob, err = StartTranscodeJob(
		t.ctx,
		t.streamMgr,
		t.jobMgr,
		daemonID,
		t.id,
		t.config.SessionID,
		t.config.ChannelID,
		t.config.ChannelName,
		t.buffer,
		startConfig,
	)
	if err != nil {
		t.cleanup()
		return nil, fmt.Errorf("starting transcode job: %w", err)
	}

	return stream, nil
}

// startRemote uses an existing daemon's stream to start a transcode job.
func (t *ESTranscoder) startRemote(sourceKey CodecVariant) (*DaemonStream, error) {
	// Get the daemon's idle stream
	stream, ok := t.streamMgr.GetIdleStream(t.daemonID)
	if !ok {
		return nil, fmt.Errorf("daemon %s stream not available or busy", t.daemonID)
	}

	// Create the active job to track transcoded output
	t.activeJob = t.jobMgr.CreateJob(
		t.id,
		t.config.SessionID,
		t.config.ChannelID,
		t.config.ChannelName,
		t.daemonID,
		stream,
		t.buffer,
	)

	// Wait for audio init data (AudioSpecificConfig for AAC) from demuxer
	var audioInitData []byte
	if t.sourceESVariant != nil {
		audioTrack := t.sourceESVariant.AudioTrack()
		audioCodec := sourceKey.AudioCodec()
		t.logger.Info("Checking for audio init data (remote)",
			slog.String("audio_codec", audioCodec))

		audioInitData = audioTrack.GetInitData()
		if audioInitData == nil {
			t.logger.Info("Waiting for audio initData from demuxer (remote)")
			waitCtx, waitCancel := context.WithTimeout(t.ctx, 2*time.Second)
			ticker := time.NewTicker(50 * time.Millisecond)
		waitLoop:
			for {
				select {
				case <-waitCtx.Done():
					t.logger.Warn("Audio initData timeout, using defaults (remote)")
					break waitLoop
				case <-ticker.C:
					audioInitData = audioTrack.GetInitData()
					if audioInitData != nil {
						break waitLoop
					}
				}
			}
			ticker.Stop()
			waitCancel()
		}
		if audioInitData != nil {
			t.logger.Info("Got audio initData for remote daemon",
				slog.Int("init_data_len", len(audioInitData)))
		} else {
			t.logger.Warn("No audio initData available (remote), ADTS headers will use defaults")
		}
	} else {
		t.logger.Warn("sourceESVariant is nil (remote), cannot get audio initData")
	}

	// Send TranscodeStart to daemon via the persistent stream
	startMsg := &proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_Start{
			Start: &proto.TranscodeStart{
				JobId:                 t.id,
				SessionId:             t.config.SessionID,
				ChannelId:             t.config.ChannelID,
				ChannelName:           t.config.ChannelName,
				SourceVideoCodec:      sourceKey.VideoCodec(),
				SourceAudioCodec:      sourceKey.AudioCodec(),
				AudioInitData:         audioInitData,
				TargetVideoCodec:      t.config.TargetVariant.VideoCodec(),
				TargetAudioCodec:      t.config.TargetVariant.AudioCodec(),
				VideoBitrateKbps:      int32(t.config.VideoBitrate),
				AudioBitrateKbps:      int32(t.config.AudioBitrate),
				VideoPreset:           t.config.VideoPreset,
				PreferredHwAccel:      t.config.HWAccel,
				HwDevice:              t.config.HWAccelDevice,
				GlobalFlags:           t.config.GlobalFlags,
				InputFlags:            t.config.InputFlags,
				OutputFlags:           t.config.OutputFlags,
				EncoderOverrides:      t.config.EncoderOverrides,
				OutputContainerFormat: t.config.OutputFormat,
			},
		},
	}

	// Log encoder overrides being sent to remote daemon
	if len(t.config.EncoderOverrides) > 0 {
		t.logger.Debug("sending encoder overrides to remote daemon",
			slog.String("daemon_id", string(t.daemonID)),
			slog.Int("override_count", len(t.config.EncoderOverrides)),
		)
		for i, override := range t.config.EncoderOverrides {
			t.logger.Debug("encoder override",
				slog.Int("index", i),
				slog.String("codec_type", override.CodecType),
				slog.String("source_codec", override.SourceCodec),
				slog.String("target_encoder", override.TargetEncoder),
				slog.String("hw_accel_match", override.HwAccelMatch),
				slog.String("cpu_match", override.CpuMatch),
				slog.Int("priority", int(override.Priority)),
			)
		}
	}

	if err := stream.Send(startMsg); err != nil {
		t.jobMgr.RemoveJob(t.id)
		return nil, fmt.Errorf("sending start message to daemon: %w", err)
	}

	return stream, nil
}

// Stop stops the transcoder.
func (t *ESTranscoder) Stop() {
	if t.closed.CompareAndSwap(false, true) {
		// Send stop message to daemon via stream
		if t.activeJob != nil && t.activeJob.Stream != nil {
			stopMsg := &proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Stop{
					Stop: &proto.TranscodeStop{
						Reason: "stop requested",
					},
				},
			}
			_ = t.activeJob.Stream.Send(stopMsg)
		}

		// Remove job from job manager
		if t.jobMgr != nil && t.id != "" {
			t.jobMgr.RemoveJob(t.id)
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
			t.logger.Warn("ES transcoder goroutines did not finish in time",
				slog.String("id", t.id),
				slog.String("mode", t.modeString()))
		}

		// Local mode: cleanup subprocess
		if t.cleanup != nil {
			t.cleanup()
		}

		t.logger.Debug("ES transcoder stopped",
			slog.String("id", t.id),
			slog.String("mode", t.modeString()))
	}
}

// Stats returns current transcoder statistics.
func (t *ESTranscoder) Stats() TranscoderStats {
	lastActivity, _ := t.lastActivity.Load().(time.Time)

	// Get encoding speed, hwaccel info, and ffmpeg command from activeJob.Stats
	var encodingSpeed float64
	var actualHWAccel, actualHWDevice, ffmpegCommand string
	if t.activeJob != nil && t.activeJob.Stats != nil {
		encodingSpeed = t.activeJob.Stats.EncodingSpeed
		actualHWAccel = t.activeJob.Stats.HWAccel
		actualHWDevice = t.activeJob.Stats.HWDevice
		ffmpegCommand = t.activeJob.Stats.FFmpegCommand
	}
	if encodingSpeed == 0 {
		encodingSpeed, _ = t.encodingSpeed.Load().(float64)
	}
	// Fall back to configured hwaccel if actual not yet received
	if actualHWAccel == "" {
		actualHWAccel = t.config.HWAccel
	}
	if actualHWDevice == "" {
		actualHWDevice = t.config.HWAccelDevice
	}

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
		HWAccel:       actualHWAccel,
		HWAccelDevice: actualHWDevice,
		EncodingSpeed: encodingSpeed,
		FFmpegCommand: ffmpegCommand,
	}
}

// ProcessStats returns process-level statistics.
func (t *ESTranscoder) ProcessStats() *TranscoderProcessStats {
	// Get stats from the active job (populated by grpc_server.go)
	if t.activeJob != nil && t.activeJob.Stats != nil {
		stats := t.activeJob.Stats
		if stats.CPUPercent != 0 || stats.MemoryMB != 0 || stats.FFmpegPID != 0 {
			return &TranscoderProcessStats{
				PID:         stats.FFmpegPID,
				CPUPercent:  stats.CPUPercent,
				MemoryRSSMB: stats.MemoryMB,
			}
		}
	}

	// Fallback to local atomic fields (if handleStats was called directly)
	cpuPct, _ := t.cpuPercent.Load().(float64)
	memMB, _ := t.memoryMB.Load().(float64)
	pid := int(t.ffmpegPID.Load())

	if cpuPct == 0 && memMB == 0 && pid == 0 {
		return nil
	}

	return &TranscoderProcessStats{
		PID:         pid,
		CPUPercent:  cpuPct,
		MemoryRSSMB: memMB,
	}
}

// GetResourceHistory returns CPU and memory history for sparkline graphs.
func (t *ESTranscoder) GetResourceHistory() (cpuHistory, memHistory []float64) {
	if t.resourceHistory == nil {
		return nil, nil
	}
	return t.resourceHistory.GetHistory()
}

// IsClosed returns true if the transcoder is closed.
func (t *ESTranscoder) IsClosed() bool {
	return t.closed.Load()
}

// ClosedChan returns a channel that is closed when transcoder stops.
func (t *ESTranscoder) ClosedChan() <-chan struct{} {
	return t.closedCh
}

// runInputLoop reads ES samples from source variant and sends them to ffmpegd.
func (t *ESTranscoder) runInputLoop(source *ESVariant, stream *DaemonStream) {
	videoTrack := source.VideoTrack()
	audioTrack := source.AudioTrack()

	// Wait for initial keyframe
	// Check for existing samples first before waiting - handles case where
	// source has finished but buffer still has content (finite streams)
	t.logger.Debug("Waiting for initial keyframe from source",
		slog.String("id", t.id),
		slog.Uint64("current_last_seq", videoTrack.LastSequence()),
		slog.Int("sample_count", videoTrack.Count()))

	waitCount := 0
	for {
		// Try to read samples immediately (non-blocking check)
		samples := videoTrack.ReadFromKeyframe(t.lastVideoSeq, 1)
		if len(samples) > 0 {
			t.logger.Debug("Found keyframe sample",
				slog.String("id", t.id),
				slog.Uint64("sequence", samples[0].Sequence),
				slog.Int64("pts", samples[0].PTS),
				slog.Bool("is_keyframe", samples[0].IsKeyframe))
			t.lastVideoSeq = samples[0].Sequence - 1
			break
		}

		waitCount++
		if waitCount%100 == 0 {
			t.logger.Debug("Still waiting for keyframe",
				slog.String("id", t.id),
				slog.Int("wait_count", waitCount),
				slog.Uint64("track_last_seq", videoTrack.LastSequence()),
				slog.Int("track_count", videoTrack.Count()))
		}

		// No samples available, wait for notification or context cancellation
		select {
		case <-t.ctx.Done():
			return
		case <-videoTrack.NotifyChan():
			// New sample notification received, loop back to read
		}
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Throughput logging
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()
	var lastSamplesIn, lastBytesIn uint64

	// Track source completion for graceful shutdown
	sourceCompletedCh := t.buffer.SourceCompletedChan()
	var sourceCompleted bool
	var idleTicksAfterComplete int
	const idleThresholdTicks = 50 // 500ms with 10ms ticker

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-sourceCompletedCh:
			// Source stream finished (EOF), mark it but continue processing remaining samples
			sourceCompleted = true
			t.logger.Info("ES transcoder: source stream completed (EOF), processing remaining samples",
				slog.String("id", t.id),
				slog.Uint64("last_video_seq", t.lastVideoSeq),
				slog.Uint64("last_audio_seq", t.lastAudioSeq))
			// Set to nil to prevent repeated triggers (channel stays closed)
			sourceCompletedCh = nil
		case <-logTicker.C:
			currentSamples := t.samplesIn.Load()
			currentBytes := t.bytesIn.Load()
			samplesDelta := currentSamples - lastSamplesIn
			bytesDelta := currentBytes - lastBytesIn
			t.logger.Log(t.ctx, observability.LevelTrace, "ES transcoder throughput",
				slog.String("id", t.id),
				slog.Uint64("samples_sent_5s", samplesDelta),
				slog.Float64("samples_per_sec", float64(samplesDelta)/5.0),
				slog.Float64("kbps", float64(bytesDelta)/5.0/1024),
				slog.Uint64("last_video_seq", t.lastVideoSeq),
				slog.Uint64("last_audio_seq", t.lastAudioSeq))
			lastSamplesIn = currentSamples
			lastBytesIn = currentBytes
		case <-ticker.C:
			prevVideoSeq := t.lastVideoSeq
			prevAudioSeq := t.lastAudioSeq

			if err := t.processSourceSamples(videoTrack, audioTrack, stream); err != nil {
				if !errors.Is(err, context.Canceled) {
					t.errorCount.Add(1)
					t.logger.Warn("Error processing source samples",
						slog.String("id", t.id),
						slog.String("error", err.Error()))
				}
				return
			}

			// Track if we processed any samples (sequences advanced)
			hadSamples := t.lastVideoSeq > prevVideoSeq || t.lastAudioSeq > prevAudioSeq

			// If source is completed, track how long we've been idle
			if sourceCompleted {
				if hadSamples {
					idleTicksAfterComplete = 0
				} else {
					idleTicksAfterComplete++
					// After threshold idle ticks, assume all input samples sent to FFmpeg
					if idleTicksAfterComplete >= idleThresholdTicks {
						t.logger.Info("ES transcoder: input exhausted, all samples sent to FFmpeg",
							slog.String("id", t.id),
							slog.Uint64("total_samples_in", t.samplesIn.Load()),
							slog.Uint64("total_bytes_in", t.bytesIn.Load()))

						// Signal input complete to daemon so it lets FFmpeg flush
						// This is a graceful signal - FFmpeg will finish encoding buffered frames
						inputCompleteMsg := &proto.TranscodeMessage{
							Payload: &proto.TranscodeMessage_InputComplete{
								InputComplete: &proto.TranscodeInputComplete{
									Reason: "source_eof",
								},
							},
						}
						if err := stream.Send(inputCompleteMsg); err != nil {
							t.logger.Warn("Failed to send input complete to daemon",
								slog.String("id", t.id),
								slog.String("error", err.Error()))
						} else {
							t.logger.Info("Sent input complete signal to daemon",
								slog.String("id", t.id))
						}

						// Mark input as exhausted - output loop will trigger Stop after FFmpeg finishes
						// DON'T call Stop() here - FFmpeg may still be encoding buffered frames
						t.inputExhausted.Store(true)
						return
					}
				}
			}
		}
	}
}

// processSourceSamples reads samples from source and sends them to ffmpegd.
func (t *ESTranscoder) processSourceSamples(videoTrack, audioTrack *ESTrack, stream *DaemonStream) error {
	var protoVideoSamples []*proto.ESSample
	var protoAudioSamples []*proto.ESSample

	// Read video samples
	isH265 := t.config.SourceVariant.VideoCodec() == "h265" ||
		t.config.SourceVariant.VideoCodec() == "hevc"

	videoSamples := videoTrack.ReadFrom(t.lastVideoSeq, 100)
	for _, sample := range videoSamples {
		// Always try to extract video params from all samples - SPS/PPS may appear
		// in non-keyframe samples in some streams
		extracted := t.videoParams.ExtractFromAnnexB(sample.Data, isH265)
		if extracted {
			t.logger.Debug("Extracted video params",
				slog.String("id", t.id),
				slog.Uint64("sequence", sample.Sequence),
				slog.Bool("is_keyframe", sample.IsKeyframe),
				slog.Bool("has_h264_params", t.videoParams.HasH264Params()),
				slog.Bool("has_h265_params", t.videoParams.HasH265Params()))
		}

		// For keyframes, analyze NAL types for debugging
		if sample.IsKeyframe && t.samplesIn.Load() < 10 {
			nalTypes := analyzeNALTypes(sample.Data, isH265)
			t.logger.Debug("Keyframe NAL analysis",
				slog.String("id", t.id),
				slog.Uint64("sequence", sample.Sequence),
				slog.String("nal_types", nalTypes),
				slog.Int("data_len", len(sample.Data)))
		}

		// Prepend SPS/PPS to keyframes to ensure FFmpeg can decode
		// Use ForceKeyframePrepend to bypass the internal IDR check since
		// the demuxer has already determined this is a keyframe
		sampleData := sample.Data
		if sample.IsKeyframe {
			beforeLen := len(sampleData)
			sampleData = t.videoParams.ForceKeyframePrepend(sample.Data, isH265)
			afterLen := len(sampleData)
			if afterLen != beforeLen {
				t.logger.Debug("Prepended video params to keyframe",
					slog.String("id", t.id),
					slog.Uint64("sequence", sample.Sequence),
					slog.Int("before_len", beforeLen),
					slog.Int("after_len", afterLen))
			} else {
				t.logger.Debug("Keyframe: no params prepended",
					slog.String("id", t.id),
					slog.Uint64("sequence", sample.Sequence),
					slog.Bool("is_h265", isH265),
					slog.Bool("has_h264_params", t.videoParams.HasH264Params()),
					slog.Bool("has_h265_params", t.videoParams.HasH265Params()))
			}
		}

		protoVideoSamples = append(protoVideoSamples, &proto.ESSample{
			Pts:        sample.PTS,
			Dts:        sample.DTS,
			Data:       sampleData,
			IsKeyframe: sample.IsKeyframe,
			Sequence:   sample.Sequence,
		})
		t.lastVideoSeq = sample.Sequence
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sampleData)))
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

	// Send samples to ffmpegd via the stream
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
		if err := stream.Send(msg); err != nil {
			return fmt.Errorf("sending samples: %w", err)
		}
		t.recordActivity()
	}

	return nil
}

// runOutputLoop receives transcoded samples from ffmpegd via the active job.
func (t *ESTranscoder) runOutputLoop(target *ESVariant) {
	if t.activeJob == nil {
		t.logger.Error("runOutputLoop called but activeJob is nil",
			slog.String("id", t.id))
		return
	}

	// Track if we exit due to context cancellation (external Stop) vs natural finish
	var contextCanceled bool

	// Cleanup: if input was exhausted (source EOF) and we exit naturally,
	// trigger Stop() to clean up resources. This ensures FFmpeg can finish
	// encoding all buffered frames before we shut down.
	defer func() {
		// Only trigger Stop if we exited naturally (not due to context cancellation)
		// and input was exhausted (indicating a finite stream that finished)
		if !contextCanceled && t.inputExhausted.Load() {
			t.logger.Info("ES transcoder: output loop finished after input exhaustion, stopping transcoder",
				slog.String("id", t.id),
				slog.Uint64("total_samples_out", t.samplesOut.Load()),
				slog.Uint64("total_bytes_out", t.bytesOut.Load()))
			go t.Stop()
		}
	}()

	// Throughput logging
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()
	var lastSamplesOut, lastBytesOut uint64
	var firstBatchReceived bool

	for {
		select {
		case <-t.ctx.Done():
			contextCanceled = true
			t.logger.Debug("runOutputLoop exiting: context done",
				slog.String("id", t.id),
				slog.Uint64("total_samples_out", t.samplesOut.Load()))
			return
		case <-logTicker.C:
			currentSamples := t.samplesOut.Load()
			currentBytes := t.bytesOut.Load()
			samplesDelta := currentSamples - lastSamplesOut
			bytesDelta := currentBytes - lastBytesOut
			t.logger.Log(t.ctx, observability.LevelTrace, "ES transcoder output throughput",
				slog.String("id", t.id),
				slog.Uint64("samples_out_5s", samplesDelta),
				slog.Float64("kbps_out", float64(bytesDelta)/5.0/1024),
				slog.Int("job_samples_ch_len", len(t.activeJob.Samples)))
			lastSamplesOut = currentSamples
			lastBytesOut = currentBytes
		case <-t.activeJob.Done:
			// Job was closed (possibly due to error)
			t.logger.Debug("runOutputLoop exiting: activeJob.Done closed",
				slog.String("id", t.id),
				slog.Uint64("total_samples_out", t.samplesOut.Load()))
			if t.activeJob.Err != nil {
				t.errorCount.Add(1)
				t.logger.Warn("Active job error",
					slog.String("id", t.id),
					slog.String("error", t.activeJob.Err.Error()))
			}
			return
		case batch, ok := <-t.activeJob.Samples:
			if !ok {
				t.logger.Debug("runOutputLoop exiting: Samples channel closed",
					slog.String("id", t.id),
					slog.Uint64("total_samples_out", t.samplesOut.Load()))
				return
			}
			// Log first batch received for debugging pipeline connectivity
			if !firstBatchReceived {
				firstBatchReceived = true
				t.logger.Info("First sample batch received from daemon",
					slog.String("id", t.id),
					slog.String("target_variant", target.Variant().String()),
					slog.Int("video_samples", len(batch.VideoSamples)),
					slog.Int("audio_samples", len(batch.AudioSamples)))
			}
			// Received transcoded samples from daemon
			if err := t.handleOutputSamples(target, batch); err != nil {
				t.errorCount.Add(1)
				t.logger.Warn("Error handling output samples",
					slog.String("id", t.id),
					slog.String("error", err.Error()))
			}
		}
	}
}

// handleOutputSamples writes transcoded samples to target variant.
func (t *ESTranscoder) handleOutputSamples(target *ESVariant, batch *proto.ESSampleBatch) error {
	// Process video samples
	for _, sample := range batch.VideoSamples {
		if sample.IsKeyframe {
			t.logger.Log(t.ctx, observability.LevelTrace, "Writing keyframe to target variant",
				slog.String("id", t.id),
				slog.String("target_variant", target.Variant().String()),
				slog.Int64("pts", sample.Pts),
				slog.Uint64("sequence", sample.Sequence),
				slog.Int("data_len", len(sample.Data)))
		}
		target.WriteVideo(sample.Pts, sample.Dts, sample.Data, sample.IsKeyframe)
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(sample.Data)))
	}

	// Process audio samples
	for _, sample := range batch.AudioSamples {
		target.WriteAudio(sample.Pts, sample.Data)
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(sample.Data)))
	}

	t.recordActivity()
	return nil
}

// handleStats updates transcoder stats from ffmpegd.
func (t *ESTranscoder) handleStats(stats *proto.TranscodeStats) {
	if stats == nil {
		return
	}
	t.encodingSpeed.Store(stats.EncodingSpeed)
	t.cpuPercent.Store(stats.CpuPercent)
	t.memoryMB.Store(stats.MemoryMb)
	if stats.FfmpegPid > 0 {
		t.ffmpegPID.Store(stats.FfmpegPid)
	}

	// Record history for sparkline graphs (sample periodically)
	if t.resourceHistory != nil && t.resourceHistory.ShouldSample() {
		t.resourceHistory.AddSample(stats.CpuPercent, stats.MemoryMb)
	}
}

// recordActivity updates last activity time.
func (t *ESTranscoder) recordActivity() {
	t.lastActivity.Store(time.Now())
}

// modeString returns a string representation of the transcoder mode.
func (t *ESTranscoder) modeString() string {
	if t.mode == ESTranscoderModeLocal {
		return "local"
	}
	return "remote"
}

// Verify ESTranscoder implements Transcoder interface.
var _ Transcoder = (*ESTranscoder)(nil)
