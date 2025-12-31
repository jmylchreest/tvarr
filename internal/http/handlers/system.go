// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
)

// FFmpegInfoProvider provides FFmpeg binary information.
type FFmpegInfoProvider interface {
	GetFFmpegInfo(ctx context.Context) (*ffmpeg.BinaryInfo, error)
}

// SystemHandler handles system information endpoints.
type SystemHandler struct {
	ffmpegProvider FFmpegInfoProvider
}

// NewSystemHandler creates a new system handler.
func NewSystemHandler(ffmpegProvider FFmpegInfoProvider) *SystemHandler {
	return &SystemHandler{
		ffmpegProvider: ffmpegProvider,
	}
}

// FFmpegInfoInput is the input for the FFmpeg info endpoint.
type FFmpegInfoInput struct{}

// FFmpegInfoOutput is the output for the FFmpeg info endpoint.
type FFmpegInfoOutput struct {
	Body FFmpegInfoResponse
}

// FFmpegInfoResponse represents the FFmpeg capabilities response.
type FFmpegInfoResponse struct {
	Available     bool                     `json:"available" doc:"Whether FFmpeg is available"`
	FFmpegPath    string                   `json:"ffmpeg_path,omitempty" doc:"Path to FFmpeg binary"`
	FFprobePath   string                   `json:"ffprobe_path,omitempty" doc:"Path to FFprobe binary"`
	Version       string                   `json:"version,omitempty" doc:"FFmpeg version string"`
	MajorVersion  int                      `json:"major_version,omitempty" doc:"Major version number"`
	MinorVersion  int                      `json:"minor_version,omitempty" doc:"Minor version number"`
	BuildDate     string                   `json:"build_date,omitempty" doc:"Build date/compiler info"`
	Configuration string                   `json:"configuration,omitempty" doc:"Build configuration flags"`
	Codecs        []FFmpegCodecResponse    `json:"codecs,omitempty" doc:"Available codecs"`
	Encoders      []string                 `json:"encoders,omitempty" doc:"Available encoders"`
	Decoders      []string                 `json:"decoders,omitempty" doc:"Available decoders"`
	HWAccels      []FFmpegHWAccelResponse  `json:"hw_accels,omitempty" doc:"Hardware acceleration methods"`
	Formats       []FFmpegFormatResponse   `json:"formats,omitempty" doc:"Available formats"`
	Recommended   *FFmpegRecommendedConfig `json:"recommended,omitempty" doc:"Recommended configuration"`
}

// FFmpegCodecResponse represents a codec in the API response.
type FFmpegCodecResponse struct {
	Name        string `json:"name" doc:"Codec name"`
	LongName    string `json:"long_name,omitempty" doc:"Human-readable name"`
	Type        string `json:"type" doc:"Codec type: video, audio, subtitle, data"`
	CanDecode   bool   `json:"can_decode" doc:"Supports decoding"`
	CanEncode   bool   `json:"can_encode" doc:"Supports encoding"`
	IsLossy     bool   `json:"is_lossy,omitempty" doc:"Lossy compression"`
	IsLossless  bool   `json:"is_lossless,omitempty" doc:"Lossless compression"`
	IsIntraOnly bool   `json:"is_intra_only,omitempty" doc:"Intra-frame only"`
}

// FFmpegHWAccelResponse represents a hardware accelerator in the API response.
type FFmpegHWAccelResponse struct {
	Type       string   `json:"type" doc:"Hardware acceleration type"`
	Name       string   `json:"name" doc:"Hardware acceleration name"`
	Available  bool     `json:"available" doc:"Whether the accelerator is available and functional"`
	DeviceName string   `json:"device_name,omitempty" doc:"Device name or path"`
	Encoders   []string `json:"encoders,omitempty" doc:"Available hardware encoders"`
	Decoders   []string `json:"decoders,omitempty" doc:"Available hardware decoders"`
}

// FFmpegFormatResponse represents a format in the API response.
type FFmpegFormatResponse struct {
	Name     string `json:"name" doc:"Format name"`
	LongName string `json:"long_name,omitempty" doc:"Human-readable name"`
	CanMux   bool   `json:"can_mux" doc:"Supports muxing (writing)"`
	CanDemux bool   `json:"can_demux" doc:"Supports demuxing (reading)"`
}

// FFmpegRecommendedConfig contains recommended FFmpeg configuration.
type FFmpegRecommendedConfig struct {
	HWAccel      string `json:"hw_accel,omitempty" doc:"Recommended hardware acceleration method"`
	HWAccelName  string `json:"hw_accel_name,omitempty" doc:"Human-readable name of recommended HW accel"`
	VideoEncoder string `json:"video_encoder,omitempty" doc:"Recommended video encoder"`
	AudioEncoder string `json:"audio_encoder,omitempty" doc:"Recommended audio encoder"`
}

// Register registers the system routes with the API.
func (h *SystemHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getFFmpegInfo",
		Method:      "GET",
		Path:        "/api/v1/system/ffmpeg",
		Summary:     "Get FFmpeg capabilities",
		Description: "Returns detailed information about the FFmpeg installation including version, codecs, hardware acceleration, and recommended configuration",
		Tags:        []string{"System"},
	}, h.GetFFmpegInfo)
}

// GetFFmpegInfo returns FFmpeg capabilities and configuration.
func (h *SystemHandler) GetFFmpegInfo(ctx context.Context, input *FFmpegInfoInput) (*FFmpegInfoOutput, error) {
	info, err := h.ffmpegProvider.GetFFmpegInfo(ctx)
	if err != nil {
		// FFmpeg not available - return minimal response
		return &FFmpegInfoOutput{
			Body: FFmpegInfoResponse{
				Available: false,
			},
		}, nil
	}

	response := FFmpegInfoResponse{
		Available:     true,
		FFmpegPath:    info.FFmpegPath,
		FFprobePath:   info.FFprobePath,
		Version:       info.Version,
		MajorVersion:  info.MajorVersion,
		MinorVersion:  info.MinorVersion,
		BuildDate:     info.BuildDate,
		Configuration: info.Configuration,
		Encoders:      info.Encoders,
		Decoders:      info.Decoders,
	}

	// Convert codecs
	response.Codecs = make([]FFmpegCodecResponse, 0, len(info.Codecs))
	for _, codec := range info.Codecs {
		response.Codecs = append(response.Codecs, FFmpegCodecResponse{
			Name:        codec.Name,
			LongName:    codec.LongName,
			Type:        codec.Type,
			CanDecode:   codec.CanDecode,
			CanEncode:   codec.CanEncode,
			IsLossy:     codec.IsLossy,
			IsLossless:  codec.IsLossless,
			IsIntraOnly: codec.IsIntraOnly,
		})
	}

	// Convert hardware accelerators
	response.HWAccels = make([]FFmpegHWAccelResponse, 0, len(info.HWAccels))
	for _, accel := range info.HWAccels {
		response.HWAccels = append(response.HWAccels, FFmpegHWAccelResponse{
			Type:       string(accel.Type),
			Name:       accel.Name,
			Available:  accel.Available,
			DeviceName: accel.DeviceName,
			Encoders:   accel.Encoders,
			Decoders:   accel.Decoders,
		})
	}

	// Convert formats
	response.Formats = make([]FFmpegFormatResponse, 0, len(info.Formats))
	for _, format := range info.Formats {
		response.Formats = append(response.Formats, FFmpegFormatResponse{
			Name:     format.Name,
			LongName: format.LongName,
			CanMux:   format.CanMux,
			CanDemux: format.CanDemux,
		})
	}

	// Add recommended configuration
	if recommended := ffmpeg.GetRecommendedHWAccel(info.HWAccels); recommended != nil {
		response.Recommended = &FFmpegRecommendedConfig{
			HWAccel:     string(recommended.Type),
			HWAccelName: recommended.Name,
		}
		// Suggest video encoder based on available HW encoders
		if len(recommended.Encoders) > 0 {
			for _, enc := range recommended.Encoders {
				// Prefer H.264 encoder for compatibility
				if containsSubstring(enc, "h264") || containsSubstring(enc, "264") {
					response.Recommended.VideoEncoder = enc
					break
				}
			}
			// Fall back to first available encoder if no H.264
			if response.Recommended.VideoEncoder == "" {
				response.Recommended.VideoEncoder = recommended.Encoders[0]
			}
		}
	}

	// Default audio encoder recommendation
	if info.HasEncoder("aac") {
		if response.Recommended == nil {
			response.Recommended = &FFmpegRecommendedConfig{}
		}
		response.Recommended.AudioEncoder = "aac"
	}

	return &FFmpegInfoOutput{
		Body: response,
	}, nil
}

// containsSubstring checks if s contains substr (case-insensitive).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			// Simple lowercase comparison for ASCII
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
