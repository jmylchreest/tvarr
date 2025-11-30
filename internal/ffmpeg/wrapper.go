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
	progressCh chan Progress
	errorCh    chan error
	doneCh     chan struct{}
}

// Progress represents FFmpeg progress information.
type Progress struct {
	Frame       int64         `json:"frame"`
	FPS         float64       `json:"fps"`
	Bitrate     string        `json:"bitrate"`
	TotalSize   int64         `json:"total_size"`
	Time        time.Duration `json:"time"`
	Speed       float64       `json:"speed"`
	DupFrames   int64         `json:"dup_frames"`
	DropFrames  int64         `json:"drop_frames"`
}

// CommandBuilder builds FFmpeg commands with a fluent API.
type CommandBuilder struct {
	binary      string
	globalArgs  []string
	inputArgs   []string
	input       string
	filterArgs  []string
	outputArgs  []string
	output      string
	logLevel    string
	overwrite   bool
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

// Progress enables progress output to a file/pipe.
func (b *CommandBuilder) Progress(target string) *CommandBuilder {
	b.globalArgs = append(b.globalArgs, "-progress", target)
	return b
}

// HWAccel sets the hardware acceleration method.
func (b *CommandBuilder) HWAccel(accel string) *CommandBuilder {
	if accel != "" && accel != "none" {
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

// Input sets the input source.
func (b *CommandBuilder) Input(input string) *CommandBuilder {
	b.input = input
	return b
}

// InputFormat sets the input format.
func (b *CommandBuilder) InputFormat(format string) *CommandBuilder {
	b.inputArgs = append(b.inputArgs, "-f", format)
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

// BufferSize sets the input buffer size.
func (b *CommandBuilder) BufferSize(size int) *CommandBuilder {
	b.inputArgs = append(b.inputArgs, "-buffer_size", strconv.Itoa(size))
	return b
}

// Timeout sets the connection timeout.
func (b *CommandBuilder) Timeout(timeout time.Duration) *CommandBuilder {
	microseconds := int(timeout.Microseconds())
	b.inputArgs = append(b.inputArgs, "-timeout", strconv.Itoa(microseconds))
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

// VideoProfile sets the video profile.
func (b *CommandBuilder) VideoProfile(profile string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-profile:v", profile)
	return b
}

// VideoPreset sets the encoding preset.
func (b *CommandBuilder) VideoPreset(preset string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-preset", preset)
	return b
}

// VideoScale sets the video scale.
func (b *CommandBuilder) VideoScale(width, height int) *CommandBuilder {
	scale := fmt.Sprintf("scale=%d:%d", width, height)
	b.filterArgs = append(b.filterArgs, scale)
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

// AudioSampleRate sets the audio sample rate.
func (b *CommandBuilder) AudioSampleRate(rate int) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-ar", strconv.Itoa(rate))
	return b
}

// CopyTimestamps copies input timestamps.
func (b *CommandBuilder) CopyTimestamps() *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-copyts")
	return b
}

// StartAtZero starts timestamps at zero.
func (b *CommandBuilder) StartAtZero() *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-start_at_zero")
	return b
}

// MapAll maps all streams from input.
func (b *CommandBuilder) MapAll() *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-map", "0")
	return b
}

// MapStream maps a specific stream.
func (b *CommandBuilder) MapStream(spec string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-map", spec)
	return b
}

// OutputFormat sets the output format.
func (b *CommandBuilder) OutputFormat(format string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, "-f", format)
	return b
}

// OutputArgs adds arbitrary output arguments.
func (b *CommandBuilder) OutputArgs(args ...string) *CommandBuilder {
	b.outputArgs = append(b.outputArgs, args...)
	return b
}

// MpegtsArgs adds common MPEG-TS output arguments.
func (b *CommandBuilder) MpegtsArgs() *CommandBuilder {
	b.outputArgs = append(b.outputArgs,
		"-f", "mpegts",
		"-mpegts_flags", "resend_headers")
	return b
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
		Binary:    b.binary,
		Args:      args,
		Input:     b.input,
		Output:    b.output,
		LogLevel:  b.logLevel,
		Overwrite: b.overwrite,
		doneCh:    make(chan struct{}),
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
func (c *Command) StreamToWriter(ctx context.Context, w io.Writer) error {
	c.mu.Lock()
	c.cmd = exec.CommandContext(ctx, c.Binary, c.Args...)
	c.cmd.Stdout = w
	c.started = time.Now()
	c.mu.Unlock()

	return c.cmd.Run()
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
