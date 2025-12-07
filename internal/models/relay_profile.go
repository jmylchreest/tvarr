package models

import (
	"time"

	"gorm.io/gorm"
)

// VideoCodec represents a video codec for transcoding.
type VideoCodec string

const (
	VideoCodecCopy VideoCodec = "copy" // Pass-through (no transcoding)
	VideoCodecNone VideoCodec = "none" // No video codec (use FFmpeg flags)
	VideoCodecH264 VideoCodec = "h264" // H.264/AVC
	VideoCodecH265 VideoCodec = "h265" // H.265/HEVC
	VideoCodecVP9  VideoCodec = "vp9"  // VP9 (fMP4 only)
	VideoCodecAV1  VideoCodec = "av1"  // AV1 (fMP4 only)
)

// GetFFmpegEncoder returns the FFmpeg encoder name based on codec and hardware acceleration.
// This maps abstract codec types to concrete FFmpeg encoder names.
func (c VideoCodec) GetFFmpegEncoder(hwaccel HWAccelType) string {
	switch c {
	case VideoCodecCopy:
		return "copy"
	case VideoCodecNone:
		return "" // No encoder, user specifies via FFmpeg flags
	case VideoCodecH264:
		switch hwaccel {
		case HWAccelNVDEC:
			return "h264_nvenc"
		case HWAccelQSV:
			return "h264_qsv"
		case HWAccelVAAPI:
			return "h264_vaapi"
		case HWAccelVT:
			return "h264_videotoolbox"
		default: // HWAccelNone, HWAccelAuto
			return "libx264"
		}
	case VideoCodecH265:
		switch hwaccel {
		case HWAccelNVDEC:
			return "hevc_nvenc"
		case HWAccelQSV:
			return "hevc_qsv"
		case HWAccelVAAPI:
			return "hevc_vaapi"
		case HWAccelVT:
			return "hevc_videotoolbox"
		default:
			return "libx265"
		}
	case VideoCodecVP9:
		// VP9 hardware encoding is rare, default to software
		switch hwaccel {
		case HWAccelQSV:
			return "vp9_qsv"
		case HWAccelVAAPI:
			return "vp9_vaapi"
		default:
			return "libvpx-vp9"
		}
	case VideoCodecAV1:
		switch hwaccel {
		case HWAccelNVDEC:
			return "av1_nvenc"
		case HWAccelQSV:
			return "av1_qsv"
		case HWAccelVAAPI:
			return "av1_vaapi"
		default:
			return "libaom-av1"
		}
	default:
		// Unknown codec, return as-is (might be a legacy value)
		return string(c)
	}
}

// IsFMP4Only returns true if this codec requires fMP4 container.
func (c VideoCodec) IsFMP4Only() bool {
	switch c {
	case VideoCodecVP9, VideoCodecAV1:
		return true
	default:
		return false
	}
}

// AudioCodec represents an audio codec for transcoding.
type AudioCodec string

const (
	AudioCodecCopy AudioCodec = "copy" // Pass-through (no transcoding)
	AudioCodecNone AudioCodec = "none" // No audio codec (use FFmpeg flags)
	AudioCodecAAC  AudioCodec = "aac"  // AAC
	AudioCodecMP3  AudioCodec = "mp3"  // MP3
	AudioCodecAC3  AudioCodec = "ac3"  // Dolby Digital
	AudioCodecEAC3 AudioCodec = "eac3" // Dolby Digital Plus
	AudioCodecOpus AudioCodec = "opus" // Opus (fMP4 only)
)

// GetFFmpegEncoder returns the FFmpeg encoder name for this audio codec.
func (c AudioCodec) GetFFmpegEncoder() string {
	switch c {
	case AudioCodecCopy:
		return "copy"
	case AudioCodecNone:
		return "" // No encoder, user specifies via FFmpeg flags
	case AudioCodecAAC:
		return "aac"
	case AudioCodecMP3:
		return "libmp3lame"
	case AudioCodecAC3:
		return "ac3"
	case AudioCodecEAC3:
		return "eac3"
	case AudioCodecOpus:
		return "libopus"
	default:
		// Unknown codec, return as-is (might be a legacy value)
		return string(c)
	}
}

// IsFMP4Only returns true if this codec requires fMP4 container.
func (c AudioCodec) IsFMP4Only() bool {
	switch c {
	case AudioCodecOpus:
		return true
	default:
		return false
	}
}

// ContainerFormat represents the media container format for streaming.
// This separates the container (TS/fMP4) from the manifest format (HLS/DASH).
type ContainerFormat string

const (
	// ContainerFormatAuto lets the system choose the best container based on codec and client.
	// - fMP4 for: DASH requests, HLS requests from modern browsers (Chrome, Firefox, Edge, Safari 10+)
	// - MPEG-TS for: explicit ?format=mpegts, legacy User-Agents, Accept header requesting video/MP2T
	ContainerFormatAuto ContainerFormat = "auto"

	// ContainerFormatFMP4 forces fragmented MP4 (CMAF) container.
	// Required for VP9, AV1, and Opus codecs. Supports HLS v7+ and DASH.
	ContainerFormatFMP4 ContainerFormat = "fmp4"

	// ContainerFormatMPEGTS forces MPEG Transport Stream container.
	// Maximum compatibility with legacy devices. Limited to H.264/H.265/AAC/MP3/AC3/EAC3.
	ContainerFormatMPEGTS ContainerFormat = "mpegts"
)

// HWAccelType represents hardware acceleration type.
type HWAccelType string

const (
	HWAccelAuto  HWAccelType = "auto"         // Auto-detect best available
	HWAccelNone  HWAccelType = "none"         // Disabled (software only)
	HWAccelNVDEC HWAccelType = "cuda"         // NVIDIA NVDEC
	HWAccelQSV   HWAccelType = "qsv"          // Intel QuickSync
	HWAccelVAAPI HWAccelType = "vaapi"        // Linux VA-API
	HWAccelVT    HWAccelType = "videotoolbox" // macOS
)

// RelayProfile defines a transcoding profile for stream relay.
type RelayProfile struct {
	BaseModel

	// Name is a unique identifier for this profile.
	Name string `gorm:"uniqueIndex;not null;size:100" json:"name"`

	// Description explains what this profile is for.
	Description string `gorm:"size:500" json:"description,omitempty"`

	// IsDefault indicates if this is the default profile.
	IsDefault bool `gorm:"default:false" json:"is_default"`

	// Enabled indicates if this profile can be used.
	Enabled bool `gorm:"default:true" json:"enabled"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only Enabled can be toggled for system profiles.
	IsSystem bool `gorm:"default:false" json:"is_system"`

	// Video settings
	VideoCodec     VideoCodec `gorm:"size:50;default:'copy'" json:"video_codec"`
	VideoBitrate   int        `gorm:"default:0" json:"video_bitrate,omitempty"`        // kbps, 0 = auto
	VideoMaxrate   int        `gorm:"default:0" json:"video_maxrate,omitempty"`        // kbps, 0 = no limit
	VideoBufsize   int        `gorm:"default:0" json:"video_bufsize,omitempty"`        // kbps, 0 = auto
	VideoWidth     int        `gorm:"default:0" json:"video_width,omitempty"`          // 0 = keep original
	VideoHeight    int        `gorm:"default:0" json:"video_height,omitempty"`         // 0 = keep original
	VideoFramerate float64    `gorm:"default:0" json:"video_framerate,omitempty"`      // 0 = keep original
	VideoPreset    string     `gorm:"size:50" json:"video_preset,omitempty"`           // ultrafast, fast, medium, slow
	VideoCRF       int        `gorm:"default:0" json:"video_crf,omitempty"`            // Constant Rate Factor (0-51, 0=lossless)
	VideoProfile   string     `gorm:"size:50" json:"video_profile,omitempty"`          // baseline, main, high
	VideoLevel     string     `gorm:"size:10" json:"video_level,omitempty"`            // 3.0, 4.0, 4.1, etc.

	// Audio settings
	AudioCodec      AudioCodec `gorm:"size:50;default:'copy'" json:"audio_codec"`
	AudioBitrate    int        `gorm:"default:0" json:"audio_bitrate,omitempty"`       // kbps, 0 = auto
	AudioSampleRate int        `gorm:"default:0" json:"audio_sample_rate,omitempty"`   // Hz, 0 = keep original
	AudioChannels   int        `gorm:"default:0" json:"audio_channels,omitempty"`      // 0 = keep original

	// Hardware acceleration
	HWAccel             HWAccelType `gorm:"size:50;default:'auto'" json:"hw_accel"`
	HWAccelDevice       string      `gorm:"size:100" json:"hw_accel_device,omitempty"`        // /dev/dri/renderD128
	HWAccelOutputFormat string      `gorm:"size:50" json:"hw_accel_output_format,omitempty"`  // cuda, qsv, vaapi
	HWAccelDecoderCodec string      `gorm:"size:50" json:"hw_accel_decoder_codec,omitempty"`  // h264_cuvid, hevc_qsv
	HWAccelExtraOptions string      `gorm:"size:500" json:"hw_accel_extra_options,omitempty"` // Additional hwaccel flags
	GpuIndex            int         `gorm:"default:-1" json:"gpu_index"`                      // -1 = auto, 0+ = specific GPU

	// Output settings
	ContainerFormat ContainerFormat `gorm:"size:20;default:'auto'" json:"container_format"`
	SegmentDuration int             `gorm:"default:6" json:"segment_duration,omitempty"` // HLS/DASH segment duration (seconds), 2-10
	PlaylistSize    int             `gorm:"default:5" json:"playlist_size,omitempty"`    // HLS/DASH playlist entries, 3-20

	// Buffer and timeout settings
	InputBufferSize  int `gorm:"default:8192" json:"input_buffer_size"`    // KB
	OutputBufferSize int `gorm:"default:8192" json:"output_buffer_size"`   // KB
	ProbeSize        int `gorm:"default:5000000" json:"probe_size"`        // Bytes for stream analysis
	AnalyzeDuration  int `gorm:"default:5000000" json:"analyze_duration"`  // Microseconds for analysis
	Timeout          int `gorm:"default:30" json:"timeout"`                // Connection timeout (seconds)

	// Advanced FFmpeg options
	InputOptions  string `gorm:"size:1000" json:"input_options,omitempty"`   // Extra FFmpeg input options
	OutputOptions string `gorm:"size:1000" json:"output_options,omitempty"`  // Extra FFmpeg output options
	FilterComplex string `gorm:"size:2000" json:"filter_complex,omitempty"`  // Custom filter graph

	// Thread settings
	Threads     int `gorm:"default:0" json:"threads,omitempty"`      // 0 = auto
	ThreadQueue int `gorm:"default:512" json:"thread_queue"`         // Thread queue size

	// Fallback settings for error handling
	FallbackEnabled          bool `gorm:"default:true" json:"fallback_enabled"`            // Enable fallback stream on error
	FallbackErrorThreshold   int  `gorm:"default:3" json:"fallback_error_threshold"`       // Errors before fallback (1-10)
	FallbackRecoveryInterval int  `gorm:"default:30" json:"fallback_recovery_interval"`    // Seconds between recovery attempts (5-300)

	// Smart codec matching - when false, use copy if source matches target codec family
	ForceVideoTranscode bool `gorm:"default:false" json:"force_video_transcode"` // Force video transcoding even if source matches target codec
	ForceAudioTranscode bool `gorm:"default:false" json:"force_audio_transcode"` // Force audio transcoding even if source matches target codec

	// Custom flags validation tracking
	CustomFlagsValidated bool   `gorm:"default:false" json:"custom_flags_validated"`
	CustomFlagsWarnings  string `gorm:"size:2000" json:"custom_flags_warnings,omitempty"` // JSON array of warnings

	// Profile usage statistics
	SuccessCount int64     `gorm:"default:0" json:"success_count"`
	FailureCount int64     `gorm:"default:0" json:"failure_count"`
	LastUsedAt   time.Time `json:"last_used_at,omitempty"`
	LastErrorAt  time.Time `json:"last_error_at,omitempty"`
	LastErrorMsg string    `gorm:"size:500" json:"last_error_msg,omitempty"`
}

// TableName returns the table name for RelayProfile.
func (RelayProfile) TableName() string {
	return "relay_profiles"
}

// Validate performs basic validation on the profile.
func (p *RelayProfile) Validate() error {
	if p.Name == "" {
		return ErrRelayProfileNameRequired
	}
	if p.VideoBitrate < 0 {
		return ErrRelayProfileInvalidBitrate
	}
	if p.AudioBitrate < 0 {
		return ErrRelayProfileInvalidBitrate
	}
	// Validate fallback settings
	if p.FallbackErrorThreshold < 1 || p.FallbackErrorThreshold > 10 {
		p.FallbackErrorThreshold = 3 // Reset to default if invalid
	}
	if p.FallbackRecoveryInterval < 5 || p.FallbackRecoveryInterval > 300 {
		p.FallbackRecoveryInterval = 30 // Reset to default if invalid
	}
	// Validate codec/format compatibility
	if err := p.ValidateCodecFormat(); err != nil {
		return err
	}
	// Validate segment configuration
	if err := p.ValidateSegmentConfig(); err != nil {
		return err
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the profile and generates ULID.
func (p *RelayProfile) BeforeCreate(tx *gorm.DB) error {
	if err := p.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return p.Validate()
}

// BeforeUpdate is a GORM hook that validates the profile before update.
func (p *RelayProfile) BeforeUpdate(tx *gorm.DB) error {
	return p.Validate()
}

// IsPassthrough returns true if the profile uses copy for both video and audio.
func (p *RelayProfile) IsPassthrough() bool {
	return p.VideoCodec == VideoCodecCopy && p.AudioCodec == AudioCodecCopy
}

// UsesHardwareAccel returns true if hardware acceleration is configured.
func (p *RelayProfile) UsesHardwareAccel() bool {
	return p.HWAccel != HWAccelNone && p.HWAccel != ""
}

// NeedsTranscode returns true if any transcoding is required.
func (p *RelayProfile) NeedsTranscode() bool {
	return p.VideoCodec != VideoCodecCopy || p.AudioCodec != AudioCodecCopy
}

// ValidateCodecFormat validates that the selected codecs are compatible with the container format.
// VP9, AV1, and Opus codecs require fMP4 container (not MPEG-TS).
func (p *RelayProfile) ValidateCodecFormat() error {
	// If user explicitly requested MPEG-TS but codecs require fMP4, that's an error
	if p.ContainerFormat == ContainerFormatMPEGTS && p.RequiresFMP4() {
		return ErrRelayProfileCodecRequiresFMP4
	}
	return nil
}

// ValidateSegmentConfig validates segment duration and playlist size settings.
func (p *RelayProfile) ValidateSegmentConfig() error {
	// Only validate if non-zero (0 means use defaults)
	if p.SegmentDuration != 0 && (p.SegmentDuration < 2 || p.SegmentDuration > 10) {
		return ErrRelayProfileInvalidSegmentDuration
	}
	if p.PlaylistSize != 0 && (p.PlaylistSize < 3 || p.PlaylistSize > 20) {
		return ErrRelayProfileInvalidPlaylistSize
	}
	return nil
}

// IsFMP4OnlyVideoCodec returns true if the codec requires fMP4 container (not compatible with MPEG-TS).
func IsFMP4OnlyVideoCodec(codec VideoCodec) bool {
	return codec.IsFMP4Only()
}

// IsFMP4OnlyAudioCodec returns true if the codec requires fMP4 container (not compatible with MPEG-TS).
func IsFMP4OnlyAudioCodec(codec AudioCodec) bool {
	return codec.IsFMP4Only()
}

// RequiresFMP4 returns true if the profile's codecs require fMP4 container.
// VP9, AV1, and Opus codecs are not supported in MPEG-TS containers.
func (p *RelayProfile) RequiresFMP4() bool {
	return IsFMP4OnlyVideoCodec(p.VideoCodec) || IsFMP4OnlyAudioCodec(p.AudioCodec)
}

// DetermineContainer returns the effective container format for this profile.
// When ContainerFormat is "auto", it selects based on codec requirements:
// - fMP4 if codecs require it (VP9/AV1/Opus)
// - fMP4 as modern default for H.264/H.265 (better seeking, caching)
// - MPEG-TS only when explicitly configured or codec is "copy"
func (p *RelayProfile) DetermineContainer() ContainerFormat {
	// Explicit container format takes precedence (unless it conflicts with codecs)
	if p.ContainerFormat == ContainerFormatFMP4 {
		return ContainerFormatFMP4
	}
	if p.ContainerFormat == ContainerFormatMPEGTS {
		// Even if explicitly MPEG-TS, modern codecs force fMP4
		if p.RequiresFMP4() {
			return ContainerFormatFMP4
		}
		return ContainerFormatMPEGTS
	}

	// Auto mode: determine based on codecs
	if p.RequiresFMP4() {
		return ContainerFormatFMP4
	}

	// For passthrough (copy), prefer MPEG-TS for maximum compatibility
	if p.VideoCodec == VideoCodecCopy && p.AudioCodec == AudioCodecCopy {
		return ContainerFormatMPEGTS
	}

	// Default to fMP4 for modern codecs (better seeking, HLS v7 support)
	return ContainerFormatFMP4
}

// Clone creates a copy of the profile suitable for customization.
// The caller should set Name, Description, etc. on the returned clone.
func (p *RelayProfile) Clone() *RelayProfile {
	clone := *p
	clone.ID = ULID{}
	clone.Name = "" // Must be set by caller
	clone.Description = ""
	clone.IsDefault = false
	clone.IsSystem = false // Clone is always user-owned
	clone.SuccessCount = 0
	clone.FailureCount = 0
	clone.LastUsedAt = time.Time{}
	clone.LastErrorAt = time.Time{}
	clone.LastErrorMsg = ""
	clone.CustomFlagsValidated = false
	clone.CustomFlagsWarnings = ""
	clone.CreatedAt = Now()
	clone.UpdatedAt = Now()
	return &clone
}
