// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	// fetcher handles both HTTP/HTTPS and file:// URLs.
	fetcher *urlutil.ResourceFetcher

	// channelMap stores channel ID to external ID mapping during ingestion.
	// This maps XMLTV channel IDs to be used when processing programmes.
	channelMap map[string]string

	// detectedTimezone stores the first detected timezone offset during ingestion.
	detectedTimezone string
	// timezoneDetected tracks whether we've already detected the timezone.
	timezoneDetected bool

	// logger is the structured logger for timezone detection and normalization events.
	logger *slog.Logger
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

// Ingest processes an XMLTV EPG source and yields programs via the callback.
func (h *XMLTVHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Fetch the XMLTV file (supports http://, https://, and file://)
	reader, err := h.fetcher.Fetch(ctx, source.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch XMLTV: %w", err)
	}
	defer reader.Close()

	// Reset state for this ingestion
	h.channelMap = make(map[string]string)
	h.detectedTimezone = ""
	h.timezoneDetected = false

	// Create XMLTV parser with callbacks
	parser := &xmltv.Parser{
		OnChannel: func(channel *xmltv.Channel) error {
			// Store channel ID mapping for programme processing
			h.channelMap[channel.ID] = channel.ID
			return nil
		},
		OnProgramme: func(programme *xmltv.Programme) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Capture timezone from the first programme that has one
			if !h.timezoneDetected && programme.TimezoneOffset != "" {
				h.detectedTimezone = programme.TimezoneOffset
				h.timezoneDetected = true
			}

			// Convert XMLTV programme to EpgProgram model
			program := h.convertProgramme(programme, source)

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

	// Update source's detected timezone (will be saved by the caller)
	if h.timezoneDetected {
		detectedTz := h.detectedTimezone
		source.DetectedTimezone = formatTimezoneOffset(detectedTz)

		// Auto-set EpgShift based on detected timezone (auto-shift feature)
		// Only update if the detected timezone differs from what we last auto-configured for.
		// This allows user overrides to persist until the source timezone actually changes.
		// Note: For XMLTV, times typically have timezone embedded, so auto-shift may be 0.
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

		// Log timezone detection using structured logging
		LogTimezoneDetection(h.logger, source.Name, source.ID.String(), source.DetectedTimezone, source.ProgramCount)
	}

	// Log normalization settings if timeshift is configured
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
