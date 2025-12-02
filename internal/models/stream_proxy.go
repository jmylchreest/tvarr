package models

import (
	"gorm.io/gorm"
)

// StreamProxyStatus represents the current status of a proxy.
type StreamProxyStatus string

const (
	// StreamProxyStatusPending indicates the proxy has not been generated yet.
	StreamProxyStatusPending StreamProxyStatus = "pending"
	// StreamProxyStatusGenerating indicates generation is in progress.
	StreamProxyStatusGenerating StreamProxyStatus = "generating"
	// StreamProxyStatusSuccess indicates the last generation was successful.
	StreamProxyStatusSuccess StreamProxyStatus = "success"
	// StreamProxyStatusFailed indicates the last generation failed.
	StreamProxyStatusFailed StreamProxyStatus = "failed"
)

// StreamProxyMode represents how the proxy serves streams.
type StreamProxyMode string

const (
	// StreamProxyModeRedirect redirects clients directly to the upstream URL.
	StreamProxyModeRedirect StreamProxyMode = "redirect"
	// StreamProxyModeProxy proxies the stream through tvarr.
	StreamProxyModeProxy StreamProxyMode = "proxy"
	// StreamProxyModeRelay relays the stream through FFmpeg for transcoding.
	StreamProxyModeRelay StreamProxyMode = "relay"
)

// NumberingMode determines how channel numbers are assigned.
type NumberingMode string

const (
	// NumberingModeSequential assigns sequential numbers starting from StartingChannelNumber.
	NumberingModeSequential NumberingMode = "sequential"
	// NumberingModePreserve keeps existing channel numbers where possible, resolving conflicts.
	NumberingModePreserve NumberingMode = "preserve"
	// NumberingModeGroup assigns numbers within groups (100s for group 1, 200s for group 2, etc.).
	NumberingModeGroup NumberingMode = "group"
)

// StreamProxy represents a proxy configuration that combines sources,
// applies filters and mappings, and generates output playlists.
type StreamProxy struct {
	BaseModel

	// Name is a unique user-friendly name for the proxy.
	Name string `gorm:"uniqueIndex;not null;size:255" json:"name"`

	// Description is an optional description of the proxy.
	Description string `gorm:"size:1024" json:"description,omitempty"`

	// ProxyMode determines how streams are served to clients.
	ProxyMode StreamProxyMode `gorm:"not null;default:'redirect';size:20" json:"proxy_mode"`

	// IsActive indicates whether this proxy is active and should be served.
	IsActive bool `gorm:"default:true" json:"is_active"`

	// AutoRegenerate indicates whether to auto-regenerate when sources change.
	AutoRegenerate bool `gorm:"default:false" json:"auto_regenerate"`

	// StartingChannelNumber is the base channel number for numbering.
	StartingChannelNumber int `gorm:"default:1" json:"starting_channel_number"`

	// NumberingMode determines how channel numbers are assigned.
	// Options: sequential (default), preserve, group
	NumberingMode NumberingMode `gorm:"not null;default:'sequential';size:20" json:"numbering_mode"`

	// GroupNumberingSize is the size of each group range when using group numbering mode.
	// Default is 100 (groups get numbers 100-199, 200-299, etc.).
	GroupNumberingSize int `gorm:"default:100" json:"group_numbering_size"`

	// UpstreamTimeout is the timeout in seconds for upstream connections.
	UpstreamTimeout int `gorm:"default:30" json:"upstream_timeout"`

	// BufferSize is the buffer size in bytes for proxy mode.
	BufferSize int `gorm:"default:8192" json:"buffer_size"`

	// MaxConcurrentStreams is the maximum concurrent streams (0 = unlimited).
	MaxConcurrentStreams int `gorm:"default:0" json:"max_concurrent_streams"`

	// CacheChannelLogos indicates whether to cache channel logos locally.
	CacheChannelLogos bool `gorm:"default:false" json:"cache_channel_logos"`

	// CacheProgramLogos indicates whether to cache EPG program logos locally.
	CacheProgramLogos bool `gorm:"default:false" json:"cache_program_logos"`

	// RelayProfileID is the optional relay profile for transcoding settings.
	RelayProfileID *ULID `gorm:"type:varchar(26)" json:"relay_profile_id,omitempty"`

	// OutputPath is the path for generated files.
	OutputPath string `gorm:"size:512" json:"output_path,omitempty"`

	// Status indicates the current generation status.
	Status StreamProxyStatus `gorm:"not null;default:'pending';size:20" json:"status"`

	// LastGeneratedAt is the timestamp of the last successful generation.
	LastGeneratedAt *Time `json:"last_generated_at,omitempty"`

	// LastError contains the error message from the last failed generation.
	LastError string `gorm:"size:4096" json:"last_error,omitempty"`

	// ChannelCount is the number of channels in the last generation.
	ChannelCount int `gorm:"default:0" json:"channel_count"`

	// ProgramCount is the number of EPG programs in the last generation.
	ProgramCount int `gorm:"default:0" json:"program_count"`

	// CronSchedule for automatic generation (optional).
	// Uses standard cron format: "0 */6 * * *" for every 6 hours.
	CronSchedule string `gorm:"size:100" json:"cron_schedule,omitempty"`

	// Relationships - these are the join tables linking to sources, filters, etc.
	Sources      []ProxySource      `gorm:"foreignKey:ProxyID;constraint:OnDelete:CASCADE" json:"sources,omitempty"`
	EpgSources   []ProxyEpgSource   `gorm:"foreignKey:ProxyID;constraint:OnDelete:CASCADE" json:"epg_sources,omitempty"`
	Filters      []ProxyFilter      `gorm:"foreignKey:ProxyID;constraint:OnDelete:CASCADE" json:"filters,omitempty"`
	MappingRules []ProxyMappingRule `gorm:"foreignKey:ProxyID;constraint:OnDelete:CASCADE" json:"mapping_rules,omitempty"`
}

// TableName returns the table name for StreamProxy.
func (StreamProxy) TableName() string {
	return "stream_proxies"
}

// MarkGenerating sets the proxy status to generating.
func (p *StreamProxy) MarkGenerating() {
	p.Status = StreamProxyStatusGenerating
	p.LastError = ""
}

// MarkSuccess sets the proxy status to success with counts.
func (p *StreamProxy) MarkSuccess(channelCount, programCount int) {
	p.Status = StreamProxyStatusSuccess
	now := Now()
	p.LastGeneratedAt = &now
	p.ChannelCount = channelCount
	p.ProgramCount = programCount
	p.LastError = ""
}

// MarkFailed sets the proxy status to failed with an error message.
func (p *StreamProxy) MarkFailed(err error) {
	p.Status = StreamProxyStatusFailed
	if err != nil {
		p.LastError = err.Error()
	}
}

// Validate performs basic validation on the proxy.
func (p *StreamProxy) Validate() error {
	if p.Name == "" {
		return ErrNameRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the proxy and generates ULID.
func (p *StreamProxy) BeforeCreate(tx *gorm.DB) error {
	if err := p.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return p.Validate()
}

// BeforeUpdate is a GORM hook that validates the proxy before update.
func (p *StreamProxy) BeforeUpdate(tx *gorm.DB) error {
	return p.Validate()
}

// ProxySource is a join table linking a proxy to a stream source.
type ProxySource struct {
	BaseModel

	// ProxyID is the ID of the proxy.
	ProxyID ULID `gorm:"not null;index:idx_proxy_source,unique" json:"proxy_id"`

	// SourceID is the ID of the stream source.
	SourceID ULID `gorm:"not null;index:idx_proxy_source,unique" json:"source_id"`

	// Priority determines the order when merging channels from multiple sources.
	// Higher priority sources take precedence for duplicate channels.
	Priority int `gorm:"default:0" json:"priority"`

	// Proxy is the relationship to the parent proxy.
	Proxy *StreamProxy `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`

	// Source is the relationship to the stream source.
	Source *StreamSource `gorm:"foreignKey:SourceID" json:"source,omitempty"`
}

// TableName returns the table name for ProxySource.
func (ProxySource) TableName() string {
	return "proxy_sources"
}

// Validate performs basic validation on the proxy source.
func (ps *ProxySource) Validate() error {
	if ps.ProxyID.IsZero() {
		return ErrProxyIDRequired
	}
	if ps.SourceID.IsZero() {
		return ErrSourceIDRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates and generates ULID.
func (ps *ProxySource) BeforeCreate(tx *gorm.DB) error {
	if err := ps.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return ps.Validate()
}

// ProxyEpgSource is a join table linking a proxy to an EPG source.
type ProxyEpgSource struct {
	BaseModel

	// ProxyID is the ID of the proxy.
	ProxyID ULID `gorm:"not null;index:idx_proxy_epg_source,unique" json:"proxy_id"`

	// EpgSourceID is the ID of the EPG source.
	EpgSourceID ULID `gorm:"not null;index:idx_proxy_epg_source,unique" json:"epg_source_id"`

	// Priority determines the order when merging EPG data from multiple sources.
	Priority int `gorm:"default:0" json:"priority"`

	// Proxy is the relationship to the parent proxy.
	Proxy *StreamProxy `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`

	// EpgSource is the relationship to the EPG source.
	EpgSource *EpgSource `gorm:"foreignKey:EpgSourceID" json:"epg_source,omitempty"`
}

// TableName returns the table name for ProxyEpgSource.
func (ProxyEpgSource) TableName() string {
	return "proxy_epg_sources"
}

// Validate performs basic validation on the proxy EPG source.
func (pes *ProxyEpgSource) Validate() error {
	if pes.ProxyID.IsZero() {
		return ErrProxyIDRequired
	}
	if pes.EpgSourceID.IsZero() {
		return ErrEpgSourceIDRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates and generates ULID.
func (pes *ProxyEpgSource) BeforeCreate(tx *gorm.DB) error {
	if err := pes.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return pes.Validate()
}

// ProxyFilter is a join table linking a proxy to a filter.
type ProxyFilter struct {
	BaseModel

	// ProxyID is the ID of the proxy.
	ProxyID ULID `gorm:"not null;index:idx_proxy_filter,unique" json:"proxy_id"`

	// FilterID is the ID of the filter.
	FilterID ULID `gorm:"not null;index:idx_proxy_filter,unique" json:"filter_id"`

	// Order determines the order in which filters are applied.
	Order int `gorm:"default:0" json:"order"`

	// Proxy is the relationship to the parent proxy.
	Proxy *StreamProxy `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`

	// Note: Filter relationship will be added when Filter model is implemented in Phase 8
}

// TableName returns the table name for ProxyFilter.
func (ProxyFilter) TableName() string {
	return "proxy_filters"
}

// Validate performs basic validation on the proxy filter.
func (pf *ProxyFilter) Validate() error {
	if pf.ProxyID.IsZero() {
		return ErrProxyIDRequired
	}
	if pf.FilterID.IsZero() {
		return ErrFilterIDRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates and generates ULID.
func (pf *ProxyFilter) BeforeCreate(tx *gorm.DB) error {
	if err := pf.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return pf.Validate()
}

// ProxyMappingRule is a join table linking a proxy to a data mapping rule.
type ProxyMappingRule struct {
	BaseModel

	// ProxyID is the ID of the proxy.
	ProxyID ULID `gorm:"not null;index:idx_proxy_mapping_rule,unique" json:"proxy_id"`

	// MappingRuleID is the ID of the data mapping rule.
	MappingRuleID ULID `gorm:"not null;index:idx_proxy_mapping_rule,unique" json:"mapping_rule_id"`

	// Order determines the order in which mapping rules are applied.
	Order int `gorm:"default:0" json:"order"`

	// Proxy is the relationship to the parent proxy.
	Proxy *StreamProxy `gorm:"foreignKey:ProxyID" json:"proxy,omitempty"`

	// Note: MappingRule relationship will be added when DataMappingRule model is implemented in Phase 7
}

// TableName returns the table name for ProxyMappingRule.
func (ProxyMappingRule) TableName() string {
	return "proxy_mapping_rules"
}

// Validate performs basic validation on the proxy mapping rule.
func (pmr *ProxyMappingRule) Validate() error {
	if pmr.ProxyID.IsZero() {
		return ErrProxyIDRequired
	}
	if pmr.MappingRuleID.IsZero() {
		return ErrMappingRuleIDRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates and generates ULID.
func (pmr *ProxyMappingRule) BeforeCreate(tx *gorm.DB) error {
	if err := pmr.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return pmr.Validate()
}
