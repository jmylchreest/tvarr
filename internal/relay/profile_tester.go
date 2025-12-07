package relay

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// ProfileTestResult contains the results of testing a relay profile
type ProfileTestResult struct {
	Success          bool            `json:"success"`
	DurationMs       int64           `json:"duration_ms"`
	FramesProcessed  int             `json:"frames_processed"`
	FPS              float64         `json:"fps"`
	VideoCodecIn     string          `json:"video_codec_in,omitempty"`
	VideoCodecOut    string          `json:"video_codec_out,omitempty"`
	AudioCodecIn     string          `json:"audio_codec_in,omitempty"`
	AudioCodecOut    string          `json:"audio_codec_out,omitempty"`
	Resolution       string          `json:"resolution,omitempty"`
	HWAccelActive    bool            `json:"hw_accel_active"`
	HWAccelMethod    string          `json:"hw_accel_method,omitempty"`
	BitrateKbps      int             `json:"bitrate_kbps,omitempty"`
	Errors           []string        `json:"errors,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
	Suggestions      []string        `json:"suggestions,omitempty"`
	FFmpegOutput     string          `json:"ffmpeg_output,omitempty"`
	FFmpegCommand    string          `json:"ffmpeg_command,omitempty"`
	ExitCode         int             `json:"exit_code"`
	StreamInfo       *TestStreamInfo `json:"stream_info,omitempty"`
}

// TestStreamInfo contains information about the test stream
type TestStreamInfo struct {
	InputURL  string `json:"input_url"`
	OutputURL string `json:"output_url"`
}

// ProfileTester runs profile tests against a sample stream
type ProfileTester struct {
	ffmpegPath  string
	ffprobePath string
	testTimeout time.Duration
}

// NewProfileTester creates a new profile tester
func NewProfileTester(ffmpegPath, ffprobePath string) *ProfileTester {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	return &ProfileTester{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath,
		testTimeout: 30 * time.Second, // Default 30 second timeout
	}
}

// SetTimeout sets the test timeout duration
func (t *ProfileTester) SetTimeout(timeout time.Duration) {
	t.testTimeout = timeout
}

// TestProfile runs a test of the given profile against a sample stream URL
func (t *ProfileTester) TestProfile(ctx context.Context, profile *models.RelayProfile, testStreamURL string) *ProfileTestResult {
	result := &ProfileTestResult{
		StreamInfo: &TestStreamInfo{
			InputURL:  testStreamURL,
			OutputURL: "null (test output)",
		},
	}

	if testStreamURL == "" {
		result.Errors = append(result.Errors, "No test stream URL provided")
		result.Suggestions = append(result.Suggestions, "Provide a valid stream URL to test the profile")
		return result
	}

	// Build the FFmpeg command using the profile settings
	args := t.buildTestCommand(profile, testStreamURL)
	result.FFmpegCommand = t.ffmpegPath + " " + strings.Join(args, " ")

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, t.testTimeout)
	defer cancel()

	// Run FFmpeg
	start := time.Now()
	cmd := exec.CommandContext(testCtx, t.ffmpegPath, args...)

	// Capture stderr (FFmpeg outputs to stderr)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to capture stderr: %v", err))
		return result
	}

	if err := cmd.Start(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to start FFmpeg: %v", err))
		t.addSuggestionsForError(result, err.Error())
		return result
	}

	// Read stderr output
	var output strings.Builder
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line + "\n")
	}

	err = cmd.Wait()
	result.DurationMs = time.Since(start).Milliseconds()
	result.FFmpegOutput = output.String()

	// Check exit status
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		// Context deadline exceeded is expected for successful streaming tests
		if testCtx.Err() == context.DeadlineExceeded {
			// This is actually success - we streamed for the full test duration
			result.Success = true
		} else {
			result.Success = false
			result.Errors = append(result.Errors, fmt.Sprintf("FFmpeg exited with error: %v", err))
		}
	} else {
		result.Success = true
	}

	// Parse the FFmpeg output
	t.parseFFmpegOutput(result, output.String())

	// Generate suggestions based on results
	t.generateSuggestions(result, profile)

	return result
}

// buildTestCommand builds the FFmpeg command for testing.
// Flag order follows m3u-proxy's proven approach for consistency with actual streaming.
func (t *ProfileTester) buildTestCommand(profile *models.RelayProfile, inputURL string) []string {
	builder := ffmpeg.NewCommandBuilder(t.ffmpegPath)

	// 1. GLOBAL FLAGS
	builder.HideBanner()
	builder.LogLevel("info")

	// 2. HARDWARE ACCELERATION - Must come BEFORE input
	hwAccelType := ""
	if profile.HWAccel == models.HWAccelAuto {
		// For testing, we skip auto-detection since we're just validating the profile
		// The actual hwaccel will be selected at runtime in session.go
	} else if profile.HWAccel != "" && profile.HWAccel != models.HWAccelNone {
		hwAccelType = string(profile.HWAccel)

		// Initialize hardware device
		devicePath := profile.HWAccelDevice
		if devicePath == "" && profile.GpuIndex >= 0 {
			devicePath = fmt.Sprintf("%d", profile.GpuIndex)
		}
		if devicePath != "" || hwAccelType != "" {
			builder.InitHWDevice(hwAccelType, devicePath)
		}

		builder.HWAccel(hwAccelType)
		if devicePath != "" {
			builder.HWAccelDevice(devicePath)
		}
		if profile.HWAccelOutputFormat != "" {
			builder.HWAccelOutputFormat(profile.HWAccelOutputFormat)
		}

		// Hardware decoder codec
		if profile.HWAccelDecoderCodec != "" {
			builder.InputArgs("-c:v", profile.HWAccelDecoderCodec)
		}

		// Extra hwaccel options
		if profile.HWAccelExtraOptions != "" {
			builder.ApplyCustomInputOptions(profile.HWAccelExtraOptions)
		}
	}

	// 3. INPUT ANALYSIS FLAGS
	builder.InputArgs("-analyzeduration", "10000000")
	builder.InputArgs("-probesize", "10000000")

	// 4. CUSTOM INPUT OPTIONS
	if profile.InputOptions != "" {
		builder.ApplyCustomInputOptions(profile.InputOptions)
	}

	// 5. INPUT
	builder.Input(inputURL)

	// 6. STREAM MAPPING
	builder.OutputArgs("-map", "0:v:0")
	builder.OutputArgs("-map", "0:a:0?")

	// 7. VIDEO CODEC
	if profile.VideoCodec != "" {
		builder.VideoCodec(string(profile.VideoCodec))

		// Add hardware upload filter when transcoding with HW acceleration
		if hwAccelType != "" && profile.VideoCodec != models.VideoCodecCopy {
			builder.HWUploadFilter(hwAccelType)
		}
	}

	// 8. VIDEO SETTINGS
	if profile.VideoBitrate > 0 {
		builder.VideoBitrate(fmt.Sprintf("%dk", profile.VideoBitrate))
	}
	if profile.VideoPreset != "" {
		builder.VideoPreset(profile.VideoPreset)
	}

	// Filter complex
	if profile.FilterComplex != "" {
		builder.ApplyFilterComplex(profile.FilterComplex)
	}

	// 9. AUDIO CODEC AND SETTINGS
	if profile.AudioCodec != "" {
		builder.AudioCodec(string(profile.AudioCodec))
	}
	if profile.AudioBitrate > 0 {
		builder.AudioBitrate(fmt.Sprintf("%dk", profile.AudioBitrate))
	}

	// 10. BITSTREAM FILTERS
	videoCodecFamily := ffmpeg.GetCodecFamily(string(profile.VideoCodec))
	isVideoCopy := profile.VideoCodec == models.VideoCodecCopy
	bsfInfo := ffmpeg.GetVideoBitstreamFilter(videoCodecFamily, ffmpeg.FormatMPEGTS, isVideoCopy)
	if bsfInfo.VideoBSF != "" {
		builder.VideoBitstreamFilter(bsfInfo.VideoBSF)
	}

	// 11. CUSTOM OUTPUT OPTIONS
	if profile.OutputOptions != "" {
		builder.ApplyCustomOutputOptions(profile.OutputOptions)
	}

	// 12. OUTPUT - Use null output for testing
	builder.OutputArgs("-t", "10") // Test for 10 seconds max
	builder.OutputFormat("null")   // Null muxer
	builder.Output("-")            // Required output target for null format

	cmd := builder.Build()
	return cmd.Args
}

// parseFFmpegOutput parses FFmpeg's stderr output for metrics
func (t *ProfileTester) parseFFmpegOutput(result *ProfileTestResult, output string) {
	// Parse frame count
	frameRe := regexp.MustCompile(`frame=\s*(\d+)`)
	if matches := frameRe.FindStringSubmatch(output); len(matches) > 1 {
		result.FramesProcessed, _ = strconv.Atoi(matches[1])
	}

	// Parse FPS
	fpsRe := regexp.MustCompile(`fps=\s*([\d.]+)`)
	if matches := fpsRe.FindStringSubmatch(output); len(matches) > 1 {
		result.FPS, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Parse bitrate
	bitrateRe := regexp.MustCompile(`bitrate=\s*([\d.]+)kbits/s`)
	if matches := bitrateRe.FindStringSubmatch(output); len(matches) > 1 {
		bitrate, _ := strconv.ParseFloat(matches[1], 64)
		result.BitrateKbps = int(bitrate)
	}

	// Parse input stream info
	inputStreamRe := regexp.MustCompile(`Stream #\d+:\d+.*: Video: (\w+).*, (\d+x\d+)`)
	if matches := inputStreamRe.FindStringSubmatch(output); len(matches) > 2 {
		result.VideoCodecIn = matches[1]
		result.Resolution = matches[2]
	}

	audioStreamRe := regexp.MustCompile(`Stream #\d+:\d+.*: Audio: (\w+)`)
	if matches := audioStreamRe.FindStringSubmatch(output); len(matches) > 1 {
		result.AudioCodecIn = matches[1]
	}

	// Check for hardware acceleration indicators
	hwAccelPatterns := map[string]string{
		"nvdec":       `Using NVIDIA NVDEC|hwaccel.*cuda|hw_frames_ctx`,
		"qsv":         `Using Intel QSV|hwaccel.*qsv`,
		"vaapi":       `Using VAAPI|hwaccel.*vaapi|libva`,
		"videotoolbox": `Using VideoToolbox|hwaccel.*videotoolbox`,
	}

	for method, pattern := range hwAccelPatterns {
		if matched, _ := regexp.MatchString(pattern, output); matched {
			result.HWAccelActive = true
			result.HWAccelMethod = method
			break
		}
	}

	// Check for encoder being used
	encoderRe := regexp.MustCompile(`encoder.*: (\w+)`)
	if matches := encoderRe.FindStringSubmatch(output); len(matches) > 1 {
		result.VideoCodecOut = matches[1]
	}

	// Check for common errors
	t.parseErrors(result, output)
}

// parseErrors checks for common FFmpeg errors in output
func (t *ProfileTester) parseErrors(result *ProfileTestResult, output string) {
	errorPatterns := []struct {
		pattern    string
		message    string
		suggestion string
	}{
		{
			pattern:    `Device or resource busy`,
			message:    "Hardware device is busy or unavailable",
			suggestion: "Another process may be using the GPU. Try software encoding or wait for other operations to complete.",
		},
		{
			pattern:    `No NVENC capable devices found`,
			message:    "NVIDIA NVENC hardware encoder not available",
			suggestion: "Use software encoding (libx264) or verify NVIDIA drivers are installed correctly.",
		},
		{
			pattern:    `Cannot load libcuda`,
			message:    "CUDA library not found",
			suggestion: "Install NVIDIA CUDA toolkit or use software encoding.",
		},
		{
			pattern:    `failed to initialise VAAPI`,
			message:    "VAAPI hardware acceleration failed to initialize",
			suggestion: "Check VAAPI drivers installation or use software encoding.",
		},
		{
			pattern:    `qsv.*failed|Intel QSV.*error`,
			message:    "Intel Quick Sync Video failed",
			suggestion: "Verify Intel GPU drivers or use software encoding.",
		},
		{
			pattern:    `Connection refused|Connection timed out`,
			message:    "Failed to connect to stream source",
			suggestion: "Verify the stream URL is correct and accessible.",
		},
		{
			pattern:    `Server returned 4\d\d`,
			message:    "Stream server returned client error",
			suggestion: "Check authentication credentials and stream URL.",
		},
		{
			pattern:    `Server returned 5\d\d`,
			message:    "Stream server returned server error",
			suggestion: "The stream source may be temporarily unavailable. Try again later.",
		},
		{
			pattern:    `Invalid data found when processing input`,
			message:    "Invalid stream data detected",
			suggestion: "The stream may be corrupted or using an unsupported format.",
		},
		{
			pattern:    `Discarding damaged frame`,
			message:    "Damaged frames in source stream",
			suggestion: "Source stream quality issues. Consider adding -fflags +discardcorrupt.",
		},
		{
			pattern:    `Discarding non-keyframe`,
			message:    "Stream starts without keyframe",
			suggestion: "Add -fflags +genpts to help with stream synchronization.",
		},
		{
			pattern:    `Error while decoding`,
			message:    "Decoding error encountered",
			suggestion: "Try different decoder settings or verify source stream format.",
		},
	}

	for _, ep := range errorPatterns {
		if matched, _ := regexp.MatchString(ep.pattern, output); matched {
			result.Warnings = append(result.Warnings, ep.message)
			// Only add suggestion if not already present
			found := false
			for _, s := range result.Suggestions {
				if s == ep.suggestion {
					found = true
					break
				}
			}
			if !found {
				result.Suggestions = append(result.Suggestions, ep.suggestion)
			}
		}
	}
}

// addSuggestionsForError adds suggestions based on error messages
func (t *ProfileTester) addSuggestionsForError(result *ProfileTestResult, errorMsg string) {
	if strings.Contains(errorMsg, "not found") {
		result.Suggestions = append(result.Suggestions, "Ensure FFmpeg is installed and accessible in PATH")
	}
	if strings.Contains(errorMsg, "permission denied") {
		result.Suggestions = append(result.Suggestions, "Check file permissions for FFmpeg binary")
	}
}

// generateSuggestions adds suggestions based on test results
func (t *ProfileTester) generateSuggestions(result *ProfileTestResult, profile *models.RelayProfile) {
	// Check FPS performance
	if result.FPS > 0 && result.FPS < 20 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Low FPS detected: %.1f fps", result.FPS))
		if profile.VideoCodec != models.VideoCodecCopy {
			result.Suggestions = append(result.Suggestions, "Consider using copy mode or reducing encoding quality")
		}
		if !result.HWAccelActive && profile.UsesHardwareAccel() {
			result.Suggestions = append(result.Suggestions, "Hardware acceleration was requested but not active - verify GPU availability")
		}
	}

	// Check if hardware acceleration was expected but not used
	if profile.UsesHardwareAccel() && !result.HWAccelActive {
		result.Warnings = append(result.Warnings, "Hardware acceleration was configured but does not appear to be active")
		result.Suggestions = append(result.Suggestions, "Check hardware acceleration settings and driver installation")
	}

	// Check frame processing
	if result.Success && result.FramesProcessed == 0 {
		result.Warnings = append(result.Warnings, "No frames were processed during test")
		result.Suggestions = append(result.Suggestions, "The stream may not contain valid video data")
	}
}
