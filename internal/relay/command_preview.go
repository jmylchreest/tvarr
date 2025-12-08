package relay

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// CommandPreview represents a preview of an FFmpeg command
type CommandPreview struct {
	Command         string   // Full command as a single string
	Args            []string // Command arguments as an array
	Binary          string   // FFmpeg binary path
	InputURL        string   // The input URL used
	OutputURL       string   // The output URL used
	VideoCodec      string   // Video codec that will be used
	AudioCodec      string   // Audio codec that will be used
	HWAccel         string   // Hardware acceleration method
	BitstreamFilter string   // Video bitstream filter applied
	Notes           []string // Notes about the command configuration
}

// GenerateCommandPreview generates a preview of the FFmpeg command for a profile.
// This mirrors the actual command generation in session.go runFFmpegPipeline.
func GenerateCommandPreview(profile *models.RelayProfile, inputURL, outputURL string) *CommandPreview {
	preview := &CommandPreview{
		Binary:    "ffmpeg",
		InputURL:  inputURL,
		OutputURL: outputURL,
		Notes:     make([]string, 0),
	}

	builder := ffmpeg.NewCommandBuilder("ffmpeg")

	// Following m3u-proxy's proven approach for flag order:
	// 1. Global flags (banner, loglevel)
	// 2. Hardware acceleration args (init_hw_device, hwaccel) - BEFORE input
	// 3. Input analysis args (analyzeduration, probesize)
	// 4. Custom input options
	// 5. Input (-i)
	// 6. Stream mapping
	// 7. Video codec and hwaccel filters
	// 8. Video settings
	// 9. Audio codec and settings
	// 10. Transport stream settings
	// 11. Custom output options
	// 12. Output

	// 1. GLOBAL FLAGS
	builder.HideBanner()
	builder.LogLevel("info")
	builder.Overwrite()

	// 2. HARDWARE ACCELERATION - Must be BEFORE input
	hwAccelType := ""
	isHWAccelTranscode := false

	if profile.HWAccel == models.HWAccelAuto {
		// For preview, show that auto mode will select the best available
		preview.HWAccel = "auto (will detect at runtime)"
		preview.Notes = append(preview.Notes, "Hardware acceleration: auto-detect best available (vaapi > cuda > qsv)")
	} else if profile.HWAccel != "" && profile.HWAccel != models.HWAccelNone {
		hwAccelType = string(profile.HWAccel)
		preview.HWAccel = hwAccelType
		preview.Notes = append(preview.Notes, fmt.Sprintf("Hardware acceleration: %s (forced)", hwAccelType))

		// Initialize hardware device first
		devicePath := profile.HWAccelDevice
		if devicePath == "" && profile.GpuIndex >= 0 {
			devicePath = fmt.Sprintf("%d", profile.GpuIndex)
		}
		builder.InitHWDevice(hwAccelType, devicePath)

		// Set hwaccel mode
		builder.HWAccel(hwAccelType)
		if devicePath != "" {
			builder.HWAccelDevice(devicePath)
		}
		if profile.HWAccelOutputFormat != "" {
			builder.HWAccelOutputFormat(profile.HWAccelOutputFormat)
		}

		// Hardware decoder codec
		if profile.HWAccelDecoderCodec != "" {
			builder.InputArgs("-c:v", profile.HWAccelDecoderCodec)
			preview.Notes = append(preview.Notes, fmt.Sprintf("Hardware decoder: %s", profile.HWAccelDecoderCodec))
		}

		// Extra hardware acceleration options
		if profile.HWAccelExtraOptions != "" {
			builder.ApplyCustomInputOptions(profile.HWAccelExtraOptions)
		}

		isHWAccelTranscode = profile.VideoCodec != models.VideoCodecCopy
	} else {
		preview.Notes = append(preview.Notes, "Hardware acceleration: disabled (software encoding)")
	}

	// 3. INPUT ANALYSIS FLAGS
	builder.InputArgs("-analyzeduration", "10000000") // 10 seconds
	builder.InputArgs("-probesize", "10000000")       // 10MB
	builder.Reconnect()                               // For network streams

	// 4. CUSTOM INPUT OPTIONS
	if profile.InputOptions != "" {
		builder.ApplyCustomInputOptions(profile.InputOptions)
		preview.Notes = append(preview.Notes, "Custom input options applied")
	}

	// 5. INPUT
	builder.Input(inputURL)

	// 6. STREAM MAPPING
	builder.OutputArgs("-map", "0:v:0")
	builder.OutputArgs("-map", "0:a:0?") // ? makes audio optional

	// 7. VIDEO CODEC
	if profile.VideoCodec != "" && profile.VideoCodec != models.VideoCodecNone {
		// Convert abstract codec type to actual FFmpeg encoder name
		videoEncoder := profile.VideoCodec.GetFFmpegEncoder(profile.HWAccel)
		if videoEncoder != "" {
			builder.VideoCodec(videoEncoder)
			preview.VideoCodec = videoEncoder
		}

		// Add hardware upload filter when transcoding with HW acceleration
		if isHWAccelTranscode && hwAccelType != "" {
			builder.HWUploadFilter(hwAccelType)
		}
	}

	// 8. VIDEO SETTINGS
	if profile.VideoBitrate > 0 {
		builder.VideoBitrate(fmt.Sprintf("%dk", profile.VideoBitrate))
	}
	if profile.VideoMaxrate > 0 {
		builder.OutputArgs("-maxrate", fmt.Sprintf("%dk", profile.VideoMaxrate))
		builder.OutputArgs("-bufsize", fmt.Sprintf("%dk", profile.VideoMaxrate*2))
	}
	if profile.VideoPreset != "" {
		builder.VideoPreset(profile.VideoPreset)
	}
	if profile.VideoWidth > 0 && profile.VideoHeight > 0 {
		builder.VideoScale(profile.VideoWidth, profile.VideoHeight)
	}

	// Filter complex (if specified)
	if profile.FilterComplex != "" {
		builder.ApplyFilterComplex(profile.FilterComplex)
		preview.Notes = append(preview.Notes, "Custom filter complex applied")
	}

	// 9. AUDIO CODEC AND SETTINGS
	if profile.AudioCodec != "" && profile.AudioCodec != models.AudioCodecNone {
		// Convert abstract codec type to actual FFmpeg encoder name
		audioEncoder := profile.AudioCodec.GetFFmpegEncoder()
		if audioEncoder != "" {
			builder.AudioCodec(audioEncoder)
			preview.AudioCodec = audioEncoder
		}
	}
	if profile.AudioBitrate > 0 {
		builder.AudioBitrate(fmt.Sprintf("%dk", profile.AudioBitrate))
	}
	if profile.AudioSampleRate > 0 {
		builder.AudioSampleRate(profile.AudioSampleRate)
	}
	if profile.AudioChannels > 0 {
		builder.AudioChannels(profile.AudioChannels)
	}

	// 10. BITSTREAM FILTERS AND TRANSPORT STREAM SETTINGS
	// Determine container format using DetermineContainer() for smart auto-selection
	containerFormat := ffmpeg.FormatMPEGTS
	container := profile.DetermineContainer()
	switch container {
	case models.ContainerFormatFMP4:
		containerFormat = ffmpeg.FormatFMP4
	case models.ContainerFormatMPEGTS:
		containerFormat = ffmpeg.FormatMPEGTS
	}

	// BSF is only needed when copying video, not when transcoding
	// Use FFmpeg encoder names for codec family detection
	videoEncoder := profile.VideoCodec.GetFFmpegEncoder(profile.HWAccel)
	audioEncoder := profile.AudioCodec.GetFFmpegEncoder()
	videoCodecFamily := ffmpeg.GetCodecFamily(videoEncoder)
	audioCodecFamily := ffmpeg.GetCodecFamily(audioEncoder)
	isVideoCopy := profile.VideoCodec == models.VideoCodecCopy
	bsfInfo := ffmpeg.GetBitstreamFilters(videoCodecFamily, audioCodecFamily, containerFormat, isVideoCopy)
	if bsfInfo.VideoBSF != "" {
		builder.VideoBitstreamFilter(bsfInfo.VideoBSF)
		preview.BitstreamFilter = bsfInfo.VideoBSF
		preview.Notes = append(preview.Notes, fmt.Sprintf("Video bitstream filter: %s (%s)", bsfInfo.VideoBSF, bsfInfo.Reason))
	}
	if bsfInfo.AudioBSF != "" {
		builder.AudioBitstreamFilter(bsfInfo.AudioBSF)
	}

	// Configure output format based on container
	switch containerFormat {
	case ffmpeg.FormatFMP4:
		// fMP4/CMAF output
		fragDuration := float64(DefaultSegmentDuration)
		if profile.SegmentDuration > 0 {
			fragDuration = float64(profile.SegmentDuration)
		}
		builder.FMP4Args(fragDuration).FlushPackets()
	case ffmpeg.FormatMPEGTS, ffmpeg.FormatHLS:
		builder.MpegtsArgs(). // Proper MPEG-TS flags (copyts, PIDs, etc.)
					FlushPackets().  // -flush_packets 1 - immediate output
					MuxDelay("0").   // -muxdelay 0 - zero muxing delay
					PatPeriod("0.1") // -pat_period 0.1 - frequent PAT/PMT
	default:
		builder.MpegtsArgs().FlushPackets().MuxDelay("0").PatPeriod("0.1")
	}

	// 11. CUSTOM OUTPUT OPTIONS (at the end, before output)
	if profile.OutputOptions != "" {
		builder.ApplyCustomOutputOptions(profile.OutputOptions)
		preview.Notes = append(preview.Notes, "Custom output options applied")
	}

	// 12. OUTPUT
	builder.Output(outputURL)

	// Build the command
	cmd := builder.Build()
	preview.Args = cmd.Args
	preview.Command = cmd.Binary + " " + strings.Join(cmd.Args, " ")

	// Add notes about copy mode
	if profile.VideoCodec == models.VideoCodecCopy && profile.AudioCodec == models.AudioCodecCopy {
		preview.Notes = append(preview.Notes, "Passthrough mode: No transcoding, lowest CPU usage")
	} else if profile.VideoCodec == models.VideoCodecCopy {
		preview.Notes = append(preview.Notes, "Video passthrough: Only audio is being transcoded")
	} else if profile.AudioCodec == models.AudioCodecCopy {
		preview.Notes = append(preview.Notes, "Audio passthrough: Only video is being transcoded")
	}

	return preview
}
