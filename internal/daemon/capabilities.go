// Package daemon implements the tvarr-ffmpegd daemon functionality.
package daemon

import (
	"context"
	"runtime"
	"strings"

	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// CapabilityDetector detects FFmpeg capabilities and converts them to protobuf format.
type CapabilityDetector struct {
	detector *ffmpeg.BinaryDetector
}

// NewCapabilityDetector creates a new capability detector.
func NewCapabilityDetector() *CapabilityDetector {
	return &CapabilityDetector{
		detector: ffmpeg.NewBinaryDetector(),
	}
}

// Detect detects capabilities and returns them in protobuf format.
func (d *CapabilityDetector) Detect(ctx context.Context) (*proto.Capabilities, *ffmpeg.BinaryInfo, error) {
	binInfo, err := d.detector.Detect(ctx)
	if err != nil {
		return nil, nil, err
	}

	gpus := detectGPUs(binInfo.HWAccels)

	// Calculate max GPU jobs as sum of all GPU encode sessions
	maxGPUJobs := int32(0)
	for _, gpu := range gpus {
		maxGPUJobs += gpu.MaxEncodeSessions
	}

	// Max CPU jobs defaults to number of CPU cores
	maxCPUJobs := int32(runtime.NumCPU())

	caps := &proto.Capabilities{
		VideoEncoders:     filterVideoEncoders(binInfo.Encoders),
		VideoDecoders:     filterVideoDecoders(binInfo.Decoders),
		AudioEncoders:     filterAudioEncoders(binInfo.Encoders),
		AudioDecoders:     filterAudioDecoders(binInfo.Decoders),
		MaxConcurrentJobs: 4, // Default, can be overridden by config
		MaxCpuJobs:        maxCPUJobs,
		MaxGpuJobs:        maxGPUJobs,
		MaxProbeJobs:      4, // Default same as MaxConcurrentJobs
		HwAccels:          convertHWAccels(binInfo.HWAccels),
		Gpus:              gpus,
	}

	return caps, binInfo, nil
}

// DetectTypes detects capabilities and returns them in types format.
func (d *CapabilityDetector) DetectTypes(ctx context.Context) (*types.Capabilities, *ffmpeg.BinaryInfo, error) {
	binInfo, err := d.detector.Detect(ctx)
	if err != nil {
		return nil, nil, err
	}

	gpus := detectGPUsToTypes(binInfo.HWAccels)

	// Calculate max GPU jobs as sum of all GPU encode sessions
	maxGPUJobs := 0
	for _, gpu := range gpus {
		maxGPUJobs += gpu.MaxEncodeSessions
	}

	caps := &types.Capabilities{
		VideoEncoders:     filterVideoEncoders(binInfo.Encoders),
		VideoDecoders:     filterVideoDecoders(binInfo.Decoders),
		AudioEncoders:     filterAudioEncoders(binInfo.Encoders),
		AudioDecoders:     filterAudioDecoders(binInfo.Decoders),
		MaxConcurrentJobs: 4,
		MaxCPUJobs:        runtime.NumCPU(),
		MaxGPUJobs:        maxGPUJobs,
		MaxProbeJobs:      4,
		HWAccels:          convertHWAccelsToTypes(binInfo.HWAccels),
		GPUs:              gpus,
	}

	return caps, binInfo, nil
}

// filterVideoEncoders filters encoders to return only video encoders.
func filterVideoEncoders(encoders []string) []string {
	// Common video encoder patterns
	videoPatterns := []string{
		"libx264", "libx265", "h264", "hevc", "av1",
		"nvenc", "vaapi", "qsv", "videotoolbox", "amf",
		"mpeg", "vp8", "vp9", "libvpx", "libaom",
		"prores", "dnxhd", "mjpeg", "gif",
	}

	var result []string
	for _, enc := range encoders {
		for _, pattern := range videoPatterns {
			if strings.Contains(strings.ToLower(enc), pattern) {
				result = append(result, enc)
				break
			}
		}
	}
	return result
}

// filterVideoDecoders filters decoders to return only video decoders.
func filterVideoDecoders(decoders []string) []string {
	videoPatterns := []string{
		"h264", "hevc", "av1", "mpeg", "vp8", "vp9",
		"cuvid", "qsv", "vaapi",
		"prores", "dnxhd", "mjpeg", "gif",
	}

	var result []string
	for _, dec := range decoders {
		for _, pattern := range videoPatterns {
			if strings.Contains(strings.ToLower(dec), pattern) {
				result = append(result, dec)
				break
			}
		}
	}
	return result
}

// filterAudioEncoders filters encoders to return only audio encoders.
func filterAudioEncoders(encoders []string) []string {
	audioPatterns := []string{
		"aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3",
		"libfdk", "libmp3lame", "libopus", "libvorbis",
		"pcm", "alac", "dts",
	}

	var result []string
	for _, enc := range encoders {
		for _, pattern := range audioPatterns {
			if strings.Contains(strings.ToLower(enc), pattern) {
				result = append(result, enc)
				break
			}
		}
	}
	return result
}

// filterAudioDecoders filters decoders to return only audio decoders.
func filterAudioDecoders(decoders []string) []string {
	audioPatterns := []string{
		"aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3",
		"pcm", "alac", "dts", "truehd",
	}

	var result []string
	for _, dec := range decoders {
		for _, pattern := range audioPatterns {
			if strings.Contains(strings.ToLower(dec), pattern) {
				result = append(result, dec)
				break
			}
		}
	}
	return result
}

// convertHWAccels converts pkg/ffmpeg HWAccelInfo to proto format.
func convertHWAccels(accels []ffmpeg.HWAccelInfo) []*proto.HWAccelInfo {
	var result []*proto.HWAccelInfo
	for _, accel := range accels {
		// Convert filtered encoders to proto format
		var filteredEncoders []*proto.FilteredEncoder
		for _, fe := range accel.FilteredEncoders {
			filteredEncoders = append(filteredEncoders, &proto.FilteredEncoder{
				Name:   fe.Name,
				Reason: fe.Reason,
			})
		}

		result = append(result, &proto.HWAccelInfo{
			Type:             string(accel.Type),
			Device:           accel.DeviceName,
			Available:        accel.Available,
			HwEncoders:       accel.Encoders,
			HwDecoders:       accel.Decoders,
			FilteredEncoders: filteredEncoders,
		})
	}
	return result
}

// convertHWAccelsToTypes converts pkg/ffmpeg HWAccelInfo to types format.
func convertHWAccelsToTypes(accels []ffmpeg.HWAccelInfo) []types.HWAccelInfo {
	var result []types.HWAccelInfo
	for _, accel := range accels {
		// Convert filtered encoders to types format
		var filteredEncoders []types.FilteredEncoder
		for _, fe := range accel.FilteredEncoders {
			filteredEncoders = append(filteredEncoders, types.FilteredEncoder{
				Name:   fe.Name,
				Reason: fe.Reason,
			})
		}

		result = append(result, types.HWAccelInfo{
			Type:             types.HWAccelType(accel.Type),
			Device:           accel.DeviceName,
			Available:        accel.Available,
			Encoders:         accel.Encoders,
			Decoders:         accel.Decoders,
			FilteredEncoders: filteredEncoders,
		})
	}
	return result
}

// detectGPUs detects GPUs from hardware accelerator info.
func detectGPUs(accels []ffmpeg.HWAccelInfo) []*proto.GPUInfo {
	var gpus []*proto.GPUInfo
	gpuIndex := 0

	for _, accel := range accels {
		if !accel.Available {
			continue
		}

		// Only create GPU entries for accelerators that represent GPUs
		switch accel.Type {
		case ffmpeg.HWAccelNVENC, ffmpeg.HWAccelNVDEC:
			gpu := &proto.GPUInfo{
				Index:             int32(gpuIndex),
				Name:              accel.DeviceName,
				GpuClass:          detectGPUClass(accel.DeviceName),
				MaxEncodeSessions: int32(getMaxEncodeSessions(detectGPUClass(accel.DeviceName))),
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelVAAPI:
			gpu := &proto.GPUInfo{
				Index:             int32(gpuIndex),
				Name:              "VA-API GPU",
				GpuClass:          proto.GPUClass_GPU_CLASS_INTEGRATED,
				MaxEncodeSessions: 2, // Conservative for integrated
			}
			if accel.DeviceName != "" {
				gpu.Name = "VA-API (" + accel.DeviceName + ")"
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelQSV:
			gpu := &proto.GPUInfo{
				Index:             int32(gpuIndex),
				Name:              "Intel Quick Sync",
				GpuClass:          proto.GPUClass_GPU_CLASS_INTEGRATED,
				MaxEncodeSessions: 4, // Intel QSV typically supports more sessions
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelVideoToolbox:
			gpu := &proto.GPUInfo{
				Index:             int32(gpuIndex),
				Name:              "Apple VideoToolbox",
				GpuClass:          proto.GPUClass_GPU_CLASS_INTEGRATED,
				MaxEncodeSessions: 8, // Apple Silicon typically has high limits
			}
			gpus = append(gpus, gpu)
			gpuIndex++
		}
	}

	return gpus
}

// detectGPUsToTypes detects GPUs and returns types format.
func detectGPUsToTypes(accels []ffmpeg.HWAccelInfo) []types.GPUInfo {
	var gpus []types.GPUInfo
	gpuIndex := 0

	for _, accel := range accels {
		if !accel.Available {
			continue
		}

		switch accel.Type {
		case ffmpeg.HWAccelNVENC, ffmpeg.HWAccelNVDEC:
			gpuClass := detectGPUClassTypes(accel.DeviceName)
			gpu := types.GPUInfo{
				Index:             gpuIndex,
				Name:              accel.DeviceName,
				Class:             gpuClass,
				MaxEncodeSessions: gpuClass.DefaultMaxEncodeSessions(),
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelVAAPI:
			gpu := types.GPUInfo{
				Index:             gpuIndex,
				Name:              "VA-API GPU",
				Class:             types.GPUClassIntegrated,
				MaxEncodeSessions: 2,
			}
			if accel.DeviceName != "" {
				gpu.Name = "VA-API (" + accel.DeviceName + ")"
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelQSV:
			gpu := types.GPUInfo{
				Index:             gpuIndex,
				Name:              "Intel Quick Sync",
				Class:             types.GPUClassIntegrated,
				MaxEncodeSessions: 4,
			}
			gpus = append(gpus, gpu)
			gpuIndex++

		case ffmpeg.HWAccelVideoToolbox:
			gpu := types.GPUInfo{
				Index:             gpuIndex,
				Name:              "Apple VideoToolbox",
				Class:             types.GPUClassIntegrated,
				MaxEncodeSessions: 8,
			}
			gpus = append(gpus, gpu)
			gpuIndex++
		}
	}

	return gpus
}

// detectGPUClass detects the GPU class from its name.
func detectGPUClass(name string) proto.GPUClass {
	nameLower := strings.ToLower(name)

	// Datacenter GPUs
	if strings.Contains(nameLower, "tesla") ||
		strings.Contains(nameLower, "a100") ||
		strings.Contains(nameLower, "h100") ||
		strings.Contains(nameLower, "a40") ||
		strings.Contains(nameLower, "l40") {
		return proto.GPUClass_GPU_CLASS_DATACENTER
	}

	// Professional GPUs
	if strings.Contains(nameLower, "quadro") ||
		strings.Contains(nameLower, "rtx a") ||
		strings.Contains(nameLower, "pro ") ||
		strings.Contains(nameLower, "wx") {
		return proto.GPUClass_GPU_CLASS_PROFESSIONAL
	}

	// Consumer GPUs (GeForce, Radeon)
	if strings.Contains(nameLower, "geforce") ||
		strings.Contains(nameLower, "gtx") ||
		strings.Contains(nameLower, "rtx") ||
		strings.Contains(nameLower, "radeon") {
		return proto.GPUClass_GPU_CLASS_CONSUMER
	}

	return proto.GPUClass_GPU_CLASS_UNKNOWN
}

// detectGPUClassTypes detects the GPU class for types format.
func detectGPUClassTypes(name string) types.GPUClass {
	nameLower := strings.ToLower(name)

	if strings.Contains(nameLower, "tesla") ||
		strings.Contains(nameLower, "a100") ||
		strings.Contains(nameLower, "h100") ||
		strings.Contains(nameLower, "a40") ||
		strings.Contains(nameLower, "l40") {
		return types.GPUClassDatacenter
	}

	if strings.Contains(nameLower, "quadro") ||
		strings.Contains(nameLower, "rtx a") ||
		strings.Contains(nameLower, "pro ") ||
		strings.Contains(nameLower, "wx") {
		return types.GPUClassProfessional
	}

	if strings.Contains(nameLower, "geforce") ||
		strings.Contains(nameLower, "gtx") ||
		strings.Contains(nameLower, "rtx") ||
		strings.Contains(nameLower, "radeon") {
		return types.GPUClassConsumer
	}

	return types.GPUClassUnknown
}

// getMaxEncodeSessions returns the max encode sessions for a GPU class.
func getMaxEncodeSessions(class proto.GPUClass) int {
	switch class {
	case proto.GPUClass_GPU_CLASS_CONSUMER:
		return 5 // NVIDIA consumer cards
	case proto.GPUClass_GPU_CLASS_PROFESSIONAL:
		return 32 // Quadro/Pro cards
	case proto.GPUClass_GPU_CLASS_DATACENTER:
		return 0 // Unlimited
	case proto.GPUClass_GPU_CLASS_INTEGRATED:
		return 2
	default:
		return 3 // Conservative default
	}
}
