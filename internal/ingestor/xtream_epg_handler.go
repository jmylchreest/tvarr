// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
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

	// logger is the structured logger for timezone detection and normalization events.
	logger *slog.Logger

	// serverLocation is the detected server timezone for parsing string times.
	// Set during Ingest() from the server info.
	serverLocation *time.Location
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

// WithLogger sets a structured logger for the handler.
func (h *XtreamEpgHandler) WithLogger(logger *slog.Logger) *XtreamEpgHandler {
	h.logger = logger
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
// The API method used depends on the source's ApiMethod field:
// - XtreamApiMethodStreamID (default): Uses per-stream JSON API for richer data (~6 days forward)
// - XtreamApiMethodBulkXMLTV: Uses bulk /xmltv.php endpoint for better performance (~2 days forward)
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

	// Reset server location for this ingestion
	h.serverLocation = nil

	// Try to detect timezone from server info
	if authInfo, err := client.GetAuthInfo(ctx); err == nil {
		if authInfo.ServerInfo.Timezone != "" {
			detectedTz := authInfo.ServerInfo.Timezone
			source.DetectedTimezone = FormatTimezoneOffset(detectedTz)

			// Try to load the timezone as an IANA location (e.g., "Europe/Amsterdam")
			// This is used for parsing string times that don't have timezone info
			if loc, err := time.LoadLocation(detectedTz); err == nil {
				h.serverLocation = loc
				if h.logger != nil {
					h.logger.Debug("loaded server timezone location",
						slog.String("timezone", detectedTz),
						slog.String("source_name", source.Name),
					)
				}
			} else if h.logger != nil {
				h.logger.Debug("could not load server timezone as location, will use UTC for string parsing",
					slog.String("timezone", detectedTz),
					slog.String("error", err.Error()),
				)
			}

			// Auto-set EpgShift based on detected timezone (T: auto-shift feature)
			// Only update if the detected timezone differs from what we last auto-configured for.
			// This allows user overrides to persist until the source timezone actually changes.
			if source.AutoShiftTimezone != detectedTz {
				newShift := CalculateAutoShift(detectedTz)
				if h.logger != nil {
					h.logger.Info("auto-adjusting EPG timeshift based on detected timezone",
						slog.String("source_name", source.Name),
						slog.String("detected_timezone", detectedTz),
						slog.String("previous_auto_shift_timezone", source.AutoShiftTimezone),
						slog.Int("previous_epg_shift", source.EpgShift),
						slog.Int("new_epg_shift", newShift),
					)
				}
				source.EpgShift = newShift
				source.AutoShiftTimezone = detectedTz
			}

			// Log timezone detection
			LogTimezoneDetection(h.logger, source.Name, source.ID.String(), source.DetectedTimezone, source.ProgramCount)
		}
	}

	// Log normalization settings if timeshift is configured
	if source.EpgShift != 0 {
		LogTimezoneNormalization(h.logger, source.Name, source.DetectedTimezone, source.EpgShift)
	}

	// Select API method based on source configuration
	apiMethod := source.ApiMethod
	if apiMethod == "" {
		apiMethod = models.XtreamApiMethodStreamID // Default to richer data
	}

	switch apiMethod {
	case models.XtreamApiMethodBulkXMLTV:
		return h.ingestBulkXMLTV(ctx, client, source, callback)
	case models.XtreamApiMethodStreamID:
		fallthrough
	default:
		return h.ingestPerStream(ctx, client, source, callback)
	}
}

// ingestPerStream fetches EPG data using the per-stream JSON API.
// This provides richer data (more fields, ~6 days forward) but requires N requests.
func (h *XtreamEpgHandler) ingestPerStream(ctx context.Context, client *xtream.Client, source *models.EpgSource, callback ProgramCallback) error {
	// Fetch all live streams to get the channel list with EPG IDs
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

			program := h.convertListing(listing, source, stream.EPGChannelID)

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

// ingestBulkXMLTV fetches EPG data using the bulk /xmltv.php endpoint.
// This is more performant (1 request) but may have fewer forward days (~2 days).
func (h *XtreamEpgHandler) ingestBulkXMLTV(ctx context.Context, client *xtream.Client, source *models.EpgSource, callback ProgramCallback) error {
	// Fetch the XMLTV data as a streaming reader
	reader, err := client.GetXMLTVReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch XMLTV: %w", err)
	}
	defer reader.Close()

	// Create XMLTV parser with callbacks
	parser := &xmltv.Parser{
		OnChannel: func(channel *xmltv.Channel) error {
			// Channel definitions are handled by the stream source, not EPG source
			return nil
		},
		OnProgramme: func(programme *xmltv.Programme) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Convert XMLTV programme to EpgProgram model
			program := h.convertXMLTVProgramme(programme, source)

			// Skip programs that fail validation (e.g., invalid time ranges)
			if err := program.Validate(); err != nil {
				return nil
			}

			return callback(program)
		},
		OnError: func(err error) {
			// Log parsing errors but continue processing
			// In production, this could be wired to a logger
		},
	}

	// Parse with auto-decompression support
	if err := parser.ParseCompressed(reader); err != nil {
		return fmt.Errorf("failed to parse XMLTV: %w", err)
	}

	return nil
}

// convertListing converts an Xtream EPG listing to an EpgProgram model.
func (h *XtreamEpgHandler) convertListing(listing xtream.EPGListing, source *models.EpgSource, channelID string) *models.EpgProgram {
	// Parse times using server timezone for string-based times (timestamps are always UTC)
	start := h.applyTimeOffset(listing.StartTimeInLocation(h.serverLocation), source)
	stop := h.applyTimeOffset(listing.EndTimeInLocation(h.serverLocation), source)

	program := &models.EpgProgram{
		SourceID:    source.ID,
		ChannelID:   channelID,
		Title:       decodeBase64OrOriginal(listing.Title),
		Description: decodeBase64OrOriginal(listing.Description),
		Language:    listing.Lang,
		Start:       start,
		Stop:        stop,
	}

	return program
}

// decodeBase64OrOriginal attempts to decode a base64 string. If decoding fails
// (the string is not valid base64), it returns the original string unchanged.
// This handles Xtream APIs that return base64-encoded title and description fields.
func decodeBase64OrOriginal(s string) string {
	if s == "" {
		return s
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Not valid base64, return original string
		return s
	}
	return string(decoded)
}

// convertXMLTVProgramme converts an XMLTV Programme to an EpgProgram model.
// This is used by the bulk XMLTV ingestion method.
func (h *XtreamEpgHandler) convertXMLTVProgramme(p *xmltv.Programme, source *models.EpgSource) *models.EpgProgram {
	// Apply time offset if configured
	start := h.applyTimeOffset(p.Start, source)
	stop := h.applyTimeOffset(p.Stop, source)

	program := &models.EpgProgram{
		SourceID:    source.ID,
		ChannelID:   p.Channel,
		Start:       start,
		Stop:        stop,
		Title:       p.Title,
		SubTitle:    p.SubTitle,
		Description: p.Description,
		Category:    p.Category,
		Icon:        p.Icon,
		EpisodeNum:  p.EpisodeNum,
		Rating:      p.Rating,
		Language:    p.Language,
		IsNew:       p.IsNew,
		IsPremiere:  p.IsPremiere,
	}

	// Convert credits to JSON string if present
	if p.Credits != nil {
		credits := make(map[string][]string)
		if len(p.Credits.Directors) > 0 {
			credits["directors"] = p.Credits.Directors
		}
		if len(p.Credits.Actors) > 0 {
			credits["actors"] = p.Credits.Actors
		}
		if len(p.Credits.Writers) > 0 {
			credits["writers"] = p.Credits.Writers
		}
		if len(p.Credits.Producers) > 0 {
			credits["producers"] = p.Credits.Producers
		}
		if len(p.Credits.Presenters) > 0 {
			credits["presenters"] = p.Credits.Presenters
		}
		if len(credits) > 0 {
			if data, err := json.Marshal(credits); err == nil {
				program.Credits = string(data)
			}
		}
	}

	return program
}

// applyTimeOffset applies timezone normalization and the source's EpgShift to a time value.
// The EpgShift is in hours and shifts times forward (positive) or back (negative).
// Times are first converted to UTC (using embedded timezone info from parsing), then the shift is applied.
func (h *XtreamEpgHandler) applyTimeOffset(t time.Time, source *models.EpgSource) time.Time {
	if source == nil {
		return t
	}

	// Parse the detected timezone offset (for logging purposes - the time already has timezone embedded)
	var detectedOffset time.Duration
	if source.DetectedTimezone != "" {
		var err error
		detectedOffset, err = ParseTimezoneOffset(source.DetectedTimezone)
		if err != nil && h.logger != nil {
			h.logger.Debug("failed to parse detected timezone offset",
				slog.String("offset", source.DetectedTimezone),
				slog.String("error", err.Error()),
			)
		}
	}

	// Normalize to UTC and apply timeshift using the utility function
	// Note: Go's time.Time already has timezone embedded from parsing, so .UTC() handles normalization
	return NormalizeProgramTime(t, detectedOffset, source.EpgShift)
}

// Ensure XtreamEpgHandler implements EpgHandler.
var _ EpgHandler = (*XtreamEpgHandler)(nil)
