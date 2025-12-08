package services

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// HWAccelType represents hardware acceleration type
type HWAccelType string

const (
	HWAccelNone         HWAccelType = "none"
	HWAccelCUDA         HWAccelType = "cuda"
	HWAccelNVDEC        HWAccelType = "nvdec"
	HWAccelQSV          HWAccelType = "qsv"
	HWAccelVAAPI        HWAccelType = "vaapi"
	HWAccelVideoToolbox HWAccelType = "videotoolbox"
	HWAccelDXVA2        HWAccelType = "dxva2"
	HWAccelD3D11VA      HWAccelType = "d3d11va"
	HWAccelVulkan       HWAccelType = "vulkan"
)

// HardwareCapability represents a detected hardware acceleration capability
type HardwareCapability struct {
	Type              HWAccelType `json:"type"`
	Name              string      `json:"name"`
	Available         bool        `json:"available"`
	DeviceName        string      `json:"device_name,omitempty"`
	DevicePath        string      `json:"device_path,omitempty"`
	GpuIndex          int         `json:"gpu_index,omitempty"`
	SupportedEncoders []string    `json:"supported_encoders,omitempty"`
	SupportedDecoders []string    `json:"supported_decoders,omitempty"`
	DetectedAt        time.Time   `json:"detected_at"`
}

// HardwareCapabilities contains all detected hardware capabilities
type HardwareCapabilities struct {
	Capabilities []HardwareCapability `json:"capabilities"`
	DetectedAt   time.Time            `json:"detected_at"`
	Recommended  *HardwareCapability  `json:"recommended,omitempty"`
}

// HardwareDetector detects and caches hardware acceleration capabilities
type HardwareDetector struct {
	ffmpegPath   string
	capabilities *HardwareCapabilities
	mu           sync.RWMutex
}

// NewHardwareDetector creates a new hardware detector
func NewHardwareDetector(ffmpegPath string) *HardwareDetector {
	return &HardwareDetector{
		ffmpegPath: ffmpegPath,
	}
}

// Detect detects all available hardware acceleration capabilities
func (d *HardwareDetector) Detect(ctx context.Context) (*HardwareCapabilities, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	caps := &HardwareCapabilities{
		Capabilities: []HardwareCapability{},
		DetectedAt:   time.Now(),
	}

	// Detect available hwaccels
	hwaccels := d.detectHWAccels(ctx)

	// Detect hardware encoders
	hwEncoders := d.detectHardwareEncoders(ctx)

	// Detect hardware decoders
	hwDecoders := d.detectHardwareDecoders(ctx)

	// Build capabilities for each detected hwaccel
	for _, accel := range hwaccels {
		cap := HardwareCapability{
			Type:       HWAccelType(accel),
			Name:       getHWAccelDisplayName(accel),
			Available:  true,
			DetectedAt: time.Now(),
		}

		// Add matching encoders
		for _, enc := range hwEncoders {
			if matchesHWAccel(enc, accel) {
				cap.SupportedEncoders = append(cap.SupportedEncoders, enc)
			}
		}

		// Add matching decoders
		for _, dec := range hwDecoders {
			if matchesHWAccel(dec, accel) {
				cap.SupportedDecoders = append(cap.SupportedDecoders, dec)
			}
		}

		// Detect device paths for VAAPI
		if accel == "vaapi" {
			cap.DevicePath = d.detectVAAPIDevice()
		}

		// Detect NVIDIA GPU info
		if accel == "cuda" || accel == "nvdec" {
			if gpuInfo := d.detectNVIDIAGPU(ctx); gpuInfo != "" {
				cap.DeviceName = gpuInfo
			}
		}

		caps.Capabilities = append(caps.Capabilities, cap)
	}

	// Set recommended capability (prioritize NVIDIA, then QSV, then VAAPI)
	caps.Recommended = d.selectRecommended(caps.Capabilities)

	d.capabilities = caps

	slog.Info("Hardware capabilities detected",
		slog.Int("count", len(caps.Capabilities)),
		slog.Any("recommended", caps.Recommended))

	return caps, nil
}

// GetCapabilities returns the cached capabilities
func (d *HardwareDetector) GetCapabilities() *HardwareCapabilities {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.capabilities
}

// Refresh re-detects hardware capabilities
func (d *HardwareDetector) Refresh(ctx context.Context) (*HardwareCapabilities, error) {
	return d.Detect(ctx)
}

// detectHWAccels parses the output of ffmpeg -hwaccels
func (d *HardwareDetector) detectHWAccels(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-hide_banner", "-hwaccels")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("Failed to detect hwaccels", slog.Any("error", err))
		return nil
	}

	var accels []string
	lines := strings.Split(string(output), "\n")
	inList := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Hardware acceleration methods:") {
			inList = true
			continue
		}
		if inList && line != "" && !strings.Contains(line, ":") {
			accels = append(accels, line)
		}
	}

	return accels
}

// detectHardwareEncoders parses ffmpeg -encoders for hardware encoders
func (d *HardwareDetector) detectHardwareEncoders(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-hide_banner", "-encoders")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("Failed to detect encoders", slog.Any("error", err))
		return nil
	}

	var encoders []string
	hwSuffixes := []string{"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf", "_mf", "_omx", "_v4l2m2m"}

	lines := strings.Split(string(output), "\n")
	encoderRe := regexp.MustCompile(`^\s*V\.{5}\s+(\S+)\s+`)

	for _, line := range lines {
		matches := encoderRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			encoder := matches[1]
			for _, suffix := range hwSuffixes {
				if strings.HasSuffix(encoder, suffix) {
					encoders = append(encoders, encoder)
					break
				}
			}
		}
	}

	return encoders
}

// detectHardwareDecoders parses ffmpeg -decoders for hardware decoders
func (d *HardwareDetector) detectHardwareDecoders(ctx context.Context) []string {
	cmd := exec.CommandContext(ctx, d.ffmpegPath, "-hide_banner", "-decoders")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("Failed to detect decoders", slog.Any("error", err))
		return nil
	}

	var decoders []string
	hwSuffixes := []string{"_cuvid", "_qsv", "_vaapi", "_videotoolbox", "_mf", "_v4l2m2m"}

	lines := strings.Split(string(output), "\n")
	decoderRe := regexp.MustCompile(`^\s*V\.{5}\s+(\S+)\s+`)

	for _, line := range lines {
		matches := decoderRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			decoder := matches[1]
			for _, suffix := range hwSuffixes {
				if strings.HasSuffix(decoder, suffix) {
					decoders = append(decoders, decoder)
					break
				}
			}
		}
	}

	return decoders
}

// detectVAAPIDevice finds the VAAPI render device
func (d *HardwareDetector) detectVAAPIDevice() string {
	// Check common paths for render devices
	paths := []string{
		"/dev/dri/renderD128",
		"/dev/dri/renderD129",
		"/dev/dri/renderD130",
	}

	for _, path := range paths {
		matches, _ := filepath.Glob(path)
		if len(matches) > 0 {
			return matches[0]
		}
	}

	// Try to find any render device
	matches, _ := filepath.Glob("/dev/dri/renderD*")
	if len(matches) > 0 {
		return matches[0]
	}

	return ""
}

// detectNVIDIAGPU gets NVIDIA GPU info using nvidia-smi
func (d *HardwareDetector) detectNVIDIAGPU(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	name := strings.TrimSpace(string(output))
	// Return only the first GPU if multiple are present
	lines := strings.Split(name, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return name
}

// matchesHWAccel checks if an encoder/decoder matches a hardware acceleration type
func matchesHWAccel(codec, accel string) bool {
	codec = strings.ToLower(codec)
	accel = strings.ToLower(accel)

	switch accel {
	case "cuda", "nvdec":
		return strings.Contains(codec, "nvenc") || strings.Contains(codec, "cuvid")
	case "qsv":
		return strings.HasSuffix(codec, "_qsv")
	case "vaapi":
		return strings.HasSuffix(codec, "_vaapi")
	case "videotoolbox":
		return strings.HasSuffix(codec, "_videotoolbox")
	case "dxva2", "d3d11va":
		return strings.Contains(codec, "_dxva") || strings.Contains(codec, "_d3d11")
	case "vulkan":
		return strings.HasSuffix(codec, "_vulkan")
	}
	return false
}

// getHWAccelDisplayName returns a user-friendly name for the hwaccel type
func getHWAccelDisplayName(accel string) string {
	names := map[string]string{
		"cuda":         "NVIDIA CUDA",
		"nvdec":        "NVIDIA NVDEC",
		"qsv":          "Intel Quick Sync Video",
		"vaapi":        "Video Acceleration API (Linux)",
		"videotoolbox": "Apple VideoToolbox",
		"dxva2":        "DirectX Video Acceleration 2",
		"d3d11va":      "Direct3D 11 Video Acceleration",
		"vulkan":       "Vulkan Video",
	}

	if name, ok := names[accel]; ok {
		return name
	}
	return accel
}

// selectRecommended selects the recommended hardware acceleration
func (d *HardwareDetector) selectRecommended(caps []HardwareCapability) *HardwareCapability {
	// Priority order: NVIDIA (cuda), Intel (qsv), VAAPI, VideoToolbox
	priority := []HWAccelType{HWAccelCUDA, HWAccelNVDEC, HWAccelQSV, HWAccelVAAPI, HWAccelVideoToolbox}

	for _, accelType := range priority {
		for i := range caps {
			if caps[i].Type == accelType && caps[i].Available && len(caps[i].SupportedEncoders) > 0 {
				return &caps[i]
			}
		}
	}

	return nil
}

// IsHardwareAccelerationAvailable returns true if any hardware acceleration is available
func (d *HardwareDetector) IsHardwareAccelerationAvailable() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.capabilities == nil {
		return false
	}

	for _, cap := range d.capabilities.Capabilities {
		if cap.Available && len(cap.SupportedEncoders) > 0 {
			return true
		}
	}

	return false
}

// GetEncodersForType returns available encoders for a specific hardware type
func (d *HardwareDetector) GetEncodersForType(accelType HWAccelType) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.capabilities == nil {
		return nil
	}

	for _, cap := range d.capabilities.Capabilities {
		if cap.Type == accelType {
			return cap.SupportedEncoders
		}
	}

	return nil
}

// GetDecodersForType returns available decoders for a specific hardware type
func (d *HardwareDetector) GetDecodersForType(accelType HWAccelType) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.capabilities == nil {
		return nil
	}

	for _, cap := range d.capabilities.Capabilities {
		if cap.Type == accelType {
			return cap.SupportedDecoders
		}
	}

	return nil
}
