package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/service"
)

// RelayStreamHandler handles stream relay API endpoints.
type RelayStreamHandler struct {
	relayService          *service.RelayService
	profileMappingService *service.RelayProfileMappingService
	logger                *slog.Logger
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

// WithProfileMappingService sets the profile mapping service for auto-detection.
func (h *RelayStreamHandler) WithProfileMappingService(svc *service.RelayProfileMappingService) *RelayStreamHandler {
	h.profileMappingService = svc
	return h
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
		Summary:     "Probe a stream URL",
		Description: "Probes a stream URL to detect codec information",
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
}

// RegisterChiRoutes registers streaming routes as raw Chi handlers.
// This is necessary because Huma's StreamResponse doesn't support HTTP 302 redirects
// or setting CORS headers before the response body is written.
func (h *RelayStreamHandler) RegisterChiRoutes(router chi.Router) {
	router.Get("/proxy/{proxyId}/{channelId}", h.handleRawStream)
	router.Options("/proxy/{proxyId}/{channelId}", h.handleRawStreamOptions)
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
- X-Stream-Origin-Kind: REDIRECT or SMART
- X-Stream-Decision: direct, passthrough, repackage, or transcode
- X-Stream-Mode: The proxy mode used (direct or smart)
- Access-Control-Allow-Origin: * (for smart mode)`,
		Tags: []string{"Stream Proxy"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Stream content (smart mode)",
				Headers: map[string]*huma.Param{
					"Content-Type":                 {Description: "video/mp2t, application/vnd.apple.mpegurl, or upstream content type"},
					"X-Stream-Origin-Kind":         {Description: "SMART"},
					"X-Stream-Decision":            {Description: "Processing decision made (passthrough, repackage, transcode)"},
					"X-Stream-Mode":                {Description: "smart"},
					"Access-Control-Allow-Origin":  {Description: "CORS header (always *)"},
					"Access-Control-Allow-Methods": {Description: "Allowed HTTP methods"},
				},
			},
			"302": {
				Description: "Redirect to source stream (direct mode)",
				Headers: map[string]*huma.Param{
					"Location":             {Description: "Source stream URL"},
					"X-Stream-Origin-Kind": {Description: "REDIRECT"},
					"X-Stream-Mode":        {Description: "direct"},
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

// parseFormatParams parses the format-related query parameters from a request.
// Returns an OutputRequest with the parsed format, segment number, and init type.
func parseFormatParams(r *http.Request) relay.OutputRequest {
	q := r.URL.Query()

	req := relay.OutputRequest{
		Format:    q.Get(relay.QueryParamFormat),
		UserAgent: r.Header.Get("User-Agent"),
		Accept:    r.Header.Get("Accept"),
	}

	// Parse segment number if present
	if segStr := q.Get(relay.QueryParamSegment); segStr != "" {
		if seg, err := strconv.ParseUint(segStr, 10, 64); err == nil {
			req.Segment = &seg
		}
	}

	// Parse init type if present (for DASH initialization segments)
	req.InitType = q.Get(relay.QueryParamInit)

	return req
}

// handleRawStream is the main raw HTTP handler for streaming.
// It dispatches to mode-specific handlers based on proxy configuration.
func (h *RelayStreamHandler) handleRawStream(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("X-Stream-Origin-Kind", "REDIRECT")
	w.Header().Set("X-Stream-Decision", "direct")
	w.Header().Set("X-Stream-Mode", "direct")
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
	serviceClassification := h.relayService.ClassifyStream(ctx, streamURL)

	// Convert service classification to relay classification for SelectDelivery
	classification := relay.ClassificationResult{
		Mode:                  serviceClassification.Mode,
		SourceFormat:          serviceClassification.SourceFormat,
		VariantCount:          serviceClassification.VariantCount,
		TargetDuration:        serviceClassification.TargetDuration,
		IsEncrypted:           serviceClassification.IsEncrypted,
		UsesFMP4:              serviceClassification.UsesFMP4,
		EligibleForCollapse:   serviceClassification.EligibleForCollapse,
		SelectedMediaPlaylist: serviceClassification.SelectedMediaPlaylist,
		SelectedBandwidth:     serviceClassification.SelectedBandwidth,
		Reasons:               serviceClassification.Reasons,
	}

	// Determine client's desired format from query param or Accept header
	clientFormat := h.resolveClientFormat(r, classification)

	// Get delivery decision using the smart delivery logic
	deliveryDecision := relay.SelectDelivery(classification, clientFormat, info.Profile)

	h.logger.Info("Smart mode: delivery decision",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
		"source_format", classification.SourceFormat,
		"client_format", clientFormat,
		"decision", deliveryDecision.String(),
	)

	// Dispatch based on delivery decision
	switch deliveryDecision {
	case relay.DeliveryPassthrough:
		h.handleSmartPassthrough(w, r, info, &serviceClassification)

	case relay.DeliveryRepackage:
		h.handleSmartRepackage(w, r, info, &serviceClassification, clientFormat)

	case relay.DeliveryTranscode:
		h.handleSmartTranscode(w, r, info, clientFormat)
	}
}

// resolveClientFormat determines the client's desired output format.
// It checks the ?format= query parameter first, then falls back to Accept header.
func (h *RelayStreamHandler) resolveClientFormat(r *http.Request, source relay.ClassificationResult) relay.ClientFormat {
	// Check explicit format query parameter
	formatParam := r.URL.Query().Get(relay.QueryParamFormat)
	switch formatParam {
	case relay.FormatValueHLS:
		return relay.ClientFormatHLS
	case relay.FormatValueDASH:
		return relay.ClientFormatDASH
	case relay.FormatValueMPEGTS:
		return relay.ClientFormatMPEGTS
	}

	// Check Accept header for format hints
	accept := r.Header.Get("Accept")
	if accept != "" {
		// HLS: application/vnd.apple.mpegurl or application/x-mpegURL
		if contains(accept, "mpegurl") || contains(accept, "x-mpegURL") {
			return relay.ClientFormatHLS
		}
		// DASH: application/dash+xml
		if contains(accept, "dash+xml") {
			return relay.ClientFormatDASH
		}
		// MPEG-TS: video/mp2t
		if contains(accept, "video/mp2t") {
			return relay.ClientFormatMPEGTS
		}
	}

	// Default to auto (serve source format as-is)
	return relay.ClientFormatAuto
}

// contains checks if substr is in s (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if matchLower(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func matchLower(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// resolveProfileWithAutoDetection resolves a profile's auto codecs using client detection.
// If the profile has auto video/audio codecs and the profile mapping service is configured,
// it evaluates the HTTP request against enabled mappings to determine the actual codecs.
// Returns a copy of the profile with resolved codecs, or the original if no auto-detection needed.
func (h *RelayStreamHandler) resolveProfileWithAutoDetection(ctx context.Context, r *http.Request, info *service.StreamInfo) *models.RelayProfile {
	profile := info.Profile
	if profile == nil {
		return nil
	}

	// Check if profile has auto codecs that need resolution
	hasAutoVideo := profile.VideoCodec == models.VideoCodecAuto
	hasAutoAudio := profile.AudioCodec == models.AudioCodecAuto

	if !hasAutoVideo && !hasAutoAudio {
		// No auto codecs, return original profile
		return profile
	}

	// Get client IP for logging
	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.Header.Get("X-Real-IP")
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	// Get channel name for logging context
	channelName := ""
	if info.Channel != nil {
		channelName = info.Channel.ChannelName
	}

	// Get cached codec info for source comparison (enables passthrough when client supports source codecs)
	var sourceCodecs *service.CodecInfo
	if info.Channel != nil && info.Channel.StreamURL != "" {
		cachedCodec, err := h.relayService.GetLastKnownCodec(ctx, info.Channel.StreamURL)
		if err == nil && cachedCodec != nil && cachedCodec.IsValid() {
			sourceCodecs = &service.CodecInfo{
				VideoCodec: models.VideoCodec(cachedCodec.VideoCodec),
				AudioCodec: models.AudioCodec(cachedCodec.AudioCodec),
				Container:  models.ContainerFormat(cachedCodec.ContainerFormat),
			}
			h.logger.Debug("Client detection: using cached source codecs",
				"channel", channelName,
				"video_codec", cachedCodec.VideoCodec,
				"audio_codec", cachedCodec.AudioCodec,
				"container", cachedCodec.ContainerFormat,
			)
		}
	}

	// Check if mapping service is available
	if h.profileMappingService == nil {
		h.logger.Warn("Profile has auto codecs but mapping service not configured, using defaults",
			"profile_id", profile.ID,
			"profile_name", profile.Name,
		)
		// Fall back to copy mode for auto codecs
		resolved := *profile
		if hasAutoVideo {
			resolved.VideoCodec = models.VideoCodecCopy
		}
		if hasAutoAudio {
			resolved.AudioCodec = models.AudioCodecCopy
		}
		return &resolved
	}

	// Evaluate request against mappings (pass source codecs to enable passthrough when client supports them)
	decision, err := h.profileMappingService.EvaluateRequest(ctx, r, sourceCodecs)
	if err != nil {
		h.logger.Warn("Failed to evaluate client detection, using copy mode",
			"profile_id", profile.ID,
			"error", err,
		)
		resolved := *profile
		if hasAutoVideo {
			resolved.VideoCodec = models.VideoCodecCopy
		}
		if hasAutoAudio {
			resolved.AudioCodec = models.AudioCodecCopy
		}
		return &resolved
	}

	if decision == nil {
		// No matching rule found, use copy mode as safe default
		resolved := *profile
		if hasAutoVideo {
			resolved.VideoCodec = models.VideoCodecCopy
		}
		if hasAutoAudio {
			resolved.AudioCodec = models.AudioCodecCopy
		}

		h.logger.Info("Client detection: no matching rule, using copy mode",
			"channel", channelName,
			"profile", profile.Name,
			"client_ip", clientIP,
			"user_agent", r.UserAgent(),
			"resolved_video", string(resolved.VideoCodec),
			"resolved_audio", string(resolved.AudioCodec),
		)

		return &resolved
	}

	// Apply the decision to create a resolved profile copy
	resolved := *profile
	if hasAutoVideo {
		resolved.VideoCodec = decision.TargetVideoCodec
	}
	if hasAutoAudio {
		resolved.AudioCodec = decision.TargetAudioCodec
	}

	// Build log message with source codec info if available
	logAttrs := []any{
		"channel", channelName,
		"profile", profile.Name,
		"client_ip", clientIP,
		"matched_rule", decision.MappingName,
		"video_action", decision.VideoAction,
		"audio_action", decision.AudioAction,
		"resolved_video", string(resolved.VideoCodec),
		"resolved_audio", string(resolved.AudioCodec),
		"user_agent", r.UserAgent(),
	}
	if sourceCodecs != nil {
		logAttrs = append(logAttrs,
			"source_video", string(sourceCodecs.VideoCodec),
			"source_audio", string(sourceCodecs.AudioCodec),
		)
	}
	h.logger.Info("Client detection: matched rule", logAttrs...)

	return &resolved
}

// handleSmartPassthrough serves the source stream as-is (passthrough mode).
// This is used when source format matches client format.
func (h *RelayStreamHandler) handleSmartPassthrough(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult) {
	// Set common headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("X-Stream-Origin-Kind", "SMART")
	w.Header().Set("X-Stream-Mode", "smart")
	w.Header().Set("X-Stream-Decision", "passthrough")

	h.logger.Info("Smart mode: passthrough delivery",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"source_format", classification.SourceFormat,
	)

	// Reuse the direct proxy streaming logic
	h.streamRawDirectProxy(w, r, info, classification)
}

// handleSmartRepackage repackages the source stream to a different manifest format.
// This is used for HLS↔DASH conversion without re-encoding.
func (h *RelayStreamHandler) handleSmartRepackage(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult, clientFormat relay.ClientFormat) {
	// Set common headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("X-Stream-Origin-Kind", "SMART")
	w.Header().Set("X-Stream-Mode", "smart")
	w.Header().Set("X-Stream-Decision", "repackage")

	h.logger.Info("Smart mode: repackage delivery",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"source_format", classification.SourceFormat,
		"target_format", clientFormat,
	)

	// TODO: Implement manifest repackaging (HLS↔DASH)
	// For now, fall back to passthrough with a warning
	h.logger.Warn("Smart mode: repackage not yet implemented, falling back to passthrough",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
	)
	h.streamRawDirectProxy(w, r, info, classification)
}

// handleSmartTranscode transcodes the source stream using FFmpeg.
// This is used when codec conversion is needed or when creating segments from raw TS.
func (h *RelayStreamHandler) handleSmartTranscode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, clientFormat relay.ClientFormat) {
	ctx := r.Context()

	// Set common headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("X-Stream-Origin-Kind", "SMART")
	w.Header().Set("X-Stream-Mode", "smart")
	w.Header().Set("X-Stream-Decision", "transcode")

	h.logger.Info("Smart mode: transcode delivery",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"client_format", clientFormat,
	)

	// Resolve profile with auto-detection if needed
	resolvedProfile := h.resolveProfileWithAutoDetection(ctx, r, info)

	// Start or join the relay session with the resolved profile
	var session *relay.RelaySession
	var err error
	if resolvedProfile != nil {
		session, err = h.relayService.StartRelayWithProfile(ctx, info.Channel.ID, resolvedProfile)
	} else {
		// No profile, use passthrough
		session, err = h.relayService.StartRelay(ctx, info.Channel.ID, nil)
	}
	if err != nil {
		h.logger.Error("Failed to start relay session for smart transcode",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		http.Error(w, "failed to start relay session", http.StatusInternalServerError)
		return
	}

	// Get request info for client registration
	userAgent := r.Header.Get("User-Agent")
	remoteAddr := r.Header.Get("X-Forwarded-For")
	if remoteAddr == "" {
		remoteAddr = r.RemoteAddr
	}

	// Add client to the session
	client, reader, err := h.relayService.AddRelayClient(session.ID, userAgent, remoteAddr)
	if err != nil {
		h.logger.Error("Failed to add relay client for smart transcode",
			"session_id", session.ID,
			"error", err,
		)
		http.Error(w, "failed to add relay client", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Client connected to smart transcode relay",
		"session_id", session.ID,
		"client_id", client.ID,
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"client_format", clientFormat,
	)

	// Set content type based on client format
	contentType := relay.ContentTypeMPEGTS
	switch clientFormat {
	case relay.ClientFormatHLS:
		contentType = relay.ContentTypeHLSPlaylist
	case relay.ClientFormatDASH:
		contentType = relay.ContentTypeDASHManifest
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	// Stream data to client
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				h.logger.Debug("Client disconnected during smart transcode write",
					"session_id", session.ID,
					"client_id", client.ID,
					"error", writeErr,
				)
				break
			}
			// Flush if possible
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				h.logger.Debug("Reader error during smart transcode",
					"session_id", session.ID,
					"client_id", client.ID,
					"error", err,
				)
			}
			break
		}
	}

	// Clean up client
	if removeErr := h.relayService.RemoveRelayClient(session.ID, client.ID); removeErr != nil {
		h.logger.Warn("Failed to remove relay client from smart transcode",
			"session_id", session.ID,
			"client_id", client.ID,
			"error", removeErr,
		)
	}

	h.logger.Info("Client disconnected from smart transcode relay",
		"session_id", session.ID,
		"client_id", client.ID,
	)
}

// streamRawHLSCollapsed streams collapsed HLS as continuous MPEG-TS via raw ResponseWriter.
func (h *RelayStreamHandler) streamRawHLSCollapsed(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult) {
	ctx := r.Context()
	playlistURL := classification.SelectedMediaPlaylist
	if playlistURL == "" {
		playlistURL = info.Channel.StreamURL
	}

	h.logger.Info("Proxy mode: starting HLS collapse",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"playlist_url", playlistURL,
		"bandwidth", classification.SelectedBandwidth,
	)

	w.Header().Set("X-Stream-Decision", "collapsed-hls")
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	// Create HLS collapser
	collapser := h.relayService.CreateHLSCollapser(playlistURL)

	// Start the collapser
	if err := collapser.Start(ctx); err != nil {
		h.logger.Error("Failed to start HLS collapser",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		return
	}
	defer collapser.Stop()

	h.logger.Info("HLS collapser started",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"session_id", collapser.SessionID(),
	)

	// Stream data to client
	buf := make([]byte, 64*1024) // 64KB buffer for larger HLS segments
	for {
		n, err := collapser.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				h.logger.Debug("Client disconnected during HLS collapse write",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", writeErr,
				)
				break
			}
			// Flush if possible
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				h.logger.Debug("HLS collapser error",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", err,
				)
			}
			break
		}
	}

	h.logger.Info("HLS collapse stream ended",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
	)
}

// streamRawDirectProxy streams content directly from upstream with CORS headers.
func (h *RelayStreamHandler) streamRawDirectProxy(w http.ResponseWriter, r *http.Request, info *service.StreamInfo, classification *service.ClassificationResult) {
	streamURL := info.Channel.StreamURL

	h.logger.Info("Proxy mode: direct proxy",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
		"stream_mode", classification.Mode.String(),
	)

	w.Header().Set("X-Stream-Decision", "direct-proxy")

	// Create HTTP request to upstream
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		h.logger.Error("Failed to create upstream request",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		http.Error(w, "failed to create upstream request", http.StatusBadGateway)
		return
	}

	// Forward relevant headers from client
	if ua := r.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// Execute request
	client := h.relayService.GetHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Upstream request failed",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Set content type from upstream or default to video/mp2t
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp2t"
	}
	w.Header().Set("Content-Type", contentType)

	// Forward content length if available
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}

	// Forward content range for partial content
	if resp.StatusCode == http.StatusPartialContent {
		if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
			w.Header().Set("Content-Range", contentRange)
		}
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
	}

	// Stream data to client
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				h.logger.Debug("Client disconnected during proxy write",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", writeErr,
				)
				break
			}
			// Flush if possible
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				h.logger.Debug("Upstream read error",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", err,
				)
			}
			break
		}
	}

	h.logger.Info("Proxy stream ended",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
	)
}


// ProbeStreamInput is the input for probing a stream.
type ProbeStreamInput struct {
	Body struct {
		URL string `json:"url" required:"true" doc:"Stream URL to probe"`
	}
}

// ProbeStreamOutput is the output for probing a stream.
type ProbeStreamOutput struct {
	Body struct {
		StreamURL       string  `json:"stream_url"`
		VideoCodec      string  `json:"video_codec,omitempty"`
		VideoWidth      int     `json:"video_width,omitempty"`
		VideoHeight     int     `json:"video_height,omitempty"`
		VideoFramerate  float64 `json:"video_framerate,omitempty"`
		VideoBitrate    int     `json:"video_bitrate,omitempty"`
		AudioCodec      string  `json:"audio_codec,omitempty"`
		AudioSampleRate int     `json:"audio_sample_rate,omitempty"`
		AudioChannels   int     `json:"audio_channels,omitempty"`
		AudioBitrate    int     `json:"audio_bitrate,omitempty"`
	}
}

// ProbeStream probes a stream URL for codec information.
func (h *RelayStreamHandler) ProbeStream(ctx context.Context, input *ProbeStreamInput) (*ProbeStreamOutput, error) {
	codec, err := h.relayService.ProbeStream(ctx, input.Body.URL)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to probe stream", err)
	}

	return &ProbeStreamOutput{
		Body: struct {
			StreamURL       string  `json:"stream_url"`
			VideoCodec      string  `json:"video_codec,omitempty"`
			VideoWidth      int     `json:"video_width,omitempty"`
			VideoHeight     int     `json:"video_height,omitempty"`
			VideoFramerate  float64 `json:"video_framerate,omitempty"`
			VideoBitrate    int     `json:"video_bitrate,omitempty"`
			AudioCodec      string  `json:"audio_codec,omitempty"`
			AudioSampleRate int     `json:"audio_sample_rate,omitempty"`
			AudioChannels   int     `json:"audio_channels,omitempty"`
			AudioBitrate    int     `json:"audio_bitrate,omitempty"`
		}{
			StreamURL:       codec.StreamURL,
			VideoCodec:      codec.VideoCodec,
			VideoWidth:      codec.VideoWidth,
			VideoHeight:     codec.VideoHeight,
			VideoFramerate:  codec.VideoFramerate,
			VideoBitrate:    codec.VideoBitrate,
			AudioCodec:      codec.AudioCodec,
			AudioSampleRate: codec.AudioSampleRate,
			AudioChannels:   codec.AudioChannels,
			AudioBitrate:    codec.AudioBitrate,
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

// StreamChannelByProxyOptions handles CORS preflight requests for the stream endpoint.
func (h *RelayStreamHandler) StreamChannelByProxyOptions(ctx context.Context, input *StreamChannelByProxyOptionsInput) (*StreamChannelByProxyOptionsOutput, error) {
	// CORS preflight response is handled by setting headers
	// The actual CORS headers are set in the StreamChannelByProxy handler
	return &StreamChannelByProxyOptionsOutput{}, nil
}


