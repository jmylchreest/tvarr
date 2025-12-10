// Package relay provides streaming relay functionality for tvarr.
package relay

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
	// Values: "x-tvarr-player", "user-agent", "accept", "default"
	DetectionSource string
}

// ClientDetector detects client capabilities from request metadata.
type ClientDetector interface {
	// Detect analyzes the request and returns client capabilities.
	// Detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent
	Detect(req OutputRequest) ClientCapabilities
}
