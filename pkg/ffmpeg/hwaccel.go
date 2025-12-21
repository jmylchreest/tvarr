package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/jmylchreest/tvarr/internal/util"
)

// HWAccelType represents a hardware acceleration type.
type HWAccelType string

const (
	HWAccelNone         HWAccelType = "none"
	HWAccelNVDEC        HWAccelType = "nvdec"        // NVIDIA NVDEC (decode)
	HWAccelNVENC        HWAccelType = "cuda"         // NVIDIA CUDA/NVENC
	HWAccelQSV          HWAccelType = "qsv"          // Intel Quick Sync
	HWAccelVAAPI        HWAccelType = "vaapi"        // VA-API (Linux)
	HWAccelVideoToolbox HWAccelType = "videotoolbox" // macOS
	HWAccelDXVA2        HWAccelType = "dxva2"        // Windows (older)
	HWAccelD3D11VA      HWAccelType = "d3d11va"      // Windows 8+
	HWAccelVulkan       HWAccelType = "vulkan"       // Cross-platform Vulkan
	HWAccelOCL          HWAccelType = "opencl"       // OpenCL
)

// FilteredEncoder describes an encoder that was filtered out and why.
type FilteredEncoder struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// HWAccelInfo contains information about a hardware accelerator.
type HWAccelInfo struct {
	Type             HWAccelType       `json:"type"`
	Name             string            `json:"name"`
	Available        bool              `json:"available"`
	DeviceName       string            `json:"device_name,omitempty"`
	Encoders         []string          `json:"encoders,omitempty"`          // Validated HW encoders
	Decoders         []string          `json:"decoders,omitempty"`          // HW decoders
	FilteredEncoders []FilteredEncoder `json:"filtered_encoders,omitempty"` // Encoders filtered out with reasons
}

// HWAccelDetector detects available hardware acceleration.
type HWAccelDetector struct {
	ffmpegPath string
}

// vainfo path cache - found once and reused
var (
	vainfoPath     string
	vainfoPathOnce sync.Once
	vainfoFound    bool
)

// getVainfoPath returns the path to vainfo binary, or empty string if not found.
// The result is cached for subsequent calls.
// Can be overridden via TVARR_VAINFO_PATH environment variable.
func getVainfoPath() string {
	vainfoPathOnce.Do(func() {
		path, err := util.FindBinary("vainfo", "TVARR_VAINFO_PATH")
		if err == nil {
			vainfoPath = path
			vainfoFound = true
		}
	})
	return vainfoPath
}

// NewHWAccelDetector creates a new hardware acceleration detector.
func NewHWAccelDetector(ffmpegPath string) *HWAccelDetector {
	return &HWAccelDetector{
		ffmpegPath: ffmpegPath,
	}
}

// Detect detects all available hardware accelerators.
func (d *HWAccelDetector) Detect(ctx context.Context) ([]HWAccelInfo, error) {
	// Get list of supported hwaccels from ffmpeg
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-hwaccels", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting hwaccels: %w", err)
	}

	supportedAccels := d.parseHWAccels(string(output))
	var results []HWAccelInfo

	// Test each accelerator
	for _, accel := range supportedAccels {
		info := HWAccelInfo{
			Type: HWAccelType(accel),
			Name: accel,
		}

		// Test if the accelerator actually works
		available, deviceName := d.testAccel(ctx, accel)
		info.Available = available
		info.DeviceName = deviceName

		if available {
			// Get encoders for this accelerator (with filtering info)
			info.Encoders, info.FilteredEncoders = d.getAccelEncoders(ctx, accel)
			info.Decoders = d.getAccelDecoders(ctx, accel)
		}

		results = append(results, info)
	}

	return results, nil
}

// parseHWAccels parses the output of ffmpeg -hwaccels.
func (d *HWAccelDetector) parseHWAccels(output string) []string {
	var accels []string
	lines := strings.Split(output, "\n")
	inList := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "Hardware acceleration methods:" {
			inList = true
			continue
		}
		if inList && line != "" {
			accels = append(accels, line)
		}
	}

	return accels
}

// testAccel tests if a hardware accelerator is actually available.
func (d *HWAccelDetector) testAccel(ctx context.Context, accel string) (bool, string) {
	switch accel {
	case "cuda", "nvdec":
		return d.testNVIDIA(ctx)
	case "qsv":
		return d.testQSV(ctx)
	case "vaapi":
		return d.testVAAPI(ctx)
	case "videotoolbox":
		return d.testVideoToolbox(ctx)
	case "dxva2", "d3d11va":
		return d.testWindowsHW(ctx, accel)
	case "vulkan":
		return d.testVulkan(ctx)
	default:
		// Unknown accelerator, assume available if listed
		return true, ""
	}
}

// testNVIDIA tests NVIDIA CUDA/NVDEC availability.
func (d *HWAccelDetector) testNVIDIA(ctx context.Context) (bool, string) {
	// Try to use nvidia-smi to detect GPU
	cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	deviceName := strings.TrimSpace(strings.Split(string(output), "\n")[0])
	if deviceName == "" {
		return false, ""
	}

	// Verify FFmpeg can use it by testing a quick decode
	testCmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-hide_banner",
		"-hwaccel", "cuda",
		"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
		"-c:v", "h264_nvenc",
		"-t", "0.01",
		"-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		return false, ""
	}

	return true, deviceName
}

// testQSV tests Intel Quick Sync availability.
func (d *HWAccelDetector) testQSV(ctx context.Context) (bool, string) {
	// Test by trying to initialize QSV
	testCmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-hide_banner",
		"-init_hw_device", "qsv=hw",
		"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
		"-vf", "hwupload=extra_hw_frames=64,format=qsv",
		"-c:v", "h264_qsv",
		"-t", "0.01",
		"-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		return false, ""
	}

	return true, "Intel Quick Sync"
}

// testVAAPI tests VA-API availability (Linux).
func (d *HWAccelDetector) testVAAPI(ctx context.Context) (bool, string) {
	if runtime.GOOS != "linux" {
		return false, ""
	}

	// Check for VA-API device
	devices := []string{"/dev/dri/renderD128", "/dev/dri/renderD129"}
	var deviceName string

	for _, device := range devices {
		testCmd := exec.CommandContext(ctx, d.ffmpegPath,
			"-hide_banner",
			"-vaapi_device", device,
			"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
			"-vf", "format=nv12,hwupload",
			"-c:v", "h264_vaapi",
			"-t", "0.01",
			"-f", "null", "-")
		if err := testCmd.Run(); err == nil {
			deviceName = device
			break
		}
	}

	if deviceName == "" {
		return false, ""
	}

	return true, deviceName
}

// testVideoToolbox tests Apple VideoToolbox availability (macOS).
func (d *HWAccelDetector) testVideoToolbox(ctx context.Context) (bool, string) {
	if runtime.GOOS != "darwin" {
		return false, ""
	}

	testCmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-hide_banner",
		"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
		"-c:v", "h264_videotoolbox",
		"-t", "0.01",
		"-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		return false, ""
	}

	return true, "Apple VideoToolbox"
}

// testWindowsHW tests Windows hardware acceleration.
func (d *HWAccelDetector) testWindowsHW(ctx context.Context, accel string) (bool, string) {
	if runtime.GOOS != "windows" {
		return false, ""
	}

	testCmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-hide_banner",
		"-hwaccel", accel,
		"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
		"-t", "0.01",
		"-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		return false, ""
	}

	return true, strings.ToUpper(accel)
}

// testVulkan tests Vulkan availability.
func (d *HWAccelDetector) testVulkan(ctx context.Context) (bool, string) {
	testCmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-hide_banner",
		"-init_hw_device", "vulkan",
		"-f", "lavfi", "-i", "nullsrc=s=320x240:d=0.1",
		"-t", "0.01",
		"-f", "null", "-")
	if err := testCmd.Run(); err != nil {
		return false, ""
	}

	return true, "Vulkan"
}

// getAccelEncoders gets encoders associated with a hardware accelerator.
// For VAAPI, this validates against vainfo to ensure the GPU actually supports encoding.
// Returns both valid encoders and filtered encoders (with reasons why they were excluded).
func (d *HWAccelDetector) getAccelEncoders(ctx context.Context, accel string) ([]string, []FilteredEncoder) {
	var encoders []string
	var filtered []FilteredEncoder

	// Map accelerator to encoder suffixes
	suffixes := map[string][]string{
		"cuda":         {"_nvenc"},
		"nvdec":        {},
		"qsv":          {"_qsv"},
		"vaapi":        {"_vaapi"},
		"videotoolbox": {"_videotoolbox"},
		"amf":          {"_amf"},
	}

	suffixList, ok := suffixes[accel]
	if !ok {
		return encoders, filtered
	}

	// Get all encoders from ffmpeg
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-encoders", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return encoders, filtered
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		for _, suffix := range suffixList {
			if strings.Contains(line, suffix) {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					encoders = append(encoders, parts[1])
				}
			}
		}
	}

	// For VAAPI, validate encoders against vainfo to ensure the GPU actually supports encoding
	if accel == "vaapi" {
		encoders, filtered = d.filterVaapiEncodersByCapability(ctx, encoders)
	}

	return encoders, filtered
}

// filterVaapiEncodersByCapability filters VAAPI encoders to only include those
// that the GPU actually supports for encoding (has VAEntrypointEncSlice).
// FFmpeg may list encoders like vp9_vaapi even when the GPU only supports VP9 decoding.
// Returns valid encoders and filtered encoders with reasons.
func (d *HWAccelDetector) filterVaapiEncodersByCapability(ctx context.Context, encoders []string) ([]string, []FilteredEncoder) {
	// Get actual encoding profiles from vainfo
	supportedCodecs := d.getVaapiEncodingProfiles(ctx)
	if len(supportedCodecs) == 0 {
		// If we can't determine capabilities, return original list
		// (fallback to old behavior - no filtering info available)
		return encoders, nil
	}

	// Filter encoders to only those the GPU can actually encode
	var validEncoders []string
	var filtered []FilteredEncoder
	for _, enc := range encoders {
		// Map encoder name to codec
		codec := vaapiEncoderToCodec(enc)
		if codec == "" {
			continue
		}

		// Check if this codec is in the supported list
		if supportedCodecs[codec] {
			validEncoders = append(validEncoders, enc)
		} else {
			// Track filtered encoder with reason
			filtered = append(filtered, FilteredEncoder{
				Name:   enc,
				Reason: fmt.Sprintf("GPU does not support %s encoding (no VAEntrypointEncSlice)", codec),
			})
		}
	}

	return validEncoders, filtered
}

// getVaapiEncodingProfiles parses vainfo output to determine which codecs
// have encoding capability (VAEntrypointEncSlice).
func (d *HWAccelDetector) getVaapiEncodingProfiles(ctx context.Context) map[string]bool {
	return d.getVaapiProfiles(ctx, "VAEntrypointEncSlice")
}

// getVaapiDecodingProfiles parses vainfo output to determine which codecs
// have decoding capability (VAEntrypointVLD).
func (d *HWAccelDetector) getVaapiDecodingProfiles(ctx context.Context) map[string]bool {
	return d.getVaapiProfiles(ctx, "VAEntrypointVLD")
}

// getVaapiProfiles parses vainfo output to determine which codecs
// have the specified entrypoint capability.
func (d *HWAccelDetector) getVaapiProfiles(ctx context.Context, entrypoint string) map[string]bool {
	supported := make(map[string]bool)

	// Find vainfo binary
	vainfoCmd := getVainfoPath()
	if vainfoCmd == "" {
		// vainfo not available, skip filtering
		return supported
	}

	// Run vainfo to get profile/entrypoint info
	cmd := exec.CommandContext(ctx, vainfoCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// vainfo failed to run
		return supported
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for lines like "VAProfileH264High: VAEntrypointEncSlice" or "VAEntrypointVLD"
		if !strings.Contains(line, entrypoint) {
			continue
		}

		// Extract the profile name
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}

		profile := strings.TrimSpace(parts[0])

		// Map VA profile to codec name
		switch {
		case strings.HasPrefix(profile, "VAProfileH264"):
			supported["h264"] = true
		case strings.HasPrefix(profile, "VAProfileHEVC"), strings.HasPrefix(profile, "VAProfileH265"):
			supported["hevc"] = true
		case strings.HasPrefix(profile, "VAProfileVP9"):
			supported["vp9"] = true
		case strings.HasPrefix(profile, "VAProfileAV1"):
			supported["av1"] = true
		case strings.HasPrefix(profile, "VAProfileJPEG"):
			supported["mjpeg"] = true
		case strings.HasPrefix(profile, "VAProfileVP8"):
			supported["vp8"] = true
		case strings.HasPrefix(profile, "VAProfileMPEG2"):
			supported["mpeg2"] = true
		case strings.HasPrefix(profile, "VAProfileVC1"):
			supported["vc1"] = true
		}
	}

	return supported
}

// vaapiEncoderToCodec maps a VAAPI encoder name to its codec.
func vaapiEncoderToCodec(encoder string) string {
	switch encoder {
	case "h264_vaapi":
		return "h264"
	case "hevc_vaapi":
		return "hevc"
	case "vp9_vaapi":
		return "vp9"
	case "av1_vaapi":
		return "av1"
	case "mjpeg_vaapi":
		return "mjpeg"
	case "vp8_vaapi":
		return "vp8"
	case "mpeg2_vaapi":
		return "mpeg2"
	default:
		return ""
	}
}

// getAccelDecoders gets decoders associated with a hardware accelerator.
// For VAAPI, this returns the codecs that can be hardware-decoded (have VAEntrypointVLD).
func (d *HWAccelDetector) getAccelDecoders(ctx context.Context, accel string) []string {
	var decoders []string

	// For VAAPI, return codecs that have hardware decode capability
	// VAAPI uses hwaccel flag rather than specific decoder binaries,
	// but we report the supported codecs for informational purposes
	if accel == "vaapi" {
		supportedCodecs := d.getVaapiDecodingProfiles(ctx)
		for codec := range supportedCodecs {
			decoders = append(decoders, codec)
		}
		return decoders
	}

	// Map accelerator to decoder suffixes/names
	patterns := map[string][]string{
		"cuda":         {"_cuvid"},
		"nvdec":        {"_cuvid"},
		"qsv":          {"_qsv"},
		"videotoolbox": {}, // VideoToolbox uses hwaccel
	}

	patternList, ok := patterns[accel]
	if !ok || len(patternList) == 0 {
		return decoders
	}

	// Get all decoders from ffmpeg
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-decoders", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return decoders
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		for _, pattern := range patternList {
			if strings.Contains(line, pattern) {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					decoders = append(decoders, parts[1])
				}
			}
		}
	}

	return decoders
}

// GetRecommendedHWAccel returns the best available hardware accelerator.
// Priority order follows m3u-proxy's proven approach: vaapi → nvenc/cuda → qsv
func GetRecommendedHWAccel(accels []HWAccelInfo) *HWAccelInfo {
	// Priority order for hardware acceleration (matches m3u-proxy)
	// vaapi is preferred on Linux due to broad GPU support
	// cuda/nvenc for NVIDIA GPUs
	// qsv for Intel iGPUs
	priority := []HWAccelType{
		HWAccelVAAPI,        // Linux VA-API - broad GPU support
		HWAccelNVENC,        // NVIDIA CUDA/NVENC
		HWAccelQSV,          // Intel Quick Sync
		HWAccelVideoToolbox, // macOS - platform native
		HWAccelD3D11VA,      // Windows 8+
		HWAccelDXVA2,        // Windows (older)
		HWAccelVulkan,       // Cross-platform
	}

	for _, prio := range priority {
		for i := range accels {
			if accels[i].Type == prio && accels[i].Available {
				return &accels[i]
			}
		}
	}

	return nil
}

// SelectBestHWAccel returns the hwaccel type string for the best available accelerator.
// Returns empty string if no hardware acceleration is available.
func SelectBestHWAccel(accels []HWAccelInfo) string {
	recommended := GetRecommendedHWAccel(accels)
	if recommended != nil {
		return string(recommended.Type)
	}
	return ""
}

// HasHWAccel returns true if any hardware acceleration is available.
func (info *BinaryInfo) HasHWAccel(accelType HWAccelType) bool {
	for _, accel := range info.HWAccels {
		if accel.Type == accelType && accel.Available {
			return true
		}
	}
	return false
}

// GetAvailableHWAccels returns all available hardware accelerators.
func (info *BinaryInfo) GetAvailableHWAccels() []HWAccelInfo {
	var available []HWAccelInfo
	for _, accel := range info.HWAccels {
		if accel.Available {
			available = append(available, accel)
		}
	}
	return available
}

// getHWAccels retrieves hardware accelerator information.
func (d *BinaryDetector) getHWAccels(ctx context.Context, ffmpegPath string) ([]HWAccelInfo, error) {
	detector := NewHWAccelDetector(ffmpegPath)
	return detector.Detect(ctx)
}
