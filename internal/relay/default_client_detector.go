// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"io"
	"log/slog"
	"strings"
)

// Header name for explicit player identification.
const XTvarrPlayerHeader = "X-Tvarr-Player"

// NOTE: Player detection patterns and X-Tvarr-Player mappings have been moved
// to database-backed ClientDetectionRules for configurability. This file now
// only contains the DefaultClientDetector fallback which handles:
// - Format override query parameters
// - Accept header parsing for format hints
// - Default capabilities when no rule matches

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
// This is a fallback detector used when ClientDetectionService is not available.
// Detection priority: FormatOverride > Accept > Default
//
// NOTE: User-Agent based player detection is now handled by database-backed
// ClientDetectionRules via ClientDetectionService. This fallback only handles
// format overrides and Accept header hints.
func (d *DefaultClientDetector) Detect(req OutputRequest) ClientCapabilities {
	// Step 1: Check explicit format override query parameter
	if req.FormatOverride != "" {
		caps := d.detectFromFormatOverride(req.FormatOverride)
		d.logDetection(caps, "format_override", req)
		return caps
	}

	// Step 2: Check Accept header for format hints
	if caps, ok := d.detectFromAccept(req.Accept); ok {
		d.logDetection(caps, "accept", req)
		return caps
	}

	// Step 3: Return default capabilities
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
