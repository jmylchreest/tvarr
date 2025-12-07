package models

import (
	"time"

	"gorm.io/gorm"
)

// VideoCodec represents a video codec for transcoding.
type VideoCodec string

const (
	VideoCodecCopy    VideoCodec = "copy"     // Pass-through (no transcoding)
	VideoCodecH264    VideoCodec = "libx264"  // Software H.264
	VideoCodecH265    VideoCodec = "libx265"  // Software H.265/HEVC
	VideoCodecVP9     VideoCodec = "libvpx-vp9"
	VideoCodecAV1     VideoCodec = "libaom-av1"
	VideoCodecNVENC   VideoCodec = "h264_nvenc"   // NVIDIA hardware
	VideoCodecNVENCH5 VideoCodec = "hevc_nvenc"   // NVIDIA hardware HEVC
	VideoCodecQSV     VideoCodec = "h264_qsv"     // Intel QuickSync
	VideoCodecQSVH5   VideoCodec = "hevc_qsv"     // Intel QuickSync HEVC
	VideoCodecVAAPI   VideoCodec = "h264_vaapi"   // Linux VA-API
	VideoCodecVAAPIH5 VideoCodec = "hevc_vaapi"   // Linux VA-API HEVC
)

// AudioCodec represents an audio codec for transcoding.
type AudioCodec string

const (
	AudioCodecCopy AudioCodec = "copy"    // Pass-through (no transcoding)
	AudioCodecAAC  AudioCodec = "aac"     // AAC
	AudioCodecMP3  AudioCodec = "libmp3lame"
	AudioCodecOpus AudioCodec = "libopus"
	AudioCodecAC3  AudioCodec = "ac3"
	AudioCodecEAC3 AudioCodec = "eac3"
)

// OutputFormat represents the output container format.
type OutputFormat string

const (
	OutputFormatMPEGTS OutputFormat = "mpegts"  // MPEG Transport Stream
	OutputFormatHLS    OutputFormat = "hls"     // HTTP Live Streaming
	OutputFormatFLV    OutputFormat = "flv"     // Flash Video
	OutputFormatMKV    OutputFormat = "matroska"
	OutputFormatMP4    OutputFormat = "mp4"
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
	OutputFormat   OutputFormat `gorm:"size:50;default:'mpegts'" json:"output_format"`
	SegmentTime    int          `gorm:"default:0" json:"segment_time,omitempty"`      // HLS segment duration (seconds)
	PlaylistSize   int          `gorm:"default:0" json:"playlist_size,omitempty"`     // HLS playlist entries

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
