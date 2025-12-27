package types

import (
	"time"
)

// JobID is a unique identifier for a transcode job.
type JobID string

// String implements fmt.Stringer.
func (j JobID) String() string {
	return string(j)
}

// JobState represents the lifecycle state of a job.
type JobState int

const (
	JobStatePending JobState = iota
	JobStateAssigned
	JobStateStarting
	JobStateRunning
	JobStateCompleted
	JobStateFailed
	JobStateCancelled
)

// String returns a human-readable state name.
func (s JobState) String() string {
	switch s {
	case JobStatePending:
		return "pending"
	case JobStateAssigned:
		return "assigned"
	case JobStateStarting:
		return "starting"
	case JobStateRunning:
		return "running"
	case JobStateCompleted:
		return "completed"
	case JobStateFailed:
		return "failed"
	case JobStateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// IsTerminal returns true if the job is in a terminal state.
func (s JobState) IsTerminal() bool {
	return s == JobStateCompleted || s == JobStateFailed || s == JobStateCancelled
}

// IsActive returns true if the job is actively running.
func (s JobState) IsActive() bool {
	return s == JobStateAssigned || s == JobStateStarting || s == JobStateRunning
}

// TranscodeJob represents an active transcoding session.
type TranscodeJob struct {
	ID          JobID  `json:"id"`
	SessionID   string `json:"session_id"` // RelaySession ID
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`

	// Assignment
	State       JobState  `json:"state"`
	DaemonID    DaemonID  `json:"daemon_id,omitempty"`
	AssignedAt  time.Time `json:"assigned_at,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Configuration
	Config *TranscodeConfig `json:"config"`

	// Runtime stats (updated from daemon)
	Stats *TranscodeStats `json:"stats,omitempty"`

	// Error tracking
	Error      string `json:"error,omitempty"`
	RetryCount int    `json:"retry_count"`
}

// RunningTime returns the duration the job has been running.
func (j *TranscodeJob) RunningTime() time.Duration {
	if j.StartedAt.IsZero() {
		return 0
	}
	if j.CompletedAt.IsZero() {
		return time.Since(j.StartedAt)
	}
	return j.CompletedAt.Sub(j.StartedAt)
}

// TranscodeConfig defines the transcoding parameters.
type TranscodeConfig struct {
	// Source codec info
	SourceVideoCodec string `json:"source_video_codec"` // h264, hevc
	SourceAudioCodec string `json:"source_audio_codec"` // aac, ac3, eac3
	VideoInitData    []byte `json:"video_init_data"`    // SPS/PPS
	AudioInitData    []byte `json:"audio_init_data"`    // AudioSpecificConfig

	// Target codec info (from EncodingProfile)
	// Note: Encoder selection is done locally by the daemon based on target codec
	// and available hardware. The coordinator only specifies target codecs.
	TargetVideoCodec string `json:"target_video_codec"`
	TargetAudioCodec string `json:"target_audio_codec"`

	// Encoding parameters
	VideoBitrateKbps int    `json:"video_bitrate_kbps"`
	AudioBitrateKbps int    `json:"audio_bitrate_kbps"`
	VideoPreset      string `json:"video_preset"`  // ultrafast, fast, medium
	VideoCRF         int    `json:"video_crf"`     // Quality-based encoding (0-51)
	VideoProfile     string `json:"video_profile"` // baseline, main, high
	VideoLevel       string `json:"video_level"`   // 3.0, 4.0, 4.1, etc.

	// Resolution scaling (optional)
	ScaleWidth  int `json:"scale_width,omitempty"` // 0 = no scaling
	ScaleHeight int `json:"scale_height,omitempty"`

	// Hardware preference
	PreferredHWAccel string `json:"preferred_hw_accel,omitempty"`
	HWDevice         string `json:"hw_device,omitempty"`
	FallbackEncoder  string `json:"fallback_encoder,omitempty"`

	// Policy
	GPUExhaustedPolicy GPUExhaustedPolicy `json:"gpu_exhausted_policy"`

	// Extra FFmpeg options (advanced)
	ExtraOptions map[string]string `json:"extra_options,omitempty"`

	// Custom FFmpeg flags from encoding profile
	InputFlags  string `json:"input_flags,omitempty"`  // Flags placed before -i input
	OutputFlags string `json:"output_flags,omitempty"` // Flags placed after -i input
	GlobalFlags string `json:"global_flags,omitempty"` // Global FFmpeg flags
}

// GPUExhaustedPolicy defines behavior when GPU sessions are exhausted.
type GPUExhaustedPolicy int

const (
	GPUPolicyFallback GPUExhaustedPolicy = iota // Use software encoder
	GPUPolicyQueue                              // Wait for GPU
	GPUPolicyReject                             // Fail immediately
)

// String returns a human-readable policy name.
func (p GPUExhaustedPolicy) String() string {
	switch p {
	case GPUPolicyFallback:
		return "fallback"
	case GPUPolicyQueue:
		return "queue"
	case GPUPolicyReject:
		return "reject"
	default:
		return "unknown"
	}
}

// TranscodeStats contains runtime statistics for a job.
type TranscodeStats struct {
	SamplesIn     uint64        `json:"samples_in"`
	SamplesOut    uint64        `json:"samples_out"`
	BytesIn       uint64        `json:"bytes_in"`
	BytesOut      uint64        `json:"bytes_out"`
	EncodingSpeed float64       `json:"encoding_speed"` // 1.0 = realtime
	CPUPercent    float64       `json:"cpu_percent"`
	MemoryMB      float64       `json:"memory_mb"`
	FFmpegPID     int           `json:"ffmpeg_pid"`
	RunningTime   time.Duration `json:"running_time"`

	// Hardware acceleration info
	HWAccel  string `json:"hw_accel,omitempty"`  // vaapi, cuda, qsv, videotoolbox (empty = software)
	HWDevice string `json:"hw_device,omitempty"` // Device path: /dev/dri/renderD128, cuda:0, etc.

	// FFmpeg command for debugging
	FFmpegCommand string `json:"ffmpeg_command,omitempty"` // Full FFmpeg command line
}

// CompressionRatio returns the output/input byte ratio.
func (s *TranscodeStats) CompressionRatio() float64 {
	if s.BytesIn == 0 {
		return 0
	}
	return float64(s.BytesOut) / float64(s.BytesIn)
}
