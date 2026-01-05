// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"context"
	"log/slog"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/codec"
	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/shirou/gopsutil/v4/cpu"
)

// EncoderSelector selects the best encoder for a target codec based on
// available hardware acceleration capabilities.
type EncoderSelector struct {
	binInfo *ffmpeg.BinaryInfo
}

// NewEncoderSelector creates a new encoder selector.
func NewEncoderSelector(binInfo *ffmpeg.BinaryInfo) *EncoderSelector {
	return &EncoderSelector{binInfo: binInfo}
}

// SelectVideoEncoder returns the best video encoder for the target codec.
// It prefers hardware encoders in this order: VAAPI > CUDA/NVENC > QSV > VideoToolbox > Software.
// Returns the encoder name, hwaccel type, and device path (empty for software encoding).
func (s *EncoderSelector) SelectVideoEncoder(targetCodec string) (encoder string, hwAccel string, hwDevice string) {
	return s.SelectVideoEncoderWithPreference(targetCodec, "")
}

// SelectVideoEncoderWithPreference returns the best video encoder for the target codec,
// optionally preferring a specific hardware accelerator if specified.
// If preferredHWAccel is empty, it auto-selects based on availability.
// Returns the encoder name, hwaccel type, and device path (empty for software encoding).
func (s *EncoderSelector) SelectVideoEncoderWithPreference(targetCodec, preferredHWAccel string) (encoder string, hwAccel string, hwDevice string) {
	// Normalize target codec using the codec package
	targetCodec = codec.Normalize(targetCodec)
	preferredHWAccel = strings.ToLower(strings.TrimSpace(preferredHWAccel))

	// Handle copy/passthrough - return "copy" to tell FFmpeg to pass through video
	if targetCodec == "copy" || targetCodec == "" {
		slog.Debug("video passthrough requested",
			slog.String("target_codec", targetCodec),
		)
		return "copy", "", ""
	}

	slog.Debug("encoder selection starting",
		slog.String("target_codec", targetCodec),
		slog.String("preferred_hwaccel", preferredHWAccel),
	)

	// Get hardware encoder mappings for this codec
	hwEncoders := getHWEncoders(targetCodec)
	if len(hwEncoders) == 0 {
		// Unknown codec, fall back to software
		slog.Debug("no hardware encoders known for codec",
			slog.String("target_codec", targetCodec),
		)
		return getSoftwareEncoder(targetCodec), "", ""
	}

	// If a preferred hwaccel is specified and available, try to use it first
	// "auto" and "none" are special values that bypass preference checking
	if preferredHWAccel != "" && preferredHWAccel != "auto" && preferredHWAccel != "none" {
		if enc, ok := hwEncoders[preferredHWAccel]; ok {
			for _, accel := range s.binInfo.HWAccels {
				accelType := strings.ToLower(string(accel.Type))
				if accelType == preferredHWAccel && accel.Available {
					// Check if this specific encoder is in the GPU-validated encoder list
					encoderSupported := slices.Contains(accel.Encoders, enc)

					slog.Debug("checking preferred hwaccel encoder support",
						slog.String("hwaccel", preferredHWAccel),
						slog.String("encoder", enc),
						slog.Bool("encoder_supported", encoderSupported),
						slog.Any("gpu_encoders", accel.Encoders),
						slog.String("device", accel.DeviceName),
					)

					if encoderSupported {
						return enc, preferredHWAccel, accel.DeviceName
					}
				}
			}
			slog.Debug("preferred hwaccel not available or encoder not supported",
				slog.String("hwaccel", preferredHWAccel),
				slog.String("encoder", enc),
			)
		} else {
			slog.Debug("preferred hwaccel not supported for codec",
				slog.String("hwaccel", preferredHWAccel),
				slog.String("target_codec", targetCodec),
			)
		}
	}

	// If "none" was explicitly requested, skip hardware and use software
	if preferredHWAccel == "none" {
		slog.Debug("software encoding explicitly requested",
			slog.String("target_codec", targetCodec),
		)
		return getSoftwareEncoder(targetCodec), "", ""
	}

	// Auto-select: check each hardware accelerator in priority order
	// Priority: vaapi > cuda > qsv > videotoolbox > amf
	priorityOrder := []string{"vaapi", "cuda", "qsv", "videotoolbox", "amf"}

	for _, hwType := range priorityOrder {
		if enc, ok := hwEncoders[hwType]; ok {
			// Check if the hwaccel is available and supports this encoder
			for _, accel := range s.binInfo.HWAccels {
				accelType := strings.ToLower(string(accel.Type))
				if accelType == hwType && accel.Available {
					// Check if this specific encoder is in the GPU-validated encoder list
					// This is critical because FFmpeg may list encoders (e.g., vp9_vaapi)
					// even when the GPU only supports decoding, not encoding
					encoderSupported := slices.Contains(accel.Encoders, enc)

					slog.Debug("checking hwaccel encoder support",
						slog.String("hwaccel", hwType),
						slog.String("encoder", enc),
						slog.Bool("encoder_supported", encoderSupported),
						slog.Any("gpu_encoders", accel.Encoders),
						slog.String("device", accel.DeviceName),
					)

					if encoderSupported {
						return enc, hwType, accel.DeviceName
					}
				}
			}
			slog.Debug("hwaccel not available or encoder not supported by GPU",
				slog.String("hwaccel", hwType),
				slog.String("encoder", enc),
			)
		}
	}

	// No hardware encoder available, fall back to software
	slog.Debug("falling back to software encoder",
		slog.String("target_codec", targetCodec),
		slog.String("encoder", getSoftwareEncoder(targetCodec)),
	)
	return getSoftwareEncoder(targetCodec), "", ""
}

// SelectAudioEncoder returns the best audio encoder for the target codec.
// Audio encoding is typically done in software.
func (s *EncoderSelector) SelectAudioEncoder(targetCodec string) string {
	targetCodec = codec.Normalize(targetCodec)

	// Handle copy/passthrough - return "copy" to tell FFmpeg to pass through audio
	if targetCodec == "copy" || targetCodec == "" {
		return "copy"
	}

	// Map target codec to encoder
	encoderMap := map[string][]string{
		"aac":  {"aac", "libfdk_aac"},
		"ac3":  {"ac3"},
		"eac3": {"eac3"},
		"mp3":  {"libmp3lame", "mp3"},
		"opus": {"libopus", "opus"},
	}

	candidates, ok := encoderMap[targetCodec]
	if !ok {
		return "aac" // Default to AAC
	}

	// Return first available encoder
	for _, enc := range candidates {
		if s.binInfo.HasEncoder(enc) {
			return enc
		}
	}

	// Return first candidate even if not explicitly available
	// (FFmpeg may still support it)
	return candidates[0]
}

// getHWEncoders returns a map of hwaccel type -> encoder name for a given codec.
func getHWEncoders(codec string) map[string]string {
	switch codec {
	case "h264":
		return map[string]string{
			"vaapi":        "h264_vaapi",
			"cuda":         "h264_nvenc",
			"qsv":          "h264_qsv",
			"videotoolbox": "h264_videotoolbox",
			"amf":          "h264_amf",
		}
	case "h265":
		return map[string]string{
			"vaapi":        "hevc_vaapi",
			"cuda":         "hevc_nvenc",
			"qsv":          "hevc_qsv",
			"videotoolbox": "hevc_videotoolbox",
			"amf":          "hevc_amf",
		}
	case "vp9":
		return map[string]string{
			"vaapi": "vp9_vaapi",
			"qsv":   "vp9_qsv",
		}
	case "av1":
		return map[string]string{
			"vaapi": "av1_vaapi",
			"cuda":  "av1_nvenc",
			"qsv":   "av1_qsv",
		}
	}
	return nil
}

// getSoftwareEncoder returns the software encoder for a codec.
func getSoftwareEncoder(codec string) string {
	switch codec {
	case "h264":
		return "libx264"
	case "h265":
		return "libx265"
	case "vp9":
		return "libvpx-vp9"
	case "av1":
		return "libaom-av1"
	default:
		return "libx264" // Default fallback
	}
}

// IsHardwareEncoder returns true if the encoder is a hardware encoder.
func IsHardwareEncoder(encoder string) bool {
	encoder = strings.ToLower(encoder)
	hwSuffixes := []string{"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf", "_mf", "_omx", "_v4l2m2m", "_cuvid"}
	for _, suffix := range hwSuffixes {
		if strings.HasSuffix(encoder, suffix) {
			return true
		}
	}
	return false
}

// ApplyEncoderOverride checks if any encoder override matches the current conditions
// and returns the override's target encoder if matched, or the original encoder otherwise.
//
// Parameters:
//   - codecType: "video" or "audio"
//   - targetCodec: the target codec (h265, aac, etc.)
//   - selectedEncoder: the encoder that was auto-selected
//   - hwAccel: current hardware accelerator (vaapi, cuda, etc.) - only relevant for video
//   - cpuInfo: CPU information string for regex matching
//   - overrides: list of enabled encoder overrides
//
// Returns the encoder to use (either from override or original).
func (s *EncoderSelector) ApplyEncoderOverride(
	codecType string,
	targetCodec string,
	selectedEncoder string,
	hwAccel string,
	cpuInfo string,
	overrides []*proto.EncoderOverride,
) string {
	if len(overrides) == 0 {
		return selectedEncoder
	}

	// Normalize inputs
	codecType = strings.ToLower(strings.TrimSpace(codecType))
	targetCodec = codec.Normalize(targetCodec)
	hwAccel = strings.ToLower(strings.TrimSpace(hwAccel))

	slog.Debug("checking encoder overrides",
		slog.String("codec_type", codecType),
		slog.String("target_codec", targetCodec),
		slog.String("selected_encoder", selectedEncoder),
		slog.String("hw_accel", hwAccel),
		slog.String("cpu_info", cpuInfo),
		slog.Int("override_count", len(overrides)),
	)

	// Filter to matching codec_type and sort by priority (highest first)
	var matching []*proto.EncoderOverride
	for _, override := range overrides {
		if strings.ToLower(override.CodecType) == codecType {
			matching = append(matching, override)
		}
	}

	if len(matching) == 0 {
		slog.Debug("no overrides match codec type",
			slog.String("codec_type", codecType),
		)
		return selectedEncoder
	}

	// Sort by priority descending
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].Priority > matching[j].Priority
	})

	// Check each override in priority order
	for _, override := range matching {
		sourceCodec := codec.Normalize(override.SourceCodec)

		slog.Debug("evaluating encoder override",
			slog.String("source_codec", override.SourceCodec),
			slog.String("target_encoder", override.TargetEncoder),
			slog.String("hw_accel_match", override.HwAccelMatch),
			slog.String("cpu_match", override.CpuMatch),
			slog.Int("priority", int(override.Priority)),
		)

		// Check source_codec match
		if sourceCodec != targetCodec {
			slog.Debug("override skipped: source_codec mismatch",
				slog.String("override_source", sourceCodec),
				slog.String("target_codec", targetCodec),
			)
			continue
		}

		// Check hw_accel_match (empty matches all)
		if override.HwAccelMatch != "" {
			hwAccelMatch := strings.ToLower(strings.TrimSpace(override.HwAccelMatch))
			if hwAccelMatch != hwAccel {
				slog.Debug("override skipped: hw_accel_match mismatch",
					slog.String("override_hwaccel", hwAccelMatch),
					slog.String("current_hwaccel", hwAccel),
				)
				continue
			}
		}

		// Check cpu_match regex (empty matches all)
		if override.CpuMatch != "" {
			matched, err := regexp.MatchString(override.CpuMatch, cpuInfo)
			if err != nil {
				slog.Warn("invalid cpu_match regex in encoder override",
					slog.String("pattern", override.CpuMatch),
					slog.String("error", err.Error()),
				)
				continue
			}
			if !matched {
				slog.Debug("override skipped: cpu_match regex did not match",
					slog.String("pattern", override.CpuMatch),
					slog.String("cpu_info", cpuInfo),
				)
				continue
			}
		}

		// All conditions matched - use this override
		slog.Info("applying encoder override",
			slog.String("codec_type", codecType),
			slog.String("source_codec", targetCodec),
			slog.String("original_encoder", selectedEncoder),
			slog.String("override_encoder", override.TargetEncoder),
			slog.String("hw_accel_match", override.HwAccelMatch),
			slog.String("cpu_match", override.CpuMatch),
			slog.Int("priority", int(override.Priority)),
		)
		return override.TargetEncoder
	}

	slog.Debug("no encoder override matched",
		slog.String("codec_type", codecType),
		slog.String("target_codec", targetCodec),
		slog.String("using_encoder", selectedEncoder),
	)
	return selectedEncoder
}

// cpuInfoCache caches CPU info to avoid repeated system calls
var (
	cpuInfoCache     string
	cpuInfoCacheOnce sync.Once
)

// getCPUInfo returns a string containing CPU information for regex matching.
// Uses gopsutil to get CPU vendor and model name in a cross-platform way.
// The result is cached since CPU info doesn't change at runtime.
func getCPUInfo() string {
	cpuInfoCacheOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		infos, err := cpu.InfoWithContext(ctx)
		if err != nil || len(infos) == 0 {
			slog.Debug("failed to get CPU info from gopsutil, using fallback",
				slog.String("error", err.Error()),
			)
			cpuInfoCache = runtime.GOARCH + " " + runtime.GOOS
			return
		}

		// Build a string with all relevant CPU info for regex matching
		// Include vendor ID, model name, and any other relevant fields
		var parts []string
		for _, info := range infos {
			if info.VendorID != "" {
				parts = append(parts, info.VendorID)
			}
			if info.ModelName != "" {
				parts = append(parts, info.ModelName)
			}
		}

		if len(parts) == 0 {
			cpuInfoCache = runtime.GOARCH + " " + runtime.GOOS
		} else {
			cpuInfoCache = strings.Join(parts, " ")
		}

		slog.Debug("cached CPU info for encoder override matching",
			slog.String("cpu_info", cpuInfoCache),
		)
	})

	return cpuInfoCache
}
