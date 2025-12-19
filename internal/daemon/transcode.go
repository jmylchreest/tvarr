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
	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

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

	// TS muxer for feeding FFmpeg (ES samples -> MPEG-TS -> stdin)
	inputMuxer *TSMuxer
	inputBuf   bytes.Buffer

	// TS demuxer for parsing FFmpeg output (stdout -> MPEG-TS -> ES samples)
	outputDemuxer *TSDemuxer

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
		outputCh:       make(chan *proto.ESSampleBatch, 100), // Buffered channel for output samples
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

	// Initialize TS muxer for FFmpeg input
	t.inputMuxer = NewTSMuxer(&t.inputBuf, TSMuxerConfig{
		Logger:     t.logger,
		VideoCodec: t.config.SourceVideoCodec,
		AudioCodec: t.config.SourceAudioCodec,
	})

	// Initialize TS demuxer for FFmpeg output
	t.outputDemuxer = NewTSDemuxer(TSDemuxerConfig{
		Logger:        t.logger,
		TargetVideoCodec: t.config.TargetVideoCodec,
		TargetAudioCodec: t.config.TargetAudioCodec,
		OnVideoSample: func(pts, dts int64, data []byte, isKeyframe bool) {
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
		},
		OnAudioSample: func(pts int64, data []byte) {
			t.samplesOut.Add(1)
			t.bytesOut.Add(uint64(len(data)))
			t.recordActivity()
			t.enqueueOutputSample(&proto.ESSample{
				Pts:      pts,
				Dts:      pts,
				Data:     data,
				Sequence: t.samplesOut.Load(),
			}, false)
		},
	})

	// Start FFmpeg process
	if err := t.startFFmpeg(); err != nil {
		return &proto.TranscodeAck{
			Success: false,
			Error:   fmt.Sprintf("failed to start FFmpeg: %v", err),
		}, nil
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

	// Flush muxer and write to FFmpeg stdin
	t.inputMuxer.Flush()
	if t.inputBuf.Len() > 0 && t.stdin != nil {
		data := t.inputBuf.Bytes()
		_, err := t.stdin.Write(data)
		if err != nil {
			if !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, context.Canceled) {
				t.errorCount.Add(1)
				t.logger.Warn("error writing to FFmpeg stdin",
					slog.String("job_id", t.id),
					slog.String("error", err.Error()),
				)
				return fmt.Errorf("writing to FFmpeg: %w", err)
			}
		}
		t.inputBuf.Reset()
		t.recordActivity()
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

		// Close FFmpeg stdin to signal end of input
		if t.stdin != nil {
			t.stdin.Close()
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
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			return fmt.Errorf("ffmpeg not found: %w", err)
		}
		ffmpegPath = path
	}

	builder := internalffmpeg.NewCommandBuilder(ffmpegPath)

	// Global flags
	builder.HideBanner().LogLevel("warning").Stats()

	// Hardware acceleration (if configured)
	hwAccel := t.config.PreferredHwAccel
	hwDevice := t.config.HwDevice
	if hwAccel != "" {
		builder.InitHWDevice(hwAccel, hwDevice)
		builder.HWAccel(hwAccel)
		if hwDevice != "" {
			builder.HWAccelDevice(hwDevice)
		}
		t.actualHWAccel = hwAccel
	}

	// Input settings - reading MPEG-TS from stdin
	builder.InputArgs("-f", "mpegts")
	builder.InputArgs("-analyzeduration", "500000")
	builder.InputArgs("-probesize", "500000")
	builder.Input("pipe:0")

	// Stream mapping
	builder.OutputArgs("-map", "0:v:0")
	builder.OutputArgs("-map", "0:a:0?")

	// Video codec
	videoEncoder := t.config.VideoEncoder
	if videoEncoder == "" {
		// Fall back to software encoder based on target codec
		switch t.config.TargetVideoCodec {
		case "h264", "avc":
			videoEncoder = "libx264"
		case "h265", "hevc":
			videoEncoder = "libx265"
		case "vp9":
			videoEncoder = "libvpx-vp9"
		case "av1":
			videoEncoder = "libaom-av1"
		default:
			videoEncoder = "libx264"
		}
	}

	t.actualVideoEncoder = videoEncoder
	builder.VideoCodec(videoEncoder)

	// Hardware upload filter if using HW encoder
	if hwAccel != "" && internalffmpeg.IsHardwareEncoder(videoEncoder) {
		builder.HWUploadFilter(hwAccel)
	}

	if t.config.VideoBitrateKbps > 0 {
		builder.VideoBitrate(fmt.Sprintf("%dk", t.config.VideoBitrateKbps))
	}
	if t.config.VideoPreset != "" {
		builder.VideoPreset(t.config.VideoPreset)
	}

	// Audio codec
	audioEncoder := t.config.AudioEncoder
	if audioEncoder == "" {
		audioEncoder = "aac"
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

	// Output MPEG-TS to stdout
	builder.MpegtsArgs()
	builder.FlushPackets()
	builder.MuxDelay("0")
	builder.Output("pipe:1")

	// Build command
	ffmpegCmd := builder.Build()
	t.logger.Debug("FFmpeg command",
		slog.String("job_id", t.id),
		slog.String("command", ffmpegCmd.String()))

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

// runOutputLoop reads FFmpeg output and sends transcoded samples to the output channel.
func (t *TranscodeJob) runOutputLoop() {
	buf := make([]byte, 188*100) // Read in chunks of TS packets

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		n, err := t.stdout.Read(buf)
		if err != nil {
			if err == io.EOF || errors.Is(err, context.Canceled) {
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
			if err := t.outputDemuxer.Write(buf[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "closed pipe") {
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

	// Non-blocking send to output channel
	select {
	case t.outputCh <- batch:
	default:
		// Channel full, drop sample
		t.logger.Warn("output channel full, dropping sample",
			slog.String("job_id", t.id),
			slog.Bool("is_video", isVideo),
		)
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

// processStats returns CPU and memory stats for the FFmpeg process.
type processStatsResult struct {
	CPUPercent float64
	MemoryMB   float64
}

func (t *TranscodeJob) processStats() *processStatsResult {
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}

	pid := t.cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	stats := &processStatsResult{}

	// Read process stats from /proc on Linux
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err == nil {
		fields := strings.Fields(string(statData))
		if len(fields) >= 14 {
			utime, _ := strconv.ParseUint(fields[13], 10, 64)
			stime, _ := strconv.ParseUint(fields[14], 10, 64)
			totalTicks := utime + stime
			clkTck := uint64(100)
			uptime := time.Since(t.startedAt).Seconds()
			if uptime > 0 {
				stats.CPUPercent = float64(totalTicks) / float64(clkTck) / uptime * 100.0
			}
		}
	}

	// Read memory stats from /proc/[pid]/statm
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	statmData, err := os.ReadFile(statmPath)
	if err == nil {
		statmFields := strings.Fields(string(statmData))
		if len(statmFields) >= 2 {
			rssPages, _ := strconv.ParseUint(statmFields[1], 10, 64)
			pageSize := uint64(os.Getpagesize())
			stats.MemoryMB = float64(rssPages*pageSize) / (1024 * 1024)
		}
	}

	return stats
}
