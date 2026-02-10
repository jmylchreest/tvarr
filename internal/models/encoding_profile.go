package models

import (
	"strings"

	"gorm.io/gorm"
)

// QualityPreset defines encoding quality levels that map to FFmpeg parameters.
type QualityPreset string

const (
	// QualityPresetLow provides bandwidth-optimized encoding (CRF 28, ~2Mbps max, 128k audio).
	// Best for: Mobile devices, low bandwidth connections.
	QualityPresetLow QualityPreset = "low"

	// QualityPresetMedium provides balanced quality and bandwidth (CRF 23, ~5Mbps max, 192k audio).
	// Best for: General streaming, most use cases.
	QualityPresetMedium QualityPreset = "medium"

	// QualityPresetHigh provides high quality streaming (CRF 20, ~10Mbps max, 256k audio).
	// Best for: High-quality home viewing, modern devices.
	QualityPresetHigh QualityPreset = "high"

	// QualityPresetUltra provides maximum quality (CRF 16, no bitrate cap, 320k audio).
	// Best for: Maximum quality where bandwidth is not a concern.
	QualityPresetUltra QualityPreset = "ultra"
)

// EncodingParams contains FFmpeg encoding parameters derived from a quality preset.
type EncodingParams struct {
	CRF          int    // Constant Rate Factor (lower = better quality)
	Maxrate      string // Maximum bitrate (empty = no limit)
	Bufsize      string // VBV buffer size
	AudioBitrate string // Audio bitrate
	VideoPreset  string // FFmpeg preset (ultrafast, fast, medium, slow)
}

// GetEncodingParams returns FFmpeg parameters for this quality preset.
func (p QualityPreset) GetEncodingParams() EncodingParams {
	switch p {
	case QualityPresetLow:
		return EncodingParams{CRF: 28, Maxrate: "2M", Bufsize: "4M", AudioBitrate: "128k", VideoPreset: "fast"}
	case QualityPresetMedium:
		return EncodingParams{CRF: 23, Maxrate: "5M", Bufsize: "10M", AudioBitrate: "192k", VideoPreset: "medium"}
	case QualityPresetHigh:
		return EncodingParams{CRF: 20, Maxrate: "10M", Bufsize: "20M", AudioBitrate: "256k", VideoPreset: "medium"}
	case QualityPresetUltra:
		return EncodingParams{CRF: 16, Maxrate: "", Bufsize: "", AudioBitrate: "320k", VideoPreset: "slow"}
	default:
		// Default to medium if unknown preset
		return EncodingParams{CRF: 23, Maxrate: "5M", Bufsize: "10M", AudioBitrate: "192k", VideoPreset: "medium"}
	}
}

// IsValid returns true if this is a recognized quality preset.
func (p QualityPreset) IsValid() bool {
	switch p {
	case QualityPresetLow, QualityPresetMedium, QualityPresetHigh, QualityPresetUltra:
		return true
	default:
		return false
	}
}

// ValidQualityPresets returns all valid quality preset values.
func ValidQualityPresets() []QualityPreset {
	return []QualityPreset{QualityPresetLow, QualityPresetMedium, QualityPresetHigh, QualityPresetUltra}
}

// EncodingProfile defines a transcoding profile for stream relay.
// It provides a simplified interface with quality presets while allowing
// advanced users to override with custom FFmpeg flags.
type EncodingProfile struct {
	BaseModel

	// Name is a unique identifier for this profile.
	Name string `gorm:"uniqueIndex;not null;size:100" json:"name"`

	// Description explains what this profile is for.
	Description string `gorm:"size:500" json:"description,omitempty"`

	// TargetVideoCodec is the video codec to transcode to.
	// Valid values: h264, h265, vp9, av1
	TargetVideoCodec VideoCodec `gorm:"size:20;default:'h264'" json:"target_video_codec"`

	// TargetAudioCodec is the audio codec to transcode to.
	// Valid values: aac, opus, ac3, eac3, mp3
	TargetAudioCodec AudioCodec `gorm:"size:20;default:'aac'" json:"target_audio_codec"`

	// QualityPreset determines encoding quality and bitrate settings.
	// Valid values: low, medium, high, ultra
	QualityPreset QualityPreset `gorm:"size:20;default:'medium'" json:"quality_preset"`

	// HWAccel is the hardware acceleration type to use.
	// Valid values: auto, none, cuda, vaapi, qsv, videotoolbox
	// Set to "none" if providing custom hardware acceleration in GlobalFlags/InputFlags.
	HWAccel HWAccelType `gorm:"size:20;default:'auto'" json:"hw_accel"`

	// Custom FFmpeg flags - when provided, these REPLACE auto-generated flags.
	// Leave empty to use auto-generated flags based on codec/quality settings.

	// GlobalFlags are FFmpeg flags placed at the very start of the command.
	// Example: "-loglevel warning -stats"
	// Auto-generated: "-hide_banner -loglevel info -y"
	GlobalFlags string `gorm:"size:500" json:"global_flags,omitempty"`

	// InputFlags are FFmpeg flags placed before the -i input.
	// Example: "-hwaccel cuda -hwaccel_device 0 -re"
	// Auto-generated: hardware acceleration, analyzeduration, probesize, reconnect settings
	InputFlags string `gorm:"size:500" json:"input_flags,omitempty"`

	// OutputFlags are FFmpeg flags placed after the -i input.
	// Example: "-c:v libx264 -preset fast -crf 23 -c:a aac -b:a 192k"
	// Auto-generated: codec settings, bitrate, preset, filters, muxing options
	OutputFlags string `gorm:"size:1000" json:"output_flags,omitempty"`

	// IsDefault indicates if this is the default profile when none is specified.
	IsDefault bool `gorm:"default:false" json:"is_default"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only Enabled can be toggled for system profiles.
	IsSystem bool `gorm:"default:false" json:"is_system"`

	// Enabled indicates if this profile can be used.
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	Enabled *bool `gorm:"default:true" json:"enabled"`
}

// TableName returns the table name for EncodingProfile.
func (EncodingProfile) TableName() string {
	return "encoding_profiles"
}

// Validate performs basic validation on the profile.
func (p *EncodingProfile) Validate() error {
	if p.Name == "" {
		return ErrEncodingProfileNameRequired
	}
	// Only validate codecs if not using custom output flags
	if p.OutputFlags == "" {
		if !p.isValidVideoCodec() {
			return ErrEncodingProfileInvalidVideoCodec
		}
		if !p.isValidAudioCodec() {
			return ErrEncodingProfileInvalidAudioCodec
		}
		if !p.QualityPreset.IsValid() {
			return ErrEncodingProfileInvalidQualityPreset
		}
	}
	// Only validate HWAccel if not using custom input flags
	if p.InputFlags == "" && !p.isValidHWAccel() {
		return ErrEncodingProfileInvalidHWAccel
	}
	return nil
}

// isValidVideoCodec returns true if the target video codec is valid for encoding profiles.
// Valid codecs are: h264, h265, vp9, av1
// Note: "auto", "copy", and "none" are NOT valid for encoding profiles since they
// represent passthrough or dynamic behavior, not actual encoding targets.
func (p *EncodingProfile) isValidVideoCodec() bool {
	switch p.TargetVideoCodec {
	case VideoCodecH264, VideoCodecH265, VideoCodecVP9, VideoCodecAV1:
		return true
	default:
		return false
	}
}

// isValidAudioCodec returns true if the target audio codec is valid for encoding profiles.
// Valid codecs are: aac, opus, ac3, eac3, mp3
// Note: "auto", "copy", and "none" are NOT valid for encoding profiles.
func (p *EncodingProfile) isValidAudioCodec() bool {
	switch p.TargetAudioCodec {
	case AudioCodecAAC, AudioCodecOpus, AudioCodecAC3, AudioCodecEAC3, AudioCodecMP3:
		return true
	default:
		return false
	}
}

// isValidHWAccel returns true if the hardware acceleration type is valid.
func (p *EncodingProfile) isValidHWAccel() bool {
	switch p.HWAccel {
	case HWAccelAuto, HWAccelNone, HWAccelNVDEC, HWAccelQSV, HWAccelVAAPI, HWAccelVT:
		return true
	case "": // Empty defaults to auto
		return true
	default:
		return false
	}
}

// BeforeCreate is a GORM hook that validates the profile and generates ULID.
func (p *EncodingProfile) BeforeCreate(tx *gorm.DB) error {
	if err := p.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	// Set defaults for empty values (only if not using custom flags)
	if p.OutputFlags == "" {
		if p.TargetVideoCodec == "" {
			p.TargetVideoCodec = VideoCodecH264
		}
		if p.TargetAudioCodec == "" {
			p.TargetAudioCodec = AudioCodecAAC
		}
		if p.QualityPreset == "" {
			p.QualityPreset = QualityPresetMedium
		}
	}
	if p.InputFlags == "" && p.HWAccel == "" {
		p.HWAccel = HWAccelAuto
	}
	return p.Validate()
}

// BeforeUpdate is a GORM hook that validates the profile before update.
func (p *EncodingProfile) BeforeUpdate(tx *gorm.DB) error {
	return p.Validate()
}

// HasCustomFlags returns true if any custom FFmpeg flags are set.
func (p *EncodingProfile) HasCustomFlags() bool {
	return p.GlobalFlags != "" || p.InputFlags != "" || p.OutputFlags != ""
}

// HasCustomGlobalFlags returns true if custom global flags are set.
func (p *EncodingProfile) HasCustomGlobalFlags() bool {
	return p.GlobalFlags != ""
}

// HasCustomInputFlags returns true if custom input flags are set.
func (p *EncodingProfile) HasCustomInputFlags() bool {
	return p.InputFlags != ""
}

// HasCustomOutputFlags returns true if custom output flags are set.
func (p *EncodingProfile) HasCustomOutputFlags() bool {
	return p.OutputFlags != ""
}

// NeedsTranscode returns true if this profile requires transcoding (non-copy codecs).
// EncodingProfiles always transcode - they define target codecs for encoding.
func (p *EncodingProfile) NeedsTranscode() bool {
	// EncodingProfiles always transcode - that's their purpose
	// If using custom output flags, assume transcoding is needed
	if p.OutputFlags != "" {
		return true
	}
	// Otherwise check if codecs are set (they should be for a valid profile)
	return p.TargetVideoCodec != "" || p.TargetAudioCodec != ""
}

// UsesHardwareAccel returns true if hardware acceleration is enabled.
func (p *EncodingProfile) UsesHardwareAccel() bool {
	// If using custom input flags, user manages hwaccel
	if p.InputFlags != "" {
		return false // Can't determine, assume user handles it
	}
	return p.HWAccel != "" && p.HWAccel != HWAccelNone
}

// RequiresFMP4 returns true if the profile's codecs require fMP4 container.
// VP9, AV1, and Opus codecs are not supported in MPEG-TS containers.
func (p *EncodingProfile) RequiresFMP4() bool {
	return p.TargetVideoCodec.IsFMP4Only() || p.TargetAudioCodec.IsFMP4Only()
}

// DetermineContainer returns the effective container format for this profile.
// VP9/AV1/Opus require fMP4; H.264/H.265 with AAC/AC3/EAC3/MP3 default to fMP4
// for modern compatibility but can also work with MPEG-TS.
func (p *EncodingProfile) DetermineContainer() ContainerFormat {
	if p.RequiresFMP4() {
		return ContainerFormatFMP4
	}
	// Default to fMP4 for modern compatibility
	return ContainerFormatFMP4
}

// GetEncodingParams returns the FFmpeg encoding parameters for this profile's quality preset.
func (p *EncodingProfile) GetEncodingParams() EncodingParams {
	return p.QualityPreset.GetEncodingParams()
}

// GetVideoBitrate returns the video bitrate in kbps based on quality preset.
// Returns 0 if using custom output flags (user manages bitrate).
func (p *EncodingProfile) GetVideoBitrate() int {
	if p.OutputFlags != "" {
		return 0
	}
	params := p.GetEncodingParams()
	return parseRateToKbps(params.Maxrate)
}

// GetAudioBitrate returns the audio bitrate in kbps based on quality preset.
// Returns 0 if using custom output flags (user manages bitrate).
func (p *EncodingProfile) GetAudioBitrate() int {
	if p.OutputFlags != "" {
		return 0
	}
	params := p.GetEncodingParams()
	return parseRateToKbps(params.AudioBitrate)
}

// GetVideoPreset returns the FFmpeg preset for this quality level.
// Returns empty string if using custom output flags.
func (p *EncodingProfile) GetVideoPreset() string {
	if p.OutputFlags != "" {
		return ""
	}
	params := p.GetEncodingParams()
	return params.VideoPreset
}

// GetVideoEncoder returns the FFmpeg encoder name for the target video codec.
func (p *EncodingProfile) GetVideoEncoder() string {
	return p.TargetVideoCodec.GetFFmpegEncoder(p.HWAccel)
}

// GetAudioEncoder returns the FFmpeg encoder name for the target audio codec.
func (p *EncodingProfile) GetAudioEncoder() string {
	return p.TargetAudioCodec.GetFFmpegEncoder()
}

// Clone creates a copy of the profile suitable for customization.
// The caller should set Name, Description, etc. on the returned clone.
func (p *EncodingProfile) Clone() *EncodingProfile {
	clone := *p
	clone.ID = ULID{}
	clone.Name = "" // Must be set by caller
	clone.Description = ""
	clone.IsDefault = false
	clone.IsSystem = false // Clone is always user-owned
	clone.CreatedAt = Now()
	clone.UpdatedAt = Now()
	return &clone
}

// DefaultFlags holds the auto-generated FFmpeg flags for an encoding profile.
// These are shown as placeholder text in the UI when no custom flags are set.
type DefaultFlags struct {
	// GlobalFlags are flags placed at the very start of the FFmpeg command.
	GlobalFlags string `json:"global_flags"`
	// InputFlags are flags placed before the -i input.
	InputFlags string `json:"input_flags"`
	// OutputFlags are flags placed after the -i input.
	OutputFlags string `json:"output_flags"`
}

// GenerateDefaultFlags returns the auto-generated FFmpeg flags for this profile.
// These can be shown as placeholder text in the UI to help users understand
// what flags would be used if they don't provide custom flags.
func (p *EncodingProfile) GenerateDefaultFlags() DefaultFlags {
	return DefaultFlags{
		GlobalFlags: p.generateDefaultGlobalFlags(),
		InputFlags:  p.generateDefaultInputFlags(),
		OutputFlags: p.generateDefaultOutputFlags(),
	}
}

// generateDefaultGlobalFlags returns auto-generated global flags.
// These are placed at the very start of the FFmpeg command.
func (p *EncodingProfile) generateDefaultGlobalFlags() string {
	// Standard global flags for transcoding
	return "-hide_banner -stats"
}

// generateDefaultInputFlags returns auto-generated input flags.
// These are placed before the -i input.
func (p *EncodingProfile) generateDefaultInputFlags() string {
	var flags []string

	// Hardware acceleration based on HWAccel setting
	switch p.HWAccel {
	case HWAccelVAAPI:
		flags = append(flags, "-init_hw_device vaapi=hw", "-hwaccel vaapi")
	case HWAccelNVDEC:
		flags = append(flags, "-init_hw_device cuda=hw", "-hwaccel cuda")
	case HWAccelQSV:
		flags = append(flags, "-init_hw_device qsv=hw", "-hwaccel qsv")
	case HWAccelVT:
		flags = append(flags, "-hwaccel videotoolbox")
	case HWAccelAuto:
		// Auto-detection is done at runtime, no static flags
		flags = append(flags, "# hwaccel auto-detected at runtime")
	}

	// Input format and analysis settings for pipe input
	flags = append(flags, "-f mpegts", "-analyzeduration 500000", "-probesize 500000")

	// Join all flags with spaces
	var result strings.Builder
	for i, f := range flags {
		if i > 0 {
			result.WriteString(" ")
		}
		result.WriteString(f)
	}
	return result.String()
}

// generateDefaultOutputFlags returns auto-generated output flags.
// These are placed after the -i input.
func (p *EncodingProfile) generateDefaultOutputFlags() string {
	var flags []string

	// Stream mapping
	flags = append(flags, "-map 0:v:0", "-map 0:a:0?")

	// Video codec
	videoEncoder := p.GetVideoEncoder()
	if videoEncoder != "" {
		flags = append(flags, "-c:v "+videoEncoder)

		// Add hardware upload filter if using HW encoder
		if p.UsesHardwareAccel() && isHardwareEncoder(videoEncoder) {
			switch p.HWAccel {
			case HWAccelVAAPI:
				flags = append(flags, "-vf format=nv12,hwupload")
			case HWAccelNVDEC:
				flags = append(flags, "-vf format=nv12,hwupload_cuda")
			case HWAccelQSV:
				flags = append(flags, "-vf format=nv12,hwupload=extra_hw_frames=64")
			}
		}
	} else {
		flags = append(flags, "-c:v copy")
	}

	// Quality settings from preset
	params := p.GetEncodingParams()
	if params.VideoPreset != "" {
		flags = append(flags, "-preset "+params.VideoPreset)
	}
	if params.Maxrate != "" {
		flags = append(flags, "-maxrate "+params.Maxrate, "-bufsize "+params.Bufsize)
	}

	// Audio codec
	audioEncoder := p.GetAudioEncoder()
	if audioEncoder != "" {
		flags = append(flags, "-c:a "+audioEncoder)
		if params.AudioBitrate != "" {
			flags = append(flags, "-b:a "+params.AudioBitrate)
		}
	} else {
		flags = append(flags, "-c:a copy")
	}

	// Output format flags for MPEG-TS streaming
	flags = append(flags, "-f mpegts", "-mpegts_copyts 1", "-avoid_negative_ts disabled")
	flags = append(flags, "-flush_packets 1", "-muxdelay 0")

	// Join all flags with spaces
	var result strings.Builder
	for i, f := range flags {
		if i > 0 {
			result.WriteString(" ")
		}
		result.WriteString(f)
	}
	return result.String()
}

// isHardwareEncoder returns true if the encoder name indicates a hardware encoder.
func isHardwareEncoder(encoder string) bool {
	// Common hardware encoder suffixes/prefixes
	hwEncoders := []string{
		"_nvenc", "_qsv", "_vaapi", "_videotoolbox", "_amf",
		"h264_nvenc", "hevc_nvenc", "av1_nvenc",
		"h264_qsv", "hevc_qsv", "vp9_qsv", "av1_qsv",
		"h264_vaapi", "hevc_vaapi", "vp9_vaapi", "av1_vaapi",
		"h264_videotoolbox", "hevc_videotoolbox",
	}
	for _, hw := range hwEncoders {
		if encoder == hw || containsSuffix(encoder, hw) {
			return true
		}
	}
	return false
}

// containsSuffix checks if s contains suffix as a suffix.
func containsSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

// parseRateToKbps converts a rate string like "5M", "192k" to kbps integer.
func parseRateToKbps(rate string) int {
	if rate == "" {
		return 0
	}
	// Simple parsing for common formats
	var value int
	var suffix string
	_, _ = parseRateSuffix(rate, &value, &suffix)

	switch suffix {
	case "M", "m":
		return value * 1000
	case "K", "k":
		return value
	default:
		// Assume kbps if no suffix
		return value
	}
}

// parseRateSuffix extracts the numeric value and suffix from a rate string.
func parseRateSuffix(rate string, value *int, suffix *string) (int, error) {
	n := 0
	for i, c := range rate {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			*value = n
			*suffix = rate[i:]
			return n, nil
		}
	}
	*value = n
	*suffix = ""
	return n, nil
}
