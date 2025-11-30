package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// HWAccelType represents a hardware acceleration type.
type HWAccelType string

const (
	HWAccelNone       HWAccelType = "none"
	HWAccelNVDEC      HWAccelType = "nvdec"      // NVIDIA NVDEC (decode)
	HWAccelNVENC      HWAccelType = "cuda"       // NVIDIA CUDA/NVENC
	HWAccelQSV        HWAccelType = "qsv"        // Intel Quick Sync
	HWAccelVAAPI      HWAccelType = "vaapi"      // VA-API (Linux)
	HWAccelVideoToolbox HWAccelType = "videotoolbox" // macOS
	HWAccelDXVA2      HWAccelType = "dxva2"      // Windows (older)
	HWAccelD3D11VA    HWAccelType = "d3d11va"    // Windows 8+
	HWAccelVulkan     HWAccelType = "vulkan"     // Cross-platform Vulkan
	HWAccelOCL        HWAccelType = "opencl"     // OpenCL
)

// HWAccelInfo contains information about a hardware accelerator.
type HWAccelInfo struct {
	Type        HWAccelType `json:"type"`
	Name        string      `json:"name"`
	Available   bool        `json:"available"`
	DeviceName  string      `json:"device_name,omitempty"`
	Encoders    []string    `json:"encoders,omitempty"`
	Decoders    []string    `json:"decoders,omitempty"`
}

// HWAccelDetector detects available hardware acceleration.
type HWAccelDetector struct {
	ffmpegPath string
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
			// Get encoders for this accelerator
			info.Encoders = d.getAccelEncoders(ctx, accel)
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
func (d *HWAccelDetector) getAccelEncoders(ctx context.Context, accel string) []string {
	var encoders []string

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
		return encoders
	}

	// Get all encoders from ffmpeg
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-encoders", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return encoders
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

	return encoders
}

// getAccelDecoders gets decoders associated with a hardware accelerator.
func (d *HWAccelDetector) getAccelDecoders(ctx context.Context, accel string) []string {
	var decoders []string

	// Map accelerator to decoder suffixes/names
	patterns := map[string][]string{
		"cuda":         {"_cuvid"},
		"nvdec":        {"_cuvid"},
		"qsv":          {"_qsv"},
		"vaapi":        {}, // VAAPI uses hwaccel, not specific decoders
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
func GetRecommendedHWAccel(accels []HWAccelInfo) *HWAccelInfo {
	// Priority order for hardware acceleration
	priority := []HWAccelType{
		HWAccelNVENC,      // NVIDIA - best compatibility
		HWAccelQSV,        // Intel - good compatibility
		HWAccelVideoToolbox, // macOS - platform native
		HWAccelVAAPI,      // Linux - good fallback
		HWAccelD3D11VA,    // Windows
		HWAccelDXVA2,      // Windows (older)
		HWAccelVulkan,     // Cross-platform
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
