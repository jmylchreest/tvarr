// Package relay provides streaming relay functionality for tvarr.
package relay

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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// FFmpegTranscoder errors.
var (
	ErrTranscoderClosed     = errors.New("transcoder closed")
	ErrTranscoderNotStarted = errors.New("transcoder not started")
	ErrUnsupportedCodec     = errors.New("unsupported codec")
)

// FFmpegTranscoderConfig configures the FFmpeg transcoder.
type FFmpegTranscoderConfig struct {
	// FFmpegPath is the path to ffmpeg binary.
	FFmpegPath string

	// SourceVariant is the source codec variant to read from.
	SourceVariant CodecVariant

	// TargetVariant is the target codec variant to produce.
	TargetVariant CodecVariant

	// VideoCodec is the target video encoder (e.g., "libx264", "h264_nvenc").
	VideoCodec string

	// AudioCodec is the target audio encoder (e.g., "aac", "libopus").
	AudioCodec string

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

	// Logger for structured logging.
	Logger *slog.Logger
}

// FFmpegTranscoder transcodes ES samples from one codec to another using FFmpeg.
// It reads from a source variant in SharedESBuffer and writes to a target variant.
type FFmpegTranscoder struct {
	id     string
	config FFmpegTranscoderConfig
	buffer *SharedESBuffer

	// FFmpeg process
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Stderr capture for debugging
	stderrLines    []string
	stderrMu       sync.RWMutex
	maxStderrLines int

	// TS muxer for feeding FFmpeg (source ES -> MPEG-TS -> stdin)
	inputMuxer *TSMuxer
	inputBuf   bytes.Buffer

	// TS demuxer for parsing FFmpeg output (stdout -> MPEG-TS -> target ES)
	outputDemuxer *TSDemuxer

	// ES reading state for source variant
	lastVideoSeq uint64
	lastAudioSeq uint64

	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  atomic.Bool
	closed   atomic.Bool
	closedCh chan struct{}

	// Stats
	samplesIn    atomic.Uint64
	samplesOut   atomic.Uint64
	bytesIn      atomic.Uint64
	bytesOut     atomic.Uint64
	errorCount   atomic.Uint64
	startedAt    time.Time
	lastActivity atomic.Value // time.Time
}

// NewFFmpegTranscoder creates a new FFmpeg transcoder.
func NewFFmpegTranscoder(
	id string,
	buffer *SharedESBuffer,
	config FFmpegTranscoderConfig,
) *FFmpegTranscoder {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	t := &FFmpegTranscoder{
		id:             id,
		config:         config,
		buffer:         buffer,
		closedCh:       make(chan struct{}),
		stderrLines:    make([]string, 0, 50),
		maxStderrLines: 50,
	}
	t.lastActivity.Store(time.Now())

	return t
}

// Start begins the transcoding process.
func (t *FFmpegTranscoder) Start(ctx context.Context) error {
	if !t.started.CompareAndSwap(false, true) {
		return errors.New("transcoder already started")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.startedAt = time.Now()

	// Get source variant to read from
	sourceVariant := t.buffer.GetVariant(t.config.SourceVariant)
	if sourceVariant == nil {
		return fmt.Errorf("source variant %s not found", t.config.SourceVariant)
	}

	// Create or get target variant
	targetVariant, err := t.buffer.CreateVariant(t.config.TargetVariant)
	if err != nil {
		return fmt.Errorf("creating target variant: %w", err)
	}

	// Initialize TS muxer for FFmpeg input with source codec information
	// Use the source variant's codec types (e.g., "h264/aac" -> video="h264", audio="aac")
	t.inputMuxer = NewTSMuxer(&t.inputBuf, TSMuxerConfig{
		Logger:     t.config.Logger,
		VideoCodec: t.config.SourceVariant.VideoCodec(),
		AudioCodec: t.config.SourceVariant.AudioCodec(),
	})

	// Initialize TS demuxer for FFmpeg output (writes to target variant)
	t.outputDemuxer = NewTSDemuxer(t.buffer, TSDemuxerConfig{
		Logger:        t.config.Logger,
		TargetVariant: t.config.TargetVariant,
		OnVideoSample: func(pts, dts int64, data []byte, isKeyframe bool) {
			t.samplesOut.Add(1)
			t.bytesOut.Add(uint64(len(data)))
			t.recordActivity()
		},
		OnAudioSample: func(pts int64, data []byte) {
			t.samplesOut.Add(1)
			t.bytesOut.Add(uint64(len(data)))
			t.recordActivity()
		},
	})

	// Start FFmpeg process
	if err := t.startFFmpeg(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	t.config.Logger.Info("Starting FFmpeg transcoder",
		slog.String("id", t.id),
		slog.String("source", string(t.config.SourceVariant)),
		slog.String("target", string(t.config.TargetVariant)))

	// Start reader goroutine (reads from source variant, writes to FFmpeg stdin)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runInputLoop(sourceVariant)
	}()

	// Start output goroutine (reads from FFmpeg stdout, writes to target variant)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runOutputLoop(targetVariant)
	}()

	return nil
}

// Stop stops the transcoder.
func (t *FFmpegTranscoder) Stop() {
	if t.closed.CompareAndSwap(false, true) {
		if t.cancel != nil {
			t.cancel()
		}

		// Close FFmpeg stdin to signal end of input
		if t.stdin != nil {
			t.stdin.Close()
		}

		// Wait for process to exit - ignore error as process may already be dead
		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Wait()
		}

		close(t.closedCh)
		t.wg.Wait()

		// Log any captured stderr on stop for debugging
		stderrLines := t.GetStderrLines()
		if len(stderrLines) > 0 {
			t.config.Logger.Info("FFmpeg transcoder stopped with stderr output",
				slog.String("id", t.id),
				slog.Int("stderr_lines", len(stderrLines)))
		} else {
			t.config.Logger.Info("FFmpeg transcoder stopped",
				slog.String("id", t.id))
		}
	}
}

// Stats returns current transcoder statistics.
func (t *FFmpegTranscoder) Stats() TranscoderStats {
	lastActivity, _ := t.lastActivity.Load().(time.Time)
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
		// Codec names from target variant (e.g., "h265", "aac")
		VideoCodec: t.config.TargetVariant.VideoCodec(),
		AudioCodec: t.config.TargetVariant.AudioCodec(),
		// Encoder names from config (e.g., "libx265", "h264_nvenc")
		VideoEncoder:  t.config.VideoCodec,
		AudioEncoder:  t.config.AudioCodec,
		HWAccel:       t.config.HWAccel,
		HWAccelDevice: t.config.HWAccelDevice,
	}
}

// TranscoderStats contains statistics about the transcoder.
type TranscoderStats struct {
	ID            string
	SourceVariant CodecVariant
	TargetVariant CodecVariant
	StartedAt     time.Time
	LastActivity  time.Time
	SamplesIn     uint64
	SamplesOut    uint64
	BytesIn       uint64
	BytesOut      uint64
	Errors        uint64
	// Codec names (e.g., "h264", "h265", "aac") - what the stream IS
	VideoCodec string
	AudioCodec string
	// Encoder names (e.g., "libx264", "h264_nvenc", "libopus") - what FFmpeg uses
	VideoEncoder  string
	AudioEncoder  string
	HWAccel       string
	HWAccelDevice string
}

// ProcessStats returns CPU and memory stats for the FFmpeg process.
// Returns nil if the process is not running.
func (t *FFmpegTranscoder) ProcessStats() *TranscoderProcessStats {
	if t.cmd == nil || t.cmd.Process == nil {
		return nil
	}

	pid := t.cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	stats := &TranscoderProcessStats{
		PID: pid,
	}

	// Read process stats from /proc on Linux
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := os.ReadFile(statPath)
	if err == nil {
		fields := strings.Fields(string(statData))
		if len(fields) >= 14 {
			// Field 14 is utime (user mode ticks), field 15 is stime (kernel mode ticks)
			utime, _ := strconv.ParseUint(fields[13], 10, 64)
			stime, _ := strconv.ParseUint(fields[14], 10, 64)
			totalTicks := utime + stime

			// Get clock ticks per second
			clkTck := uint64(100) // Default on most Linux systems

			// Calculate CPU percentage based on process uptime
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
			// RSS is the second field, in pages
			rssPages, _ := strconv.ParseUint(statmFields[1], 10, 64)
			pageSize := uint64(os.Getpagesize())
			stats.MemoryRSSMB = float64(rssPages*pageSize) / (1024 * 1024)
		}
	}

	// Calculate memory percentage
	meminfoPath := "/proc/meminfo"
	meminfoData, err := os.ReadFile(meminfoPath)
	if err == nil {
		for _, line := range strings.Split(string(meminfoData), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					totalKB, _ := strconv.ParseUint(fields[1], 10, 64)
					totalMB := float64(totalKB) / 1024.0
					if totalMB > 0 {
						stats.MemoryPercent = (stats.MemoryRSSMB / totalMB) * 100.0
					}
				}
				break
			}
		}
	}

	return stats
}

// TranscoderProcessStats contains process-level stats for the transcoder.
type TranscoderProcessStats struct {
	PID           int
	CPUPercent    float64
	MemoryRSSMB   float64
	MemoryPercent float64
}

// startFFmpeg starts the FFmpeg process.
func (t *FFmpegTranscoder) startFFmpeg() error {
	ffmpegPath := t.config.FFmpegPath
	if ffmpegPath == "" {
		// Try to find ffmpeg
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			return fmt.Errorf("ffmpeg not found: %w", err)
		}
		ffmpegPath = path
	}

	builder := ffmpeg.NewCommandBuilder(ffmpegPath)

	// Global flags
	builder.HideBanner().LogLevel("warning")

	// Hardware acceleration (if configured)
	if t.config.HWAccel != "" {
		builder.InitHWDevice(t.config.HWAccel, t.config.HWAccelDevice)
		builder.HWAccel(t.config.HWAccel)
		if t.config.HWAccelDevice != "" {
			builder.HWAccelDevice(t.config.HWAccelDevice)
		}
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
	if t.config.VideoCodec != "" {
		builder.VideoCodec(t.config.VideoCodec)

		// Hardware upload filter if using HW encoder
		if t.config.HWAccel != "" && ffmpeg.IsHardwareEncoder(t.config.VideoCodec) {
			builder.HWUploadFilter(t.config.HWAccel)
		}

		if t.config.VideoBitrate > 0 {
			builder.VideoBitrate(fmt.Sprintf("%dk", t.config.VideoBitrate))
		}
		if t.config.VideoPreset != "" {
			builder.VideoPreset(t.config.VideoPreset)
		}
	} else {
		builder.VideoCodec("copy")
	}

	// Audio codec
	if t.config.AudioCodec != "" {
		builder.AudioCodec(t.config.AudioCodec)
		if t.config.AudioBitrate > 0 {
			builder.AudioBitrate(fmt.Sprintf("%dk", t.config.AudioBitrate))
		}
	} else {
		builder.AudioCodec("copy")
	}

	// Output MPEG-TS to stdout
	builder.MpegtsArgs()
	builder.FlushPackets()
	builder.MuxDelay("0")
	builder.Output("pipe:1")

	// Build command
	ffmpegCmd := builder.Build()
	t.config.Logger.Debug("FFmpeg transcoder command",
		slog.String("command", ffmpegCmd.String()))

	// Create command with context
	t.cmd = exec.CommandContext(t.ctx, ffmpegCmd.Binary, ffmpegCmd.Args...)

	// Set up pipes
	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	t.stderr, err = t.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	// Start process
	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	// Start stderr reader goroutine to capture FFmpeg errors
	go t.readStderr()

	return nil
}

// readStderr reads FFmpeg stderr and stores recent lines for debugging.
func (t *FFmpegTranscoder) readStderr() {
	scanner := bufio.NewScanner(t.stderr)
	for scanner.Scan() {
		line := scanner.Text()

		t.stderrMu.Lock()
		t.stderrLines = append(t.stderrLines, line)
		// Keep only the last N lines
		if len(t.stderrLines) > t.maxStderrLines {
			t.stderrLines = t.stderrLines[1:]
		}
		t.stderrMu.Unlock()

		// Log FFmpeg errors at warning level
		if line != "" {
			t.config.Logger.Warn("FFmpeg stderr",
				slog.String("transcoder_id", t.id),
				slog.String("line", line))
		}
	}
}

// GetStderrLines returns the recent stderr lines from FFmpeg.
func (t *FFmpegTranscoder) GetStderrLines() []string {
	t.stderrMu.RLock()
	defer t.stderrMu.RUnlock()

	lines := make([]string, len(t.stderrLines))
	copy(lines, t.stderrLines)
	return lines
}

// runInputLoop reads ES samples from source variant and feeds them to FFmpeg.
func (t *FFmpegTranscoder) runInputLoop(source *ESVariant) {
	videoTrack := source.VideoTrack()
	audioTrack := source.AudioTrack()

	// Wait for initial keyframe
	t.config.Logger.Debug("Waiting for initial keyframe from source")
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
			t.processSourceSamples(videoTrack, audioTrack)
		}
	}
}

// processSourceSamples reads samples from source and muxes them for FFmpeg.
func (t *FFmpegTranscoder) processSourceSamples(videoTrack, audioTrack *ESTrack) {
	// Read video samples
	videoSamples := videoTrack.ReadFrom(t.lastVideoSeq, 100)
	for _, sample := range videoSamples {
		// Ignore muxer errors - continue on failures
		_ = t.inputMuxer.WriteVideo(sample.PTS, sample.DTS, sample.Data, sample.IsKeyframe)
		t.lastVideoSeq = sample.Sequence
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Read audio samples
	audioSamples := audioTrack.ReadFrom(t.lastAudioSeq, 200)
	for _, sample := range audioSamples {
		// Ignore muxer errors - continue on failures
		_ = t.inputMuxer.WriteAudio(sample.PTS, sample.Data)
		t.lastAudioSeq = sample.Sequence
		t.samplesIn.Add(1)
		t.bytesIn.Add(uint64(len(sample.Data)))
	}

	// Flush muxer and write to FFmpeg stdin
	t.inputMuxer.Flush()
	if t.inputBuf.Len() > 0 {
		data := t.inputBuf.Bytes()
		_, err := t.stdin.Write(data)
		if err != nil {
			if !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, context.Canceled) {
				t.errorCount.Add(1)
				// Include recent stderr lines for debugging
				stderrLines := t.GetStderrLines()
				t.config.Logger.Warn("Error writing to FFmpeg stdin",
					slog.String("error", err.Error()),
					slog.Int("stderr_lines", len(stderrLines)),
					slog.Any("recent_stderr", stderrLines))
			}
		}
		t.inputBuf.Reset()
		t.recordActivity()
	}
}

// runOutputLoop reads FFmpeg output and demuxes it to target variant.
func (t *FFmpegTranscoder) runOutputLoop(target *ESVariant) {
	buf := make([]byte, TSPacketSize*100) // Read in chunks

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
			t.config.Logger.Warn("Error reading FFmpeg output",
				slog.String("error", err.Error()))
			return
		}

		if n > 0 {
			if err := t.outputDemuxer.Write(buf[:n]); err != nil {
				t.errorCount.Add(1)
				t.config.Logger.Warn("Error demuxing FFmpeg output",
					slog.String("error", err.Error()))
			}
			t.recordActivity()
		}
	}
}

// recordActivity updates last activity time.
func (t *FFmpegTranscoder) recordActivity() {
	t.lastActivity.Store(time.Now())
}

// IsClosed returns true if the transcoder is closed.
func (t *FFmpegTranscoder) IsClosed() bool {
	return t.closed.Load()
}

// ClosedChan returns a channel that is closed when transcoder stops.
func (t *FFmpegTranscoder) ClosedChan() <-chan struct{} {
	return t.closedCh
}

// CreateTranscoderFromProfile creates an FFmpegTranscoder configured from a relay profile.
func CreateTranscoderFromProfile(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	profile *models.RelayProfile,
	ffmpegBin *ffmpeg.BinaryInfo,
	logger *slog.Logger,
) (*FFmpegTranscoder, error) {
	if profile == nil {
		return nil, errors.New("profile is required")
	}

	// Determine target variant from profile codecs
	targetVariant := MakeCodecVariant(
		string(profile.VideoCodec),
		string(profile.AudioCodec),
	)

	config := FFmpegTranscoderConfig{
		FFmpegPath:    ffmpegBin.FFmpegPath,
		SourceVariant: sourceVariant,
		TargetVariant: targetVariant,
		VideoCodec:    profile.VideoCodec.GetFFmpegEncoder(profile.HWAccel),
		AudioCodec:    profile.AudioCodec.GetFFmpegEncoder(),
		VideoBitrate:  profile.VideoBitrate,
		AudioBitrate:  profile.AudioBitrate,
		VideoPreset:   profile.VideoPreset,
		HWAccel:       string(profile.HWAccel),
		HWAccelDevice: profile.HWAccelDevice,
		Logger:        logger,
	}

	return NewFFmpegTranscoder(id, buffer, config), nil
}

// MakeCodecVariant creates a CodecVariant from video and audio codec names.
// Codec names should be like "h264", "h265", "aac" - NOT encoder names like "libx265".
func MakeCodecVariant(videoCodec, audioCodec string) CodecVariant {
	// Warn if encoder names are passed instead of codec names - this indicates a bug
	if IsEncoderName(videoCodec) {
		slog.Warn("MakeCodecVariant called with encoder name instead of codec name",
			slog.String("video_codec", videoCodec),
			slog.String("expected", "codec name like h264, h265, vp9"),
			slog.String("stack", getCallerInfo()))
	}
	if IsEncoderName(audioCodec) {
		slog.Warn("MakeCodecVariant called with encoder name instead of codec name",
			slog.String("audio_codec", audioCodec),
			slog.String("expected", "codec name like aac, opus, mp3"),
			slog.String("stack", getCallerInfo()))
	}

	// Handle empty/copy values
	if videoCodec == "" || videoCodec == "copy" {
		videoCodec = "copy"
	}
	if audioCodec == "" || audioCodec == "copy" {
		audioCodec = "copy"
	}

	return CodecVariant(fmt.Sprintf("%s/%s", videoCodec, audioCodec))
}

// getCallerInfo returns a string with caller information for debugging
func getCallerInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// CreateTranscoderFromVariant creates an FFmpeg transcoder directly from source and target variants.
// This is used when we don't have a profile but know the target codec variant.
// It uses default settings for bitrate, preset, etc.
func CreateTranscoderFromVariant(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	targetVariant CodecVariant,
	ffmpegBin *ffmpeg.BinaryInfo,
	logger *slog.Logger,
) (*FFmpegTranscoder, error) {
	// Parse video and audio codecs from target variant
	videoCodecStr := targetVariant.VideoCodec()
	audioCodecStr := targetVariant.AudioCodec()

	// Map variant codec names to FFmpeg encoders
	var videoEncoder, audioEncoder string

	switch videoCodecStr {
	case "h264", "avc":
		videoEncoder = "libx264"
	case "h265", "hevc":
		videoEncoder = "libx265"
	case "vp9":
		videoEncoder = "libvpx-vp9"
	case "av1":
		videoEncoder = "libaom-av1"
	case "copy", "":
		videoEncoder = "copy"
	default:
		// Try to use the codec name directly as encoder
		videoEncoder = videoCodecStr
	}

	switch audioCodecStr {
	case "aac":
		audioEncoder = "aac"
	case "ac3":
		audioEncoder = "ac3"
	case "opus":
		audioEncoder = "libopus"
	case "mp3":
		audioEncoder = "libmp3lame"
	case "copy", "":
		audioEncoder = "copy"
	default:
		// Try to use the codec name directly as encoder
		audioEncoder = audioCodecStr
	}

	config := FFmpegTranscoderConfig{
		FFmpegPath:    ffmpegBin.FFmpegPath,
		SourceVariant: sourceVariant,
		TargetVariant: targetVariant,
		VideoCodec:    videoEncoder,
		AudioCodec:    audioEncoder,
		VideoBitrate:  0, // Use FFmpeg defaults
		AudioBitrate:  0, // Use FFmpeg defaults
		VideoPreset:   "medium",
		HWAccel:       "",
		HWAccelDevice: "",
		Logger:        logger,
	}

	logger.Info("Creating transcoder from variant",
		slog.String("id", id),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder))

	return NewFFmpegTranscoder(id, buffer, config), nil
}
