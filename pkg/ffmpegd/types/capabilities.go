package types

// Capabilities describes what a daemon can do.
type Capabilities struct {
	// Hardware acceleration
	HWAccels []HWAccelInfo `json:"hw_accels"`
	GPUs     []GPUInfo     `json:"gpus"`

	// Codec support
	VideoEncoders []string `json:"video_encoders"`
	VideoDecoders []string `json:"video_decoders"`
	AudioEncoders []string `json:"audio_encoders"`
	AudioDecoders []string `json:"audio_decoders"`

	// Capacity limits - MaxConcurrentJobs is the overall guard limit
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`

	// Optional overrides for CPU/GPU job limits (if 0, use detected defaults)
	// MaxCPUJobs: defaults to detected CPU cores
	// MaxGPUJobs: defaults to sum of GPU encode sessions
	// MaxProbeJobs: defaults to MaxConcurrentJobs
	MaxCPUJobs   int `json:"max_cpu_jobs,omitempty"`
	MaxGPUJobs   int `json:"max_gpu_jobs,omitempty"`
	MaxProbeJobs int `json:"max_probe_jobs,omitempty"`

	// Performance baseline (optional benchmarks)
	Performance *PerformanceMetrics `json:"performance,omitempty"`
}

// HasEncoder returns true if the capability includes the given encoder.
func (c *Capabilities) HasEncoder(encoder string) bool {
	for _, enc := range c.VideoEncoders {
		if enc == encoder {
			return true
		}
	}
	for _, enc := range c.AudioEncoders {
		if enc == encoder {
			return true
		}
	}
	return false
}

// HasDecoder returns true if the capability includes the given decoder.
func (c *Capabilities) HasDecoder(decoder string) bool {
	for _, dec := range c.VideoDecoders {
		if dec == decoder {
			return true
		}
	}
	for _, dec := range c.AudioDecoders {
		if dec == decoder {
			return true
		}
	}
	return false
}

// HasHWAccel returns true if the capability includes the given hardware acceleration type.
func (c *Capabilities) HasHWAccel(hwType HWAccelType) bool {
	for _, hw := range c.HWAccels {
		if hw.Type == hwType && hw.Available {
			return true
		}
	}
	return false
}

// FilteredEncoder describes an encoder that was filtered out and why.
type FilteredEncoder struct {
	Name   string `json:"name"`   // Encoder name (e.g., "vp9_vaapi")
	Reason string `json:"reason"` // Reason it was filtered
}

// HWAccelInfo describes a hardware acceleration method.
type HWAccelInfo struct {
	Type             HWAccelType       `json:"type"`              // vaapi, cuda, qsv, videotoolbox
	Device           string            `json:"device"`            // /dev/dri/renderD128, etc.
	Available        bool              `json:"available"`
	Encoders         []string          `json:"encoders"`          // Validated HW encoders: h264_nvenc, hevc_vaapi
	Decoders         []string          `json:"decoders"`          // HW decoders: h264_cuvid, hevc_qsv
	FilteredEncoders []FilteredEncoder `json:"filtered_encoders"` // Encoders filtered out with reasons
}

// HWAccelType enumerates hardware acceleration types.
type HWAccelType string

const (
	HWAccelNone         HWAccelType = "none"
	HWAccelVAAPI        HWAccelType = "vaapi"
	HWAccelCUDA         HWAccelType = "cuda"
	HWAccelQSV          HWAccelType = "qsv"
	HWAccelVideoToolbox HWAccelType = "videotoolbox"
	HWAccelD3D11VA      HWAccelType = "d3d11va"
	HWAccelDXVA2        HWAccelType = "dxva2"
	HWAccelVulkan       HWAccelType = "vulkan"
	HWAccelOpenCL       HWAccelType = "opencl"
)

// String implements fmt.Stringer.
func (t HWAccelType) String() string {
	return string(t)
}

// GPUInfo describes a GPU and its session limits.
type GPUInfo struct {
	Index  int      `json:"index"`
	Name   string   `json:"name"`   // "NVIDIA GeForce RTX 3080"
	Class  GPUClass `json:"class"`
	Driver string   `json:"driver"` // Driver version

	// Session limits (critical for load balancing)
	MaxEncodeSessions    int `json:"max_encode_sessions"`
	MaxDecodeSessions    int `json:"max_decode_sessions"`
	ActiveEncodeSessions int `json:"active_encode_sessions"`
	ActiveDecodeSessions int `json:"active_decode_sessions"`

	// Utilization (from heartbeat)
	Utilization float64 `json:"utilization"`     // 0-100%
	MemoryTotal uint64  `json:"memory_total"`    // bytes
	MemoryUsed  uint64  `json:"memory_used"`     // bytes
	EncoderUtil float64 `json:"encoder_util"`    // 0-100%
	DecoderUtil float64 `json:"decoder_util"`    // 0-100%
	Temperature int     `json:"temperature"`     // Celsius
	PowerWatts  int     `json:"power_watts"`
}

// AvailableEncodeSessions returns the number of available encode sessions.
func (g *GPUInfo) AvailableEncodeSessions() int {
	return g.MaxEncodeSessions - g.ActiveEncodeSessions
}

// AvailableDecodeSessions returns the number of available decode sessions.
func (g *GPUInfo) AvailableDecodeSessions() int {
	return g.MaxDecodeSessions - g.ActiveDecodeSessions
}

// GPUClass categorizes GPU types for session limit detection.
type GPUClass string

const (
	GPUClassUnknown      GPUClass = "unknown"
	GPUClassConsumer     GPUClass = "consumer"     // GeForce, Radeon
	GPUClassProfessional GPUClass = "professional" // Quadro, Pro
	GPUClassDatacenter   GPUClass = "datacenter"   // Tesla, A100
	GPUClassIntegrated   GPUClass = "integrated"   // Intel iGPU, AMD APU
)

// String implements fmt.Stringer.
func (c GPUClass) String() string {
	return string(c)
}

// DefaultMaxEncodeSessions returns the default max encode sessions for this GPU class.
func (c GPUClass) DefaultMaxEncodeSessions() int {
	switch c {
	case GPUClassConsumer:
		return 5 // NVIDIA consumer cards limit (can be 3 on older cards)
	case GPUClassProfessional:
		return 32 // Quadro/Pro cards have higher limits
	case GPUClassDatacenter:
		return 0 // Unlimited (0 means no limit)
	case GPUClassIntegrated:
		return 2 // Integrated GPUs typically have limited resources
	default:
		return 3 // Conservative default
	}
}

// PerformanceMetrics contains optional benchmark results.
type PerformanceMetrics struct {
	H264_1080p_FPS float64 `json:"h264_1080p_fps,omitempty"`
	H265_1080p_FPS float64 `json:"h265_1080p_fps,omitempty"`
	MemoryGB       float64 `json:"memory_gb"`
	CPUCores       int     `json:"cpu_cores"`
}
