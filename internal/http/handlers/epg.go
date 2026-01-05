package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// EpgHandler handles EPG browsing API endpoints.
type EpgHandler struct {
	db *gorm.DB
}

// NewEpgHandler creates a new EPG handler.
func NewEpgHandler(db *gorm.DB) *EpgHandler {
	return &EpgHandler{db: db}
}

// Register registers the EPG routes with the API.
func (h *EpgHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listEpgPrograms",
		Method:      "GET",
		Path:        "/api/v1/epg/programs",
		Summary:     "List EPG programs",
		Description: "Returns paginated list of EPG programs with optional filtering",
		Tags:        []string{"EPG"},
	}, h.ListPrograms)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgProgram",
		Method:      "GET",
		Path:        "/api/v1/epg/programs/{id}",
		Summary:     "Get EPG program by ID",
		Description: "Returns a specific EPG program by ID",
		Tags:        []string{"EPG"},
	}, h.GetProgram)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgNowPlaying",
		Method:      "GET",
		Path:        "/api/v1/epg/now",
		Summary:     "Get currently airing programs",
		Description: "Returns programs currently airing across all channels",
		Tags:        []string{"EPG"},
	}, h.GetNowPlaying)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgByChannel",
		Method:      "GET",
		Path:        "/api/v1/epg/channel/{channel_id}",
		Summary:     "Get EPG for channel",
		Description: "Returns EPG data for a specific channel",
		Tags:        []string{"EPG"},
	}, h.GetByChannel)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgChannels",
		Method:      "GET",
		Path:        "/api/v1/epg/channels",
		Summary:     "Get EPG channels",
		Description: "Returns list of distinct channel IDs with EPG data",
		Tags:        []string{"EPG"},
	}, h.GetChannels)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgCategories",
		Method:      "GET",
		Path:        "/api/v1/epg/categories",
		Summary:     "Get EPG categories",
		Description: "Returns list of distinct program categories/genres",
		Tags:        []string{"EPG"},
	}, h.GetCategories)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgStats",
		Method:      "GET",
		Path:        "/api/v1/epg/stats",
		Summary:     "Get EPG statistics",
		Description: "Returns statistics about EPG data",
		Tags:        []string{"EPG"},
	}, h.GetStats)

	huma.Register(api, huma.Operation{
		OperationID: "searchEpg",
		Method:      "GET",
		Path:        "/api/v1/epg/search",
		Summary:     "Search EPG programs",
		Description: "Search EPG programs by title, description, or category",
		Tags:        []string{"EPG"},
	}, h.Search)

	huma.Register(api, huma.Operation{
		OperationID: "getEpgGuide",
		Method:      "GET",
		Path:        "/api/v1/epg/guide",
		Summary:     "Get EPG TV guide",
		Description: "Returns EPG data formatted for TV guide display with channels and programs",
		Tags:        []string{"EPG"},
	}, h.GetGuide)
}

// ListProgramsInput is the input for listing EPG programs.
type ListProgramsInput struct {
	Page      int    `query:"page" default:"1" minimum:"1"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"500"`
	ChannelID string `query:"channel_id"`
	SourceID  string `query:"source_id"`
	Category  string `query:"category"`
	StartFrom string `query:"start_from"` // RFC3339 time
	StartTo   string `query:"start_to"`   // RFC3339 time
	OnAir     bool   `query:"on_air"`
}

// ListProgramsOutput is the output for listing EPG programs.
type ListProgramsOutput struct {
	Body struct {
		Success    bool                 `json:"success"`
		Items      []EpgProgramResponse `json:"items"`
		Total      int64                `json:"total"`
		Page       int                  `json:"page"`
		PerPage    int                  `json:"per_page"`
		TotalPages int                  `json:"total_pages"`
		HasNext    bool                 `json:"has_next"`
		HasPrev    bool                 `json:"has_previous"`
	}
}

// ListPrograms returns paginated list of EPG programs.
func (h *EpgHandler) ListPrograms(ctx context.Context, input *ListProgramsInput) (*ListProgramsOutput, error) {
	var programs []models.EpgProgram
	var total int64

	query := h.db.WithContext(ctx).Model(&models.EpgProgram{})

	// Apply filters
	if input.ChannelID != "" {
		query = query.Where("channel_id = ?", input.ChannelID)
	}
	if input.SourceID != "" {
		query = query.Where("source_id = ?", input.SourceID)
	}
	if input.Category != "" {
		query = query.Where("category = ?", input.Category)
	}
	if input.StartFrom != "" {
		if t, err := time.Parse(time.RFC3339, input.StartFrom); err == nil {
			query = query.Where("start >= ?", t)
		}
	}
	if input.StartTo != "" {
		if t, err := time.Parse(time.RFC3339, input.StartTo); err == nil {
			query = query.Where("start <= ?", t)
		}
	}
	if input.OnAir {
		now := time.Now()
		query = query.Where("start <= ? AND stop > ?", now, now)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to count EPG programs")
	}

	// Apply sorting and pagination
	offset := (input.Page - 1) * input.Limit
	if err := query.Order("start ASC").Offset(offset).Limit(input.Limit).Find(&programs).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch EPG programs")
	}

	// Convert to response format using shared type
	items := make([]EpgProgramResponse, len(programs))
	for i := range programs {
		items[i] = EpgProgramFromModel(&programs[i])
	}

	totalPages := int(total) / input.Limit
	if int(total)%input.Limit > 0 {
		totalPages++
	}

	resp := &ListProgramsOutput{}
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

// GetProgramInput is the input for getting an EPG program.
type GetProgramInput struct {
	ID string `path:"id" required:"true"`
}

// GetProgramOutput is the output for getting an EPG program.
type GetProgramOutput struct {
	Body struct {
		Success bool               `json:"success"`
		Data    EpgProgramResponse `json:"data"`
	}
}

// GetProgram returns a specific EPG program by ID.
func (h *EpgHandler) GetProgram(ctx context.Context, input *GetProgramInput) (*GetProgramOutput, error) {
	var program models.EpgProgram

	if err := h.db.WithContext(ctx).Where("id = ?", input.ID).First(&program).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, huma.Error404NotFound("EPG program not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch EPG program")
	}

	resp := &GetProgramOutput{}
	resp.Body.Success = true
	resp.Body.Data = EpgProgramFromModel(&program)

	return resp, nil
}

// GetNowPlayingInput is the input for getting currently airing programs.
type GetNowPlayingInput struct {
	Limit int `query:"limit" default:"100" minimum:"1" maximum:"500"`
}

// GetNowPlayingOutput is the output for getting currently airing programs.
type GetNowPlayingOutput struct {
	Body struct {
		Success   bool                          `json:"success"`
		Timestamp time.Time                     `json:"timestamp"`
		Programs  map[string]EpgProgramResponse `json:"programs"` // keyed by channel_id
		Count     int                           `json:"count"`
	}
}

// GetNowPlaying returns programs currently airing.
func (h *EpgHandler) GetNowPlaying(ctx context.Context, input *GetNowPlayingInput) (*GetNowPlayingOutput, error) {
	now := time.Now()
	var programs []models.EpgProgram

	if err := h.db.WithContext(ctx).
		Where("start <= ? AND stop > ?", now, now).
		Order("channel_id ASC").
		Limit(input.Limit).
		Find(&programs).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch currently airing programs")
	}

	programsMap := make(map[string]EpgProgramResponse)
	for i := range programs {
		programsMap[programs[i].ChannelID] = EpgProgramFromModel(&programs[i])
	}

	resp := &GetNowPlayingOutput{}
	resp.Body.Success = true
	resp.Body.Timestamp = now
	resp.Body.Programs = programsMap
	resp.Body.Count = len(programsMap)

	return resp, nil
}

// GetByChannelInput is the input for getting EPG for a channel.
type GetByChannelInput struct {
	ChannelID string `path:"channel_id" required:"true"`
	StartFrom string `query:"start_from"` // RFC3339 time, defaults to now
	Hours     int    `query:"hours" default:"24" minimum:"1" maximum:"168"`
	Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"200"`
}

// GetByChannelOutput is the output for getting EPG for a channel.
type GetByChannelOutput struct {
	Body struct {
		Success   bool                 `json:"success"`
		ChannelID string               `json:"channel_id"`
		Programs  []EpgProgramResponse `json:"programs"`
		Count     int                  `json:"count"`
		TimeRange struct {
			Start time.Time `json:"start"`
			End   time.Time `json:"end"`
		} `json:"time_range"`
	}
}

// GetByChannel returns EPG data for a specific channel.
func (h *EpgHandler) GetByChannel(ctx context.Context, input *GetByChannelInput) (*GetByChannelOutput, error) {
	start := time.Now()
	if input.StartFrom != "" {
		if t, err := time.Parse(time.RFC3339, input.StartFrom); err == nil {
			start = t
		}
	}
	end := start.Add(time.Duration(input.Hours) * time.Hour)

	var programs []models.EpgProgram

	// Get programs that overlap with the time range
	if err := h.db.WithContext(ctx).
		Where("channel_id = ? AND start < ? AND stop > ?", input.ChannelID, end, start).
		Order("start ASC").
		Limit(input.Limit).
		Find(&programs).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch EPG programs")
	}

	items := make([]EpgProgramResponse, len(programs))
	for i := range programs {
		items[i] = EpgProgramFromModel(&programs[i])
	}

	resp := &GetByChannelOutput{}
	resp.Body.Success = true
	resp.Body.ChannelID = input.ChannelID
	resp.Body.Programs = items
	resp.Body.Count = len(items)
	resp.Body.TimeRange.Start = start
	resp.Body.TimeRange.End = end

	return resp, nil
}

// GetChannelsInput is the input for getting EPG channels.
type GetChannelsInput struct{}

// GetChannelsOutput is the output for getting EPG channels.
type GetChannelsOutput struct {
	Body struct {
		Success  bool     `json:"success"`
		Channels []string `json:"channels"`
		Count    int      `json:"count"`
	}
}

// GetChannels returns distinct channel IDs with EPG data.
func (h *EpgHandler) GetChannels(ctx context.Context, input *GetChannelsInput) (*GetChannelsOutput, error) {
	var channels []string

	if err := h.db.WithContext(ctx).
		Model(&models.EpgProgram{}).
		Distinct("channel_id").
		Order("channel_id ASC").
		Pluck("channel_id", &channels).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch EPG channels")
	}

	resp := &GetChannelsOutput{}
	resp.Body.Success = true
	resp.Body.Channels = channels
	resp.Body.Count = len(channels)

	return resp, nil
}

// GetCategoriesInput is the input for getting EPG categories.
type GetCategoriesInput struct{}

// GetCategoriesOutput is the output for getting EPG categories.
type GetCategoriesOutput struct {
	Body struct {
		Success    bool     `json:"success"`
		Categories []string `json:"categories"`
		Count      int      `json:"count"`
	}
}

// GetCategories returns distinct program categories.
func (h *EpgHandler) GetCategories(ctx context.Context, input *GetCategoriesInput) (*GetCategoriesOutput, error) {
	var categories []string

	if err := h.db.WithContext(ctx).
		Model(&models.EpgProgram{}).
		Distinct("category").
		Where("category != ''").
		Order("category ASC").
		Pluck("category", &categories).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch EPG categories")
	}

	resp := &GetCategoriesOutput{}
	resp.Body.Success = true
	resp.Body.Categories = categories
	resp.Body.Count = len(categories)

	return resp, nil
}

// GetStatsInput is the input for getting EPG statistics.
type GetStatsInput struct{}

// GetStatsOutput is the output for getting EPG statistics.
type GetStatsOutput struct {
	Body struct {
		Success bool `json:"success"`
		Stats   struct {
			TotalPrograms    int64     `json:"total_programs"`
			UniqueChannels   int       `json:"unique_channels"`
			UniqueCategories int       `json:"unique_categories"`
			CurrentlyAiring  int64     `json:"currently_airing"`
			UpcomingPrograms int64     `json:"upcoming_programs"`
			ExpiredPrograms  int64     `json:"expired_programs"`
			EarliestProgram  time.Time `json:"earliest_program"`
			LatestProgram    time.Time `json:"latest_program"`
		} `json:"stats"`
	}
}

// GetStats returns EPG statistics.
func (h *EpgHandler) GetStats(ctx context.Context, input *GetStatsInput) (*GetStatsOutput, error) {
	now := time.Now()
	resp := &GetStatsOutput{}
	resp.Body.Success = true

	// Total programs
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).Count(&resp.Body.Stats.TotalPrograms)

	// Currently airing
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Where("start <= ? AND stop > ?", now, now).
		Count(&resp.Body.Stats.CurrentlyAiring)

	// Upcoming (start in future)
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Where("start > ?", now).
		Count(&resp.Body.Stats.UpcomingPrograms)

	// Expired (stop in past)
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Where("stop <= ?", now).
		Count(&resp.Body.Stats.ExpiredPrograms)

	// Unique channels
	var channels []string
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Distinct("channel_id").
		Pluck("channel_id", &channels)
	resp.Body.Stats.UniqueChannels = len(channels)

	// Unique categories
	var categories []string
	h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Distinct("category").
		Where("category != ''").
		Pluck("category", &categories)
	resp.Body.Stats.UniqueCategories = len(categories)

	// Earliest and latest programs
	var earliest, latest models.EpgProgram
	if err := h.db.WithContext(ctx).Order("start ASC").First(&earliest).Error; err == nil {
		resp.Body.Stats.EarliestProgram = earliest.Start
	}
	if err := h.db.WithContext(ctx).Order("stop DESC").First(&latest).Error; err == nil {
		resp.Body.Stats.LatestProgram = latest.Stop
	}

	return resp, nil
}

// SearchInput is the input for searching EPG programs.
type SearchInput struct {
	Query    string `query:"q" required:"true" minLength:"2"`
	Page     int    `query:"page" default:"1" minimum:"1"`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"200"`
	Category string `query:"category"`
	OnAir    bool   `query:"on_air"`
}

// SearchOutput is the output for searching EPG programs.
type SearchOutput struct {
	Body struct {
		Success    bool                 `json:"success"`
		Query      string               `json:"query"`
		Items      []EpgProgramResponse `json:"items"`
		Total      int64                `json:"total"`
		Page       int                  `json:"page"`
		PerPage    int                  `json:"per_page"`
		TotalPages int                  `json:"total_pages"`
	}
}

// Search searches EPG programs.
func (h *EpgHandler) Search(ctx context.Context, input *SearchInput) (*SearchOutput, error) {
	var programs []models.EpgProgram
	var total int64

	searchPattern := "%" + input.Query + "%"
	query := h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Where("title LIKE ? OR sub_title LIKE ? OR description LIKE ?",
			searchPattern, searchPattern, searchPattern)

	if input.Category != "" {
		query = query.Where("category = ?", input.Category)
	}
	if input.OnAir {
		now := time.Now()
		query = query.Where("start <= ? AND stop > ?", now, now)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to search EPG programs")
	}

	// Apply pagination
	offset := (input.Page - 1) * input.Limit
	if err := query.Order("start ASC").Offset(offset).Limit(input.Limit).Find(&programs).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to search EPG programs")
	}

	items := make([]EpgProgramResponse, len(programs))
	for i := range programs {
		items[i] = EpgProgramFromModel(&programs[i])
	}

	totalPages := int(total) / input.Limit
	if int(total)%input.Limit > 0 {
		totalPages++
	}

	resp := &SearchOutput{}
	resp.Body.Success = true
	resp.Body.Query = input.Query
	resp.Body.Items = items
	resp.Body.Total = total
	resp.Body.Page = input.Page
	resp.Body.PerPage = input.Limit
	resp.Body.TotalPages = totalPages

	return resp, nil
}

// GetGuideInput is the input for getting the EPG TV guide.
type GetGuideInput struct {
	StartTime string `query:"start_time"` // RFC3339 time, defaults to current hour
	EndTime   string `query:"end_time"`   // RFC3339 time, defaults to start + 12 hours
	SourceID  string `query:"source_id"`  // Comma-separated source IDs to filter
}

// GuideChannelInfo represents channel info in the guide response.
type GuideChannelInfo struct {
	ID         string `json:"id"`                    // The EPG channel ID (e.g., tvg_id)
	DatabaseID string `json:"database_id,omitempty"` // The database channel ID (ULID) for streaming
	Name       string `json:"name"`
	Logo       string `json:"logo,omitempty"`
	StreamURL  string `json:"stream_url,omitempty"` // The upstream stream URL for direct playback
}

// GuideProgramInfo represents a program in the guide response.
type GuideProgramInfo struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelLogo string `json:"channel_logo,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Category    string `json:"category,omitempty"`
	Rating      string `json:"rating,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	IsLive      bool   `json:"is_live"`
}

// GetGuideOutput is the output for the EPG TV guide.
type GetGuideOutput struct {
	Body struct {
		Success bool       `json:"success"`
		Data    *GuideData `json:"data"`
	}
}

// GuideData contains the guide response data.
type GuideData struct {
	Channels  map[string]GuideChannelInfo   `json:"channels"`
	Programs  map[string][]GuideProgramInfo `json:"programs"`
	TimeSlots []string                      `json:"time_slots"`
	StartTime string                        `json:"start_time"`
	EndTime   string                        `json:"end_time"`
}

// GetGuide returns EPG data formatted for TV guide display.
func (h *EpgHandler) GetGuide(ctx context.Context, input *GetGuideInput) (*GetGuideOutput, error) {
	// Parse time range
	now := time.Now()
	startTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	endTime := startTime.Add(12 * time.Hour)

	if input.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, input.StartTime); err == nil {
			startTime = t
		}
	}
	if input.EndTime != "" {
		if t, err := time.Parse(time.RFC3339, input.EndTime); err == nil {
			endTime = t
		}
	}

	// Build query for programs in time range
	query := h.db.WithContext(ctx).Model(&models.EpgProgram{}).
		Where("start < ? AND stop > ?", endTime, startTime)

	// Apply source filter if provided
	if input.SourceID != "" {
		query = query.Where("source_id IN ?", splitSourceIDs(input.SourceID))
	}

	// Fetch programs
	var programs []models.EpgProgram
	if err := query.Order("channel_id ASC, start ASC").Find(&programs).Error; err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch EPG programs")
	}

	// Build response data
	channels := make(map[string]GuideChannelInfo)
	programsByChannel := make(map[string][]GuideProgramInfo)

	for _, p := range programs {
		// Add channel info if not already present
		if _, exists := channels[p.ChannelID]; !exists {
			channels[p.ChannelID] = GuideChannelInfo{
				ID:   p.ChannelID,
				Name: p.ChannelID, // Will be updated below if we can find the channel name
				Logo: "",
			}
		}

		// Check if program is currently live
		isLive := now.After(p.Start) && now.Before(p.Stop)

		// Add program
		programInfo := GuideProgramInfo{
			ID:          p.ID.String(),
			ChannelID:   p.ChannelID,
			ChannelName: p.ChannelID,
			Title:       p.Title,
			Description: p.Description,
			StartTime:   p.Start.Format(time.RFC3339),
			EndTime:     p.Stop.Format(time.RFC3339),
			Category:    p.Category,
			Rating:      p.Rating,
			SourceID:    p.SourceID.String(),
			IsLive:      isLive,
		}
		programsByChannel[p.ChannelID] = append(programsByChannel[p.ChannelID], programInfo)
	}

	// Try to get channel names from the channels table
	var channelIDs []string
	for id := range channels {
		channelIDs = append(channelIDs, id)
	}

	if len(channelIDs) > 0 {
		var dbChannels []models.Channel
		if err := h.db.WithContext(ctx).
			Where("tvg_id IN ?", channelIDs).
			Find(&dbChannels).Error; err == nil {
			for _, ch := range dbChannels {
				if info, exists := channels[ch.TvgID]; exists {
					info.Name = ch.ChannelName
					info.Logo = ch.TvgLogo
					info.DatabaseID = ch.ID.String() // Add the database ID for streaming
					info.StreamURL = ch.StreamURL    // Add the stream URL for direct playback
					channels[ch.TvgID] = info
					// Update program channel names
					for i := range programsByChannel[ch.TvgID] {
						programsByChannel[ch.TvgID][i].ChannelName = ch.ChannelName
						programsByChannel[ch.TvgID][i].ChannelLogo = ch.TvgLogo
					}
				}
			}
		}
	}

	// Generate time slots (hourly)
	var timeSlots []string
	for t := startTime; t.Before(endTime); t = t.Add(time.Hour) {
		timeSlots = append(timeSlots, t.Format(time.RFC3339))
	}

	resp := &GetGuideOutput{}
	resp.Body.Success = true
	resp.Body.Data = &GuideData{
		Channels:  channels,
		Programs:  programsByChannel,
		TimeSlots: timeSlots,
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	return resp, nil
}

// splitSourceIDs splits a comma-separated string of source IDs.
func splitSourceIDs(s string) []string {
	if s == "" {
		return nil
	}
	var ids []string
	for _, id := range splitString(s, ',') {
		id = trimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// splitString splits a string by a separator.
func splitString(s string, sep rune) []string {
	var result []string
	var current string
	for _, r := range s {
		if r == sep {
			result = append(result, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
