package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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

// Register registers the relay stream routes with the API.
func (h *RelayStreamHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "streamChannel",
		Method:        "GET",
		Path:          "/relay/stream/{channelId}",
		Summary:       "Stream a channel",
		Description:   "Starts streaming a channel through the relay system",
		Tags:          []string{"Stream Relay"},
		SkipValidateBody: true,
	}, h.StreamChannel)

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

// StreamChannelInput is the input for streaming a channel.
type StreamChannelInput struct {
	ChannelID string  `path:"channelId" doc:"Channel ID (ULID)"`
	ProfileID *string `query:"profile" doc:"Optional relay profile ID (ULID)"`
}

// StreamChannel streams a channel through the relay system.
// Note: This handler writes directly to the response and doesn't use Huma's body handling.
func (h *RelayStreamHandler) StreamChannel(ctx context.Context, input *StreamChannelInput) (*huma.StreamResponse, error) {
	channelID, err := models.ParseULID(input.ChannelID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid channel ID format", err)
	}

	var profileID *models.ULID
	if input.ProfileID != nil && *input.ProfileID != "" {
		parsed, err := models.ParseULID(*input.ProfileID)
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
