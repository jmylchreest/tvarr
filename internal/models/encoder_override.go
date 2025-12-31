package models

import (
	"strings"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// EncoderOverrideCodecType represents the type of codec (video or audio).
type EncoderOverrideCodecType string

const (
	EncoderOverrideCodecTypeVideo EncoderOverrideCodecType = "video"
	EncoderOverrideCodecTypeAudio EncoderOverrideCodecType = "audio"
)

// EncoderOverride represents a rule for overriding encoder selection based on conditions.
// This allows working around hardware encoder bugs (like AMD's hevc_vaapi with Mesa 21.1+)
// by forcing specific encoders when conditions match.
type EncoderOverride struct {
	BaseModel

	// Name is a human-readable name for this override rule.
	Name string `gorm:"size:255;not null" json:"name"`

	// Description provides additional details about why this override exists.
	Description string `gorm:"type:text" json:"description,omitempty"`

	// CodecType specifies whether this override applies to video or audio encoding.
	// Values: "video", "audio"
	CodecType EncoderOverrideCodecType `gorm:"size:10;not null" json:"codec_type"`

	// SourceCodec is the target codec that triggers this override.
	// For video: h264, h265, vp9, av1
	// For audio: aac, ac3, eac3, opus, mp3
	SourceCodec string `gorm:"size:50;not null" json:"source_codec"`

	// TargetEncoder is the encoder to use when this override matches.
	// Examples: libx265, h264_nvenc, libopus, aac
	TargetEncoder string `gorm:"size:100;not null" json:"target_encoder"`

	// HWAccelMatch optionally matches against the detected hardware accelerator.
	// Values: vaapi, cuda, qsv, videotoolbox, amf
	// Empty string matches all hardware accelerators.
	// This is only relevant for video overrides.
	HWAccelMatch string `gorm:"size:50" json:"hw_accel_match,omitempty"`

	// CPUMatch is an optional regex pattern to match against the CPU model name.
	// Example: "AMD" matches any AMD CPU, "AMD Ryzen AI 5" matches specific models.
	// Empty string matches all CPUs.
	CPUMatch string `gorm:"size:255" json:"cpu_match,omitempty"`

	// Priority determines evaluation order (higher = evaluated first).
	// System rules use priorities 100-999, leaving 1-99 for low-priority custom rules
	// and 1000+ for high-priority custom rules that should override system rules.
	Priority int `gorm:"default:100;index" json:"priority"`

	// IsEnabled determines if the override is active.
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	IsEnabled *bool `gorm:"default:true" json:"is_enabled"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only IsEnabled can be toggled for system rules.
	IsSystem bool `gorm:"default:false" json:"is_system"`
}

// TableName returns the table name for EncoderOverride.
func (EncoderOverride) TableName() string {
	return "encoder_overrides"
}

// Validate checks if the override configuration is valid.
func (o *EncoderOverride) Validate() error {
	if o.Name == "" {
		return ValidationError{Field: "name", Message: "name is required"}
	}
	if o.CodecType == "" {
		return ValidationError{Field: "codec_type", Message: "codec_type is required"}
	}
	if o.CodecType != EncoderOverrideCodecTypeVideo && o.CodecType != EncoderOverrideCodecTypeAudio {
		return ValidationError{Field: "codec_type", Message: "codec_type must be 'video' or 'audio'"}
	}
	if o.SourceCodec == "" {
		return ValidationError{Field: "source_codec", Message: "source_codec is required"}
	}
	if o.TargetEncoder == "" {
		return ValidationError{Field: "target_encoder", Message: "target_encoder is required"}
	}
	// Validate source codec based on codec type
	if o.CodecType == EncoderOverrideCodecTypeVideo {
		validVideoCodecs := []string{"h264", "h265", "hevc", "vp9", "av1"}
		if !containsIgnoreCase(validVideoCodecs, o.SourceCodec) {
			return ValidationError{Field: "source_codec", Message: "invalid video codec; must be one of: h264, h265, hevc, vp9, av1"}
		}
	} else {
		validAudioCodecs := []string{"aac", "ac3", "eac3", "opus", "mp3", "flac", "vorbis"}
		if !containsIgnoreCase(validAudioCodecs, o.SourceCodec) {
			return ValidationError{Field: "source_codec", Message: "invalid audio codec; must be one of: aac, ac3, eac3, opus, mp3, flac, vorbis"}
		}
	}
	// HWAccelMatch validation is lenient - any non-empty value is accepted
	// to allow for future hardware accelerators
	return nil
}

// containsIgnoreCase checks if a slice contains a string (case-insensitive).
func containsIgnoreCase(slice []string, s string) bool {
	sLower := strings.ToLower(s)
	for _, item := range slice {
		if strings.ToLower(item) == sLower {
			return true
		}
	}
	return false
}

// Enabled returns true if this override is enabled.
func (o *EncoderOverride) Enabled() bool {
	return BoolVal(o.IsEnabled)
}

// MatchesCodec returns true if this override matches the given codec type and source codec.
// Uses codec.Normalize to handle aliases (e.g., hevc=h265, avc=h264, ec3=eac3).
func (o *EncoderOverride) MatchesCodec(codecType EncoderOverrideCodecType, sourceCodec string) bool {
	if o.CodecType != codecType {
		return false
	}
	// Normalize codec names using canonical forms
	return codec.Normalize(o.SourceCodec) == codec.Normalize(sourceCodec)
}
