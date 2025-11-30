// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// Common response types

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// Pagination contains pagination parameters for list requests.
type Pagination struct {
	Page  int `query:"page" default:"1" minimum:"1" doc:"Page number (1-indexed)"`
	Limit int `query:"limit" default:"50" minimum:"1" maximum:"1000" doc:"Items per page"`
}

// PaginationMeta contains pagination metadata in responses.
type PaginationMeta struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int64 `json:"total_pages"`
}

// Stream Source types

// StreamSourceResponse represents a stream source in API responses.
type StreamSourceResponse struct {
	ID              models.ULID         `json:"id"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	Name            string              `json:"name"`
	Type            models.SourceType   `json:"type"`
	URL             string              `json:"url"`
	Username        string              `json:"username,omitempty"`
	UserAgent       string              `json:"user_agent,omitempty"`
	Enabled         bool                `json:"enabled"`
	Priority        int                 `json:"priority"`
	Status          models.SourceStatus `json:"status"`
	LastIngestionAt *time.Time          `json:"last_ingestion_at,omitempty"`
	LastError       string              `json:"last_error,omitempty"`
	ChannelCount    int                 `json:"channel_count"`
	CronSchedule    string              `json:"cron_schedule,omitempty"`
}

// StreamSourceFromModel converts a model to a response.
func StreamSourceFromModel(s *models.StreamSource) StreamSourceResponse {
	return StreamSourceResponse{
		ID:              s.ID,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		Name:            s.Name,
		Type:            s.Type,
		URL:             s.URL,
		Username:        s.Username,
		UserAgent:       s.UserAgent,
		Enabled:         s.Enabled,
		Priority:        s.Priority,
		Status:          s.Status,
		LastIngestionAt: s.LastIngestionAt,
		LastError:       s.LastError,
		ChannelCount:    s.ChannelCount,
		CronSchedule:    s.CronSchedule,
	}
}

// CreateStreamSourceRequest is the request body for creating a stream source.
type CreateStreamSourceRequest struct {
	Name         string            `json:"name" doc:"User-friendly name for the source" minLength:"1" maxLength:"255"`
	Type         models.SourceType `json:"type" doc:"Source type: m3u or xtream" enum:"m3u,xtream"`
	URL          string            `json:"url" doc:"M3U playlist URL or Xtream server URL" minLength:"1" maxLength:"2048"`
	Username     string            `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password     string            `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent    string            `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled      *bool             `json:"enabled,omitempty" doc:"Whether the source is enabled (default: true)"`
	Priority     *int              `json:"priority,omitempty" doc:"Priority for channel merging (higher = preferred)"`
	CronSchedule string            `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
}

// ToModel converts the request to a model.
func (r *CreateStreamSourceRequest) ToModel() *models.StreamSource {
	source := &models.StreamSource{
		Name:         r.Name,
		Type:         r.Type,
		URL:          r.URL,
		Username:     r.Username,
		Password:     r.Password,
		UserAgent:    r.UserAgent,
		Enabled:      true,
		Priority:     0,
		CronSchedule: r.CronSchedule,
	}
	if r.Enabled != nil {
		source.Enabled = *r.Enabled
	}
	if r.Priority != nil {
		source.Priority = *r.Priority
	}
	return source
}

// UpdateStreamSourceRequest is the request body for updating a stream source.
type UpdateStreamSourceRequest struct {
	Name         *string            `json:"name,omitempty" doc:"User-friendly name for the source" maxLength:"255"`
	Type         *models.SourceType `json:"type,omitempty" doc:"Source type: m3u or xtream" enum:"m3u,xtream"`
	URL          *string            `json:"url,omitempty" doc:"M3U playlist URL or Xtream server URL" maxLength:"2048"`
	Username     *string            `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password     *string            `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent    *string            `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled      *bool              `json:"enabled,omitempty" doc:"Whether the source is enabled"`
	Priority     *int               `json:"priority,omitempty" doc:"Priority for channel merging"`
	CronSchedule *string            `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
}

// ApplyToModel applies the update request to an existing model.
func (r *UpdateStreamSourceRequest) ApplyToModel(s *models.StreamSource) {
	if r.Name != nil {
		s.Name = *r.Name
	}
	if r.Type != nil {
		s.Type = *r.Type
	}
	if r.URL != nil {
		s.URL = *r.URL
	}
	if r.Username != nil {
		s.Username = *r.Username
	}
	if r.Password != nil {
		s.Password = *r.Password
	}
	if r.UserAgent != nil {
		s.UserAgent = *r.UserAgent
	}
	if r.Enabled != nil {
		s.Enabled = *r.Enabled
	}
	if r.Priority != nil {
		s.Priority = *r.Priority
	}
	if r.CronSchedule != nil {
		s.CronSchedule = *r.CronSchedule
	}
}

// EPG Source types

// EpgSourceResponse represents an EPG source in API responses.
type EpgSourceResponse struct {
	ID              models.ULID            `json:"id"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Name            string                 `json:"name"`
	Type            models.EpgSourceType   `json:"type"`
	URL             string                 `json:"url"`
	Username        string                 `json:"username,omitempty"`
	UserAgent       string                 `json:"user_agent,omitempty"`
	Enabled         bool                   `json:"enabled"`
	Priority        int                    `json:"priority"`
	Status          models.EpgSourceStatus `json:"status"`
	LastIngestionAt *time.Time             `json:"last_ingestion_at,omitempty"`
	LastError       string                 `json:"last_error,omitempty"`
	ProgramCount    int                    `json:"program_count"`
	CronSchedule    string                 `json:"cron_schedule,omitempty"`
	RetentionDays   int                    `json:"retention_days"`
}

// EpgSourceFromModel converts a model to a response.
func EpgSourceFromModel(s *models.EpgSource) EpgSourceResponse {
	return EpgSourceResponse{
		ID:              s.ID,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		Name:            s.Name,
		Type:            s.Type,
		URL:             s.URL,
		Username:        s.Username,
		UserAgent:       s.UserAgent,
		Enabled:         s.Enabled,
		Priority:        s.Priority,
		Status:          s.Status,
		LastIngestionAt: s.LastIngestionAt,
		LastError:       s.LastError,
		ProgramCount:    s.ProgramCount,
		CronSchedule:    s.CronSchedule,
		RetentionDays:   s.RetentionDays,
	}
}

// CreateEpgSourceRequest is the request body for creating an EPG source.
type CreateEpgSourceRequest struct {
	Name          string               `json:"name" doc:"User-friendly name for the source" minLength:"1" maxLength:"255"`
	Type          models.EpgSourceType `json:"type" doc:"Source type: xmltv or xtream" enum:"xmltv,xtream"`
	URL           string               `json:"url" doc:"XMLTV URL or Xtream server URL" minLength:"1" maxLength:"2048"`
	Username      string               `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password      string               `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent     string               `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled       *bool                `json:"enabled,omitempty" doc:"Whether the source is enabled (default: true)"`
	Priority      *int                 `json:"priority,omitempty" doc:"Priority for program merging (higher = preferred)"`
	CronSchedule  string               `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
	RetentionDays *int                 `json:"retention_days,omitempty" doc:"Days to retain EPG data after expiry (default: 1)"`
}

// ToModel converts the request to a model.
func (r *CreateEpgSourceRequest) ToModel() *models.EpgSource {
	source := &models.EpgSource{
		Name:          r.Name,
		Type:          r.Type,
		URL:           r.URL,
		Username:      r.Username,
		Password:      r.Password,
		UserAgent:     r.UserAgent,
		Enabled:       true,
		Priority:      0,
		CronSchedule:  r.CronSchedule,
		RetentionDays: 1,
	}
	if r.Enabled != nil {
		source.Enabled = *r.Enabled
	}
	if r.Priority != nil {
		source.Priority = *r.Priority
	}
	if r.RetentionDays != nil {
		source.RetentionDays = *r.RetentionDays
	}
	return source
}

// UpdateEpgSourceRequest is the request body for updating an EPG source.
type UpdateEpgSourceRequest struct {
	Name          *string               `json:"name,omitempty" doc:"User-friendly name for the source" maxLength:"255"`
	Type          *models.EpgSourceType `json:"type,omitempty" doc:"Source type: xmltv or xtream" enum:"xmltv,xtream"`
	URL           *string               `json:"url,omitempty" doc:"XMLTV URL or Xtream server URL" maxLength:"2048"`
	Username      *string               `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password      *string               `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent     *string               `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled       *bool                 `json:"enabled,omitempty" doc:"Whether the source is enabled"`
	Priority      *int                  `json:"priority,omitempty" doc:"Priority for program merging"`
	CronSchedule  *string               `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
	RetentionDays *int                  `json:"retention_days,omitempty" doc:"Days to retain EPG data after expiry"`
}

// ApplyToModel applies the update request to an existing model.
func (r *UpdateEpgSourceRequest) ApplyToModel(s *models.EpgSource) {
	if r.Name != nil {
		s.Name = *r.Name
	}
	if r.Type != nil {
		s.Type = *r.Type
	}
	if r.URL != nil {
		s.URL = *r.URL
	}
	if r.Username != nil {
		s.Username = *r.Username
	}
	if r.Password != nil {
		s.Password = *r.Password
	}
	if r.UserAgent != nil {
		s.UserAgent = *r.UserAgent
	}
	if r.Enabled != nil {
		s.Enabled = *r.Enabled
	}
	if r.Priority != nil {
		s.Priority = *r.Priority
	}
	if r.CronSchedule != nil {
		s.CronSchedule = *r.CronSchedule
	}
	if r.RetentionDays != nil {
		s.RetentionDays = *r.RetentionDays
	}
}

// Stream Proxy types

// StreamProxyResponse represents a stream proxy in API responses.
type StreamProxyResponse struct {
	ID                    models.ULID              `json:"id"`
	CreatedAt             time.Time                `json:"created_at"`
	UpdatedAt             time.Time                `json:"updated_at"`
	Name                  string                   `json:"name"`
	Description           string                   `json:"description,omitempty"`
	ProxyMode             models.StreamProxyMode   `json:"proxy_mode"`
	IsActive              bool                     `json:"is_active"`
	AutoRegenerate        bool                     `json:"auto_regenerate"`
	StartingChannelNumber int                      `json:"starting_channel_number"`
	UpstreamTimeout       int                      `json:"upstream_timeout,omitempty"`
	BufferSize            int                      `json:"buffer_size,omitempty"`
	MaxConcurrentStreams  int                      `json:"max_concurrent_streams,omitempty"`
	CacheChannelLogos     bool                     `json:"cache_channel_logos"`
	CacheProgramLogos     bool                     `json:"cache_program_logos"`
	RelayProfileID        *models.ULID             `json:"relay_profile_id,omitempty"`
	Status                models.StreamProxyStatus `json:"status"`
	LastGeneratedAt       *time.Time               `json:"last_generated_at,omitempty"`
	LastError             string                   `json:"last_error,omitempty"`
	ChannelCount          int                      `json:"channel_count"`
	ProgramCount          int                      `json:"program_count"`
	OutputPath            string                   `json:"output_path,omitempty"`
}

// StreamProxyFromModel converts a model to a response.
func StreamProxyFromModel(p *models.StreamProxy) StreamProxyResponse {
	return StreamProxyResponse{
		ID:                    p.ID,
		CreatedAt:             p.CreatedAt,
		UpdatedAt:             p.UpdatedAt,
		Name:                  p.Name,
		Description:           p.Description,
		ProxyMode:             p.ProxyMode,
		IsActive:              p.IsActive,
		AutoRegenerate:        p.AutoRegenerate,
		StartingChannelNumber: p.StartingChannelNumber,
		UpstreamTimeout:       p.UpstreamTimeout,
		BufferSize:            p.BufferSize,
		MaxConcurrentStreams:  p.MaxConcurrentStreams,
		CacheChannelLogos:     p.CacheChannelLogos,
		CacheProgramLogos:     p.CacheProgramLogos,
		RelayProfileID:        p.RelayProfileID,
		Status:                p.Status,
		LastGeneratedAt:       p.LastGeneratedAt,
		LastError:             p.LastError,
		ChannelCount:          p.ChannelCount,
		ProgramCount:          p.ProgramCount,
		OutputPath:            p.OutputPath,
	}
}

// StreamProxyDetailResponse includes related sources in the response.
type StreamProxyDetailResponse struct {
	StreamProxyResponse
	Sources    []StreamSourceResponse `json:"sources,omitempty"`
	EpgSources []EpgSourceResponse    `json:"epg_sources,omitempty"`
}

// StreamProxyDetailFromModel converts a model with relations to a detail response.
func StreamProxyDetailFromModel(p *models.StreamProxy) StreamProxyDetailResponse {
	resp := StreamProxyDetailResponse{
		StreamProxyResponse: StreamProxyFromModel(p),
	}
	for _, ps := range p.Sources {
		if ps.Source != nil {
			resp.Sources = append(resp.Sources, StreamSourceFromModel(ps.Source))
		}
	}
	for _, pes := range p.EpgSources {
		if pes.EpgSource != nil {
			resp.EpgSources = append(resp.EpgSources, EpgSourceFromModel(pes.EpgSource))
		}
	}
	return resp
}

// CreateStreamProxyRequest is the request body for creating a stream proxy.
type CreateStreamProxyRequest struct {
	Name                  string                 `json:"name" doc:"Unique name for the proxy" minLength:"1" maxLength:"255"`
	Description           string                 `json:"description,omitempty" doc:"Optional description" maxLength:"1024"`
	ProxyMode             models.StreamProxyMode `json:"proxy_mode,omitempty" doc:"How to serve streams: redirect, proxy, or relay" enum:"redirect,proxy,relay"`
	IsActive              *bool                  `json:"is_active,omitempty" doc:"Whether the proxy is active (default: true)"`
	AutoRegenerate        *bool                  `json:"auto_regenerate,omitempty" doc:"Auto-regenerate when sources change (default: false)"`
	StartingChannelNumber *int                   `json:"starting_channel_number,omitempty" doc:"Base channel number (default: 1)"`
	UpstreamTimeout       *int                   `json:"upstream_timeout,omitempty" doc:"Timeout in seconds for upstream connections"`
	BufferSize            *int                   `json:"buffer_size,omitempty" doc:"Buffer size in bytes for proxy mode"`
	MaxConcurrentStreams  *int                   `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams (0 = unlimited)"`
	CacheChannelLogos     *bool                  `json:"cache_channel_logos,omitempty" doc:"Cache channel logos locally"`
	CacheProgramLogos     *bool                  `json:"cache_program_logos,omitempty" doc:"Cache EPG program logos locally"`
	RelayProfileID        *models.ULID           `json:"relay_profile_id,omitempty" doc:"Relay profile for transcoding settings"`
	OutputPath            string                 `json:"output_path,omitempty" doc:"Path for generated files" maxLength:"512"`
	SourceIDs             []models.ULID          `json:"source_ids,omitempty" doc:"Stream source IDs to include"`
	EpgSourceIDs          []models.ULID          `json:"epg_source_ids,omitempty" doc:"EPG source IDs to include"`
}

// ToModel converts the request to a model.
func (r *CreateStreamProxyRequest) ToModel() *models.StreamProxy {
	proxy := &models.StreamProxy{
		Name:                  r.Name,
		Description:           r.Description,
		ProxyMode:             models.StreamProxyModeRedirect,
		IsActive:              true,
		AutoRegenerate:        false,
		StartingChannelNumber: 1,
		UpstreamTimeout:       30,
		BufferSize:            8192,
		MaxConcurrentStreams:  0,
		CacheChannelLogos:     false,
		CacheProgramLogos:     false,
		OutputPath:            r.OutputPath,
	}
	if r.ProxyMode != "" {
		proxy.ProxyMode = r.ProxyMode
	}
	if r.IsActive != nil {
		proxy.IsActive = *r.IsActive
	}
	if r.AutoRegenerate != nil {
		proxy.AutoRegenerate = *r.AutoRegenerate
	}
	if r.StartingChannelNumber != nil {
		proxy.StartingChannelNumber = *r.StartingChannelNumber
	}
	if r.UpstreamTimeout != nil {
		proxy.UpstreamTimeout = *r.UpstreamTimeout
	}
	if r.BufferSize != nil {
		proxy.BufferSize = *r.BufferSize
	}
	if r.MaxConcurrentStreams != nil {
		proxy.MaxConcurrentStreams = *r.MaxConcurrentStreams
	}
	if r.CacheChannelLogos != nil {
		proxy.CacheChannelLogos = *r.CacheChannelLogos
	}
	if r.CacheProgramLogos != nil {
		proxy.CacheProgramLogos = *r.CacheProgramLogos
	}
	if r.RelayProfileID != nil {
		proxy.RelayProfileID = r.RelayProfileID
	}
	return proxy
}

// UpdateStreamProxyRequest is the request body for updating a stream proxy.
type UpdateStreamProxyRequest struct {
	Name                  *string                 `json:"name,omitempty" doc:"Unique name for the proxy" maxLength:"255"`
	Description           *string                 `json:"description,omitempty" doc:"Optional description" maxLength:"1024"`
	ProxyMode             *models.StreamProxyMode `json:"proxy_mode,omitempty" doc:"How to serve streams" enum:"redirect,proxy,relay"`
	IsActive              *bool                   `json:"is_active,omitempty" doc:"Whether the proxy is active"`
	AutoRegenerate        *bool                   `json:"auto_regenerate,omitempty" doc:"Auto-regenerate when sources change"`
	StartingChannelNumber *int                    `json:"starting_channel_number,omitempty" doc:"Base channel number"`
	UpstreamTimeout       *int                    `json:"upstream_timeout,omitempty" doc:"Timeout in seconds for upstream connections"`
	BufferSize            *int                    `json:"buffer_size,omitempty" doc:"Buffer size in bytes for proxy mode"`
	MaxConcurrentStreams  *int                    `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams (0 = unlimited)"`
	CacheChannelLogos     *bool                   `json:"cache_channel_logos,omitempty" doc:"Cache channel logos locally"`
	CacheProgramLogos     *bool                   `json:"cache_program_logos,omitempty" doc:"Cache EPG program logos locally"`
	RelayProfileID        *models.ULID            `json:"relay_profile_id,omitempty" doc:"Relay profile for transcoding settings"`
	OutputPath            *string                 `json:"output_path,omitempty" doc:"Path for generated files" maxLength:"512"`
}

// ApplyToModel applies the update request to an existing model.
func (r *UpdateStreamProxyRequest) ApplyToModel(p *models.StreamProxy) {
	if r.Name != nil {
		p.Name = *r.Name
	}
	if r.Description != nil {
		p.Description = *r.Description
	}
	if r.ProxyMode != nil {
		p.ProxyMode = *r.ProxyMode
	}
	if r.IsActive != nil {
		p.IsActive = *r.IsActive
	}
	if r.AutoRegenerate != nil {
		p.AutoRegenerate = *r.AutoRegenerate
	}
	if r.StartingChannelNumber != nil {
		p.StartingChannelNumber = *r.StartingChannelNumber
	}
	if r.UpstreamTimeout != nil {
		p.UpstreamTimeout = *r.UpstreamTimeout
	}
	if r.BufferSize != nil {
		p.BufferSize = *r.BufferSize
	}
	if r.MaxConcurrentStreams != nil {
		p.MaxConcurrentStreams = *r.MaxConcurrentStreams
	}
	if r.CacheChannelLogos != nil {
		p.CacheChannelLogos = *r.CacheChannelLogos
	}
	if r.CacheProgramLogos != nil {
		p.CacheProgramLogos = *r.CacheProgramLogos
	}
	if r.RelayProfileID != nil {
		p.RelayProfileID = r.RelayProfileID
	}
	if r.OutputPath != nil {
		p.OutputPath = *r.OutputPath
	}
}

// SetProxySourcesRequest is the request body for setting proxy stream sources.
type SetProxySourcesRequest struct {
	SourceIDs  []models.ULID       `json:"source_ids" doc:"Stream source IDs to include"`
	Priorities map[models.ULID]int `json:"priorities,omitempty" doc:"Priority per source ID (higher = preferred)"`
}

// SetProxyEpgSourcesRequest is the request body for setting proxy EPG sources.
type SetProxyEpgSourcesRequest struct {
	EpgSourceIDs []models.ULID       `json:"epg_source_ids" doc:"EPG source IDs to include"`
	Priorities   map[models.ULID]int `json:"priorities,omitempty" doc:"Priority per source ID (higher = preferred)"`
}

// Health types

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  string            `json:"uptime"`
	Checks  map[string]string `json:"checks,omitempty"`
}

// Channel types

// ChannelResponse represents a channel in API responses.
type ChannelResponse struct {
	ID            models.ULID `json:"id"`
	SourceID      models.ULID `json:"source_id"`
	ExtID         string      `json:"ext_id,omitempty"`
	TvgID         string      `json:"tvg_id,omitempty"`
	TvgName       string      `json:"tvg_name,omitempty"`
	TvgLogo       string      `json:"tvg_logo,omitempty"`
	GroupTitle    string      `json:"group_title,omitempty"`
	ChannelName   string      `json:"channel_name"`
	ChannelNumber int         `json:"channel_number,omitempty"`
	StreamURL     string      `json:"stream_url"`
	StreamType    string      `json:"stream_type,omitempty"`
	Language      string      `json:"language,omitempty"`
	Country       string      `json:"country,omitempty"`
	IsAdult       bool        `json:"is_adult"`
}

// ChannelFromModel converts a model to a response.
func ChannelFromModel(c *models.Channel) ChannelResponse {
	return ChannelResponse{
		ID:            c.ID,
		SourceID:      c.SourceID,
		ExtID:         c.ExtID,
		TvgID:         c.TvgID,
		TvgName:       c.TvgName,
		TvgLogo:       c.TvgLogo,
		GroupTitle:    c.GroupTitle,
		ChannelName:   c.ChannelName,
		ChannelNumber: c.ChannelNumber,
		StreamURL:     c.StreamURL,
		StreamType:    c.StreamType,
		Language:      c.Language,
		Country:       c.Country,
		IsAdult:       c.IsAdult,
	}
}

// ChannelListResponse is the paginated response for channel listings.
type ChannelListResponse struct {
	Pagination PaginationMeta    `json:"pagination"`
	Channels   []ChannelResponse `json:"channels"`
}

// EPG Program types

// EpgProgramResponse represents an EPG program in API responses.
type EpgProgramResponse struct {
	ID          models.ULID `json:"id"`
	SourceID    models.ULID `json:"source_id"`
	ChannelID   string      `json:"channel_id"`
	Start       time.Time   `json:"start"`
	Stop        time.Time   `json:"stop"`
	Title       string      `json:"title"`
	SubTitle    string      `json:"sub_title,omitempty"`
	Description string      `json:"description,omitempty"`
	Category    string      `json:"category,omitempty"`
	Icon        string      `json:"icon,omitempty"`
	EpisodeNum  string      `json:"episode_num,omitempty"`
	Rating      string      `json:"rating,omitempty"`
	Language    string      `json:"language,omitempty"`
	IsNew       bool        `json:"is_new"`
	IsPremiere  bool        `json:"is_premiere"`
	IsLive      bool        `json:"is_live"`
}

// EpgProgramFromModel converts a model to a response.
func EpgProgramFromModel(p *models.EpgProgram) EpgProgramResponse {
	return EpgProgramResponse{
		ID:          p.ID,
		SourceID:    p.SourceID,
		ChannelID:   p.ChannelID,
		Start:       p.Start,
		Stop:        p.Stop,
		Title:       p.Title,
		SubTitle:    p.SubTitle,
		Description: p.Description,
		Category:    p.Category,
		Icon:        p.Icon,
		EpisodeNum:  p.EpisodeNum,
		Rating:      p.Rating,
		Language:    p.Language,
		IsNew:       p.IsNew,
		IsPremiere:  p.IsPremiere,
		IsLive:      p.IsLive,
	}
}

// EpgProgramListResponse is the paginated response for EPG program listings.
type EpgProgramListResponse struct {
	Pagination PaginationMeta       `json:"pagination"`
	Programs   []EpgProgramResponse `json:"programs"`
}

// Job types

// JobResponse represents a job in API responses.
type JobResponse struct {
	ID             models.ULID     `json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Type           models.JobType  `json:"type"`
	TargetID       models.ULID     `json:"target_id,omitempty"`
	TargetName     string          `json:"target_name,omitempty"`
	Status         models.JobStatus `json:"status"`
	CronSchedule   string          `json:"cron_schedule,omitempty"`
	NextRunAt      *time.Time      `json:"next_run_at,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	DurationMs     int64           `json:"duration_ms,omitempty"`
	AttemptCount   int             `json:"attempt_count"`
	MaxAttempts    int             `json:"max_attempts"`
	BackoffSeconds int             `json:"backoff_seconds"`
	LastError      string          `json:"last_error,omitempty"`
	Result         string          `json:"result,omitempty"`
	Priority       int             `json:"priority"`
	LockedBy       string          `json:"locked_by,omitempty"`
	LockedAt       *time.Time      `json:"locked_at,omitempty"`
}

// JobFromModel converts a job model to a response.
func JobFromModel(j *models.Job) JobResponse {
	resp := JobResponse{
		ID:             j.ID,
		CreatedAt:      j.CreatedAt,
		UpdatedAt:      j.UpdatedAt,
		Type:           j.Type,
		TargetID:       j.TargetID,
		TargetName:     j.TargetName,
		Status:         j.Status,
		CronSchedule:   j.CronSchedule,
		DurationMs:     j.DurationMs,
		AttemptCount:   j.AttemptCount,
		MaxAttempts:    j.MaxAttempts,
		BackoffSeconds: j.BackoffSeconds,
		LastError:      j.LastError,
		Result:         j.Result,
		Priority:       j.Priority,
		LockedBy:       j.LockedBy,
	}
	if j.NextRunAt != nil {
		t := time.Time(*j.NextRunAt)
		resp.NextRunAt = &t
	}
	if j.StartedAt != nil {
		t := time.Time(*j.StartedAt)
		resp.StartedAt = &t
	}
	if j.CompletedAt != nil {
		t := time.Time(*j.CompletedAt)
		resp.CompletedAt = &t
	}
	if j.LockedAt != nil {
		t := time.Time(*j.LockedAt)
		resp.LockedAt = &t
	}
	return resp
}

// JobHistoryResponse represents a job history record in API responses.
type JobHistoryResponse struct {
	ID            models.ULID     `json:"id"`
	CreatedAt     time.Time       `json:"created_at"`
	JobID         models.ULID     `json:"job_id"`
	Type          models.JobType  `json:"type"`
	TargetID      models.ULID     `json:"target_id,omitempty"`
	TargetName    string          `json:"target_name,omitempty"`
	Status        models.JobStatus `json:"status"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
	DurationMs    int64           `json:"duration_ms,omitempty"`
	AttemptNumber int             `json:"attempt_number"`
	Error         string          `json:"error,omitempty"`
	Result        string          `json:"result,omitempty"`
}

// JobHistoryFromModel converts a job history model to a response.
func JobHistoryFromModel(h *models.JobHistory) JobHistoryResponse {
	resp := JobHistoryResponse{
		ID:            h.ID,
		CreatedAt:     h.CreatedAt,
		JobID:         h.JobID,
		Type:          h.Type,
		TargetID:      h.TargetID,
		TargetName:    h.TargetName,
		Status:        h.Status,
		DurationMs:    h.DurationMs,
		AttemptNumber: h.AttemptNumber,
		Error:         h.Error,
		Result:        h.Result,
	}
	if h.StartedAt != nil {
		t := time.Time(*h.StartedAt)
		resp.StartedAt = &t
	}
	if h.CompletedAt != nil {
		t := time.Time(*h.CompletedAt)
		resp.CompletedAt = &t
	}
	return resp
}

// JobStatsResponse represents job statistics.
type JobStatsResponse struct {
	PendingCount   int64            `json:"pending_count"`
	RunningCount   int64            `json:"running_count"`
	CompletedCount int64            `json:"completed_count"`
	FailedCount    int64            `json:"failed_count"`
	ByType         map[string]int64 `json:"by_type"`
}

// RunnerStatusResponse represents the job runner status.
type RunnerStatusResponse struct {
	Running      bool   `json:"running"`
	WorkerCount  int    `json:"worker_count"`
	WorkerID     string `json:"worker_id"`
	PendingJobs  int64  `json:"pending_jobs"`
	RunningJobs  int64  `json:"running_jobs"`
	PollInterval string `json:"poll_interval"`
}

// ValidateCronRequest is the request body for validating cron expressions.
type ValidateCronRequest struct {
	Expression string `json:"expression" doc:"Cron expression to validate" minLength:"1"`
}

// ValidateCronResponse is the response for cron validation.
type ValidateCronResponse struct {
	Valid   bool       `json:"valid"`
	Error   string     `json:"error,omitempty"`
	NextRun *time.Time `json:"next_run,omitempty"`
}
