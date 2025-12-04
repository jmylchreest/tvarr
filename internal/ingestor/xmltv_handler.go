// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
)

// XMLTV handler configuration defaults.
const (
	defaultXMLTVTimeout = 5 * time.Minute
)

// XMLTVHandler handles XMLTV EPG source ingestion.
type XMLTVHandler struct {
	// httpClient is the resilient HTTP client used for fetching remote XMLTV files.
	httpClient *httpclient.Client

	// channelMap stores channel ID to external ID mapping during ingestion.
	// This maps XMLTV channel IDs to be used when processing programmes.
	channelMap map[string]string
}

// NewXMLTVHandler creates a new XMLTV handler with default settings.
func NewXMLTVHandler() *XMLTVHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultXMLTVTimeout

	return &XMLTVHandler{
		httpClient: httpclient.New(cfg),
		channelMap: make(map[string]string),
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *XMLTVHandler) WithHTTPClientConfig(cfg httpclient.Config) *XMLTVHandler {
	h.httpClient = httpclient.New(cfg)
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
	if !strings.HasPrefix(source.URL, httpPrefix) && !strings.HasPrefix(source.URL, httpsPrefix) {
		return fmt.Errorf("URL must be an HTTP(S) URL")
	}
	return nil
}

// Ingest processes an XMLTV EPG source and yields programs via the callback.
func (h *XMLTVHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Fetch the XMLTV file
	reader, err := h.fetch(ctx, source.URL)
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
			program := h.convertProgramme(programme, source.ID)

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

// fetch retrieves the XMLTV file from a URL using the resilient HTTP client.
func (h *XMLTVHandler) fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	resp, err := h.httpClient.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// convertProgramme converts an XMLTV Programme to an EpgProgram model.
func (h *XMLTVHandler) convertProgramme(p *xmltv.Programme, sourceID models.ULID) *models.EpgProgram {
	program := &models.EpgProgram{
		SourceID:    sourceID,
		ChannelID:   p.Channel,
		Start:       p.Start,
		Stop:        p.Stop,
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

// Ensure XMLTVHandler implements EpgHandler.
var _ EpgHandler = (*XMLTVHandler)(nil)
