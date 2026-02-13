// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/scheduler"
)

// Common response types

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
	ID                   models.ULID         `json:"id"`
	CreatedAt            time.Time           `json:"created_at"`
	UpdatedAt            time.Time           `json:"updated_at"`
	Name                 string              `json:"name"`
	Type                 models.SourceType   `json:"type"`
	URL                  string              `json:"url"`
	Username             string              `json:"username,omitempty"`
	UserAgent            string              `json:"user_agent,omitempty"`
	Enabled              bool                `json:"enabled"`
	Priority             int                 `json:"priority"`
	MaxConcurrentStreams int                 `json:"max_concurrent_streams"`
	Status               models.SourceStatus `json:"status"`
	LastIngestionAt      *time.Time          `json:"last_ingestion_at,omitempty"`
	LastError            string              `json:"last_error,omitempty"`
	ChannelCount         int                 `json:"channel_count"`
	CronSchedule         string              `json:"cron_schedule,omitempty"`
	NextScheduledUpdate  *time.Time          `json:"next_scheduled_update,omitempty"`
}

// StreamSourceFromModel converts a model to a response.
func StreamSourceFromModel(s *models.StreamSource) StreamSourceResponse {
	return StreamSourceResponse{
		ID:                   s.ID,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
		Name:                 s.Name,
		Type:                 s.Type,
		URL:                  s.URL,
		Username:             s.Username,
		UserAgent:            s.UserAgent,
		Enabled:              models.BoolVal(s.Enabled),
		Priority:             s.Priority,
		MaxConcurrentStreams: s.MaxConcurrentStreams,
		Status:               s.Status,
		LastIngestionAt:      s.LastIngestionAt,
		LastError:            s.LastError,
		ChannelCount:         s.ChannelCount,
		CronSchedule:         s.CronSchedule,
		NextScheduledUpdate:  scheduler.CalculateNextRun(s.CronSchedule),
	}
}

// CreateStreamSourceRequest is the request body for creating a stream source.
type CreateStreamSourceRequest struct {
	Name                 string            `json:"name" doc:"User-friendly name for the source" minLength:"1" maxLength:"255"`
	Type                 models.SourceType `json:"type" doc:"Source type: m3u or xtream" enum:"m3u,xtream"`
	URL                  string            `json:"url" doc:"M3U playlist URL or Xtream server URL" minLength:"1" maxLength:"2048"`
	Username             string            `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password             string            `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent            string            `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled              *bool             `json:"enabled,omitempty" doc:"Whether the source is enabled (default: true)"`
	Priority             *int              `json:"priority,omitempty" doc:"Priority for channel merging (higher = preferred)"`
	MaxConcurrentStreams *int              `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams from this source (0 = unlimited, default: 1)"`
	CronSchedule         string            `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
}

// ToModel converts the request to a model.
func (r *CreateStreamSourceRequest) ToModel() *models.StreamSource {
	source := &models.StreamSource{
		Name:                 r.Name,
		Type:                 r.Type,
		URL:                  r.URL,
		Username:             r.Username,
		Password:             r.Password,
		UserAgent:            r.UserAgent,
		Enabled:              models.BoolPtr(true),
		Priority:             0,
		MaxConcurrentStreams: 1, // Default: 1 concurrent stream
		CronSchedule:         r.CronSchedule,
	}
	if r.Enabled != nil {
		source.Enabled = r.Enabled
	}
	if r.Priority != nil {
		source.Priority = *r.Priority
	}
	if r.MaxConcurrentStreams != nil {
		source.MaxConcurrentStreams = *r.MaxConcurrentStreams
	}
	return source
}

// UpdateStreamSourceRequest is the request body for updating a stream source.
type UpdateStreamSourceRequest struct {
	Name                 *string            `json:"name,omitempty" doc:"User-friendly name for the source" maxLength:"255"`
	Type                 *models.SourceType `json:"type,omitempty" doc:"Source type: m3u or xtream" enum:"m3u,xtream"`
	URL                  *string            `json:"url,omitempty" doc:"M3U playlist URL or Xtream server URL" maxLength:"2048"`
	Username             *string            `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password             *string            `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	UserAgent            *string            `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	Enabled              *bool              `json:"enabled,omitempty" doc:"Whether the source is enabled"`
	Priority             *int               `json:"priority,omitempty" doc:"Priority for channel merging"`
	MaxConcurrentStreams *int               `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams from this source (0 = unlimited)"`
	CronSchedule         *string            `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
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
		s.Enabled = r.Enabled
	}
	if r.Priority != nil {
		s.Priority = *r.Priority
	}
	if r.MaxConcurrentStreams != nil {
		s.MaxConcurrentStreams = *r.MaxConcurrentStreams
	}
	if r.CronSchedule != nil {
		s.CronSchedule = *r.CronSchedule
	}
}

// EPG Source types

// EpgSourceResponse represents an EPG source in API responses.
type EpgSourceResponse struct {
	ID                  models.ULID            `json:"id"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
	Name                string                 `json:"name"`
	Type                models.EpgSourceType   `json:"type"`
	URL                 string                 `json:"url"`
	Username            string                 `json:"username,omitempty"`
	ApiMethod           models.XtreamApiMethod `json:"api_method,omitempty"`
	UserAgent           string                 `json:"user_agent,omitempty"`
	DetectedTimezone    string                 `json:"detected_timezone,omitempty"`
	EpgShift            int                    `json:"epg_shift"`
	Enabled             bool                   `json:"enabled"`
	Priority            int                    `json:"priority"`
	Status              models.EpgSourceStatus `json:"status"`
	LastIngestionAt     *time.Time             `json:"last_ingestion_at,omitempty"`
	LastError           string                 `json:"last_error,omitempty"`
	ProgramCount        int                    `json:"program_count"`
	CronSchedule        string                 `json:"cron_schedule,omitempty"`
	RetentionDays       int                    `json:"retention_days"`
	NextScheduledUpdate *time.Time             `json:"next_scheduled_update,omitempty"`
}

// EpgSourceFromModel converts a model to a response.
func EpgSourceFromModel(s *models.EpgSource) EpgSourceResponse {
	return EpgSourceResponse{
		ID:                  s.ID,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
		Name:                s.Name,
		Type:                s.Type,
		URL:                 s.URL,
		Username:            s.Username,
		ApiMethod:           s.ApiMethod,
		UserAgent:           s.UserAgent,
		DetectedTimezone:    s.DetectedTimezone,
		EpgShift:            s.EpgShift,
		Enabled:             models.BoolVal(s.Enabled),
		Priority:            s.Priority,
		Status:              s.Status,
		LastIngestionAt:     s.LastIngestionAt,
		LastError:           s.LastError,
		ProgramCount:        s.ProgramCount,
		CronSchedule:        s.CronSchedule,
		RetentionDays:       s.RetentionDays,
		NextScheduledUpdate: scheduler.CalculateNextRun(s.CronSchedule),
	}
}

// CreateEpgSourceRequest is the request body for creating an EPG source.
type CreateEpgSourceRequest struct {
	Name          string                 `json:"name" doc:"User-friendly name for the source" minLength:"1" maxLength:"255"`
	Type          models.EpgSourceType   `json:"type" doc:"Source type: xmltv or xtream" enum:"xmltv,xtream"`
	URL           string                 `json:"url" doc:"XMLTV URL or Xtream server URL" minLength:"1" maxLength:"2048"`
	Username      string                 `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password      string                 `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	ApiMethod     models.XtreamApiMethod `json:"api_method,omitempty" doc:"API method for Xtream sources: stream_id (richer data, ~6 days) or bulk_xmltv (faster, ~2 days)" enum:"stream_id,bulk_xmltv"`
	UserAgent     string                 `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	EpgShift      *int                   `json:"epg_shift,omitempty" doc:"Manual time shift in hours to adjust EPG times (default: 0)" minimum:"-12" maximum:"12"`
	Enabled       *bool                  `json:"enabled,omitempty" doc:"Whether the source is enabled (default: true)"`
	Priority      *int                   `json:"priority,omitempty" doc:"Priority for program merging (higher = preferred)"`
	CronSchedule  string                 `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
	RetentionDays *int                   `json:"retention_days,omitempty" doc:"Days to retain EPG data after expiry (default: 1)"`
}

// ToModel converts the request to a model.
func (r *CreateEpgSourceRequest) ToModel() *models.EpgSource {
	source := &models.EpgSource{
		Name:          r.Name,
		Type:          r.Type,
		URL:           r.URL,
		Username:      r.Username,
		Password:      r.Password,
		ApiMethod:     r.ApiMethod,
		UserAgent:     r.UserAgent,
		EpgShift:      0,
		Enabled:       models.BoolPtr(true),
		Priority:      0,
		CronSchedule:  r.CronSchedule,
		RetentionDays: 1,
	}
	if r.EpgShift != nil {
		source.EpgShift = *r.EpgShift
	}
	if r.Enabled != nil {
		source.Enabled = r.Enabled
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
// Note: DetectedTimezone is read-only (auto-detected during ingestion)
type UpdateEpgSourceRequest struct {
	Name          *string                 `json:"name,omitempty" doc:"User-friendly name for the source" maxLength:"255"`
	Type          *models.EpgSourceType   `json:"type,omitempty" doc:"Source type: xmltv or xtream" enum:"xmltv,xtream"`
	URL           *string                 `json:"url,omitempty" doc:"XMLTV URL or Xtream server URL" maxLength:"2048"`
	Username      *string                 `json:"username,omitempty" doc:"Username for Xtream authentication" maxLength:"255"`
	Password      *string                 `json:"password,omitempty" doc:"Password for Xtream authentication" maxLength:"255"`
	ApiMethod     *models.XtreamApiMethod `json:"api_method,omitempty" doc:"API method for Xtream sources: stream_id or bulk_xmltv" enum:"stream_id,bulk_xmltv"`
	UserAgent     *string                 `json:"user_agent,omitempty" doc:"Custom User-Agent header" maxLength:"512"`
	EpgShift      *int                    `json:"epg_shift,omitempty" doc:"Manual time shift in hours to adjust EPG times" minimum:"-12" maximum:"12"`
	Enabled       *bool                   `json:"enabled,omitempty" doc:"Whether the source is enabled"`
	Priority      *int                    `json:"priority,omitempty" doc:"Priority for program merging"`
	CronSchedule  *string                 `json:"cron_schedule,omitempty" doc:"Cron schedule for automatic ingestion" maxLength:"100"`
	RetentionDays *int                    `json:"retention_days,omitempty" doc:"Days to retain EPG data after expiry"`
}

// ApplyToModel applies the update request to an existing model.
// Note: DetectedTimezone is not updated here - it's auto-detected during ingestion
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
	if r.ApiMethod != nil {
		s.ApiMethod = *r.ApiMethod
	}
	if r.UserAgent != nil {
		s.UserAgent = *r.UserAgent
	}
	if r.EpgShift != nil {
		s.EpgShift = *r.EpgShift
	}
	if r.Enabled != nil {
		s.Enabled = r.Enabled
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
	EncodingProfileID     *models.ULID             `json:"encoding_profile_id,omitempty"`
	Status                models.StreamProxyStatus `json:"status"`
	LastGeneratedAt       *time.Time               `json:"last_generated_at,omitempty"`
	LastError             string                   `json:"last_error,omitempty"`
	ChannelCount          int                      `json:"channel_count"`
	ProgramCount          int                      `json:"program_count"`
	OutputPath            string                   `json:"output_path,omitempty"`
	M3U8URL               string                   `json:"m3u8_url,omitempty"`
	XMLTVURL              string                   `json:"xmltv_url,omitempty"`
}

// StreamProxyFromModel converts a model to a response.
// The baseURL parameter is optional; if empty, relative URLs are used.
func StreamProxyFromModel(p *models.StreamProxy, baseURL string) StreamProxyResponse {
	resp := StreamProxyResponse{
		ID:                    p.ID,
		CreatedAt:             p.CreatedAt,
		UpdatedAt:             p.UpdatedAt,
		Name:                  p.Name,
		Description:           p.Description,
		ProxyMode:             p.ProxyMode,
		IsActive:              models.BoolVal(p.IsActive),
		AutoRegenerate:        p.AutoRegenerate,
		StartingChannelNumber: p.StartingChannelNumber,
		UpstreamTimeout:       p.UpstreamTimeout,
		BufferSize:            p.BufferSize,
		MaxConcurrentStreams:  p.MaxConcurrentStreams,
		CacheChannelLogos:     p.CacheChannelLogos,
		CacheProgramLogos:     p.CacheProgramLogos,
		EncodingProfileID:     p.EncodingProfileID,
		Status:                p.Status,
		LastGeneratedAt:       p.LastGeneratedAt,
		LastError:             p.LastError,
		ChannelCount:          p.ChannelCount,
		ProgramCount:          p.ProgramCount,
		OutputPath:            p.OutputPath,
	}

	// Only populate URLs if the proxy has been generated (has a last_generated_at timestamp)
	if p.LastGeneratedAt != nil {
		idStr := p.ID.String()
		if baseURL != "" {
			resp.M3U8URL = fmt.Sprintf("%s/proxy/%s.m3u", baseURL, idStr)
			resp.XMLTVURL = fmt.Sprintf("%s/proxy/%s.xmltv", baseURL, idStr)
		} else {
			resp.M3U8URL = fmt.Sprintf("/proxy/%s.m3u", idStr)
			resp.XMLTVURL = fmt.Sprintf("/proxy/%s.xmltv", idStr)
		}
	}

	return resp
}

// ProxySourceAssignmentResponse represents a stream source assignment in a proxy.
// This matches the frontend's expected format with source_id and priority_order.
type ProxySourceAssignmentResponse struct {
	SourceID      string `json:"source_id" doc:"ID of the stream source"`
	SourceName    string `json:"source_name" doc:"Name of the stream source"`
	PriorityOrder int    `json:"priority_order" doc:"Priority for merging channels"`
}

// ProxyEpgSourceAssignmentResponse represents an EPG source assignment in a proxy.
// This matches the frontend's expected format with epg_source_id and priority_order.
type ProxyEpgSourceAssignmentResponse struct {
	EpgSourceID   string `json:"epg_source_id" doc:"ID of the EPG source"`
	EpgSourceName string `json:"epg_source_name" doc:"Name of the EPG source"`
	PriorityOrder int    `json:"priority_order" doc:"Priority for merging EPG data"`
}

// ProxyFilterAssignmentRequest represents a filter assignment in a proxy request.
// This matches the frontend's format with filter_id, priority_order, and is_active.
type ProxyFilterAssignmentRequest struct {
	FilterID      models.ULID `json:"filter_id" doc:"ID of the filter"`
	PriorityOrder int         `json:"priority_order" doc:"Order in which filters are applied"`
	IsActive      bool        `json:"is_active" doc:"Whether the filter is active"`
}

// ProxyFilterAssignmentResponse represents a filter assignment in a proxy.
// This matches the frontend's expected format with filter_id and priority_order.
type ProxyFilterAssignmentResponse struct {
	FilterID      string `json:"filter_id" doc:"ID of the filter"`
	FilterName    string `json:"filter_name" doc:"Name of the filter"`
	PriorityOrder int    `json:"priority_order" doc:"Order in which filters are applied"`
	IsActive      bool   `json:"is_active" doc:"Whether the filter is active"`
}

// StreamProxyDetailResponse includes related sources and filters in the response.
// Field names match the original Rust API contract for frontend compatibility.
type StreamProxyDetailResponse struct {
	StreamProxyResponse
	StreamSources []ProxySourceAssignmentResponse    `json:"stream_sources,omitempty"`
	EpgSources    []ProxyEpgSourceAssignmentResponse `json:"epg_sources,omitempty"`
	Filters       []ProxyFilterAssignmentResponse    `json:"filters,omitempty"`
}

// StreamProxyDetailFromModel converts a model with relations to a detail response.
// The baseURL parameter is optional; if empty, relative URLs are used.
func StreamProxyDetailFromModel(p *models.StreamProxy, baseURL string) StreamProxyDetailResponse {
	resp := StreamProxyDetailResponse{
		StreamProxyResponse: StreamProxyFromModel(p, baseURL),
		StreamSources:       make([]ProxySourceAssignmentResponse, 0, len(p.Sources)),
		EpgSources:          make([]ProxyEpgSourceAssignmentResponse, 0, len(p.EpgSources)),
		Filters:             make([]ProxyFilterAssignmentResponse, 0, len(p.Filters)),
	}
	for _, ps := range p.Sources {
		sourceName := ""
		if ps.Source != nil {
			sourceName = ps.Source.Name
		}
		resp.StreamSources = append(resp.StreamSources, ProxySourceAssignmentResponse{
			SourceID:      ps.SourceID.String(),
			SourceName:    sourceName,
			PriorityOrder: ps.Priority,
		})
	}
	for _, pes := range p.EpgSources {
		epgSourceName := ""
		if pes.EpgSource != nil {
			epgSourceName = pes.EpgSource.Name
		}
		resp.EpgSources = append(resp.EpgSources, ProxyEpgSourceAssignmentResponse{
			EpgSourceID:   pes.EpgSourceID.String(),
			EpgSourceName: epgSourceName,
			PriorityOrder: pes.Priority,
		})
	}
	for _, pf := range p.Filters {
		filterName := ""
		if pf.Filter != nil {
			filterName = pf.Filter.Name
		}
		// Default to true if IsActive pointer is nil
		isActive := pf.IsActive == nil || *pf.IsActive
		resp.Filters = append(resp.Filters, ProxyFilterAssignmentResponse{
			FilterID:      pf.FilterID.String(),
			FilterName:    filterName,
			PriorityOrder: pf.Priority,
			IsActive:      isActive,
		})
	}
	return resp
}

// CreateStreamProxyRequest is the request body for creating a stream proxy.
type CreateStreamProxyRequest struct {
	Name                  string                         `json:"name" doc:"Unique name for the proxy" minLength:"1" maxLength:"255"`
	Description           string                         `json:"description,omitempty" doc:"Optional description" maxLength:"1024"`
	ProxyMode             models.StreamProxyMode         `json:"proxy_mode,omitempty" doc:"How to serve streams: direct (302 redirect) or smart (auto-optimize)" enum:"direct,smart"`
	IsActive              *bool                          `json:"is_active,omitempty" doc:"Whether the proxy is active (default: true)"`
	AutoRegenerate        *bool                          `json:"auto_regenerate,omitempty" doc:"Auto-regenerate when sources change (default: false)"`
	StartingChannelNumber *int                           `json:"starting_channel_number,omitempty" doc:"Base channel number (default: 1)"`
	NumberingMode         *models.NumberingMode          `json:"numbering_mode,omitempty" doc:"How to assign channel numbers: sequential, preserve, or group" enum:"sequential,preserve,group"`
	GroupNumberingSize    *int                           `json:"group_numbering_size,omitempty" doc:"Size of each group range when using group numbering mode (default: 100)"`
	UpstreamTimeout       *int                           `json:"upstream_timeout,omitempty" doc:"Timeout in seconds for upstream connections"`
	BufferSize            *int                           `json:"buffer_size,omitempty" doc:"Buffer size in bytes for proxy mode"`
	MaxConcurrentStreams  *int                           `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams (0 = unlimited)"`
	CacheChannelLogos     *bool                          `json:"cache_channel_logos,omitempty" doc:"Cache channel logos locally"`
	CacheProgramLogos     *bool                          `json:"cache_program_logos,omitempty" doc:"Cache EPG program logos locally"`
	EncodingProfileID     *models.ULID                   `json:"encoding_profile_id,omitempty" doc:"Fallback encoding profile when no client detection rule matches"`
	OutputPath            string                         `json:"output_path,omitempty" doc:"Path for generated files" maxLength:"512"`
	SourceIDs             []models.ULID                  `json:"source_ids,omitempty" doc:"Stream source IDs to include"`
	EpgSourceIDs          []models.ULID                  `json:"epg_source_ids,omitempty" doc:"EPG source IDs to include"`
	FilterIDs             []models.ULID                  `json:"filter_ids,omitempty" doc:"Filter IDs to include (deprecated, use filters)"`
	Filters               []ProxyFilterAssignmentRequest `json:"filters,omitempty" doc:"Filter assignments with priority and active state"`
}

// ToModel converts the request to a model.
func (r *CreateStreamProxyRequest) ToModel() *models.StreamProxy {
	proxy := &models.StreamProxy{
		Name:                  r.Name,
		Description:           r.Description,
		ProxyMode:             models.StreamProxyModeDirect, // Default
		IsActive:              models.BoolPtr(true),
		AutoRegenerate:        false,
		StartingChannelNumber: 1,
		NumberingMode:         models.NumberingModePreserve, // Default
		GroupNumberingSize:    100,                          // Default
		UpstreamTimeout:       30,
		BufferSize:            8192,
		MaxConcurrentStreams:  0,
		CacheChannelLogos:     true,
		CacheProgramLogos:     true,
		OutputPath:            r.OutputPath,
	}
	if r.ProxyMode != "" && models.IsValidProxyMode(r.ProxyMode) {
		proxy.ProxyMode = r.ProxyMode
	}
	if r.IsActive != nil {
		proxy.IsActive = r.IsActive
	}
	if r.AutoRegenerate != nil {
		proxy.AutoRegenerate = *r.AutoRegenerate
	}
	if r.StartingChannelNumber != nil {
		proxy.StartingChannelNumber = *r.StartingChannelNumber
	}
	if r.NumberingMode != nil && models.IsValidNumberingMode(*r.NumberingMode) {
		proxy.NumberingMode = *r.NumberingMode
	}
	if r.GroupNumberingSize != nil {
		proxy.GroupNumberingSize = *r.GroupNumberingSize
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
	if r.EncodingProfileID != nil {
		proxy.EncodingProfileID = r.EncodingProfileID
	}
	return proxy
}

// UpdateStreamProxyRequest is the request body for updating a stream proxy.
type UpdateStreamProxyRequest struct {
	Name                  *string                        `json:"name,omitempty" doc:"Unique name for the proxy" maxLength:"255"`
	Description           *string                        `json:"description,omitempty" doc:"Optional description" maxLength:"1024"`
	ProxyMode             *models.StreamProxyMode        `json:"proxy_mode,omitempty" doc:"How to serve streams: direct (302 redirect) or smart (auto-optimize)" enum:"direct,smart"`
	IsActive              *bool                          `json:"is_active,omitempty" doc:"Whether the proxy is active"`
	AutoRegenerate        *bool                          `json:"auto_regenerate,omitempty" doc:"Auto-regenerate when sources change"`
	StartingChannelNumber *int                           `json:"starting_channel_number,omitempty" doc:"Base channel number"`
	NumberingMode         *models.NumberingMode          `json:"numbering_mode,omitempty" doc:"How to assign channel numbers: sequential, preserve, or group" enum:"sequential,preserve,group"`
	GroupNumberingSize    *int                           `json:"group_numbering_size,omitempty" doc:"Size of each group range when using group numbering mode"`
	UpstreamTimeout       *int                           `json:"upstream_timeout,omitempty" doc:"Timeout in seconds for upstream connections"`
	BufferSize            *int                           `json:"buffer_size,omitempty" doc:"Buffer size in bytes for proxy mode"`
	MaxConcurrentStreams  *int                           `json:"max_concurrent_streams,omitempty" doc:"Max concurrent streams (0 = unlimited)"`
	CacheChannelLogos     *bool                          `json:"cache_channel_logos,omitempty" doc:"Cache channel logos locally"`
	CacheProgramLogos     *bool                          `json:"cache_program_logos,omitempty" doc:"Cache EPG program logos locally"`
	EncodingProfileID     *models.ULID                   `json:"encoding_profile_id,omitempty" doc:"Fallback encoding profile when no client detection rule matches"`
	OutputPath            *string                        `json:"output_path,omitempty" doc:"Path for generated files" maxLength:"512"`
	SourceIDs             []models.ULID                  `json:"source_ids,omitempty" doc:"Stream source IDs to include"`
	EpgSourceIDs          []models.ULID                  `json:"epg_source_ids,omitempty" doc:"EPG source IDs to include"`
	FilterIDs             []models.ULID                  `json:"filter_ids,omitempty" doc:"Filter IDs to include (deprecated, use filters)"`
	Filters               []ProxyFilterAssignmentRequest `json:"filters,omitempty" doc:"Filter assignments with priority and active state"`
}

// ApplyToModel applies the update request to an existing model.
func (r *UpdateStreamProxyRequest) ApplyToModel(p *models.StreamProxy) {
	if r.Name != nil {
		p.Name = *r.Name
	}
	if r.Description != nil {
		p.Description = *r.Description
	}
	if r.ProxyMode != nil && models.IsValidProxyMode(*r.ProxyMode) {
		p.ProxyMode = *r.ProxyMode
	}
	if r.IsActive != nil {
		p.IsActive = r.IsActive
	}
	if r.AutoRegenerate != nil {
		p.AutoRegenerate = *r.AutoRegenerate
	}
	if r.StartingChannelNumber != nil {
		p.StartingChannelNumber = *r.StartingChannelNumber
	}
	if r.NumberingMode != nil && models.IsValidNumberingMode(*r.NumberingMode) {
		p.NumberingMode = *r.NumberingMode
	}
	if r.GroupNumberingSize != nil {
		p.GroupNumberingSize = *r.GroupNumberingSize
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
	if r.EncodingProfileID != nil {
		p.EncodingProfileID = r.EncodingProfileID
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

// HealthResponse represents the comprehensive health check response.
type HealthResponse struct {
	Status        string            `json:"status"`
	Timestamp     string            `json:"timestamp"`
	Version       string            `json:"version"`
	Uptime        string            `json:"uptime"`
	UptimeSeconds float64           `json:"uptime_seconds"`
	SystemLoad    float64           `json:"system_load"`
	CPUInfo       CPUInfo           `json:"cpu_info"`
	Memory        MemoryInfo        `json:"memory"`
	Components    HealthComponents  `json:"components"`
	Checks        map[string]string `json:"checks,omitempty"`
}

// CPUInfo contains CPU load information.
type CPUInfo struct {
	Cores              int     `json:"cores"`
	Load1Min           float64 `json:"load_1min"`
	Load5Min           float64 `json:"load_5min"`
	Load15Min          float64 `json:"load_15min"`
	LoadPercentage1Min float64 `json:"load_percentage_1min"`
}

// MemoryInfo contains memory usage information.
type MemoryInfo struct {
	TotalMemoryMB     float64           `json:"total_memory_mb"`
	UsedMemoryMB      float64           `json:"used_memory_mb"`
	FreeMemoryMB      float64           `json:"free_memory_mb"`
	AvailableMemoryMB float64           `json:"available_memory_mb"`
	SwapUsedMB        float64           `json:"swap_used_mb"`
	SwapTotalMB       float64           `json:"swap_total_mb"`
	ProcessMemory     ProcessMemoryInfo `json:"process_memory"`
}

// ProcessMemoryInfo contains process-specific memory information.
type ProcessMemoryInfo struct {
	MainProcessMB      float64 `json:"main_process_mb"`
	ChildProcessesMB   float64 `json:"child_processes_mb"`
	TotalProcessTreeMB float64 `json:"total_process_tree_mb"`
	PercentageOfSystem float64 `json:"percentage_of_system"`
	ChildProcessCount  int     `json:"child_process_count"`
}

// HealthComponents contains health status of various components.
type HealthComponents struct {
	Database        DatabaseHealth                  `json:"database"`
	Scheduler       SchedulerHealth                 `json:"scheduler"`
	CircuitBreakers map[string]CircuitBreakerStatus `json:"circuit_breakers"`
}

// DatabaseHealth contains database health information.
type DatabaseHealth struct {
	Status                 string  `json:"status"`
	ConnectionPoolSize     int     `json:"connection_pool_size"`
	ActiveConnections      int     `json:"active_connections"`
	IdleConnections        int     `json:"idle_connections"`
	PoolUtilizationPercent float64 `json:"pool_utilization_percent"`
	ResponseTimeMS         float64 `json:"response_time_ms"`
	ResponseTimeStatus     string  `json:"response_time_status"`
	TablesAccessible       bool    `json:"tables_accessible"`
	WriteCapability        bool    `json:"write_capability"`
	NoBlockingLocks        bool    `json:"no_blocking_locks"`
}

// SchedulerHealth contains scheduler health information.
type SchedulerHealth struct {
	Status           string           `json:"status"`
	SourcesScheduled SourcesScheduled `json:"sources_scheduled"`
}

// SourcesScheduled contains counts of scheduled sources.
type SourcesScheduled struct {
	StreamSources int `json:"stream_sources"`
	EpgSources    int `json:"epg_sources"`
}

// CircuitBreakerStatus represents the status of a circuit breaker.
type CircuitBreakerStatus struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	Failures        int     `json:"failures"`
	SuccessfulCalls int64   `json:"successful_calls"`
	FailedCalls     int64   `json:"failed_calls"`
	TotalCalls      int64   `json:"total_calls"`
	FailureRate     float64 `json:"failure_rate"`
}

// Channel types

// ChannelResponse represents a channel in API responses.
type ChannelResponse struct {
	ID            models.ULID `json:"id"`
	SourceID      models.ULID `json:"source_id"`
	SourceName    string      `json:"source_name,omitempty"`
	ExtID         string      `json:"ext_id,omitempty"`
	TvgID         string      `json:"tvg_id,omitempty"`
	TvgName       string      `json:"tvg_name,omitempty"`
	TvgChno       string      `json:"tvg_chno,omitempty"`
	TvgLogo       string      `json:"tvg_logo,omitempty"`
	LogoURL       string      `json:"logo_url,omitempty"`
	Group         string      `json:"group,omitempty"`
	Name          string      `json:"name"`
	ChannelNumber int         `json:"channel_number,omitempty"`
	StreamURL     string      `json:"stream_url"`
	StreamType    string      `json:"stream_type,omitempty"`
	Language      string      `json:"language,omitempty"`
	Country       string      `json:"country,omitempty"`
	IsAdult       bool        `json:"is_adult"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`

	// Codec/probe info from last_known_codecs table (optional, populated when available)
	VideoCodec      string     `json:"video_codec,omitempty"`
	VideoWidth      int        `json:"video_width,omitempty"`
	VideoHeight     int        `json:"video_height,omitempty"`
	VideoFramerate  float64    `json:"video_framerate,omitempty"`
	AudioCodec      string     `json:"audio_codec,omitempty"`
	AudioChannels   int        `json:"audio_channels,omitempty"`
	AudioSampleRate int        `json:"audio_sample_rate,omitempty"`
	ContainerFormat string     `json:"container_format,omitempty"`
	IsLiveStream    bool       `json:"is_live_stream,omitempty"`
	LastProbedAt    *time.Time `json:"last_probed_at,omitempty"`
}

// ChannelFromModel converts a model to a response.
func ChannelFromModel(c *models.Channel) ChannelResponse {
	// Convert channel number to string for tvg_chno field
	tvgChno := ""
	if c.ChannelNumber > 0 {
		tvgChno = fmt.Sprintf("%d", c.ChannelNumber)
	}

	return ChannelResponse{
		ID:            c.ID,
		SourceID:      c.SourceID,
		ExtID:         c.ExtID,
		TvgID:         c.TvgID,
		TvgName:       c.TvgName,
		TvgChno:       tvgChno,
		TvgLogo:       c.TvgLogo,
		LogoURL:       c.TvgLogo, // Use TvgLogo as LogoURL
		Group:         c.GroupTitle,
		Name:          c.ChannelName,
		ChannelNumber: c.ChannelNumber,
		StreamURL:     c.StreamURL,
		StreamType:    c.StreamType,
		Language:      c.Language,
		Country:       c.Country,
		IsAdult:       c.IsAdult,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

// ChannelResponseWithCodec populates codec fields from LastKnownCodec into a ChannelResponse.
func (r *ChannelResponse) PopulateCodecInfo(codec *models.LastKnownCodec) {
	if codec == nil {
		return
	}
	r.VideoCodec = codec.VideoCodec
	r.VideoWidth = codec.VideoWidth
	r.VideoHeight = codec.VideoHeight
	r.VideoFramerate = codec.VideoFramerate
	r.AudioCodec = codec.AudioCodec
	r.AudioChannels = codec.AudioChannels
	r.AudioSampleRate = codec.AudioSampleRate
	r.ContainerFormat = codec.ContainerFormat
	r.IsLiveStream = codec.IsLiveStream
	probedAt := time.Time(codec.ProbedAt)
	r.LastProbedAt = &probedAt
}

// ChannelListResponse is the paginated response for channel listings.
type ChannelListResponse struct {
	Pagination PaginationMeta    `json:"pagination"`
	Channels   []ChannelResponse `json:"channels"`
}

// EPG Program types

// EpgProgramResponse represents an EPG program in API responses.
type EpgProgramResponse struct {
	ID          models.ULID   `json:"id"`
	SourceID    models.ULID   `json:"source_id"`
	ChannelID   string        `json:"channel_id"`
	Start       time.Time     `json:"start"`
	Stop        time.Time     `json:"stop"`
	Duration    time.Duration `json:"duration"`
	Title       string        `json:"title"`
	SubTitle    string        `json:"sub_title,omitempty"`
	Description string        `json:"description,omitempty"`
	Category    string        `json:"category,omitempty"`
	Icon        string        `json:"icon,omitempty"`
	EpisodeNum  string        `json:"episode_num,omitempty"`
	Rating      string        `json:"rating,omitempty"`
	Language    string        `json:"language,omitempty"`
	IsNew       bool          `json:"is_new"`
	IsPremiere  bool          `json:"is_premiere"`
	IsLive      bool          `json:"is_live"`
	IsOnAir     bool          `json:"is_on_air"`
	HasEnded    bool          `json:"has_ended"`
	CreatedAt   time.Time     `json:"created_at"`
}

// EpgProgramFromModel converts a model to a response.
func EpgProgramFromModel(p *models.EpgProgram) EpgProgramResponse {
	return EpgProgramResponse{
		ID:          p.ID,
		SourceID:    p.SourceID,
		ChannelID:   p.ChannelID,
		Start:       p.Start,
		Stop:        p.Stop,
		Duration:    p.Duration(),
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
		IsOnAir:     p.IsOnAir(),
		HasEnded:    p.HasEnded(),
		CreatedAt:   p.CreatedAt,
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
	ID             models.ULID      `json:"id"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Type           models.JobType   `json:"type"`
	TargetID       models.ULID      `json:"target_id,omitempty"`
	TargetName     string           `json:"target_name,omitempty"`
	Status         models.JobStatus `json:"status"`
	CronSchedule   string           `json:"cron_schedule,omitempty"`
	NextRunAt      *time.Time       `json:"next_run_at,omitempty"`
	StartedAt      *time.Time       `json:"started_at,omitempty"`
	CompletedAt    *time.Time       `json:"completed_at,omitempty"`
	DurationMs     int64            `json:"duration_ms,omitempty"`
	AttemptCount   int              `json:"attempt_count"`
	MaxAttempts    int              `json:"max_attempts"`
	BackoffSeconds int              `json:"backoff_seconds"`
	LastError      string           `json:"last_error,omitempty"`
	Result         string           `json:"result,omitempty"`
	Priority       int              `json:"priority"`
	LockedBy       string           `json:"locked_by,omitempty"`
	LockedAt       *time.Time       `json:"locked_at,omitempty"`
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
	ID            models.ULID      `json:"id"`
	CreatedAt     time.Time        `json:"created_at"`
	JobID         models.ULID      `json:"job_id"`
	Type          models.JobType   `json:"type"`
	TargetID      models.ULID      `json:"target_id,omitempty"`
	TargetName    string           `json:"target_name,omitempty"`
	Status        models.JobStatus `json:"status"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
	DurationMs    int64            `json:"duration_ms,omitempty"`
	AttemptNumber int              `json:"attempt_number"`
	Error         string           `json:"error,omitempty"`
	Result        string           `json:"result,omitempty"`
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

// Manual Channel types

// ManualChannelResponse represents a manual channel in API responses.
type ManualChannelResponse struct {
	ID            models.ULID `json:"id"`
	SourceID      models.ULID `json:"source_id"`
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
	Enabled       bool        `json:"enabled"`
	Priority      int         `json:"priority"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// ManualChannelFromModel converts a model to a response.
func ManualChannelFromModel(c *models.ManualStreamChannel) ManualChannelResponse {
	return ManualChannelResponse{
		ID:            c.ID,
		SourceID:      c.SourceID,
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
		Enabled:       models.BoolVal(c.Enabled),
		Priority:      c.Priority,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

// ManualChannelInput is a single channel in PUT/import requests.
type ManualChannelInput struct {
	TvgID         string `json:"tvg_id,omitempty" doc:"EPG ID for matching" maxLength:"255"`
	TvgName       string `json:"tvg_name,omitempty" doc:"Display name" maxLength:"512"`
	TvgLogo       string `json:"tvg_logo,omitempty" doc:"Logo URL or @logo:token" maxLength:"2048"`
	GroupTitle    string `json:"group_title,omitempty" doc:"Category/group" maxLength:"255"`
	ChannelName   string `json:"channel_name" doc:"Required display name" minLength:"1" maxLength:"512"`
	ChannelNumber int    `json:"channel_number,omitempty" doc:"Optional channel number"`
	StreamURL     string `json:"stream_url" doc:"Stream URL (http/https/rtsp)" minLength:"1" maxLength:"4096"`
	StreamType    string `json:"stream_type,omitempty" doc:"Stream format" maxLength:"50"`
	Language      string `json:"language,omitempty" doc:"Language code" maxLength:"50"`
	Country       string `json:"country,omitempty" doc:"Country code" maxLength:"10"`
	IsAdult       bool   `json:"is_adult,omitempty" doc:"Adult content flag"`
	Enabled       *bool  `json:"enabled,omitempty" doc:"Include in materialization (default: true)"`
	Priority      int    `json:"priority,omitempty" doc:"Sort order"`
}

// ToModel converts input to model for persistence.
func (r *ManualChannelInput) ToModel(sourceID models.ULID) *models.ManualStreamChannel {
	enabled := models.BoolPtr(true)
	if r.Enabled != nil {
		enabled = r.Enabled
	}
	return &models.ManualStreamChannel{
		SourceID:      sourceID,
		TvgID:         r.TvgID,
		TvgName:       r.TvgName,
		TvgLogo:       r.TvgLogo,
		GroupTitle:    r.GroupTitle,
		ChannelName:   r.ChannelName,
		ChannelNumber: r.ChannelNumber,
		StreamURL:     r.StreamURL,
		StreamType:    r.StreamType,
		Language:      r.Language,
		Country:       r.Country,
		IsAdult:       r.IsAdult,
		Enabled:       enabled,
		Priority:      r.Priority,
	}
}

// ReplaceManualChannelsRequest is the PUT request body.
type ReplaceManualChannelsRequest struct {
	Channels []ManualChannelInput `json:"channels" doc:"Complete list of channels (replaces all existing)"`
}

// M3UImportResult is the response for import operations.
type M3UImportResult struct {
	ParsedCount  int                     `json:"parsed_count" doc:"Number of channels parsed from M3U"`
	SkippedCount int                     `json:"skipped_count" doc:"Number of invalid entries skipped"`
	Applied      bool                    `json:"applied" doc:"Whether changes were persisted"`
	Channels     []ManualChannelResponse `json:"channels" doc:"Parsed/applied channels"`
	Errors       []string                `json:"errors,omitempty" doc:"Parse errors encountered"`
}
