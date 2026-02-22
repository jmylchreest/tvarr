package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// EncodingProfileHandler handles encoding profile API endpoints.
type EncodingProfileHandler struct {
	service           *service.EncodingProfileService
	proxyUsageChecker ProxyUsageChecker
}

// NewEncodingProfileHandler creates a new encoding profile handler.
func NewEncodingProfileHandler(svc *service.EncodingProfileService) *EncodingProfileHandler {
	return &EncodingProfileHandler{service: svc}
}

// WithProxyUsageChecker sets the proxy usage checker for delete validation.
func (h *EncodingProfileHandler) WithProxyUsageChecker(checker ProxyUsageChecker) *EncodingProfileHandler {
	h.proxyUsageChecker = checker
	return h
}

// Register registers the encoding profile routes with the API.
func (h *EncodingProfileHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listEncodingProfiles",
		Method:      "GET",
		Path:        "/api/v1/encoding-profiles",
		Summary:     "List encoding profiles",
		Description: "Returns all encoding profiles, with default and system profiles first",
		Tags:        []string{"Encoding Profiles"},
	}, h.List)

	huma.Register(api, huma.Operation{
		OperationID: "getEncodingProfile",
		Method:      "GET",
		Path:        "/api/v1/encoding-profiles/{id}",
		Summary:     "Get encoding profile",
		Description: "Returns an encoding profile by ID",
		Tags:        []string{"Encoding Profiles"},
	}, h.GetByID)

	huma.Register(api, huma.Operation{
		OperationID: "createEncodingProfile",
		Method:      "POST",
		Path:        "/api/v1/encoding-profiles",
		Summary:     "Create encoding profile",
		Description: "Creates a new encoding profile",
		Tags:        []string{"Encoding Profiles"},
	}, h.Create)

	huma.Register(api, huma.Operation{
		OperationID: "updateEncodingProfile",
		Method:      "PUT",
		Path:        "/api/v1/encoding-profiles/{id}",
		Summary:     "Update encoding profile",
		Description: "Updates an existing encoding profile. System profiles can only have enabled toggled.",
		Tags:        []string{"Encoding Profiles"},
	}, h.Update)

	huma.Register(api, huma.Operation{
		OperationID: "deleteEncodingProfile",
		Method:      "DELETE",
		Path:        "/api/v1/encoding-profiles/{id}",
		Summary:     "Delete encoding profile",
		Description: "Deletes an encoding profile. System profiles cannot be deleted.",
		Tags:        []string{"Encoding Profiles"},
	}, h.Delete)

	huma.Register(api, huma.Operation{
		OperationID: "setDefaultEncodingProfile",
		Method:      "POST",
		Path:        "/api/v1/encoding-profiles/{id}/set-default",
		Summary:     "Set default encoding profile",
		Description: "Sets an encoding profile as the default",
		Tags:        []string{"Encoding Profiles"},
	}, h.SetDefault)

	huma.Register(api, huma.Operation{
		OperationID: "toggleEncodingProfileEnabled",
		Method:      "POST",
		Path:        "/api/v1/encoding-profiles/{id}/toggle-enabled",
		Summary:     "Toggle encoding profile enabled",
		Description: "Toggles the enabled state of an encoding profile",
		Tags:        []string{"Encoding Profiles"},
	}, h.ToggleEnabled)

	huma.Register(api, huma.Operation{
		OperationID: "cloneEncodingProfile",
		Method:      "POST",
		Path:        "/api/v1/encoding-profiles/{id}/clone",
		Summary:     "Clone encoding profile",
		Description: "Creates a copy of an existing encoding profile",
		Tags:        []string{"Encoding Profiles"},
	}, h.Clone)

	huma.Register(api, huma.Operation{
		OperationID: "getEncodingProfileStats",
		Method:      "GET",
		Path:        "/api/v1/encoding-profiles/stats",
		Summary:     "Get encoding profile statistics",
		Description: "Returns statistics about encoding profiles",
		Tags:        []string{"Encoding Profiles"},
	}, h.GetStats)

	huma.Register(api, huma.Operation{
		OperationID: "getEncodingProfileOptions",
		Method:      "GET",
		Path:        "/api/v1/encoding-profiles/options",
		Summary:     "Get encoding profile options",
		Description: "Returns available options for encoding profile fields (quality presets, codecs, hw accel)",
		Tags:        []string{"Encoding Profiles"},
	}, h.GetOptions)

	huma.Register(api, huma.Operation{
		OperationID: "previewEncodingProfileCommand",
		Method:      "POST",
		Path:        "/api/v1/encoding-profiles/preview",
		Summary:     "Preview FFmpeg command",
		Description: "Returns the FFmpeg command that would be generated for the given profile configuration",
		Tags:        []string{"Encoding Profiles"},
	}, h.PreviewCommand)
}

// EncodingProfileResponse represents an encoding profile in API responses.
type EncodingProfileResponse struct {
	ID               string `json:"id" doc:"Profile ID (ULID)"`
	Name             string `json:"name" doc:"Profile name"`
	Description      string `json:"description,omitempty" doc:"Profile description"`
	TargetVideoCodec string `json:"target_video_codec" doc:"Target video codec (h264, h265, vp9, av1)"`
	TargetAudioCodec string `json:"target_audio_codec" doc:"Target audio codec (aac, opus, ac3, eac3, mp3)"`
	QualityPreset    string `json:"quality_preset" doc:"Quality preset (low, medium, high, ultra)"`
	HWAccel          string `json:"hw_accel" doc:"Hardware acceleration (auto, none, cuda, vaapi, qsv, videotoolbox)"`

	// Custom FFmpeg flags - when set, these replace auto-generated flags
	GlobalFlags string `json:"global_flags,omitempty" doc:"Custom global FFmpeg flags (replaces auto-generated)"`
	InputFlags  string `json:"input_flags,omitempty" doc:"Custom input FFmpeg flags (replaces auto-generated)"`
	OutputFlags string `json:"output_flags,omitempty" doc:"Custom output FFmpeg flags (replaces auto-generated)"`

	// Auto-generated default flags for placeholder text
	DefaultFlags DefaultFlagsResponse `json:"default_flags" doc:"Auto-generated FFmpeg flags (for placeholder text)"`

	IsDefault bool   `json:"is_default" doc:"Whether this is the default profile"`
	IsSystem  bool   `json:"is_system" doc:"Whether this is a system-provided profile"`
	Enabled   bool   `json:"enabled" doc:"Whether the profile is enabled"`
	CreatedAt string `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt string `json:"updated_at" doc:"Last update timestamp"`
}

// DefaultFlagsResponse represents auto-generated FFmpeg flags for a profile.
type DefaultFlagsResponse struct {
	GlobalFlags string `json:"global_flags" doc:"Auto-generated global flags"`
	InputFlags  string `json:"input_flags" doc:"Auto-generated input flags"`
	OutputFlags string `json:"output_flags" doc:"Auto-generated output flags"`
}

// EncodingProfileFromModel converts a models.EncodingProfile to EncodingProfileResponse.
func EncodingProfileFromModel(p *models.EncodingProfile) EncodingProfileResponse {
	// Generate default flags for placeholder text
	defaults := p.GenerateDefaultFlags()

	return EncodingProfileResponse{
		ID:               p.ID.String(),
		Name:             p.Name,
		Description:      p.Description,
		TargetVideoCodec: string(p.TargetVideoCodec),
		TargetAudioCodec: string(p.TargetAudioCodec),
		QualityPreset:    string(p.QualityPreset),
		HWAccel:          string(p.HWAccel),
		GlobalFlags:      p.GlobalFlags,
		InputFlags:       p.InputFlags,
		OutputFlags:      p.OutputFlags,
		DefaultFlags: DefaultFlagsResponse{
			GlobalFlags: defaults.GlobalFlags,
			InputFlags:  defaults.InputFlags,
			OutputFlags: defaults.OutputFlags,
		},
		IsDefault: p.IsDefault,
		IsSystem:  p.IsSystem,
		Enabled:   models.BoolVal(p.Enabled),
		CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListEncodingProfilesInput is the input for listing profiles.
type ListEncodingProfilesInput struct {
	Enabled string `query:"enabled" doc:"Filter by enabled status (true or false)" required:"false" enum:"true,false,"`
	System  string `query:"system" doc:"Filter by system status (true or false)" required:"false" enum:"true,false,"`
}

// ListEncodingProfilesOutput is the output for listing profiles.
type ListEncodingProfilesOutput struct {
	Body struct {
		Profiles []EncodingProfileResponse `json:"profiles"`
	}
}

// List returns all encoding profiles.
func (h *EncodingProfileHandler) List(ctx context.Context, input *ListEncodingProfilesInput) (*ListEncodingProfilesOutput, error) {
	var profiles []*models.EncodingProfile
	var err error

	if input.Enabled == "true" {
		profiles, err = h.service.GetEnabled(ctx)
	} else if input.System == "true" {
		profiles, err = h.service.GetSystem(ctx)
	} else {
		profiles, err = h.service.GetAll(ctx)
	}

	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list encoding profiles", err)
	}

	resp := &ListEncodingProfilesOutput{}
	resp.Body.Profiles = make([]EncodingProfileResponse, len(profiles))
	for i, p := range profiles {
		resp.Body.Profiles[i] = EncodingProfileFromModel(p)
	}

	return resp, nil
}

// GetEncodingProfileInput is the input for getting a profile.
type GetEncodingProfileInput struct {
	ID string `path:"id" doc:"Profile ID (ULID)" format:"ulid"`
}

// GetEncodingProfileOutput is the output for getting a profile.
type GetEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// GetByID returns an encoding profile by ID.
func (h *EncodingProfileHandler) GetByID(ctx context.Context, input *GetEncodingProfileInput) (*GetEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	profile, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to get encoding profile", err)
	}

	return &GetEncodingProfileOutput{Body: EncodingProfileFromModel(profile)}, nil
}

// CreateEncodingProfileInput is the input for creating a profile.
type CreateEncodingProfileInput struct {
	Body struct {
		Name             string `json:"name" doc:"Profile name" minLength:"1" maxLength:"100"`
		Description      string `json:"description,omitempty" doc:"Profile description" maxLength:"500"`
		TargetVideoCodec string `json:"target_video_codec" doc:"Target video codec" enum:"h264,h265,vp9,av1"`
		TargetAudioCodec string `json:"target_audio_codec" doc:"Target audio codec" enum:"aac,opus,ac3,eac3,mp3"`
		QualityPreset    string `json:"quality_preset" doc:"Quality preset" enum:"low,medium,high,ultra" default:"medium"`
		HWAccel          string `json:"hw_accel,omitempty" doc:"Hardware acceleration" enum:"auto,none,cuda,vaapi,qsv,videotoolbox" default:"auto"`
		IsDefault        bool   `json:"is_default,omitempty" doc:"Set as default encoding profile for proxies" default:"false"`

		// Custom FFmpeg flags - when provided, these REPLACE auto-generated flags
		GlobalFlags string `json:"global_flags,omitempty" doc:"Custom global FFmpeg flags (replaces auto-generated)" maxLength:"500"`
		InputFlags  string `json:"input_flags,omitempty" doc:"Custom input FFmpeg flags (replaces auto-generated)" maxLength:"500"`
		OutputFlags string `json:"output_flags,omitempty" doc:"Custom output FFmpeg flags (replaces auto-generated)" maxLength:"1000"`
	}
}

// CreateEncodingProfileOutput is the output for creating a profile.
type CreateEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// Create creates a new encoding profile.
func (h *EncodingProfileHandler) Create(ctx context.Context, input *CreateEncodingProfileInput) (*CreateEncodingProfileOutput, error) {
	profile := &models.EncodingProfile{
		Name:             input.Body.Name,
		Description:      input.Body.Description,
		TargetVideoCodec: models.VideoCodec(input.Body.TargetVideoCodec),
		TargetAudioCodec: models.AudioCodec(input.Body.TargetAudioCodec),
		QualityPreset:    models.QualityPreset(input.Body.QualityPreset),
		HWAccel:          models.HWAccelType(input.Body.HWAccel),
		GlobalFlags:      input.Body.GlobalFlags,
		InputFlags:       input.Body.InputFlags,
		OutputFlags:      input.Body.OutputFlags,
		IsDefault:        input.Body.IsDefault,
		Enabled:          new(true),
	}

	if err := h.service.Create(ctx, profile); err != nil {
		return nil, huma.Error500InternalServerError("failed to create encoding profile", err)
	}

	return &CreateEncodingProfileOutput{Body: EncodingProfileFromModel(profile)}, nil
}

// UpdateEncodingProfileInput is the input for updating a profile.
type UpdateEncodingProfileInput struct {
	ID   string `path:"id" doc:"Profile ID (ULID)" format:"ulid"`
	Body struct {
		Name             string `json:"name,omitempty" doc:"Profile name" maxLength:"100"`
		Description      string `json:"description,omitempty" doc:"Profile description" maxLength:"500"`
		TargetVideoCodec string `json:"target_video_codec,omitempty" doc:"Target video codec" enum:"h264,h265,vp9,av1,"`
		TargetAudioCodec string `json:"target_audio_codec,omitempty" doc:"Target audio codec" enum:"aac,opus,ac3,eac3,mp3,"`
		QualityPreset    string `json:"quality_preset,omitempty" doc:"Quality preset" enum:"low,medium,high,ultra,"`
		HWAccel          string `json:"hw_accel,omitempty" doc:"Hardware acceleration" enum:"auto,none,cuda,vaapi,qsv,videotoolbox,"`
		Enabled          *bool  `json:"enabled,omitempty" doc:"Whether the profile is enabled"`

		// Custom FFmpeg flags - when provided, these REPLACE auto-generated flags
		// Use pointer to distinguish between "not provided" and "set to empty string" (to clear)
		GlobalFlags *string `json:"global_flags,omitempty" doc:"Custom global FFmpeg flags (replaces auto-generated)" maxLength:"500"`
		InputFlags  *string `json:"input_flags,omitempty" doc:"Custom input FFmpeg flags (replaces auto-generated)" maxLength:"500"`
		OutputFlags *string `json:"output_flags,omitempty" doc:"Custom output FFmpeg flags (replaces auto-generated)" maxLength:"1000"`
	}
}

// UpdateEncodingProfileOutput is the output for updating a profile.
type UpdateEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// Update updates an existing encoding profile.
func (h *EncodingProfileHandler) Update(ctx context.Context, input *UpdateEncodingProfileInput) (*UpdateEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	// Get existing profile
	existing, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to get encoding profile", err)
	}

	// Apply updates
	if input.Body.Name != "" {
		existing.Name = input.Body.Name
	}
	if input.Body.Description != "" {
		existing.Description = input.Body.Description
	}
	if input.Body.TargetVideoCodec != "" {
		existing.TargetVideoCodec = models.VideoCodec(input.Body.TargetVideoCodec)
	}
	if input.Body.TargetAudioCodec != "" {
		existing.TargetAudioCodec = models.AudioCodec(input.Body.TargetAudioCodec)
	}
	if input.Body.QualityPreset != "" {
		existing.QualityPreset = models.QualityPreset(input.Body.QualityPreset)
	}
	if input.Body.HWAccel != "" {
		existing.HWAccel = models.HWAccelType(input.Body.HWAccel)
	}
	if input.Body.Enabled != nil {
		existing.Enabled = input.Body.Enabled
	}
	// Custom flag fields - use pointers to allow clearing by setting to empty string
	if input.Body.GlobalFlags != nil {
		existing.GlobalFlags = *input.Body.GlobalFlags
	}
	if input.Body.InputFlags != nil {
		existing.InputFlags = *input.Body.InputFlags
	}
	if input.Body.OutputFlags != nil {
		existing.OutputFlags = *input.Body.OutputFlags
	}

	if err := h.service.Update(ctx, existing); err != nil {
		if errors.Is(err, service.ErrEncodingProfileCannotEditSystem) {
			return nil, huma.Error403Forbidden("cannot edit system encoding profile (only enabled toggle allowed)")
		}
		return nil, huma.Error500InternalServerError("failed to update encoding profile", err)
	}

	// Refetch to get updated timestamps
	updated, err := h.service.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get updated encoding profile", err)
	}

	return &UpdateEncodingProfileOutput{Body: EncodingProfileFromModel(updated)}, nil
}

// DeleteEncodingProfileInput is the input for deleting a profile.
type DeleteEncodingProfileInput struct {
	ID string `path:"id" doc:"Profile ID (ULID)" format:"ulid"`
}

// DeleteEncodingProfileOutput is the output for deleting a profile.
type DeleteEncodingProfileOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// Delete deletes an encoding profile.
func (h *EncodingProfileHandler) Delete(ctx context.Context, input *DeleteEncodingProfileInput) (*DeleteEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	// Check if encoding profile is in use by any proxies
	if h.proxyUsageChecker != nil {
		proxyNames, err := h.proxyUsageChecker.GetProxyNamesByEncodingProfileID(ctx, id)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to check proxy usage", err)
		}
		if len(proxyNames) > 0 {
			return nil, huma.Error409Conflict(fmt.Sprintf(
				"cannot delete encoding profile: in use by %d proxy(s): %s. Remove it from these proxies first.",
				len(proxyNames), strings.Join(proxyNames, ", ")))
		}
	}

	if err := h.service.Delete(ctx, id); err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		if errors.Is(err, service.ErrEncodingProfileCannotDeleteSystem) {
			return nil, huma.Error403Forbidden("cannot delete system encoding profile")
		}
		return nil, huma.Error500InternalServerError("failed to delete encoding profile", err)
	}

	resp := &DeleteEncodingProfileOutput{}
	resp.Body.Success = true
	return resp, nil
}

// SetDefaultEncodingProfileInput is the input for setting default profile.
type SetDefaultEncodingProfileInput struct {
	ID string `path:"id" doc:"Profile ID (ULID)" format:"ulid"`
}

// SetDefaultEncodingProfileOutput is the output for setting default profile.
type SetDefaultEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// SetDefault sets an encoding profile as the default.
func (h *EncodingProfileHandler) SetDefault(ctx context.Context, input *SetDefaultEncodingProfileInput) (*SetDefaultEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	if err := h.service.SetDefault(ctx, id); err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to set default encoding profile", err)
	}

	profile, err := h.service.GetByID(ctx, id)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get encoding profile", err)
	}

	return &SetDefaultEncodingProfileOutput{Body: EncodingProfileFromModel(profile)}, nil
}

// ToggleEnabledEncodingProfileInput is the input for toggling enabled.
type ToggleEnabledEncodingProfileInput struct {
	ID string `path:"id" doc:"Profile ID (ULID)" format:"ulid"`
}

// ToggleEnabledEncodingProfileOutput is the output for toggling enabled.
type ToggleEnabledEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// ToggleEnabled toggles the enabled state of an encoding profile.
func (h *EncodingProfileHandler) ToggleEnabled(ctx context.Context, input *ToggleEnabledEncodingProfileInput) (*ToggleEnabledEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	profile, err := h.service.ToggleEnabled(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to toggle encoding profile", err)
	}

	return &ToggleEnabledEncodingProfileOutput{Body: EncodingProfileFromModel(profile)}, nil
}

// CloneEncodingProfileInput is the input for cloning a profile.
type CloneEncodingProfileInput struct {
	ID   string `path:"id" doc:"Profile ID (ULID) to clone" format:"ulid"`
	Body struct {
		Name string `json:"name" doc:"Name for the cloned profile" minLength:"1" maxLength:"100"`
	}
}

// CloneEncodingProfileOutput is the output for cloning a profile.
type CloneEncodingProfileOutput struct {
	Body EncodingProfileResponse
}

// Clone creates a copy of an existing encoding profile.
func (h *EncodingProfileHandler) Clone(ctx context.Context, input *CloneEncodingProfileInput) (*CloneEncodingProfileOutput, error) {
	id, err := models.ParseULID(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid profile ID", err)
	}

	clone, err := h.service.Clone(ctx, id, input.Body.Name)
	if err != nil {
		if errors.Is(err, models.ErrEncodingProfileNotFound) {
			return nil, huma.Error404NotFound("encoding profile not found")
		}
		return nil, huma.Error500InternalServerError("failed to clone encoding profile", err)
	}

	return &CloneEncodingProfileOutput{Body: EncodingProfileFromModel(clone)}, nil
}

// EncodingProfileStatsOutput is the output for getting stats.
type EncodingProfileStatsOutput struct {
	Body struct {
		TotalProfiles   int64 `json:"total_profiles" doc:"Total number of encoding profiles"`
		EnabledProfiles int64 `json:"enabled_profiles" doc:"Number of enabled encoding profiles"`
		SystemProfiles  int64 `json:"system_profiles" doc:"Number of system encoding profiles"`
	}
}

// GetStats returns statistics about encoding profiles.
func (h *EncodingProfileHandler) GetStats(ctx context.Context, input *struct{}) (*EncodingProfileStatsOutput, error) {
	total, err := h.service.Count(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to count encoding profiles", err)
	}

	enabled, err := h.service.CountEnabled(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to count enabled encoding profiles", err)
	}

	system, err := h.service.GetSystem(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get system encoding profiles", err)
	}

	resp := &EncodingProfileStatsOutput{}
	resp.Body.TotalProfiles = total
	resp.Body.EnabledProfiles = enabled
	resp.Body.SystemProfiles = int64(len(system))

	return resp, nil
}

// EncodingProfileOptionsOutput is the output for getting options.
type EncodingProfileOptionsOutput struct {
	Body struct {
		VideoCodecs    []CodecOption       `json:"video_codecs" doc:"Available video codecs"`
		AudioCodecs    []CodecOption       `json:"audio_codecs" doc:"Available audio codecs"`
		QualityPresets []QualityPresetInfo `json:"quality_presets" doc:"Available quality presets"`
		HWAccelOptions []HWAccelOption     `json:"hw_accel_options" doc:"Available hardware acceleration options"`
	}
}

// CodecOption represents a codec option.
type CodecOption struct {
	Value       string `json:"value" doc:"Codec value"`
	Label       string `json:"label" doc:"Human-readable label"`
	Description string `json:"description,omitempty" doc:"Description"`
}

// QualityPresetInfo represents a quality preset with its parameters.
type QualityPresetInfo struct {
	Value        string `json:"value" doc:"Preset value"`
	Label        string `json:"label" doc:"Human-readable label"`
	Description  string `json:"description" doc:"Description"`
	CRF          int    `json:"crf" doc:"CRF value (lower = better quality)"`
	MaxBitrate   string `json:"max_bitrate" doc:"Maximum bitrate"`
	AudioBitrate string `json:"audio_bitrate" doc:"Audio bitrate"`
}

// HWAccelOption represents a hardware acceleration option.
type HWAccelOption struct {
	Value       string `json:"value" doc:"HW accel value"`
	Label       string `json:"label" doc:"Human-readable label"`
	Description string `json:"description" doc:"Description"`
}

// GetOptions returns available options for encoding profile fields.
func (h *EncodingProfileHandler) GetOptions(ctx context.Context, input *struct{}) (*EncodingProfileOptionsOutput, error) {
	resp := &EncodingProfileOptionsOutput{}

	resp.Body.VideoCodecs = []CodecOption{
		{Value: "h264", Label: "H.264 / AVC", Description: "Maximum compatibility - works everywhere"},
		{Value: "h265", Label: "H.265 / HEVC", Description: "40-50% smaller than H.264, modern device support"},
		{Value: "vp9", Label: "VP9", Description: "Open/royalty-free, good for web browsers"},
		{Value: "av1", Label: "AV1", Description: "30% smaller than H.265, requires modern hardware"},
	}

	resp.Body.AudioCodecs = []CodecOption{
		{Value: "aac", Label: "AAC", Description: "Maximum compatibility - works everywhere"},
		{Value: "opus", Label: "Opus", Description: "Best quality at low bitrates, great for web"},
		{Value: "ac3", Label: "AC3 (Dolby Digital)", Description: "5.1 surround sound for home theater"},
		{Value: "eac3", Label: "E-AC3 (Dolby Digital Plus)", Description: "Enhanced 5.1/7.1 surround sound"},
		{Value: "mp3", Label: "MP3", Description: "Legacy format for maximum compatibility"},
	}

	resp.Body.QualityPresets = []QualityPresetInfo{
		{Value: "low", Label: "Low", Description: "Mobile/cellular streaming", CRF: 28, MaxBitrate: "2M", AudioBitrate: "128k"},
		{Value: "medium", Label: "Medium", Description: "Balanced quality and bandwidth", CRF: 23, MaxBitrate: "5M", AudioBitrate: "192k"},
		{Value: "high", Label: "High", Description: "High quality home viewing", CRF: 20, MaxBitrate: "10M", AudioBitrate: "256k"},
		{Value: "ultra", Label: "Ultra", Description: "Maximum quality, no bandwidth limit", CRF: 16, MaxBitrate: "unlimited", AudioBitrate: "320k"},
	}

	resp.Body.HWAccelOptions = []HWAccelOption{
		{Value: "auto", Label: "Auto", Description: "Automatically detect and use available hardware"},
		{Value: "none", Label: "None (Software)", Description: "Use CPU-based encoding only"},
		{Value: "cuda", Label: "NVIDIA CUDA", Description: "NVIDIA GPU acceleration (NVENC)"},
		{Value: "vaapi", Label: "VA-API", Description: "Linux video acceleration (Intel/AMD)"},
		{Value: "qsv", Label: "Intel QSV", Description: "Intel Quick Sync Video"},
		{Value: "videotoolbox", Label: "VideoToolbox", Description: "macOS hardware acceleration"},
	}

	return resp, nil
}

// PreviewEncodingProfileInput is the input for previewing FFmpeg command.
type PreviewEncodingProfileInput struct {
	Body struct {
		TargetVideoCodec string `json:"target_video_codec" doc:"Target video codec" enum:"h264,h265,vp9,av1"`
		TargetAudioCodec string `json:"target_audio_codec" doc:"Target audio codec" enum:"aac,opus,ac3,eac3,mp3"`
		QualityPreset    string `json:"quality_preset" doc:"Quality preset" enum:"low,medium,high,ultra" default:"medium"`
		HWAccel          string `json:"hw_accel,omitempty" doc:"Hardware acceleration" enum:"auto,none,cuda,vaapi,qsv,videotoolbox" default:"auto"`

		// Custom FFmpeg flags - when provided, these REPLACE auto-generated flags
		GlobalFlags string `json:"global_flags,omitempty" doc:"Custom global FFmpeg flags"`
		InputFlags  string `json:"input_flags,omitempty" doc:"Custom input FFmpeg flags"`
		OutputFlags string `json:"output_flags,omitempty" doc:"Custom output FFmpeg flags"`
	}
}

// PreviewEncodingProfileOutput is the output for previewing FFmpeg command.
type PreviewEncodingProfileOutput struct {
	Body struct {
		Command      string `json:"command" doc:"Full FFmpeg command"`
		GlobalFlags  string `json:"global_flags" doc:"Global flags portion"`
		InputFlags   string `json:"input_flags" doc:"Input flags portion"`
		OutputFlags  string `json:"output_flags" doc:"Output flags portion"`
		UsingCustom  bool   `json:"using_custom" doc:"Whether custom flags are being used"`
		VideoEncoder string `json:"video_encoder" doc:"Video encoder being used"`
		AudioEncoder string `json:"audio_encoder" doc:"Audio encoder being used"`
	}
}

// PreviewCommand generates a preview of the FFmpeg command for the given profile configuration.
func (h *EncodingProfileHandler) PreviewCommand(ctx context.Context, input *PreviewEncodingProfileInput) (*PreviewEncodingProfileOutput, error) {
	// Create a temporary profile from the input to generate flags
	profile := &models.EncodingProfile{
		TargetVideoCodec: models.VideoCodec(input.Body.TargetVideoCodec),
		TargetAudioCodec: models.AudioCodec(input.Body.TargetAudioCodec),
		QualityPreset:    models.QualityPreset(input.Body.QualityPreset),
		HWAccel:          models.HWAccelType(input.Body.HWAccel),
		GlobalFlags:      input.Body.GlobalFlags,
		InputFlags:       input.Body.InputFlags,
		OutputFlags:      input.Body.OutputFlags,
	}

	// Set default HW accel if empty
	if profile.HWAccel == "" {
		profile.HWAccel = models.HWAccelAuto
	}

	resp := &PreviewEncodingProfileOutput{}

	// Determine which flags to use
	var globalFlags, inputFlags, outputFlags string

	if profile.GlobalFlags != "" {
		globalFlags = profile.GlobalFlags
		resp.Body.UsingCustom = true
	} else {
		defaults := profile.GenerateDefaultFlags()
		globalFlags = defaults.GlobalFlags
	}

	if profile.InputFlags != "" {
		inputFlags = profile.InputFlags
		resp.Body.UsingCustom = true
	} else {
		defaults := profile.GenerateDefaultFlags()
		inputFlags = defaults.InputFlags
	}

	if profile.OutputFlags != "" {
		outputFlags = profile.OutputFlags
		resp.Body.UsingCustom = true
	} else {
		defaults := profile.GenerateDefaultFlags()
		outputFlags = defaults.OutputFlags
	}

	resp.Body.GlobalFlags = globalFlags
	resp.Body.InputFlags = inputFlags
	resp.Body.OutputFlags = outputFlags
	resp.Body.VideoEncoder = profile.GetVideoEncoder()
	resp.Body.AudioEncoder = profile.GetAudioEncoder()

	// Build the full command
	resp.Body.Command = "ffmpeg " + globalFlags + " " + inputFlags + " -i pipe:0 " + outputFlags + " pipe:1"

	return resp, nil
}
