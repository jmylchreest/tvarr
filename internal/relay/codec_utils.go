// Package relay provides streaming relay functionality for tvarr.
package relay

// NormalizeCodecName converts encoder names to codec names.
// This handles cases where encoder names (libx265, h264_nvenc) are accidentally
// used instead of codec names (h265, h264).
//
// Examples:
//   - "libx265" -> "h265"
//   - "h264_nvenc" -> "h264"
//   - "libfdk_aac" -> "aac"
//   - "h265" -> "h265" (unchanged)
//   - "hevc" -> "h265" (normalized alias)
func NormalizeCodecName(name string) string {
	switch name {
	// H.264 encoders -> h264
	case "libx264", "h264_nvenc", "h264_qsv", "h264_vaapi", "h264_videotoolbox", "h264_amf", "h264_v4l2m2m":
		return "h264"
	// H.265/HEVC encoders -> h265
	case "libx265", "hevc_nvenc", "hevc_qsv", "hevc_vaapi", "hevc_videotoolbox", "hevc_amf", "hevc_v4l2m2m":
		return "h265"
	// VP9 encoders -> vp9
	case "libvpx-vp9", "vp9_qsv", "vp9_vaapi":
		return "vp9"
	// AV1 encoders -> av1
	case "libaom-av1", "libsvtav1", "av1_nvenc", "av1_qsv", "av1_vaapi", "av1_amf":
		return "av1"
	// AAC encoders -> aac
	case "libfdk_aac", "aac_at":
		return "aac"
	// MP3 encoders -> mp3
	case "libmp3lame":
		return "mp3"
	// Opus encoders -> opus
	case "libopus":
		return "opus"
	// Vorbis encoders -> vorbis
	case "libvorbis":
		return "vorbis"
	// FLAC encoders -> flac
	case "libflac":
		return "flac"
	// AC3 encoders -> ac3
	case "ac3_fixed":
		return "ac3"
	// Common codec aliases
	case "hevc":
		return "h265"
	case "avc":
		return "h264"
	case "h.264":
		return "h264"
	case "h.265":
		return "h265"
	default:
		return name
	}
}

// IsEncoderName returns true if the name appears to be an FFmpeg encoder name
// rather than a codec name.
func IsEncoderName(name string) bool {
	// Check for common encoder prefixes/patterns
	switch name {
	case "libx264", "libx265", "libvpx", "libvpx-vp9", "libaom-av1", "libsvtav1",
		"libfdk_aac", "libmp3lame", "libopus", "libvorbis", "libflac":
		return true
	}

	// Hardware encoder patterns contain underscore with platform suffix
	encoderSuffixes := []string{
		"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf", "_v4l2m2m", "_cuvid",
	}
	for _, suffix := range encoderSuffixes {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return true
		}
	}

	return false
}

// CodecToEncoderFamily returns the base codec family for an encoder name.
// Useful for determining codec compatibility.
func CodecToEncoderFamily(encoder string) string {
	normalized := NormalizeCodecName(encoder)
	switch normalized {
	case "h264", "avc":
		return "h264"
	case "h265", "hevc":
		return "h265"
	case "vp8":
		return "vp8"
	case "vp9":
		return "vp9"
	case "av1":
		return "av1"
	case "aac":
		return "aac"
	case "mp3":
		return "mp3"
	case "ac3":
		return "ac3"
	case "eac3":
		return "eac3"
	case "opus":
		return "opus"
	case "vorbis":
		return "vorbis"
	case "flac":
		return "flac"
	default:
		return normalized
	}
}
