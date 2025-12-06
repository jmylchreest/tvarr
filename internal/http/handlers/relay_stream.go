package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// RelayStreamHandler handles stream relay API endpoints.
type RelayStreamHandler struct {
	relayService *service.RelayService
	logger       *slog.Logger
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

// Register registers the relay stream routes with the API (Huma routes).
// Note: The /proxy/{proxyId}/{channelId} streaming endpoints are registered
// via RegisterChiRoutes for raw HTTP handler access (needed for 302 redirects, CORS).
func (h *RelayStreamHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:      "streamChannel",
		Method:           "GET",
		Path:             "/relay/stream/{channelId}",
		Summary:          "Stream a channel",
		Description:      "Starts streaming a channel through the relay system",
		Tags:             []string{"Stream Relay"},
		SkipValidateBody: true,
	}, h.StreamChannel)

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
		Path:        "/relay/probe",
		Summary:     "Probe a stream URL",
		Description: "Probes a stream URL to detect codec information",
		Tags:        []string{"Stream Relay"},
	}, h.ProbeStream)

	huma.Register(api, huma.Operation{
		OperationID: "classifyStream",
		Method:      "POST",
		Path:        "/relay/classify",
		Summary:     "Classify a stream URL",
		Description: "Classifies a stream URL to determine processing mode",
		Tags:        []string{"Stream Relay"},
	}, h.ClassifyStream)

	huma.Register(api, huma.Operation{
		OperationID: "getCodecCache",
		Method:      "GET",
		Path:        "/relay/codecs",
		Summary:     "Get codec cache stats",
		Description: "Returns statistics about the codec cache",
		Tags:        []string{"Stream Relay"},
	}, h.GetCodecCacheStats)
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
- **redirect**: Returns HTTP 302 redirect to the source stream URL (zero overhead)
- **proxy**: Fetches upstream content and forwards with CORS headers, optional HLS collapse
- **relay**: FFmpeg transcoding with cyclic buffer for multi-client sharing

**Response Headers:**
- X-Stream-Origin-Kind: REDIRECT, PROXY, or RELAY
- X-Stream-Decision: redirect, direct-proxy, collapsed-hls, or transcoded
- X-Stream-Mode: The proxy mode used
- Access-Control-Allow-Origin: * (for proxy and relay modes)`,
		Tags: []string{"Stream Proxy"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Stream content (proxy or relay mode)",
				Headers: map[string]*huma.Param{
					"Content-Type":                 {Description: "video/mp2t or upstream content type"},
					"X-Stream-Origin-Kind":         {Description: "PROXY or RELAY"},
					"X-Stream-Decision":            {Description: "Processing decision made"},
					"X-Stream-Mode":                {Description: "Proxy mode used"},
					"Access-Control-Allow-Origin":  {Description: "CORS header (always *)"},
					"Access-Control-Allow-Methods": {Description: "Allowed HTTP methods"},
				},
			},
			"302": {
				Description: "Redirect to source stream (redirect mode)",
				Headers: map[string]*huma.Param{
					"Location":             {Description: "Source stream URL"},
					"X-Stream-Origin-Kind": {Description: "REDIRECT"},
					"X-Stream-Mode":        {Description: "redirect"},
				},
			},
			"400": {Description: "Invalid proxy or channel ID format"},
			"404": {Description: "Stream proxy or channel not found"},
			"500": {Description: "Internal server error"},
			"502": {Description: "Upstream server error (proxy mode)"},
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
	case models.StreamProxyModeRedirect:
		h.handleRawRedirectMode(w, r, streamInfo)

	case models.StreamProxyModeProxy:
		h.handleRawProxyMode(w, r, streamInfo)

	case models.StreamProxyModeRelay:
		h.handleRawRelayMode(w, r, streamInfo)

	default:
		h.logger.Error("Unknown proxy mode",
			"proxy_id", proxyIDStr,
			"mode", streamInfo.Proxy.ProxyMode,
		)
		http.Error(w, fmt.Sprintf("unknown proxy mode: %s", streamInfo.Proxy.ProxyMode), http.StatusInternalServerError)
	}
}

// handleRawRedirectMode returns an HTTP 302 redirect to the source stream URL.
func (h *RelayStreamHandler) handleRawRedirectMode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo) {
	streamURL := info.Channel.StreamURL

	h.logger.Info("Redirect mode: sending 302 redirect",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
	)

	w.Header().Set("Location", streamURL)
	w.Header().Set("X-Stream-Origin-Kind", "REDIRECT")
	w.Header().Set("X-Stream-Decision", "redirect")
	w.Header().Set("X-Stream-Mode", "redirect")
	w.WriteHeader(http.StatusFound)
}

// handleRawProxyMode fetches upstream content and streams it with CORS headers.
func (h *RelayStreamHandler) handleRawProxyMode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo) {
	ctx := r.Context()
	streamURL := info.Channel.StreamURL

	// Classify the stream to determine optimal handling
	classification := h.relayService.ClassifyStream(ctx, streamURL)

	h.logger.Info("Proxy mode: stream classified",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
		"mode", classification.Mode.String(),
		"eligible_for_collapse", classification.EligibleForCollapse,
	)

	// Set CORS headers first (before any writes)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("X-Stream-Origin-Kind", "PROXY")
	w.Header().Set("X-Stream-Mode", "proxy")

	// Check if HLS collapse is enabled and stream is eligible
	hlsCollapseEnabled := info.Proxy.HLSCollapse
	shouldCollapse := hlsCollapseEnabled && classification.EligibleForCollapse

	// Handle based on classification
	if shouldCollapse && classification.SelectedMediaPlaylist != "" {
		// HLS collapse mode - convert multi-variant HLS to continuous TS
		h.streamRawHLSCollapsed(w, r, info, &classification)
	} else {
		// Direct proxy mode - simple passthrough
		h.streamRawDirectProxy(w, r, info, &classification)
	}
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

// handleRawRelayMode starts or joins a relay session for FFmpeg transcoding.
func (h *RelayStreamHandler) handleRawRelayMode(w http.ResponseWriter, r *http.Request, info *service.StreamInfo) {
	ctx := r.Context()

	// Determine profile ID to use
	var profileID *models.ULID
	if info.Profile != nil {
		profileID = &info.Profile.ID
	}

	// Start or join the relay session
	session, err := h.relayService.StartRelay(ctx, info.Channel.ID, profileID)
	if err != nil {
		h.logger.Error("Failed to start relay session",
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
		h.logger.Error("Failed to add relay client",
			"session_id", session.ID,
			"error", err,
		)
		http.Error(w, "failed to add relay client", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Client connected to relay",
		"session_id", session.ID,
		"client_id", client.ID,
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
	)

	// Set CORS headers for browser compatibility
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")

	// Set X-Stream-* debug headers
	w.Header().Set("X-Stream-Origin-Kind", "RELAY")
	w.Header().Set("X-Stream-Decision", "transcoded")
	w.Header().Set("X-Stream-Mode", "relay-transcode")

	// Set appropriate headers for streaming
	w.Header().Set("Content-Type", "video/mp2t")
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
				h.logger.Debug("Client disconnected during write",
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
				h.logger.Debug("Reader error",
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
		h.logger.Warn("Failed to remove relay client",
			"session_id", session.ID,
			"client_id", client.ID,
			"error", removeErr,
		)
	}

	h.logger.Info("Client disconnected from relay",
		"session_id", session.ID,
		"client_id", client.ID,
	)
}

// StreamChannelInput is the input for streaming a channel.
type StreamChannelInput struct {
	ChannelID string `path:"channelId" doc:"Channel ID (ULID)"`
	ProfileID string `query:"profile" doc:"Optional relay profile ID (ULID)"`
}

// StreamChannel streams a channel through the relay system.
// Note: This handler writes directly to the response and doesn't use Huma's body handling.
func (h *RelayStreamHandler) StreamChannel(ctx context.Context, input *StreamChannelInput) (*huma.StreamResponse, error) {
	channelID, err := models.ParseULID(input.ChannelID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid channel ID format", err)
	}

	var profileID *models.ULID
	if input.ProfileID != "" {
		parsed, err := models.ParseULID(input.ProfileID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid profile ID format", err)
		}
		profileID = &parsed
	}

	// Start or join the relay session
	session, err := h.relayService.StartRelay(ctx, channelID, profileID)
	if err != nil {
		h.logger.Error("Failed to start relay session",
			"channel_id", input.ChannelID,
			"error", err,
		)
		return nil, huma.Error500InternalServerError("failed to start relay session", err)
	}

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			// Get request info for client registration
			userAgent := ctx.Header("User-Agent")
			remoteAddr := ctx.Header("X-Forwarded-For")
			if remoteAddr == "" {
				// Fall back to direct connection
				remoteAddr = "unknown"
			}

			// Add client to the session
			client, reader, err := h.relayService.AddRelayClient(session.ID, userAgent, remoteAddr)
			if err != nil {
				h.logger.Error("Failed to add relay client",
					"session_id", session.ID,
					"error", err,
				)
				ctx.SetStatus(http.StatusInternalServerError)
				return
			}

			h.logger.Info("Client connected to relay",
				"session_id", session.ID,
				"client_id", client.ID,
				"user_agent", userAgent,
				"remote_addr", remoteAddr,
			)

			// Set appropriate headers for streaming
			ctx.SetHeader("Content-Type", "video/mp2t")
			ctx.SetHeader("Cache-Control", "no-cache, no-store")
			ctx.SetHeader("Connection", "keep-alive")
			ctx.SetHeader("Transfer-Encoding", "chunked")
			ctx.SetStatus(http.StatusOK)

			// Stream data to client
			buf := make([]byte, 32*1024) // 32KB buffer
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					if _, writeErr := ctx.BodyWriter().Write(buf[:n]); writeErr != nil {
						h.logger.Debug("Client disconnected during write",
							"session_id", session.ID,
							"client_id", client.ID,
							"error", writeErr,
						)
						break
					}
				}
				if err != nil {
					if err != io.EOF {
						h.logger.Debug("Reader error",
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
				h.logger.Warn("Failed to remove relay client",
					"session_id", session.ID,
					"client_id", client.ID,
					"error", removeErr,
				)
			}

			h.logger.Info("Client disconnected from relay",
				"session_id", session.ID,
				"client_id", client.ID,
			)
		},
	}, nil
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

// StreamWithProfileInput is the input for streaming with a named profile.
type StreamWithProfileInput struct {
	ChannelID   string `path:"channelId" doc:"Channel ID (ULID)"`
	ProfileName string `path:"profileName" doc:"Relay profile name"`
}

// RegisterStreamByProfileName registers route for streaming by profile name.
func (h *RelayStreamHandler) RegisterStreamByProfileName(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "streamChannelWithProfile",
		Method:      "GET",
		Path:        "/relay/stream/{channelId}/{profileName}",
		Summary:     "Stream a channel with named profile",
		Description: "Streams a channel using a named relay profile",
		Tags:        []string{"Stream Relay"},
		SkipValidateBody: true,
	}, h.StreamWithProfile)
}

// StreamWithProfile streams a channel using a named profile.
func (h *RelayStreamHandler) StreamWithProfile(ctx context.Context, input *StreamWithProfileInput) (*huma.StreamResponse, error) {
	channelID, err := models.ParseULID(input.ChannelID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid channel ID format", err)
	}

	// Look up profile by name
	profile, err := h.relayService.GetProfileByName(ctx, input.ProfileName)
	if err != nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("relay profile %q not found", input.ProfileName))
	}

	// Start or join the relay session
	session, err := h.relayService.StartRelay(ctx, channelID, &profile.ID)
	if err != nil {
		h.logger.Error("Failed to start relay session",
			"channel_id", input.ChannelID,
			"profile", input.ProfileName,
			"error", err,
		)
		return nil, huma.Error500InternalServerError("failed to start relay session", err)
	}

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			userAgent := ctx.Header("User-Agent")
			remoteAddr := ctx.Header("X-Forwarded-For")
			if remoteAddr == "" {
				remoteAddr = "unknown"
			}

			client, reader, err := h.relayService.AddRelayClient(session.ID, userAgent, remoteAddr)
			if err != nil {
				h.logger.Error("Failed to add relay client",
					"session_id", session.ID,
					"error", err,
				)
				ctx.SetStatus(http.StatusInternalServerError)
				return
			}

			h.logger.Info("Client connected to relay",
				"session_id", session.ID,
				"client_id", client.ID,
				"profile", input.ProfileName,
			)

			ctx.SetHeader("Content-Type", "video/mp2t")
			ctx.SetHeader("Cache-Control", "no-cache, no-store")
			ctx.SetHeader("Connection", "keep-alive")
			ctx.SetHeader("Transfer-Encoding", "chunked")
			ctx.SetStatus(http.StatusOK)

			buf := make([]byte, 32*1024)
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					if _, writeErr := ctx.BodyWriter().Write(buf[:n]); writeErr != nil {
						break
					}
				}
				if err != nil {
					break
				}
			}

			h.relayService.RemoveRelayClient(session.ID, client.ID)
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

// StreamChannelByProxy streams a channel using the proxy's configured mode.
// This is the main streaming endpoint that dispatches based on proxy_mode:
// - redirect: Returns HTTP 302 redirect to the source URL
// - proxy: Fetches upstream and repackages with CORS headers (future)
// - relay: FFmpeg transcoding with cyclic buffer (existing relay mode)
func (h *RelayStreamHandler) StreamChannelByProxy(ctx context.Context, input *StreamChannelByProxyInput) (*huma.StreamResponse, error) {
	proxyID, err := models.ParseULID(input.ProxyID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid proxy ID format", err)
	}

	channelID, err := models.ParseULID(input.ChannelID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid channel ID format", err)
	}

	// Get stream info (proxy, channel, optional profile)
	streamInfo, err := h.relayService.GetStreamInfo(ctx, proxyID, channelID)
	if err != nil {
		h.logger.Error("Failed to get stream info",
			"proxy_id", input.ProxyID,
			"channel_id", input.ChannelID,
			"error", err,
		)
		return nil, huma.Error404NotFound(fmt.Sprintf("stream not found: %v", err))
	}

	// Dispatch based on proxy mode
	switch streamInfo.Proxy.ProxyMode {
	case models.StreamProxyModeRedirect:
		return h.handleRedirectMode(ctx, streamInfo)

	case models.StreamProxyModeProxy:
		return h.handleProxyMode(ctx, streamInfo)

	case models.StreamProxyModeRelay:
		return h.handleRelayMode(ctx, streamInfo)

	default:
		h.logger.Error("Unknown proxy mode",
			"proxy_id", input.ProxyID,
			"mode", streamInfo.Proxy.ProxyMode,
		)
		return nil, huma.Error500InternalServerError(fmt.Sprintf("unknown proxy mode: %s", streamInfo.Proxy.ProxyMode), nil)
	}
}

// handleRedirectMode returns an HTTP 302 redirect to the source stream URL.
// This implements T016-T018: Redirect mode with zero overhead (no session creation).
// Uses direct http.ResponseWriter access to set 302 status before Huma commits 200.
func (h *RelayStreamHandler) handleRedirectMode(ctx context.Context, info *service.StreamInfo) (*huma.StreamResponse, error) {
	streamURL := info.Channel.StreamURL

	h.logger.Info("Redirect mode: sending 302 redirect",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
	)

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			// Access the underlying http.ResponseWriter directly.
			// Huma's StreamResponse normally commits 200 before calling Body,
			// but we can intercept via type assertion and set 302 first.
			w := ctx.BodyWriter()
			if rw, ok := w.(http.ResponseWriter); ok {
				rw.Header().Set("Location", streamURL)
				rw.Header().Set("X-Stream-Origin-Kind", "REDIRECT")
				rw.Header().Set("X-Stream-Decision", "redirect")
				rw.Header().Set("X-Stream-Mode", "redirect")
				rw.WriteHeader(http.StatusFound)
				return
			}

			// Fallback to Huma methods if type assertion fails
			h.logger.Warn("Could not access raw ResponseWriter for redirect, using fallback")
			ctx.SetHeader("Location", streamURL)
			ctx.SetHeader("X-Stream-Origin-Kind", "REDIRECT")
			ctx.SetHeader("X-Stream-Decision", "redirect")
			ctx.SetHeader("X-Stream-Mode", "redirect")
			ctx.SetStatus(http.StatusFound)
			_, _ = ctx.BodyWriter().Write([]byte{})
		},
	}, nil
}

// handleProxyMode fetches upstream content and streams it with CORS headers.
// This implements T019-T028: Proxy mode that fetches and repackages streams.
// It classifies the stream and optionally collapses HLS to continuous TS.
func (h *RelayStreamHandler) handleProxyMode(ctx context.Context, info *service.StreamInfo) (*huma.StreamResponse, error) {
	streamURL := info.Channel.StreamURL

	// Classify the stream to determine optimal handling
	classification := h.relayService.ClassifyStream(ctx, streamURL)

	h.logger.Info("Proxy mode: stream classified",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
		"mode", classification.Mode.String(),
		"eligible_for_collapse", classification.EligibleForCollapse,
	)

	// Check if HLS collapse is enabled and stream is eligible
	hlsCollapseEnabled := info.Proxy.HLSCollapse
	shouldCollapse := hlsCollapseEnabled && classification.EligibleForCollapse

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			// Access the underlying http.ResponseWriter to set CORS headers
			// before Huma commits the response. This ensures browsers receive
			// proper CORS headers for cross-origin streaming.
			w := ctx.BodyWriter()
			if rw, ok := w.(http.ResponseWriter); ok {
				rw.Header().Set("Access-Control-Allow-Origin", "*")
				rw.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				rw.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
				rw.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
				rw.Header().Set("X-Stream-Origin-Kind", "PROXY")
				rw.Header().Set("X-Stream-Mode", "proxy")
			} else {
				// Fallback to Huma methods
				ctx.SetHeader("Access-Control-Allow-Origin", "*")
				ctx.SetHeader("Access-Control-Allow-Methods", "GET, OPTIONS")
				ctx.SetHeader("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
				ctx.SetHeader("Access-Control-Expose-Headers", "Content-Length, Content-Range")
				ctx.SetHeader("X-Stream-Origin-Kind", "PROXY")
				ctx.SetHeader("X-Stream-Mode", "proxy")
			}

			// Handle based on classification
			if shouldCollapse && classification.SelectedMediaPlaylist != "" {
				// HLS collapse mode - convert multi-variant HLS to continuous TS
				h.streamHLSCollapsed(ctx, info, &classification)
			} else {
				// Direct proxy mode - simple passthrough
				h.streamDirectProxy(ctx, info, &classification)
			}
		},
	}, nil
}

// streamHLSCollapsed streams collapsed HLS as continuous MPEG-TS.
func (h *RelayStreamHandler) streamHLSCollapsed(ctx huma.Context, info *service.StreamInfo, classification *service.ClassificationResult) {
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

	ctx.SetHeader("X-Stream-Decision", "collapsed-hls")
	ctx.SetHeader("Content-Type", "video/mp2t")
	ctx.SetHeader("Cache-Control", "no-cache, no-store")
	ctx.SetHeader("Connection", "keep-alive")
	ctx.SetHeader("Transfer-Encoding", "chunked")
	ctx.SetStatus(http.StatusOK)

	// Create HLS collapser
	collapser := h.relayService.CreateHLSCollapser(playlistURL)

	// Start the collapser
	if err := collapser.Start(context.Background()); err != nil {
		h.logger.Error("Failed to start HLS collapser",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		ctx.SetStatus(http.StatusInternalServerError)
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
			if _, writeErr := ctx.BodyWriter().Write(buf[:n]); writeErr != nil {
				h.logger.Debug("Client disconnected during HLS collapse write",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", writeErr,
				)
				break
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

// streamDirectProxy streams content directly from upstream with CORS headers.
func (h *RelayStreamHandler) streamDirectProxy(ctx huma.Context, info *service.StreamInfo, classification *service.ClassificationResult) {
	streamURL := info.Channel.StreamURL

	h.logger.Info("Proxy mode: direct proxy",
		"proxy_id", info.Proxy.ID,
		"channel_id", info.Channel.ID,
		"stream_url", streamURL,
		"stream_mode", classification.Mode.String(),
	)

	ctx.SetHeader("X-Stream-Decision", "direct-proxy")

	// Create HTTP request to upstream
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		h.logger.Error("Failed to create upstream request",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		ctx.SetStatus(http.StatusBadGateway)
		_, _ = ctx.BodyWriter().Write([]byte{}) // Commit headers
		return
	}

	// Forward relevant headers from client
	if ua := ctx.Header("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if accept := ctx.Header("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if rangeHeader := ctx.Header("Range"); rangeHeader != "" {
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
		ctx.SetStatus(http.StatusBadGateway)
		_, _ = ctx.BodyWriter().Write([]byte{}) // Commit headers
		return
	}
	defer resp.Body.Close()

	// Set content type from upstream or default to video/mp2t
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp2t"
	}
	ctx.SetHeader("Content-Type", contentType)

	// Forward content length if available
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		ctx.SetHeader("Content-Length", contentLength)
	}

	// Forward content range for partial content
	if resp.StatusCode == http.StatusPartialContent {
		if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
			ctx.SetHeader("Content-Range", contentRange)
		}
		ctx.SetStatus(http.StatusPartialContent)
	} else {
		ctx.SetHeader("Cache-Control", "no-cache, no-store")
		ctx.SetHeader("Connection", "keep-alive")
		ctx.SetStatus(http.StatusOK)
	}

	// Stream data to client
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := ctx.BodyWriter().Write(buf[:n]); writeErr != nil {
				h.logger.Debug("Client disconnected during proxy write",
					"proxy_id", info.Proxy.ID,
					"channel_id", info.Channel.ID,
					"error", writeErr,
				)
				break
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

// handleRelayMode starts or joins a relay session for FFmpeg transcoding.
func (h *RelayStreamHandler) handleRelayMode(ctx context.Context, info *service.StreamInfo) (*huma.StreamResponse, error) {
	// Determine profile ID to use
	var profileID *models.ULID
	if info.Profile != nil {
		profileID = &info.Profile.ID
	}

	// Start or join the relay session
	session, err := h.relayService.StartRelay(ctx, info.Channel.ID, profileID)
	if err != nil {
		h.logger.Error("Failed to start relay session",
			"proxy_id", info.Proxy.ID,
			"channel_id", info.Channel.ID,
			"error", err,
		)
		return nil, huma.Error500InternalServerError("failed to start relay session", err)
	}

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			// Get request info for client registration
			userAgent := ctx.Header("User-Agent")
			remoteAddr := ctx.Header("X-Forwarded-For")
			if remoteAddr == "" {
				remoteAddr = "unknown"
			}

			// Add client to the session
			client, reader, err := h.relayService.AddRelayClient(session.ID, userAgent, remoteAddr)
			if err != nil {
				h.logger.Error("Failed to add relay client",
					"session_id", session.ID,
					"error", err,
				)
				ctx.SetStatus(http.StatusInternalServerError)
				return
			}

			h.logger.Info("Client connected to relay",
				"session_id", session.ID,
				"client_id", client.ID,
				"proxy_id", info.Proxy.ID,
				"channel_id", info.Channel.ID,
			)

			// Set CORS headers for browser compatibility
			ctx.SetHeader("Access-Control-Allow-Origin", "*")
			ctx.SetHeader("Access-Control-Allow-Methods", "GET, OPTIONS")
			ctx.SetHeader("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
			ctx.SetHeader("Access-Control-Expose-Headers", "Content-Length, Content-Range")

			// Set X-Stream-* debug headers
			ctx.SetHeader("X-Stream-Origin-Kind", "RELAY")
			ctx.SetHeader("X-Stream-Decision", "transcoded")
			ctx.SetHeader("X-Stream-Mode", "relay-transcode")

			// Set appropriate headers for streaming
			ctx.SetHeader("Content-Type", "video/mp2t")
			ctx.SetHeader("Cache-Control", "no-cache, no-store")
			ctx.SetHeader("Connection", "keep-alive")
			ctx.SetHeader("Transfer-Encoding", "chunked")
			ctx.SetStatus(http.StatusOK)

			// Stream data to client
			buf := make([]byte, 32*1024) // 32KB buffer
			for {
				n, err := reader.Read(buf)
				if n > 0 {
					if _, writeErr := ctx.BodyWriter().Write(buf[:n]); writeErr != nil {
						h.logger.Debug("Client disconnected during write",
							"session_id", session.ID,
							"client_id", client.ID,
							"error", writeErr,
						)
						break
					}
				}
				if err != nil {
					if err != io.EOF {
						h.logger.Debug("Reader error",
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
				h.logger.Warn("Failed to remove relay client",
					"session_id", session.ID,
					"client_id", client.ID,
					"error", removeErr,
				)
			}

			h.logger.Info("Client disconnected from relay",
				"session_id", session.ID,
				"client_id", client.ID,
			)
		},
	}, nil
}
