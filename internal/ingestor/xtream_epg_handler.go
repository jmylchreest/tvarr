// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/xtream"
)

// EPG handler configuration defaults.
const (
	defaultEpgTimeout  = 5 * time.Minute
	defaultDaysToFetch = 7
	httpSchemePrefix   = "http://"
	httpsSchemePrefix  = "https://"
)

// XtreamEpgHandler handles EPG ingestion from Xtream Codes API sources.
type XtreamEpgHandler struct {
	// httpClient is the resilient HTTP client used for API requests.
	httpClient *httpclient.Client

	// DaysToFetch is the number of days of EPG data to fetch (default: 7).
	DaysToFetch int
}

// NewXtreamEpgHandler creates a new Xtream EPG handler with default settings.
func NewXtreamEpgHandler() *XtreamEpgHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultEpgTimeout

	return &XtreamEpgHandler{
		httpClient:  httpclient.New(cfg),
		DaysToFetch: defaultDaysToFetch,
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *XtreamEpgHandler) WithHTTPClientConfig(cfg httpclient.Config) *XtreamEpgHandler {
	h.httpClient = httpclient.New(cfg)
	return h
}

// WithDaysToFetch sets the number of days of EPG data to fetch.
func (h *XtreamEpgHandler) WithDaysToFetch(days int) *XtreamEpgHandler {
	h.DaysToFetch = days
	return h
}

// Type returns the EPG source type this handler supports.
func (h *XtreamEpgHandler) Type() models.EpgSourceType {
	return models.EpgSourceTypeXtream
}

// Validate checks if the EPG source configuration is valid for Xtream.
func (h *XtreamEpgHandler) Validate(source *models.EpgSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.EpgSourceTypeXtream {
		return fmt.Errorf("invalid source type: expected %s, got %s", models.EpgSourceTypeXtream, source.Type)
	}
	if source.URL == "" {
		return fmt.Errorf("URL is required for Xtream EPG sources")
	}
	if !strings.HasPrefix(source.URL, httpSchemePrefix) && !strings.HasPrefix(source.URL, httpsSchemePrefix) {
		return fmt.Errorf("URL must be an HTTP(S) URL")
	}
	if source.Username == "" {
		return fmt.Errorf("username is required for Xtream EPG sources")
	}
	if source.Password == "" {
		return fmt.Errorf("password is required for Xtream EPG sources")
	}
	return nil
}

// Ingest fetches EPG data from the Xtream API and yields programs via the callback.
func (h *XtreamEpgHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
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

	// First, fetch all live streams to get the channel list with EPG IDs
	streams, err := client.GetLiveStreams(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch live streams: %w", err)
	}

	// For each stream with an EPG channel ID, fetch the EPG data
	for _, stream := range streams {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip streams without EPG channel ID
		if stream.EPGChannelID == "" {
			continue
		}

		// Fetch EPG for this stream
		epgListings, err := client.GetFullEPG(ctx, int(stream.StreamID.Int()))
		if err != nil {
			// Log error but continue with other streams
			continue
		}

		// Convert and emit each program
		for _, listing := range epgListings {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			program := h.convertListing(listing, source.ID, stream.EPGChannelID)

			// Skip programs that fail validation (e.g., invalid time ranges)
			if err := program.Validate(); err != nil {
				continue
			}

			if err := callback(program); err != nil {
				return fmt.Errorf("callback error: %w", err)
			}
		}
	}

	return nil
}

// convertListing converts an Xtream EPG listing to an EpgProgram model.
func (h *XtreamEpgHandler) convertListing(listing xtream.EPGListing, sourceID models.ULID, channelID string) *models.EpgProgram {
	program := &models.EpgProgram{
		SourceID:    sourceID,
		ChannelID:   channelID,
		Title:       listing.Title,
		Description: listing.Description,
		Language:    listing.Lang,
		Start:       listing.StartTime(),
		Stop:        listing.EndTime(),
	}

	return program
}

// Ensure XtreamEpgHandler implements EpgHandler.
var _ EpgHandler = (*XtreamEpgHandler)(nil)
