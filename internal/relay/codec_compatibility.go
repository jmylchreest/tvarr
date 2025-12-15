// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"strings"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// ContainerFormat represents the target output container format.
type ContainerFormat string

// Container format constants for routing decisions.
const (
	ContainerMPEGTS  ContainerFormat = "mpegts"
	ContainerHLSTS   ContainerFormat = "hls-ts"
	ContainerHLSFMP4 ContainerFormat = "hls-fmp4"
	ContainerDASH    ContainerFormat = "dash"
	ContainerFMP4    ContainerFormat = "fmp4"
)

// ParseContainerFormat converts a client format string to ContainerFormat.
func ParseContainerFormat(format string) ContainerFormat {
	switch strings.ToLower(format) {
	case "mpegts", "ts", "mpeg-ts":
		return ContainerMPEGTS
	case "hls-ts":
		return ContainerHLSTS
	case "hls-fmp4", "hls":
		return ContainerHLSFMP4
	case "dash":
		return ContainerDASH
	case "fmp4", "cmaf":
		return ContainerFMP4
	default:
		// Default to HLS-fMP4 for modern clients
		return ContainerHLSFMP4
	}
}

// String returns the string representation of the container format.
func (c ContainerFormat) String() string {
	return string(c)
}

// UsesSegmentedTS returns true if the container uses MPEG-TS segments.
// HLS-TS and plain MPEG-TS both use TS container for segments.
func (c ContainerFormat) UsesSegmentedTS() bool {
	return c == ContainerMPEGTS || c == ContainerHLSTS
}

// UsesFMP4 returns true if the container uses fragmented MP4.
// HLS-fMP4, DASH, and plain fMP4 all use fragmented MP4 segments.
func (c ContainerFormat) UsesFMP4() bool {
	return c == ContainerHLSFMP4 || c == ContainerDASH || c == ContainerFMP4
}

// CodecCompatibility defines codec support for a container format.
type CodecCompatibility struct {
	// Format is the container format.
	Format ContainerFormat

	// SupportedVideoCodecs is a set of video codecs supported by this container.
	// If empty, all video codecs are supported.
	SupportedVideoCodecs map[string]bool

	// SupportedAudioCodecs is a set of audio codecs supported by this container.
	// If empty, all audio codecs are supported.
	SupportedAudioCodecs map[string]bool
}

// containerCompatibilityMap defines codec support for each container format.
var containerCompatibilityMap = map[ContainerFormat]*CodecCompatibility{
	ContainerMPEGTS: {
		Format: ContainerMPEGTS,
		SupportedVideoCodecs: map[string]bool{
			"h264": true, "avc": true, "avc1": true,
			"h265": true, "hevc": true, "hvc1": true, "hev1": true,
			"mpeg1": true, "mpeg2": true, "mpeg4": true,
		},
		SupportedAudioCodecs: map[string]bool{
			"aac": true, "mp4a": true,
			"mp3": true,
			"ac3": true, "ec3": true, "eac3": true,
			"dts": true,
		},
	},
	ContainerHLSTS: {
		Format: ContainerHLSTS,
		// HLS-TS uses MPEG-TS segments, same codec support
		SupportedVideoCodecs: map[string]bool{
			"h264": true, "avc": true, "avc1": true,
			"h265": true, "hevc": true, "hvc1": true, "hev1": true,
		},
		SupportedAudioCodecs: map[string]bool{
			"aac": true, "mp4a": true,
			"mp3": true,
			"ac3": true, "ec3": true, "eac3": true,
		},
	},
	ContainerHLSFMP4: {
		Format: ContainerHLSFMP4,
		// fMP4 supports virtually all codecs
		SupportedVideoCodecs: nil, // nil = all supported
		SupportedAudioCodecs: nil, // nil = all supported
	},
	ContainerDASH: {
		Format: ContainerDASH,
		// DASH with fMP4 supports virtually all codecs
		SupportedVideoCodecs: nil, // nil = all supported
		SupportedAudioCodecs: nil, // nil = all supported
	},
	ContainerFMP4: {
		Format: ContainerFMP4,
		// Plain fMP4 supports virtually all codecs
		SupportedVideoCodecs: nil, // nil = all supported
		SupportedAudioCodecs: nil, // nil = all supported
	},
}

// GetContainerCompatibility returns the codec compatibility info for a container format.
func GetContainerCompatibility(format ContainerFormat) *CodecCompatibility {
	if compat, ok := containerCompatibilityMap[format]; ok {
		return compat
	}
	// Default to fMP4 compatibility (all codecs supported)
	return containerCompatibilityMap[ContainerHLSFMP4]
}

// IsCodecCompatible checks if a single codec is compatible with the container format.
// Returns true if the codec can be muxed into the container without transcoding.
func IsCodecCompatible(format ContainerFormat, codecName string) bool {
	compat := GetContainerCompatibility(format)

	// Normalize the codec name for consistent lookup
	normalizedCodec := codec.Normalize(codecName)
	codecLower := strings.ToLower(normalizedCodec)

	// Check against video codecs
	if compat.SupportedVideoCodecs != nil {
		// Check if it's a known video codec
		if _, isVideo := codec.ParseVideo(codecName); isVideo {
			// Extract base codec from full codec string (e.g., "avc1.64001f" -> "avc1")
			parts := strings.Split(codecLower, ".")
			baseCodec := parts[0]
			return compat.SupportedVideoCodecs[baseCodec]
		}
	}

	// Check against audio codecs
	if compat.SupportedAudioCodecs != nil {
		// Check if it's a known audio codec
		if _, isAudio := codec.ParseAudio(codecName); isAudio {
			parts := strings.Split(codecLower, ".")
			baseCodec := parts[0]
			return compat.SupportedAudioCodecs[baseCodec]
		}
	}

	// If the codec map is nil, all codecs are supported
	if compat.SupportedVideoCodecs == nil && compat.SupportedAudioCodecs == nil {
		return true
	}

	// Unknown codec type - check both maps
	parts := strings.Split(codecLower, ".")
	baseCodec := parts[0]

	if compat.SupportedVideoCodecs != nil && compat.SupportedVideoCodecs[baseCodec] {
		return true
	}
	if compat.SupportedAudioCodecs != nil && compat.SupportedAudioCodecs[baseCodec] {
		return true
	}

	// If we have a nil map for either type, consider it compatible
	return compat.SupportedVideoCodecs == nil || compat.SupportedAudioCodecs == nil
}

// AreCodecsCompatible checks if all codecs are compatible with the container format.
// Returns true if all provided codecs can be muxed into the container without transcoding.
func AreCodecsCompatible(format ContainerFormat, codecs []string) bool {
	if len(codecs) == 0 {
		return true
	}

	for _, c := range codecs {
		if !IsCodecCompatible(format, c) {
			return false
		}
	}
	return true
}

// CodecsCompatibleWithMPEGTS is a convenience function to check MPEG-TS compatibility.
// Returns true if all codecs can be muxed into MPEG-TS container.
func CodecsCompatibleWithMPEGTS(codecs []string) bool {
	return AreCodecsCompatible(ContainerMPEGTS, codecs)
}

// CodecsCompatibleWithHLS is a convenience function to check HLS-TS compatibility.
// Returns true if all codecs can be muxed into HLS with MPEG-TS segments.
func CodecsCompatibleWithHLS(codecs []string) bool {
	return AreCodecsCompatible(ContainerHLSTS, codecs)
}

// RequiresTranscodeForContainer checks if any codec requires transcoding for the target container.
// Returns a list of codecs that need transcoding, or empty slice if remux is possible.
func RequiresTranscodeForContainer(format ContainerFormat, codecs []string) []string {
	var incompatible []string
	for _, c := range codecs {
		if !IsCodecCompatible(format, c) {
			incompatible = append(incompatible, c)
		}
	}
	return incompatible
}
