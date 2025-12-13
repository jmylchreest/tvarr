// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
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
		source.DetectedTimezone = formatTimezoneOffset(h.detectedTimezone)
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

// applyTimeOffset applies the source's EpgShift to a time value.
// The EpgShift is in hours and shifts times forward (positive) or back (negative).
// Times are first converted to UTC, then the shift is applied.
func (h *XMLTVHandler) applyTimeOffset(t time.Time, source *models.EpgSource) time.Time {
	if source == nil {
		return t
	}

	// Convert to UTC first (the time already has timezone info from parsing)
	t = t.UTC()

	// Apply EpgShift if configured (shift in hours)
	if source.EpgShift != 0 {
		t = t.Add(time.Duration(source.EpgShift) * time.Hour)
	}

	return t
}

// formatTimezoneOffset converts a timezone offset like "+0000" or "-0500" to a
// more readable format like "+00:00" or "-05:00".
func formatTimezoneOffset(offset string) string {
	if offset == "" {
		return ""
	}

	// Already in colon format
	if len(offset) == 6 && offset[3] == ':' {
		return offset
	}

	// Convert "+0000" to "+00:00"
	if len(offset) == 5 {
		return offset[:3] + ":" + offset[3:]
	}

	return offset
}

// Ensure XMLTVHandler implements EpgHandler.
var _ EpgHandler = (*XMLTVHandler)(nil)
