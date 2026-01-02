// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
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

// GenerateCodecString generates an RFC 6381 codec string from an mp4.Codec.
// Returns a codec string suitable for use in DASH manifests and HLS playlists.
// Examples: "vp09.02.10.08" for VP9, "avc1.64001f" for H.264, "opus" for Opus.
func GenerateCodecString(c mp4.Codec) string {
	switch codec := c.(type) {
	case *mp4.CodecH264:
		// H.264: avc1.PPCCLL where PP=profile, CC=constraints, LL=level
		// Use profile_idc, constraint_set flags, and level_idc from SPS
		// SPS structure: NAL header (1), profile_idc (1), constraint flags (1), level_idc (1)
		if len(codec.SPS) >= 4 {
			profileIDC := codec.SPS[1]
			constraintFlags := codec.SPS[2]
			levelIDC := codec.SPS[3]
			return fmt.Sprintf("avc1.%02x%02x%02x", profileIDC, constraintFlags, levelIDC)
		}
		// Fallback: High Profile @ Level 3.1 (common default)
		return "avc1.64001f"

	case *mp4.CodecH265:
		// H.265/HEVC: hvc1.P.C.TLLL.BB or hev1.P.C.TLLL.BB
		// Simplified format: hev1.1.6.L93.B0 (Main profile, level 3.1)
		if len(codec.VPS) > 0 && len(codec.SPS) > 0 {
			// Parse tier and level from VPS/SPS
			// Simplified: use hev1.1.6.L93.B0 for Main profile Level 3.1
			return "hev1.1.6.L93.B0"
		}
		// Fallback: Main profile, level 5.1
		return "hev1.1.6.L153.B0"

	case *mp4.CodecVP9:
		// VP9: vp09.PP.LL.DD.CC.cp.tc.mc.FF
		// PP = profile (00-03)
		// LL = level (10, 11, 20, 21, 30, 31, 40, 41, 50, 51, 52, 60, 61, 62)
		// DD = bit depth (08, 10, 12)
		// Simplified: vp09.PP.LL.DD
		profile := codec.Profile
		bitDepth := codec.BitDepth
		if bitDepth == 0 {
			bitDepth = 8 // Default to 8-bit
		}
		// Default level based on resolution (simplified: level 31 for 1080p)
		level := 31
		if codec.Width > 1920 || codec.Height > 1080 {
			level = 41
		}
		return fmt.Sprintf("vp09.%02d.%02d.%02d", profile, level, bitDepth)

	case *mp4.CodecAV1:
		// AV1: av01.P.LLT.DD.M.CCC.cp.tc.mc.F
		// Simplified: av01.0.04M.08 (Main profile, level 4.0, 8-bit)
		if len(codec.SequenceHeader) > 0 {
			// Parse profile and level from sequence header
			return "av01.0.04M.08"
		}
		return "av01.0.04M.08"

	case *mp4.CodecMPEG4Audio:
		// AAC: mp4a.40.OT where OT is object type
		// Common: mp4a.40.2 (AAC-LC), mp4a.40.5 (HE-AAC)
		// The Type field is the AudioObjectType (2=AAC-LC, 5=HE-AAC, etc.)
		objectType := int(codec.Config.Type)
		if objectType > 0 {
			return fmt.Sprintf("mp4a.40.%d", objectType)
		}
		// Fallback: AAC-LC
		return "mp4a.40.2"

	case *mp4.CodecOpus:
		// Opus in MP4: just "opus" or "Opus"
		return "opus"

	case *mp4.CodecAC3:
		// AC-3: ac-3
		return "ac-3"

	case *mp4.CodecLPCM:
		// Linear PCM
		return "lpcm"

	case *mp4.CodecMPEG1Audio:
		// MP3: mp4a.40.34 or .mp3
		return "mp4a.40.34"

	default:
		// Unknown codec - return type name as fallback
		return fmt.Sprintf("%T", c)
	}
}

// ExtractCodecsFromInitData parses an init segment and returns video/audio codec strings.
// This is useful for generating DASH manifests with correct codec strings.
func ExtractCodecsFromInitData(initData []byte) (videoCodec, audioCodec string, err error) {
	if len(initData) == 0 {
		return "", "", fmt.Errorf("empty init data")
	}

	// Parse the init segment using fmp4
	var parsedInit fmp4.Init
	if err := parsedInit.Unmarshal(bytes.NewReader(initData)); err != nil {
		return "", "", fmt.Errorf("parsing init segment: %w", err)
	}

	for _, track := range parsedInit.Tracks {
		codecStr := GenerateCodecString(track.Codec)
		if track.Codec.IsVideo() {
			videoCodec = codecStr
		} else {
			audioCodec = codecStr
		}
	}

	return videoCodec, audioCodec, nil
}
