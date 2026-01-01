// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"strings"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// StreamTarget represents the complete delivery specification for a client.
// It combines video codec, audio codec, and container format into a single
// unified target determined at client connection time.
type StreamTarget struct {
	VideoCodec      string // h264, h265, av1, vp9 (normalized, never "copy")
	AudioCodec      string // aac, ac3, opus, mp3 (normalized, never "copy")
	ContainerFormat string // hls-fmp4, hls-ts, dash, mpegts
}

// DaemonOutputFormat returns the container format for daemon FFmpeg output.
// Returns "fmp4" or "mpegts".
func (t StreamTarget) DaemonOutputFormat() string {
	switch t.ContainerFormat {
	case "hls-fmp4", "dash", "fmp4":
		return "fmp4"
	case "hls-ts", "mpegts", "ts":
		return "mpegts"
	default:
		// Default based on codec - AV1/VP9 require fMP4
		if codec.VideoRequiresFMP4(t.VideoCodec) || codec.AudioRequiresFMP4(t.AudioCodec) {
			return "fmp4"
		}
		return "mpegts"
	}
}

// CodecFormatCompatibility maps video codecs to their compatible container formats.
// This is the authoritative source for codec/format compatibility decisions.
var CodecFormatCompatibility = map[string][]string{
	"av1":  {"hls-fmp4", "dash", "fmp4"},                     // AV1 requires fMP4
	"vp9":  {"hls-fmp4", "dash", "fmp4"},                     // VP9 requires fMP4
	"h264": {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // H.264 works everywhere
	"h265": {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // H.265 works everywhere
	"opus": {"hls-fmp4", "dash", "fmp4"},                     // Opus requires fMP4
	"aac":  {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // AAC works everywhere
	"ac3":  {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // AC3 works everywhere
	"eac3": {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // EAC3 works everywhere
	"mp3":  {"hls-fmp4", "hls-ts", "dash", "mpegts", "fmp4"}, // MP3 works everywhere
}

// IsFormatCompatibleWithCodec checks if a format is compatible with a codec.
func IsFormatCompatibleWithCodec(codecName, format string) bool {
	codecName = codec.Normalize(codecName)
	formats, ok := CodecFormatCompatibility[codecName]
	if !ok {
		// Unknown codec - assume fMP4 formats are safe
		return strings.Contains(format, "fmp4") || format == "dash"
	}
	for _, f := range formats {
		if f == format {
			return true
		}
	}
	return false
}

// ValidateAndFix ensures codec/format compatibility, fixing invalid combinations.
// Returns the fixed target with explanatory changes.
func ValidateAndFix(target StreamTarget) StreamTarget {
	result := target

	// Normalize codecs
	result.VideoCodec = codec.Normalize(result.VideoCodec)
	result.AudioCodec = codec.Normalize(result.AudioCodec)

	// Handle empty format - default to hls-fmp4 for maximum compatibility
	if result.ContainerFormat == "" {
		result.ContainerFormat = "hls-fmp4"
	}

	// Check video codec compatibility
	if !IsFormatCompatibleWithCodec(result.VideoCodec, result.ContainerFormat) {
		// Force to fMP4 for incompatible codecs
		switch result.ContainerFormat {
		case "hls-ts":
			result.ContainerFormat = "hls-fmp4"
		case "mpegts":
			result.ContainerFormat = "fmp4"
		}
	}

	// Check audio codec compatibility
	if !IsFormatCompatibleWithCodec(result.AudioCodec, result.ContainerFormat) {
		// Force to fMP4 for incompatible codecs
		switch result.ContainerFormat {
		case "hls-ts":
			result.ContainerFormat = "hls-fmp4"
		case "mpegts":
			result.ContainerFormat = "fmp4"
		}
	}

	return result
}

// NewStreamTarget creates a StreamTarget from individual components.
func NewStreamTarget(videoCodec, audioCodec, containerFormat string) StreamTarget {
	return ValidateAndFix(StreamTarget{
		VideoCodec:      codec.Normalize(videoCodec),
		AudioCodec:      codec.Normalize(audioCodec),
		ContainerFormat: containerFormat,
	})
}

