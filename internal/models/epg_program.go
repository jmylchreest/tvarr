package models

import (
	"time"

	"gorm.io/gorm"
)

// EpgProgram represents a single TV program entry from an EPG source.
type EpgProgram struct {
	BaseModel

	// SourceID is the foreign key to the parent EpgSource.
	SourceID ULID `gorm:"type:varchar(26);not null;uniqueIndex:idx_program_unique" json:"source_id"`

	// ChannelID is the EPG channel identifier (matches Channel.TvgID).
	ChannelID string `gorm:"not null;size:255;uniqueIndex:idx_program_unique;index:idx_channel_time" json:"channel_id"`

	// Start is the program start time.
	Start time.Time `gorm:"not null;uniqueIndex:idx_program_unique;index:idx_channel_time" json:"start"`

	// Stop is the program end time.
	Stop time.Time `gorm:"not null;index" json:"stop"`

	// Title is the program title.
	Title string `gorm:"not null;size:512" json:"title"`

	// SubTitle is the episode title or subtitle.
	SubTitle string `gorm:"size:512" json:"sub_title,omitempty"`

	// Description is the full program description.
	Description string `gorm:"type:text" json:"description,omitempty"`

	// Category is the program genre/category.
	Category string `gorm:"size:255;index" json:"category,omitempty"`

	// Icon is the URL to a program image.
	Icon string `gorm:"size:2048" json:"icon,omitempty"`

	// EpisodeNum is the episode number in various formats (e.g., "S01E05").
	EpisodeNum string `gorm:"size:100" json:"episode_num,omitempty"`

	// Rating is the content rating (e.g., "TV-14", "PG-13").
	Rating string `gorm:"size:50" json:"rating,omitempty"`

	// Language is the program language.
	Language string `gorm:"size:50" json:"language,omitempty"`

	// IsNew indicates if this is a new episode.
	IsNew bool `gorm:"default:false" json:"is_new"`

	// IsPremiere indicates if this is a premiere.
	IsPremiere bool `gorm:"default:false" json:"is_premiere"`

	// IsLive indicates if this is a live broadcast.
	IsLive bool `gorm:"default:false" json:"is_live"`

	// Credits stores cast/crew as JSON.
	Credits string `gorm:"type:text" json:"credits,omitempty"`

	// Source is the relationship back to the parent EpgSource.
	Source *EpgSource `gorm:"foreignKey:SourceID" json:"source,omitempty"`
}

// TableName returns the table name for EpgProgram.
func (EpgProgram) TableName() string {
	return "epg_programs"
}

// GetSourceID returns the source ID.
func (p *EpgProgram) GetSourceID() ULID {
	return p.SourceID
}

// Duration returns the program duration.
func (p *EpgProgram) Duration() time.Duration {
	return p.Stop.Sub(p.Start)
}

// IsOnAir returns true if the program is currently airing.
func (p *EpgProgram) IsOnAir() bool {
	now := time.Now()
	return now.After(p.Start) && now.Before(p.Stop)
}

// HasEnded returns true if the program has ended.
func (p *EpgProgram) HasEnded() bool {
	return time.Now().After(p.Stop)
}

// Validate performs basic validation on the EPG program.
func (p *EpgProgram) Validate() error {
	if p.SourceID.IsZero() {
		return ErrSourceIDRequired
	}
	if p.ChannelID == "" {
		return ErrChannelIDRequired
	}
	if p.Start.IsZero() {
		return ErrStartTimeRequired
	}
	if p.Stop.IsZero() {
		return ErrEndTimeRequired
	}
	if p.Title == "" {
		return ErrTitleRequired
	}
	if !p.Stop.After(p.Start) {
		return ErrInvalidTimeRange
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the program and generates ULID.
func (p *EpgProgram) BeforeCreate(tx *gorm.DB) error {
	if err := p.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return p.Validate()
}

// BeforeUpdate is a GORM hook that validates the program before update.
func (p *EpgProgram) BeforeUpdate(tx *gorm.DB) error {
	return p.Validate()
}
