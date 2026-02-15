// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/urlutil"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
)

// XMLTV handler configuration defaults.
const (
	defaultXMLTVTimeout = 5 * time.Minute
)

// XMLTVHandler handles XMLTV EPG source ingestion.
type XMLTVHandler struct {
	fetcher          *urlutil.ResourceFetcher
	channelMap       map[string]string
	detectedTimezone string
	timezoneDetected bool
	logger           *slog.Logger
}

// NewXMLTVHandler creates a new XMLTV handler with default settings.
func NewXMLTVHandler() *XMLTVHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultXMLTVTimeout

	return &XMLTVHandler{
		fetcher:    urlutil.NewResourceFetcher(cfg),
		channelMap: make(map[string]string),
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *XMLTVHandler) WithHTTPClientConfig(cfg httpclient.Config) *XMLTVHandler {
	h.fetcher = urlutil.NewResourceFetcher(cfg)
	return h
}

// WithLogger sets a structured logger for the handler.
func (h *XMLTVHandler) WithLogger(logger *slog.Logger) *XMLTVHandler {
	h.logger = logger
	return h
}

// Type returns the EPG source type this handler supports.
func (h *XMLTVHandler) Type() models.EpgSourceType {
	return models.EpgSourceTypeXMLTV
}

// Validate checks if the EPG source configuration is valid for XMLTV.
func (h *XMLTVHandler) Validate(source *models.EpgSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.EpgSourceTypeXMLTV {
		return fmt.Errorf("invalid source type: expected %s, got %s", models.EpgSourceTypeXMLTV, source.Type)
	}
	if source.URL == "" {
		return fmt.Errorf("URL is required for XMLTV sources")
	}
	if !urlutil.IsSupportedURL(source.URL) {
		return fmt.Errorf("URL must be HTTP, HTTPS, or file:// URL")
	}
	return nil
}

// xmltvBreakerNameFromURL extracts a circuit breaker name from the source URL.
func xmltvBreakerNameFromURL(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Host == "" {
		return "xmltv-ingestion"
	}
	return "ingestion-" + parsed.Host
}

// Ingest processes an XMLTV EPG source and yields programs via the callback.
func (h *XMLTVHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	breakerName := xmltvBreakerNameFromURL(source.URL)
	breaker := httpclient.DefaultManager.GetOrCreate(breakerName)

	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultXMLTVTimeout
	fetcher := urlutil.NewResourceFetcherWithBreaker(cfg, breaker)

	reader, err := fetcher.Fetch(ctx, source.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch XMLTV: %w", err)
	}
	defer reader.Close()

	h.channelMap = make(map[string]string)
	h.detectedTimezone = ""
	h.timezoneDetected = false

	parser := &xmltv.Parser{
		OnChannel: func(channel *xmltv.Channel) error {
			h.channelMap[channel.ID] = channel.ID
			return nil
		},
		OnProgramme: func(programme *xmltv.Programme) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if !h.timezoneDetected && programme.TimezoneOffset != "" {
				h.detectedTimezone = programme.TimezoneOffset
				h.timezoneDetected = true
			}

			program := h.convertProgramme(programme, source)

			if err := program.Validate(); err != nil {
				return nil
			}

			return callback(program)
		},
		OnError: func(err error) {
		},
	}

	if err := parser.ParseCompressed(reader); err != nil {
		return fmt.Errorf("failed to parse XMLTV: %w", err)
	}

	if h.timezoneDetected {
		detectedTz := h.detectedTimezone
		source.DetectedTimezone = formatTimezoneOffset(detectedTz)

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

		LogTimezoneDetection(h.logger, source.Name, source.ID.String(), source.DetectedTimezone, source.ProgramCount)
	}

	if source.EpgShift != 0 {
		LogTimezoneNormalization(h.logger, source.Name, source.DetectedTimezone, source.EpgShift)
	}

	return nil
}

// convertProgramme converts an XMLTV Programme to an EpgProgram model.
func (h *XMLTVHandler) convertProgramme(p *xmltv.Programme, source *models.EpgSource) *models.EpgProgram {
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
func (h *XMLTVHandler) applyTimeOffset(t time.Time, source *models.EpgSource) time.Time {
	if source == nil {
		return t
	}

	// Parse the detected timezone offset (for logging purposes - the time already has timezone embedded)
	var detectedOffset time.Duration
	if h.detectedTimezone != "" {
		var err error
		detectedOffset, err = ParseTimezoneOffset(h.detectedTimezone)
		if err != nil && h.logger != nil {
			h.logger.Debug("failed to parse detected timezone offset",
				slog.String("offset", h.detectedTimezone),
				slog.String("error", err.Error()),
			)
		}
	}

	// Normalize to UTC and apply timeshift using the utility function
	// Note: Go's time.Time already has timezone embedded from parsing, so .UTC() handles normalization
	return NormalizeProgramTime(t, detectedOffset, source.EpgShift)
}

// formatTimezoneOffset converts a timezone offset like "+0000" or "-0500" to a
// more readable format like "+00:00" or "-05:00".
// This is a convenience wrapper around FormatTimezoneOffset.
func formatTimezoneOffset(offset string) string {
	if offset == "" {
		return ""
	}
	return FormatTimezoneOffset(offset)
}

// Ensure XMLTVHandler implements EpgHandler.
var _ EpgHandler = (*XMLTVHandler)(nil)
