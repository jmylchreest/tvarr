# Data Model: FFmpeg Profile Configuration

**Feature**: FFmpeg Profile Configuration
**Branch**: `007-ffmpeg-profile-configuration`
**Date**: 2025-12-06

## Entity Definitions

### RelayProfile (Extended)

Extends existing `internal/models/relay_profile.go` with additional fields.

#### Existing Fields (unchanged)

```go
type RelayProfile struct {
    BaseModel                           // ID, CreatedAt, UpdatedAt

    // Identity
    Name        string `gorm:"uniqueIndex;not null;size:100"`
    Description string `gorm:"size:500"`
    IsDefault   bool   `gorm:"default:false"`
    Enabled     bool   `gorm:"default:true"`
    IsSystem    bool   `gorm:"default:false"`

    // Video settings
    VideoCodec     VideoCodec
    VideoBitrate   int
    VideoMaxrate   int
    VideoBufsize   int
    VideoWidth     int
    VideoHeight    int
    VideoFramerate float64
    VideoPreset    string
    VideoCRF       int
    VideoProfile   string
    VideoLevel     string

    // Audio settings
    AudioCodec      AudioCodec
    AudioBitrate    int
    AudioSampleRate int
    AudioChannels   int

    // Hardware acceleration (existing)
    HWAccel       HWAccelType
    HWAccelDevice string

    // Output settings
    OutputFormat   OutputFormat
    SegmentTime    int
    PlaylistSize   int

    // Buffer and timeout settings
    InputBufferSize  int
    OutputBufferSize int
    ProbeSize        int
    AnalyzeDuration  int
    Timeout          int

    // Custom options (existing but not wired)
    InputOptions  string `gorm:"size:1000"`  // FR-001
    OutputOptions string `gorm:"size:1000"`  // FR-002
    FilterComplex string `gorm:"size:2000"`  // FR-003

    // Thread settings
    Threads     int
    ThreadQueue int

    // Fallback settings
    FallbackEnabled          bool
    FallbackErrorThreshold   int
    FallbackRecoveryInterval int
}
```

#### New Fields

```go
// Hardware acceleration extensions (FR-006, FR-007, FR-008)
HWAccelOutputFormat  string `gorm:"size:50" json:"hw_accel_output_format,omitempty"`  // cuda, qsv, vaapi
HWAccelDecoderCodec  string `gorm:"size:50" json:"hw_accel_decoder_codec,omitempty"`  // h264_cuvid, hevc_qsv
HWAccelExtraOptions  string `gorm:"size:500" json:"hw_accel_extra_options,omitempty"` // Additional hwaccel flags
GpuIndex             int    `gorm:"default:-1" json:"gpu_index"`                       // -1 = auto, 0+ = specific GPU

// Validation tracking (FR-005)
CustomFlagsValidated bool      `gorm:"default:false" json:"custom_flags_validated"`
CustomFlagsWarnings  string    `gorm:"size:2000" json:"custom_flags_warnings,omitempty"` // JSON array

// Profile statistics (FR-022)
SuccessCount   int64     `gorm:"default:0" json:"success_count"`
FailureCount   int64     `gorm:"default:0" json:"failure_count"`
LastUsedAt     time.Time `json:"last_used_at,omitempty"`
LastErrorAt    time.Time `json:"last_error_at,omitempty"`
LastErrorMsg   string    `gorm:"size:500" json:"last_error_msg,omitempty"`
```

### ProfileTestResult (New Entity - Transient)

Not stored in database - returned from test endpoint.

```go
type ProfileTestResult struct {
    // Test execution details
    ProfileID       string        `json:"profile_id"`
    ProfileName     string        `json:"profile_name"`
    TestStreamURL   string        `json:"test_stream_url"`
    TestDuration    time.Duration `json:"test_duration"`
    TestedAt        time.Time     `json:"tested_at"`

    // Results
    Success         bool          `json:"success"`
    FramesProcessed int64         `json:"frames_processed"`
    FPS             float64       `json:"fps"`
    AvgBitrate      string        `json:"avg_bitrate,omitempty"`

    // Codec detection
    DetectedVideoCodec string     `json:"detected_video_codec,omitempty"`
    DetectedAudioCodec string     `json:"detected_audio_codec,omitempty"`
    DetectedResolution string     `json:"detected_resolution,omitempty"`

    // Hardware acceleration verification (FR-009)
    HWAccelRequested  bool        `json:"hw_accel_requested"`
    HWAccelActive     bool        `json:"hw_accel_active"`
    HWAccelDevice     string      `json:"hw_accel_device,omitempty"`
    HWAccelEncoder    string      `json:"hw_accel_encoder,omitempty"`
    HWAccelDecoder    string      `json:"hw_accel_decoder,omitempty"`

    // Diagnostics
    Warnings        []string      `json:"warnings,omitempty"`
    Errors          []string      `json:"errors,omitempty"`
    Suggestions     []string      `json:"suggestions,omitempty"`
    FFmpegOutput    string        `json:"ffmpeg_output,omitempty"`
    CommandExecuted string        `json:"command_executed"`

    // Resource usage estimate
    EstimatedCPUUsage   float64   `json:"estimated_cpu_usage,omitempty"`
    EstimatedMemoryMB   float64   `json:"estimated_memory_mb,omitempty"`
}
```

### HardwareCapability (Cached in memory)

Detected at startup, cached in memory (FR-009).

```go
type HardwareCapability struct {
    Type             HWAccelType  `json:"type"`
    Name             string       `json:"name"`
    Available        bool         `json:"available"`
    DeviceName       string       `json:"device_name,omitempty"`
    DevicePath       string       `json:"device_path,omitempty"`  // e.g., /dev/dri/renderD128
    GpuIndex         int          `json:"gpu_index,omitempty"`
    SupportedEncoders []string    `json:"supported_encoders,omitempty"`
    SupportedDecoders []string    `json:"supported_decoders,omitempty"`
    DetectedAt       time.Time    `json:"detected_at"`
}
```

### CommandPreview (Transient Response)

Response for command preview endpoint (FR-017).

```go
type CommandPreview struct {
    ProfileID     string   `json:"profile_id"`
    ProfileName   string   `json:"profile_name"`
    Command       string   `json:"command"`
    Arguments     []string `json:"arguments"`
    FullCommand   string   `json:"full_command"`  // Single line for copying
    Warnings      []string `json:"warnings,omitempty"`
}
```

### FlagValidationResult (Transient)

Result of custom flag validation.

```go
type FlagValidationResult struct {
    Valid       bool     `json:"valid"`
    Flags       []string `json:"flags"`
    Warnings    []string `json:"warnings,omitempty"`
    Errors      []string `json:"errors,omitempty"`
    Suggestions []string `json:"suggestions,omitempty"`
}
```

## Validation Rules

### Custom Flags Validation (FR-005)

```go
const (
    MaxInputOptionsLength  = 1000
    MaxOutputOptionsLength = 1000
    MaxFilterComplexLength = 2000
)

var DangerousPatterns = []string{
    `\$\(`,           // Command substitution
    "`",              // Backtick substitution
    `\$\{`,           // Variable expansion
    `\$[A-Za-z_]`,    // Variable reference
    `;`,              // Command separator
    `\|(?!\|)`,       // Pipe (allow ||)
    `&&`,             // Command chaining
    `>>?`,            // Output redirection
    `<`,              // Input redirection
}

var BlockedFlags = []string{
    "-i",
    "-y",
    "-n",
    "-filter_script",
    "-protocol_whitelist",
}

func ValidateCustomFlags(flags string) FlagValidationResult {
    // Implementation validates against patterns and blocklist
    // Returns warnings but allows saving for advanced use cases
}
```

### Hardware Acceleration Validation

```go
var ValidHWAccelOutputFormats = map[HWAccelType][]string{
    HWAccelNVDEC: {"cuda"},
    HWAccelQSV:   {"qsv"},
    HWAccelVAAPI: {"vaapi"},
    HWAccelVT:    {"videotoolbox_vld"},
}

var ValidHWAccelDecoderCodecs = map[HWAccelType][]string{
    HWAccelNVDEC: {"h264_cuvid", "hevc_cuvid", "vp9_cuvid", "av1_cuvid"},
    HWAccelQSV:   {"h264_qsv", "hevc_qsv", "vp9_qsv", "av1_qsv"},
    HWAccelVAAPI: {}, // VAAPI uses generic hwaccel, not specific decoders
    HWAccelVT:    {}, // VideoToolbox uses generic hwaccel
}
```

### Profile Clone Rules (FR-012)

```go
func (p *RelayProfile) Clone(newName string) *RelayProfile {
    clone := *p
    clone.ID = ULID{}           // New ID
    clone.Name = newName
    clone.IsDefault = false     // Never clone as default
    clone.IsSystem = false      // Clone is always user-owned
    clone.SuccessCount = 0      // Reset statistics
    clone.FailureCount = 0
    clone.LastUsedAt = time.Time{}
    clone.LastErrorAt = time.Time{}
    clone.LastErrorMsg = ""
    clone.CreatedAt = Now()
    clone.UpdatedAt = Now()
    return &clone
}
```

## State Transitions

### Profile Lifecycle

```
                  ┌──────────────┐
                  │   CREATED    │
                  └──────┬───────┘
                         │
    ┌────────────────────┼────────────────────┐
    │                    │                    │
    ▼                    ▼                    ▼
┌───────┐         ┌─────────────┐       ┌────────┐
│ENABLED│◄───────►│  DISABLED   │◄─────►│DELETED │
└───┬───┘         └─────────────┘       └────────┘
    │
    │ (used in relay)
    ▼
┌───────────────────────────────────────────────┐
│                   IN USE                       │
│  - SuccessCount/FailureCount incremented      │
│  - LastUsedAt updated                         │
│  - LastErrorAt/LastErrorMsg on failure        │
└───────────────────────────────────────────────┘
```

### System Profile Restrictions

System profiles (IsSystem = true):
- Cannot be edited (except Enabled flag)
- Cannot be deleted
- Can be cloned to create editable copy

## Relationships

```
┌─────────────────────┐
│    RelayProfile     │
└──────────┬──────────┘
           │
           │ 1:N (via relay_profile_id)
           ▼
┌─────────────────────┐
│    StreamProxy      │
│  (uses profile for  │
│   relay streaming)  │
└─────────────────────┘
```

## Migration Requirements

### Database Migration

```sql
-- Add new columns to relay_profiles table
ALTER TABLE relay_profiles ADD COLUMN hw_accel_output_format VARCHAR(50);
ALTER TABLE relay_profiles ADD COLUMN hw_accel_decoder_codec VARCHAR(50);
ALTER TABLE relay_profiles ADD COLUMN hw_accel_extra_options VARCHAR(500);
ALTER TABLE relay_profiles ADD COLUMN gpu_index INTEGER DEFAULT -1;
ALTER TABLE relay_profiles ADD COLUMN custom_flags_validated BOOLEAN DEFAULT false;
ALTER TABLE relay_profiles ADD COLUMN custom_flags_warnings TEXT;
ALTER TABLE relay_profiles ADD COLUMN success_count INTEGER DEFAULT 0;
ALTER TABLE relay_profiles ADD COLUMN failure_count INTEGER DEFAULT 0;
ALTER TABLE relay_profiles ADD COLUMN last_used_at TIMESTAMP;
ALTER TABLE relay_profiles ADD COLUMN last_error_at TIMESTAMP;
ALTER TABLE relay_profiles ADD COLUMN last_error_msg VARCHAR(500);
```

### Default System Profiles Update

Update existing system profiles to include sensible default custom options:

```go
var SystemProfiles = []RelayProfile{
    {
        Name:        "Passthrough",
        Description: "Direct stream copy without transcoding",
        IsSystem:    true,
        IsDefault:   true,
        VideoCodec:  VideoCodecCopy,
        AudioCodec:  AudioCodecCopy,
        InputOptions: "-fflags +genpts+discardcorrupt",
    },
    {
        Name:        "NVIDIA Hardware",
        Description: "NVIDIA GPU-accelerated transcoding",
        IsSystem:    true,
        VideoCodec:  VideoCodecNVENC,
        AudioCodec:  AudioCodecCopy,
        HWAccel:     HWAccelNVDEC,
        HWAccelOutputFormat: "cuda",
        InputOptions: "-hwaccel_output_format cuda",
    },
    // ... additional system profiles
}
```

## Indexes

Existing indexes are sufficient. No new indexes required for the new fields as they are not used in queries.
