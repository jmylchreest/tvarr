// Package codec provides a unified codec registry for video and audio codecs.
// It consolidates codec definitions, encoder mappings, and capability information
// used throughout tvarr for transcoding, muxing, and stream handling.
package codec

import "strings"

// Video represents a video codec.
type Video string

// Video codec constants.
const (
	VideoH264 Video = "h264" // H.264/AVC
	VideoH265 Video = "h265" // H.265/HEVC
	VideoVP8  Video = "vp8"  // VP8
	VideoVP9  Video = "vp9"  // VP9 (fMP4 only)
	VideoAV1  Video = "av1"  // AV1 (fMP4 only)
	// Legacy/less common codecs (for detection, not encoding targets)
	VideoMPEG1  Video = "mpeg1"
	VideoMPEG2  Video = "mpeg2"
	VideoMPEG4  Video = "mpeg4"
	VideoVC1    Video = "vc1"
	VideoProRes Video = "prores"
	VideoDNxHD  Video = "dnxhd"
	VideoTheora Video = "theora"
)

// Audio represents an audio codec.
type Audio string

// Audio codec constants.
const (
	AudioAAC    Audio = "aac"    // AAC
	AudioMP3    Audio = "mp3"    // MP3
	AudioAC3    Audio = "ac3"    // Dolby Digital (AC-3)
	AudioEAC3   Audio = "eac3"   // Dolby Digital Plus (E-AC-3)
	AudioOpus   Audio = "opus"   // Opus (fMP4 only)
	AudioVorbis Audio = "vorbis" // Vorbis
	AudioFLAC   Audio = "flac"   // FLAC
	AudioDTS    Audio = "dts"    // DTS
	AudioTrueHD Audio = "truehd" // Dolby TrueHD
	AudioPCM    Audio = "pcm"    // PCM
)

// Container represents a media container format.
type Container string

// Container format constants.
const (
	ContainerAuto   Container = "auto"   // Auto-detect best container
	ContainerFMP4   Container = "fmp4"   // Fragmented MP4 (CMAF)
	ContainerMPEGTS Container = "mpegts" // MPEG Transport Stream
)

// HWAccel represents a hardware acceleration type.
type HWAccel string

// Hardware acceleration constants.
const (
	HWAccelAuto  HWAccel = "auto"         // Auto-detect best available
	HWAccelNone  HWAccel = "none"         // Disabled (software only)
	HWAccelCUDA  HWAccel = "cuda"         // NVIDIA CUDA/NVDEC
	HWAccelQSV   HWAccel = "qsv"          // Intel QuickSync
	HWAccelVAAPI HWAccel = "vaapi"        // Linux VA-API
	HWAccelVT    HWAccel = "videotoolbox" // macOS VideoToolbox
)

// OutputFormat represents an output container/format type for FFmpeg.
type OutputFormat string

// Output format constants.
const (
	FormatMPEGTS  OutputFormat = "mpegts"
	FormatHLS     OutputFormat = "hls"
	FormatFLV     OutputFormat = "flv"
	FormatMP4     OutputFormat = "mp4"
	FormatFMP4    OutputFormat = "fmp4" // Fragmented MP4 (CMAF)
	FormatMKV     OutputFormat = "matroska"
	FormatWebM    OutputFormat = "webm"
	FormatUnknown OutputFormat = ""
)

// String returns the string representation of the video codec.
func (v Video) String() string {
	return string(v)
}

// String returns the string representation of the audio codec.
func (a Audio) String() string {
	return string(a)
}

// String returns the string representation of the container.
func (c Container) String() string {
	return string(c)
}

// String returns the string representation of the hardware acceleration type.
func (h HWAccel) String() string {
	return string(h)
}

// String returns the string representation of the output format.
func (o OutputFormat) String() string {
	return string(o)
}

// videoInfo contains metadata about a video codec.
type videoInfo struct {
	// Canonical name (h264, h265, etc.)
	Name Video
	// All known aliases and encoder names that map to this codec
	Aliases []string
	// FFmpeg encoders for each hardware acceleration type
	Encoders map[HWAccel]string
	// Whether this codec requires fMP4 container (can't use MPEG-TS)
	FMP4Only bool
	// Whether this codec can be demuxed by mediacommon MPEG-TS demuxer
	Demuxable bool
	// MPEG-TS stream type identifier (0 if not supported)
	MPEGTSStreamType uint8
}

// audioInfo contains metadata about an audio codec.
type audioInfo struct {
	// Canonical name (aac, mp3, etc.)
	Name Audio
	// All known aliases and encoder names that map to this codec
	Aliases []string
	// FFmpeg encoder name
	Encoder string
	// Whether this codec requires fMP4 container (can't use MPEG-TS)
	FMP4Only bool
	// Whether this codec can be demuxed by mediacommon MPEG-TS demuxer
	Demuxable bool
	// MPEG-TS stream type identifier (0 if not supported)
	MPEGTSStreamType uint8
}

// MPEG-TS stream type constants.
const (
	StreamTypeH264 uint8 = 0x1B
	StreamTypeH265 uint8 = 0x24
	StreamTypeAAC  uint8 = 0x0F
	StreamTypeAC3  uint8 = 0x81
	StreamTypeEAC3 uint8 = 0x87
	StreamTypeMP3  uint8 = 0x03
)

// videoRegistry contains all video codec definitions.
var videoRegistry = map[Video]*videoInfo{
	VideoH264: {
		Name: VideoH264,
		Aliases: []string{
			"h264", "avc", "avc1", "h.264",
			// Encoders
			"libx264", "h264_nvenc", "h264_qsv", "h264_vaapi",
			"h264_videotoolbox", "h264_amf", "h264_mf", "h264_omx", "h264_v4l2m2m",
		},
		Encoders: map[HWAccel]string{
			HWAccelNone:  "libx264",
			HWAccelAuto:  "libx264",
			HWAccelCUDA:  "h264_nvenc",
			HWAccelQSV:   "h264_qsv",
			HWAccelVAAPI: "h264_vaapi",
			HWAccelVT:    "h264_videotoolbox",
		},
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: StreamTypeH264,
	},
	VideoH265: {
		Name: VideoH265,
		Aliases: []string{
			"h265", "hevc", "hev1", "hvc1", "h.265",
			// Encoders
			"libx265", "hevc_nvenc", "hevc_qsv", "hevc_vaapi",
			"hevc_videotoolbox", "hevc_amf", "hevc_mf", "hevc_v4l2m2m",
		},
		Encoders: map[HWAccel]string{
			HWAccelNone:  "libx265",
			HWAccelAuto:  "libx265",
			HWAccelCUDA:  "hevc_nvenc",
			HWAccelQSV:   "hevc_qsv",
			HWAccelVAAPI: "hevc_vaapi",
			HWAccelVT:    "hevc_videotoolbox",
		},
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: StreamTypeH265,
	},
	VideoVP8: {
		Name:             VideoVP8,
		Aliases:          []string{"vp8", "libvpx"},
		Encoders:         map[HWAccel]string{HWAccelNone: "libvpx", HWAccelAuto: "libvpx"},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoVP9: {
		Name:    VideoVP9,
		Aliases: []string{"vp9", "vp09", "libvpx-vp9", "vp9_qsv", "vp9_vaapi"},
		Encoders: map[HWAccel]string{
			HWAccelNone:  "libvpx-vp9",
			HWAccelAuto:  "libvpx-vp9",
			HWAccelQSV:   "vp9_qsv",
			HWAccelVAAPI: "vp9_vaapi",
		},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoAV1: {
		Name: VideoAV1,
		Aliases: []string{
			"av1", "av01",
			"libaom-av1", "libsvtav1", "librav1e",
			"av1_nvenc", "av1_qsv", "av1_vaapi", "av1_amf",
		},
		Encoders: map[HWAccel]string{
			HWAccelNone:  "libaom-av1",
			HWAccelAuto:  "libaom-av1",
			HWAccelCUDA:  "av1_nvenc",
			HWAccelQSV:   "av1_qsv",
			HWAccelVAAPI: "av1_vaapi",
		},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoMPEG1: {
		Name:             VideoMPEG1,
		Aliases:          []string{"mpeg1", "mpeg1video"},
		Encoders:         map[HWAccel]string{HWAccelNone: "mpeg1video"},
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: 0x01,
	},
	VideoMPEG2: {
		Name:             VideoMPEG2,
		Aliases:          []string{"mpeg2", "mpeg2video"},
		Encoders:         map[HWAccel]string{HWAccelNone: "mpeg2video"},
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: 0x02,
	},
	VideoMPEG4: {
		Name:             VideoMPEG4,
		Aliases:          []string{"mpeg4"},
		Encoders:         map[HWAccel]string{HWAccelNone: "mpeg4"},
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: 0x10,
	},
	VideoVC1: {
		Name:             VideoVC1,
		Aliases:          []string{"vc1", "wmv3"},
		Encoders:         nil, // decode only
		FMP4Only:         false,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoProRes: {
		Name:             VideoProRes,
		Aliases:          []string{"prores", "prores_ks"},
		Encoders:         map[HWAccel]string{HWAccelNone: "prores_ks"},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoDNxHD: {
		Name:             VideoDNxHD,
		Aliases:          []string{"dnxhd", "dnxhr"},
		Encoders:         map[HWAccel]string{HWAccelNone: "dnxhd"},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	VideoTheora: {
		Name:             VideoTheora,
		Aliases:          []string{"theora", "libtheora"},
		Encoders:         map[HWAccel]string{HWAccelNone: "libtheora"},
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
}

// audioRegistry contains all audio codec definitions.
var audioRegistry = map[Audio]*audioInfo{
	AudioAAC: {
		Name:             AudioAAC,
		Aliases:          []string{"aac", "mp4a", "libfdk_aac", "aac_at"},
		Encoder:          "aac",
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: StreamTypeAAC,
	},
	AudioMP3: {
		Name:             AudioMP3,
		Aliases:          []string{"mp3", "mp3float", "libmp3lame"},
		Encoder:          "libmp3lame",
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: StreamTypeMP3,
	},
	AudioAC3: {
		Name:             AudioAC3,
		Aliases:          []string{"ac3", "ac-3", "a52", "ac3_fixed"},
		Encoder:          "ac3",
		FMP4Only:         false,
		Demuxable:        true,
		MPEGTSStreamType: StreamTypeAC3,
	},
	AudioEAC3: {
		Name:             AudioEAC3,
		Aliases:          []string{"eac3", "ec-3"},
		Encoder:          "eac3",
		FMP4Only:         false, // E-AC3 can be in MPEG-TS, just not demuxable by mediacommon
		Demuxable:        false,
		MPEGTSStreamType: 0x87, // E-AC3 in MPEG-TS
	},
	AudioOpus: {
		Name:             AudioOpus,
		Aliases:          []string{"opus", "libopus"},
		Encoder:          "libopus",
		FMP4Only:         true,
		Demuxable:        true,
		MPEGTSStreamType: 0,
	},
	AudioVorbis: {
		Name:             AudioVorbis,
		Aliases:          []string{"vorbis", "libvorbis"},
		Encoder:          "libvorbis",
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	AudioFLAC: {
		Name:             AudioFLAC,
		Aliases:          []string{"flac", "libflac"},
		Encoder:          "flac",
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	AudioDTS: {
		Name:             AudioDTS,
		Aliases:          []string{"dts", "dca"},
		Encoder:          "dca",
		FMP4Only:         false,
		Demuxable:        false,
		MPEGTSStreamType: 0x82,
	},
	AudioTrueHD: {
		Name:             AudioTrueHD,
		Aliases:          []string{"truehd", "mlp"},
		Encoder:          "truehd",
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
	AudioPCM: {
		Name:             AudioPCM,
		Aliases:          []string{"pcm", "pcm_s16le", "pcm_s24le", "pcm_s32le"},
		Encoder:          "pcm_s16le",
		FMP4Only:         true,
		Demuxable:        false,
		MPEGTSStreamType: 0,
	},
}

// videoAliasIndex maps all aliases to their canonical codec.
var videoAliasIndex map[string]Video

// audioAliasIndex maps all aliases to their canonical codec.
var audioAliasIndex map[string]Audio

func init() {
	// Build video alias index
	videoAliasIndex = make(map[string]Video)
	for codec, info := range videoRegistry {
		for _, alias := range info.Aliases {
			videoAliasIndex[strings.ToLower(alias)] = codec
		}
	}

	// Build audio alias index
	audioAliasIndex = make(map[string]Audio)
	for codec, info := range audioRegistry {
		for _, alias := range info.Aliases {
			audioAliasIndex[strings.ToLower(alias)] = codec
		}
	}
}

// ParseVideo parses a string (codec name, alias, or encoder) to a Video codec.
// Returns the canonical codec and whether the parse was successful.
func ParseVideo(s string) (Video, bool) {
	if s == "" {
		return "", false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	codec, ok := videoAliasIndex[s]
	return codec, ok
}

// ParseAudio parses a string (codec name, alias, or encoder) to an Audio codec.
// Returns the canonical codec and whether the parse was successful.
func ParseAudio(s string) (Audio, bool) {
	if s == "" {
		return "", false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	codec, ok := audioAliasIndex[s]
	return codec, ok
}

// Normalize converts any codec string (encoder name, alias) to its canonical form.
// Returns the input unchanged if not recognized.
func Normalize(name string) string {
	if name == "" {
		return name
	}
	lower := strings.ToLower(name)

	// Check video codecs
	if codec, ok := videoAliasIndex[lower]; ok {
		return string(codec)
	}

	// Check audio codecs
	if codec, ok := audioAliasIndex[lower]; ok {
		return string(codec)
	}

	return name
}

// NormalizeHLSCodec normalizes codec strings from HLS/DASH manifests to canonical form.
// HLS codec strings include version/profile info (e.g., "avc1.64001f", "mp4a.40.2").
// This function extracts the base codec and normalizes it.
func NormalizeHLSCodec(name string) string {
	if name == "" {
		return name
	}

	lower := strings.ToLower(name)

	// First try exact match (handles simple cases like "h264", "aac")
	if codec, ok := videoAliasIndex[lower]; ok {
		return string(codec)
	}
	if codec, ok := audioAliasIndex[lower]; ok {
		return string(codec)
	}

	// Handle HLS codec strings with version/profile suffixes
	// Common formats: avc1.*, hev1.*, hvc1.*, mp4a.*, vp09.*, av01.*, ac-3, ec-3
	if len(lower) >= 4 {
		prefix := lower[:4]
		switch prefix {
		case "avc1", "avc3":
			return string(VideoH264)
		case "hev1", "hvc1":
			return string(VideoH265)
		case "mp4a":
			return string(AudioAAC) // mp4a.40.2 = AAC-LC, mp4a.40.5 = HE-AAC, etc.
		case "vp09":
			return string(VideoVP9)
		case "av01":
			return string(VideoAV1)
		case "ac-3":
			return string(AudioAC3)
		case "ec-3":
			return string(AudioEAC3)
		}
	}

	// Handle common aliases not in the index
	switch lower {
	case "hevc":
		return string(VideoH265)
	case "avc":
		return string(VideoH264)
	}

	return name
}

// NormalizeVideo normalizes a video codec/encoder name to its canonical form.
// Returns the canonical codec string (e.g., "h264", "h265") or the input unchanged.
func NormalizeVideo(name string) string {
	if codec, ok := ParseVideo(name); ok {
		return string(codec)
	}
	return name
}

// NormalizeAudio normalizes an audio codec/encoder name to its canonical form.
// Returns the canonical codec string (e.g., "aac", "mp3") or the input unchanged.
func NormalizeAudio(name string) string {
	if codec, ok := ParseAudio(name); ok {
		return string(codec)
	}
	return name
}

// IsEncoder returns true if the name appears to be an FFmpeg encoder name
// rather than a codec name.
func IsEncoder(name string) bool {
	name = strings.ToLower(name)

	// Check for lib* prefix (software encoders)
	if strings.HasPrefix(name, "lib") {
		return true
	}

	// Check for hardware encoder suffixes
	hwSuffixes := []string{
		"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf",
		"_mf", "_omx", "_v4l2m2m", "_cuvid", "_at", "_fixed",
	}
	for _, suffix := range hwSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

// GetVideoEncoder returns the FFmpeg encoder name for a video codec with the given
// hardware acceleration. Falls back to software encoder if hwaccel not supported.
func GetVideoEncoder(v Video, hwaccel HWAccel) string {
	info, ok := videoRegistry[v]
	if !ok {
		return string(v) // Return as-is for unknown codecs
	}

	if info.Encoders == nil {
		return "" // Decode-only codec
	}

	// Try requested hwaccel first
	if encoder, ok := info.Encoders[hwaccel]; ok {
		return encoder
	}

	// Fall back to software encoder
	if encoder, ok := info.Encoders[HWAccelNone]; ok {
		return encoder
	}

	return string(v)
}

// GetAudioEncoder returns the FFmpeg encoder name for an audio codec.
func GetAudioEncoder(a Audio) string {
	info, ok := audioRegistry[a]
	if !ok {
		return string(a) // Return as-is for unknown codecs
	}
	return info.Encoder
}

// IsFMP4Only returns true if the video codec requires fMP4 container.
func (v Video) IsFMP4Only() bool {
	info, ok := videoRegistry[v]
	if !ok {
		return false
	}
	return info.FMP4Only
}

// IsFMP4Only returns true if the audio codec requires fMP4 container.
func (a Audio) IsFMP4Only() bool {
	info, ok := audioRegistry[a]
	if !ok {
		return false
	}
	return info.FMP4Only
}

// IsDemuxable returns true if the video codec can be demuxed by mediacommon.
func (v Video) IsDemuxable() bool {
	info, ok := videoRegistry[v]
	if !ok {
		return true // Assume demuxable for unknown (most common codecs are)
	}
	return info.Demuxable
}

// IsDemuxable returns true if the audio codec can be demuxed by mediacommon.
func (a Audio) IsDemuxable() bool {
	info, ok := audioRegistry[a]
	if !ok {
		return false // Assume NOT demuxable for unknown (safer)
	}
	return info.Demuxable
}

// MPEGTSStreamType returns the MPEG-TS stream type for the video codec.
// Returns 0 if not supported in MPEG-TS.
func (v Video) MPEGTSStreamType() uint8 {
	info, ok := videoRegistry[v]
	if !ok {
		return 0
	}
	return info.MPEGTSStreamType
}

// MPEGTSStreamType returns the MPEG-TS stream type for the audio codec.
// Returns 0 if not supported in MPEG-TS.
func (a Audio) MPEGTSStreamType() uint8 {
	info, ok := audioRegistry[a]
	if !ok {
		return 0
	}
	return info.MPEGTSStreamType
}

// IsVideoDemuxable checks if a video codec string is demuxable by mediacommon.
// This is a convenience function that parses and checks demuxability.
func IsVideoDemuxable(codecName string) bool {
	codec, ok := ParseVideo(codecName)
	if !ok {
		return true // Assume demuxable for unknown (most common codecs are H.264/H.265)
	}
	return codec.IsDemuxable()
}

// IsAudioDemuxable checks if an audio codec string is demuxable by mediacommon.
// This is a convenience function that parses and checks demuxability.
func IsAudioDemuxable(codecName string) bool {
	codec, ok := ParseAudio(codecName)
	if !ok {
		return false // Assume NOT demuxable for unknown (safer)
	}
	return codec.IsDemuxable()
}

// VideoRequiresFMP4 checks if a video codec string requires fMP4 container.
func VideoRequiresFMP4(codecName string) bool {
	codec, ok := ParseVideo(codecName)
	if !ok {
		return false
	}
	return codec.IsFMP4Only()
}

// AudioRequiresFMP4 checks if an audio codec string requires fMP4 container.
func AudioRequiresFMP4(codecName string) bool {
	codec, ok := ParseAudio(codecName)
	if !ok {
		return false
	}
	return codec.IsFMP4Only()
}

// Match returns true if two codec strings represent the same codec.
// Handles aliases, encoder names, and case differences.
func Match(a, b string) bool {
	if a == "" || b == "" {
		return false
	}

	// Normalize both
	normA := Normalize(a)
	normB := Normalize(b)

	return strings.EqualFold(normA, normB)
}

// VideoMatch returns true if two video codec strings represent the same codec.
func VideoMatch(a, b string) bool {
	codecA, okA := ParseVideo(a)
	codecB, okB := ParseVideo(b)
	if !okA || !okB {
		return false
	}
	return codecA == codecB
}

// AudioMatch returns true if two audio codec strings represent the same codec.
func AudioMatch(a, b string) bool {
	codecA, okA := ParseAudio(a)
	codecB, okB := ParseAudio(b)
	if !okA || !okB {
		return false
	}
	return codecA == codecB
}

// ValidVideoCodecs returns a map of all valid video codec names to their Video type.
// Includes canonical names and common aliases.
func ValidVideoCodecs() map[string]Video {
	result := make(map[string]Video)
	// Only include canonical names and common aliases (not encoder names)
	commonAliases := map[string]Video{
		"h264": VideoH264,
		"h265": VideoH265,
		"hevc": VideoH265, // Alias
		"vp8":  VideoVP8,
		"vp9":  VideoVP9,
		"av1":  VideoAV1,
	}
	for name, codec := range commonAliases {
		result[name] = codec
	}
	return result
}

// ValidAudioCodecs returns a map of all valid audio codec names to their Audio type.
// Includes canonical names.
func ValidAudioCodecs() map[string]Audio {
	result := make(map[string]Audio)
	commonAliases := map[string]Audio{
		"aac":  AudioAAC,
		"mp3":  AudioMP3,
		"ac3":  AudioAC3,
		"eac3": AudioEAC3,
		"opus": AudioOpus,
	}
	for name, codec := range commonAliases {
		result[name] = codec
	}
	return result
}

// ValidHWAccels returns a map of valid hardware acceleration types.
func ValidHWAccels() map[string]HWAccel {
	return map[string]HWAccel{
		"auto":         HWAccelAuto,
		"none":         HWAccelNone,
		"cuda":         HWAccelCUDA,
		"qsv":          HWAccelQSV,
		"vaapi":        HWAccelVAAPI,
		"videotoolbox": HWAccelVT,
	}
}

// ParseHWAccel parses a hardware acceleration string.
func ParseHWAccel(s string) (HWAccel, bool) {
	hwaccels := ValidHWAccels()
	hw, ok := hwaccels[strings.ToLower(strings.TrimSpace(s))]
	return hw, ok
}

// ParseOutputFormat converts a string to OutputFormat.
func ParseOutputFormat(format string) OutputFormat {
	switch strings.ToLower(format) {
	case "mpegts", "ts":
		return FormatMPEGTS
	case "hls", "m3u8":
		return FormatHLS
	case "flv":
		return FormatFLV
	case "mp4":
		return FormatMP4
	case "fmp4", "cmaf":
		return FormatFMP4
	case "matroska", "mkv":
		return FormatMKV
	case "webm":
		return FormatWebM
	default:
		return FormatUnknown
	}
}

// RequiresAnnexB returns true if the output format requires Annex B NAL format.
func (o OutputFormat) RequiresAnnexB() bool {
	switch o {
	case FormatMPEGTS, FormatHLS:
		return true
	default:
		return false
	}
}

// SupportedEncodingVideoCodecs returns the list of video codecs supported as encoding targets.
func SupportedEncodingVideoCodecs() []Video {
	return []Video{VideoH264, VideoH265, VideoVP9, VideoAV1}
}

// SupportedEncodingAudioCodecs returns the list of audio codecs supported as encoding targets.
func SupportedEncodingAudioCodecs() []Audio {
	return []Audio{AudioAAC, AudioMP3, AudioAC3, AudioEAC3, AudioOpus}
}
