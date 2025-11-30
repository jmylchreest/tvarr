package ingestor

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/xtream"
)

// Handler configuration defaults.
const (
	defaultXtreamTimeout = 2 * time.Minute
)

// Stream type constants.
const (
	streamTypeLive = "live"
	extTS          = "ts"
)

// XtreamHandler handles ingestion of Xtream Codes API sources.
type XtreamHandler struct {
	// httpClient is the resilient HTTP client used for API requests.
	httpClient *httpclient.Client
}

// NewXtreamHandler creates a new Xtream handler with default settings.
func NewXtreamHandler() *XtreamHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultXtreamTimeout

	return &XtreamHandler{
		httpClient: httpclient.New(cfg),
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *XtreamHandler) WithHTTPClientConfig(cfg httpclient.Config) *XtreamHandler {
	h.httpClient = httpclient.New(cfg)
	return h
}

// Type returns the source type this handler supports.
func (h *XtreamHandler) Type() models.SourceType {
	return models.SourceTypeXtream
}

// Validate checks if the source configuration is valid for Xtream ingestion.
func (h *XtreamHandler) Validate(source *models.StreamSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.SourceTypeXtream {
		return fmt.Errorf("source type must be xtream, got %s", source.Type)
	}
	if source.URL == "" {
		return fmt.Errorf("source URL is required")
	}
	if source.Username == "" {
		return fmt.Errorf("username is required for Xtream sources")
	}
	if source.Password == "" {
		return fmt.Errorf("password is required for Xtream sources")
	}
	return nil
}

// Ingest fetches channels from the Xtream API, calling the callback for each channel.
func (h *XtreamHandler) Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Create Xtream client with the resilient HTTP client (as standard *http.Client)
	client := xtream.NewClient(
		source.URL,
		source.Username,
		source.Password,
		xtream.WithHTTPClient(h.httpClient.StandardClient()),
	)

	// Fetch categories first to build category name lookup
	categories, err := client.GetLiveCategories(ctx)
	if err != nil {
		return fmt.Errorf("fetching categories: %w", err)
	}

	categoryMap := make(map[string]string)
	for _, cat := range categories {
		categoryMap[cat.CategoryID.String()] = cat.CategoryName
	}

	// Fetch live streams
	streams, err := client.GetLiveStreams(ctx, nil)
	if err != nil {
		return fmt.Errorf("fetching live streams: %w", err)
	}

	// Process each stream
	for _, stream := range streams {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		channel := h.streamToChannel(stream, source.ID, client, categoryMap)
		if err := callback(channel); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	return nil
}

// streamToChannel converts an Xtream stream to a Channel model.
func (h *XtreamHandler) streamToChannel(stream xtream.Stream, sourceID models.ULID, client *xtream.Client, categoryMap map[string]string) *models.Channel {
	channel := &models.Channel{
		SourceID:      sourceID,
		ExtID:         strconv.FormatInt(stream.StreamID.Int(), 10),
		TvgID:         stream.EPGChannelID,
		TvgName:       stream.Name,
		TvgLogo:       stream.StreamIcon,
		GroupTitle:    categoryMap[stream.CategoryID.String()],
		ChannelName:   stream.Name,
		ChannelNumber: int(stream.Num.Int()),
		StreamURL:     client.GetLiveStreamURL(int(stream.StreamID.Int()), extTS),
		StreamType:    streamTypeLive,
		IsAdult:       stream.IsAdult.Int() == 1,
	}

	return channel
}
