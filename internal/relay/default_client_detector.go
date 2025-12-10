// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
)

// Header name for explicit player identification.
const XTvarrPlayerHeader = "X-Tvarr-Player"

// Known player patterns for User-Agent detection.
var playerPatterns = []struct {
	pattern       *regexp.Regexp
	name          string
	prefersFMP4   bool
	prefersMPEGTS bool
	preferredFmt  string
}{
	// HLS.js with fMP4 preference
	{regexp.MustCompile(`(?i)hls\.js[/ ]?(\d+\.\d+)?`), "hls.js", true, false, FormatValueHLSFMP4},
	// mpegts.js prefers MPEG-TS
	{regexp.MustCompile(`(?i)mpegts\.js`), "mpegts.js", false, true, FormatValueMPEGTS},
	// Video.js
	{regexp.MustCompile(`(?i)video\.js[/ ]?(\d+)?`), "video.js", true, true, ""},
	// ExoPlayer (Android) - supports fMP4
	{regexp.MustCompile(`(?i)exoplayer[/ ]?(\d+)?`), "exoplayer", true, false, FormatValueHLSFMP4},
	// AVPlayer (iOS/macOS) - native HLS
	{regexp.MustCompile(`(?i)avplayer|applecoremedia`), "avplayer", true, false, FormatValueHLS},
	// VLC
	{regexp.MustCompile(`(?i)vlc[/ ]?(\d+)?`), "vlc", true, true, ""},
	// Kodi
	{regexp.MustCompile(`(?i)kodi[/ ]?(\d+)?`), "kodi", true, true, ""},
	// IPTV clients
	{regexp.MustCompile(`(?i)iptv|tivimate|ott`), "iptv-client", false, true, FormatValueMPEGTS},
	// Safari on Apple devices - native HLS
	{regexp.MustCompile(`(?i)safari.*mac os|iphone|ipad`), "safari", true, false, FormatValueHLS},
	// Shaka Player
	{regexp.MustCompile(`(?i)shaka[/ ]?(\d+)?`), "shaka", true, false, FormatValueDASH},
	// dash.js
	{regexp.MustCompile(`(?i)dash\.js`), "dash.js", true, false, FormatValueDASH},
}

// X-Tvarr-Player format mappings for explicit player identification.
// Format: "player:format" or just "player"
var xTvarrPlayerFormats = map[string]ClientCapabilities{
	"hls.js": {
		PlayerName:      "hls.js",
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		PreferredFormat: FormatValueHLSFMP4,
		DetectionSource: "x-tvarr-player",
	},
	"mpegts.js": {
		PlayerName:      "mpegts.js",
		SupportsFMP4:    false,
		SupportsMPEGTS:  true,
		PreferredFormat: FormatValueMPEGTS,
		DetectionSource: "x-tvarr-player",
	},
	"video.js": {
		PlayerName:      "video.js",
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		PreferredFormat: "",
		DetectionSource: "x-tvarr-player",
	},
	"exoplayer": {
		PlayerName:      "exoplayer",
		SupportsFMP4:    true,
		SupportsMPEGTS:  false,
		PreferredFormat: FormatValueHLSFMP4,
		DetectionSource: "x-tvarr-player",
	},
	"avplayer": {
		PlayerName:      "avplayer",
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		PreferredFormat: FormatValueHLS,
		DetectionSource: "x-tvarr-player",
	},
	"vlc": {
		PlayerName:      "vlc",
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		PreferredFormat: "",
		DetectionSource: "x-tvarr-player",
	},
	"kodi": {
		PlayerName:      "kodi",
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		PreferredFormat: "",
		DetectionSource: "x-tvarr-player",
	},
	"shaka": {
		PlayerName:      "shaka",
		SupportsFMP4:    true,
		SupportsMPEGTS:  false,
		PreferredFormat: FormatValueDASH,
		DetectionSource: "x-tvarr-player",
	},
	"dash.js": {
		PlayerName:      "dash.js",
		SupportsFMP4:    true,
		SupportsMPEGTS:  false,
		PreferredFormat: FormatValueDASH,
		DetectionSource: "x-tvarr-player",
	},
}

// DefaultClientDetector implements the ClientDetector interface.
type DefaultClientDetector struct {
	logger *slog.Logger
}

// NewDefaultClientDetector creates a new client detector with optional logger.
func NewDefaultClientDetector(logger *slog.Logger) *DefaultClientDetector {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &DefaultClientDetector{logger: logger}
}

// Detect analyzes the request and returns client capabilities.
// Detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent > Default
func (d *DefaultClientDetector) Detect(req OutputRequest) ClientCapabilities {
	// Step 1: Check explicit format override query parameter
	if req.FormatOverride != "" {
		caps := d.detectFromFormatOverride(req.FormatOverride)
		d.logDetection(caps, "format_override", req)
		return caps
	}

	// Step 2: Check X-Tvarr-Player header (highest priority header)
	if xPlayer := req.GetHeader(XTvarrPlayerHeader); xPlayer != "" {
		if caps, ok := d.detectFromXTvarrPlayer(xPlayer); ok {
			d.logDetection(caps, "x-tvarr-player", req)
			return caps
		}
	}

	// Step 3: Check Accept header for format hints
	if caps, ok := d.detectFromAccept(req.Accept); ok {
		d.logDetection(caps, "accept", req)
		return caps
	}

	// Step 4: Check User-Agent for known players
	if caps, ok := d.detectFromUserAgent(req.UserAgent); ok {
		d.logDetection(caps, "user-agent", req)
		return caps
	}

	// Step 5: Return default capabilities
	caps := d.getDefaultCapabilities()
	d.logDetection(caps, "default", req)
	return caps
}

// detectFromFormatOverride parses explicit format override.
func (d *DefaultClientDetector) detectFromFormatOverride(format string) ClientCapabilities {
	format = strings.ToLower(strings.TrimSpace(format))
	caps := ClientCapabilities{
		DetectionSource: "format_override",
	}

	switch format {
	case "fmp4", "hls-fmp4":
		caps.PreferredFormat = FormatValueHLSFMP4
		caps.SupportsFMP4 = true
	case "mpegts", "mpeg-ts", "ts":
		caps.PreferredFormat = FormatValueMPEGTS
		caps.SupportsMPEGTS = true
	case "hls":
		caps.PreferredFormat = FormatValueHLS
		caps.SupportsFMP4 = true
		caps.SupportsMPEGTS = true
	case "dash":
		caps.PreferredFormat = FormatValueDASH
		caps.SupportsFMP4 = true
	case "hls-ts":
		caps.PreferredFormat = FormatValueHLSTS
		caps.SupportsMPEGTS = true
	default:
		// Unknown format, use generic HLS
		caps.PreferredFormat = FormatValueHLS
		caps.SupportsFMP4 = true
		caps.SupportsMPEGTS = true
	}

	return caps
}

// detectFromXTvarrPlayer parses X-Tvarr-Player header.
// Format: "player" or "player:format"
func (d *DefaultClientDetector) detectFromXTvarrPlayer(header string) (ClientCapabilities, bool) {
	header = strings.ToLower(strings.TrimSpace(header))
	if header == "" {
		return ClientCapabilities{}, false
	}

	// Check for "player:format" syntax
	parts := strings.SplitN(header, ":", 2)
	playerName := parts[0]

	// Look up player in known formats
	if caps, ok := xTvarrPlayerFormats[playerName]; ok {
		// Clone to avoid modifying the global map
		result := caps

		// If format is explicitly specified, override preferred format
		if len(parts) > 1 {
			formatOverride := strings.TrimSpace(parts[1])
			result.PreferredFormat = d.normalizeFormat(formatOverride)
		}

		return result, true
	}

	// Unknown player, but header was present - use it as player name
	caps := ClientCapabilities{
		PlayerName:      playerName,
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		DetectionSource: "x-tvarr-player",
	}

	// If format was specified, use it
	if len(parts) > 1 {
		caps.PreferredFormat = d.normalizeFormat(parts[1])
	}

	return caps, true
}

// detectFromAccept parses Accept header for format hints.
func (d *DefaultClientDetector) detectFromAccept(accept string) (ClientCapabilities, bool) {
	if accept == "" {
		return ClientCapabilities{}, false
	}

	accept = strings.ToLower(accept)

	// Check for DASH preference
	if strings.Contains(accept, "application/dash+xml") {
		return ClientCapabilities{
			PreferredFormat: FormatValueDASH,
			SupportsFMP4:    true,
			DetectionSource: "accept",
		}, true
	}

	// Check for HLS preference
	if strings.Contains(accept, "application/vnd.apple.mpegurl") ||
		strings.Contains(accept, "application/x-mpegurl") {
		return ClientCapabilities{
			PreferredFormat: FormatValueHLS,
			SupportsFMP4:    true,
			SupportsMPEGTS:  true,
			DetectionSource: "accept",
		}, true
	}

	// Check for MPEG-TS preference
	if strings.Contains(accept, "video/mp2t") {
		return ClientCapabilities{
			PreferredFormat: FormatValueMPEGTS,
			SupportsMPEGTS:  true,
			DetectionSource: "accept",
		}, true
	}

	return ClientCapabilities{}, false
}

// detectFromUserAgent parses User-Agent for known player patterns.
func (d *DefaultClientDetector) detectFromUserAgent(userAgent string) (ClientCapabilities, bool) {
	if userAgent == "" {
		return ClientCapabilities{}, false
	}

	for _, p := range playerPatterns {
		if matches := p.pattern.FindStringSubmatch(userAgent); matches != nil {
			caps := ClientCapabilities{
				PlayerName:      p.name,
				SupportsFMP4:    p.prefersFMP4,
				SupportsMPEGTS:  p.prefersMPEGTS,
				PreferredFormat: p.preferredFmt,
				DetectionSource: "user-agent",
			}

			// Extract version if available
			if len(matches) > 1 && matches[1] != "" {
				caps.PlayerVersion = matches[1]
			}

			return caps, true
		}
	}

	return ClientCapabilities{}, false
}

// getDefaultCapabilities returns sensible defaults for unknown clients.
func (d *DefaultClientDetector) getDefaultCapabilities() ClientCapabilities {
	return ClientCapabilities{
		PlayerName:      "",
		PlayerVersion:   "",
		PreferredFormat: "", // Let routing logic decide
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
		DetectionSource: "default",
	}
}

// normalizeFormat normalizes format string to standard values.
func (d *DefaultClientDetector) normalizeFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "fmp4", "hls-fmp4":
		return FormatValueHLSFMP4
	case "ts", "mpegts", "mpeg-ts":
		return FormatValueMPEGTS
	case "hls":
		return FormatValueHLS
	case "hls-ts":
		return FormatValueHLSTS
	case "dash":
		return FormatValueDASH
	default:
		return format
	}
}

// logDetection logs the client detection result.
func (d *DefaultClientDetector) logDetection(caps ClientCapabilities, source string, req OutputRequest) {
	d.logger.Debug("client capabilities detected",
		slog.String("source", source),
		slog.String("player_name", caps.PlayerName),
		slog.String("player_version", caps.PlayerVersion),
		slog.String("preferred_format", caps.PreferredFormat),
		slog.Bool("supports_fmp4", caps.SupportsFMP4),
		slog.Bool("supports_mpegts", caps.SupportsMPEGTS),
		slog.String("user_agent", req.UserAgent),
		slog.String("format_override", req.FormatOverride),
	)
}
