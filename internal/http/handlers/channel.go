package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"gorm.io/gorm"
)

// ChannelHandler handles channel browsing API endpoints.
type ChannelHandler struct {
	db           *gorm.DB
	relayService *service.RelayService
}

// NewChannelHandler creates a new channel handler.
func NewChannelHandler(db *gorm.DB, relayService *service.RelayService) *ChannelHandler {
	return &ChannelHandler{
		db:           db,
		relayService: relayService,
	}
}

// Register registers the channel routes with the API.
func (h *ChannelHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listChannels",
		Method:      "GET",
		Path:        "/api/v1/channels",
		Summary:     "List all channels",
		Description: "Returns paginated list of channels across all sources",
		Tags:        []string{"Channels"},
	}, h.ListChannels)

	huma.Register(api, huma.Operation{
		OperationID: "getChannel",
		Method:      "GET",
		Path:        "/api/v1/channels/{id}",
		Summary:     "Get channel by ID",
		Description: "Returns a specific channel by ID",
		Tags:        []string{"Channels"},
	}, h.GetChannel)

	huma.Register(api, huma.Operation{
		OperationID: "probeChannel",
		Method:      "POST",
		Path:        "/api/v1/channels/{id}/probe",
		Summary:     "Probe channel stream",
		Description: "Probes the channel stream to get codec information",
		Tags:        []string{"Channels"},
	}, h.ProbeChannel)

	huma.Register(api, huma.Operation{
		OperationID: "getChannelGroups",
		Method:      "GET",
		Path:        "/api/v1/channels/groups",
		Summary:     "Get channel groups",
		Description: "Returns list of distinct channel groups/categories",
		Tags:        []string{"Channels"},
	}, h.GetGroups)
}

// ListChannelsInput is the input for listing channels.
type ListChannelsInput struct {
	Page      int    `query:"page" default:"1" minimum:"1"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"500"`
	Search    string `query:"search"`
	SourceID  string `query:"source_id"`
	Group     string `query:"group"`
	SortBy    string `query:"sort_by" default:"channel_name"`
	SortOrder string `query:"sort_order" default:"asc" enum:"asc,desc"`
}

// ListChannelsOutput is the output for listing channels.
type ListChannelsOutput struct {
	Body struct {
		Success    bool              `json:"success"`
		Items      []ChannelResponse `json:"items"`
		Total      int64             `json:"total"`
		Page       int               `json:"page"`
		PerPage    int               `json:"per_page"`
		TotalPages int               `json:"total_pages"`
		HasNext    bool              `json:"has_next"`
		HasPrev    bool              `json:"has_previous"`
	}
}

// ListChannels returns paginated list of channels.
func (h *ChannelHandler) ListChannels(ctx context.Context, input *ListChannelsInput) (*ListChannelsOutput, error) {
	var channels []models.Channel
	var total int64

	query := h.db.WithContext(ctx).Model(&models.Channel{})

	// Apply filters
	if input.SourceID != "" {
		query = query.Where("source_id = ?", input.SourceID)
	}
	if input.Group != "" {
		query = query.Where("group_title = ?", input.Group)
	}
	if input.Search != "" {
		searchPattern := "%" + input.Search + "%"
		query = query.Where("channel_name LIKE ? OR tvg_name LIKE ? OR tvg_id LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to count channels")
	}

	// Apply sorting
	sortColumn := "channel_name"
	switch input.SortBy {
	case "channel_number":
		sortColumn = "channel_number"
	case "group_title":
		sortColumn = "group_title"
	case "updated_at":
		sortColumn = "updated_at"
	case "created_at":
		sortColumn = "created_at"
	}
	sortOrder := "ASC"
	if input.SortOrder == "desc" {
		sortOrder = "DESC"
	}
	query = query.Order(sortColumn + " " + sortOrder)

	// Apply pagination
	offset := (input.Page - 1) * input.Limit
	if err := query.Offset(offset).Limit(input.Limit).Find(&channels).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch channels")
	}

	// Convert to response format using shared type
	items := make([]ChannelResponse, len(channels))
	for i := range channels {
		items[i] = ChannelFromModel(&channels[i])
	}

	totalPages := int(total) / input.Limit
	if int(total)%input.Limit > 0 {
		totalPages++
	}

	resp := &ListChannelsOutput{}
	resp.Body.Success = true
	resp.Body.Items = items
	resp.Body.Total = total
	resp.Body.Page = input.Page
	resp.Body.PerPage = input.Limit
	resp.Body.TotalPages = totalPages
	resp.Body.HasNext = input.Page < totalPages
	resp.Body.HasPrev = input.Page > 1

	return resp, nil
}

// GetChannelInput is the input for getting a channel.
type GetChannelInput struct {
	ID string `path:"id" required:"true"`
}

// GetChannelOutput is the output for getting a channel.
type GetChannelOutput struct {
	Body struct {
		Success bool            `json:"success"`
		Data    ChannelResponse `json:"data"`
	}
}

// GetChannel returns a specific channel by ID.
func (h *ChannelHandler) GetChannel(ctx context.Context, input *GetChannelInput) (*GetChannelOutput, error) {
	var channel models.Channel

	if err := h.db.WithContext(ctx).Where("id = ?", input.ID).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, huma.Error404NotFound("Channel not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch channel")
	}

	resp := &GetChannelOutput{}
	resp.Body.Success = true
	resp.Body.Data = ChannelFromModel(&channel)

	return resp, nil
}

// ProbeChannelInput is the input for probing a channel.
type ProbeChannelInput struct {
	ID string `path:"id" required:"true"`
}

// ProbeChannelOutput is the output for probing a channel.
type ProbeChannelOutput struct {
	Body struct {
		Success   bool   `json:"success"`
		ChannelID string `json:"channel_id"`
		StreamURL string `json:"stream_url"`
		Codecs    struct {
			Video string `json:"video,omitempty"`
			Audio string `json:"audio,omitempty"`
		} `json:"codecs"`
		Container  string `json:"container,omitempty"`
		Duration   string `json:"duration,omitempty"`
		Resolution string `json:"resolution,omitempty"`
		Bitrate    int    `json:"bitrate,omitempty"`
		Message    string `json:"message,omitempty"`
	}
}

// ProbeChannel probes a channel stream for codec information using ffprobe.
func (h *ChannelHandler) ProbeChannel(ctx context.Context, input *ProbeChannelInput) (*ProbeChannelOutput, error) {
	var channel models.Channel

	if err := h.db.WithContext(ctx).Where("id = ?", input.ID).First(&channel).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, huma.Error404NotFound("Channel not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch channel")
	}

	// Check if relay service is available
	if h.relayService == nil {
		resp := &ProbeChannelOutput{}
		resp.Body.Success = false
		resp.Body.ChannelID = channel.ID.String()
		resp.Body.StreamURL = channel.StreamURL
		resp.Body.Message = "Stream probing not available (relay service not configured)"
		return resp, nil
	}

	// Probe the stream using the relay service
	codec, err := h.relayService.ProbeStream(ctx, channel.StreamURL)
	if err != nil {
		resp := &ProbeChannelOutput{}
		resp.Body.Success = false
		resp.Body.ChannelID = channel.ID.String()
		resp.Body.StreamURL = channel.StreamURL
		resp.Body.Message = "Failed to probe stream: " + err.Error()
		return resp, nil
	}

	// Build successful response
	resp := &ProbeChannelOutput{}
	resp.Body.Success = true
	resp.Body.ChannelID = channel.ID.String()
	resp.Body.StreamURL = channel.StreamURL
	resp.Body.Codecs.Video = codec.VideoCodec
	resp.Body.Codecs.Audio = codec.AudioCodec

	// Build resolution string
	if codec.VideoWidth > 0 && codec.VideoHeight > 0 {
		resp.Body.Resolution = formatResolution(codec.VideoWidth, codec.VideoHeight)
	}

	// Calculate total bitrate
	resp.Body.Bitrate = codec.VideoBitrate + codec.AudioBitrate

	return resp, nil
}

// formatResolution formats video dimensions as a resolution string.
func formatResolution(width, height int) string {
	if width == 0 || height == 0 {
		return ""
	}
	// Common resolution names
	switch {
	case height >= 2160:
		return "4K"
	case height >= 1080:
		return "1080p"
	case height >= 720:
		return "720p"
	case height >= 480:
		return "480p"
	default:
		return ""
	}
}

// GetGroupsInput is the input for getting channel groups.
type GetGroupsInput struct{}

// GetGroupsOutput is the output for getting channel groups.
type GetGroupsOutput struct {
	Body struct {
		Success bool     `json:"success"`
		Groups  []string `json:"groups"`
		Count   int      `json:"count"`
	}
}

// GetGroups returns distinct channel groups.
func (h *ChannelHandler) GetGroups(ctx context.Context, input *GetGroupsInput) (*GetGroupsOutput, error) {
	var groups []string

	if err := h.db.WithContext(ctx).
		Model(&models.Channel{}).
		Distinct("group_title").
		Where("group_title != ''").
		Order("group_title ASC").
		Pluck("group_title", &groups).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch groups")
	}

	resp := &GetGroupsOutput{}
	resp.Body.Success = true
	resp.Body.Groups = groups
	resp.Body.Count = len(groups)

	return resp, nil
}
