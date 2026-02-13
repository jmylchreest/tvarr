package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// ChannelHandler handles channel browsing API endpoints.
type ChannelHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewChannelHandler creates a new channel handler.
func NewChannelHandler(db *gorm.DB) *ChannelHandler {
	return &ChannelHandler{
		db:     db,
		logger: slog.Default(),
	}
}

// getCodecMapForStreamURLs retrieves codec info for multiple stream URLs efficiently.
// Returns a map of stream_url -> LastKnownCodec for easy lookup.
func (h *ChannelHandler) getCodecMapForStreamURLs(ctx context.Context, streamURLs []string) map[string]*models.LastKnownCodec {
	codecMap := make(map[string]*models.LastKnownCodec)
	if len(streamURLs) == 0 {
		return codecMap
	}

	var codecs []models.LastKnownCodec
	if err := h.db.WithContext(ctx).
		Where("stream_url IN ?", streamURLs).
		Find(&codecs).Error; err != nil {
		h.logger.Warn("Failed to fetch codec info for channels", "error", err)
		return codecMap
	}

	for i := range codecs {
		codecMap[codecs[i].StreamURL] = &codecs[i]
	}
	return codecMap
}

// WithLogger sets the logger for the handler.
func (h *ChannelHandler) WithLogger(logger *slog.Logger) *ChannelHandler {
	h.logger = logger
	return h
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

	// Apply filters — source_id supports comma-separated values for multi-source filtering
	if input.SourceID != "" {
		ids := strings.Split(input.SourceID, ",")
		trimmed := make([]string, 0, len(ids))
		for _, id := range ids {
			if s := strings.TrimSpace(id); s != "" {
				trimmed = append(trimmed, s)
			}
		}
		if len(trimmed) == 1 {
			query = query.Where("source_id = ?", trimmed[0])
		} else if len(trimmed) > 1 {
			query = query.Where("source_id IN ?", trimmed)
		}
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

	// Collect stream URLs for batch codec lookup
	streamURLs := make([]string, len(channels))
	for i := range channels {
		streamURLs[i] = channels[i].StreamURL
	}

	// Batch fetch codec info for all channels
	codecMap := h.getCodecMapForStreamURLs(ctx, streamURLs)

	// Convert to response format using shared type
	items := make([]ChannelResponse, len(channels))
	for i := range channels {
		items[i] = ChannelFromModel(&channels[i])
		// Populate codec info if available
		if codec, ok := codecMap[channels[i].StreamURL]; ok {
			items[i].PopulateCodecInfo(codec)
		}
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

	// Look up codec info for this channel
	var codec models.LastKnownCodec
	if err := h.db.WithContext(ctx).Where("stream_url = ?", channel.StreamURL).First(&codec).Error; err == nil {
		resp.Body.Data.PopulateCodecInfo(&codec)
	}

	return resp, nil
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
