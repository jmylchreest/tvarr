package models

import (
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// SourceType represents the type of stream source.
type SourceType string

const (
	// SourceTypeM3U represents an M3U playlist source.
	SourceTypeM3U SourceType = "m3u"
	// SourceTypeXtream represents an Xtream Codes API source.
	SourceTypeXtream SourceType = "xtream"
	// SourceTypeManual represents a manual source with user-defined channels.
	// Manual sources do not fetch from a URL; channels are defined statically
	// in the manual_stream_channels table and materialized during ingestion.
	SourceTypeManual SourceType = "manual"
)

// SourceStatus represents the current status of a source.
type SourceStatus string

const (
	// SourceStatusPending indicates the source has not been ingested yet.
	SourceStatusPending SourceStatus = "pending"
	// SourceStatusIngesting indicates ingestion is in progress.
	SourceStatusIngesting SourceStatus = "ingesting"
	// SourceStatusSuccess indicates the last ingestion was successful.
	SourceStatusSuccess SourceStatus = "success"
	// SourceStatusFailed indicates the last ingestion failed.
	SourceStatusFailed SourceStatus = "failed"
)

// StreamSource represents an upstream channel source (M3U URL or Xtream server).
type StreamSource struct {
	BaseModel

	// Name is a user-friendly name for the source.
	// Must be unique across all stream sources.
	Name string `gorm:"uniqueIndex;not null;size:255" json:"name"`

	// Type indicates whether this is an M3U or Xtream source.
	Type SourceType `gorm:"not null;size:20" json:"type"`

	// URL is the M3U playlist URL or Xtream server base URL.
	URL string `gorm:"not null;size:2048" json:"url"`

	// Username for Xtream authentication (optional for M3U).
	Username string `gorm:"size:255" json:"username,omitempty"`

	// Password for Xtream authentication (optional for M3U).
	Password string `gorm:"size:255" json:"password,omitempty"`

	// UserAgent to use when fetching the source (optional).
	UserAgent string `gorm:"size:512" json:"user_agent,omitempty"`

	// Enabled indicates whether this source should be included in ingestion.
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	Enabled *bool `gorm:"default:true" json:"enabled"`

	// Priority determines the order when merging channels from multiple sources.
	// Higher priority sources take precedence for duplicate channels.
	Priority int `gorm:"default:0" json:"priority"`

	// Status indicates the current ingestion status.
	Status SourceStatus `gorm:"not null;default:'pending';size:20" json:"status"`

	// LastIngestionAt is the timestamp of the last successful ingestion.
	LastIngestionAt *Time `json:"last_ingestion_at,omitempty"`

	// LastError contains the error message from the last failed ingestion.
	LastError string `gorm:"size:4096" json:"last_error,omitempty"`

	// ChannelCount is the number of channels from the last ingestion.
	ChannelCount int `gorm:"default:0" json:"channel_count"`

	// CronSchedule for automatic ingestion (optional).
	// Uses standard cron format: "0 */6 * * *" for every 6 hours.
	CronSchedule string `gorm:"size:100" json:"cron_schedule,omitempty"`

	// Channels is the relationship to channels from this source.
	Channels []Channel `gorm:"foreignKey:SourceID;constraint:OnDelete:CASCADE" json:"channels,omitempty"`
}

// TableName returns the table name for StreamSource.
func (StreamSource) TableName() string {
	return "stream_sources"
}

// IsM3U returns true if this is an M3U source.
func (s *StreamSource) IsM3U() bool {
	return s.Type == SourceTypeM3U
}

// IsXtream returns true if this is an Xtream source.
func (s *StreamSource) IsXtream() bool {
	return s.Type == SourceTypeXtream
}

// IsManual returns true if this is a Manual source.
func (s *StreamSource) IsManual() bool {
	return s.Type == SourceTypeManual
}

// MarkIngesting sets the source status to ingesting.
func (s *StreamSource) MarkIngesting() {
	s.Status = SourceStatusIngesting
	s.LastError = ""
}

// MarkSuccess sets the source status to success with the channel count.
func (s *StreamSource) MarkSuccess(channelCount int) {
	s.Status = SourceStatusSuccess
	now := Now()
	s.LastIngestionAt = &now
	s.ChannelCount = channelCount
	s.LastError = ""
}

// MarkFailed sets the source status to failed with an error message.
func (s *StreamSource) MarkFailed(err error) {
	s.Status = SourceStatusFailed
	if err != nil {
		s.LastError = err.Error()
	}
}

// Sanitize trims whitespace from user-provided fields.
func (s *StreamSource) Sanitize() {
	s.Name = strings.TrimSpace(s.Name)
	s.URL = strings.TrimSpace(s.URL)
	s.Username = strings.TrimSpace(s.Username)
	s.Password = strings.TrimSpace(s.Password)
	s.UserAgent = strings.TrimSpace(s.UserAgent)
}

// Validate performs basic validation on the source.
func (s *StreamSource) Validate() error {
	// Sanitize inputs first
	s.Sanitize()

	if s.Name == "" {
		return ErrNameRequired
	}
	// URL is required for M3U and Xtream sources, optional for Manual sources
	if s.URL == "" && s.Type != SourceTypeManual {
		return ErrURLRequired
	}
	// Validate URL format if provided
	if s.URL != "" {
		if _, err := url.Parse(s.URL); err != nil {
			return ErrInvalidURL
		}
	}
	if s.Type != SourceTypeM3U && s.Type != SourceTypeXtream && s.Type != SourceTypeManual {
		return ErrInvalidSourceType
	}
	if s.Type == SourceTypeXtream && (s.Username == "" || s.Password == "") {
		return ErrXtreamCredentialsRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the source and generates ULID.
func (s *StreamSource) BeforeCreate(tx *gorm.DB) error {
	if err := s.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return s.Validate()
}

// BeforeUpdate is a GORM hook that validates the source before update.
func (s *StreamSource) BeforeUpdate(tx *gorm.DB) error {
	return s.Validate()
}
