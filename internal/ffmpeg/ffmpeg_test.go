package ffmpeg

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoFFmpeg skips the test if ffmpeg is not installed.
func skipIfNoFFmpeg(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	return path
}

// skipIfNoFFprobe skips the test if ffprobe is not installed.
func skipIfNoFFprobe(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	return path
}

func TestBinaryDetector_Detect(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	ctx := context.Background()
	detector := NewBinaryDetector()

	info, err := detector.Detect(ctx)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.NotEmpty(t, info.FFmpegPath)
	assert.NotEmpty(t, info.FFprobePath)
	assert.NotEmpty(t, info.Version)
	assert.Greater(t, info.MajorVersion, 0)
}

func TestBinaryDetector_Caching(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	ctx := context.Background()
	detector := NewBinaryDetector().WithCacheTTL(1 * time.Hour)

	// First detection
	info1, err := detector.Detect(ctx)
	require.NoError(t, err)

	// Second detection should return cached result
	info2, err := detector.Detect(ctx)
	require.NoError(t, err)

	assert.Equal(t, info1.FFmpegPath, info2.FFmpegPath)
	assert.Equal(t, info1.Version, info2.Version)
}

func TestBinaryDetector_Clear(t *testing.T) {
	skipIfNoFFmpeg(t)
	skipIfNoFFprobe(t)

	ctx := context.Background()
	detector := NewBinaryDetector()

	// Detect and cache
	_, err := detector.Detect(ctx)
	require.NoError(t, err)

	// Clear cache
	detector.Clear()

	// Verify cache is cleared (will need to detect again)
	assert.Nil(t, detector.info)
}

func TestBinaryInfo_HasEncoder(t *testing.T) {
	info := &BinaryInfo{
		Encoders: []string{"libx264", "libx265", "aac", "libmp3lame"},
	}

	assert.True(t, info.HasEncoder("libx264"))
	assert.True(t, info.HasEncoder("aac"))
	assert.False(t, info.HasEncoder("h264_nvenc"))
}

func TestBinaryInfo_HasDecoder(t *testing.T) {
	info := &BinaryInfo{
		Decoders: []string{"h264", "hevc", "aac", "mp3"},
	}

	assert.True(t, info.HasDecoder("h264"))
	assert.True(t, info.HasDecoder("aac"))
	assert.False(t, info.HasDecoder("vp9"))
}

func TestBinaryInfo_HasFormat(t *testing.T) {
	info := &BinaryInfo{
		Formats: []FormatInfo{
			{Name: "mpegts", CanMux: true, CanDemux: true},
			{Name: "hls", CanMux: true, CanDemux: true},
			{Name: "rawvideo", CanMux: false, CanDemux: true},
		},
	}

	assert.True(t, info.HasFormat("mpegts"))
	assert.True(t, info.HasFormat("hls"))
	assert.False(t, info.HasFormat("rawvideo")) // Can't mux
	assert.False(t, info.HasFormat("nonexistent"))
}

func TestBinaryInfo_SupportsMinVersion(t *testing.T) {
	info := &BinaryInfo{
		MajorVersion: 6,
		MinorVersion: 1,
	}

	assert.True(t, info.SupportsMinVersion(5, 0))
	assert.True(t, info.SupportsMinVersion(6, 0))
	assert.True(t, info.SupportsMinVersion(6, 1))
	assert.False(t, info.SupportsMinVersion(6, 2))
	assert.False(t, info.SupportsMinVersion(7, 0))
}

func TestBinaryInfo_JSON(t *testing.T) {
	info := &BinaryInfo{
		FFmpegPath:   "/usr/bin/ffmpeg",
		FFprobePath:  "/usr/bin/ffprobe",
		Version:      "6.0",
		MajorVersion: 6,
		MinorVersion: 0,
	}

	jsonStr := info.JSON()
	assert.Contains(t, jsonStr, "ffmpeg_path")
	assert.Contains(t, jsonStr, "/usr/bin/ffmpeg")
}

func TestCommandBuilder_Build(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		HideBanner().
		Overwrite().
		Input("input.mp4").
		VideoCodec("libx264").
		AudioCodec("aac").
		Output("output.mp4").
		Build()

	assert.Equal(t, "/usr/bin/ffmpeg", cmd.Binary)
	assert.Contains(t, cmd.Args, "-hide_banner")
	assert.Contains(t, cmd.Args, "-y")
	assert.Contains(t, cmd.Args, "-i")
	assert.Contains(t, cmd.Args, "input.mp4")
	assert.Contains(t, cmd.Args, "-c:v")
	assert.Contains(t, cmd.Args, "libx264")
	assert.Contains(t, cmd.Args, "-c:a")
	assert.Contains(t, cmd.Args, "aac")
	assert.Equal(t, "output.mp4", cmd.Args[len(cmd.Args)-1])
}

func TestCommandBuilder_String(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		HideBanner().
		Input("input.mp4").
		VideoCodec("copy").
		Output("output.mp4").
		Build()

	str := cmd.String()
	assert.Contains(t, str, "/usr/bin/ffmpeg")
	assert.Contains(t, str, "-hide_banner")
	assert.Contains(t, str, "input.mp4")
	assert.Contains(t, str, "output.mp4")
}

func TestCommandBuilder_WithHWAccel(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		HWAccel("cuda").
		HWAccelDevice("0").
		Input("input.mp4").
		VideoCodec("h264_nvenc").
		Output("output.mp4").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-hwaccel cuda")
	assert.Contains(t, cmdStr, "-hwaccel_device 0")
}

func TestCommandBuilder_WithVideoFilter(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		Input("input.mp4").
		VideoFilter("scale=1280:720").
		VideoFilter("fps=30").
		Output("output.mp4").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-vf scale=1280:720,fps=30")
}

func TestCommandBuilder_MpegtsArgs(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		Input("input.mp4").
		VideoCodec("copy").
		MpegtsArgs().
		Output("pipe:1").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-f mpegts")
	assert.Contains(t, cmdStr, "-mpegts_copyts 1")
	assert.Contains(t, cmdStr, "-avoid_negative_ts disabled")
	assert.Contains(t, cmdStr, "-mpegts_start_pid 256")
	assert.Contains(t, cmdStr, "-mpegts_pmt_start_pid 4096")
}

func TestCommandBuilder_HLSArgs(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		Input("input.mp4").
		VideoCodec("libx264").
		HLSArgs(4, 5).
		Output("output.m3u8").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-f hls")
	assert.Contains(t, cmdStr, "-hls_time 4")
	assert.Contains(t, cmdStr, "-hls_list_size 5")
}

func TestCommand_IsRunning(t *testing.T) {
	cmd := &Command{
		Binary: "/usr/bin/ffmpeg",
		Args:   []string{"-version"},
	}

	assert.False(t, cmd.IsRunning())
}

func TestParseFramerate(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"30/1", 30.0},
		{"25/1", 25.0},
		{"30000/1001", 29.97002997002997},
		{"24000/1001", 23.976023976023978},
		{"60", 60.0},
		{"invalid", 0},
		{"0/0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseFramerate(tt.input)
			if tt.expected == 0 {
				assert.Equal(t, float64(0), result)
			} else {
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestProbeResult_GetVideoStream(t *testing.T) {
	result := &ProbeResult{
		Streams: []ProbeStream{
			{Index: 0, CodecType: "audio", CodecName: "aac"},
			{Index: 1, CodecType: "video", CodecName: "h264"},
			{Index: 2, CodecType: "subtitle", CodecName: "srt"},
		},
	}

	video := result.GetVideoStream()
	require.NotNil(t, video)
	assert.Equal(t, "h264", video.CodecName)
	assert.Equal(t, 1, video.Index)
}

func TestProbeResult_GetAudioStream(t *testing.T) {
	result := &ProbeResult{
		Streams: []ProbeStream{
			{Index: 0, CodecType: "video", CodecName: "h264"},
			{Index: 1, CodecType: "audio", CodecName: "aac"},
			{Index: 2, CodecType: "audio", CodecName: "mp3"},
		},
	}

	audio := result.GetAudioStream()
	require.NotNil(t, audio)
	assert.Equal(t, "aac", audio.CodecName)
	assert.Equal(t, 1, audio.Index)
}

func TestProbeResult_GetStreamsByType(t *testing.T) {
	result := &ProbeResult{
		Streams: []ProbeStream{
			{Index: 0, CodecType: "video", CodecName: "h264"},
			{Index: 1, CodecType: "audio", CodecName: "aac"},
			{Index: 2, CodecType: "audio", CodecName: "mp3"},
			{Index: 3, CodecType: "subtitle", CodecName: "srt"},
		},
	}

	audioStreams := result.GetStreamsByType("audio")
	assert.Len(t, audioStreams, 2)

	videoStreams := result.GetStreamsByType("video")
	assert.Len(t, videoStreams, 1)

	dataStreams := result.GetStreamsByType("data")
	assert.Len(t, dataStreams, 0)
}

func TestProbeResult_Duration(t *testing.T) {
	result := &ProbeResult{
		Format: ProbeFormat{
			Duration: "123.456",
		},
	}

	assert.Equal(t, int64(123456), result.Duration())

	emptyResult := &ProbeResult{}
	assert.Equal(t, int64(0), emptyResult.Duration())
}

func TestProbeResult_Bitrate(t *testing.T) {
	result := &ProbeResult{
		Format: ProbeFormat{
			BitRate: "5000000",
		},
	}

	assert.Equal(t, 5000000, result.Bitrate())

	emptyResult := &ProbeResult{}
	assert.Equal(t, 0, emptyResult.Bitrate())
}

func TestProbeStream_Framerate(t *testing.T) {
	stream := &ProbeStream{
		AvgFrameRate: "30000/1001",
		RFrameRate:   "30/1",
	}

	fr := stream.Framerate()
	assert.InDelta(t, 29.97, fr, 0.01)

	streamWithoutAvg := &ProbeStream{
		RFrameRate: "25/1",
	}
	assert.InDelta(t, 25.0, streamWithoutAvg.Framerate(), 0.01)
}

func TestHWAccelInfo(t *testing.T) {
	info := &BinaryInfo{
		HWAccels: []HWAccelInfo{
			{Type: HWAccelNVENC, Name: "cuda", Available: true},
			{Type: HWAccelQSV, Name: "qsv", Available: false},
			{Type: HWAccelVAAPI, Name: "vaapi", Available: true},
		},
	}

	assert.True(t, info.HasHWAccel(HWAccelNVENC))
	assert.False(t, info.HasHWAccel(HWAccelQSV)) // Not available
	assert.True(t, info.HasHWAccel(HWAccelVAAPI))
	assert.False(t, info.HasHWAccel(HWAccelVideoToolbox))

	available := info.GetAvailableHWAccels()
	assert.Len(t, available, 2)
}

func TestGetRecommendedHWAccel(t *testing.T) {
	accels := []HWAccelInfo{
		{Type: HWAccelVAAPI, Name: "vaapi", Available: true},
		{Type: HWAccelNVENC, Name: "cuda", Available: true},
		{Type: HWAccelQSV, Name: "qsv", Available: false},
	}

	recommended := GetRecommendedHWAccel(accels)
	require.NotNil(t, recommended)
	// VAAPI is preferred on Linux due to broad GPU support
	assert.Equal(t, HWAccelVAAPI, recommended.Type)

	// NVENC is returned when VAAPI is not available
	nvencOnlyAccels := []HWAccelInfo{
		{Type: HWAccelNVENC, Name: "cuda", Available: true},
		{Type: HWAccelQSV, Name: "qsv", Available: false},
	}
	nvencRecommended := GetRecommendedHWAccel(nvencOnlyAccels)
	require.NotNil(t, nvencRecommended)
	assert.Equal(t, HWAccelNVENC, nvencRecommended.Type)

	// No available accels
	noAccels := []HWAccelInfo{
		{Type: HWAccelQSV, Name: "qsv", Available: false},
	}
	assert.Nil(t, GetRecommendedHWAccel(noAccels))
}

// Integration tests that require FFmpeg to be installed

func TestIntegration_BinaryDetector_GetCodecs(t *testing.T) {
	ffmpegPath := skipIfNoFFmpeg(t)

	ctx := context.Background()
	detector := NewBinaryDetector()

	codecs, err := detector.getCodecs(ctx, ffmpegPath)
	require.NoError(t, err)
	require.NotEmpty(t, codecs)

	// Check for common codecs
	var hasH264, hasAAC bool
	for _, codec := range codecs {
		if codec.Name == "h264" {
			hasH264 = true
			assert.Equal(t, "video", codec.Type)
			assert.True(t, codec.CanDecode)
		}
		if codec.Name == "aac" {
			hasAAC = true
			assert.Equal(t, "audio", codec.Type)
		}
	}

	assert.True(t, hasH264, "h264 codec not found")
	assert.True(t, hasAAC, "aac codec not found")
}

func TestIntegration_BinaryDetector_GetEncoders(t *testing.T) {
	ffmpegPath := skipIfNoFFmpeg(t)

	ctx := context.Background()
	detector := NewBinaryDetector()

	encoders, err := detector.getEncoders(ctx, ffmpegPath)
	require.NoError(t, err)
	require.NotEmpty(t, encoders)

	// Check for common encoders
	hasLibx264 := false
	for _, enc := range encoders {
		if enc == "libx264" {
			hasLibx264 = true
			break
		}
	}

	// libx264 might not be available in all builds
	if hasLibx264 {
		t.Log("libx264 encoder available")
	}
}

func TestIntegration_BinaryDetector_GetFormats(t *testing.T) {
	ffmpegPath := skipIfNoFFmpeg(t)

	ctx := context.Background()
	detector := NewBinaryDetector()

	formats, err := detector.getFormats(ctx, ffmpegPath)
	require.NoError(t, err)
	require.NotEmpty(t, formats)

	// Check for common formats
	var hasMpegts, hasHLS bool
	for _, fmt := range formats {
		if strings.HasPrefix(fmt.Name, "mpegts") {
			hasMpegts = true
			assert.True(t, fmt.CanMux || fmt.CanDemux)
		}
		if fmt.Name == "hls" {
			hasHLS = true
		}
	}

	assert.True(t, hasMpegts, "mpegts format not found")
	assert.True(t, hasHLS, "hls format not found")
}

func TestIntegration_Prober_ProbeTestVideo(t *testing.T) {
	ffprobePath := skipIfNoFFprobe(t)
	ffmpegPath := skipIfNoFFmpeg(t)

	// Create a test video using ffmpeg
	ctx := context.Background()
	testFile := "/tmp/tvarr_test_probe.mp4"

	// Generate a short test video
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=duration=1:frequency=440:sample_rate=48000",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		testFile)

	if err := cmd.Run(); err != nil {
		t.Skipf("Could not create test video: %v", err)
	}

	// Now probe it
	prober := NewProber(ffprobePath)
	result, err := prober.Probe(ctx, testFile)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify video stream
	videoStream := result.GetVideoStream()
	require.NotNil(t, videoStream)
	assert.Equal(t, "h264", videoStream.CodecName)
	assert.Equal(t, 320, videoStream.Width)
	assert.Equal(t, 240, videoStream.Height)

	// Verify audio stream
	audioStream := result.GetAudioStream()
	require.NotNil(t, audioStream)
	assert.Equal(t, "aac", audioStream.CodecName)

	// Verify format
	assert.Contains(t, result.Format.FormatName, "mp4")

	// Clean up
	exec.Command("rm", "-f", testFile).Run()
}

func TestIntegration_Prober_ProbeSimple(t *testing.T) {
	ffprobePath := skipIfNoFFprobe(t)
	ffmpegPath := skipIfNoFFmpeg(t)

	ctx := context.Background()
	testFile := "/tmp/tvarr_test_probe_simple.mp4"

	// Generate test video
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=1920x1080:rate=25",
		"-f", "lavfi", "-i", "sine=duration=1:frequency=440:sample_rate=44100",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		testFile)

	if err := cmd.Run(); err != nil {
		t.Skipf("Could not create test video: %v", err)
	}

	prober := NewProber(ffprobePath)
	info, err := prober.ProbeSimple(ctx, testFile)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "h264", info.VideoCodec)
	assert.Equal(t, 1920, info.VideoWidth)
	assert.Equal(t, 1080, info.VideoHeight)
	assert.InDelta(t, 25.0, info.VideoFramerate, 0.1)
	assert.Equal(t, "aac", info.AudioCodec)
	assert.Contains(t, info.ContainerFormat, "mp4")
	assert.Greater(t, info.Duration, int64(0))
	assert.False(t, info.IsLiveStream)

	// Clean up
	exec.Command("rm", "-f", testFile).Run()
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, 3, cfg.MaxAttempts)
	assert.Equal(t, 500*time.Millisecond, cfg.InitialDelay)
	assert.Equal(t, 5*time.Second, cfg.MaxDelay)
	assert.Equal(t, 2.0, cfg.BackoffFactor)
	assert.Equal(t, 5*time.Second, cfg.MinRunTime)
	assert.False(t, cfg.RetryOnAnyError)
}

func TestCommand_CloneForRetry(t *testing.T) {
	original := NewCommandBuilder("/usr/bin/ffmpeg").
		Input("input.mp4").
		VideoCodec("libx264").
		AudioCodec("aac").
		StderrLogPath("/tmp/test.log").
		Output("output.mp4").
		Build()

	clone := original.cloneForRetry()

	// Verify clone has same settings
	assert.Equal(t, original.Binary, clone.Binary)
	assert.Equal(t, original.Args, clone.Args)
	assert.Equal(t, original.Input, clone.Input)
	assert.Equal(t, original.Output, clone.Output)
	assert.Equal(t, original.LogLevel, clone.LogLevel)
	assert.Equal(t, original.Overwrite, clone.Overwrite)
	assert.Equal(t, original.stderrLogPath, clone.stderrLogPath)

	// Verify clone has independent slices (modifying one doesn't affect the other)
	originalArgs := original.Args
	original.Args = append(original.Args, "-extra")
	assert.NotEqual(t, len(originalArgs)+1, len(clone.Args))

	// Verify clone has fresh channel
	assert.NotNil(t, clone.doneCh)
}

func TestCommandBuilder_InitHWDevice(t *testing.T) {
	tests := []struct {
		name     string
		hwType   string
		device   string
		expected string
	}{
		{"vaapi with device", "vaapi", "/dev/dri/renderD128", "-init_hw_device vaapi=hw:/dev/dri/renderD128"},
		{"vaapi without device", "vaapi", "", "-init_hw_device vaapi=hw"},
		{"cuda with device", "cuda", "0", "-init_hw_device cuda=hw:0"},
		{"qsv without device", "qsv", "", "-init_hw_device qsv=hw"},
		{"none type", "none", "", ""},
		{"empty type", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCommandBuilder("/usr/bin/ffmpeg").
				InitHWDevice(tt.hwType, tt.device).
				Input("input.mp4").
				Output("output.mp4").
				Build()

			cmdStr := cmd.String()
			if tt.expected != "" {
				assert.Contains(t, cmdStr, tt.expected)
			} else {
				assert.NotContains(t, cmdStr, "-init_hw_device")
			}
		})
	}
}

func TestCommandBuilder_HWUploadFilter(t *testing.T) {
	tests := []struct {
		name     string
		hwType   string
		expected string
	}{
		{"vaapi", "vaapi", "format=nv12,hwupload"},
		{"cuda", "cuda", "format=nv12,hwupload_cuda"},
		{"nvenc", "nvenc", "format=nv12,hwupload_cuda"},
		{"qsv", "qsv", "format=nv12,hwupload=extra_hw_frames=64"},
		{"videotoolbox", "videotoolbox", "format=nv12,hwupload"},
		{"unknown", "unknown", "format=nv12,hwupload"},
		{"none type", "none", ""},
		{"empty type", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCommandBuilder("/usr/bin/ffmpeg").
				Input("input.mp4").
				HWUploadFilter(tt.hwType).
				Output("output.mp4").
				Build()

			cmdStr := cmd.String()
			if tt.expected != "" {
				assert.Contains(t, cmdStr, "-vf "+tt.expected)
			} else {
				assert.NotContains(t, cmdStr, "-vf")
			}
		})
	}
}

func TestCommandBuilder_Reconnect(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		Reconnect().
		Input("http://example.com/stream").
		Output("output.mp4").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-reconnect 1")
	assert.Contains(t, cmdStr, "-reconnect_streamed 1")
	assert.Contains(t, cmdStr, "-reconnect_delay_max 5")
}

func TestCommandBuilder_FMP4Args(t *testing.T) {
	tests := []struct {
		name         string
		fragDuration float64
		expected     []string
		notExpected  []string
	}{
		{
			name:         "with fragment duration",
			fragDuration: 6.0,
			expected: []string{
				"-f mp4",
				// NOTE: frag_keyframe is intentionally NOT included - it creates tiny
				// fragments at every keyframe. We rely on frag_duration instead.
				"-movflags empty_moov+default_base_moof+skip_trailer+cmaf",
				"-frag_duration 6000000",
			},
		},
		{
			name:         "without fragment duration",
			fragDuration: 0,
			expected: []string{
				"-f mp4",
				"-movflags empty_moov+default_base_moof+skip_trailer+cmaf",
			},
			notExpected: []string{
				"-frag_duration",
			},
		},
		{
			name:         "short fragment duration",
			fragDuration: 2.0,
			expected: []string{
				"-f mp4",
				"-frag_duration 2000000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCommandBuilder("/usr/bin/ffmpeg").
				Input("input.mp4").
				VideoCodec("libx264").
				FMP4Args(tt.fragDuration).
				Output("pipe:1").
				Build()

			cmdStr := cmd.String()
			for _, exp := range tt.expected {
				assert.Contains(t, cmdStr, exp)
			}
			for _, notExp := range tt.notExpected {
				assert.NotContains(t, cmdStr, notExp)
			}
		})
	}
}

func TestCommandBuilder_FMP4ArgsWithMinFrag(t *testing.T) {
	cmd := NewCommandBuilder("/usr/bin/ffmpeg").
		Input("input.mp4").
		VideoCodec("libx264").
		FMP4ArgsWithMinFrag(6.0, 2.0).
		Output("pipe:1").
		Build()

	cmdStr := cmd.String()
	assert.Contains(t, cmdStr, "-f mp4")
	// NOTE: frag_keyframe is intentionally NOT included - it creates tiny
	// fragments at every keyframe. We rely on frag_duration instead.
	assert.Contains(t, cmdStr, "-movflags empty_moov+default_base_moof+skip_trailer+cmaf")
	assert.Contains(t, cmdStr, "-frag_duration 6000000")
	assert.Contains(t, cmdStr, "-min_frag_duration 2000000")
}

func TestIntegration_FFmpegFMP4Output(t *testing.T) {
	ffmpegPath := skipIfNoFFmpeg(t)
	ffprobePath := skipIfNoFFprobe(t)

	ctx := context.Background()
	testFile := "/tmp/tvarr_test_fmp4.mp4"

	// Generate a short fMP4 test video using the FMP4Args builder
	// Note: We use exec.CommandContext directly since CommandBuilder doesn't
	// support multiple inputs naturally. This tests the output args generation.
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=duration=2:frequency=440:sample_rate=48000",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof+skip_trailer+cmaf",
		"-frag_duration", "1000000",
		testFile)

	t.Logf("Running FFmpeg command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("FFmpeg output: %s", string(output))
	}
	require.NoError(t, err, "FFmpeg fMP4 generation failed")

	// Verify the output file exists and has content
	prober := NewProber(ffprobePath)
	result, err := prober.Probe(ctx, testFile)
	require.NoError(t, err, "Failed to probe fMP4 output")

	// Verify video stream
	videoStream := result.GetVideoStream()
	require.NotNil(t, videoStream)
	assert.Equal(t, "h264", videoStream.CodecName)

	// Verify audio stream
	audioStream := result.GetAudioStream()
	require.NotNil(t, audioStream)
	assert.Equal(t, "aac", audioStream.CodecName)

	// Verify it's an MP4 container (fMP4 uses mp4 muxer)
	assert.Contains(t, result.Format.FormatName, "mp4")

	// Clean up
	exec.Command("rm", "-f", testFile).Run()
}
