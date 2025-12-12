// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jmylchreest/tvarr/internal/models"
)

// FormatRouter errors.
var (
	ErrUnsupportedFormat    = errors.New("unsupported output format")
	ErrUnsupportedOperation = errors.New("unsupported operation for this format")
	ErrNoHandlerAvailable   = errors.New("no handler available for format")
	ErrSegmentNotFound      = errors.New("segment not found")
)

// OutputRequest represents a client request for stream output.
type OutputRequest struct {
	// Format is the requested output format (hls, dash, mpegts, auto).
	Format string

	// Segment is the segment number for HLS/DASH segment requests.
	// nil means playlist/manifest request.
	Segment *uint64

	// InitType is the initialization segment type for DASH.
	// "v" for video, "a" for audio, empty for media segments.
	InitType string

	// UserAgent is the client's User-Agent header.
	UserAgent string

	// Accept is the client's Accept header.
	Accept string

	// Headers contains all HTTP request headers for flexible client detection.
	// This allows expressions to access any header via @header_req:<name> syntax
	// and enables detection of X-Tvarr-Player or any custom player headers.
	Headers map[string][]string

	// FormatOverride is the ?format= query parameter for explicit format override.
	// Values: "mpegts", "fmp4", "hls", "dash"
	FormatOverride string
}

// IsPlaylistRequest returns true if this is a playlist/manifest request.
func (r *OutputRequest) IsPlaylistRequest() bool {
	return r.Segment == nil && r.InitType == ""
}

// IsSegmentRequest returns true if this is a segment request.
func (r *OutputRequest) IsSegmentRequest() bool {
	return r.Segment != nil
}

// IsInitRequest returns true if this is a DASH initialization segment request.
func (r *OutputRequest) IsInitRequest() bool {
	return r.InitType != ""
}

// GetHeader returns the first value for the named header, or empty string if not present.
// Header names are case-insensitive per HTTP specification.
func (r *OutputRequest) GetHeader(name string) string {
	if r.Headers == nil {
		return ""
	}
	// HTTP headers are case-insensitive, check canonical form
	for key, values := range r.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// PassthroughHandler is the interface for passthrough proxy handlers.
type PassthroughHandler interface {
	GetUpstreamURL() string
}

// HLSPassthrough is the interface for HLS passthrough handlers.
type HLSPassthrough interface {
	PassthroughHandler
	ServePlaylist(ctx context.Context, w http.ResponseWriter) error
	ServeSegment(ctx context.Context, w http.ResponseWriter, segmentIndex int) error
}

// DASHPassthrough is the interface for DASH passthrough handlers.
type DASHPassthrough interface {
	PassthroughHandler
	ServeManifest(ctx context.Context, w http.ResponseWriter) error
	ServeSegment(ctx context.Context, w http.ResponseWriter, segmentID string) error
	ServeInitSegment(ctx context.Context, w http.ResponseWriter, initID string) error
}

// FormatRouter routes requests to appropriate output handlers.
type FormatRouter struct {
	defaultFormat models.ContainerFormat
	handlers      map[string]OutputHandler

	// Passthrough handlers for HLS/DASH sources
	hlsPassthrough  HLSPassthrough
	dashPassthrough DASHPassthrough

	// Logger for structured logging
	logger *slog.Logger
}

// NewFormatRouter creates a new format router with a default no-op logger.
func NewFormatRouter(defaultFormat models.ContainerFormat) *FormatRouter {
	return NewFormatRouterWithLogger(defaultFormat, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// NewFormatRouterWithLogger creates a new format router with a custom logger.
func NewFormatRouterWithLogger(defaultFormat models.ContainerFormat, logger *slog.Logger) *FormatRouter {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &FormatRouter{
		defaultFormat: defaultFormat,
		handlers:      make(map[string]OutputHandler),
		logger:        logger,
	}
}

// RegisterHandler registers an output handler for a format.
func (r *FormatRouter) RegisterHandler(format string, handler OutputHandler) {
	r.handlers[format] = handler
	r.logger.Debug("registered output handler",
		slog.String("format", format),
	)
}

// RegisterPassthroughHandler registers a passthrough handler for HLS or DASH.
func (r *FormatRouter) RegisterPassthroughHandler(format string, handler PassthroughHandler) {
	switch format {
	case FormatValueHLS:
		if h, ok := handler.(HLSPassthrough); ok {
			r.hlsPassthrough = h
			r.logger.Debug("registered HLS passthrough handler",
				slog.String("upstream_url", h.GetUpstreamURL()),
			)
		}
	case FormatValueDASH:
		if h, ok := handler.(DASHPassthrough); ok {
			r.dashPassthrough = h
			r.logger.Debug("registered DASH passthrough handler",
				slog.String("upstream_url", h.GetUpstreamURL()),
			)
		}
	}
}

// GetHLSPassthrough returns the HLS passthrough handler if registered.
func (r *FormatRouter) GetHLSPassthrough() HLSPassthrough {
	return r.hlsPassthrough
}

// GetDASHPassthrough returns the DASH passthrough handler if registered.
func (r *FormatRouter) GetDASHPassthrough() DASHPassthrough {
	return r.dashPassthrough
}

// IsPassthroughMode returns true if passthrough handlers are registered.
func (r *FormatRouter) IsPassthroughMode(format string) bool {
	switch format {
	case FormatValueHLS:
		return r.hlsPassthrough != nil
	case FormatValueDASH:
		return r.dashPassthrough != nil
	default:
		return false
	}
}

// GetHandler returns the handler for the requested format.
func (r *FormatRouter) GetHandler(req OutputRequest) (OutputHandler, error) {
	format := r.ResolveFormat(req)

	handler, ok := r.handlers[format]
	if !ok {
		r.logger.Warn("no handler available for format",
			slog.String("resolved_format", format),
			slog.String("requested_format", req.Format),
		)
		return nil, ErrNoHandlerAvailable
	}

	r.logger.Debug("handler selected",
		slog.String("format", format),
		slog.Bool("is_playlist", req.IsPlaylistRequest()),
		slog.Bool("is_segment", req.IsSegmentRequest()),
	)
	return handler, nil
}

// ResolveFormat determines the output format for a request.
func (r *FormatRouter) ResolveFormat(req OutputRequest) string {
	format := strings.ToLower(strings.TrimSpace(req.Format))

	var resolved string
	var reason string

	switch format {
	case FormatValueHLS, FormatValueDASH, FormatValueMPEGTS:
		resolved = format
		reason = "explicit"
	case FormatValueAuto, "":
		resolved = r.DetectOptimalFormat(req.UserAgent, req.Accept)
		reason = "auto-detected"
	default:
		resolved = string(r.defaultFormat)
		reason = "unknown_format_fallback"
	}

	r.logger.Debug("format resolved",
		slog.String("requested", req.Format),
		slog.String("resolved", resolved),
		slog.String("reason", reason),
	)

	return resolved
}

// DetectOptimalFormat auto-detects the best format for a client.
// Returns HLS for Apple devices, DASH for clients requesting it,
// and MPEG-TS as the universal fallback.
func (r *FormatRouter) DetectOptimalFormat(userAgent, accept string) string {
	ua := strings.ToLower(userAgent)
	acc := strings.ToLower(accept)

	var format string
	var detection string

	// Apple devices prefer HLS
	if isAppleDevice(ua) {
		format = FormatValueHLS
		detection = "apple_device"
	} else if strings.Contains(acc, "application/dash+xml") {
		// Check Accept header for DASH preference
		format = FormatValueDASH
		detection = "accept_header_dash"
	} else if strings.Contains(acc, "application/vnd.apple.mpegurl") ||
		strings.Contains(acc, "application/x-mpegurl") {
		// Check Accept header for HLS preference
		format = FormatValueHLS
		detection = "accept_header_hls"
	} else if strings.Contains(ua, "shaka") || strings.Contains(ua, "dash") {
		// Some players identify themselves
		format = FormatValueDASH
		detection = "player_identification"
	} else if r.defaultFormat != "" {
		// Default to configured format
		format = string(r.defaultFormat)
		detection = "configured_default"
	} else {
		// Ultimate fallback to MPEG-TS
		format = FormatValueMPEGTS
		detection = "fallback_mpegts"
	}

	r.logger.Debug("auto-detected format",
		slog.String("format", format),
		slog.String("detection_method", detection),
		slog.String("user_agent", userAgent),
		slog.String("accept", accept),
	)

	return format
}

// isAppleDevice returns true if the User-Agent indicates an Apple device.
func isAppleDevice(ua string) bool {
	appleIndicators := []string{
		"iphone",
		"ipad",
		"ipod",
		"mac os x",
		"safari",
		"applecoremedia",
		"apple tv",
		"tvos",
	}

	for _, indicator := range appleIndicators {
		if strings.Contains(ua, indicator) {
			// Exclude Chrome/Firefox on Mac which support other formats
			if !strings.Contains(ua, "chrome") && !strings.Contains(ua, "firefox") {
				return true
			}
			// Safari on Mac still prefers HLS
			if indicator == "safari" && !strings.Contains(ua, "chrome") {
				return true
			}
		}
	}
	return false
}

// DefaultFormat returns the configured default format.
func (r *FormatRouter) DefaultFormat() models.ContainerFormat {
	return r.defaultFormat
}

// SetDefaultFormat sets the default format.
func (r *FormatRouter) SetDefaultFormat(format models.ContainerFormat) {
	r.defaultFormat = format
}

// HasHandler returns true if a handler is registered for the format.
func (r *FormatRouter) HasHandler(format string) bool {
	_, ok := r.handlers[format]
	return ok
}

// SupportedFormats returns the list of formats with registered handlers.
func (r *FormatRouter) SupportedFormats() []string {
	formats := make([]string, 0, len(r.handlers))
	for format := range r.handlers {
		formats = append(formats, format)
	}
	return formats
}
