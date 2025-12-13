package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// OutputFormatType represents output container formats.
// Deprecated: Use codec.OutputFormat instead.
type OutputFormatType = codec.OutputFormat

// Output format constants - use codec package.
const (
	FormatMPEGTS  = codec.FormatMPEGTS
	FormatHLS     = codec.FormatHLS
	FormatFLV     = codec.FormatFLV
	FormatMP4     = codec.FormatMP4
	FormatFMP4    = codec.FormatFMP4
	FormatMKV     = codec.FormatMKV
	FormatWebM    = codec.FormatWebM
	FormatUnknown = codec.FormatUnknown
)

// CodecFamily represents the base codec family (independent of encoder implementation).
// This is kept for backwards compatibility - new code should use codec.Video or codec.Audio.
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

// GetCodecFamily returns the codec family for an encoder name.
// Uses the unified codec package for normalization.
func GetCodecFamily(encoder string) CodecFamily {
	if encoder == "" || encoder == "copy" {
		return CodecFamilyUnknown
	}

	// Use codec package to normalize the encoder name
	normalized := codec.Normalize(encoder)

	// Map normalized names to codec families
	switch normalized {
	case "h264":
		return CodecFamilyH264
	case "h265":
		return CodecFamilyHEVC
	case "vp9":
		return CodecFamilyVP9
	case "av1":
		return CodecFamilyAV1
	case "aac":
		return CodecFamilyAAC
	case "ac3":
		return CodecFamilyAC3
	case "eac3":
		return CodecFamilyEAC3
	case "mp3":
		return CodecFamilyMP3
	case "opus":
		return CodecFamilyOpus
	default:
		return CodecFamilyUnknown
	}
}

// GetVideoBitstreamFilter returns the appropriate video bitstream filter
// for converting from a source codec to a target output format.
//
// IMPORTANT: The isCopying parameter determines whether video is being copied or transcoded:
//   - When COPYING (isCopying=true): BSF may be needed to convert between container formats
//     (e.g., h264_mp4toannexb converts AVCC from MP4 to Annex B for MPEG-TS)
//   - When TRANSCODING (isCopying=false): The encoder outputs the correct format directly,
//     and FFmpeg's muxer handles it. Adding BSF would corrupt the stream.
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
		// For H.264/HEVC copy mode to MPEG-TS, use dump_extra to ensure SPS/PPS are included
		// This is critical for live streams where the player may join mid-stream and miss
		// the parameter sets that were sent earlier with a keyframe.
		// dump_extra=freq=keyframe outputs SPS/PPS with every keyframe for robust playback.
		switch codecFamily {
		case CodecFamilyH264:
			return BitstreamFilterInfo{
				VideoBSF: "dump_extra=freq=keyframe",
				Reason:   "H.264 copy to MPEG-TS: ensure SPS/PPS included for mid-stream joins",
			}
		case CodecFamilyHEVC:
			return BitstreamFilterInfo{
				VideoBSF: "dump_extra=freq=keyframe",
				Reason:   "HEVC copy to MPEG-TS: ensure VPS/SPS/PPS included for mid-stream joins",
			}
		default:
			return BitstreamFilterInfo{
				Reason: "MPEG-TS: no BSF needed for this codec",
			}
		}
	case FormatFLV, FormatMP4, FormatFMP4:
		// FLV, MP4, and fMP4 use AVCC format natively, no video BSF needed
		return BitstreamFilterInfo{
			Reason: "FLV/MP4/fMP4 use AVCC format natively",
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
	VideoCodec  string // e.g., "h264", "hevc"
	AudioCodec  string // e.g., "aac", "ac3"
	VideoFamily CodecFamily
	AudioFamily CodecFamily
	Resolution  string // e.g., "1920x1080"
	FrameRate   string // e.g., "25"
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
		"-probesize", "5000000", // 5MB
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
			info.VideoFamily = MapFFprobeCodecToFamily(value)
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
				info.AudioFamily = MapFFprobeCodecToFamily(info.AudioCodec)
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

// MapFFprobeCodecToFamily maps ffprobe codec names to codec families.
// Use this when you have a codec name from ffprobe output (e.g., "h264", "aac")
// and need to compare it with an encoder-based codec family.
// Uses the unified codec package for parsing.
func MapFFprobeCodecToFamily(codecName string) CodecFamily {
	// Use GetCodecFamily which internally uses codec.Normalize
	return GetCodecFamily(codecName)
}

// SourceMatchesTargetCodec checks if the source codec (from ffprobe) matches the target encoder.
// This is useful for determining if we can use copy mode instead of re-encoding.
// For example, if source is "h264" and target is "libx264" or "h264_nvenc", they match (same family).
func SourceMatchesTargetCodec(sourceCodec, targetEncoder string) bool {
	// If target is copy, obviously no encoding needed
	if targetEncoder == "copy" || targetEncoder == "" {
		return true
	}

	sourceFamily := MapFFprobeCodecToFamily(sourceCodec)
	targetFamily := GetCodecFamily(targetEncoder)

	// If either is unknown, can't determine match - default to transcoding
	if sourceFamily == CodecFamilyUnknown || targetFamily == CodecFamilyUnknown {
		return false
	}

	return sourceFamily == targetFamily
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

// RequiresAnnexBConversion returns true if the output format requires Annex B NAL format.
// Deprecated: Use outputFormat.RequiresAnnexB() instead.
func RequiresAnnexBConversion(outputFormat OutputFormatType) bool {
	return outputFormat.RequiresAnnexB()
}

// ParseOutputFormat converts a string to OutputFormatType.
// Deprecated: Use codec.ParseOutputFormat instead.
func ParseOutputFormat(format string) OutputFormatType {
	return codec.ParseOutputFormat(format)
}

// IsHardwareEncoder returns true if the encoder is a hardware encoder.
// Uses the unified codec package.
func IsHardwareEncoder(encoder string) bool {
	// codec.IsEncoder returns true for both lib* prefixed and hardware encoders.
	// We need to specifically check for hardware encoder suffixes.
	encoder = strings.ToLower(encoder)
	hwSuffixes := []string{"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf", "_mf", "_omx", "_v4l2m2m", "_cuvid"}
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
