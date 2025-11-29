package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/httpclient"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/m3u"
)

// M3U handler configuration defaults.
const (
	defaultM3UTimeout = 5 * time.Minute
)

// URL scheme prefixes.
const (
	httpPrefix  = "http://"
	httpsPrefix = "https://"
)

// M3UHandler handles ingestion of M3U playlist sources.
type M3UHandler struct {
	// httpClient is the resilient HTTP client used for fetching remote M3U files.
	httpClient *httpclient.Client
}

// NewM3UHandler creates a new M3U handler with default settings.
func NewM3UHandler() *M3UHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultM3UTimeout

	return &M3UHandler{
		httpClient: httpclient.New(cfg),
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *M3UHandler) WithHTTPClientConfig(cfg httpclient.Config) *M3UHandler {
	h.httpClient = httpclient.New(cfg)
	return h
}

// Type returns the source type this handler supports.
func (h *M3UHandler) Type() models.SourceType {
	return models.SourceTypeM3U
}

// Validate checks if the source configuration is valid for M3U ingestion.
func (h *M3UHandler) Validate(source *models.StreamSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.SourceTypeM3U {
		return fmt.Errorf("source type must be m3u, got %s", source.Type)
	}
	if source.URL == "" {
		return fmt.Errorf("source URL is required")
	}
	// Basic URL validation
	if !strings.HasPrefix(source.URL, httpPrefix) && !strings.HasPrefix(source.URL, httpsPrefix) {
		return fmt.Errorf("source URL must be HTTP or HTTPS")
	}
	return nil
}

// Ingest fetches and parses the M3U source, calling the callback for each channel.
func (h *M3UHandler) Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Fetch the M3U content
	body, err := h.fetch(ctx, source.URL)
	if err != nil {
		return fmt.Errorf("fetching M3U: %w", err)
	}
	defer body.Close()

	// Parse with streaming callbacks
	parser := &m3u.Parser{
		OnEntry: func(entry *m3u.Entry) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Convert M3U entry to Channel model
			channel := h.entryToChannel(entry, source.ID)

			// Call the callback
			return callback(channel)
		},
		OnError: func(lineNum int, err error) {
			// Log parse errors but continue processing
			// In production, you might want to collect these
		},
	}

	// Parse with automatic compression detection
	if err := parser.ParseCompressed(body); err != nil {
		return fmt.Errorf("parsing M3U: %w", err)
	}

	return nil
}

// fetch retrieves content from a URL using the resilient HTTP client.
func (h *M3UHandler) fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	resp, err := h.httpClient.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// entryToChannel converts an M3U entry to a Channel model.
func (h *M3UHandler) entryToChannel(entry *m3u.Entry, sourceID models.ULID) *models.Channel {
	channel := &models.Channel{
		SourceID:      sourceID,
		TvgID:         entry.TvgID,
		TvgName:       entry.TvgName,
		TvgLogo:       entry.TvgLogo,
		GroupTitle:    entry.GroupTitle,
		ChannelName:   h.determineChannelName(entry),
		ChannelNumber: entry.ChannelNumber,
		StreamURL:     entry.URL,
	}

	// Generate ExtID for deduplication
	channel.ExtID = h.generateExtID(entry)

	// Store extra attributes as JSON
	if len(entry.Extra) > 0 {
		if extraJSON, err := json.Marshal(entry.Extra); err == nil {
			channel.Extra = string(extraJSON)
		}
	}

	return channel
}

// determineChannelName determines the best channel name from the entry.
func (h *M3UHandler) determineChannelName(entry *m3u.Entry) string {
	// Priority: Title > TvgName > derived from URL
	if entry.Title != "" {
		return entry.Title
	}
	if entry.TvgName != "" {
		return entry.TvgName
	}
	// Fallback to extracting from URL
	return extractNameFromURL(entry.URL)
}

// generateExtID generates a unique external ID for deduplication.
func (h *M3UHandler) generateExtID(entry *m3u.Entry) string {
	// Priority: TvgID > URL hash
	if entry.TvgID != "" {
		return entry.TvgID
	}
	// Use URL as the identifier since it's unique within a source
	return entry.URL
}

// extractNameFromURL extracts a channel name from a URL as a fallback.
func extractNameFromURL(url string) string {
	// Find the last path segment
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash >= 0 && lastSlash < len(url)-1 {
		name := url[lastSlash+1:]
		// Remove query string
		if qMark := strings.Index(name, "?"); qMark > 0 {
			name = name[:qMark]
		}
		// Remove extension
		if dot := strings.LastIndex(name, "."); dot > 0 {
			name = name[:dot]
		}
		if name != "" {
			return name
		}
	}
	return "Unknown"
}

// IngestFromReader ingests from an io.Reader instead of fetching from URL.
// This is useful for testing or processing local files.
func (h *M3UHandler) IngestFromReader(ctx context.Context, r io.Reader, sourceID models.ULID, callback ChannelCallback) error {
	parser := &m3u.Parser{
		OnEntry: func(entry *m3u.Entry) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			channel := h.entryToChannel(entry, sourceID)
			return callback(channel)
		},
	}

	return parser.ParseCompressed(r)
}
