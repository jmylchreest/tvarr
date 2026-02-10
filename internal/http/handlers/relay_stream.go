package handlers

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/version"
)

// RelayStreamHandler handles stream relay API endpoints.
type RelayStreamHandler struct {
	relayService           *service.RelayService
	clientDetectionService *service.ClientDetectionService
	logger                 *slog.Logger
}

// NewRelayStreamHandler creates a new relay stream handler.
func NewRelayStreamHandler(relayService *service.RelayService) *RelayStreamHandler {
	return &RelayStreamHandler{
		relayService: relayService,
		logger:       slog.Default(),
	}
}

// WithLogger sets the logger for the handler.
func (h *RelayStreamHandler) WithLogger(logger *slog.Logger) *RelayStreamHandler {
	h.logger = logger
	return h
}

// WithClientDetectionService sets the client detection service.
func (h *RelayStreamHandler) WithClientDetectionService(svc *service.ClientDetectionService) *RelayStreamHandler {
	h.clientDetectionService = svc
	return h
}

// setStreamHeaders sets the X-Stream-* and X-Tvarr-Version headers on the response.
// This centralizes all stream header logic to avoid repetition.
func setStreamHeaders(w http.ResponseWriter, mode, decision string) {
	w.Header().Set("X-Stream-Mode", mode)
	w.Header().Set("X-Stream-Decision", decision)
	w.Header().Set("X-Tvarr-Version", version.Version)
}

// setCORSHeaders sets the CORS headers for cross-origin streaming.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
}

// Register registers the relay stream routes with the API (Huma routes).
// Note: The /proxy/{proxyId}/{channelId} streaming endpoints are registered
// via RegisterChiRoutes for raw HTTP handler access (needed for 302 redirects, CORS).
func (h *RelayStreamHandler) Register(api huma.API) {
	// Documentation-only registration for the proxy streaming endpoint.
	// The actual request handling is done by raw Chi handlers (RegisterChiRoutes)
	// because Huma's StreamResponse commits HTTP 200 before Body runs, making
	// HTTP 302 redirects and pre-stream CORS headers impossible.
	h.registerProxyStreamDocs(api)

	// Note: /proxy/{proxyId}/{channelId} GET and OPTIONS are registered via
	// RegisterChiRoutes() as raw Chi handlers for proper HTTP control
	// (redirect mode needs 302, proxy mode needs CORS headers before streaming)

	huma.Register(api, huma.Operation{
		OperationID: "probeStream",
		Method:      "POST",
		Path:        "/api/v1/relay/probe",
		Summary:     "Probe a stream for codec information",
		Description: "Probes a stream to detect codec information. Either provide a URL directly, or a channel_id to look up the channel's stream URL from the database.",
		Tags:        []string{"Stream Relay"},
	}, h.ProbeStream)

	huma.Register(api, huma.Operation{
		OperationID: "classifyStream",
		Method:      "POST",
		Path:        "/api/v1/relay/classify",
		Summary:     "Classify a stream URL",
		Description: "Classifies a stream URL to determine processing mode",
		Tags:        []string{"Stream Relay"},
	}, h.ClassifyStream)

	huma.Register(api, huma.Operation{
		OperationID: "getLastKnownCodecs",
		Method:      "GET",
		Path:        "/api/v1/relay/lastknowncodecs",
		Summary:     "Get last known codec cache stats",
		Description: "Returns statistics about the cached codec information from ffprobe",
		Tags:        []string{"Stream Relay"},
	}, h.GetCodecCacheStats)

	huma.Register(api, huma.Operation{
		OperationID: "clearLastKnownCodecs",
		Method:      "DELETE",
		Path:        "/api/v1/relay/lastknowncodecs",
		Summary:     "Clear last known codec cache",
		Description: "Clears all cached codec information, forcing re-probe on next stream request",
		Tags:        []string{"Stream Relay"},
	}, h.ClearCodecCache)

	huma.Register(api, huma.Operation{
		OperationID: "listRelaySessions",
		Method:      "GET",
		Path:        "/api/v1/relay/sessions",
		Summary:     "List active relay sessions",
		Description: "Returns all active relay sessions with their statistics for flow visualization",
		Tags:        []string{"Stream Relay"},
	}, h.ListRelaySessions)
}

// RegisterChiRoutes registers streaming routes as raw Chi handlers.
// This is necessary because Huma's StreamResponse doesn't support HTTP 302 redirects
// or setting CORS headers before the response body is written.
func (h *RelayStreamHandler) RegisterChiRoutes(router chi.Router) {
	// Full proxy streaming: /proxy/{proxyId}/{channelId}
	router.Get("/proxy/{proxyId}/{channelId}", h.handleRawStream)
	router.Options("/proxy/{proxyId}/{channelId}", h.handleRawStreamOptions)

	// Channel preview streaming: /proxy/{channelId}
	// Uses zero-transcode smart delivery (passthrough/repackage only)
	router.Get("/proxy/{channelId}", h.handleChannelPreview)
	router.Options("/proxy/{channelId}", h.handleRawStreamOptions)
}

// proxyStreamDocsHandler is a no-op handler for documentation-only registrations.
// The actual request handling is done by raw Chi handlers registered via RegisterChiRoutes.
func (h *RelayStreamHandler) proxyStreamDocsHandler(ctx context.Context, input *StreamChannelByProxyInput) (*huma.StreamResponse, error) {
	// This handler should never be called because Chi handles the route first.
	// It exists only for OpenAPI documentation generation.
	return nil, huma.Error500InternalServerError("this endpoint is handled by raw Chi handlers", nil)
}

// proxyStreamOptionsDocsHandler is a no-op handler for CORS preflight documentation.
func (h *RelayStreamHandler) proxyStreamOptionsDocsHandler(ctx context.Context, input *StreamChannelByProxyOptionsInput) (*StreamChannelByProxyOptionsOutput, error) {
	// This handler should never be called because Chi handles the route first.
	return &StreamChannelByProxyOptionsOutput{}, nil
}

// registerProxyStreamDocs registers documentation-only operations for the proxy streaming endpoint.
// The actual request handling is done by raw Chi handlers (RegisterChiRoutes).
// This ensures the endpoints appear in OpenAPI docs while maintaining proper HTTP behavior.
func (h *RelayStreamHandler) registerProxyStreamDocs(api huma.API) {
	// GET /proxy/{proxyId}/{channelId} - Stream a channel through proxy
	huma.Register(api, huma.Operation{
		OperationID: "streamChannelByProxy",
		Method:      "GET",
		Path:        "/proxy/{proxyId}/{channelId}",
		Summary:     "Stream a channel through proxy",
		Description: `Streams a channel using the proxy's configured mode.

**Modes:**
- **direct**: Returns HTTP 302 redirect to the source stream URL (zero overhead)
- **smart**: Intelligent delivery that automatically selects the optimal strategy:
  - Passthrough: Serves source as-is when formats match client
  - Repackage: Converts manifest format without re-encoding (HLS↔DASH)
  - Transcode: FFmpeg transcoding when codec conversion is needed

**Response Headers:**
- X-Stream-Mode: direct or smart (matches proxy mode)
- X-Stream-Decision: redirect, proxy, passthrough, repackage, or transcode
- X-Stream-Format: output format (hls-ts, hls-fmp4, dash, mpegts)
- X-Tvarr-Version: tvarr version
- Access-Control-Allow-Origin: * (for smart mode)`,
		Tags: []string{"Stream Proxy"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Stream content (smart mode)",
				Headers: map[string]*huma.Param{
					"Content-Type":                 {Description: "video/mp2t, application/vnd.apple.mpegurl, or upstream content type"},
					"X-Stream-Mode":                {Description: "smart"},
					"X-Stream-Decision":            {Description: "Processing decision made (passthrough, repackage, transcode)"},
					"X-Stream-Format":              {Description: "Output format (hls-ts, hls-fmp4, dash, mpegts)"},
					"X-Tvarr-Version":              {Description: "tvarr version"},
					"Access-Control-Allow-Origin":  {Description: "CORS header (always *)"},
					"Access-Control-Allow-Methods": {Description: "Allowed HTTP methods"},
				},
			},
			"302": {
				Description: "Redirect to source stream (direct mode)",
				Headers: map[string]*huma.Param{
					"Location":          {Description: "Source stream URL"},
					"X-Stream-Mode":     {Description: "direct"},
					"X-Stream-Decision": {Description: "redirect"},
					"X-Tvarr-Version":   {Description: "tvarr version"},
				},
			},
			"400": {Description: "Invalid proxy or channel ID format"},
			"404": {Description: "Stream proxy or channel not found"},
			"500": {Description: "Internal server error"},
			"502": {Description: "Upstream server error"},
		},
		SkipValidateBody: true,
	}, h.proxyStreamDocsHandler)

	// OPTIONS /proxy/{proxyId}/{channelId} - CORS preflight
	huma.Register(api, huma.Operation{
		OperationID: "streamChannelByProxyOptions",
		Method:      "OPTIONS",
		Path:        "/proxy/{proxyId}/{channelId}",
		Summary:     "CORS preflight for stream endpoint",
		Description: "Handles CORS preflight requests for browser-based stream clients.",
		Tags:        []string{"Stream Proxy"},
		Responses: map[string]*huma.Response{
			"204": {
				Description: "CORS preflight response",
				Headers: map[string]*huma.Param{
					"Access-Control-Allow-Origin":  {Description: "Allowed origins (*)"},
					"Access-Control-Allow-Methods": {Description: "Allowed methods (GET, OPTIONS)"},
					"Access-Control-Allow-Headers": {Description: "Allowed headers"},
				},
			},
		},
	}, h.proxyStreamOptionsDocsHandler)
}

// handleRawStreamOptions handles CORS preflight requests for the stream endpoint.
func (h *RelayStreamHandler) handleRawStreamOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.WriteHeader(http.StatusNoContent)
}

// validFormatValues contains valid ?format= query parameter values.
var validFormatValues = map[string]bool{
	relay.FormatValueHLS:     true,
	relay.FormatValueDASH:    true,
	relay.FormatValueMPEGTS:  true,
	relay.FormatValueFMP4:    true,
	relay.FormatValueHLSFMP4: true,
	relay.FormatValueHLSTS:   true,
	relay.FormatValueAuto:    true,
	"ts":                     true, // Alias for mpegts
	"mpeg-ts":                true, // Alias for mpegts
}

// validateFormatParam validates the ?format= query parameter and returns 400 Bad Request for invalid values.
// Returns true if format is valid (or empty), false if invalid format was detected and error response was sent.
func (h *RelayStreamHandler) validateFormatParam(w http.ResponseWriter, r *http.Request) bool {
	formatParam := r.URL.Query().Get(relay.QueryParamFormat)
	if formatParam == "" {
		return true // No format specified is valid
	}

	// Normalize to lowercase for comparison
	formatLower := strings.ToLower(formatParam)
	if !validFormatValues[formatLower] {
		h.logger.Warn("Invalid format parameter",
			"format", formatParam,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, fmt.Sprintf("invalid format parameter: %q (valid values: hls, dash, mpegts, fmp4, hls-fmp4, hls-ts, auto)", formatParam), http.StatusBadRequest)
		return false
	}
	return true
}

// handleRawStream is the main raw HTTP handler for streaming.
// It dispatches to mode-specific handlers based on proxy configuration.
func (h *RelayStreamHandler) handleRawStream(w http.ResponseWriter, r *http.Request) {
	// Validate format parameter early (T040/T041)
	if !h.validateFormatParam(w, r) {
		return
	}

	ctx := r.Context()

	proxyIDStr := chi.URLParam(r, "proxyId")
	channelIDStr := chi.URLParam(r, "channelId")

	proxyID, err := models.ParseULID(proxyIDStr)
	if err != nil {
		http.Error(w, "invalid proxy ID format", http.StatusBadRequest)
		return
	}

	channelID, err := models.ParseULID(channelIDStr)
	if err != nil {
		http.Error(w, "invalid channel ID format", http.StatusBadRequest)
		return
	}

	// Get stream info (proxy, channel, optional profile)
	streamInfo, err := h.relayService.GetStreamInfo(ctx, proxyID, channelID)
	if err != nil {
		h.logger.Error("Failed to get stream info",
			"proxy_id", proxyIDStr,
			"channel_id", channelIDStr,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("stream not found: %v", err), http.StatusNotFound)
		return
	}

	// Dispatch based on proxy mode
	switch streamInfo.Proxy.ProxyMode {
	case models.StreamProxyModeDirect:
		h.handleRawDirectMode(w, r, streamInfo)

	case models.StreamProxyModeSmart:
		h.handleRawSmartMode(w, r, streamInfo)

	default:
		h.logger.Error("Unknown proxy mode",
			"proxy_id", proxyIDStr,
			"mode", streamInfo.Proxy.ProxyMode,
		)
		http.Error(w, fmt.Sprintf("unknown proxy mode: %s (valid modes: direct, smart)", streamInfo.Proxy.ProxyMode), http.StatusInternalServerError)
	}
}

// handleChannelPreview handles channel preview streaming at /proxy/{channelId}.
// This provides zero-transcode smart delivery for admin UI previews.
// Unlike the full proxy endpoint, this only uses passthrough or repackage modes.
func (h *RelayStreamHandler) handleChannelPreview(w http.ResponseWriter, r *http.Request) {
	// Validate format parameter early (T040/T041)
	if !h.validateFormatParam(w, r) {
		return
	}

	ctx := r.Context()

	channelIDStr := chi.URLParam(r, "channelId")

	channelID, err := models.ParseULID(channelIDStr)
	if err != nil {
		http.Error(w, "invalid channel ID format", http.StatusBadRequest)
		return
	}

	// Get channel directly from relay service
	channel, err := h.relayService.GetChannel(ctx, channelID)
	if err != nil {
		h.logger.Error("Failed to get channel for preview",
			"channel_id", channelIDStr,
			"error", err,
		)
		http.Error(w, fmt.Sprintf("channel not found: %v", err), http.StatusNotFound)
		return
	}

	streamURL := channel.StreamURL

	// Classify the source stream
	classification := relay.ClassificationResult(h.relayService.ClassifyStream(ctx, streamURL))

	// Determine client's desired format from query param or Accept header
	clientFormat := h.resolveClientFormat(r, classification)

	// Create a minimal StreamInfo for the handlers
	previewInfo := &service.StreamInfo{
		Channel: channel,
		// No Proxy or Profile - this is a preview-only stream
	}

	// Check if there's already an active session for this channel.
	// If so, we should join it to share the SharedESBuffer rather than
	// creating a separate direct proxy connection.
	existingSession := h.relayService.GetSessionForChannel(channelID)

	if existingSession != nil {
		h.logger.Info("Channel preview: joining existing session",
			"channel_id", channel.ID,
			"session_id", existingSession.ID,
			"client_format", clientFormat,
		)

		// Set common headers
		setCORSHeaders(w)
		setStreamHeaders(w, "smart", "join-existing")

		// Use the existing session's SharedESBuffer
		// Preview mode doesn't use transcoding - pass VariantSource for passthrough
		// Preview mode doesn't use client detection, so preferredFormat is empty
		switch clientFormat {
		case relay.FormatValueHLS, relay.FormatValueDASH:
			h.handleMultiFormatOutput(w, r, existingSession, previewInfo, clientFormat, relay.VariantSource, "")
		default:
			// For MPEG-TS and auto format, use the MPEG-TS processor
			h.streamMPEGTSFromRelay(w, r, existingSession, previewInfo, relay.VariantSource)
		}
		return
	}

	// No existing session - determine delivery strategy
	// Get routing decision - use nil profile to force zero-transcode behavior
	// With no profile, SelectRoute will choose passthrough or repackage only
	routingDecision := relay.SelectRoute(classification, clientFormat, nil)

	h.logger.Info("Channel preview: routing decision",
		"channel_id", channel.ID,
		"channel_name", channel.ChannelName,
		"stream_url", streamURL,
		"source_format", classification.SourceFormat,
		"client_format", clientFormat,
		"decision", routingDecision.String(),
	)

	// Dispatch based on routing decision
	// For preview mode, we allow HLS/DASH/MPEGTS format requests to use the ES pipeline
	// to ensure consistent behavior with proxy routes and enable session sharing.
	// Preview mode doesn't use transcoding - pass VariantSource for passthrough
	// Preview mode doesn't use client detection, so preferredFormat is empty
	switch routingDecision {
	case relay.RoutePassthrough:
		// Check if client explicitly requested a format that benefits from ES pipeline
		if clientFormat == relay.FormatValueMPEGTS {
			h.logger.Info("Channel preview: using ES pipeline for explicit MPEG-TS request",
				"channel_id", channel.ID,
			)
			h.handleSmartTranscode(w, r, previewInfo, clientFormat, relay.VariantSource, "")
		} else {
			h.handleSmartPassthrough(w, r, previewInfo, &classification)
		}

	case relay.RouteRepackage:
		h.handleSmartRepackage(w, r, previewInfo, &classification, clientFormat, relay.VariantSource, "")

	case relay.RouteTranscode:
		// For HLS/DASH/MPEGTS format requests, use the ES pipeline
		if clientFormat == relay.FormatValueHLS || clientFormat == relay.FormatValueDASH || clientFormat == relay.FormatValueMPEGTS {
			h.logger.Info("Channel preview: using ES pipeline for format request",
				"channel_id", channel.ID,
				"client_format", clientFormat,
			)
			h.handleSmartTranscode(w, r, previewInfo, clientFormat, relay.VariantSource, "")
		} else {
			// For auto/unknown formats, fall back to passthrough to avoid FFmpeg overhead
			h.logger.Info("Channel preview: transcoding requested but falling back to passthrough",
				"channel_id", channel.ID,
			)
			h.handleSmartPassthrough(w, r, previewInfo, &classification)
		}
	}
}

// handleRawDirectMode returns an HTTP 302 redirect to the source stream URL.
// This is the new simplified mode that replaces the deprecated "redirect" mode.
func (h *RelayStreamHandler) handleRawDirectMode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo) {
	streamURL := info.Channel.StreamURL

	h.logger.Info("Direct mode: sending 302 redirect",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
	)

	w.Header().Set("Location", streamURL)
	setStreamHeaders(w, "direct", "redirect")
	w.WriteHeader(http.StatusFound)
}

// handleRawSmartMode uses smart delivery logic to determine the optimal delivery strategy.
// It classifies the source stream and uses SelectDelivery to choose between:
// - Passthrough: serve source as-is when formats match
// - Repackage: change manifest format without re-encoding (HLS↔DASH)
// - Transcode: run through FFmpeg for codec/format conversion
func (h *RelayStreamHandler) handleRawSmartMode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo) {
	ctx := r.Context()
	streamURL := info.Channel.StreamURL

	// Classify the source stream
	classification := relay.ClassificationResult(h.relayService.ClassifyStream(ctx, streamURL))

	// Detect client capabilities (codecs, formats)
	clientCaps := h.detectClientCapabilities(r)

	// If client detection matched a rule with an encoding profile, use it
	// This allows client detection rules to override the proxy's default profile
	if clientCaps.EncodingProfile != nil {
		h.logger.Debug("Using client detection rule's encoding profile",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"rule_name", clientCaps.MatchedRuleName,
			"profile_name", clientCaps.EncodingProfile.Name,
			"target_video", clientCaps.EncodingProfile.TargetVideoCodec,
			"target_audio", clientCaps.EncodingProfile.TargetAudioCodec,
		)
		info.EncodingProfile = clientCaps.EncodingProfile
	}

	// Determine client's desired format
	clientFormat := h.capsToClientFormat(clientCaps)

	// Get source codec info for smart delivery decision
	// Uses intelligent probing that respects connection limits and reuses session data
	var sourceCodecs []string
	var sourceVideoCodec, sourceAudioCodec string
	if codecInfo := h.relayService.GetOrProbeCodecInfo(ctx, info.Channel.ID, streamURL); codecInfo != nil {
		sourceVideoCodec = codecInfo.VideoCodec
		sourceAudioCodec = codecInfo.AudioCodec
		if sourceVideoCodec != "" {
			sourceCodecs = append(sourceCodecs, sourceVideoCodec)
		}
		if sourceAudioCodec != "" {
			sourceCodecs = append(sourceCodecs, sourceAudioCodec)
		}
	}

	// Get routing decision using the RoutingDecider with codec compatibility
	decider := relay.NewDefaultRoutingDecider(h.logger)
	routingResult := decider.Decide(classification.SourceFormat, sourceCodecs, clientCaps, info.EncodingProfile)

	h.logger.Info("Routing decision",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"decision", routingResult.Decision.String(),
		"source_format", classification.SourceFormat,
		"source_video", sourceVideoCodec,
		"source_audio", sourceAudioCodec,
		"client_format", clientFormat,
		"client_player", clientCaps.PlayerName,
	)

	// If routing decision is transcode but we have no encoding profile, create a dynamic one
	// from the client's preferred codecs. This handles X-Video-Codec/X-Audio-Codec headers.
	// Must be done BEFORE computeTargetVariant since that uses the encoding profile.
	if routingResult.Decision == relay.RouteTranscode && info.EncodingProfile == nil {
		if clientCaps.PreferredVideoCodec != "" || clientCaps.PreferredAudioCodec != "" {
			// Create a dynamic profile based on client preferences
			// Empty codec string means copy/passthrough for that stream
			videoCodec := models.VideoCodec(clientCaps.PreferredVideoCodec)
			audioCodec := models.AudioCodec(clientCaps.PreferredAudioCodec)

			info.EncodingProfile = &models.EncodingProfile{
				Name:             "Dynamic Client Request",
				TargetVideoCodec: videoCodec,
				TargetAudioCodec: audioCodec,
				QualityPreset:    models.QualityPresetMedium,
			}

			h.logger.Debug("Created dynamic encoding profile from client preferences",
				"proxy_id", info.Proxy.ID,
				"channel_id", info.Channel.ID,
				"rule_name", clientCaps.MatchedRuleName,
				"target_video", videoCodec,
				"target_audio", audioCodec,
				"source_video", sourceVideoCodec,
				"source_audio", sourceAudioCodec,
			)
		}
	}

	// Compute target codec variant based on client detection or encoding profile
	// This determines what codecs to transcode TO (if transcoding is needed)
	targetVariant := h.computeTargetVariant(info, clientCaps, sourceVideoCodec, sourceAudioCodec)

	// Dispatch based on routing decision
	switch routingResult.Decision {
	case relay.RoutePassthrough:
		h.handleSmartPassthrough(w, r, info, &classification)

	case relay.RouteRepackage:
		// For repackage mode (client accepts source codecs), we need to clear the
		// encoding profile so the relay session doesn't transcode. Create a copy
		// of info without the encoding profile.
		infoNoTranscode := *info
		infoNoTranscode.EncodingProfile = nil
		h.handleSmartRepackage(w, r, &infoNoTranscode, &classification, clientFormat, targetVariant, clientCaps.PreferredFormat)

	case relay.RouteTranscode:
		h.handleSmartTranscode(w, r, info, clientFormat, targetVariant, clientCaps.PreferredFormat)
	}
}

// resolveClientFormat determines the client's desired output format.
// It checks the ?format= query parameter first, then uses client detection
// based on User-Agent, Accept headers, and X-Tvarr-Player header.
func (h *RelayStreamHandler) resolveClientFormat(r *http.Request, source relay.ClassificationResult) string {
	caps := h.detectClientCapabilities(r)
	return h.capsToClientFormat(caps)
}

// detectClientCapabilities detects client capabilities using rules or default detection.
func (h *RelayStreamHandler) detectClientCapabilities(r *http.Request) relay.ClientCapabilities {
	formatParam := r.URL.Query().Get(relay.QueryParamFormat)

	// If ClientDetectionService is available and has cached rules, use it
	if h.clientDetectionService != nil {
		// Check format override first (handled by service)
		result := h.clientDetectionService.EvaluateRequest(r)

		// Convert service result to relay.ClientCapabilities
		caps := relay.ClientCapabilities{
			AcceptedVideoCodecs: result.AcceptedVideoCodecs,
			AcceptedAudioCodecs: result.AcceptedAudioCodecs,
			PreferredVideoCodec: result.PreferredVideoCodec,
			PreferredAudioCodec: result.PreferredAudioCodec,
			SupportsFMP4:        result.SupportsFMP4,
			SupportsMPEGTS:      result.SupportsMPEGTS,
			PreferredFormat:     result.PreferredFormat,
			DetectionSource:     result.DetectionSource,
		}
		if result.MatchedRule != nil {
			caps.MatchedRuleName = result.MatchedRule.Name
			// If the matched rule has an encoding profile, use it for transcoding
			// This overrides the proxy's default encoding profile
			if result.MatchedRule.EncodingProfile != nil {
				caps.EncodingProfile = result.MatchedRule.EncodingProfile
			}
		}

		// Handle format override from query param (takes precedence)
		if formatParam != "" {
			caps = h.applyFormatOverride(caps, formatParam)
		}

		return caps
	}

	// Fallback to DefaultClientDetector if no service available
	detector := relay.NewDefaultClientDetector(h.logger)
	outputReq := relay.OutputRequest{
		UserAgent:      r.Header.Get("User-Agent"),
		Accept:         r.Header.Get("Accept"),
		Headers:        r.Header,
		FormatOverride: formatParam,
	}

	return detector.Detect(outputReq)
}

// applyFormatOverride applies a format query parameter override to capabilities.
func (h *RelayStreamHandler) applyFormatOverride(caps relay.ClientCapabilities, format string) relay.ClientCapabilities {
	switch format {
	case relay.FormatValueHLS:
		caps.PreferredFormat = relay.FormatValueHLS
		caps.SupportsFMP4 = true
		caps.SupportsMPEGTS = true
		caps.DetectionSource = "format_override"
	case relay.FormatValueHLSFMP4:
		caps.PreferredFormat = relay.FormatValueHLSFMP4
		caps.SupportsFMP4 = true
		caps.SupportsMPEGTS = false
		caps.DetectionSource = "format_override"
	case relay.FormatValueHLSTS:
		caps.PreferredFormat = relay.FormatValueHLSTS
		caps.SupportsFMP4 = false
		caps.SupportsMPEGTS = true
		caps.DetectionSource = "format_override"
	case relay.FormatValueDASH:
		caps.PreferredFormat = relay.FormatValueDASH
		caps.SupportsFMP4 = true
		caps.SupportsMPEGTS = false
		caps.DetectionSource = "format_override"
	case relay.FormatValueMPEGTS:
		caps.PreferredFormat = relay.FormatValueMPEGTS
		caps.SupportsFMP4 = false
		caps.SupportsMPEGTS = true
		caps.DetectionSource = "format_override"
	}
	return caps
}

// capsToClientFormat converts ClientCapabilities to a client format string.
func (h *RelayStreamHandler) capsToClientFormat(caps relay.ClientCapabilities) string {
	// If client detection found a preferred format, use it
	if caps.PreferredFormat != "" {
		switch caps.PreferredFormat {
		case relay.FormatValueHLS, relay.FormatValueHLSTS:
			h.logger.Debug("Client detection resolved format",
				"format", "hls",
				"source", caps.DetectionSource,
				"rule", caps.MatchedRuleName)
			return relay.FormatValueHLS
		case relay.FormatValueHLSFMP4:
			h.logger.Debug("Client detection resolved format",
				"format", "hls-fmp4",
				"source", caps.DetectionSource,
				"rule", caps.MatchedRuleName)
			return relay.FormatValueHLS
		case relay.FormatValueDASH:
			h.logger.Debug("Client detection resolved format",
				"format", "dash",
				"source", caps.DetectionSource,
				"rule", caps.MatchedRuleName)
			return relay.FormatValueDASH
		case relay.FormatValueMPEGTS:
			h.logger.Debug("Client detection resolved format",
				"format", "mpegts",
				"source", caps.DetectionSource,
				"rule", caps.MatchedRuleName)
			return relay.FormatValueMPEGTS
		}
	}

	// If client supports MPEG-TS and prefers it (e.g., libmpv, VLC)
	if caps.SupportsMPEGTS && !caps.SupportsFMP4 {
		h.logger.Debug("Client detection prefers MPEG-TS",
			"source", caps.DetectionSource,
			"rule", caps.MatchedRuleName)
		return relay.FormatValueMPEGTS
	}

	// Default to auto (serve source format as-is)
	return relay.FormatValueAuto
}

// computeTargetVariant determines the target codec variant for transcoding.
// Logic:
// 1. Client capabilities (from headers/detection) define what codecs the client ACCEPTS
// 2. Source codecs define what we HAVE
// 3. Encoding profile defines what we WANT to transcode to
// 4. Priority: client compatibility > profile preferences
//   - If client accepts source codec, use source (no transcoding needed for that track)
//   - If client doesn't accept source, use profile target if client accepts it
//   - Otherwise use client's preferred codec
//
// The returned variant always contains actual codec names (e.g., "h265/aac"), never "copy".
func (h *RelayStreamHandler) computeTargetVariant(
	info *service.StreamInfo,
	clientCaps relay.ClientCapabilities,
	sourceVideoCodec, sourceAudioCodec string,
) relay.CodecVariant {
	// Start with source codecs
	videoCodec := sourceVideoCodec
	audioCodec := sourceAudioCodec

	// Get profile targets if available
	var profileVideoCodec, profileAudioCodec string
	if info.EncodingProfile != nil {
		profileVideoCodec = string(info.EncodingProfile.TargetVideoCodec)
		profileAudioCodec = string(info.EncodingProfile.TargetAudioCodec)
		// Resolve "copy" or empty to source
		if profileVideoCodec == "" || profileVideoCodec == "copy" || profileVideoCodec == "none" {
			profileVideoCodec = sourceVideoCodec
		}
		if profileAudioCodec == "" || profileAudioCodec == "copy" || profileAudioCodec == "none" {
			profileAudioCodec = sourceAudioCodec
		}
	}

	// Determine video codec based on client compatibility
	if sourceVideoCodec != "" {
		if clientCaps.AcceptsVideoCodec(sourceVideoCodec) {
			// Client accepts source - no video transcoding needed
			videoCodec = sourceVideoCodec
		} else if profileVideoCodec != "" && clientCaps.AcceptsVideoCodec(profileVideoCodec) {
			// Client accepts profile target - use it
			videoCodec = profileVideoCodec
		} else if clientCaps.PreferredVideoCodec != "" {
			// Fall back to client's preferred codec
			videoCodec = clientCaps.PreferredVideoCodec
		}
	} else {
		// Source unknown (no ffprobe) - use profile or client preference
		if profileVideoCodec != "" {
			videoCodec = profileVideoCodec
		} else if clientCaps.PreferredVideoCodec != "" {
			videoCodec = clientCaps.PreferredVideoCodec
		}
	}

	// Determine audio codec based on client compatibility
	if sourceAudioCodec != "" {
		if clientCaps.AcceptsAudioCodec(sourceAudioCodec) {
			// Client accepts source - no audio transcoding needed
			audioCodec = sourceAudioCodec
		} else if profileAudioCodec != "" && clientCaps.AcceptsAudioCodec(profileAudioCodec) {
			// Client accepts profile target - use it
			audioCodec = profileAudioCodec
		} else if clientCaps.PreferredAudioCodec != "" {
			// Fall back to client's preferred codec
			audioCodec = clientCaps.PreferredAudioCodec
		}
	} else {
		// Source unknown (no ffprobe) - use profile or client preference
		if profileAudioCodec != "" {
			audioCodec = profileAudioCodec
		} else if clientCaps.PreferredAudioCodec != "" {
			audioCodec = clientCaps.PreferredAudioCodec
		}
	}

	// If both codecs match source (or are empty), return VariantSource for passthrough
	if videoCodec == sourceVideoCodec && audioCodec == sourceAudioCodec {
		return relay.VariantSource
	}

	variant := relay.NewCodecVariant(videoCodec, audioCodec)
	h.logger.Debug("Target variant computed",
		"video_target", videoCodec,
		"audio_target", audioCodec,
		"source_video", sourceVideoCodec,
		"source_audio", sourceAudioCodec,
		"profile_video", profileVideoCodec,
		"profile_audio", profileAudioCodec,
		"variant", variant.String(),
		"detection_source", clientCaps.DetectionSource,
		"matched_rule", clientCaps.MatchedRuleName,
	)
	return variant
}

// getEncodingProfile returns the encoding profile from stream info.
// EncodingProfile always has concrete target codecs (no auto-detection).
// This is a simplified version that replaced the old resolveProfileWithAutoDetection.
func (h *RelayStreamHandler) getEncodingProfile(info *service.StreamInfo) *models.EncodingProfile {
	return info.EncodingProfile
}

// handleSmartPassthrough serves the source stream using copy mode through the ES pipeline.
// This enables connection sharing between multiple clients viewing the same stream.
// Source format is preserved (no transcoding) but goes through the ES buffer.
func (h *RelayStreamHandler) handleSmartPassthrough(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult) {
	// Set common headers
	setCORSHeaders(w)
	setStreamHeaders(w, "smart", "passthrough")

	// Determine client format from classification
	clientFormat := string(classification.SourceFormat)
	if clientFormat == "" {
		clientFormat = relay.FormatValueMPEGTS
	}

	// For passthrough mode (client accepts source codecs), we need to clear the
	// encoding profile so the relay session doesn't transcode. Create a copy
	// of info without the encoding profile.
	infoNoTranscode := *info
	infoNoTranscode.EncodingProfile = nil

	// Route through ES pipeline with copy mode for connection sharing
	h.handleSmartTranscode(w, r, &infoNoTranscode, clientFormat, relay.VariantSource, "")
}

// handleSmartRepackage repackages the source stream to a different manifest format.
// This is used for:
// - HLS↔DASH conversion without re-encoding
// - MPEG-TS passthrough via the ES buffer pipeline (enables connection sharing)
//
// Even when no format conversion is needed, we route through the ES buffer pipeline
// to enable multiple clients sharing a single upstream connection.
func (h *RelayStreamHandler) handleSmartRepackage(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult, clientFormat string, targetVariant relay.CodecVariant, preferredFormat string) {
	// Set common headers
	setCORSHeaders(w)
	setStreamHeaders(w, "smart", "repackage")

	// For MPEG-TS sources where client accepts codecs directly, use the ES buffer pipeline
	// without transcoding. This enables connection sharing between multiple clients.
	// The transcode handler will detect that no transcoding is needed and just remux.
	h.handleSmartTranscode(w, r, info, clientFormat, targetVariant, preferredFormat)
}

// handleSmartTranscode transcodes the source stream using FFmpeg.
// This is used when codec conversion is needed or when creating segments from raw TS.
// For HLS/DASH requests, it serves playlists and segments using the format handlers.
// The preferredFormat parameter preserves the specific HLS sub-format (hls-fmp4, hls-ts) from client detection.
func (h *RelayStreamHandler) handleSmartTranscode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, clientFormat string, targetVariant relay.CodecVariant, preferredFormat string) {
	ctx := r.Context()

	// Set common headers
	setCORSHeaders(w)
	// Only set stream decision headers if not already set by caller (e.g., handleSmartRepackage)
	if w.Header().Get("X-Stream-Decision") == "" {
		setStreamHeaders(w, "smart", "transcode")
	}

	// Get the encoding profile (no auto-detection needed with EncodingProfile)
	profile := h.getEncodingProfile(info)

	// For HLS/DASH formats, we need to start the relay session with multi-format support
	// and initialize multi-format output before FFmpeg starts to ensure segments are captured.
	needsMultiFormat := clientFormat == relay.FormatValueHLS || clientFormat == relay.FormatValueDASH

	// Start or join the relay session with the encoding profile
	session, err := h.relayService.StartRelayWithProfile(ctx, info.Channel.ID, profile)
	if err != nil {
		errAttrs := []any{
			"channel_id", info.Channel.ID,
			"error", err,
		}
		if info.Proxy != nil {
			errAttrs = append([]any{"proxy_id", info.Proxy.ID}, errAttrs...)
		}
		h.logger.Error("Failed to start relay session for smart transcode", errAttrs...)
		http.Error(w, "failed to start relay session", http.StatusInternalServerError)
		return
	}

	// For HLS/DASH formats, use the format router for output
	// The ES pipeline initializes all output handlers during session startup
	if needsMultiFormat {
		h.handleMultiFormatOutput(w, r, session, info, clientFormat, targetVariant, preferredFormat)
		return
	}

	// For MPEG-TS and other formats, stream directly
	h.streamMPEGTSFromRelay(w, r, session, info, targetVariant)
}

// handleMultiFormatOutput handles HLS/DASH output using on-demand processor creation.
// It serves playlists on base requests and segments on segment requests.
// Processors are only created when clients actually request their format.
// Each client can have a different codec variant based on their profile or client detection.
// The preferredFormat parameter preserves the specific HLS sub-format (hls-fmp4, hls-ts) from client detection.
func (h *RelayStreamHandler) handleMultiFormatOutput(w http.ResponseWriter, r *http.Request, session *relay.RelaySession, info *service.StreamInfo, clientFormat string, targetVariant relay.CodecVariant, preferredFormat string) {
	// Parse request parameters
	segmentStr := r.URL.Query().Get(relay.QueryParamSegment)
	initStr := r.URL.Query().Get(relay.QueryParamInit)
	formatOverride := r.URL.Query().Get(relay.QueryParamFormat)
	variantOverride := r.URL.Query().Get(relay.QueryParamVariant)
	trackType := r.URL.Query().Get("track") // For DASH track-specific init segments (video/audio)

	// Use the pre-computed target variant (determined by computeTargetVariant)
	// If variant is specified in URL (from playlist segment URLs), use that instead
	clientVariant := targetVariant
	if variantOverride != "" {
		// Parse variant from URL (format: "video/audio", e.g., "h264/aac")
		clientVariant = relay.ParseCodecVariant(variantOverride)
		h.logger.Debug("Using variant from URL parameter",
			"session_id", session.ID,
			"variant", clientVariant.String(),
			"variant_param", variantOverride,
		)
	}

	h.logger.Debug("HLS/DASH client using target variant",
		"session_id", session.ID,
		"variant", clientVariant.String(),
	)

	// Generate a client ID for tracking HLS/DASH clients
	// Use IP (without port) + User-Agent hash to identify unique clients
	// This handles multiple TCP connections from same client (e.g., mpv parallel segment fetches)
	remoteAddr := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		remoteAddr = strings.Split(fwd, ",")[0]
	}
	// Strip port from remote address (e.g., "192.168.1.100:54321" -> "192.168.1.100")
	clientIP := remoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		clientIP = host
	}
	// Create a short hash of User-Agent to distinguish different clients from same IP
	// (e.g., multiple browser tabs, different players)
	userAgent := r.UserAgent()
	uaHasher := fnv.New32a()
	uaHasher.Write([]byte(userAgent))
	uaHash := fmt.Sprintf("%08x", uaHasher.Sum32())
	// Determine format prefix for client ID
	formatPrefix := string(clientFormat)
	if formatPrefix == "" {
		formatPrefix = "stream"
	}
	clientID := fmt.Sprintf("%s-%s-%s", formatPrefix, clientIP, uaHash)

	// Wait for the session pipeline to be ready before accessing processors
	if err := session.WaitReady(r.Context()); err != nil {
		h.logger.Error("Session not ready for streaming",
			"session_id", session.ID,
			"error", err,
		)
		http.Error(w, "session not ready", http.StatusServiceUnavailable)
		return
	}

	// Clear any idle state since a client is actively connecting
	// This prevents the session from being cleaned up while serving requests
	session.ClearIdleState()

	// Build output request for format determination
	var segment *uint64
	if segmentStr != "" {
		if seq, err := strconv.ParseUint(segmentStr, 10, 64); err == nil {
			segment = &seq
		}
	}

	outputReq := relay.OutputRequest{
		Format:         string(clientFormat),
		Segment:        segment,
		InitType:       initStr,
		UserAgent:      r.Header.Get("User-Agent"),
		Accept:         r.Header.Get("Accept"),
		Headers:        r.Header,
		FormatOverride: formatOverride,
	}

	// Determine which format/processor to use based on client format
	// and create the processor on-demand
	var handler relay.OutputHandler

	// Determine effective format with priority:
	// 1. URL query param override (formatOverride)
	// 2. Specific HLS sub-format from client detection (preferredFormat like "hls-fmp4")
	// 3. Generic client format (clientFormat like "hls")
	effectiveFormat := string(clientFormat)
	if formatOverride != "" {
		effectiveFormat = formatOverride
	} else if preferredFormat != "" && (preferredFormat == relay.FormatValueHLSFMP4 || preferredFormat == relay.FormatValueHLSTS) {
		// Use the specific HLS sub-format from client detection
		effectiveFormat = preferredFormat
	}

	h.logger.Debug("Output format resolved",
		"effective_format", effectiveFormat,
		"client_format", clientFormat,
		"preferred_format", preferredFormat,
		"format_override", formatOverride,
		"variant", clientVariant.String(),
	)

	switch effectiveFormat {
	case relay.FormatValueHLS, relay.FormatValueHLSTS:
		// HLS-TS format - get or create HLS-TS processor for client's variant
		processor, err := session.GetOrCreateHLSTSProcessorForVariant(clientVariant)
		if err != nil {
			h.logger.Error("Failed to create HLS-TS processor",
				"session_id", session.ID,
				"variant", clientVariant.String(),
				"error", err,
			)
			http.Error(w, "HLS streaming not available", http.StatusServiceUnavailable)
			return
		}
		// Register client for tracking (will update existing or create new)
		_ = processor.RegisterClient(clientID, w, r)
		handler = relay.NewHLSHandlerWithVariant(processor, clientVariant.String())

	case relay.FormatValueFMP4, relay.FormatValueHLSFMP4:
		// HLS-fMP4/CMAF format - get or create HLS-fMP4 processor for client's variant
		fmp4Processor, err := session.GetOrCreateHLSfMP4ProcessorForVariant(clientVariant)
		if err != nil {
			h.logger.Error("Failed to create HLS-fMP4 processor",
				"session_id", session.ID,
				"variant", clientVariant.String(),
				"error", err,
			)
			http.Error(w, "HLS-fMP4 streaming not available", http.StatusServiceUnavailable)
			return
		}
		// Register client for tracking (will update existing or create new)
		_ = fmp4Processor.RegisterClient(clientID, w, r)
		handler = relay.NewHLSHandlerWithVariant(fmp4Processor, clientVariant.String())

		// For init segment requests, call processor directly to enable client tracking
		if outputReq.IsInitRequest() {
			if err := fmp4Processor.ServeSegment(w, r, "init.mp4"); err != nil {
				h.logger.Debug("Failed to serve init segment",
					"session_id", session.ID,
					"error", err,
				)
			}
			return
		}

	case relay.FormatValueDASH:
		// DASH format - get or create DASH processor for client's variant
		processor, err := session.GetOrCreateDASHProcessorForVariant(clientVariant)
		if err != nil {
			h.logger.Error("Failed to create DASH processor",
				"session_id", session.ID,
				"variant", clientVariant.String(),
				"error", err,
			)
			http.Error(w, "DASH streaming not available", http.StatusServiceUnavailable)
			return
		}
		// Register client for tracking (will update existing or create new)
		_ = processor.RegisterClient(clientID, w, r)
		handler = relay.NewDASHHandler(processor)

	default:
		// Default to HLS-TS for unknown formats
		processor, err := session.GetOrCreateHLSTSProcessorForVariant(clientVariant)
		if err != nil {
			h.logger.Error("Failed to create HLS-TS processor for default",
				"session_id", session.ID,
				"variant", clientVariant.String(),
				"error", err,
			)
			http.Error(w, "streaming not available", http.StatusServiceUnavailable)
			return
		}
		// Register client for tracking (will update existing or create new)
		_ = processor.RegisterClient(clientID, w, r)
		handler = relay.NewHLSHandlerWithVariant(processor, clientVariant.String())
	}

	// Build base URL for playlist (used to generate segment URLs)
	baseURL := h.buildBaseURL(r)

	// Dispatch based on request type
	if outputReq.IsInitRequest() {
		// Init segment request (for fMP4/CMAF)
		if hlsHandler, ok := handler.(*relay.HLSHandler); ok {
			if err := hlsHandler.ServeInitSegment(w); err != nil {
				h.logger.Debug("Failed to serve init segment",
					"session_id", session.ID,
					"error", err,
				)
			}
		} else if dashHandler, ok := handler.(*relay.DASHHandler); ok {
			// DASH init segment request - pass track type (video/audio) from URL
			// trackType comes from the "track" query param for CMAF with separate init segments
			if err := dashHandler.ServeInitSegment(w, trackType); err != nil {
				h.logger.Debug("Failed to serve DASH init segment",
					"session_id", session.ID,
					"track_type", trackType,
					"error", err,
				)
			}
		} else {
			http.Error(w, "init segment not available", http.StatusNotFound)
		}
	} else if outputReq.IsSegmentRequest() {
		// Segment request
		h.logger.Debug("Serving segment request",
			"session_id", session.ID,
			"segment", *outputReq.Segment,
			"format", effectiveFormat,
			"variant", clientVariant.String(),
			"track_type", trackType,
		)
		// For DASH, use track-filtered segment serving if track type is specified
		if dashHandler, ok := handler.(*relay.DASHHandler); ok && trackType != "" {
			if err := dashHandler.ServeSegmentFiltered(w, *outputReq.Segment, trackType); err != nil {
				h.logger.Warn("Failed to serve filtered DASH segment",
					"session_id", session.ID,
					"segment", *outputReq.Segment,
					"track_type", trackType,
					"error", err,
				)
			}
		} else if err := handler.ServeSegment(w, *outputReq.Segment); err != nil {
			h.logger.Warn("Failed to serve segment",
				"session_id", session.ID,
				"segment", *outputReq.Segment,
				"error", err,
			)
		}
	} else {
		// Playlist/manifest request
		// Use context-aware method if available to support waiting for segments
		if hlsHandler, ok := handler.(*relay.HLSHandler); ok {
			if err := hlsHandler.ServePlaylistWithContext(r.Context(), w, baseURL); err != nil {
				h.logger.Debug("Failed to serve playlist",
					"session_id", session.ID,
					"error", err,
				)
			}
		} else if dashHandler, ok := handler.(*relay.DASHHandler); ok {
			if err := dashHandler.ServePlaylistWithContext(r.Context(), w, baseURL); err != nil {
				h.logger.Debug("Failed to serve DASH manifest",
					"session_id", session.ID,
					"error", err,
				)
			}
		} else if err := handler.ServePlaylist(w, baseURL); err != nil {
			h.logger.Debug("Failed to serve playlist",
				"session_id", session.ID,
				"error", err,
			)
		}
	}
}

// buildBaseURL constructs the base URL for playlist segment references.
func (h *RelayStreamHandler) buildBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Check X-Forwarded-Proto header
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	host := r.Host
	// Check X-Forwarded-Host header
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, r.URL.Path)
}

// streamMPEGTSFromRelay streams MPEG-TS data directly from a relay session.
// This uses on-demand processor creation - the MPEG-TS processor is only started
// when a client actually requests this format.
func (h *RelayStreamHandler) streamMPEGTSFromRelay(w http.ResponseWriter, r *http.Request, session *relay.RelaySession, info *service.StreamInfo, targetVariant relay.CodecVariant) {
	connAttrs := []any{
		"session_id", session.ID,
		"channel_id", info.Channel.ID,
	}
	if info.Proxy != nil {
		connAttrs = append([]any{"proxy_id", info.Proxy.ID}, connAttrs...)
	}
	h.logger.Debug("Client connecting for MPEG-TS stream", connAttrs...)

	// Use the pre-computed target variant (determined by computeTargetVariant)
	clientVariant := targetVariant

	h.logger.Debug("MPEG-TS client using target variant",
		"session_id", session.ID,
		"variant", clientVariant.String(),
	)

	// Wait for the session pipeline to be ready before accessing processors
	if err := session.WaitReady(r.Context()); err != nil {
		h.logger.Error("Session not ready for MPEG-TS streaming",
			"session_id", session.ID,
			"error", err,
		)
		http.Error(w, "session not ready", http.StatusServiceUnavailable)
		return
	}

	// Clear any idle state since a client is actively connecting
	session.ClearIdleState()

	// Get or create the MPEG-TS processor for the client's variant on-demand
	// This enables per-client codec variants
	processor, err := session.GetOrCreateMPEGTSProcessorForVariant(clientVariant)
	if err != nil {
		h.logger.Error("Failed to create MPEG-TS processor",
			"session_id", session.ID,
			"error", err,
		)
		http.Error(w, "MPEG-TS streaming not available", http.StatusServiceUnavailable)
		return
	}

	// Create handler for this processor
	mpegtsHandler := relay.NewMPEGTSHandler(processor)

	h.logger.Debug("Serving MPEG-TS stream via on-demand processor",
		"session_id", session.ID,
	)

	// Serve the stream - this blocks until client disconnects
	if err := mpegtsHandler.ServeStreamWithRequest(w, r); err != nil {
		h.logger.Debug("MPEG-TS stream ended",
			"session_id", session.ID,
			"error", err,
		)
	}
}

// ProbeStreamInput is the input for probing a stream.
// Either URL or ChannelID must be provided. If ChannelID is provided, the channel's
// stream URL will be looked up from the database.
type ProbeStreamInput struct {
	Body struct {
		URL       string `json:"url,omitempty" doc:"Stream URL to probe (required if channel_id not provided)"`
		ChannelID string `json:"channel_id,omitempty" doc:"Channel ULID to probe (required if url not provided)"`
	}
}

// ProbeStreamOutput is the output for probing a stream.
type ProbeStreamOutput struct {
	Body struct {
		ChannelID       string  `json:"channel_id,omitempty" doc:"Channel ULID if probed by channel_id"`
		StreamURL       string  `json:"stream_url"`
		VideoCodec      string  `json:"video_codec,omitempty" doc:"Primary video codec (from selected track)"`
		VideoWidth      int     `json:"video_width,omitempty"`
		VideoHeight     int     `json:"video_height,omitempty"`
		VideoFramerate  float64 `json:"video_framerate,omitempty"`
		VideoBitrate    int     `json:"video_bitrate,omitempty"`
		AudioCodec      string  `json:"audio_codec,omitempty" doc:"Primary audio codec (from selected track)"`
		AudioSampleRate int     `json:"audio_sample_rate,omitempty"`
		AudioChannels   int     `json:"audio_channels,omitempty"`
		AudioBitrate    int     `json:"audio_bitrate,omitempty"`
		ContainerFormat string  `json:"container_format,omitempty" doc:"Container format (hls, mpegts, dash, etc)"`
		IsLiveStream    bool    `json:"is_live_stream" doc:"Whether stream is live (no duration)"`
		HasSubtitles    bool    `json:"has_subtitles" doc:"Whether subtitles are present"`
		StreamCount     int     `json:"stream_count" doc:"Total number of streams in container"`
		// All discovered tracks for display and selection
		VideoTracks        []ffmpeg.VideoTrackInfo    `json:"video_tracks,omitempty" doc:"All video tracks discovered"`
		AudioTracks        []ffmpeg.AudioTrackInfo    `json:"audio_tracks,omitempty" doc:"All audio tracks discovered"`
		SubtitleTracks     []ffmpeg.SubtitleTrackInfo `json:"subtitle_tracks,omitempty" doc:"All subtitle tracks discovered"`
		SelectedVideoTrack int                        `json:"selected_video_track" doc:"Index of selected video track (-1=auto)"`
		SelectedAudioTrack int                        `json:"selected_audio_track" doc:"Index of selected audio track (-1=auto)"`
	}
}

// ProbeStream probes a stream URL for codec information.
// Accepts either a URL directly or a channel_id to look up the URL from the database.
// Returns full track information including all discovered video, audio, and subtitle tracks.
func (h *RelayStreamHandler) ProbeStream(ctx context.Context, input *ProbeStreamInput) (*ProbeStreamOutput, error) {
	var streamURL string
	var channelIDStr string

	// Determine the stream URL - either from direct URL or channel lookup
	if input.Body.ChannelID != "" {
		// Parse and look up channel by ULID
		channelID, err := models.ParseULID(input.Body.ChannelID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid channel_id format")
		}

		channel, err := h.relayService.GetChannel(ctx, channelID)
		if err != nil {
			return nil, huma.Error404NotFound("channel not found")
		}

		streamURL = channel.StreamURL
		channelIDStr = input.Body.ChannelID
	} else if input.Body.URL != "" {
		streamURL = input.Body.URL
	} else {
		return nil, huma.Error400BadRequest("either url or channel_id must be provided")
	}

	// Use ProbeStreamFull to get all track information
	streamInfo, err := h.relayService.ProbeStreamFull(ctx, streamURL)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to probe stream", err)
	}

	// Also cache the result (ProbeStreamFull doesn't cache, so call ProbeStream for caching)
	// This is done asynchronously to not delay the response
	go func() {
		_, _ = h.relayService.ProbeStream(context.Background(), streamURL)
	}()

	return &ProbeStreamOutput{
		Body: struct {
			ChannelID          string                     `json:"channel_id,omitempty" doc:"Channel ULID if probed by channel_id"`
			StreamURL          string                     `json:"stream_url"`
			VideoCodec         string                     `json:"video_codec,omitempty" doc:"Primary video codec (from selected track)"`
			VideoWidth         int                        `json:"video_width,omitempty"`
			VideoHeight        int                        `json:"video_height,omitempty"`
			VideoFramerate     float64                    `json:"video_framerate,omitempty"`
			VideoBitrate       int                        `json:"video_bitrate,omitempty"`
			AudioCodec         string                     `json:"audio_codec,omitempty" doc:"Primary audio codec (from selected track)"`
			AudioSampleRate    int                        `json:"audio_sample_rate,omitempty"`
			AudioChannels      int                        `json:"audio_channels,omitempty"`
			AudioBitrate       int                        `json:"audio_bitrate,omitempty"`
			ContainerFormat    string                     `json:"container_format,omitempty" doc:"Container format (hls, mpegts, dash, etc)"`
			IsLiveStream       bool                       `json:"is_live_stream" doc:"Whether stream is live (no duration)"`
			HasSubtitles       bool                       `json:"has_subtitles" doc:"Whether subtitles are present"`
			StreamCount        int                        `json:"stream_count" doc:"Total number of streams in container"`
			VideoTracks        []ffmpeg.VideoTrackInfo    `json:"video_tracks,omitempty" doc:"All video tracks discovered"`
			AudioTracks        []ffmpeg.AudioTrackInfo    `json:"audio_tracks,omitempty" doc:"All audio tracks discovered"`
			SubtitleTracks     []ffmpeg.SubtitleTrackInfo `json:"subtitle_tracks,omitempty" doc:"All subtitle tracks discovered"`
			SelectedVideoTrack int                        `json:"selected_video_track" doc:"Index of selected video track (-1=auto)"`
			SelectedAudioTrack int                        `json:"selected_audio_track" doc:"Index of selected audio track (-1=auto)"`
		}{
			ChannelID:          channelIDStr,
			StreamURL:          streamURL,
			VideoCodec:         streamInfo.VideoCodec,
			VideoWidth:         streamInfo.VideoWidth,
			VideoHeight:        streamInfo.VideoHeight,
			VideoFramerate:     streamInfo.VideoFramerate,
			VideoBitrate:       streamInfo.VideoBitrate,
			AudioCodec:         streamInfo.AudioCodec,
			AudioSampleRate:    streamInfo.AudioSampleRate,
			AudioChannels:      streamInfo.AudioChannels,
			AudioBitrate:       streamInfo.AudioBitrate,
			ContainerFormat:    streamInfo.ContainerFormat,
			IsLiveStream:       streamInfo.IsLiveStream,
			HasSubtitles:       streamInfo.HasSubtitles,
			StreamCount:        streamInfo.StreamCount,
			VideoTracks:        streamInfo.VideoTracks,
			AudioTracks:        streamInfo.AudioTracks,
			SubtitleTracks:     streamInfo.SubtitleTracks,
			SelectedVideoTrack: streamInfo.SelectedVideoTrack,
			SelectedAudioTrack: streamInfo.SelectedAudioTrack,
		},
	}, nil
}

// ClassifyStreamInput is the input for classifying a stream.
type ClassifyStreamInput struct {
	Body struct {
		URL string `json:"url" required:"true" doc:"Stream URL to classify"`
	}
}

// ClassifyStreamOutput is the output for classifying a stream.
type ClassifyStreamOutput struct {
	Body struct {
		URL                   string   `json:"url"`
		Mode                  string   `json:"mode"`
		VariantCount          int      `json:"variant_count,omitempty"`
		TargetDuration        float64  `json:"target_duration,omitempty"`
		IsEncrypted           bool     `json:"is_encrypted,omitempty"`
		UsesFMP4              bool     `json:"uses_fmp4,omitempty"`
		EligibleForCollapse   bool     `json:"eligible_for_collapse"`
		SelectedMediaPlaylist string   `json:"selected_media_playlist,omitempty"`
		SelectedBandwidth     int64    `json:"selected_bandwidth,omitempty"`
		Reasons               []string `json:"reasons,omitempty"`
	}
}

// ClassifyStream classifies a stream URL to determine processing mode.
func (h *RelayStreamHandler) ClassifyStream(ctx context.Context, input *ClassifyStreamInput) (*ClassifyStreamOutput, error) {
	result := h.relayService.ClassifyStream(ctx, input.Body.URL)

	return &ClassifyStreamOutput{
		Body: struct {
			URL                   string   `json:"url"`
			Mode                  string   `json:"mode"`
			VariantCount          int      `json:"variant_count,omitempty"`
			TargetDuration        float64  `json:"target_duration,omitempty"`
			IsEncrypted           bool     `json:"is_encrypted,omitempty"`
			UsesFMP4              bool     `json:"uses_fmp4,omitempty"`
			EligibleForCollapse   bool     `json:"eligible_for_collapse"`
			SelectedMediaPlaylist string   `json:"selected_media_playlist,omitempty"`
			SelectedBandwidth     int64    `json:"selected_bandwidth,omitempty"`
			Reasons               []string `json:"reasons,omitempty"`
		}{
			URL:                   input.Body.URL,
			Mode:                  result.Mode.String(),
			VariantCount:          result.VariantCount,
			TargetDuration:        result.TargetDuration,
			IsEncrypted:           result.IsEncrypted,
			UsesFMP4:              result.UsesFMP4,
			EligibleForCollapse:   result.EligibleForCollapse,
			SelectedMediaPlaylist: result.SelectedMediaPlaylist,
			SelectedBandwidth:     result.SelectedBandwidth,
			Reasons:               result.Reasons,
		},
	}, nil
}

// GetCodecCacheStatsInput is the input for getting codec cache stats.
type GetCodecCacheStatsInput struct{}

// GetCodecCacheStatsOutput is the output for getting codec cache stats.
type GetCodecCacheStatsOutput struct {
	Body struct {
		TotalEntries   int64 `json:"total_entries"`
		ValidEntries   int64 `json:"valid_entries"`
		ExpiredEntries int64 `json:"expired_entries"`
		ErrorEntries   int64 `json:"error_entries"`
		TotalHits      int64 `json:"total_hits"`
	}
}

// GetCodecCacheStats returns statistics about the codec cache.
func (h *RelayStreamHandler) GetCodecCacheStats(ctx context.Context, input *GetCodecCacheStatsInput) (*GetCodecCacheStatsOutput, error) {
	stats, err := h.relayService.GetCodecCacheStats(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get codec cache stats", err)
	}

	return &GetCodecCacheStatsOutput{
		Body: struct {
			TotalEntries   int64 `json:"total_entries"`
			ValidEntries   int64 `json:"valid_entries"`
			ExpiredEntries int64 `json:"expired_entries"`
			ErrorEntries   int64 `json:"error_entries"`
			TotalHits      int64 `json:"total_hits"`
		}{
			TotalEntries:   stats.TotalEntries,
			ValidEntries:   stats.ValidEntries,
			ExpiredEntries: stats.ExpiredEntries,
			ErrorEntries:   stats.ErrorEntries,
			TotalHits:      stats.TotalHits,
		},
	}, nil
}

// ClearCodecCacheInput is the input for clearing codec cache.
type ClearCodecCacheInput struct{}

// ClearCodecCacheOutput is the output for clearing codec cache.
type ClearCodecCacheOutput struct {
	Body struct {
		DeletedCount int64  `json:"deleted_count" doc:"Number of cache entries deleted"`
		Message      string `json:"message" doc:"Status message"`
	}
}

// ClearCodecCache clears all codec cache entries.
func (h *RelayStreamHandler) ClearCodecCache(ctx context.Context, input *ClearCodecCacheInput) (*ClearCodecCacheOutput, error) {
	count, err := h.relayService.ClearAllCodecCache(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to clear codec cache", err)
	}

	return &ClearCodecCacheOutput{
		Body: struct {
			DeletedCount int64  `json:"deleted_count" doc:"Number of cache entries deleted"`
			Message      string `json:"message" doc:"Status message"`
		}{
			DeletedCount: count,
			Message:      "Codec cache cleared successfully. Streams will be re-probed on next request.",
		},
	}, nil
}

// StreamChannelByProxyInput is the input for streaming a channel through a specific proxy.
type StreamChannelByProxyInput struct {
	ProxyID   string `path:"proxyId" doc:"Stream Proxy ID (ULID)"`
	ChannelID string `path:"channelId" doc:"Channel ID (ULID)"`
}

// StreamChannelByProxyOptionsInput is the input for CORS preflight requests.
type StreamChannelByProxyOptionsInput struct {
	ProxyID   string `path:"proxyId" doc:"Stream Proxy ID (ULID)"`
	ChannelID string `path:"channelId" doc:"Channel ID (ULID)"`
}

// StreamChannelByProxyOptionsOutput is the output for CORS preflight requests.
type StreamChannelByProxyOptionsOutput struct {
	Body struct{} `json:"-"`
}

// ListRelaySessionsInput is the input for listing relay sessions.
type ListRelaySessionsInput struct{}

// ListRelaySessionsOutput is the output for listing relay sessions.
// It returns the complete flow graph for visualization.
type ListRelaySessionsOutput struct {
	Body relay.RelayFlowGraph
}

// ListRelaySessions returns all active relay sessions as a flow graph for visualization.
func (h *RelayStreamHandler) ListRelaySessions(ctx context.Context, input *ListRelaySessionsInput) (*ListRelaySessionsOutput, error) {
	// Get manager stats which includes session information
	stats := h.relayService.GetRelayStats()

	// Convert SessionStats to RelaySessionInfo for flow visualization
	sessions := make([]relay.RelaySessionInfo, 0, len(stats.Sessions))
	for _, sessionStats := range stats.Sessions {
		sessions = append(sessions, sessionStats.ToSessionInfo())
	}

	// Build the flow graph from sessions
	builder := relay.NewFlowBuilder()
	flowGraph := builder.BuildFlowGraph(sessions)

	return &ListRelaySessionsOutput{
		Body: flowGraph,
	}, nil
}
