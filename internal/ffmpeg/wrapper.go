package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Command represents an FFmpeg command to execute.
type Command struct {
	Binary    string
	Args      []string
	Input     string
	Output    string
	LogLevel  string
	Overwrite bool

	// Process control
	cmd     *exec.Cmd
	started time.Time
	mu      sync.RWMutex

	// Progress tracking
	doneCh chan struct{}

	// Process monitoring
	monitor *ProcessMonitor

	// Stderr logging
	stderrLogPath string       // Path to write stderr log (empty = no file logging)
	stderrLines   []string     // Recent stderr lines for debugging
	stderrMu      sync.RWMutex // Protects stderrLines
}

// Progress represents FFmpeg progress information.
type Progress struct {
	Frame      int64         `json:"frame"`
	FPS        float64       `json:"fps"`
	Bitrate    string        `json:"bitrate"`
	TotalSize  int64         `json:"total_size"`
	Time       time.Duration `json:"time"`
	Speed      float64       `json:"speed"`
	DupFrames  int64         `json:"dup_frames"`
	DropFrames int64         `json:"drop_frames"`
}

// RetryConfig configures retry behavior for FFmpeg process startup.
type RetryConfig struct {
	MaxAttempts     int           // Maximum number of retry attempts (default: 3)
	InitialDelay    time.Duration // Initial delay before first retry (default: 500ms)
	MaxDelay        time.Duration // Maximum delay between retries (default: 5s)
	BackoffFactor   float64       // Multiplier for exponential backoff (default: 2.0)
	MinRunTime      time.Duration // Minimum run time to consider success (default: 5s)
	RetryOnAnyError bool          // Retry on any error, not just startup failures
}

// DefaultRetryConfig returns sensible defaults for FFmpeg retry.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    500 * time.Millisecond,
		MaxDelay:        5 * time.Second,
		BackoffFactor:   2.0,
		MinRunTime:      5 * time.Second,
		RetryOnAnyError: false,
	}
}

// CommandBuilder builds FFmpeg commands with a fluent API.
type CommandBuilder struct {
	binary        string
	globalArgs    []string
	inputArgs     []string
	input         string
	filterArgs    []string
	outputArgs    []string
	output        string
	logLevel      string
	overwrite     bool
	stderrLogPath string
}

// NewCommandBuilder creates a new FFmpeg command builder.
func NewCommandBuilder(ffmpegPath string) *CommandBuilder {
	return &CommandBuilder{
		binary:   ffmpegPath,
		logLevel: "error",
	}
}

// LogLevel sets the FFmpeg log level.
func (b *CommandBuilder) LogLevel(level string) *CommandBuilder {
	b.logLevel = level
	return b
}

// HideBanner hides the FFmpeg banner.
func (b *CommandBuilder) HideBanner() *CommandBuilder {
	b.globalArgs = append(b.globalArgs, "-hide_banner")
	return b
}

// Overwrite enables output file overwriting.
func (b *CommandBuilder) Overwrite() *CommandBuilder {
	b.overwrite = true
	return b
}

// Stats enables progress stats output.
func (b *CommandBuilder) Stats() *CommandBuilder {
	b.globalArgs = append(b.globalArgs, "-stats")
	return b
}

// InitHWDevice initializes a hardware device for acceleration.
// This should be called before HWAccel for proper device setup.
// Example: InitHWDevice("cuda", "0")
func (b *CommandBuilder) InitHWDevice(hwType string, device string) *CommandBuilder {
	// Skip if empty, "none", or "auto" - FFmpeg doesn't understand "auto",
	// it needs specific types like vaapi, cuda, qsv, etc.
	if hwType == "" || hwType == "none" || hwType == "auto" {
		return b
	}
	if device != "" {
		b.globalArgs = append(b.globalArgs, "-init_hw_device", fmt.Sprintf("%s=hw:%s", hwType, device))
	} else {
		b.globalArgs = append(b.globalArgs, "-init_hw_device", fmt.Sprintf("%s=hw", hwType))
	}
	return b
}

// HWAccel sets the hardware acceleration method.
// Skips "auto" since FFmpeg doesn't understand it - it needs specific types.
func (b *CommandBuilder) HWAccel(accel string) *CommandBuilder {
	if accel != "" && accel != "none" && accel != "auto" {
		b.inputArgs = append(b.inputArgs, "-hwaccel", accel)
	}
	return b
}

// HWAccelDevice sets the hardware acceleration device.
func (b *CommandBuilder) HWAccelDevice(device string) *CommandBuilder {
	if device != "" {
		b.inputArgs = append(b.inputArgs, "-hwaccel_device", device)
	}
	return b
}

// HWAccelOutputFormat sets the hardware acceleration output format.
func (b *CommandBuilder) HWAccelOutputFormat(format string) *CommandBuilder {
	if format != "" {
		b.inputArgs = append(b.inputArgs, "-hwaccel_output_format", format)
	}
	return b
}

// HWUploadFilter adds the appropriate hardware upload filter for the given hwaccel type.
// This is needed when transcoding with hardware acceleration to upload frames to GPU.
// Note: Requires -filter_hw_device to be set via GlobalArgs to specify the target device.
func (b *CommandBuilder) HWUploadFilter(hwType string) *CommandBuilder {
	if hwType == "" || hwType == "none" || hwType == "auto" {
		return b
	}

	var filter string
	switch hwType {
	case "vaapi":
		filter = "format=nv12,hwupload"
	case "cuda", "nvenc":
		filter = "format=nv12,hwupload_cuda"
	case "qsv":
		filter = "format=nv12,hwupload=extra_hw_frames=64"
	case "videotoolbox":
		filter = "format=nv12,hwupload"
	default:
		filter = "format=nv12,hwupload"
	}

	b.filterArgs = append(b.filterArgs, filter)
	return b
}

// Input sets the input source.
func (b *CommandBuilder) Input(input string) *CommandBuilder {
	b.input = input
	return b
}

// InputArgs adds arbitrary input arguments.
func (b *CommandBuilder) InputArgs(args ...string) *CommandBuilder {
	b.inputArgs = append(b.inputArgs, args...)
	return b
}

// Reconnect enables automatic reconnection for network streams.
func (b *CommandBuilder) Reconnect() *CommandBuilder {
	b.inputArgs = append(b.inputArgs,
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5")
	return b
}

// VideoCodec sets the video codec.
func (b *CommandBuilder) VideoCodec(codec string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-c:v", codec)
	return b
}

// AudioCodec sets the audio codec.
func (b *CommandBuilder) AudioCodec(codec string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-c:a", codec)
	return b
}

// VideoBitrate sets the video bitrate.
func (b *CommandBuilder) VideoBitrate(bitrate string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-b:v", bitrate)
	return b
}

// AudioBitrate sets the audio bitrate.
func (b *CommandBuilder) AudioBitrate(bitrate string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-b:a", bitrate)
	return b
}

// VideoPreset sets the encoding preset.
func (b *CommandBuilder) VideoPreset(preset string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-preset", preset)
	return b
}

// VideoFilter adds a video filter.
func (b *CommandBuilder) VideoFilter(filter string) *CommandBuilder {
	b.filterArgs = append(b.filterArgs, filter)
	return b
}

// AudioChannels sets the number of audio channels.
func (b *CommandBuilder) AudioChannels(channels int) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-ac", strconv.Itoa(channels))
	return b
}

// OutputArgs adds arbitrary output arguments.
func (b *CommandBuilder) OutputArgs(args ...string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, args...)
	return b
}

// MpegtsArgs adds common MPEG-TS output arguments.
// Based on m3u-proxy's proven configuration for reliable streaming.
func (b *CommandBuilder) MpegtsArgs() *CommandBuilder {
	b.outputArgs = append(b.outputArgs,
		"-f", "mpegts",
		"-mpegts_copyts", "1", // Preserve original timestamps
		"-avoid_negative_ts", "disabled", // Don't shift timestamps (critical!)
		"-mpegts_start_pid", "256", // Standard program start PID
		"-mpegts_pmt_start_pid", "4096", // Program Map Table PID
	)
	return b
}

// StderrLogPath sets a file path to write FFmpeg stderr output for debugging.
func (b *CommandBuilder) StderrLogPath(path string) *CommandBuilder {
	b.stderrLogPath = path
	return b
}

// ApplyCustomInputOptions parses and applies custom input options string.
// Options are inserted after existing input args but before the -i input.
func (b *CommandBuilder) ApplyCustomInputOptions(opts string) *CommandBuilder {
	if opts == "" {
		return b
	}
	flags := parseOptionsString(opts)
	b.inputArgs = append(b.inputArgs, flags...)
	return b
}

// ApplyCustomOutputOptions parses and applies custom output options string.
// Options are appended after existing output args.
func (b *CommandBuilder) ApplyCustomOutputOptions(opts string) *CommandBuilder {
	if opts == "" {
		return b
	}
	flags := parseOptionsString(opts)
	b.outputArgs = append(b.outputArgs, flags...)
	return b
}

// FlushPackets enables immediate packet flushing for low latency.
func (b *CommandBuilder) FlushPackets() *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-flush_packets", "1")
	return b
}

// MuxDelay sets the muxer delay for live streaming.
func (b *CommandBuilder) MuxDelay(delay string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-muxdelay", delay)
	return b
}

// parseOptionsString splits an options string respecting quotes.
func parseOptionsString(s string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		if r == '"' || r == '\'' {
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
			continue
		}

		if r == ' ' && !inQuote {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// HLSArgs adds common HLS output arguments.
func (b *CommandBuilder) HLSArgs(segmentTime int, playlistSize int) *CommandBuilder {
	b.outputArgs = append(b.outputArgs,
		"-f", "hls",
		"-hls_time", strconv.Itoa(segmentTime),
		"-hls_list_size", strconv.Itoa(playlistSize),
		"-hls_flags", "delete_segments")
	return b
}

// FMP4Args adds fragmented MP4 (CMAF) output arguments for live streaming.
// This configures FFmpeg to output fMP4 segments suitable for HLS v7+ and DASH.
// fragDuration is the target fragment duration in seconds (typically matches segment duration).
func (b *CommandBuilder) FMP4Args(fragDuration float64) *CommandBuilder {
	// -f mp4: Use MP4 muxer
	// -movflags: Control MP4 fragmentation behavior
	//   - empty_moov: Create an empty moov box and move atoms to moof/mdat pairs
	//                 This is essential for streaming as it allows playback to start
	//                 before the entire file is written
	//   - default_base_moof: Use moof as base for data offsets (more compatible)
	//   - skip_trailer: Don't write the trailer (not needed for live streaming)
	//   - cmaf: Enable CMAF compliance mode for maximum compatibility
	//
	// NOTE: We intentionally DO NOT use frag_keyframe here because it creates
	// tiny fragments at every keyframe (~30ms for 30fps with short GOP).
	// Instead, we rely on frag_duration to create segments at the target duration.
	// This trades keyframe alignment for predictable segment duration.
	// For live streaming where users don't seek much, this is acceptable.
	b.outputArgs = append(b.outputArgs,
		"-f", "mp4",
		"-movflags", "empty_moov+default_base_moof+skip_trailer+cmaf",
	)

	// Set fragment duration - this is now the primary fragmentation control
	// Without frag_keyframe, fragments are created purely based on this duration
	if fragDuration > 0 {
		// frag_duration is in microseconds
		fragDurationUs := int(fragDuration * 1000000)
		b.outputArgs = append(b.outputArgs, "-frag_duration", strconv.Itoa(fragDurationUs))
	}

	return b
}

// FMP4ArgsWithMinFrag adds fMP4 output arguments with minimum fragment duration.
// This variant also sets min_frag_duration to prevent very short fragments.
func (b *CommandBuilder) FMP4ArgsWithMinFrag(fragDuration, minFragDuration float64) *CommandBuilder {
	b.FMP4Args(fragDuration)

	if minFragDuration > 0 {
		// min_frag_duration is in microseconds
		minFragDurationUs := int(minFragDuration * 1000000)
		b.outputArgs = append(b.outputArgs, "-min_frag_duration", strconv.Itoa(minFragDurationUs))
	}

	return b
}

// Output sets the output destination.
func (b *CommandBuilder) Output(output string) *CommandBuilder {
	b.output = output
	return b
}

// Build builds the command.
func (b *CommandBuilder) Build() *Command {
	var args []string

	// Global args (loglevel, banner, etc.)
	args = append(args, "-loglevel", b.logLevel)
	args = append(args, b.globalArgs...)

	// Overwrite
	if b.overwrite {
		args = append(args, "-y")
	}

	// Input args
	args = append(args, b.inputArgs...)
	args = append(args, "-i", b.input)

	// Video filter complex
	if len(b.filterArgs) > 0 {
		args = append(args, "-vf", strings.Join(b.filterArgs, ","))
	}

	// Output args
	args = append(args, b.outputArgs...)

	// Output
	args = append(args, b.output)

	return &Command{
		Binary:        b.binary,
		Args:          args,
		Input:         b.input,
		Output:        b.output,
		LogLevel:      b.logLevel,
		Overwrite:     b.overwrite,
		doneCh:        make(chan struct{}),
		stderrLogPath: b.stderrLogPath,
		stderrLines:   make([]string, 0, 100), // Pre-allocate for recent lines
	}
}

// String returns the command as a string.
func (c *Command) String() string {
	return c.Binary + " " + strings.Join(c.Args, " ")
}

// Run executes the command and waits for completion.
func (c *Command) Run(ctx context.Context) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.started = time.Now()
	c.mu.Unlock()

	return c.cmd.Run()
}

// Start starts the command without waiting.
func (c *Command) Start(ctx context.Context) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.started = time.Now()
	c.mu.Unlock()

	return c.cmd.Start()
}

// Wait waits for the command to complete.
func (c *Command) Wait() error {
	c.mu.RLock()
	cmd := c.cmd
	c.mu.RUnlock()

	if cmd == nil {
		return fmt.Errorf("command not started")
	}

	return cmd.Wait()
}

// Kill terminates the FFmpeg process.
func (c *Command) Kill() error {
	c.mu.RLock()
	cmd := c.cmd
	c.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	return cmd.Process.Kill()
}

// Signal sends a signal to the FFmpeg process.
func (c *Command) Signal(sig os.Signal) error {
	c.mu.RLock()
	cmd := c.cmd
	c.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	return cmd.Process.Signal(sig)
}

// IsRunning returns true if the command is running.
func (c *Command) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}

	// Check if process is still running
	return c.cmd.ProcessState == nil
}

// Duration returns how long the command has been running.
func (c *Command) Duration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.started.IsZero() {
		return 0
	}

	return time.Since(c.started)
}

// Stderr returns a pipe to stderr.
func (c *Command) Stderr() (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil {
		return nil, fmt.Errorf("command not initialized")
	}

	return c.cmd.StderrPipe()
}

// Stdout returns a pipe to stdout.
func (c *Command) Stdout() (io.ReadCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil {
		return nil, fmt.Errorf("command not initialized")
	}

	return c.cmd.StdoutPipe()
}

// RunWithProgress runs the command and reports progress.
func (c *Command) RunWithProgress(ctx context.Context, progressCh chan<- Progress) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.started = time.Now()

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("getting stderr pipe: %w", err)
	}
	c.mu.Unlock()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	// Parse progress from stderr
	go c.parseProgress(stderr, progressCh)

	return c.cmd.Wait()
}

// parseProgress parses FFmpeg progress output from stderr.
func (c *Command) parseProgress(r io.Reader, progressCh chan<- Progress) {
	scanner := bufio.NewScanner(r)
	progress := Progress{}

	// Regex patterns for parsing FFmpeg output
	frameRe := regexp.MustCompile(`frame=\s*(\d+)`)
	fpsRe := regexp.MustCompile(`fps=\s*([\d.]+)`)
	bitrateRe := regexp.MustCompile(`bitrate=\s*([\d.]+\s*\w+/s)`)
	sizeRe := regexp.MustCompile(`size=\s*(\d+)`)
	timeRe := regexp.MustCompile(`time=(\d+):(\d+):(\d+).(\d+)`)
	speedRe := regexp.MustCompile(`speed=\s*([\d.]+)x`)
	dupRe := regexp.MustCompile(`dup=\s*(\d+)`)
	dropRe := regexp.MustCompile(`drop=\s*(\d+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := frameRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.Frame, _ = strconv.ParseInt(matches[1], 10, 64)
		}

		if matches := fpsRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.FPS, _ = strconv.ParseFloat(matches[1], 64)
		}

		if matches := bitrateRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.Bitrate = matches[1]
		}

		if matches := sizeRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.TotalSize, _ = strconv.ParseInt(matches[1], 10, 64)
		}

		if matches := timeRe.FindStringSubmatch(line); len(matches) > 4 {
			hours, _ := strconv.Atoi(matches[1])
			mins, _ := strconv.Atoi(matches[2])
			secs, _ := strconv.Atoi(matches[3])
			ms, _ := strconv.Atoi(matches[4])
			progress.Time = time.Duration(hours)*time.Hour +
				time.Duration(mins)*time.Minute +
				time.Duration(secs)*time.Second +
				time.Duration(ms)*time.Millisecond*10
		}

		if matches := speedRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.Speed, _ = strconv.ParseFloat(matches[1], 64)
		}

		if matches := dupRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.DupFrames, _ = strconv.ParseInt(matches[1], 10, 64)
		}

		if matches := dropRe.FindStringSubmatch(line); len(matches) > 1 {
			progress.DropFrames, _ = strconv.ParseInt(matches[1], 10, 64)
		}

		// Send progress update
		select {
		case progressCh <- progress:
		default:
			// Don't block if channel is full
		}
	}
}

// StreamToWriter runs FFmpeg and writes output to a writer.
// It monitors process resource usage (CPU, memory) and tracks bytes written.
// Stderr is captured for logging and debugging.
func (c *Command) StreamToWriter(ctx context.Context, w io.Writer) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.started = time.Now()

	// Get stdout pipe before starting (required by exec.Cmd)
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("getting stdout pipe: %w", err)
	}

	// Get stderr pipe for logging
	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("getting stderr pipe: %w", err)
	}
	c.mu.Unlock()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	// Start process monitoring after we have a PID
	c.mu.Lock()
	c.monitor = NewProcessMonitor(c.cmd.Process.Pid)
	c.monitor.Start()
	stderrLogPath := c.stderrLogPath
	c.mu.Unlock()

	// Start stderr capture goroutine
	stderrDone := make(chan struct{})
	go c.captureStderr(stderr, stderrLogPath, stderrDone)

	// Create counting writer to track bandwidth
	countingWriter := NewCountingWriter(w, c.monitor)

	// Copy in a goroutine so we can wait for process
	copyDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(countingWriter, stdout)
		copyDone <- err
	}()

	// Wait for process to complete
	waitErr := c.cmd.Wait()

	// Wait for stderr capture to finish
	<-stderrDone

	// Stop monitoring
	c.stopMonitor()

	// Check copy error
	select {
	case copyErr := <-copyDone:
		if copyErr != nil && waitErr == nil {
			return fmt.Errorf("copying output: %w", copyErr)
		}
	default:
	}

	return waitErr
}

// StreamToWriterWithRetry runs FFmpeg with automatic retry on startup failures.
// It will retry up to MaxAttempts times if FFmpeg fails quickly (within MinRunTime).
// This is useful for handling transient network issues or temporary resource exhaustion.
func (c *Command) StreamToWriterWithRetry(ctx context.Context, w io.Writer, cfg RetryConfig) error {
	// Apply defaults for zero values
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 500 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 5 * time.Second
	}
	if cfg.BackoffFactor <= 0 {
		cfg.BackoffFactor = 2.0
	}
	if cfg.MinRunTime <= 0 {
		cfg.MinRunTime = 5 * time.Second
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		startTime := time.Now()

		// For the first attempt, use the original command so the caller can access
		// the monitor for stats while FFmpeg is running. Only clone for retries
		// since exec.Cmd can only be started once.
		var attemptCmd *Command
		if attempt == 1 {
			attemptCmd = c
		} else {
			attemptCmd = c.cloneForRetry()
		}

		// Run the attempt
		err := attemptCmd.StreamToWriter(ctx, w)
		runDuration := time.Since(startTime)

		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Success case: no error
		if err == nil {
			return nil
		}

		lastErr = err

		// Determine if we should retry
		shouldRetry := false
		if cfg.RetryOnAnyError {
			shouldRetry = true
		} else {
			// Only retry if FFmpeg failed quickly (startup failure)
			shouldRetry = runDuration < cfg.MinRunTime
		}

		if !shouldRetry {
			// Process ran for a while before failing - not a startup issue
			return err
		}

		// Don't retry on last attempt
		if attempt >= cfg.MaxAttempts {
			break
		}

		// Log retry attempt
		stderrLines := attemptCmd.GetStderrLines()
		lastStderr := ""
		if len(stderrLines) > 0 {
			lastStderr = stderrLines[len(stderrLines)-1]
		}

		fmt.Fprintf(os.Stderr, "FFmpeg attempt %d/%d failed after %v: %v (last stderr: %s), retrying in %v\n",
			attempt, cfg.MaxAttempts, runDuration.Round(time.Millisecond), err, lastStderr, delay)

		// Wait before retry (with context awareness)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		// Increase delay with exponential backoff
		delay = min(time.Duration(float64(delay)*cfg.BackoffFactor), cfg.MaxDelay)
	}

	return fmt.Errorf("ffmpeg failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// cloneForRetry creates a new Command with the same configuration for retry.
// This is necessary because exec.Cmd can only be started once.
func (c *Command) cloneForRetry() *Command {
	return &Command{
		Binary:        c.Binary,
		Args:          append([]string{}, c.Args...), // Copy slice
		Input:         c.Input,
		Output:        c.Output,
		LogLevel:      c.LogLevel,
		Overwrite:     c.Overwrite,
		doneCh:        make(chan struct{}),
		stderrLogPath: c.stderrLogPath,
		stderrLines:   make([]string, 0, 100),
	}
}

// captureStderr reads FFmpeg stderr and optionally writes to a log file.
// It also stores recent lines for debugging.
func (c *Command) captureStderr(stderr io.ReadCloser, logPath string, done chan struct{}) {
	defer close(done)

	// Open log file if path is provided
	var logFile *os.File
	if logPath != "" {
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
		if err != nil {
			// Log error but continue - we'll still capture to memory
			fmt.Fprintf(os.Stderr, "failed to open ffmpeg log file %s: %v\n", logPath, err)
		} else {
			defer logFile.Close()
			// Write header with timestamp and command
			fmt.Fprintf(logFile, "\n=== FFmpeg session started at %s ===\n", time.Now().Format(time.RFC3339))
			fmt.Fprintf(logFile, "Command: %s\n\n", c.String())
		}
	}

	scanner := bufio.NewScanner(stderr)
	const maxLines = 100 // Keep last 100 lines in memory

	for scanner.Scan() {
		line := scanner.Text()

		// Store in memory (ring buffer behavior)
		c.stderrMu.Lock()
		if len(c.stderrLines) >= maxLines {
			c.stderrLines = c.stderrLines[1:] // Remove oldest
		}
		c.stderrLines = append(c.stderrLines, line)
		c.stderrMu.Unlock()

		// Write to log file if open
		if logFile != nil {
			fmt.Fprintln(logFile, line)
		}
	}

	// Write footer on completion
	if logFile != nil {
		fmt.Fprintf(logFile, "\n=== FFmpeg session ended at %s ===\n", time.Now().Format(time.RFC3339))
	}
}

// GetStderrLines returns the recent stderr lines captured from FFmpeg.
func (c *Command) GetStderrLines() []string {
	c.stderrMu.RLock()
	defer c.stderrMu.RUnlock()

	// Return a copy to avoid race conditions
	lines := make([]string, len(c.stderrLines))
	copy(lines, c.stderrLines)
	return lines
}

// stopMonitor stops the process monitor if running.
func (c *Command) stopMonitor() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.monitor != nil {
		c.monitor.Stop()
	}
}

// ProcessStats returns the current process statistics.
// Returns nil if the process is not running or monitoring is not active.
func (c *Command) ProcessStats() *ProcessStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.monitor == nil {
		return nil
	}

	stats := c.monitor.Stats()
	return &stats
}

// Monitor returns the process monitor for direct access.
// Returns nil if monitoring is not active.
func (c *Command) Monitor() *ProcessMonitor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.monitor
}

// PipeToWriter runs FFmpeg and pipes output to a writer with buffer.
func (c *Command) PipeToWriter(ctx context.Context, w io.Writer) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.started = time.Now()

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("getting stdout pipe: %w", err)
	}
	c.mu.Unlock()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	// Copy output to writer
	_, copyErr := io.Copy(w, stdout)

	// Wait for process to complete
	waitErr := c.cmd.Wait()

	if copyErr != nil {
		return fmt.Errorf("copying output: %w", copyErr)
	}

	return waitErr
}
