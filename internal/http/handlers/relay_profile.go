package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/service"
	"gorm.io/gorm"
)

// RelayProfileHandler handles relay profile API endpoints.
type RelayProfileHandler struct {
	relayService *service.RelayService
}

// NewRelayProfileHandler creates a new relay profile handler.
func NewRelayProfileHandler(relayService *service.RelayService) *RelayProfileHandler {
	return &RelayProfileHandler{
		relayService: relayService,
	}
}

// Register registers the relay profile routes with the API.
func (h *RelayProfileHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listRelayProfiles",
		Method:      "GET",
		Path:        "/api/v1/relay/profiles",
		Summary:     "List relay profiles",
		Description: "Returns all relay profiles",
		Tags:        []string{"Relay Profiles"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getRelayProfile",
		Method:      "GET",
		Path:        "/api/v1/relay/profiles/{id}",
		Summary:     "Get relay profile",
		Description: "Returns a relay profile by ID",
		Tags:        []string{"Relay Profiles"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "getDefaultRelayProfile",
		Method:      "GET",
		Path:        "/api/v1/relay/profiles/default",
		Summary:     "Get default relay profile",
		Description: "Returns the default relay profile",
		Tags:        []string{"Relay Profiles"},
	}, h.GetDefault)

	huma.Register(api, huma.Operation{
		OperationID: "createRelayProfile",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles",
		Summary:     "Create relay profile",
		Description: "Creates a new relay profile",
		Tags:        []string{"Relay Profiles"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateRelayProfile",
		Method:      "PUT",
		Path:        "/api/v1/relay/profiles/{id}",
		Summary:     "Update relay profile",
		Description: "Updates an existing relay profile",
		Tags:        []string{"Relay Profiles"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteRelayProfile",
		Method:      "DELETE",
		Path:        "/api/v1/relay/profiles/{id}",
		Summary:     "Delete relay profile",
		Description: "Deletes a relay profile",
		Tags:        []string{"Relay Profiles"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "setDefaultRelayProfile",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles/{id}/set-default",
		Summary:     "Set default relay profile",
		Description: "Sets a relay profile as the default",
		Tags:        []string{"Relay Profiles"},
	}, h.SetDefault)

	huma.Register(api, huma.Operation{
		OperationID: "getFFmpegInfo",
		Method:      "GET",
		Path:        "/api/v1/relay/ffmpeg",
		Summary:     "Get FFmpeg info",
		Description: "Returns information about the detected FFmpeg installation",
		Tags:        []string{"Relay Profiles"},
	}, h.GetFFmpegInfo)

	huma.Register(api, huma.Operation{
		OperationID: "getRelayStats",
		Method:      "GET",
		Path:        "/api/v1/relay/stats",
		Summary:     "Get relay statistics",
		Description: "Returns statistics about active relay sessions",
		Tags:        []string{"Relay Profiles"},
	}, h.GetStats)

	huma.Register(api, huma.Operation{
		OperationID: "getRelayHealth",
		Method:      "GET",
		Path:        "/api/v1/relay/health",
		Summary:     "Get relay health status",
		Description: "Returns health status of relay processes",
		Tags:        []string{"Relay Profiles"},
	}, h.GetHealth)

	huma.Register(api, huma.Operation{
		OperationID: "validateRelayProfileFlags",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles/validate-flags",
		Summary:     "Validate custom FFmpeg flags",
		Description: "Validates custom FFmpeg flags for security and correctness",
		Tags:        []string{"Relay Profiles"},
	}, h.ValidateFlags)

	huma.Register(api, huma.Operation{
		OperationID: "cloneRelayProfile",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles/{id}/clone",
		Summary:     "Clone relay profile",
		Description: "Creates a copy of an existing relay profile",
		Tags:        []string{"Relay Profiles"},
	}, h.Clone)

	huma.Register(api, huma.Operation{
		OperationID: "getHardwareCapabilities",
		Method:      "GET",
		Path:        "/api/v1/hardware-capabilities",
		Summary:     "Get hardware capabilities",
		Description: "Returns detected hardware acceleration capabilities",
		Tags:        []string{"Hardware"},
	}, h.GetHardwareCapabilities)

	huma.Register(api, huma.Operation{
		OperationID: "refreshHardwareCapabilities",
		Method:      "POST",
		Path:        "/api/v1/hardware-capabilities/refresh",
		Summary:     "Refresh hardware capabilities",
		Description: "Re-detects hardware acceleration capabilities",
		Tags:        []string{"Hardware"},
	}, h.RefreshHardwareCapabilities)

	huma.Register(api, huma.Operation{
		OperationID: "testRelayProfile",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles/{id}/test",
		Summary:     "Test relay profile",
		Description: "Tests a relay profile against a sample stream to verify it works correctly",
		Tags:        []string{"Relay Profiles"},
	}, h.TestProfile)

	huma.Register(api, huma.Operation{
		OperationID: "previewRelayProfileCommand",
		Method:      "POST",
		Path:        "/api/v1/relay/profiles/{id}/preview",
		Summary:     "Preview FFmpeg command",
		Description: "Returns a preview of the FFmpeg command that would be generated for this profile",
		Tags:        []string{"Relay Profiles"},
	}, h.PreviewCommand)
}

// RelayProfileResponse represents a relay profile in API responses.
type RelayProfileResponse struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	Description              string `json:"description,omitempty"`
	VideoCodec               string `json:"video_codec"`
	AudioCodec               string `json:"audio_codec"`
	VideoBitrate             int    `json:"video_bitrate,omitempty"`
	AudioBitrate             int    `json:"audio_bitrate,omitempty"`
	VideoMaxrate             int    `json:"video_maxrate,omitempty"`
	VideoPreset              string `json:"video_preset,omitempty"`
	VideoWidth               int    `json:"video_width,omitempty"`
	VideoHeight              int    `json:"video_height,omitempty"`
	AudioSampleRate          int    `json:"audio_sample_rate,omitempty"`
	AudioChannels            int    `json:"audio_channels,omitempty"`
	HWAccel                  string `json:"hw_accel"`
	HWAccelDevice            string `json:"hw_accel_device,omitempty"`
	HWAccelOutputFormat      string `json:"hw_accel_output_format,omitempty"`
	HWAccelDecoderCodec      string `json:"hw_accel_decoder_codec,omitempty"`
	HWAccelExtraOptions      string `json:"hw_accel_extra_options,omitempty"`
	GpuIndex                 int    `json:"gpu_index,omitempty"`
	OutputFormat             string `json:"output_format"`
	InputOptions             string `json:"input_options,omitempty" doc:"Custom FFmpeg input options"`
	OutputOptions            string `json:"output_options,omitempty" doc:"Custom FFmpeg output options"`
	FilterComplex            string `json:"filter_complex,omitempty" doc:"Custom filter complex string"`
	CustomFlagsValidated     bool   `json:"custom_flags_validated" doc:"Whether custom flags have been validated"`
	CustomFlagsWarnings      string `json:"custom_flags_warnings,omitempty" doc:"Validation warnings for custom flags"`
	IsDefault                bool   `json:"is_default"`
	IsSystem                 bool   `json:"is_system" doc:"Whether this is a system-provided profile (cannot be edited/deleted)"`
	Enabled                  bool   `json:"enabled"`
	FallbackEnabled          bool   `json:"fallback_enabled"`
	FallbackErrorThreshold   int    `json:"fallback_error_threshold"`
	FallbackRecoveryInterval int    `json:"fallback_recovery_interval"`
	// Statistics
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
	LastUsedAt   string `json:"last_used_at,omitempty"`
	LastErrorAt  string `json:"last_error_at,omitempty"`
	LastErrorMsg string `json:"last_error_msg,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// RelayProfileFromModel converts a model to a response.
func RelayProfileFromModel(p *models.RelayProfile) RelayProfileResponse {
	resp := RelayProfileResponse{
		ID:                       p.ID.String(),
		Name:                     p.Name,
		Description:              p.Description,
		VideoCodec:               string(p.VideoCodec),
		AudioCodec:               string(p.AudioCodec),
		VideoBitrate:             p.VideoBitrate,
		AudioBitrate:             p.AudioBitrate,
		VideoMaxrate:             p.VideoMaxrate,
		VideoPreset:              p.VideoPreset,
		VideoWidth:               p.VideoWidth,
		VideoHeight:              p.VideoHeight,
		AudioSampleRate:          p.AudioSampleRate,
		AudioChannels:            p.AudioChannels,
		HWAccel:                  string(p.HWAccel),
		HWAccelDevice:            p.HWAccelDevice,
		HWAccelOutputFormat:      p.HWAccelOutputFormat,
		HWAccelDecoderCodec:      p.HWAccelDecoderCodec,
		HWAccelExtraOptions:      p.HWAccelExtraOptions,
		GpuIndex:                 p.GpuIndex,
		OutputFormat:             string(p.OutputFormat),
		InputOptions:             p.InputOptions,
		OutputOptions:            p.OutputOptions,
		FilterComplex:            p.FilterComplex,
		CustomFlagsValidated:     p.CustomFlagsValidated,
		CustomFlagsWarnings:      p.CustomFlagsWarnings,
		IsDefault:                p.IsDefault,
		IsSystem:                 p.IsSystem,
		Enabled:                  p.Enabled,
		FallbackEnabled:          p.FallbackEnabled,
		FallbackErrorThreshold:   p.FallbackErrorThreshold,
		FallbackRecoveryInterval: p.FallbackRecoveryInterval,
		SuccessCount:             p.SuccessCount,
		FailureCount:             p.FailureCount,
		LastErrorMsg:             p.LastErrorMsg,
		CreatedAt:                p.CreatedAt.String(),
		UpdatedAt:                p.UpdatedAt.String(),
	}

	// Format optional time fields
	if !p.LastUsedAt.IsZero() {
		resp.LastUsedAt = p.LastUsedAt.Format(time.RFC3339)
	}
	if !p.LastErrorAt.IsZero() {
		resp.LastErrorAt = p.LastErrorAt.Format(time.RFC3339)
	}

	return resp
}

// ListRelayProfilesInput is the input for listing relay profiles.
type ListRelayProfilesInput struct{}

// ListRelayProfilesOutput is the output for listing relay profiles.
type ListRelayProfilesOutput struct {
	Body struct {
		Profiles []RelayProfileResponse `json:"profiles"`
	}
}

// List returns all relay profiles.
func (h *RelayProfileHandler) List(ctx context.Context, input *ListRelayProfilesInput) (*ListRelayProfilesOutput, error) {
	profiles, err := h.relayService.GetAllProfiles(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list relay profiles", err)
	}

	resp := &ListRelayProfilesOutput{}
	resp.Body.Profiles = make([]RelayProfileResponse, 0, len(profiles))
	for _, p := range profiles {
		resp.Body.Profiles = append(resp.Body.Profiles, RelayProfileFromModel(p))
	}

	return resp, nil
}

// GetRelayProfileInput is the input for getting a relay profile.
type GetRelayProfileInput struct {
	ID string `path:"id" doc:"Relay profile ID (UUID)"`
}

// GetRelayProfileOutput is the output for getting a relay profile.
type GetRelayProfileOutput struct {
	Body RelayProfileResponse
}

// GetByID returns a relay profile by ID.
func (h *RelayProfileHandler) GetByID(ctx context.Context, input *GetRelayProfileInput) (*GetRelayProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile", err)
	}

	return &GetRelayProfileOutput{
		Body: RelayProfileFromModel(profile),
	}, nil
}

// GetDefaultRelayProfileInput is the input for getting the default relay profile.
type GetDefaultRelayProfileInput struct{}

// GetDefaultRelayProfileOutput is the output for getting the default relay profile.
type GetDefaultRelayProfileOutput struct {
	Body RelayProfileResponse
}

// GetDefault returns the default relay profile.
func (h *RelayProfileHandler) GetDefault(ctx context.Context, input *GetDefaultRelayProfileInput) (*GetDefaultRelayProfileOutput, error) {
	profile, err := h.relayService.GetDefaultProfile(ctx)
	if err != nil {
		if errors.Is(err, models.ErrRelayProfileNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("no default relay profile set")
		}
		return nil, huma.Error500InternalServerError("failed to get default relay profile", err)
	}

	return &GetDefaultRelayProfileOutput{
		Body: RelayProfileFromModel(profile),
	}, nil
}

// CreateRelayProfileInput is the input for creating a relay profile.
type CreateRelayProfileInput struct {
	Body struct {
		Name                string `json:"name" required:"true" doc:"Profile name"`
		Description         string `json:"description,omitempty" doc:"Profile description"`
		VideoCodec          string `json:"video_codec" doc:"Video codec (copy, h264, hevc, vp9, av1)"`
		AudioCodec          string `json:"audio_codec" doc:"Audio codec (copy, aac, mp3, opus, flac)"`
		VideoBitrate        int    `json:"video_bitrate,omitempty" doc:"Video bitrate in kbps"`
		AudioBitrate        int    `json:"audio_bitrate,omitempty" doc:"Audio bitrate in kbps"`
		VideoMaxrate        int    `json:"video_maxrate,omitempty" doc:"Video max bitrate in kbps"`
		VideoPreset         string `json:"video_preset,omitempty" doc:"Encoder preset"`
		VideoWidth          int    `json:"video_width,omitempty" doc:"Output video width"`
		VideoHeight         int    `json:"video_height,omitempty" doc:"Output video height"`
		AudioSampleRate     int    `json:"audio_sample_rate,omitempty" doc:"Audio sample rate in Hz"`
		AudioChannels       int    `json:"audio_channels,omitempty" doc:"Audio channels (1=mono, 2=stereo)"`
		HWAccel             string `json:"hw_accel,omitempty" doc:"Hardware acceleration (none, nvenc, qsv, vaapi, amf, videotoolbox)"`
		HWAccelDevice       string `json:"hw_accel_device,omitempty" doc:"Hardware acceleration device path"`
		HWAccelOutputFormat string `json:"hw_accel_output_format,omitempty" doc:"Hardware acceleration output format"`
		HWAccelDecoderCodec string `json:"hw_accel_decoder_codec,omitempty" doc:"Hardware acceleration decoder codec"`
		HWAccelExtraOptions string `json:"hw_accel_extra_options,omitempty" doc:"Extra hardware acceleration options"`
		GpuIndex            int    `json:"gpu_index,omitempty" doc:"GPU device index (-1 for auto)"`
		OutputFormat        string `json:"output_format,omitempty" doc:"Output format (mpegts, hls, flv, mp4)"`
		InputOptions        string `json:"input_options,omitempty" doc:"Custom FFmpeg input options"`
		OutputOptions       string `json:"output_options,omitempty" doc:"Custom FFmpeg output options"`
		FilterComplex       string `json:"filter_complex,omitempty" doc:"Custom filter complex string"`
		IsDefault           bool   `json:"is_default,omitempty" doc:"Set as default profile"`
	}
}

// CreateRelayProfileOutput is the output for creating a relay profile.
type CreateRelayProfileOutput struct {
	Body RelayProfileResponse
}

// Create creates a new relay profile.
func (h *RelayProfileHandler) Create(ctx context.Context, input *CreateRelayProfileInput) (*CreateRelayProfileOutput, error) {
	// Validate custom flags if provided
	var flagsValidated bool
	var flagsWarnings string
	if input.Body.InputOptions != "" || input.Body.OutputOptions != "" || input.Body.FilterComplex != "" {
		result := ffmpeg.ValidateCustomFlags(input.Body.InputOptions, input.Body.OutputOptions, input.Body.FilterComplex)
		if !result.Valid {
			return nil, huma.Error400BadRequest(fmt.Sprintf("invalid custom flags: %v", result.Errors))
		}
		flagsValidated = true
		if len(result.Warnings) > 0 {
			flagsWarnings = fmt.Sprintf("%v", result.Warnings)
		}
	}

	profile := &models.RelayProfile{
		Name:                 input.Body.Name,
		Description:          input.Body.Description,
		VideoCodec:           models.VideoCodec(input.Body.VideoCodec),
		AudioCodec:           models.AudioCodec(input.Body.AudioCodec),
		VideoBitrate:         input.Body.VideoBitrate,
		AudioBitrate:         input.Body.AudioBitrate,
		VideoMaxrate:         input.Body.VideoMaxrate,
		VideoPreset:          input.Body.VideoPreset,
		VideoWidth:           input.Body.VideoWidth,
		VideoHeight:          input.Body.VideoHeight,
		AudioSampleRate:      input.Body.AudioSampleRate,
		AudioChannels:        input.Body.AudioChannels,
		HWAccel:              models.HWAccelType(input.Body.HWAccel),
		HWAccelDevice:        input.Body.HWAccelDevice,
		HWAccelOutputFormat:  input.Body.HWAccelOutputFormat,
		HWAccelDecoderCodec:  input.Body.HWAccelDecoderCodec,
		HWAccelExtraOptions:  input.Body.HWAccelExtraOptions,
		GpuIndex:             input.Body.GpuIndex,
		OutputFormat:         models.OutputFormat(input.Body.OutputFormat),
		InputOptions:         input.Body.InputOptions,
		OutputOptions:        input.Body.OutputOptions,
		FilterComplex:        input.Body.FilterComplex,
		CustomFlagsValidated: flagsValidated,
		CustomFlagsWarnings:  flagsWarnings,
		IsDefault:            input.Body.IsDefault,
	}

	// Set defaults if not provided
	if profile.VideoCodec == "" {
		profile.VideoCodec = models.VideoCodecCopy
	}
	if profile.AudioCodec == "" {
		profile.AudioCodec = models.AudioCodecCopy
	}
	if profile.HWAccel == "" {
		profile.HWAccel = models.HWAccelNone
	}
	if profile.OutputFormat == "" {
		profile.OutputFormat = models.OutputFormatMPEGTS
	}
	// Default GPU index to -1 (auto)
	if profile.GpuIndex == 0 && input.Body.GpuIndex == 0 {
		profile.GpuIndex = -1
	}

	if err := h.relayService.CreateProfile(ctx, profile); err != nil {
		return nil, huma.Error500InternalServerError("failed to create relay profile", err)
	}

	return &CreateRelayProfileOutput{
		Body: RelayProfileFromModel(profile),
	}, nil
}

// UpdateRelayProfileInput is the input for updating a relay profile.
type UpdateRelayProfileInput struct {
	ID   string `path:"id" doc:"Relay profile ID (UUID)"`
	Body struct {
		Name                string  `json:"name,omitempty" doc:"Profile name"`
		Description         string  `json:"description,omitempty" doc:"Profile description"`
		VideoCodec          string  `json:"video_codec,omitempty" doc:"Video codec"`
		AudioCodec          string  `json:"audio_codec,omitempty" doc:"Audio codec"`
		VideoBitrate        int     `json:"video_bitrate,omitempty" doc:"Video bitrate in kbps"`
		AudioBitrate        int     `json:"audio_bitrate,omitempty" doc:"Audio bitrate in kbps"`
		VideoMaxrate        int     `json:"video_maxrate,omitempty" doc:"Video max bitrate in kbps"`
		VideoPreset         string  `json:"video_preset,omitempty" doc:"Encoder preset"`
		VideoWidth          int     `json:"video_width,omitempty" doc:"Output video width"`
		VideoHeight         int     `json:"video_height,omitempty" doc:"Output video height"`
		AudioSampleRate     int     `json:"audio_sample_rate,omitempty" doc:"Audio sample rate in Hz"`
		AudioChannels       int     `json:"audio_channels,omitempty" doc:"Audio channels"`
		HWAccel             string  `json:"hw_accel,omitempty" doc:"Hardware acceleration"`
		HWAccelDevice       *string `json:"hw_accel_device,omitempty" doc:"Hardware acceleration device path"`
		HWAccelOutputFormat *string `json:"hw_accel_output_format,omitempty" doc:"Hardware acceleration output format"`
		HWAccelDecoderCodec *string `json:"hw_accel_decoder_codec,omitempty" doc:"Hardware acceleration decoder codec"`
		HWAccelExtraOptions *string `json:"hw_accel_extra_options,omitempty" doc:"Extra hardware acceleration options"`
		GpuIndex            *int    `json:"gpu_index,omitempty" doc:"GPU device index (-1 for auto)"`
		OutputFormat        string  `json:"output_format,omitempty" doc:"Output format"`
		InputOptions        *string `json:"input_options,omitempty" doc:"Custom FFmpeg input options"`
		OutputOptions       *string `json:"output_options,omitempty" doc:"Custom FFmpeg output options"`
		FilterComplex       *string `json:"filter_complex,omitempty" doc:"Custom filter complex string"`
		Enabled             *bool   `json:"enabled,omitempty" doc:"Whether the profile is enabled"`
	}
}

// UpdateRelayProfileOutput is the output for updating a relay profile.
type UpdateRelayProfileOutput struct {
	Body RelayProfileResponse
}

// Update updates an existing relay profile.
func (h *RelayProfileHandler) Update(ctx context.Context, input *UpdateRelayProfileInput) (*UpdateRelayProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	// Get existing profile
	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile", err)
	}

	// System profiles can only have Enabled toggled
	if profile.IsSystem {
		if input.Body.Name != "" || input.Body.Description != "" ||
			input.Body.VideoCodec != "" || input.Body.AudioCodec != "" ||
			input.Body.VideoBitrate != 0 || input.Body.AudioBitrate != 0 ||
			input.Body.VideoMaxrate != 0 || input.Body.VideoPreset != "" ||
			input.Body.VideoWidth != 0 || input.Body.VideoHeight != 0 ||
			input.Body.AudioSampleRate != 0 || input.Body.AudioChannels != 0 ||
			input.Body.HWAccel != "" || input.Body.OutputFormat != "" ||
			input.Body.InputOptions != nil || input.Body.OutputOptions != nil ||
			input.Body.FilterComplex != nil || input.Body.HWAccelDevice != nil ||
			input.Body.HWAccelOutputFormat != nil || input.Body.HWAccelDecoderCodec != nil ||
			input.Body.HWAccelExtraOptions != nil || input.Body.GpuIndex != nil {
			return nil, huma.Error403Forbidden("system profiles can only have enabled toggled")
		}
		// Only allow Enabled update
		if input.Body.Enabled != nil {
			profile.Enabled = *input.Body.Enabled
		}
	} else {
		// Update fields if provided for non-system profiles
		if input.Body.Name != "" {
			profile.Name = input.Body.Name
		}
		if input.Body.Description != "" {
			profile.Description = input.Body.Description
		}
		if input.Body.VideoCodec != "" {
			profile.VideoCodec = models.VideoCodec(input.Body.VideoCodec)
		}
		if input.Body.AudioCodec != "" {
			profile.AudioCodec = models.AudioCodec(input.Body.AudioCodec)
		}
		if input.Body.VideoBitrate != 0 {
			profile.VideoBitrate = input.Body.VideoBitrate
		}
		if input.Body.AudioBitrate != 0 {
			profile.AudioBitrate = input.Body.AudioBitrate
		}
		if input.Body.VideoMaxrate != 0 {
			profile.VideoMaxrate = input.Body.VideoMaxrate
		}
		if input.Body.VideoPreset != "" {
			profile.VideoPreset = input.Body.VideoPreset
		}
		if input.Body.VideoWidth != 0 {
			profile.VideoWidth = input.Body.VideoWidth
		}
		if input.Body.VideoHeight != 0 {
			profile.VideoHeight = input.Body.VideoHeight
		}
		if input.Body.AudioSampleRate != 0 {
			profile.AudioSampleRate = input.Body.AudioSampleRate
		}
		if input.Body.AudioChannels != 0 {
			profile.AudioChannels = input.Body.AudioChannels
		}
		if input.Body.HWAccel != "" {
			profile.HWAccel = models.HWAccelType(input.Body.HWAccel)
		}
		if input.Body.HWAccelDevice != nil {
			profile.HWAccelDevice = *input.Body.HWAccelDevice
		}
		if input.Body.HWAccelOutputFormat != nil {
			profile.HWAccelOutputFormat = *input.Body.HWAccelOutputFormat
		}
		if input.Body.HWAccelDecoderCodec != nil {
			profile.HWAccelDecoderCodec = *input.Body.HWAccelDecoderCodec
		}
		if input.Body.HWAccelExtraOptions != nil {
			profile.HWAccelExtraOptions = *input.Body.HWAccelExtraOptions
		}
		if input.Body.GpuIndex != nil {
			profile.GpuIndex = *input.Body.GpuIndex
		}
		if input.Body.OutputFormat != "" {
			profile.OutputFormat = models.OutputFormat(input.Body.OutputFormat)
		}
		if input.Body.InputOptions != nil {
			profile.InputOptions = *input.Body.InputOptions
		}
		if input.Body.OutputOptions != nil {
			profile.OutputOptions = *input.Body.OutputOptions
		}
		if input.Body.FilterComplex != nil {
			profile.FilterComplex = *input.Body.FilterComplex
		}
		if input.Body.Enabled != nil {
			profile.Enabled = *input.Body.Enabled
		}

		// Validate custom flags if any were updated
		if input.Body.InputOptions != nil || input.Body.OutputOptions != nil || input.Body.FilterComplex != nil {
			result := ffmpeg.ValidateCustomFlags(profile.InputOptions, profile.OutputOptions, profile.FilterComplex)
			if !result.Valid {
				return nil, huma.Error400BadRequest(fmt.Sprintf("invalid custom flags: %v", result.Errors))
			}
			profile.CustomFlagsValidated = true
			if len(result.Warnings) > 0 {
				profile.CustomFlagsWarnings = fmt.Sprintf("%v", result.Warnings)
			} else {
				profile.CustomFlagsWarnings = ""
			}
		}
	}

	if err := h.relayService.UpdateProfile(ctx, profile); err != nil {
		return nil, huma.Error500InternalServerError("failed to update relay profile", err)
	}

	return &UpdateRelayProfileOutput{
		Body: RelayProfileFromModel(profile),
	}, nil
}

// DeleteRelayProfileInput is the input for deleting a relay profile.
type DeleteRelayProfileInput struct {
	ID string `path:"id" doc:"Relay profile ID (UUID)"`
}

// DeleteRelayProfileOutput is the output for deleting a relay profile.
type DeleteRelayProfileOutput struct{}

// Delete deletes a relay profile.
func (h *RelayProfileHandler) Delete(ctx context.Context, input *DeleteRelayProfileInput) (*DeleteRelayProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	// Check if profile exists and is not a system profile
	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile", err)
	}

	// Prevent deletion of system profiles
	if profile.IsSystem {
		return nil, huma.Error403Forbidden("system profiles cannot be deleted")
	}

	// Check if profile is in use by any proxies
	usage, err := h.relayService.GetProfileUsage(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to check profile usage", err)
	}

	if usage.ProxyCount > 0 {
		// Return 409 Conflict with the names of affected proxies
		proxyNames := make([]string, 0, len(usage.Proxies))
		for _, p := range usage.Proxies {
			proxyNames = append(proxyNames, p.Name)
		}
		return nil, huma.Error409Conflict(fmt.Sprintf(
			"profile is in use by %d proxy configuration(s): %v. Reassign these proxies to another profile before deleting.",
			usage.ProxyCount, proxyNames))
	}

	if err := h.relayService.DeleteProfile(ctx, id); err != nil {
		if errors.Is(err, models.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to delete relay profile", err)
	}

	return &DeleteRelayProfileOutput{}, nil
}

// SetDefaultRelayProfileInput is the input for setting the default relay profile.
type SetDefaultRelayProfileInput struct {
	ID string `path:"id" doc:"Relay profile ID (UUID)"`
}

// SetDefaultRelayProfileOutput is the output for setting the default relay profile.
type SetDefaultRelayProfileOutput struct {
	Body RelayProfileResponse
}

// SetDefault sets a relay profile as the default.
func (h *RelayProfileHandler) SetDefault(ctx context.Context, input *SetDefaultRelayProfileInput) (*SetDefaultRelayProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	if err := h.relayService.SetDefaultProfile(ctx, id); err != nil {
		if errors.Is(err, models.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to set default relay profile", err)
	}

	// Get the updated profile
	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated relay profile", err)
	}

	return &SetDefaultRelayProfileOutput{
		Body: RelayProfileFromModel(profile),
	}, nil
}

// GetFFmpegInfoInput is the input for getting FFmpeg info.
type GetFFmpegInfoInput struct{}

// GetFFmpegInfoOutput is the output for getting FFmpeg info.
type GetFFmpegInfoOutput struct {
	Body struct {
		FFmpegPath   string   `json:"ffmpeg_path"`
		FFprobePath  string   `json:"ffprobe_path"`
		Version      string   `json:"version"`
		MajorVersion int      `json:"major_version"`
		MinorVersion int      `json:"minor_version"`
		Encoders     []string `json:"encoders,omitempty"`
		Decoders     []string `json:"decoders,omitempty"`
	}
}

// GetFFmpegInfo returns information about the detected FFmpeg installation.
func (h *RelayProfileHandler) GetFFmpegInfo(ctx context.Context, input *GetFFmpegInfoInput) (*GetFFmpegInfoOutput, error) {
	info, err := h.relayService.GetFFmpegInfo(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to detect FFmpeg", err)
	}

	return &GetFFmpegInfoOutput{
		Body: struct {
			FFmpegPath   string   `json:"ffmpeg_path"`
			FFprobePath  string   `json:"ffprobe_path"`
			Version      string   `json:"version"`
			MajorVersion int      `json:"major_version"`
			MinorVersion int      `json:"minor_version"`
			Encoders     []string `json:"encoders,omitempty"`
			Decoders     []string `json:"decoders,omitempty"`
		}{
			FFmpegPath:   info.FFmpegPath,
			FFprobePath:  info.FFprobePath,
			Version:      info.Version,
			MajorVersion: info.MajorVersion,
			MinorVersion: info.MinorVersion,
			Encoders:     info.Encoders,
			Decoders:     info.Decoders,
		},
	}, nil
}

// GetRelayStatsInput is the input for getting relay stats.
type GetRelayStatsInput struct{}

// GetRelayStatsOutput is the output for getting relay stats.
type GetRelayStatsOutput struct {
	Body struct {
		ActiveSessions int                    `json:"active_sessions"`
		MaxSessions    int                    `json:"max_sessions"`
		Sessions       []map[string]any       `json:"sessions,omitempty"`
		ConnectionPool map[string]any         `json:"connection_pool"`
	}
}

// GetStats returns relay manager statistics.
func (h *RelayProfileHandler) GetStats(ctx context.Context, input *GetRelayStatsInput) (*GetRelayStatsOutput, error) {
	stats := h.relayService.GetRelayStats()

	sessions := make([]map[string]any, 0, len(stats.Sessions))
	for _, s := range stats.Sessions {
		sessions = append(sessions, map[string]any{
			"id":             s.ID,
			"channel_id":     s.ChannelID,
			"stream_url":     s.StreamURL,
			"classification": s.Classification,
			"started_at":     s.StartedAt,
			"last_activity":  s.LastActivity,
			"client_count":   s.ClientCount,
			"bytes_written":  s.BytesWritten,
			"closed":         s.Closed,
			"error":          s.Error,
		})
	}

	return &GetRelayStatsOutput{
		Body: struct {
			ActiveSessions int                    `json:"active_sessions"`
			MaxSessions    int                    `json:"max_sessions"`
			Sessions       []map[string]any       `json:"sessions,omitempty"`
			ConnectionPool map[string]any         `json:"connection_pool"`
		}{
			ActiveSessions: stats.ActiveSessions,
			MaxSessions:    stats.MaxSessions,
			Sessions:       sessions,
			ConnectionPool: map[string]any{
				"global_connections": stats.ConnectionPool.GlobalConnections,
				"max_global":         stats.ConnectionPool.MaxGlobal,
				"host_connections":   stats.ConnectionPool.HostConnections,
				"max_per_host":       stats.ConnectionPool.MaxPerHost,
				"waiting_count":      stats.ConnectionPool.WaitingCount,
			},
		},
	}, nil
}

// GetRelayHealthInput is the input for getting relay health.
type GetRelayHealthInput struct{}

// RelayConnectedClientHealth represents a connected client in the health response.
type RelayConnectedClientHealth struct {
	ID           string `json:"id"`
	IP           string `json:"ip"`
	UserAgent    string `json:"user_agent,omitempty"`
	ConnectedAt  string `json:"connected_at"`
	BytesServed  string `json:"bytes_served"`
	LastActivity string `json:"last_activity"`
}

// RelayProcessHealth represents the health status of a relay process.
type RelayProcessHealth struct {
	ConfigID                string                       `json:"config_id"`
	ProfileID               string                       `json:"profile_id"`
	ProfileName             string                       `json:"profile_name"`
	ChannelName             string                       `json:"channel_name,omitempty"`
	SourceURL               string                       `json:"source_url"`
	Status                  string                       `json:"status"`
	PID                     string                       `json:"pid,omitempty"`
	UptimeSeconds           string                       `json:"uptime_seconds"`
	MemoryUsageMB           string                       `json:"memory_usage_mb"`
	CPUUsagePercent         string                       `json:"cpu_usage_percent"`
	BytesReceivedUpstream   string                       `json:"bytes_received_upstream"`
	BytesDeliveredDownstream string                      `json:"bytes_delivered_downstream"`
	ConnectedClients        []RelayConnectedClientHealth `json:"connected_clients"`
	LastHeartbeat           string                       `json:"last_heartbeat"`
	Error                   string                       `json:"error,omitempty"`
}

// GetRelayHealthOutput is the output for getting relay health.
type GetRelayHealthOutput struct {
	Body struct {
		TotalProcesses     string               `json:"total_processes"`
		HealthyProcesses   string               `json:"healthy_processes"`
		UnhealthyProcesses string               `json:"unhealthy_processes"`
		LastCheck          string               `json:"last_check"`
		Processes          []RelayProcessHealth `json:"processes"`
	}
}

// GetHealth returns the health status of relay processes.
func (h *RelayProfileHandler) GetHealth(ctx context.Context, input *GetRelayHealthInput) (*GetRelayHealthOutput, error) {
	stats := h.relayService.GetRelayStats()

	// Count healthy/unhealthy based on active sessions
	healthy := 0
	unhealthy := 0
	processes := make([]RelayProcessHealth, 0)
	var lastCheck string

	for _, s := range stats.Sessions {
		// Calculate uptime
		uptimeSeconds := time.Since(s.StartedAt).Seconds()

		// Build connected clients list
		clients := make([]RelayConnectedClientHealth, 0, len(s.Clients))
		for _, c := range s.Clients {
			clients = append(clients, RelayConnectedClientHealth{
				ID:           c.ID,
				IP:           c.RemoteAddr,
				UserAgent:    c.UserAgent,
				ConnectedAt:  c.ConnectedAt.Format(time.RFC3339),
				BytesServed:  fmt.Sprintf("%d", c.BytesRead),
				LastActivity: c.LastRead.Format(time.RFC3339),
			})
		}

		// Get CPU and memory from FFmpeg stats if available
		var cpuPercent, memoryMB float64
		var pid int
		if s.FFmpegStats != nil {
			cpuPercent = s.FFmpegStats.CPUPercent
			memoryMB = s.FFmpegStats.MemoryRSSMB
			pid = s.FFmpegStats.PID
		}

		proc := RelayProcessHealth{
			ConfigID:                 s.ID,
			ProfileID:                s.ChannelID,
			ProfileName:              s.ProfileName,
			ChannelName:              s.ChannelName,
			SourceURL:                s.StreamURL,
			UptimeSeconds:            fmt.Sprintf("%.0f", uptimeSeconds),
			MemoryUsageMB:            fmt.Sprintf("%.2f", memoryMB),
			CPUUsagePercent:          fmt.Sprintf("%.2f", cpuPercent),
			BytesReceivedUpstream:    fmt.Sprintf("%d", s.BytesFromUpstream),
			BytesDeliveredDownstream: fmt.Sprintf("%d", s.BytesWritten),
			ConnectedClients:         clients,
			LastHeartbeat:            s.LastActivity.Format(time.RFC3339),
		}

		if pid > 0 {
			proc.PID = fmt.Sprintf("%d", pid)
		}

		if s.Closed || s.Error != "" {
			proc.Status = "unhealthy"
			proc.Error = s.Error
			unhealthy++
		} else {
			proc.Status = "healthy"
			healthy++
		}
		processes = append(processes, proc)

		// Track most recent activity
		if lastCheck == "" || s.LastActivity.Format(time.RFC3339) > lastCheck {
			lastCheck = s.LastActivity.Format(time.RFC3339)
		}
	}

	// Default to current time if no sessions
	if lastCheck == "" {
		lastCheck = time.Now().UTC().Format(time.RFC3339)
	}

	return &GetRelayHealthOutput{
		Body: struct {
			TotalProcesses     string               `json:"total_processes"`
			HealthyProcesses   string               `json:"healthy_processes"`
			UnhealthyProcesses string               `json:"unhealthy_processes"`
			LastCheck          string               `json:"last_check"`
			Processes          []RelayProcessHealth `json:"processes"`
		}{
			TotalProcesses:     fmt.Sprintf("%d", healthy+unhealthy),
			HealthyProcesses:   fmt.Sprintf("%d", healthy),
			UnhealthyProcesses: fmt.Sprintf("%d", unhealthy),
			LastCheck:          lastCheck,
			Processes:          processes,
		},
	}, nil
}

// ValidateFlagsInput is the input for validating custom FFmpeg flags.
type ValidateFlagsInput struct {
	Body struct {
		InputOptions  string `json:"input_options,omitempty" doc:"Custom FFmpeg input options to validate"`
		OutputOptions string `json:"output_options,omitempty" doc:"Custom FFmpeg output options to validate"`
		FilterComplex string `json:"filter_complex,omitempty" doc:"Custom filter complex string to validate"`
	}
}

// ValidateFlagsOutput is the output for validating custom FFmpeg flags.
type ValidateFlagsOutput struct {
	Body struct {
		Valid       bool     `json:"valid"`
		Flags       []string `json:"flags,omitempty"`
		Warnings    []string `json:"warnings,omitempty"`
		Errors      []string `json:"errors,omitempty"`
		Suggestions []string `json:"suggestions,omitempty"`
	}
}

// ValidateFlags validates custom FFmpeg flags without creating a profile.
func (h *RelayProfileHandler) ValidateFlags(ctx context.Context, input *ValidateFlagsInput) (*ValidateFlagsOutput, error) {
	result := ffmpeg.ValidateCustomFlags(input.Body.InputOptions, input.Body.OutputOptions, input.Body.FilterComplex)

	return &ValidateFlagsOutput{
		Body: struct {
			Valid       bool     `json:"valid"`
			Flags       []string `json:"flags,omitempty"`
			Warnings    []string `json:"warnings,omitempty"`
			Errors      []string `json:"errors,omitempty"`
			Suggestions []string `json:"suggestions,omitempty"`
		}{
			Valid:       result.Valid,
			Flags:       result.Flags,
			Warnings:    result.Warnings,
			Errors:      result.Errors,
			Suggestions: result.Suggestions,
		},
	}, nil
}

// CloneRelayProfileInput is the input for cloning a relay profile.
type CloneRelayProfileInput struct {
	ID   string `path:"id" doc:"Relay profile ID to clone"`
	Body struct {
		Name        string `json:"name" required:"true" doc:"Name for the cloned profile"`
		Description string `json:"description,omitempty" doc:"Description for the cloned profile"`
	}
}

// CloneRelayProfileOutput is the output for cloning a relay profile.
type CloneRelayProfileOutput struct {
	Body RelayProfileResponse
}

// Clone creates a copy of an existing relay profile.
func (h *RelayProfileHandler) Clone(ctx context.Context, input *CloneRelayProfileInput) (*CloneRelayProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	// Get the source profile
	source, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrRelayProfileNotFound) {
			return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %s not found", input.ID))
		}
		return nil, huma.Error500InternalServerError("failed to get relay profile", err)
	}

	// Create a clone with the new name
	clone := source.Clone()
	clone.Name = input.Body.Name
	if input.Body.Description != "" {
		clone.Description = input.Body.Description
	} else {
		clone.Description = fmt.Sprintf("Clone of %s", source.Name)
	}

	if err := h.relayService.CreateProfile(ctx, clone); err != nil {
		return nil, huma.Error500InternalServerError("failed to create cloned profile", err)
	}

	return &CloneRelayProfileOutput{
		Body: RelayProfileFromModel(clone),
	}, nil
}

// HardwareCapabilityResponse represents a hardware acceleration capability.
type HardwareCapabilityResponse struct {
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	Available         bool     `json:"available"`
	DeviceName        string   `json:"device_name,omitempty"`
	DevicePath        string   `json:"device_path,omitempty"`
	GpuIndex          int      `json:"gpu_index,omitempty"`
	SupportedEncoders []string `json:"supported_encoders,omitempty"`
	SupportedDecoders []string `json:"supported_decoders,omitempty"`
	DetectedAt        string   `json:"detected_at"`
}

// GetHardwareCapabilitiesInput is the input for getting hardware capabilities.
type GetHardwareCapabilitiesInput struct{}

// GetHardwareCapabilitiesOutput is the output for getting hardware capabilities.
type GetHardwareCapabilitiesOutput struct {
	Body struct {
		Capabilities []HardwareCapabilityResponse `json:"capabilities"`
		DetectedAt   string                       `json:"detected_at"`
		Recommended  *HardwareCapabilityResponse  `json:"recommended,omitempty"`
	}
}

// GetHardwareCapabilities returns detected hardware acceleration capabilities.
func (h *RelayProfileHandler) GetHardwareCapabilities(ctx context.Context, input *GetHardwareCapabilitiesInput) (*GetHardwareCapabilitiesOutput, error) {
	caps, err := h.relayService.GetHardwareCapabilities(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get hardware capabilities", err)
	}

	resp := &GetHardwareCapabilitiesOutput{}
	resp.Body.Capabilities = make([]HardwareCapabilityResponse, 0, len(caps.Capabilities))
	resp.Body.DetectedAt = caps.DetectedAt.Format(time.RFC3339)

	for _, cap := range caps.Capabilities {
		resp.Body.Capabilities = append(resp.Body.Capabilities, HardwareCapabilityResponse{
			Type:              string(cap.Type),
			Name:              cap.Name,
			Available:         cap.Available,
			DeviceName:        cap.DeviceName,
			DevicePath:        cap.DevicePath,
			GpuIndex:          cap.GpuIndex,
			SupportedEncoders: cap.SupportedEncoders,
			SupportedDecoders: cap.SupportedDecoders,
			DetectedAt:        cap.DetectedAt.Format(time.RFC3339),
		})
	}

	if caps.Recommended != nil {
		resp.Body.Recommended = &HardwareCapabilityResponse{
			Type:              string(caps.Recommended.Type),
			Name:              caps.Recommended.Name,
			Available:         caps.Recommended.Available,
			DeviceName:        caps.Recommended.DeviceName,
			DevicePath:        caps.Recommended.DevicePath,
			GpuIndex:          caps.Recommended.GpuIndex,
			SupportedEncoders: caps.Recommended.SupportedEncoders,
			SupportedDecoders: caps.Recommended.SupportedDecoders,
			DetectedAt:        caps.Recommended.DetectedAt.Format(time.RFC3339),
		}
	}

	return resp, nil
}

// RefreshHardwareCapabilitiesInput is the input for refreshing hardware capabilities.
type RefreshHardwareCapabilitiesInput struct{}

// RefreshHardwareCapabilitiesOutput is the output for refreshing hardware capabilities.
type RefreshHardwareCapabilitiesOutput struct {
	Body struct {
		Capabilities []HardwareCapabilityResponse `json:"capabilities"`
		DetectedAt   string                       `json:"detected_at"`
		Recommended  *HardwareCapabilityResponse  `json:"recommended,omitempty"`
	}
}

// RefreshHardwareCapabilities re-detects hardware acceleration capabilities.
func (h *RelayProfileHandler) RefreshHardwareCapabilities(ctx context.Context, input *RefreshHardwareCapabilitiesInput) (*RefreshHardwareCapabilitiesOutput, error) {
	caps, err := h.relayService.RefreshHardwareCapabilities(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to refresh hardware capabilities", err)
	}

	resp := &RefreshHardwareCapabilitiesOutput{}
	resp.Body.Capabilities = make([]HardwareCapabilityResponse, 0, len(caps.Capabilities))
	resp.Body.DetectedAt = caps.DetectedAt.Format(time.RFC3339)

	for _, cap := range caps.Capabilities {
		resp.Body.Capabilities = append(resp.Body.Capabilities, HardwareCapabilityResponse{
			Type:              string(cap.Type),
			Name:              cap.Name,
			Available:         cap.Available,
			DeviceName:        cap.DeviceName,
			DevicePath:        cap.DevicePath,
			GpuIndex:          cap.GpuIndex,
			SupportedEncoders: cap.SupportedEncoders,
			SupportedDecoders: cap.SupportedDecoders,
			DetectedAt:        cap.DetectedAt.Format(time.RFC3339),
		})
	}

	if caps.Recommended != nil {
		resp.Body.Recommended = &HardwareCapabilityResponse{
			Type:              string(caps.Recommended.Type),
			Name:              caps.Recommended.Name,
			Available:         caps.Recommended.Available,
			DeviceName:        caps.Recommended.DeviceName,
			DevicePath:        caps.Recommended.DevicePath,
			GpuIndex:          caps.Recommended.GpuIndex,
			SupportedEncoders: caps.Recommended.SupportedEncoders,
			SupportedDecoders: caps.Recommended.SupportedDecoders,
			DetectedAt:        caps.Recommended.DetectedAt.Format(time.RFC3339),
		}
	}

	return resp, nil
}

// TestProfileInput is the input for testing a relay profile
type TestProfileInput struct {
	ID   string `path:"id" doc:"Profile ID"`
	Body struct {
		TestStreamURL string `json:"test_stream_url" doc:"URL of a stream to test the profile against" required:"true"`
		TimeoutSec    int    `json:"timeout_sec,omitempty" doc:"Test timeout in seconds (5-60, default 30)"`
	}
}

// TestProfileOutput is the output for testing a relay profile
type TestProfileOutput struct {
	Body TestProfileResultResponse
}

// TestProfileResultResponse represents the test result in API responses
type TestProfileResultResponse struct {
	Success         bool               `json:"success" doc:"Whether the test was successful"`
	DurationMs      int64              `json:"duration_ms" doc:"Test duration in milliseconds"`
	FramesProcessed int                `json:"frames_processed" doc:"Number of frames processed"`
	FPS             float64            `json:"fps" doc:"Average frames per second"`
	VideoCodecIn    string             `json:"video_codec_in,omitempty" doc:"Detected input video codec"`
	VideoCodecOut   string             `json:"video_codec_out,omitempty" doc:"Output video codec used"`
	AudioCodecIn    string             `json:"audio_codec_in,omitempty" doc:"Detected input audio codec"`
	AudioCodecOut   string             `json:"audio_codec_out,omitempty" doc:"Output audio codec used"`
	Resolution      string             `json:"resolution,omitempty" doc:"Video resolution"`
	HWAccelActive   bool               `json:"hw_accel_active" doc:"Whether hardware acceleration was active"`
	HWAccelMethod   string             `json:"hw_accel_method,omitempty" doc:"Hardware acceleration method used"`
	BitrateKbps     int                `json:"bitrate_kbps,omitempty" doc:"Output bitrate in kbps"`
	Errors          []string           `json:"errors,omitempty" doc:"List of errors encountered"`
	Warnings        []string           `json:"warnings,omitempty" doc:"List of warnings"`
	Suggestions     []string           `json:"suggestions,omitempty" doc:"Suggestions for improving the configuration"`
	FFmpegCommand   string             `json:"ffmpeg_command,omitempty" doc:"The FFmpeg command that was executed"`
	ExitCode        int                `json:"exit_code" doc:"FFmpeg exit code"`
	StreamInfo      *TestStreamInfoDTO `json:"stream_info,omitempty" doc:"Information about the test stream"`
}

// TestStreamInfoDTO is the DTO for test stream info
type TestStreamInfoDTO struct {
	InputURL  string `json:"input_url"`
	OutputURL string `json:"output_url"`
}

// TestProfile tests a relay profile against a sample stream
func (h *RelayProfileHandler) TestProfile(ctx context.Context, input *TestProfileInput) (*TestProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to get profile", err)
	}

	if input.Body.TestStreamURL == "" {
		return nil, huma.Error400BadRequest("test_stream_url is required")
	}

	// Validate timeout
	timeoutSec := input.Body.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 30
	} else if timeoutSec < 5 {
		timeoutSec = 5
	} else if timeoutSec > 60 {
		timeoutSec = 60
	}

	// Create profile tester
	tester := relay.NewProfileTester("", "") // Uses default ffmpeg/ffprobe paths
	tester.SetTimeout(time.Duration(timeoutSec) * time.Second)

	// Run the test
	result := tester.TestProfile(ctx, profile, input.Body.TestStreamURL)

	// Convert to response
	resp := &TestProfileOutput{
		Body: TestProfileResultResponse{
			Success:         result.Success,
			DurationMs:      result.DurationMs,
			FramesProcessed: result.FramesProcessed,
			FPS:             result.FPS,
			VideoCodecIn:    result.VideoCodecIn,
			VideoCodecOut:   result.VideoCodecOut,
			AudioCodecIn:    result.AudioCodecIn,
			AudioCodecOut:   result.AudioCodecOut,
			Resolution:      result.Resolution,
			HWAccelActive:   result.HWAccelActive,
			HWAccelMethod:   result.HWAccelMethod,
			BitrateKbps:     result.BitrateKbps,
			Errors:          result.Errors,
			Warnings:        result.Warnings,
			Suggestions:     result.Suggestions,
			FFmpegCommand:   result.FFmpegCommand,
			ExitCode:        result.ExitCode,
		},
	}

	if result.StreamInfo != nil {
		resp.Body.StreamInfo = &TestStreamInfoDTO{
			InputURL:  result.StreamInfo.InputURL,
			OutputURL: result.StreamInfo.OutputURL,
		}
	}

	return resp, nil
}

// PreviewCommandInput is the input for previewing an FFmpeg command
type PreviewCommandInput struct {
	ID   string `path:"id" doc:"Profile ID"`
	Body struct {
		InputURL  string `json:"input_url" doc:"Sample input URL to use in the command preview" required:"true"`
		OutputURL string `json:"output_url,omitempty" doc:"Sample output URL (optional, defaults to pipe:1)"`
	}
}

// PreviewCommandOutput is the output for command preview
type PreviewCommandOutput struct {
	Body CommandPreviewResponse
}

// CommandPreviewResponse represents the command preview in API responses
type CommandPreviewResponse struct {
	Command         string   `json:"command" doc:"Full command as a single string"`
	Args            []string `json:"args" doc:"Command arguments as an array"`
	Binary          string   `json:"binary" doc:"FFmpeg binary path"`
	InputURL        string   `json:"input_url" doc:"The input URL used"`
	OutputURL       string   `json:"output_url" doc:"The output URL used"`
	VideoCodec      string   `json:"video_codec,omitempty" doc:"Video codec that will be used"`
	AudioCodec      string   `json:"audio_codec,omitempty" doc:"Audio codec that will be used"`
	HWAccel         string   `json:"hw_accel,omitempty" doc:"Hardware acceleration method"`
	BitstreamFilter string   `json:"bitstream_filter,omitempty" doc:"Video bitstream filter applied"`
	Notes           []string `json:"notes,omitempty" doc:"Notes about the command configuration"`
}

// PreviewCommand generates a preview of the FFmpeg command for a profile
func (h *RelayProfileHandler) PreviewCommand(ctx context.Context, input *PreviewCommandInput) (*PreviewCommandOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	profile, err := h.relayService.GetProfileByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to get profile", err)
	}

	if input.Body.InputURL == "" {
		return nil, huma.Error400BadRequest("input_url is required")
	}

	outputURL := input.Body.OutputURL
	if outputURL == "" {
		outputURL = "pipe:1"
	}

	// Generate the command preview
	preview := relay.GenerateCommandPreview(profile, input.Body.InputURL, outputURL)

	// Build response
	resp := &PreviewCommandOutput{
		Body: CommandPreviewResponse{
			Command:         preview.Command,
			Args:            preview.Args,
			Binary:          preview.Binary,
			InputURL:        preview.InputURL,
			OutputURL:       preview.OutputURL,
			VideoCodec:      preview.VideoCodec,
			AudioCodec:      preview.AudioCodec,
			HWAccel:         preview.HWAccel,
			BitstreamFilter: preview.BitstreamFilter,
			Notes:           preview.Notes,
		},
	}

	return resp, nil
}
