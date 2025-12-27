// Package relay provides streaming relay functionality for tvarr.
package relay

import "github.com/jmylchreest/tvarr/internal/models"

// ClientCapabilities represents detected client information.
type ClientCapabilities struct {
	// PlayerName is the player identifier (e.g., "hls.js", "mpegts.js")
	PlayerName string

	// PlayerVersion is the version string if available
	PlayerVersion string

	// PreferredFormat is the detected format preference
	// Values: "hls-fmp4", "hls-ts", "mpegts", "dash", ""
	PreferredFormat string

	// SupportsFMP4 indicates fMP4 segment support
	SupportsFMP4 bool

	// SupportsMPEGTS indicates MPEG-TS segment support
	SupportsMPEGTS bool

	// DetectionSource indicates how capabilities were detected
	// Values: "rule", "x-tvarr-player", "user-agent", "accept", "format_override", "default"
	DetectionSource string

	// AcceptedVideoCodecs are the video codecs this client can decode.
	AcceptedVideoCodecs []string

	// AcceptedAudioCodecs are the audio codecs this client can decode.
	AcceptedAudioCodecs []string

	// PreferredVideoCodec is the video codec to transcode to if source is not accepted.
	PreferredVideoCodec string

	// PreferredAudioCodec is the audio codec to transcode to if source is not accepted.
	PreferredAudioCodec string

	// MatchedRuleName is the name of the matched rule (if detection was rule-based).
	MatchedRuleName string

	// EncodingProfile is the encoding profile from the matched client detection rule.
	// If set, this overrides the proxy's default encoding profile for transcoding.
	EncodingProfile *models.EncodingProfile
}

// AcceptsVideoCodec returns true if the client accepts the given video codec.
func (c *ClientCapabilities) AcceptsVideoCodec(codec string) bool {
	if len(c.AcceptedVideoCodecs) == 0 {
		return true // No restrictions = accepts all
	}
	for _, accepted := range c.AcceptedVideoCodecs {
		if accepted == codec {
			return true
		}
	}
	return false
}

// AcceptsAudioCodec returns true if the client accepts the given audio codec.
func (c *ClientCapabilities) AcceptsAudioCodec(codec string) bool {
	if len(c.AcceptedAudioCodecs) == 0 {
		return true // No restrictions = accepts all
	}
	for _, accepted := range c.AcceptedAudioCodecs {
		if accepted == codec {
			return true
		}
	}
	return false
}

// ClientDetector detects client capabilities from request metadata.
type ClientDetector interface {
	// Detect analyzes the request and returns client capabilities.
	// Detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent/Rules
	Detect(req OutputRequest) ClientCapabilities
}
