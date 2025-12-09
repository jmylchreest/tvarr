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

	// Reset channel map for this ingestion
	h.channelMap = make(map[string]string)

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

// applyTimeOffset applies the source's time offset to a time value.
// The offset can be in formats like "+01:00", "-05:00", "+0100", "-0500".
// If OriginalTimezone is set (e.g., "Europe/London"), it will be used to
// convert times that don't have timezone info to the correct timezone first.
func (h *XMLTVHandler) applyTimeOffset(t time.Time, source *models.EpgSource) time.Time {
	if source == nil {
		return t
	}

	// If time has no timezone info (UTC from parser default) and OriginalTimezone is set,
	// treat the time as being in that timezone
	if source.OriginalTimezone != "" && t.Location() == time.UTC {
		loc, err := time.LoadLocation(source.OriginalTimezone)
		if err == nil {
			// The time value is correct but was interpreted as UTC.
			// We need to treat it as if it were in the original timezone.
			// Create a new time with the same wall clock but in the original timezone.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
		}
	}

	// Apply manual time offset if configured
	if source.TimeOffset == "" {
		return t
	}

	// Parse offset in formats like "+01:00", "-05:00", "+0100", "-0500"
	offset := source.TimeOffset
	if len(offset) < 5 {
		return t
	}

	sign := 1
	if offset[0] == '-' {
		sign = -1
		offset = offset[1:]
	} else if offset[0] == '+' {
		offset = offset[1:]
	}

	var hours, minutes int
	if len(offset) >= 5 && offset[2] == ':' {
		// Format: "01:00"
		fmt.Sscanf(offset, "%02d:%02d", &hours, &minutes)
	} else if len(offset) >= 4 {
		// Format: "0100"
		fmt.Sscanf(offset, "%02d%02d", &hours, &minutes)
	} else {
		return t
	}

	duration := time.Duration(sign) * (time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute)
	return t.Add(duration)
}

// Ensure XMLTVHandler implements EpgHandler.
var _ EpgHandler = (*XMLTVHandler)(nil)
