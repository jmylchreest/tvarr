package models

import (
	"gorm.io/gorm"
)

// Channel represents an individual channel parsed from a stream source.
type Channel struct {
	BaseModel

	// SourceID is the foreign key to the parent StreamSource.
	SourceID ULID `gorm:"type:varchar(26);not null;index" json:"source_id"`

	// ExtID is an external identifier used for deduplication within a source.
	// For M3U this might be derived from tvg-id or stream URL.
	// For Xtream this is the stream ID from the API.
	ExtID string `gorm:"size:255;index:idx_source_ext_id,unique" json:"ext_id"`

	// TvgID is the EPG channel identifier for matching with program data.
	TvgID string `gorm:"size:255;index" json:"tvg_id,omitempty"`

	// TvgName is the display name from the M3U tvg-name attribute.
	TvgName string `gorm:"size:512" json:"tvg_name,omitempty"`

	// TvgLogo is the URL to the channel logo.
	TvgLogo string `gorm:"size:2048" json:"tvg_logo,omitempty"`

	// GroupTitle is the category/group from the M3U group-title attribute.
	GroupTitle string `gorm:"size:255;index" json:"group_title,omitempty"`

	// ChannelName is the display name (from EXTINF title or computed).
	ChannelName string `gorm:"not null;size:512" json:"channel_name"`

	// ChannelNumber is the channel number (tvg-chno) if specified.
	ChannelNumber int `gorm:"default:0" json:"channel_number,omitempty"`

	// StreamURL is the actual stream URL.
	StreamURL string `gorm:"not null;size:4096" json:"stream_url"`

	// StreamType indicates the stream format (e.g., "live", "movie", "series").
	StreamType string `gorm:"size:50" json:"stream_type,omitempty"`

	// Language is the channel language if known.
	Language string `gorm:"size:50" json:"language,omitempty"`

	// Country is the channel country code if known.
	Country string `gorm:"size:10" json:"country,omitempty"`

	// IsAdult indicates whether this is adult content.
	IsAdult bool `gorm:"default:false" json:"is_adult"`

	// Extra stores additional attributes from the M3U as JSON.
	Extra string `gorm:"type:text" json:"extra,omitempty"`

	// Source is the relationship back to the parent StreamSource.
	Source *StreamSource `gorm:"foreignKey:SourceID" json:"source,omitempty"`
}

// TableName returns the table name for Channel.
func (Channel) TableName() string {
	return "channels"
}

// GetSourceID returns the source ID.
func (c *Channel) GetSourceID() ULID {
	return c.SourceID
}

// Validate performs basic validation on the channel.
func (c *Channel) Validate() error {
	if c.SourceID.IsZero() {
		return ErrSourceIDRequired
	}
	if c.ChannelName == "" {
		return ErrNameRequired
	}
	if c.StreamURL == "" {
		return ErrStreamURLRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates the channel and generates ULID.
func (c *Channel) BeforeCreate(tx *gorm.DB) error {
	if err := c.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	return c.Validate()
}

// BeforeUpdate is a GORM hook that validates the channel before update.
func (c *Channel) BeforeUpdate(tx *gorm.DB) error {
	return c.Validate()
}

// GenerateExtID generates an external ID for deduplication.
// Uses TvgID if available, otherwise derives from stream URL.
func (c *Channel) GenerateExtID() string {
	if c.ExtID != "" {
		return c.ExtID
	}
	if c.TvgID != "" {
		return c.TvgID
	}
	// Use a hash of the stream URL as fallback
	return hashString(c.StreamURL)
}

// hashString creates a simple hash of a string for ID generation.
func hashString(s string) string {
	// Simple hash - in production you might use a proper hash function
	var h uint64
	for i := 0; i < len(s); i++ {
		h = h*31 + uint64(s[i])
	}
	return string(rune(h))
}
