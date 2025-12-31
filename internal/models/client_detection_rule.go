package models

import (
	"encoding/json"

	"github.com/jmylchreest/tvarr/internal/codec"
)

// ClientDetectionRule represents a rule for detecting client capabilities from HTTP requests.
// Rules are evaluated in priority order (lower priority = evaluated first) against the
// request's User-Agent and other headers using expression-based matching.
type ClientDetectionRule struct {
	BaseModel

	// Name is a human-readable name for this rule.
	Name string `gorm:"size:255;not null" json:"name"`

	// Description provides additional details about what this rule matches.
	Description string `gorm:"size:1024" json:"description,omitempty"`

	// Expression is the filter expression to match against requests.
	// Uses the same expression language as filters (e.g., `user_agent contains "Android" AND user_agent contains "TV"`).
	Expression string `gorm:"type:text;not null" json:"expression"`

	// Priority determines evaluation order (lower = evaluated first).
	// System rules use priorities 100-999, leaving 1-99 for high-priority custom rules.
	Priority int `gorm:"default:0;index" json:"priority"`

	// IsEnabled determines if the rule is active.
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	IsEnabled *bool `gorm:"default:true" json:"is_enabled"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only IsEnabled can be toggled for system rules.
	IsSystem bool `gorm:"default:false" json:"is_system"`

	// AcceptedVideoCodecs is a JSON array of video codecs this client can decode.
	// Example: ["h264","h265"]
	AcceptedVideoCodecs string `gorm:"size:255" json:"accepted_video_codecs"`

	// AcceptedAudioCodecs is a JSON array of audio codecs this client can decode.
	// Example: ["aac","mp3","ac3"]
	AcceptedAudioCodecs string `gorm:"size:255" json:"accepted_audio_codecs"`

	// PreferredVideoCodec is the video codec to transcode to if source is not accepted.
	PreferredVideoCodec VideoCodec `gorm:"size:20" json:"preferred_video_codec"`

	// PreferredAudioCodec is the audio codec to transcode to if source is not accepted.
	PreferredAudioCodec AudioCodec `gorm:"size:20" json:"preferred_audio_codec"`

	// SupportsFMP4 indicates the client can handle fMP4 segments (modern HLS/DASH).
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	SupportsFMP4 *bool `gorm:"default:true" json:"supports_fmp4"`

	// SupportsMPEGTS indicates the client can handle MPEG-TS segments (legacy HLS).
	// Using pointer to distinguish between "not set" (nil->default true) and "explicitly false".
	SupportsMPEGTS *bool `gorm:"default:true" json:"supports_mpegts"`

	// PreferredFormat is the output format to use when source is compatible.
	// Values: "hls-fmp4", "hls-ts", "dash", "" (auto)
	PreferredFormat string `gorm:"size:20" json:"preferred_format"`

	// EncodingProfileID optionally overrides the proxy's default encoding profile.
	// If nil, uses the proxy's default encoding profile when transcoding is needed.
	EncodingProfileID *ULID `gorm:"type:varchar(26)" json:"encoding_profile_id,omitempty"`

	// EncodingProfile is the related encoding profile (loaded via preload).
	EncodingProfile *EncodingProfile `gorm:"foreignKey:EncodingProfileID" json:"encoding_profile,omitempty"`
}

// TableName returns the table name for ClientDetectionRule.
func (ClientDetectionRule) TableName() string {
	return "client_detection_rules"
}

// Validate checks if the rule configuration is valid.
func (r *ClientDetectionRule) Validate() error {
	if r.Name == "" {
		return ValidationError{Field: "name", Message: "name is required"}
	}
	if r.Expression == "" {
		return ValidationError{Field: "expression", Message: "expression is required"}
	}
	// Validate accepted codecs JSON format
	if r.AcceptedVideoCodecs != "" {
		var codecs []string
		if err := json.Unmarshal([]byte(r.AcceptedVideoCodecs), &codecs); err != nil {
			return ValidationError{Field: "accepted_video_codecs", Message: "must be a valid JSON array"}
		}
	}
	if r.AcceptedAudioCodecs != "" {
		var codecs []string
		if err := json.Unmarshal([]byte(r.AcceptedAudioCodecs), &codecs); err != nil {
			return ValidationError{Field: "accepted_audio_codecs", Message: "must be a valid JSON array"}
		}
	}
	// Must support at least one container format
	// Nil pointers default to true, so only fail if both are explicitly false
	supportsFMP4 := r.SupportsFMP4 == nil || *r.SupportsFMP4
	supportsMPEGTS := r.SupportsMPEGTS == nil || *r.SupportsMPEGTS
	if !supportsFMP4 && !supportsMPEGTS {
		return ValidationError{Field: "supports_fmp4", Message: "must support at least one container format"}
	}
	return nil
}

// GetAcceptedVideoCodecs parses and returns the accepted video codecs as a slice.
func (r *ClientDetectionRule) GetAcceptedVideoCodecs() []string {
	if r.AcceptedVideoCodecs == "" {
		return nil
	}
	var codecs []string
	if err := json.Unmarshal([]byte(r.AcceptedVideoCodecs), &codecs); err != nil {
		return nil
	}
	return codecs
}

// GetAcceptedAudioCodecs parses and returns the accepted audio codecs as a slice.
func (r *ClientDetectionRule) GetAcceptedAudioCodecs() []string {
	if r.AcceptedAudioCodecs == "" {
		return nil
	}
	var codecs []string
	if err := json.Unmarshal([]byte(r.AcceptedAudioCodecs), &codecs); err != nil {
		return nil
	}
	return codecs
}

// SetAcceptedVideoCodecs sets the accepted video codecs from a slice.
func (r *ClientDetectionRule) SetAcceptedVideoCodecs(codecs []string) error {
	if len(codecs) == 0 {
		r.AcceptedVideoCodecs = ""
		return nil
	}
	data, err := json.Marshal(codecs)
	if err != nil {
		return err
	}
	r.AcceptedVideoCodecs = string(data)
	return nil
}

// SetAcceptedAudioCodecs sets the accepted audio codecs from a slice.
func (r *ClientDetectionRule) SetAcceptedAudioCodecs(codecs []string) error {
	if len(codecs) == 0 {
		r.AcceptedAudioCodecs = ""
		return nil
	}
	data, err := json.Marshal(codecs)
	if err != nil {
		return err
	}
	r.AcceptedAudioCodecs = string(data)
	return nil
}

// AcceptsVideoCodec returns true if the client accepts the given video codec.
// Uses codec.VideoMatch to handle aliases (e.g., hevc=h265, avc=h264).
func (r *ClientDetectionRule) AcceptsVideoCodec(codecName string) bool {
	codecs := r.GetAcceptedVideoCodecs()
	if len(codecs) == 0 {
		return true // No restrictions = accepts all
	}
	for _, accepted := range codecs {
		if codec.VideoMatch(accepted, codecName) {
			return true
		}
	}
	return false
}

// AcceptsAudioCodec returns true if the client accepts the given audio codec.
// Uses codec.AudioMatch to handle aliases (e.g., ec3=eac3).
func (r *ClientDetectionRule) AcceptsAudioCodec(codecName string) bool {
	codecs := r.GetAcceptedAudioCodecs()
	if len(codecs) == 0 {
		return true // No restrictions = accepts all
	}
	for _, accepted := range codecs {
		if codec.AudioMatch(accepted, codecName) {
			return true
		}
	}
	return false
}


// ClientDetectionResult contains the result of evaluating client detection rules.
// Multiple rules can contribute to the result - the first rule to set each attribute wins.
type ClientDetectionResult struct {
	// MatchedRule is the first rule that matched, or nil if using defaults.
	// Note: Other rules may have contributed additional attributes.
	MatchedRule *ClientDetectionRule `json:"matched_rule,omitempty"`

	// AcceptedVideoCodecs from the first rule that specified video codecs.
	AcceptedVideoCodecs []string `json:"accepted_video_codecs"`

	// AcceptedAudioCodecs from the first rule that specified audio codecs.
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs"`

	// PreferredVideoCodec from the first rule that specified a video codec.
	PreferredVideoCodec string `json:"preferred_video_codec"`

	// PreferredAudioCodec from the first rule that specified an audio codec.
	PreferredAudioCodec string `json:"preferred_audio_codec"`

	// SupportsFMP4 from the first rule that matched.
	SupportsFMP4 bool `json:"supports_fmp4"`

	// SupportsMPEGTS from the first rule that matched.
	SupportsMPEGTS bool `json:"supports_mpegts"`

	// PreferredFormat from the first rule that specified a format.
	PreferredFormat string `json:"preferred_format"`

	// DetectionSource indicates how the result was determined.
	// Values: "rule", "format_override", "accept_header", "default"
	DetectionSource string `json:"detection_source"`
}
