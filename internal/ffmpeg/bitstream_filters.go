package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// OutputFormatType represents output container formats
type OutputFormatType string

const (
	FormatMPEGTS   OutputFormatType = "mpegts"
	FormatHLS      OutputFormatType = "hls"
	FormatFLV      OutputFormatType = "flv"
	FormatMP4      OutputFormatType = "mp4"
	FormatMKV      OutputFormatType = "matroska"
	FormatWebM     OutputFormatType = "webm"
	FormatUnknown  OutputFormatType = ""
)

// CodecFamily represents the base codec family (independent of encoder implementation)
type CodecFamily string

const (
	CodecFamilyH264    CodecFamily = "h264"
	CodecFamilyHEVC    CodecFamily = "hevc"
	CodecFamilyVP9     CodecFamily = "vp9"
	CodecFamilyAV1     CodecFamily = "av1"
	CodecFamilyAAC     CodecFamily = "aac"
	CodecFamilyAC3     CodecFamily = "ac3"
	CodecFamilyEAC3    CodecFamily = "eac3"
	CodecFamilyMP3     CodecFamily = "mp3"
	CodecFamilyOpus    CodecFamily = "opus"
	CodecFamilyUnknown CodecFamily = ""
)

// BitstreamFilterInfo contains information about a bitstream filter to apply
type BitstreamFilterInfo struct {
	VideoBSF string // Bitstream filter for video (e.g., "h264_mp4toannexb")
	AudioBSF string // Bitstream filter for audio (e.g., "aac_adtstoasc")
	Reason   string // Why this filter is needed
}

// encoderToCodecFamily maps encoder names to their codec families
var encoderToCodecFamily = map[string]CodecFamily{
	// H.264 encoders
	"libx264":           CodecFamilyH264,
	"h264_nvenc":        CodecFamilyH264,
	"h264_qsv":          CodecFamilyH264,
	"h264_vaapi":        CodecFamilyH264,
	"h264_videotoolbox": CodecFamilyH264,
	"h264_amf":          CodecFamilyH264,
	"h264_mf":           CodecFamilyH264,
	"h264_omx":          CodecFamilyH264,
	"h264_v4l2m2m":      CodecFamilyH264,
	"copy":              CodecFamilyUnknown, // Need to detect source

	// HEVC/H.265 encoders
	"libx265":           CodecFamilyHEVC,
	"hevc_nvenc":        CodecFamilyHEVC,
	"hevc_qsv":          CodecFamilyHEVC,
	"hevc_vaapi":        CodecFamilyHEVC,
	"hevc_videotoolbox": CodecFamilyHEVC,
	"hevc_amf":          CodecFamilyHEVC,
	"hevc_mf":           CodecFamilyHEVC,

	// VP9 encoders
	"libvpx-vp9": CodecFamilyVP9,
	"vp9_vaapi":  CodecFamilyVP9,
	"vp9_qsv":    CodecFamilyVP9,

	// AV1 encoders
	"libaom-av1":  CodecFamilyAV1,
	"libsvtav1":   CodecFamilyAV1,
	"av1_nvenc":   CodecFamilyAV1,
	"av1_qsv":     CodecFamilyAV1,
	"av1_vaapi":   CodecFamilyAV1,
	"librav1e":    CodecFamilyAV1,

	// Audio encoders
	"aac":        CodecFamilyAAC,
	"libfdk_aac": CodecFamilyAAC,
	"ac3":        CodecFamilyAC3,
	"eac3":       CodecFamilyEAC3,
	"libmp3lame": CodecFamilyMP3,
	"libopus":    CodecFamilyOpus,
}

// GetCodecFamily returns the codec family for an encoder name
func GetCodecFamily(encoder string) CodecFamily {
	encoder = strings.ToLower(encoder)
	if family, ok := encoderToCodecFamily[encoder]; ok {
		return family
	}
	// Try to infer from encoder name
	if strings.Contains(encoder, "264") || strings.Contains(encoder, "avc") {
		return CodecFamilyH264
	}
	if strings.Contains(encoder, "265") || strings.Contains(encoder, "hevc") {
		return CodecFamilyHEVC
	}
	if strings.Contains(encoder, "vp9") {
		return CodecFamilyVP9
	}
	if strings.Contains(encoder, "av1") {
		return CodecFamilyAV1
	}
	return CodecFamilyUnknown
}

// GetVideoBitstreamFilter returns the appropriate video bitstream filter
// for converting from a source codec to a target output format.
//
// IMPORTANT: The isCopying parameter determines whether video is being copied or transcoded:
// - When COPYING (isCopying=true): BSF may be needed to convert between container formats
//   (e.g., h264_mp4toannexb converts AVCC from MP4 to Annex B for MPEG-TS)
// - When TRANSCODING (isCopying=false): The encoder outputs the correct format directly,
//   and FFmpeg's muxer handles it. Adding BSF would corrupt the stream.
func GetVideoBitstreamFilter(codecFamily CodecFamily, outputFormat OutputFormatType, isCopying bool) BitstreamFilterInfo {
	// When transcoding (encoding), the encoder and muxer handle format correctly.
	// BSF is only needed when copying to convert between container formats.
	if !isCopying {
		return BitstreamFilterInfo{
			Reason: "Transcoding: encoder outputs correct format for muxer",
		}
	}

	switch outputFormat {
	case FormatMPEGTS, FormatHLS:
		// Following m3u-proxy's approach: no bitstream filters for MPEG-TS.
		// FFmpeg's muxer handles the format conversion internally.
		// The -mpegts_copyts and -avoid_negative_ts flags handle timestamp preservation.
		return BitstreamFilterInfo{
			Reason: "MPEG-TS: no BSF needed (m3u-proxy proven approach)",
		}
	case FormatFLV, FormatMP4:
		// FLV and MP4 use AVCC format natively, no video BSF needed
		return BitstreamFilterInfo{
			Reason: "FLV/MP4 use AVCC format natively",
		}
	case FormatMKV, FormatWebM:
		// Matroska handles both formats, no BSF typically needed
		return BitstreamFilterInfo{
			Reason: "Matroska handles both AVCC and Annex B formats",
		}
	}

	return BitstreamFilterInfo{}
}

// GetAudioBitstreamFilter returns the appropriate audio bitstream filter
func GetAudioBitstreamFilter(codecFamily CodecFamily, outputFormat OutputFormatType) BitstreamFilterInfo {
	switch outputFormat {
	case FormatFLV, FormatMP4:
		// AAC in FLV/MP4 needs ASC format (convert from ADTS if coming from MPEG-TS)
		if codecFamily == CodecFamilyAAC {
			return BitstreamFilterInfo{
				AudioBSF: "aac_adtstoasc",
				Reason:   "Convert AAC from ADTS (MPEG-TS) to ASC (MP4/FLV) format",
			}
		}
	case FormatMPEGTS, FormatHLS:
		// MPEG-TS uses ADTS for AAC which is the FFmpeg default
		// No BSF needed
		return BitstreamFilterInfo{
			Reason: "MPEG-TS uses ADTS format for AAC which is default",
		}
	}

	return BitstreamFilterInfo{}
}

// GetBitstreamFilters returns both video and audio bitstream filters needed
// for a given codec and output format combination.
// isCopyingVideo indicates whether video is being copied (true) or transcoded (false).
func GetBitstreamFilters(videoCodecFamily, audioCodecFamily CodecFamily, outputFormat OutputFormatType, isCopyingVideo bool) BitstreamFilterInfo {
	videoBSF := GetVideoBitstreamFilter(videoCodecFamily, outputFormat, isCopyingVideo)
	audioBSF := GetAudioBitstreamFilter(audioCodecFamily, outputFormat)

	return BitstreamFilterInfo{
		VideoBSF: videoBSF.VideoBSF,
		AudioBSF: audioBSF.AudioBSF,
		Reason:   combineReasons(videoBSF.Reason, audioBSF.Reason),
	}
}

func combineReasons(video, audio string) string {
	if video != "" && audio != "" {
		return video + "; " + audio
	}
	if video != "" {
		return video
	}
	return audio
}

// SourceCodecInfo contains detected codec information from a stream
type SourceCodecInfo struct {
	VideoCodec  string      // e.g., "h264", "hevc"
	AudioCodec  string      // e.g., "aac", "ac3"
	VideoFamily CodecFamily
	AudioFamily CodecFamily
	Resolution  string      // e.g., "1920x1080"
	FrameRate   string      // e.g., "25"
}

// CodecDetector handles codec detection using ffprobe
type CodecDetector struct {
	ffprobePath string
	mu          sync.Mutex
}

// NewCodecDetector creates a new codec detector
func NewCodecDetector(ffprobePath string) *CodecDetector {
	return &CodecDetector{
		ffprobePath: ffprobePath,
	}
}

// DetectSourceCodecs probes a stream URL to detect its codecs
// This is essential for copy mode where we need to know the source codec
// to apply the correct bitstream filter
func (d *CodecDetector) DetectSourceCodecs(ctx context.Context, streamURL string) (*SourceCodecInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Use ffprobe to get stream information
	args := []string{
		"-v", "quiet",
		"-show_streams",
		"-select_streams", "v:0", // Get first video stream
		"-show_entries", "stream=codec_name,width,height,r_frame_rate",
		"-of", "default=noprint_wrappers=1",
		"-analyzeduration", "5000000", // 5 seconds
		"-probesize", "5000000",       // 5MB
		streamURL,
	}

	cmd := exec.CommandContext(ctx, d.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	info := &SourceCodecInfo{}

	// Parse output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

		switch key {
		case "codec_name":
			info.VideoCodec = value
			info.VideoFamily = mapFFprobeCodecToFamily(value)
		case "width":
			info.Resolution = value
		case "height":
			if info.Resolution != "" {
				info.Resolution += "x" + value
			}
		case "r_frame_rate":
			// Parse frame rate like "25/1" to "25"
			if idx := strings.Index(value, "/"); idx > 0 {
				info.FrameRate = value[:idx]
			} else {
				info.FrameRate = value
			}
		}
	}

	// Now get audio stream info
	audioArgs := []string{
		"-v", "quiet",
		"-show_streams",
		"-select_streams", "a:0", // Get first audio stream
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1",
		"-analyzeduration", "5000000",
		"-probesize", "5000000",
		streamURL,
	}

	audioCmd := exec.CommandContext(ctx, d.ffprobePath, audioArgs...)
	audioOutput, err := audioCmd.Output()
	if err == nil {
		// Parse audio output
		audioLines := strings.Split(string(audioOutput), "\n")
		for _, line := range audioLines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "codec_name" {
				info.AudioCodec = strings.TrimSpace(parts[1])
				info.AudioFamily = mapFFprobeCodecToFamily(info.AudioCodec)
				break
			}
		}
	}

	if info.VideoCodec == "" {
		return nil, fmt.Errorf("no video codec detected in stream")
	}

	slog.Debug("Detected source codecs",
		slog.String("url", streamURL),
		slog.String("video_codec", info.VideoCodec),
		slog.String("video_family", string(info.VideoFamily)),
		slog.String("audio_codec", info.AudioCodec),
		slog.String("resolution", info.Resolution))

	return info, nil
}

// mapFFprobeCodecToFamily maps ffprobe codec names to codec families
func mapFFprobeCodecToFamily(codecName string) CodecFamily {
	codecName = strings.ToLower(codecName)

	// Direct mappings
	switch codecName {
	case "h264", "avc", "avc1":
		return CodecFamilyH264
	case "hevc", "h265", "hev1", "hvc1":
		return CodecFamilyHEVC
	case "vp9", "vp09":
		return CodecFamilyVP9
	case "av1", "av01":
		return CodecFamilyAV1
	case "aac", "mp4a":
		return CodecFamilyAAC
	case "ac3", "ac-3", "a52":
		return CodecFamilyAC3
	case "eac3", "ec-3":
		return CodecFamilyEAC3
	case "mp3", "mp3float":
		return CodecFamilyMP3
	case "opus":
		return CodecFamilyOpus
	}

	// Pattern matching for variations
	if strings.Contains(codecName, "264") || strings.Contains(codecName, "avc") {
		return CodecFamilyH264
	}
	if strings.Contains(codecName, "265") || strings.Contains(codecName, "hevc") {
		return CodecFamilyHEVC
	}

	return CodecFamilyUnknown
}

// ApplyBitstreamFilters adds the appropriate bitstream filter arguments to a CommandBuilder
func ApplyBitstreamFilters(builder *CommandBuilder, bsfInfo BitstreamFilterInfo) *CommandBuilder {
	if bsfInfo.VideoBSF != "" {
		builder.OutputArgs("-bsf:v", bsfInfo.VideoBSF)
		slog.Debug("Applying video bitstream filter",
			slog.String("bsf_v", bsfInfo.VideoBSF),
			slog.String("reason", bsfInfo.Reason))
	}
	if bsfInfo.AudioBSF != "" {
		builder.OutputArgs("-bsf:a", bsfInfo.AudioBSF)
		slog.Debug("Applying audio bitstream filter",
			slog.String("bsf_a", bsfInfo.AudioBSF),
			slog.String("reason", bsfInfo.Reason))
	}
	return builder
}

// RequiresAnnexBConversion returns true if the output format requires Annex B NAL format
func RequiresAnnexBConversion(outputFormat OutputFormatType) bool {
	switch outputFormat {
	case FormatMPEGTS, FormatHLS:
		return true
	default:
		return false
	}
}

// ParseOutputFormat converts a string to OutputFormatType
func ParseOutputFormat(format string) OutputFormatType {
	format = strings.ToLower(format)
	switch format {
	case "mpegts", "ts":
		return FormatMPEGTS
	case "hls", "m3u8":
		return FormatHLS
	case "flv":
		return FormatFLV
	case "mp4":
		return FormatMP4
	case "matroska", "mkv":
		return FormatMKV
	case "webm":
		return FormatWebM
	default:
		return FormatUnknown
	}
}

// IsHardwareEncoder returns true if the encoder is a hardware encoder
func IsHardwareEncoder(encoder string) bool {
	encoder = strings.ToLower(encoder)
	hwSuffixes := []string{"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf", "_mf", "_omx", "_v4l2m2m"}
	for _, suffix := range hwSuffixes {
		if strings.HasSuffix(encoder, suffix) {
			return true
		}
	}
	return false
}

// ValidateBitstreamFilterAvailable checks if a bitstream filter is available in FFmpeg
func ValidateBitstreamFilterAvailable(ctx context.Context, ffmpegPath, filterName string) bool {
	if filterName == "" {
		return true
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-bsfs")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("Failed to list bitstream filters", slog.Any("error", err))
		return true // Assume available
	}

	pattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(filterName) + `\s*$`)
	return pattern.Match(output)
}
