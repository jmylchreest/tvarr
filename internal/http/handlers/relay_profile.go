package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
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
}

// RelayProfileResponse represents a relay profile in API responses.
type RelayProfileResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	VideoCodec      string `json:"video_codec"`
	AudioCodec      string `json:"audio_codec"`
	VideoBitrate    int    `json:"video_bitrate,omitempty"`
	AudioBitrate    int    `json:"audio_bitrate,omitempty"`
	VideoMaxrate    int    `json:"video_maxrate,omitempty"`
	VideoPreset     string `json:"video_preset,omitempty"`
	VideoWidth      int    `json:"video_width,omitempty"`
	VideoHeight     int    `json:"video_height,omitempty"`
	AudioSampleRate int    `json:"audio_sample_rate,omitempty"`
	AudioChannels   int    `json:"audio_channels,omitempty"`
	HWAccel         string `json:"hw_accel"`
	OutputFormat    string `json:"output_format"`
	IsDefault       bool   `json:"is_default"`
	IsSystem        bool   `json:"is_system" doc:"Whether this is a system-provided profile (cannot be edited/deleted)"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// RelayProfileFromModel converts a model to a response.
func RelayProfileFromModel(p *models.RelayProfile) RelayProfileResponse {
	return RelayProfileResponse{
		ID:              p.ID.String(),
		Name:            p.Name,
		Description:     p.Description,
		VideoCodec:      string(p.VideoCodec),
		AudioCodec:      string(p.AudioCodec),
		VideoBitrate:    p.VideoBitrate,
		AudioBitrate:    p.AudioBitrate,
		VideoMaxrate:    p.VideoMaxrate,
		VideoPreset:     p.VideoPreset,
		VideoWidth:      p.VideoWidth,
		VideoHeight:     p.VideoHeight,
		AudioSampleRate: p.AudioSampleRate,
		AudioChannels:   p.AudioChannels,
		HWAccel:         string(p.HWAccel),
		OutputFormat:    string(p.OutputFormat),
		IsDefault:       p.IsDefault,
		IsSystem:        p.IsSystem,
		Enabled:         p.Enabled,
		CreatedAt:       p.CreatedAt.String(),
		UpdatedAt:       p.UpdatedAt.String(),
	}
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
		Name            string `json:"name" required:"true" doc:"Profile name"`
		Description     string `json:"description,omitempty" doc:"Profile description"`
		VideoCodec      string `json:"video_codec" doc:"Video codec (copy, h264, hevc, vp9, av1)"`
		AudioCodec      string `json:"audio_codec" doc:"Audio codec (copy, aac, mp3, opus, flac)"`
		VideoBitrate    int    `json:"video_bitrate,omitempty" doc:"Video bitrate in kbps"`
		AudioBitrate    int    `json:"audio_bitrate,omitempty" doc:"Audio bitrate in kbps"`
		VideoMaxrate    int    `json:"video_maxrate,omitempty" doc:"Video max bitrate in kbps"`
		VideoPreset     string `json:"video_preset,omitempty" doc:"Encoder preset"`
		VideoWidth      int    `json:"video_width,omitempty" doc:"Output video width"`
		VideoHeight     int    `json:"video_height,omitempty" doc:"Output video height"`
		AudioSampleRate int    `json:"audio_sample_rate,omitempty" doc:"Audio sample rate in Hz"`
		AudioChannels   int    `json:"audio_channels,omitempty" doc:"Audio channels (1=mono, 2=stereo)"`
		HWAccel         string `json:"hw_accel,omitempty" doc:"Hardware acceleration (none, nvenc, qsv, vaapi, amf, videotoolbox)"`
		OutputFormat    string `json:"output_format,omitempty" doc:"Output format (mpegts, hls, flv, mp4)"`
		IsDefault       bool   `json:"is_default,omitempty" doc:"Set as default profile"`
	}
}

// CreateRelayProfileOutput is the output for creating a relay profile.
type CreateRelayProfileOutput struct {
	Body RelayProfileResponse
}

// Create creates a new relay profile.
func (h *RelayProfileHandler) Create(ctx context.Context, input *CreateRelayProfileInput) (*CreateRelayProfileOutput, error) {
	profile := &models.RelayProfile{
		Name:            input.Body.Name,
		Description:     input.Body.Description,
		VideoCodec:      models.VideoCodec(input.Body.VideoCodec),
		AudioCodec:      models.AudioCodec(input.Body.AudioCodec),
		VideoBitrate:    input.Body.VideoBitrate,
		AudioBitrate:    input.Body.AudioBitrate,
		VideoMaxrate:    input.Body.VideoMaxrate,
		VideoPreset:     input.Body.VideoPreset,
		VideoWidth:      input.Body.VideoWidth,
		VideoHeight:     input.Body.VideoHeight,
		AudioSampleRate: input.Body.AudioSampleRate,
		AudioChannels:   input.Body.AudioChannels,
		HWAccel:         models.HWAccelType(input.Body.HWAccel),
		OutputFormat:    models.OutputFormat(input.Body.OutputFormat),
		IsDefault:       input.Body.IsDefault,
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
		Name            string `json:"name,omitempty" doc:"Profile name"`
		Description     string `json:"description,omitempty" doc:"Profile description"`
		VideoCodec      string `json:"video_codec,omitempty" doc:"Video codec"`
		AudioCodec      string `json:"audio_codec,omitempty" doc:"Audio codec"`
		VideoBitrate    int    `json:"video_bitrate,omitempty" doc:"Video bitrate in kbps"`
		AudioBitrate    int    `json:"audio_bitrate,omitempty" doc:"Audio bitrate in kbps"`
		VideoMaxrate    int    `json:"video_maxrate,omitempty" doc:"Video max bitrate in kbps"`
		VideoPreset     string `json:"video_preset,omitempty" doc:"Encoder preset"`
		VideoWidth      int    `json:"video_width,omitempty" doc:"Output video width"`
		VideoHeight     int    `json:"video_height,omitempty" doc:"Output video height"`
		AudioSampleRate int    `json:"audio_sample_rate,omitempty" doc:"Audio sample rate in Hz"`
		AudioChannels   int    `json:"audio_channels,omitempty" doc:"Audio channels"`
		HWAccel         string `json:"hw_accel,omitempty" doc:"Hardware acceleration"`
		OutputFormat    string `json:"output_format,omitempty" doc:"Output format"`
		Enabled         *bool  `json:"enabled,omitempty" doc:"Whether the profile is enabled"`
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
			input.Body.HWAccel != "" || input.Body.OutputFormat != "" {
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
		if input.Body.OutputFormat != "" {
			profile.OutputFormat = models.OutputFormat(input.Body.OutputFormat)
		}
		if input.Body.Enabled != nil {
			profile.Enabled = *input.Body.Enabled
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

// RelayProcessHealth represents the health status of a relay process.
type RelayProcessHealth struct {
	ConfigID   string `json:"config_id"`
	ProfileID  string `json:"profile_id"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	LastUpdate string `json:"last_update"`
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
		proc := RelayProcessHealth{
			ConfigID:   s.ID,
			ProfileID:  s.ChannelID,
			LastUpdate: s.LastActivity.Format("2006-01-02T15:04:05Z07:00"),
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
		if lastCheck == "" || s.LastActivity.Format("2006-01-02T15:04:05Z07:00") > lastCheck {
			lastCheck = s.LastActivity.Format("2006-01-02T15:04:05Z07:00")
		}
	}

	// Default to current time if no sessions
	if lastCheck == "" {
		lastCheck = time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
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
