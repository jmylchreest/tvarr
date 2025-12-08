package models

import (
	"time"

	"gorm.io/gorm"
)

// LastKnownCodec caches codec information discovered via ffprobe.
// This avoids re-probing streams that haven't changed.
type LastKnownCodec struct {
	BaseModel

	// StreamURL is the URL that was probed (unique index).
	StreamURL string `gorm:"uniqueIndex;not null;size:2048" json:"stream_url"`

	// SourceID is the source this stream belongs to (optional, for cleanup).
	SourceID ULID `gorm:"type:varchar(26);index" json:"source_id,omitempty"`

	// Video codec information
	VideoCodec     string  `gorm:"size:50" json:"video_codec,omitempty"`
	VideoProfile   string  `gorm:"size:50" json:"video_profile,omitempty"`
	VideoLevel     string  `gorm:"size:20" json:"video_level,omitempty"`
	VideoWidth     int     `json:"video_width,omitempty"`
	VideoHeight    int     `json:"video_height,omitempty"`
	VideoFramerate float64 `json:"video_framerate,omitempty"`
	VideoBitrate   int     `json:"video_bitrate,omitempty"`                // bps
	VideoPixFmt    string  `gorm:"size:50" json:"video_pix_fmt,omitempty"` // yuv420p, etc.

	// Audio codec information
	AudioCodec      string `gorm:"size:50" json:"audio_codec,omitempty"`
	AudioSampleRate int    `json:"audio_sample_rate,omitempty"` // Hz
	AudioChannels   int    `json:"audio_channels,omitempty"`
	AudioBitrate    int    `json:"audio_bitrate,omitempty"` // bps

	// Container information
	ContainerFormat string `gorm:"size:100" json:"container_format,omitempty"`
	Duration        int64  `json:"duration,omitempty"` // milliseconds, 0 for live

	// Stream metadata
	IsLiveStream bool   `gorm:"default:false" json:"is_live_stream"`
	HasSubtitles bool   `gorm:"default:false" json:"has_subtitles"`
	StreamCount  int    `json:"stream_count,omitempty"`
	Title        string `gorm:"size:500" json:"title,omitempty"`

	// Probing metadata
	ProbedAt   Time   `gorm:"not null;index" json:"probed_at"`
	ProbeError string `gorm:"size:1000" json:"probe_error,omitempty"`
	ProbeMs    int64  `json:"probe_ms,omitempty"` // Probe duration in milliseconds

	// Cache control
	ExpiresAt *Time `gorm:"index" json:"expires_at,omitempty"` // When to re-probe
	HitCount  int64 `gorm:"default:0" json:"hit_count"`        // Usage tracking
}

// TableName returns the table name for LastKnownCodec.
func (LastKnownCodec) TableName() string {
	return "last_known_codecs"
}

// Validate performs basic validation on the codec info.
func (c *LastKnownCodec) Validate() error {
	if c.StreamURL == "" {
		return ErrStreamURLRequired
	}
	return nil
}

// BeforeCreate is a GORM hook that validates and sets defaults.
func (c *LastKnownCodec) BeforeCreate(tx *gorm.DB) error {
	if err := c.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	if c.ProbedAt.IsZero() {
		c.ProbedAt = Now()
	}
	return c.Validate()
}

// BeforeUpdate is a GORM hook that validates before update.
func (c *LastKnownCodec) BeforeUpdate(tx *gorm.DB) error {
	return c.Validate()
}

// IsExpired returns true if the cached codec info has expired.
func (c *LastKnownCodec) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().After(time.Time(*c.ExpiresAt))
}

// IsValid returns true if we have valid codec information.
func (c *LastKnownCodec) IsValid() bool {
	return c.ProbeError == "" && (c.VideoCodec != "" || c.AudioCodec != "")
}

// Resolution returns a string representation of the video resolution.
func (c *LastKnownCodec) Resolution() string {
	if c.VideoWidth == 0 || c.VideoHeight == 0 {
		return ""
	}
	return string(rune(c.VideoWidth)) + "x" + string(rune(c.VideoHeight))
}

// SetExpiry sets the expiration time based on a duration from now.
func (c *LastKnownCodec) SetExpiry(d time.Duration) {
	expires := Now().Add(d)
	c.ExpiresAt = &expires
}

// Touch updates the last access time and increments hit count.
func (c *LastKnownCodec) Touch() {
	c.HitCount++
	c.UpdatedAt = Now()
}

// VideoInfo returns a summary of video codec info.
type VideoInfo struct {
	Codec     string  `json:"codec"`
	Profile   string  `json:"profile,omitempty"`
	Level     string  `json:"level,omitempty"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Framerate float64 `json:"framerate,omitempty"`
	Bitrate   int     `json:"bitrate,omitempty"`
	PixFmt    string  `json:"pix_fmt,omitempty"`
}

// AudioInfo returns a summary of audio codec info.
type AudioInfo struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
}

// GetVideoInfo returns structured video information.
func (c *LastKnownCodec) GetVideoInfo() *VideoInfo {
	if c.VideoCodec == "" {
		return nil
	}
	return &VideoInfo{
		Codec:     c.VideoCodec,
		Profile:   c.VideoProfile,
		Level:     c.VideoLevel,
		Width:     c.VideoWidth,
		Height:    c.VideoHeight,
		Framerate: c.VideoFramerate,
		Bitrate:   c.VideoBitrate,
		PixFmt:    c.VideoPixFmt,
	}
}

// GetAudioInfo returns structured audio information.
func (c *LastKnownCodec) GetAudioInfo() *AudioInfo {
	if c.AudioCodec == "" {
		return nil
	}
	return &AudioInfo{
		Codec:      c.AudioCodec,
		SampleRate: c.AudioSampleRate,
		Channels:   c.AudioChannels,
		Bitrate:    c.AudioBitrate,
	}
}

// NeedsVideoTranscode returns true if video needs transcoding to match target.
func (c *LastKnownCodec) NeedsVideoTranscode(targetCodec string, targetWidth, targetHeight int) bool {
	if targetCodec != "" && targetCodec != "copy" && c.VideoCodec != targetCodec {
		return true
	}
	if targetWidth > 0 && c.VideoWidth > targetWidth {
		return true
	}
	if targetHeight > 0 && c.VideoHeight > targetHeight {
		return true
	}
	return false
}

// NeedsAudioTranscode returns true if audio needs transcoding to match target.
func (c *LastKnownCodec) NeedsAudioTranscode(targetCodec string, targetChannels int) bool {
	if targetCodec != "" && targetCodec != "copy" && c.AudioCodec != targetCodec {
		return true
	}
	if targetChannels > 0 && c.AudioChannels > targetChannels {
		return true
	}
	return false
}
