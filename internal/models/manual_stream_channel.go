package models

import (
	"gorm.io/gorm"
)

// ManualStreamChannel represents a user-defined channel for a Manual stream source.
// These channels are materialized into the main channels table during ingestion.
type ManualStreamChannel struct {
	BaseModel

	// SourceID is the Manual stream source this channel belongs to.
	SourceID ULID `gorm:"not null;index" json:"source_id"`

	// TvgID is the EPG channel identifier for matching with program data.
	TvgID string `gorm:"size:255;index" json:"tvg_id,omitempty"`

	// TvgName is the display name.
	TvgName string `gorm:"size:512" json:"tvg_name,omitempty"`

	// TvgLogo is the URL to the channel logo.
	TvgLogo string `gorm:"size:2048" json:"tvg_logo,omitempty"`

	// GroupTitle is the category/group.
	GroupTitle string `gorm:"size:255;index" json:"group_title,omitempty"`

	// ChannelName is the display name.
	ChannelName string `gorm:"not null;size:512" json:"channel_name"`

	// ChannelNumber is the channel number if specified.
	ChannelNumber int `gorm:"default:0" json:"channel_number,omitempty"`

	// StreamURL is the actual stream URL.
	StreamURL string `gorm:"not null;size:4096" json:"stream_url"`

	// StreamType indicates the stream format.
	StreamType string `gorm:"size:50" json:"stream_type,omitempty"`

	// Language is the channel language if known.
	Language string `gorm:"size:50" json:"language,omitempty"`

	// Country is the channel country code if known.
	Country string `gorm:"size:10" json:"country,omitempty"`

	// IsAdult indicates whether this is adult content.
	IsAdult bool `gorm:"default:false" json:"is_adult"`

	// Enabled indicates whether this channel should be included.
	Enabled bool `gorm:"default:true" json:"enabled"`

	// Priority for ordering among manual channels.
	Priority int `gorm:"default:0" json:"priority"`

	// Extra stores additional attributes as JSON.
	Extra string `gorm:"type:text" json:"extra,omitempty"`
}

// TableName returns the table name for ManualStreamChannel.
func (ManualStreamChannel) TableName() string {
	return "manual_stream_channels"
}

// Validate performs basic validation on the manual channel.
func (c *ManualStreamChannel) Validate() error {
	if c.ChannelName == "" {
		return ErrNameRequired
	}
	if c.StreamURL == "" {
		return ErrStreamURLRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the channel and generates ULID.
func (c *ManualStreamChannel) BeforeCreate(tx *gorm.DB) error {
	if err := c.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return c.Validate()
}

// BeforeUpdate is a GORM hook that validates the channel before update.
func (c *ManualStreamChannel) BeforeUpdate(tx *gorm.DB) error {
	return c.Validate()
}

// ToChannel converts a ManualStreamChannel to a Channel for materialization.
// The resulting channel is linked to the same source as the manual channel.
func (c *ManualStreamChannel) ToChannel() *Channel {
	return &Channel{
		SourceID:      c.SourceID,
		ExtID:         c.ID.String(), // Use manual channel ID as external ID for deduplication
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
		Extra:         c.Extra,
	}
}
