// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"github.com/jmylchreest/tvarr/internal/codec"
)

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
//
// Deprecated: Use codec.Normalize instead.
func NormalizeCodecName(name string) string {
	return codec.Normalize(name)
}

// IsEncoderName returns true if the name appears to be an FFmpeg encoder name
// rather than a codec name.
//
// Deprecated: Use codec.IsEncoder instead.
func IsEncoderName(name string) bool {
	return codec.IsEncoder(name)
}

// CodecToEncoderFamily returns the base codec family for an encoder name.
// Useful for determining codec compatibility.
//
// Deprecated: Use codec.Normalize instead.
func CodecToEncoderFamily(encoder string) string {
	return codec.Normalize(encoder)
}

// IsAudioCodecDemuxable returns true if the audio codec can be demuxed by mediacommon.
// Some audio codecs (e.g., E-AC3) are not supported by the mediacommon MPEG-TS demuxer
// and require direct URL input to FFmpeg.
//
// Deprecated: Use codec.IsAudioDemuxable instead.
func IsAudioCodecDemuxable(codecName string) bool {
	return codec.IsAudioDemuxable(codecName)
}

// IsVideoCodecDemuxable returns true if the video codec can be demuxed by mediacommon.
// Some video codecs are not supported by the mediacommon MPEG-TS demuxer
// and require direct URL input to FFmpeg.
//
// Deprecated: Use codec.IsVideoDemuxable instead.
func IsVideoCodecDemuxable(codecName string) bool {
	return codec.IsVideoDemuxable(codecName)
}
