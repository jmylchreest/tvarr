package models

import (
	"strings"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// VideoCodec represents a video codec for transcoding.
// This type is kept for backwards compatibility with existing model code.
// New code should consider using codec.Video directly.
type VideoCodec string

const (
	VideoCodecH264 VideoCodec = VideoCodec(codec.VideoH264) // H.264/AVC
	VideoCodecH265 VideoCodec = VideoCodec(codec.VideoH265) // H.265/HEVC
	VideoCodecVP9  VideoCodec = VideoCodec(codec.VideoVP9)  // VP9 (fMP4 only)
	VideoCodecAV1  VideoCodec = VideoCodec(codec.VideoAV1)  // AV1 (fMP4 only)
)

// GetFFmpegEncoder returns the FFmpeg encoder name based on codec and hardware acceleration.
// This maps abstract codec types to concrete FFmpeg encoder names.
func (c VideoCodec) GetFFmpegEncoder(hwaccel HWAccelType) string {
	// Convert to codec package types and use centralized encoder mapping
	v, ok := codec.ParseVideo(string(c))
	if !ok {
		return string(c)
	}
	hw, _ := codec.ParseHWAccel(string(hwaccel))
	return codec.GetVideoEncoder(v, hw)
}

// IsFMP4Only returns true if this codec requires fMP4 container.
func (c VideoCodec) IsFMP4Only() bool {
	v, ok := codec.ParseVideo(string(c))
	if !ok {
		return false
	}
	return v.IsFMP4Only()
}

// AudioCodec represents an audio codec for transcoding.
type AudioCodec string

const (
	AudioCodecAAC  AudioCodec = AudioCodec(codec.AudioAAC)  // AAC
	AudioCodecMP3  AudioCodec = AudioCodec(codec.AudioMP3)  // MP3
	AudioCodecAC3  AudioCodec = AudioCodec(codec.AudioAC3)  // Dolby Digital
	AudioCodecEAC3 AudioCodec = AudioCodec(codec.AudioEAC3) // Dolby Digital Plus
	AudioCodecOpus AudioCodec = AudioCodec(codec.AudioOpus) // Opus (fMP4 only)
)

// GetFFmpegEncoder returns the FFmpeg encoder name for this audio codec.
func (c AudioCodec) GetFFmpegEncoder() string {
	a, ok := codec.ParseAudio(string(c))
	if !ok {
		return string(c)
	}
	return codec.GetAudioEncoder(a)
}

// IsFMP4Only returns true if this codec requires fMP4 container.
func (c AudioCodec) IsFMP4Only() bool {
	a, ok := codec.ParseAudio(string(c))
	if !ok {
		return false
	}
	return a.IsFMP4Only()
}

// ContainerFormat represents the media container format for streaming.
// This separates the container (TS/fMP4) from the manifest format (HLS/DASH).
type ContainerFormat string

const (
	// ContainerFormatAuto lets the system choose the best container based on codec and client.
	// - fMP4 for: DASH requests, HLS requests from modern browsers (Chrome, Firefox, Edge, Safari 10+)
	// - MPEG-TS for: explicit ?format=mpegts, legacy User-Agents, Accept header requesting video/MP2T
	ContainerFormatAuto ContainerFormat = ContainerFormat(codec.ContainerAuto)

	// ContainerFormatFMP4 forces fragmented MP4 (CMAF) container.
	// Required for VP9, AV1, and Opus codecs. Supports HLS v7+ and DASH.
	ContainerFormatFMP4 ContainerFormat = ContainerFormat(codec.ContainerFMP4)

	// ContainerFormatMPEGTS forces MPEG Transport Stream container.
	// Maximum compatibility with legacy devices. Limited to H.264/H.265/AAC/MP3/AC3/EAC3.
	ContainerFormatMPEGTS ContainerFormat = ContainerFormat(codec.ContainerMPEGTS)
)

// HWAccelType represents hardware acceleration type.
type HWAccelType string

const (
	HWAccelAuto  HWAccelType = HWAccelType(codec.HWAccelAuto)  // Auto-detect best available
	HWAccelNone  HWAccelType = HWAccelType(codec.HWAccelNone)  // Disabled (software only)
	HWAccelNVDEC HWAccelType = HWAccelType(codec.HWAccelCUDA)  // NVIDIA NVDEC
	HWAccelQSV   HWAccelType = HWAccelType(codec.HWAccelQSV)   // Intel QuickSync
	HWAccelVAAPI HWAccelType = HWAccelType(codec.HWAccelVAAPI) // Linux VA-API
	HWAccelVT    HWAccelType = HWAccelType(codec.HWAccelVT)    // macOS
)

// IsFMP4OnlyVideoCodec returns true if the codec requires fMP4 container (not compatible with MPEG-TS).
func IsFMP4OnlyVideoCodec(c VideoCodec) bool {
	return c.IsFMP4Only()
}

// IsFMP4OnlyAudioCodec returns true if the codec requires fMP4 container (not compatible with MPEG-TS).
func IsFMP4OnlyAudioCodec(c AudioCodec) bool {
	return c.IsFMP4Only()
}

// ValidVideoCodecs is the set of all valid video codec values.
var ValidVideoCodecs = map[string]VideoCodec{
	"h264": VideoCodecH264,
	"h265": VideoCodecH265,
	"hevc": VideoCodecH265, // Alias
	"vp9":  VideoCodecVP9,
	"av1":  VideoCodecAV1,
}

// ValidAudioCodecs is the set of all valid audio codec values.
var ValidAudioCodecs = map[string]AudioCodec{
	"aac":  AudioCodecAAC,
	"mp3":  AudioCodecMP3,
	"ac3":  AudioCodecAC3,
	"eac3": AudioCodecEAC3,
	"opus": AudioCodecOpus,
}

// ParseVideoCodec parses a string to a VideoCodec, returning the codec and whether it's valid.
// Handles common aliases (e.g., "hevc" -> h265).
func ParseVideoCodec(s string) (VideoCodec, bool) {
	// Use codec package for parsing
	v, ok := codec.ParseVideo(s)
	if !ok {
		return "", false
	}
	return VideoCodec(v), true
}

// ParseAudioCodec parses a string to an AudioCodec, returning the codec and whether it's valid.
func ParseAudioCodec(s string) (AudioCodec, bool) {
	// Use codec package for parsing
	a, ok := codec.ParseAudio(s)
	if !ok {
		return "", false
	}
	return AudioCodec(a), true
}

// ValidPreferredFormats is the set of all valid preferred format values for client detection.
// Maps input values to canonical format names.
var ValidPreferredFormats = map[string]string{
	"hls-fmp4": "hls-fmp4",
	"hls-ts":   "hls-ts",
	"dash":     "dash",
	"mpegts":   "mpegts", // Raw MPEG-TS stream (for VLC, mpv, etc.)
	"fmp4":     "hls-fmp4", // Alias
	"ts":       "hls-ts",   // Alias (HLS with TS segments, not raw TS)
}

// ParsePreferredFormat parses a string to a valid preferred format, returning the format and whether it's valid.
// Handles common aliases (e.g., "fmp4" -> "hls-fmp4", "ts" -> "hls-ts").
func ParsePreferredFormat(s string) (string, bool) {
	format, ok := ValidPreferredFormats[strings.ToLower(strings.TrimSpace(s))]
	return format, ok
}

// NormalizeCodecName converts encoder names and aliases to canonical codec names.
// This wraps the codec package's Normalize function for convenience.
func NormalizeCodecName(name string) string {
	return codec.Normalize(name)
}

// IsVideoDemuxable returns true if the video codec can be demuxed by mediacommon.
func IsVideoDemuxable(codecName string) bool {
	return codec.IsVideoDemuxable(codecName)
}

// IsAudioDemuxable returns true if the audio codec can be demuxed by mediacommon.
func IsAudioDemuxable(codecName string) bool {
	return codec.IsAudioDemuxable(codecName)
}

// CodecsMatch returns true if two codec strings represent the same codec.
func CodecsMatch(a, b string) bool {
	return codec.Match(a, b)
}
