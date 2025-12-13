package models

import (
	"net/url"
	"strings"

	"gorm.io/gorm"
)

// EpgSourceType represents the type of EPG source.
type EpgSourceType string

const (
	// EpgSourceTypeXMLTV represents an XMLTV file source.
	EpgSourceTypeXMLTV EpgSourceType = "xmltv"
	// EpgSourceTypeXtream represents an Xtream Codes API EPG source.
	EpgSourceTypeXtream EpgSourceType = "xtream"
)

// EpgSourceStatus represents the current status of an EPG source.
type EpgSourceStatus string

const (
	// EpgSourceStatusPending indicates the source has not been ingested yet.
	EpgSourceStatusPending EpgSourceStatus = "pending"
	// EpgSourceStatusIngesting indicates ingestion is in progress.
	EpgSourceStatusIngesting EpgSourceStatus = "ingesting"
	// EpgSourceStatusSuccess indicates the last ingestion was successful.
	EpgSourceStatusSuccess EpgSourceStatus = "success"
	// EpgSourceStatusFailed indicates the last ingestion failed.
	EpgSourceStatusFailed EpgSourceStatus = "failed"
)

// XtreamApiMethod represents the API method to use for Xtream EPG sources.
type XtreamApiMethod string

const (
	// XtreamApiMethodStreamID uses the per-stream JSON API (action=get_simple_date_table).
	// This provides richer EPG data with more forward days but requires N requests.
	XtreamApiMethodStreamID XtreamApiMethod = "stream_id"
	// XtreamApiMethodBulkXMLTV uses the bulk XMLTV endpoint (/xmltv.php).
	// This is more performant (1 request) but may have fewer forward days.
	XtreamApiMethodBulkXMLTV XtreamApiMethod = "bulk_xmltv"
)

// EpgSource represents an upstream EPG (Electronic Program Guide) source.
type EpgSource struct {
	BaseModel

	// Name is a user-friendly name for the source.
	// Must be unique across all EPG sources.
	Name string `gorm:"uniqueIndex;not null;size:255" json:"name"`

	// Type indicates whether this is an XMLTV or Xtream source.
	Type EpgSourceType `gorm:"not null;size:20" json:"type"`

	// URL is the XMLTV file URL or Xtream server base URL.
	URL string `gorm:"not null;size:2048" json:"url"`

	// Username for Xtream authentication (optional for XMLTV).
	Username string `gorm:"size:255" json:"username,omitempty"`

	// Password for Xtream authentication (optional for XMLTV).
	Password string `gorm:"size:255" json:"password,omitempty"`

	// ApiMethod specifies the API method for Xtream sources.
	// Only applicable when Type is "xtream". Defaults to "stream_id".
	ApiMethod XtreamApiMethod `gorm:"size:20;default:'stream_id'" json:"api_method,omitempty"`

	// UserAgent to use when fetching the source (optional).
	UserAgent string `gorm:"size:512" json:"user_agent,omitempty"`

	// DetectedTimezone is the timezone detected from the EPG data (e.g., "+00:00", "+01:00", "Europe/London").
	// This is auto-detected during ingestion and updated on each ingest. Read-only for users.
	DetectedTimezone string `gorm:"size:50" json:"detected_timezone,omitempty"`

	// EpgShift is a manual time shift in hours to adjust EPG times (e.g., -2, +1).
	// Use this when the detected timezone is incorrect or you want to shift all times.
	// Positive values shift times forward, negative values shift times back.
	EpgShift int `gorm:"default:0" json:"epg_shift"`

	// Enabled indicates whether this source should be included in ingestion.
	Enabled bool `gorm:"default:true" json:"enabled"`

	// Priority determines the order when merging programs from multiple sources.
	Priority int `gorm:"default:0" json:"priority"`

	// Status indicates the current ingestion status.
	Status EpgSourceStatus `gorm:"not null;default:'pending';size:20" json:"status"`

	// LastIngestionAt is the timestamp of the last successful ingestion.
	LastIngestionAt *Time `json:"last_ingestion_at,omitempty"`

	// LastError contains the error message from the last failed ingestion.
	LastError string `gorm:"size:4096" json:"last_error,omitempty"`

	// ProgramCount is the number of programs from the last ingestion.
	ProgramCount int `gorm:"default:0" json:"program_count"`

	// CronSchedule for automatic ingestion (optional).
	CronSchedule string `gorm:"size:100" json:"cron_schedule,omitempty"`

	// RetentionDays is how long to keep EPG data after it expires.
	// Default is 1 day past the program end time.
	RetentionDays int `gorm:"default:1" json:"retention_days"`
}

// TableName returns the table name for EpgSource.
func (EpgSource) TableName() string {
	return "epg_sources"
}

// IsXMLTV returns true if this is an XMLTV source.
func (s *EpgSource) IsXMLTV() bool {
	return s.Type == EpgSourceTypeXMLTV
}

// IsXtream returns true if this is an Xtream source.
func (s *EpgSource) IsXtream() bool {
	return s.Type == EpgSourceTypeXtream
}

// MarkIngesting sets the source status to ingesting.
func (s *EpgSource) MarkIngesting() {
	s.Status = EpgSourceStatusIngesting
	s.LastError = ""
}

// MarkSuccess sets the source status to success with the program count.
func (s *EpgSource) MarkSuccess(programCount int) {
	s.Status = EpgSourceStatusSuccess
	now := Now()
	s.LastIngestionAt = &now
	s.ProgramCount = programCount
	s.LastError = ""
}

// MarkFailed sets the source status to failed with an error message.
func (s *EpgSource) MarkFailed(err error) {
	s.Status = EpgSourceStatusFailed
	if err != nil {
		s.LastError = err.Error()
	}
}

// Sanitize trims whitespace from user-provided fields.
func (s *EpgSource) Sanitize() {
	s.Name = strings.TrimSpace(s.Name)
	s.URL = strings.TrimSpace(s.URL)
	s.Username = strings.TrimSpace(s.Username)
	s.Password = strings.TrimSpace(s.Password)
	s.UserAgent = strings.TrimSpace(s.UserAgent)
}

// Validate performs basic validation on the EPG source.
func (s *EpgSource) Validate() error {
	// Sanitize inputs first
	s.Sanitize()

	if s.Name == "" {
		return ErrNameRequired
	}
	if s.URL == "" {
		return ErrURLRequired
	}
	// Validate URL format
	if _, err := url.Parse(s.URL); err != nil {
		return ErrInvalidURL
	}
	if s.Type != EpgSourceTypeXMLTV && s.Type != EpgSourceTypeXtream {
		return ErrInvalidEpgSourceType
	}
	if s.Type == EpgSourceTypeXtream && (s.Username == "" || s.Password == "") {
		return ErrXtreamCredentialsRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the source and generates ULID.
func (s *EpgSource) BeforeCreate(tx *gorm.DB) error {
	if err := s.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return s.Validate()
}

// BeforeUpdate is a GORM hook that validates the source before update.
func (s *EpgSource) BeforeUpdate(tx *gorm.DB) error {
	return s.Validate()
}
