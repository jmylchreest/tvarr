// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internalffmpeg "github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/util"
	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/shirou/gopsutil/v4/process"
)

// OutputDemuxer is the interface for demuxing FFmpeg output streams.
// Implementations include TSDemuxer for MPEG-TS and FMP4Demuxer for fragmented MP4.
type OutputDemuxer interface {
	// Write processes data from FFmpeg output and calls callbacks for each sample
	Write(data []byte) error
	// Close releases resources
	Close()
}

// TranscodeJob handles a single transcoding job with FFmpeg process management.
// It receives ES samples via gRPC, feeds them to FFmpeg, and returns transcoded samples.
type TranscodeJob struct {
	id     string
	logger *slog.Logger
	config *proto.TranscodeStart
	binInfo *ffmpeg.BinaryInfo

	// FFmpeg process
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Stderr capture for debugging
	stderrLines    []string
	stderrMu       sync.RWMutex
	maxStderrLines int

	// Input muxer for feeding FFmpeg (ES samples -> container -> stdin)
	// Can be TSMuxer (for H.264/H.265) or FMP4Muxer (for VP9/AV1)
	inputMuxer InputMuxer
	inputBuf   bytes.Buffer

	// Async input writer (decouples gRPC receive from FFmpeg stdin writes)
	inputCh       chan []byte
	inputDone     chan struct{}
	inputDropped  atomic.Uint64

	// Output demuxer for parsing FFmpeg output (stdout -> ES samples)
	// Can be TSDemuxer for MPEG-TS or FMP4Demuxer for fragmented MP4
	outputDemuxer OutputDemuxer

	// Output channel for transcoded samples
	outputCh chan *proto.ESSampleBatch

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
	encodingSpeed atomic.Value // float64 - realtime encoding speed (1.0 = realtime)

	// Actual configuration used (may differ from requested)
	actualVideoEncoder string
	actualAudioEncoder string
	actualHWAccel      string
	actualHWDevice     string
	ffmpegCommand      string // Full FFmpeg command line for debugging
}

// NewTranscodeJob creates a new transcode job from a TranscodeStart message.
func NewTranscodeJob(
	id string,
	config *proto.TranscodeStart,
	binInfo *ffmpeg.BinaryInfo,
	logger *slog.Logger,
) *TranscodeJob {
	if logger == nil {
		logger = slog.Default()
	}

	t := &TranscodeJob{
		id:             id,
		logger:         logger,
		config:         config,
		binInfo:        binInfo,
		closedCh:       make(chan struct{}),
		outputCh:       make(chan *proto.ESSampleBatch, 10000), // Large buffer - coordinator can terminate if needed
		inputCh:        make(chan []byte, 1000),                // Async input buffer - decouples gRPC from FFmpeg stdin
		inputDone:      make(chan struct{}),
		stderrLines:    make([]string, 0, 50),
		maxStderrLines: 50,
	}
	t.lastActivity.Store(time.Now())

	return t
}

// Start begins the transcoding process and returns the actual configuration used.
func (t *TranscodeJob) Start(ctx context.Context) (*proto.TranscodeAck, error) {
	if !t.started.CompareAndSwap(false, true) {
		return nil, errors.New("job already started")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.startedAt = time.Now()

	// Log received audio init data
	if len(t.config.AudioInitData) > 0 {
		t.logger.Info("Received AudioInitData from coordinator",
			slog.Int("init_data_len", len(t.config.AudioInitData)),
			slog.String("source_audio_codec", t.config.SourceAudioCodec))
	} else {
		t.logger.Warn("No AudioInitData received from coordinator, will use defaults",
			slog.String("source_audio_codec", t.config.SourceAudioCodec))
	}

	// Select input muxer based on source video codec:
	// - VP9/AV1: Use fMP4 muxer (not compatible with MPEG-TS)
	// - H.264/H.265: Use MPEG-TS muxer (standard for these codecs)
	if RequiresFMP4Input(t.config.SourceVideoCodec) {
		t.logger.Info("Using fMP4 input muxer for source codec",
			slog.String("source_video_codec", t.config.SourceVideoCodec))
		t.inputMuxer = NewFMP4Muxer(&t.inputBuf, FMP4MuxerConfig{
			Logger:        t.logger,
			VideoCodec:    t.config.SourceVideoCodec,
			AudioCodec:    t.config.SourceAudioCodec,
			AudioInitData: t.config.AudioInitData,
		})
	} else {
		t.logger.Info("Using MPEG-TS input muxer for source codec",
			slog.String("source_video_codec", t.config.SourceVideoCodec))
		t.inputMuxer = NewTSMuxer(&t.inputBuf, TSMuxerConfig{
			Logger:        t.logger,
			VideoCodec:    t.config.SourceVideoCodec,
			AudioCodec:    t.config.SourceAudioCodec,
			AudioInitData: t.config.AudioInitData, // Pass AudioSpecificConfig for correct ADTS parameters
		})
	}

	// Pre-initialize the muxer to write headers immediately
	// For MPEG-TS: writes PAT/PMT tables
	// For fMP4: initialization segment is written on first flush after video params are available
	if _, err := t.inputMuxer.InitializeAndGetHeader(); err != nil {
		return &proto.TranscodeAck{
			Success: false,
			Error:   fmt.Sprintf("failed to initialize muxer: %v", err),
		}, nil
	}

	// Common callbacks for both demuxers
	videoCallback := func(pts, dts int64, data []byte, isKeyframe bool) {
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(data)))
		t.recordActivity()
		t.enqueueOutputSample(&proto.ESSample{
			Pts:        pts,
			Dts:        dts,
			Data:       data,
			IsKeyframe: isKeyframe,
			Sequence:   t.samplesOut.Load(),
		}, true)
	}
	audioCallback := func(pts int64, data []byte) {
		t.samplesOut.Add(1)
		t.bytesOut.Add(uint64(len(data)))
		t.recordActivity()
		t.enqueueOutputSample(&proto.ESSample{
			Pts:      pts,
			Dts:      pts,
			Data:     data,
			Sequence: t.samplesOut.Load(),
		}, false)
	}

	// Select demuxer based on output format
	// AV1/VP9 require fMP4 demuxer, H.264/H.265 use MPEG-TS demuxer
	outputFormat := t.selectOutputFormat()
	if outputFormat == "fmp4" {
		t.logger.Info("using fMP4 demuxer for output",
			slog.String("job_id", t.id),
			slog.String("target_video_codec", t.config.TargetVideoCodec),
		)
		t.outputDemuxer = NewFMP4Demuxer(FMP4DemuxerConfig{
			Logger:           t.logger,
			TargetVideoCodec: t.config.TargetVideoCodec,
			TargetAudioCodec: t.config.TargetAudioCodec,
			OnVideoSample:    videoCallback,
			OnAudioSample:    audioCallback,
		})
	} else {
		t.logger.Info("using MPEG-TS demuxer for output",
			slog.String("job_id", t.id),
			slog.String("target_video_codec", t.config.TargetVideoCodec),
		)
		t.outputDemuxer = NewTSDemuxer(TSDemuxerConfig{
			Logger:           t.logger,
			TargetVideoCodec: t.config.TargetVideoCodec,
			TargetAudioCodec: t.config.TargetAudioCodec,
			OnVideoSample:    videoCallback,
			OnAudioSample:    audioCallback,
		})
	}

	// Start FFmpeg process
	if err := t.startFFmpeg(); err != nil {
		return &proto.TranscodeAck{
			Success: false,
			Error:   fmt.Sprintf("failed to start FFmpeg: %v", err),
		}, nil
	}

	// Start the async input writer goroutine BEFORE sending any data
	// This goroutine reads from inputCh and writes to FFmpeg stdin asynchronously,
	// preventing gRPC flow control from blocking when FFmpeg stdin buffer fills
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runInputWriter()
	}()

	// Flush the PAT/PMT header to FFmpeg stdin immediately after starting
	// This ensures FFmpeg sees the codec information before any media data
	if t.inputBuf.Len() > 0 {
		headerData := make([]byte, t.inputBuf.Len())
		copy(headerData, t.inputBuf.Bytes())
		t.inputBuf.Reset()

		select {
		case t.inputCh <- headerData:
			t.logger.Debug("queued PAT/PMT header for FFmpeg",
				slog.String("job_id", t.id),
				slog.Int("bytes", len(headerData)),
			)
		case <-t.ctx.Done():
			return &proto.TranscodeAck{
				Success: false,
				Error:   "job cancelled while queueing header",
			}, nil
		}
	}

	t.logger.Debug("FFmpeg transcoder started",
		slog.String("job_id", t.id),
		slog.String("source_video", t.config.SourceVideoCodec),
		slog.String("target_video", t.config.TargetVideoCodec),
		slog.String("video_encoder", t.actualVideoEncoder),
		slog.String("hw_accel", t.actualHWAccel),
	)

	// Start output reader goroutine
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runOutputLoop()
	}()

	return &proto.TranscodeAck{
		Success:            true,
		ActualVideoEncoder: t.actualVideoEncoder,
		ActualAudioEncoder: t.actualAudioEncoder,
		ActualHwAccel:      t.actualHWAccel,
	}, nil
}

// ProcessSamples feeds ES samples to the FFmpeg process.
func (t *TranscodeJob) ProcessSamples(batch *proto.ESSampleBatch) error {
	if t.closed.Load() {
		return errors.New("job closed")
	}

	// Write video samples to muxer
	for _, sample := range batch.VideoSamples {
		if err := t.inputMuxer.WriteVideo(sample.Pts, sample.Dts, sample.Data, sample.IsKeyframe); err != nil {
			t.errorCount.Add(1)
			t.logger.Warn("error writing video sample to muxer",
				slog.String("job_id", t.id),
				slog.String("error", err.Error()),
			)
			continue
		}
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Write audio samples to muxer
	for _, sample := range batch.AudioSamples {
		if err := t.inputMuxer.WriteAudio(sample.Pts, sample.Data); err != nil {
			t.errorCount.Add(1)
			t.logger.Warn("error writing audio sample to muxer",
				slog.String("job_id", t.id),
				slog.String("error", err.Error()),
			)
			continue
		}
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Flush muxer and queue data for async write to FFmpeg stdin
	if err := t.inputMuxer.Flush(); err != nil {
		t.errorCount.Add(1)
		t.logger.Warn("error flushing muxer",
			slog.String("job_id", t.id),
			slog.String("error", err.Error()),
		)
	}
	if t.inputBuf.Len() > 0 {
		// Copy the data since we're sending to a channel and need to reset the buffer
		data := make([]byte, t.inputBuf.Len())
		copy(data, t.inputBuf.Bytes())
		t.inputBuf.Reset()

		// Non-blocking send to input channel
		// If channel is full, we drop data rather than blocking gRPC
		select {
		case t.inputCh <- data:
			t.recordActivity()
		default:
			// Channel full - drop data and track it
			dropped := t.inputDropped.Add(1)
			if dropped == 1 || dropped%100 == 0 {
				t.logger.Warn("input buffer full, dropping data",
					slog.String("job_id", t.id),
					slog.Int("bytes", len(data)),
					slog.Uint64("total_dropped", dropped),
				)
			}
		}
	}

	return nil
}

// OutputChannel returns the channel that receives transcoded sample batches.
func (t *TranscodeJob) OutputChannel() <-chan *proto.ESSampleBatch {
	return t.outputCh
}

// Stop stops the transcoding job.
func (t *TranscodeJob) Stop() {
	if t.closed.CompareAndSwap(false, true) {
		if t.cancel != nil {
			t.cancel()
		}

		// Close input channel to signal the input writer to stop
		// The input writer will close stdin when it exits
		close(t.inputCh)

		// Wait for input writer to finish and close stdin
		select {
		case <-t.inputDone:
		case <-time.After(2 * time.Second):
			t.logger.Warn("input writer did not finish in time, forcing stdin close",
				slog.String("job_id", t.id))
			if t.stdin != nil {
				t.stdin.Close()
			}
		}

		// Wait for process to exit with timeout
		if t.cmd != nil && t.cmd.Process != nil {
			t.waitWithTimeout(3 * time.Second)
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
			t.logger.Warn("transcode job goroutines did not finish in time",
				slog.String("job_id", t.id))
		}

		// Close output channel
		close(t.outputCh)

		t.logger.Debug("transcode job stopped",
			slog.String("job_id", t.id),
			slog.Uint64("samples_in", t.samplesIn.Load()),
			slog.Uint64("samples_out", t.samplesOut.Load()),
			slog.Uint64("input_dropped", t.inputDropped.Load()),
		)
	}
}

// Stats returns current job statistics.
func (t *TranscodeJob) Stats() *proto.TranscodeStats {
	lastActivity, _ := t.lastActivity.Load().(time.Time)
	encodingSpeed, _ := t.encodingSpeed.Load().(float64)

	var pid int32
	if t.cmd != nil && t.cmd.Process != nil {
		pid = int32(t.cmd.Process.Pid)
	}

	// Calculate CPU and memory from process
	var cpuPercent, memoryMB float64
	if procStats := t.processStats(); procStats != nil {
		cpuPercent = procStats.CPUPercent
		memoryMB = procStats.MemoryMB
	}

	_ = lastActivity // unused for now, but can be used for calculating running time

	return &proto.TranscodeStats{
		SamplesIn:     t.samplesIn.Load(),
		SamplesOut:    t.samplesOut.Load(),
		BytesIn:       t.bytesIn.Load(),
		BytesOut:      t.bytesOut.Load(),
		EncodingSpeed: encodingSpeed,
		CpuPercent:    cpuPercent,
		MemoryMb:      memoryMB,
		FfmpegPid:     pid,
		RunningTime:   nil, // Set by caller based on lastActivity
		HwAccel:       t.actualHWAccel,
		HwDevice:      t.actualHWDevice,
		FfmpegCommand: t.ffmpegCommand,
	}
}

// IsClosed returns true if the job is closed.
func (t *TranscodeJob) IsClosed() bool {
	return t.closed.Load()
}

// startFFmpeg starts the FFmpeg process.
func (t *TranscodeJob) startFFmpeg() error {
	ffmpegPath := t.binInfo.FFmpegPath
	if ffmpegPath == "" {
		// Use shared binary finder: TVARR_FFMPEG_BINARY env -> ./ffmpeg -> PATH
		path, err := util.FindBinary("ffmpeg", "TVARR_FFMPEG_BINARY")
		if err != nil {
			return fmt.Errorf("ffmpeg not found: %w", err)
		}
		ffmpegPath = path
	}

	builder := internalffmpeg.NewCommandBuilder(ffmpegPath)

	// Global flags
	builder.HideBanner().LogLevel("warning").Stats()

	// Apply custom global flags from encoding profile (placed early in command)
	// Note: Using ApplyCustomInputOptions since global flags go before -i
	if t.config.GlobalFlags != "" {
		builder.ApplyCustomInputOptions(t.config.GlobalFlags)
	}

	// Select the best encoder for the target codec based on available hardware.
	// The daemon determines the optimal encoder locally based on its detected
	// capabilities, optionally preferring a specific hwaccel if requested.
	selector := NewEncoderSelector(t.binInfo)

	// Log available hardware info for debugging
	t.logger.Debug("available hardware acceleration",
		slog.String("job_id", t.id),
		slog.Int("encoder_count", len(t.binInfo.Encoders)),
		slog.Int("hwaccel_count", len(t.binInfo.HWAccels)),
		slog.String("preferred_hwaccel", t.config.PreferredHwAccel),
	)
	for _, accel := range t.binInfo.HWAccels {
		t.logger.Debug("hwaccel info",
			slog.String("job_id", t.id),
			slog.String("type", string(accel.Type)),
			slog.Bool("available", accel.Available),
			slog.String("device", accel.DeviceName),
			slog.Any("encoders", accel.Encoders),
			slog.Any("decoders", accel.Decoders),
		)
	}

	// Use the preferred hwaccel from the config if specified
	videoEncoder, hwAccel, hwDevice := selector.SelectVideoEncoderWithPreference(
		t.config.TargetVideoCodec,
		t.config.PreferredHwAccel,
	)

	// Apply encoder overrides if any match the current conditions.
	// This allows forcing specific encoders when hardware encoders are known to be broken
	// (e.g., hevc_vaapi on AMD GPUs with Mesa 21.1+).
	if len(t.config.EncoderOverrides) > 0 {
		cpuInfo := getCPUInfo()
		t.logger.Debug("checking video encoder overrides",
			slog.String("job_id", t.id),
			slog.Int("override_count", len(t.config.EncoderOverrides)),
			slog.String("target_codec", t.config.TargetVideoCodec),
			slog.String("current_encoder", videoEncoder),
			slog.String("hw_accel", hwAccel),
			slog.String("cpu_info", cpuInfo),
		)
		originalEncoder := videoEncoder
		videoEncoder = selector.ApplyEncoderOverride(
			"video",
			t.config.TargetVideoCodec,
			videoEncoder,
			hwAccel,
			cpuInfo,
			t.config.EncoderOverrides,
		)
		// If encoder was overridden to software, clear hwaccel settings
		if videoEncoder != originalEncoder && !IsHardwareEncoder(videoEncoder) {
			hwAccel = ""
			hwDevice = ""
		}
	}

	t.logger.Info("selected video encoder",
		slog.String("job_id", t.id),
		slog.String("target_codec", t.config.TargetVideoCodec),
		slog.String("preferred_hwaccel", t.config.PreferredHwAccel),
		slog.String("encoder", videoEncoder),
		slog.String("hw_accel", hwAccel),
		slog.String("hw_device", hwDevice),
	)

	// Hardware acceleration setup if a hardware encoder was selected.
	// Uses the same pattern as main: init_hw_device + hwaccel + hwupload
	//
	// FFmpeg command structure:
	//   -init_hw_device vaapi=hw:/dev/dri/renderD128
	//   -hwaccel vaapi -hwaccel_device /dev/dri/renderD128
	//   -vf format=nv12,hwupload
	//
	// The hwupload filter transfers frames to GPU memory for encoding.
	// Skip if custom flags already contain hwaccel options (user manages it).
	customFlagsHaveHwaccel := strings.Contains(t.config.GlobalFlags, "-hwaccel") ||
		strings.Contains(t.config.InputFlags, "-hwaccel")
	if hwAccel != "" && !customFlagsHaveHwaccel {
		builder.InitHWDevice(hwAccel, hwDevice)
		builder.HWAccel(hwAccel)
		if hwDevice != "" {
			builder.HWAccelDevice(hwDevice)
		}
		t.actualHWAccel = hwAccel
		t.actualHWDevice = hwDevice
	} else if customFlagsHaveHwaccel {
		t.logger.Debug("skipping auto hwaccel, custom flags contain hwaccel options",
			slog.String("job_id", t.id))
	}

	// Input settings - format depends on the input muxer
	// MPEG-TS for H.264/H.265, fMP4 for VP9/AV1
	inputFormat := t.inputMuxer.Format()
	builder.InputArgs("-f", inputFormat)
	builder.InputArgs("-analyzeduration", "5000000") // 5 seconds
	builder.InputArgs("-probesize", "5000000")       // 5MB
	t.logger.Debug("FFmpeg input format",
		slog.String("job_id", t.id),
		slog.String("format", inputFormat))

	// Apply custom input flags from encoding profile (placed before -i)
	if t.config.InputFlags != "" {
		builder.ApplyCustomInputOptions(t.config.InputFlags)
	}

	builder.Input("pipe:0")

	// Stream mapping
	builder.OutputArgs("-map", "0:v:0")
	builder.OutputArgs("-map", "0:a:0?")

	// Video codec - use the locally selected encoder
	t.actualVideoEncoder = videoEncoder
	builder.VideoCodec(videoEncoder)

	// Add hardware upload filter when using hardware encoder.
	// This matches main branch behavior - always add hwupload for HW encoders.
	// The hwupload filter transfers decoded frames to GPU memory for encoding.
	if hwAccel != "" && IsHardwareEncoder(videoEncoder) {
		builder.HWUploadFilter(hwAccel)
	}

	if t.config.VideoBitrateKbps > 0 {
		builder.VideoBitrate(fmt.Sprintf("%dk", t.config.VideoBitrateKbps))
	}
	// Only apply preset for software encoders - hardware encoders don't use the same preset system
	if t.config.VideoPreset != "" && !IsHardwareEncoder(videoEncoder) {
		builder.VideoPreset(t.config.VideoPreset)
	}

	// Audio codec - use locally selected encoder
	audioEncoder := selector.SelectAudioEncoder(t.config.TargetAudioCodec)

	// Apply encoder overrides for audio if any match
	if len(t.config.EncoderOverrides) > 0 {
		cpuInfo := getCPUInfo()
		t.logger.Debug("checking audio encoder overrides",
			slog.String("job_id", t.id),
			slog.Int("override_count", len(t.config.EncoderOverrides)),
			slog.String("target_codec", t.config.TargetAudioCodec),
			slog.String("current_encoder", audioEncoder),
		)
		audioEncoder = selector.ApplyEncoderOverride(
			"audio",
			t.config.TargetAudioCodec,
			audioEncoder,
			"", // No hwaccel for audio
			cpuInfo,
			t.config.EncoderOverrides,
		)
	}

	t.actualAudioEncoder = audioEncoder
	builder.AudioCodec(audioEncoder)

	if t.config.AudioBitrateKbps > 0 {
		builder.AudioBitrate(fmt.Sprintf("%dk", t.config.AudioBitrateKbps))
	}

	// For AAC encoding, force stereo output
	if audioEncoder == "aac" {
		builder.AudioChannels(2)
	}

	// Select output format based on target codec
	// AV1/VP9 require fMP4 since they're not compatible with MPEG-TS
	outputFormat := t.selectOutputFormat()

	// Apply audio bitstream filter when copying AAC to fMP4/MP4 container.
	// AAC in MPEG-TS uses ADTS format, but MP4/fMP4 requires ASC format.
	// The aac_adtstoasc filter converts between these formats without re-encoding.
	if audioEncoder == "copy" && outputFormat == "fmp4" {
		sourceAudioCodec := normalizeCodec(t.config.SourceAudioCodec)
		if sourceAudioCodec == "aac" {
			builder.OutputArgs("-bsf:a", "aac_adtstoasc")
			t.logger.Debug("applying aac_adtstoasc bitstream filter for AAC copy to fMP4",
				slog.String("job_id", t.id),
				slog.String("source_audio", t.config.SourceAudioCodec),
			)
		}
	}

	// Apply custom output flags from encoding profile (placed after codec settings)
	if t.config.OutputFlags != "" {
		builder.ApplyCustomOutputOptions(t.config.OutputFlags)
	}
	t.logger.Info("selected output format",
		slog.String("job_id", t.id),
		slog.String("output_format", outputFormat),
		slog.String("target_video_codec", t.config.TargetVideoCodec),
		slog.String("config_format", t.config.OutputContainerFormat),
	)

	if outputFormat == "fmp4" {
		// Fragmented MP4 output for AV1/VP9 or when requested
		builder.OutputArgs("-f", "mp4")

		// Choose movflags based on audio codec requirements.
		// - empty_moov: Writes moov immediately (before any media processed)
		// - delay_moov: Waits until first fragment is processed to write moov
		//
		// delay_moov is required for:
		// 1. Audio copy mode - FFmpeg needs to extract codec info from ADTS headers
		// 2. AC3/EAC3 encoding - FFmpeg cannot write moov before AC3 packets are analyzed
		// 3. Opus encoding - Similar issue with codec parameter extraction
		needsDelayMoov := audioEncoder == "copy" ||
			audioEncoder == "ac3" ||
			audioEncoder == "eac3" ||
			audioEncoder == "libopus"

		if needsDelayMoov {
			builder.OutputArgs("-movflags", "frag_keyframe+delay_moov+default_base_moof")
			t.logger.Debug("using delay_moov for audio codec",
				slog.String("job_id", t.id),
				slog.String("audio_encoder", audioEncoder),
				slog.String("reason", "codec requires packet analysis before moov can be written"),
			)
		} else {
			builder.OutputArgs("-movflags", "frag_keyframe+empty_moov+default_base_moof")
		}
	} else {
		// MPEG-TS output for H.264/H.265
		builder.MpegtsArgs()
		builder.MuxDelay("0")
	}
	builder.FlushPackets()
	builder.Output("pipe:1")

	// Build command
	ffmpegCmd := builder.Build()
	t.ffmpegCommand = ffmpegCmd.String()
	t.logger.Info("FFmpeg command",
		slog.String("job_id", t.id),
		slog.String("command", t.ffmpegCommand))

	// Create command with context
	t.cmd = exec.CommandContext(t.ctx, ffmpegCmd.Binary, ffmpegCmd.Args...)

	// Set up pipes
	var err error

	closePipes := func() {
		if t.stdin != nil {
			t.stdin.Close()
			t.stdin = nil
		}
		if t.stdout != nil {
			t.stdout.Close()
			t.stdout = nil
		}
		if t.stderr != nil {
			t.stderr.Close()
			t.stderr = nil
		}
	}

	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		closePipes()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	t.stderr, err = t.cmd.StderrPipe()
	if err != nil {
		closePipes()
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Start process
	if err := t.cmd.Start(); err != nil {
		closePipes()
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	// Start stderr reader goroutine
	go t.readStderr()

	return nil
}

// runInputWriter reads from the input channel and writes to FFmpeg stdin.
// This goroutine decouples gRPC receive from FFmpeg stdin writes, preventing
// blocking when FFmpeg's stdin buffer is full (e.g., during probe phase).
func (t *TranscodeJob) runInputWriter() {
	defer close(t.inputDone)
	defer func() {
		// Close stdin to signal EOF to FFmpeg when we're done
		if t.stdin != nil {
			t.stdin.Close()
		}
	}()

	t.logger.Debug("input writer started",
		slog.String("job_id", t.id),
	)

	bytesWritten := uint64(0)
	writeCount := 0

	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug("input writer stopping due to context cancellation",
				slog.String("job_id", t.id),
				slog.Uint64("bytes_written", bytesWritten),
				slog.Int("write_count", writeCount),
			)
			return

		case data, ok := <-t.inputCh:
			if !ok {
				t.logger.Debug("input writer stopping - channel closed",
					slog.String("job_id", t.id),
					slog.Uint64("bytes_written", bytesWritten),
					slog.Int("write_count", writeCount),
				)
				return
			}

			if t.stdin == nil {
				continue
			}

			// Write to FFmpeg stdin - this may block, but that's OK since
			// we're in a dedicated goroutine and gRPC won't be blocked
			n, err := t.stdin.Write(data)
			if err != nil {
				if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) {
					t.logger.Debug("input writer stopping - pipe closed",
						slog.String("job_id", t.id),
						slog.Uint64("bytes_written", bytesWritten),
					)
					return
				}
				t.errorCount.Add(1)
				t.logger.Warn("error writing to FFmpeg stdin",
					slog.String("job_id", t.id),
					slog.String("error", err.Error()),
				)
				// Continue trying - FFmpeg might recover
				continue
			}

			bytesWritten += uint64(n)
			writeCount++

			// Log throughput periodically
			if writeCount%500 == 0 {
				t.logger.Log(t.ctx, observability.LevelTrace, "input writer throughput",
					slog.String("job_id", t.id),
					slog.Uint64("bytes_written", bytesWritten),
					slog.Int("write_count", writeCount),
					slog.Int("channel_len", len(t.inputCh)),
				)
			}
		}
	}
}

// runOutputLoop reads FFmpeg output and sends transcoded samples to the output channel.
func (t *TranscodeJob) runOutputLoop() {
	buf := make([]byte, 188*100) // Read in chunks of TS packets

	// Throughput logging
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()
	var lastBytesRead, totalBytesRead uint64

	for {
		select {
		case <-t.ctx.Done():
			t.logger.Debug("runOutputLoop exiting: context done",
				slog.String("job_id", t.id),
				slog.Uint64("total_bytes_read", totalBytesRead))
			return
		case <-logTicker.C:
			bytesDelta := totalBytesRead - lastBytesRead
			t.logger.Log(t.ctx, observability.LevelTrace, "FFmpeg output throughput",
				slog.String("job_id", t.id),
				slog.Uint64("bytes_read_5s", bytesDelta),
				slog.Float64("kbps", float64(bytesDelta)/5.0/1024),
				slog.Uint64("samples_out", t.samplesOut.Load()),
				slog.Int("output_ch_len", len(t.outputCh)))
			lastBytesRead = totalBytesRead
		default:
		}

		n, err := t.stdout.Read(buf)
		if err != nil {
			if err == io.EOF || errors.Is(err, context.Canceled) {
				t.logger.Debug("runOutputLoop exiting: FFmpeg stdout closed",
					slog.String("job_id", t.id),
					slog.String("reason", err.Error()),
					slog.Uint64("total_bytes_read", totalBytesRead))
				return
			}
			t.errorCount.Add(1)
			t.logger.Warn("error reading FFmpeg output",
				slog.String("job_id", t.id),
				slog.String("error", err.Error()),
			)
			return
		}

		if n > 0 {
			totalBytesRead += uint64(n)
			if err := t.outputDemuxer.Write(buf[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "closed pipe") {
					t.logger.Debug("runOutputLoop exiting: demuxer pipe closed",
						slog.String("job_id", t.id))
					return
				}
				t.errorCount.Add(1)
				t.logger.Warn("error demuxing FFmpeg output",
					slog.String("job_id", t.id),
					slog.String("error", err.Error()),
				)
				return
			}
			t.recordActivity()
		}
	}
}

// enqueueOutputSample adds a sample to the output batch and sends when ready.
func (t *TranscodeJob) enqueueOutputSample(sample *proto.ESSample, isVideo bool) {
	// Create a batch with the single sample
	// In a more sophisticated implementation, we would batch samples together
	batch := &proto.ESSampleBatch{
		IsSource:      false,
		BatchSequence: sample.Sequence,
	}

	if isVideo {
		batch.VideoSamples = []*proto.ESSample{sample}
	} else {
		batch.AudioSamples = []*proto.ESSample{sample}
	}

	// Non-blocking send with timeout to detect backpressure issues
	select {
	case t.outputCh <- batch:
		// Sample queued successfully
	case <-t.ctx.Done():
		// Job cancelled, stop processing
		return
	case <-time.After(5 * time.Second):
		// Output channel blocked for 5 seconds - log warning and drop sample
		t.logger.Warn("output channel blocked for 5s, dropping sample (backpressure)",
			slog.String("job_id", t.id),
			slog.Int("output_ch_len", len(t.outputCh)),
			slog.Int("output_ch_cap", cap(t.outputCh)),
			slog.Bool("is_video", isVideo),
			slog.Uint64("sequence", sample.Sequence))
	}
}

// readStderr reads FFmpeg stderr and stores recent lines for debugging.
func (t *TranscodeJob) readStderr() {
	scanner := bufio.NewScanner(t.stderr)
	scanner.Split(scanLinesWithCR)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse encoding speed from FFmpeg output
		if speed := t.parseEncodingSpeed(line); speed > 0 {
			t.encodingSpeed.Store(speed)
		}

		// Only store non-progress lines
		if !strings.Contains(line, "frame=") {
			t.stderrMu.Lock()
			t.stderrLines = append(t.stderrLines, line)
			if len(t.stderrLines) > t.maxStderrLines {
				t.stderrLines = t.stderrLines[1:]
			}
			t.stderrMu.Unlock()

			if !strings.Contains(line, "speed=") {
				t.logger.Warn("FFmpeg stderr",
					slog.String("job_id", t.id),
					slog.String("line", line))
			}
		}
	}
}

// scanLinesWithCR handles both \r and \n as line delimiters.
func scanLinesWithCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			advance = i + 1
			for advance < len(data) && (data[advance] == '\r' || data[advance] == '\n') {
				advance++
			}
			return advance, data[0:i], nil
		}
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

// parseEncodingSpeed extracts the encoding speed from FFmpeg stderr output.
func (t *TranscodeJob) parseEncodingSpeed(line string) float64 {
	idx := strings.Index(line, "speed=")
	if idx == -1 {
		return 0
	}

	speedStr := line[idx+6:]
	speedStr = strings.TrimLeft(speedStr, " ")

	endIdx := strings.IndexAny(speedStr, "x \t")
	if endIdx > 0 {
		speedStr = speedStr[:endIdx]
	}

	speed, err := strconv.ParseFloat(strings.TrimSpace(speedStr), 64)
	if err != nil {
		return 0
	}

	return speed
}

// GetStderrLines returns the recent stderr lines from FFmpeg.
func (t *TranscodeJob) GetStderrLines() []string {
	t.stderrMu.RLock()
	defer t.stderrMu.RUnlock()

	lines := make([]string, len(t.stderrLines))
	copy(lines, t.stderrLines)
	return lines
}

// waitWithTimeout waits for the FFmpeg process to exit.
func (t *TranscodeJob) waitWithTimeout(timeout time.Duration) {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}

	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()

	select {
	case <-done:
		return
	case <-time.After(timeout):
		t.logger.Warn("FFmpeg process did not exit in time, sending SIGTERM",
			slog.String("job_id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		_ = t.cmd.Process.Signal(os.Interrupt)
	}

	select {
	case <-done:
		return
	case <-time.After(500 * time.Millisecond):
		t.logger.Warn("FFmpeg process did not respond to SIGTERM, killing",
			slog.String("job_id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		_ = t.cmd.Process.Kill()
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.logger.Error("FFmpeg process could not be killed",
			slog.String("job_id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		go func() { <-done }()
	}
}

// recordActivity updates last activity time.
func (t *TranscodeJob) recordActivity() {
	t.lastActivity.Store(time.Now())
}

// selectOutputFormat returns the output container format to use.
// Returns "fmp4" or "mpegts".
func (t *TranscodeJob) selectOutputFormat() string {
	// Use explicit format from coordinator if provided
	if t.config.OutputContainerFormat != "" {
		return t.config.OutputContainerFormat
	}

	// Auto-select based on target video codec
	// AV1 and VP9 require fMP4 since they're not compatible with MPEG-TS
	switch strings.ToLower(t.config.TargetVideoCodec) {
	case "av1", "vp9":
		return "fmp4"
	default:
		return "mpegts"
	}
}

// processStatsResult contains CPU and memory stats for the FFmpeg process.
type processStatsResult struct {
	CPUPercent float64
	MemoryMB   float64
}

// processStats returns CPU and memory stats for the FFmpeg process using gopsutil.
func (t *TranscodeJob) processStats() *processStatsResult {
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}

	pid := int32(t.cmd.Process.Pid)
	if pid <= 0 {
		return nil
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	stats := &processStatsResult{}

	// Get CPU percent (interval-based for accuracy)
	if cpuPercent, err := proc.CPUPercent(); err == nil {
		stats.CPUPercent = cpuPercent
	}

	// Get memory info
	if memInfo, err := proc.MemoryInfo(); err == nil && memInfo != nil {
		stats.MemoryMB = float64(memInfo.RSS) / (1024 * 1024)
	}

	return stats
}
