// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"strings"

	"github.com/jmylchreest/tvarr/internal/codec"
	"github.com/jmylchreest/tvarr/internal/models"
)

// StreamTarget represents the complete delivery specification for a client.
// It combines video codec, audio codec, and container format into a single
// unified target determined at client connection time.
type StreamTarget struct {
	VideoCodec      string // h264, h265, av1, vp9 (normalized, never "copy")
	AudioCodec      string // aac, ac3, opus, mp3 (normalized, never "copy")
	ContainerFormat string // hls-fmp4, hls-ts, dash, mpegts
}

// String returns a human-readable representation of the target.
func (t StreamTarget) String() string {
	return t.VideoCodec + "/" + t.AudioCodec + "/" + t.ContainerFormat
}

// VariantKey returns the key used for buffer variant keying.
// This matches the CodecVariant format "video/audio".
func (t StreamTarget) VariantKey() string {
	return t.VideoCodec + "/" + t.AudioCodec
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

// RequiresTranscode returns true if the source needs transcoding to reach this target.
func (t StreamTarget) RequiresTranscode(source StreamTarget) bool {
	// Video needs transcode if codecs don't match
	if !codec.VideoMatch(source.VideoCodec, t.VideoCodec) {
		return true
	}
	// Audio needs transcode if codecs don't match
	if !codec.AudioMatch(source.AudioCodec, t.AudioCodec) {
		return true
	}
	return false
}

// IsValid returns true if the target has all required fields.
func (t StreamTarget) IsValid() bool {
	return t.VideoCodec != "" && t.AudioCodec != "" && t.ContainerFormat != ""
}

// NeedsVideoTranscode returns true if video transcoding is needed.
func (t StreamTarget) NeedsVideoTranscode(sourceVideoCodec string) bool {
	return !codec.VideoMatch(sourceVideoCodec, t.VideoCodec)
}

// NeedsAudioTranscode returns true if audio transcoding is needed.
func (t StreamTarget) NeedsAudioTranscode(sourceAudioCodec string) bool {
	return !codec.AudioMatch(sourceAudioCodec, t.AudioCodec)
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

// ResolveStreamTarget determines the target from client capabilities or encoding profile.
// This is the main decision function called when a client connects.
//
// Parameters:
//   - source: The source stream codecs
//   - clientCaps: Client capabilities from detection (may be nil)
//   - profile: Encoding profile (may be nil if passthrough desired)
//
// Returns the resolved StreamTarget with normalized codecs and valid format.
func ResolveStreamTarget(source StreamTarget, clientCaps *ClientCapabilities, profile *models.EncodingProfile) StreamTarget {
	target := StreamTarget{}

	// Determine container format
	if clientCaps != nil && clientCaps.PreferredFormat != "" {
		target.ContainerFormat = clientCaps.PreferredFormat
	} else {
		// Default to hls-fmp4 for modern compatibility
		target.ContainerFormat = "hls-fmp4"
	}

	// Determine target codecs from client detection
	if clientCaps != nil && len(clientCaps.AcceptedVideoCodecs) > 0 {
		// Check if source video is accepted
		sourceVideoNorm := codec.Normalize(source.VideoCodec)
		videoAccepted := false
		for _, accepted := range clientCaps.AcceptedVideoCodecs {
			if codec.VideoMatch(sourceVideoNorm, accepted) {
				videoAccepted = true
				target.VideoCodec = sourceVideoNorm
				break
			}
		}
		if !videoAccepted && clientCaps.PreferredVideoCodec != "" {
			target.VideoCodec = codec.Normalize(clientCaps.PreferredVideoCodec)
		} else if !videoAccepted {
			// Fallback to h264 if client doesn't accept source and no preference
			target.VideoCodec = "h264"
		}
	}

	if clientCaps != nil && len(clientCaps.AcceptedAudioCodecs) > 0 {
		// Check if source audio is accepted
		sourceAudioNorm := codec.Normalize(source.AudioCodec)
		audioAccepted := false
		for _, accepted := range clientCaps.AcceptedAudioCodecs {
			if codec.AudioMatch(sourceAudioNorm, accepted) {
				audioAccepted = true
				target.AudioCodec = sourceAudioNorm
				break
			}
		}
		if !audioAccepted && clientCaps.PreferredAudioCodec != "" {
			target.AudioCodec = codec.Normalize(clientCaps.PreferredAudioCodec)
		} else if !audioAccepted {
			// Fallback to aac if client doesn't accept source and no preference
			target.AudioCodec = "aac"
		}
	}

	// If no client detection, fall back to encoding profile
	if target.VideoCodec == "" || target.AudioCodec == "" {
		if profile != nil {
			if target.VideoCodec == "" {
				target.VideoCodec = string(profile.TargetVideoCodec)
			}
			if target.AudioCodec == "" {
				target.AudioCodec = string(profile.TargetAudioCodec)
			}
		}
	}

	// Ultimate fallback: use source codecs (passthrough)
	if target.VideoCodec == "" {
		target.VideoCodec = codec.Normalize(source.VideoCodec)
	}
	if target.AudioCodec == "" {
		target.AudioCodec = codec.Normalize(source.AudioCodec)
	}

	// Validate and fix any incompatible combinations
	return ValidateAndFix(target)
}

// NewStreamTarget creates a StreamTarget from individual components.
func NewStreamTarget(videoCodec, audioCodec, containerFormat string) StreamTarget {
	return ValidateAndFix(StreamTarget{
		VideoCodec:      codec.Normalize(videoCodec),
		AudioCodec:      codec.Normalize(audioCodec),
		ContainerFormat: containerFormat,
	})
}

// StreamTargetFromVariant creates a StreamTarget from a CodecVariant and format.
// This is useful when bridging from existing CodecVariant-based code.
func StreamTargetFromVariant(variant CodecVariant, containerFormat string) StreamTarget {
	return NewStreamTarget(variant.VideoCodec(), variant.AudioCodec(), containerFormat)
}

// StreamTargetFromProfile creates a StreamTarget from an EncodingProfile.
// Uses the profile's target codecs and determines appropriate container format.
func StreamTargetFromProfile(profile *models.EncodingProfile) StreamTarget {
	if profile == nil {
		return StreamTarget{}
	}

	videoCodec := string(profile.TargetVideoCodec)
	audioCodec := string(profile.TargetAudioCodec)

	// Determine container format based on codec requirements
	format := "hls-fmp4" // Default to fMP4 for maximum compatibility
	if profile.RequiresFMP4() {
		format = "hls-fmp4"
	}

	return NewStreamTarget(videoCodec, audioCodec, format)
}
