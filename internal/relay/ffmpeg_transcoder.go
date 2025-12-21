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

	// SourceURL is the direct URL to the source stream.
	// Used when UseDirectInput is true (e.g., for unsupported audio codecs like E-AC3).
	SourceURL string

	// UseDirectInput enables direct URL input mode.
	// When true, FFmpeg reads directly from SourceURL instead of stdin.
	// This is used when the audio codec can't be demuxed by mediacommon.
	UseDirectInput bool

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

	// Reference to the source ES variant for consumer tracking
	sourceESVariant *ESVariant

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

	// Get source variant to read from.
	// Use the buffer's current source variant key instead of the configured one,
	// because the source variant may have been updated (e.g., audio codec detected
	// after video) between when the transcoder was created and when Start() is called.
	currentSourceKey := t.buffer.SourceVariantKey()
	sourceVariant := t.buffer.GetVariant(currentSourceKey)
	if sourceVariant == nil {
		return fmt.Errorf("source variant %s not found", currentSourceKey)
	}

	// Create or get target variant
	targetVariant, err := t.buffer.CreateVariant(t.config.TargetVariant)
	if err != nil {
		return fmt.Errorf("creating target variant: %w", err)
	}

	// Store source variant reference and register as a consumer to prevent eviction of unread samples
	t.sourceESVariant = sourceVariant
	sourceVariant.RegisterConsumer(t.id)

	// Initialize TS muxer for FFmpeg input with source codec information
	// Get actual codec from the track (not from variant name which may be incomplete at startup)
	videoCodec := sourceVariant.VideoTrack().Codec()
	audioCodec := sourceVariant.AudioTrack().Codec()
	// Fall back to current source variant name if track codec not yet set
	if videoCodec == "" {
		videoCodec = currentSourceKey.VideoCodec()
	}
	if audioCodec == "" {
		audioCodec = currentSourceKey.AudioCodec()
	}

	t.config.Logger.Debug("Initializing transcoder input muxer",
		slog.String("id", t.id),
		slog.String("video_codec", videoCodec),
		slog.String("audio_codec", audioCodec),
		slog.String("source_variant", string(currentSourceKey)))

	t.inputMuxer = NewTSMuxer(&t.inputBuf, TSMuxerConfig{
		Logger:     t.config.Logger,
		VideoCodec: videoCodec,
		AudioCodec: audioCodec,
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

	t.config.Logger.Debug("Starting FFmpeg transcoder",
		slog.String("id", t.id),
		slog.String("source", string(currentSourceKey)),
		slog.String("target", string(t.config.TargetVariant)),
		slog.Bool("direct_input", t.config.UseDirectInput))

	// Start reader goroutine (reads from source variant, writes to FFmpeg stdin)
	// Only needed when NOT using direct input mode
	if !t.config.UseDirectInput {
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.runInputLoop(sourceVariant)
		}()
	}

	// Start output goroutine (reads from FFmpeg stdout, writes to target variant)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.runOutputLoop(targetVariant)
	}()

	return nil
}

// Stop stops the transcoder.
// This method is non-blocking - it signals the process to stop and waits
// with a timeout, forcefully killing if necessary.
func (t *FFmpegTranscoder) Stop() {
	if t.closed.CompareAndSwap(false, true) {
		if t.cancel != nil {
			t.cancel()
		}

		// Unregister as a consumer to allow eviction of our unread samples
		if t.sourceESVariant != nil {
			t.sourceESVariant.UnregisterConsumer(t.id)
		}

		// Close FFmpeg stdin to signal end of input
		if t.stdin != nil {
			t.stdin.Close()
		}

		// Wait for process to exit with timeout - don't block forever
		if t.cmd != nil && t.cmd.Process != nil {
			t.waitWithTimeout(3 * time.Second)
		}

		close(t.closedCh)

		// Wait for goroutines with timeout to avoid blocking
		done := make(chan struct{})
		go func() {
			t.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// Goroutines finished cleanly
		case <-time.After(2 * time.Second):
			t.config.Logger.Warn("FFmpeg transcoder goroutines did not finish in time",
				slog.String("id", t.id))
		}

		// Log any captured stderr on stop for debugging
		stderrLines := t.GetStderrLines()
		if len(stderrLines) > 0 {
			t.config.Logger.Debug("FFmpeg transcoder stopped with stderr output",
				slog.String("id", t.id),
				slog.Int("stderr_lines", len(stderrLines)))
		} else {
			t.config.Logger.Debug("FFmpeg transcoder stopped",
				slog.String("id", t.id))
		}
	}
}

// waitWithTimeout waits for the FFmpeg process to exit, killing it if it
// doesn't exit within the timeout. This prevents blocking forever on hung processes.
func (t *FFmpegTranscoder) waitWithTimeout(timeout time.Duration) {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}

	// Create a channel to signal process exit
	done := make(chan error, 1)
	go func() {
		done <- t.cmd.Wait()
	}()

	// Wait for process to exit or timeout
	select {
	case <-done:
		// Process exited cleanly
		return
	case <-time.After(timeout):
		// Process didn't exit in time, send SIGTERM first
		t.config.Logger.Warn("FFmpeg process did not exit in time, sending SIGTERM",
			slog.String("id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		_ = t.cmd.Process.Signal(os.Interrupt)
	}

	// Give it a short grace period after SIGTERM
	select {
	case <-done:
		return
	case <-time.After(500 * time.Millisecond):
		// Still not dead, force kill
		t.config.Logger.Warn("FFmpeg process did not respond to SIGTERM, killing",
			slog.String("id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		_ = t.cmd.Process.Kill()
	}

	// Final wait with short timeout - if still stuck, log and drain in background
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.config.Logger.Error("FFmpeg process could not be killed, draining in background",
			slog.String("id", t.id),
			slog.Int("pid", t.cmd.Process.Pid))
		// Drain the channel in background to prevent goroutine leak
		// The process was killed, so Wait() will eventually return
		go func() { <-done }()
	}
}

// Stats returns current transcoder statistics.
func (t *FFmpegTranscoder) Stats() TranscoderStats {
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
		// Codec names from target variant (e.g., "h265", "aac")
		VideoCodec: t.config.TargetVariant.VideoCodec(),
		AudioCodec: t.config.TargetVariant.AudioCodec(),
		// Encoder names from config (e.g., "libx265", "h264_nvenc")
		VideoEncoder:  t.config.VideoCodec,
		AudioEncoder:  t.config.AudioCodec,
		HWAccel:       t.config.HWAccel,
		HWAccelDevice: t.config.HWAccelDevice,
		// Encoding speed
		EncodingSpeed: encodingSpeed,
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
	// Codec names (e.g., "h265", "aac") - what the stream IS
	VideoCodec string
	AudioCodec string
	// Encoder names (e.g., "libx265", "h264_nvenc") - what FFmpeg uses
	VideoEncoder  string
	AudioEncoder  string
	HWAccel       string
	HWAccelDevice string
	// Encoding speed (1.0 = realtime, 2.0 = 2x realtime, 0.5 = half realtime)
	EncodingSpeed float64
	// FFmpeg command for debugging
	FFmpegCommand string
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
	// Use Stats() to force progress output (including speed) even when not a terminal
	builder.HideBanner().LogLevel("warning").Stats()

	// Hardware acceleration (if configured)
	if t.config.HWAccel != "" {
		builder.InitHWDevice(t.config.HWAccel, t.config.HWAccelDevice)
		builder.HWAccel(t.config.HWAccel)
		if t.config.HWAccelDevice != "" {
			builder.HWAccelDevice(t.config.HWAccelDevice)
		}
	}

	// Input settings
	if t.config.UseDirectInput && t.config.SourceURL != "" {
		// Direct URL input mode - FFmpeg reads directly from source
		// Used for streams with unsupported audio codecs (e.g., E-AC3)
		builder.InputArgs("-analyzeduration", "2000000")
		builder.InputArgs("-probesize", "2000000")
		builder.Input(t.config.SourceURL)
	} else {
		// Standard mode - reading MPEG-TS from stdin (ES demux/mux)
		builder.InputArgs("-f", "mpegts")
		builder.InputArgs("-analyzeduration", "500000")
		builder.InputArgs("-probesize", "500000")
		builder.Input("pipe:0")
	}

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
		// For AAC encoding, force stereo output to ensure FFmpeg uses an explicit
		// channel configuration (channelConfig 1-7) in the ADTS header.
		// Without this, FFmpeg may use channelConfig=0 (PCE-defined) when
		// transcoding from multichannel sources like E-AC3 5.1.
		// While our demuxer can parse PCE, most downstream players (mpv, browsers,
		// smart TVs) cannot handle channelConfig=0 and fail with errors like
		// "channel element 0.0 is not allocated".
		if t.config.AudioCodec == "aac" {
			builder.AudioChannels(2)
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
		slog.String("id", t.id),
		slog.Bool("direct_input", t.config.UseDirectInput),
		slog.String("command", ffmpegCmd.String()))

	// Create command with context
	t.cmd = exec.CommandContext(t.ctx, ffmpegCmd.Binary, ffmpegCmd.Args...)

	// Set up pipes with cleanup on failure
	var err error

	// Helper to close pipes on error - only closes non-nil pipes
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

	// Only create stdin pipe when not using direct input
	if !t.config.UseDirectInput {
		t.stdin, err = t.cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("creating stdin pipe: %w", err)
		}
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

	// Start stderr reader goroutine to capture FFmpeg errors
	go t.readStderr()

	return nil
}

// readStderr reads FFmpeg stderr and stores recent lines for debugging.
// Also parses encoding speed from FFmpeg progress output.
// Note: FFmpeg uses carriage returns (\r) for progress updates, so we need a custom scanner.
func (t *FFmpegTranscoder) readStderr() {
	scanner := bufio.NewScanner(t.stderr)
	// Custom split function to handle both \r and \n as line delimiters
	// FFmpeg progress output uses \r to update the same line (like a terminal progress bar)
	scanner.Split(scanLinesWithCR)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse encoding speed from FFmpeg output (e.g., "frame= 1234 fps= 30 ... speed=1.2x")
		if speed := t.parseEncodingSpeed(line); speed > 0 {
			t.encodingSpeed.Store(speed)
		}

		// Only store non-progress lines to avoid filling buffer with repeated progress updates
		if !strings.Contains(line, "frame=") {
			t.stderrMu.Lock()
			t.stderrLines = append(t.stderrLines, line)
			// Keep only the last N lines
			if len(t.stderrLines) > t.maxStderrLines {
				t.stderrLines = t.stderrLines[1:]
			}
			t.stderrMu.Unlock()

			// Log FFmpeg errors at warning level (but not progress lines)
			if !strings.Contains(line, "speed=") {
				t.config.Logger.Warn("FFmpeg stderr",
					slog.String("transcoder_id", t.id),
					slog.String("line", line))
			}
		}
	}
}

// scanLinesWithCR is a custom split function for bufio.Scanner that treats both
// carriage return (\r) and newline (\n) as line delimiters. This is needed because
// FFmpeg uses \r for progress updates to overwrite the same line in terminals.
func scanLinesWithCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for either \r or \n
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			// Found a delimiter - return the line up to it
			// Skip any trailing \r or \n (handles \r\n sequences)
			advance = i + 1
			for advance < len(data) && (data[advance] == '\r' || data[advance] == '\n') {
				advance++
			}
			return advance, data[0:i], nil
		}
	}

	// If at EOF and we have data, return it
	if atEOF {
		return len(data), data, nil
	}

	// Request more data
	return 0, nil, nil
}

// parseEncodingSpeed extracts the encoding speed from FFmpeg stderr output.
// FFmpeg outputs lines like: "frame= 1234 fps= 30 ... speed=1.2x" or "speed= 0.95x"
func (t *FFmpegTranscoder) parseEncodingSpeed(line string) float64 {
	// Look for "speed=" pattern
	idx := strings.Index(line, "speed=")
	if idx == -1 {
		return 0
	}

	// Extract the speed value after "speed="
	speedStr := line[idx+6:]
	// Trim leading spaces
	speedStr = strings.TrimLeft(speedStr, " ")

	// Find the end of the number (before 'x' or space)
	endIdx := strings.IndexAny(speedStr, "x \t")
	if endIdx > 0 {
		speedStr = speedStr[:endIdx]
	}

	// Parse the float value
	speed, err := strconv.ParseFloat(strings.TrimSpace(speedStr), 64)
	if err != nil {
		return 0
	}

	return speed
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

	// Update consumer position to allow eviction of samples we've processed
	if t.sourceESVariant != nil && (len(videoSamples) > 0 || len(audioSamples) > 0) {
		t.sourceESVariant.UpdateConsumerPosition(t.id, t.lastVideoSeq, t.lastAudioSeq)
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
				// Check for closed pipe - this is normal when client disconnects or demuxer exits
				if errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "closed pipe") {
					t.config.Logger.Debug("FFmpeg output demuxer closed",
						slog.String("id", t.id))
					return
				}
				// Any other error - log and exit
				t.errorCount.Add(1)
				t.config.Logger.Warn("Error demuxing FFmpeg output, stopping output loop",
					slog.String("id", t.id),
					slog.String("error", err.Error()))
				return
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

// CreateTranscoderFromProfileOptions contains optional parameters for CreateTranscoderFromProfile.
type CreateTranscoderFromProfileOptions struct {
	// SourceURL for direct input mode (bypasses ES demux/mux)
	SourceURL string
	// UseDirectInput enables direct URL input when audio codec can't be demuxed
	UseDirectInput bool
}

// CreateTranscoderFromProfile creates an FFmpegTranscoder configured from an encoding profile.
func CreateTranscoderFromProfile(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	profile *models.EncodingProfile,
	ffmpegBin *ffmpeg.BinaryInfo,
	logger *slog.Logger,
	opts ...CreateTranscoderFromProfileOptions,
) (*FFmpegTranscoder, error) {
	if profile == nil {
		return nil, errors.New("profile is required")
	}

	// Merge optional parameters
	var opt CreateTranscoderFromProfileOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Determine target variant from profile codecs
	targetVariant := NewCodecVariant(
		string(profile.TargetVideoCodec),
		string(profile.TargetAudioCodec),
	)

	// Get encoding parameters from quality preset
	encodingParams := profile.GetEncodingParams()

	config := FFmpegTranscoderConfig{
		FFmpegPath:     ffmpegBin.FFmpegPath,
		SourceVariant:  sourceVariant,
		TargetVariant:  targetVariant,
		VideoCodec:     profile.GetVideoEncoder(),
		AudioCodec:     profile.GetAudioEncoder(),
		VideoBitrate:   profile.GetVideoBitrate(),
		AudioBitrate:   profile.GetAudioBitrate(),
		VideoPreset:    encodingParams.VideoPreset,
		HWAccel:        string(profile.HWAccel),
		HWAccelDevice:  "", // Not configurable in EncodingProfile, uses auto-detection
		Logger:         logger,
		SourceURL:      opt.SourceURL,
		UseDirectInput: opt.UseDirectInput,
	}

	return NewFFmpegTranscoder(id, buffer, config), nil
}


// getCallerInfo returns a string with caller information for debugging
func getCallerInfo() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file, line)
}

// CreateTranscoderFromVariantOptions contains optional parameters for CreateTranscoderFromVariant.
type CreateTranscoderFromVariantOptions struct {
	// SourceURL for direct input mode (bypasses ES demux/mux)
	SourceURL string
	// UseDirectInput enables direct URL input when audio codec can't be demuxed
	UseDirectInput bool
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
	opts ...CreateTranscoderFromVariantOptions,
) (*FFmpegTranscoder, error) {
	// Merge optional parameters
	var opt CreateTranscoderFromVariantOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

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
		FFmpegPath:     ffmpegBin.FFmpegPath,
		SourceVariant:  sourceVariant,
		TargetVariant:  targetVariant,
		VideoCodec:     videoEncoder,
		AudioCodec:     audioEncoder,
		VideoBitrate:   0, // Use FFmpeg defaults
		AudioBitrate:   0, // Use FFmpeg defaults
		VideoPreset:    "medium",
		HWAccel:        "",
		HWAccelDevice:  "",
		Logger:         logger,
		SourceURL:      opt.SourceURL,
		UseDirectInput: opt.UseDirectInput,
	}

	logger.Debug("Creating transcoder from variant",
		slog.String("id", id),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder),
		slog.Bool("direct_input", opt.UseDirectInput))

	return NewFFmpegTranscoder(id, buffer, config), nil
}

// Verify FFmpegTranscoder implements Transcoder interface.
var _ Transcoder = (*FFmpegTranscoder)(nil)
