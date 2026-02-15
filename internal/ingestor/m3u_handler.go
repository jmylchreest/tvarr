package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/urlutil"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/m3u"
)

// M3U handler configuration defaults.
const (
	defaultM3UTimeout = 5 * time.Minute
)

// M3UHandler handles ingestion of M3U playlist sources.
type M3UHandler struct {
	fetcher *urlutil.ResourceFetcher
}

// NewM3UHandler creates a new M3U handler with default settings.
func NewM3UHandler() *M3UHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultM3UTimeout

	return &M3UHandler{
		fetcher: urlutil.NewResourceFetcher(cfg),
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *M3UHandler) WithHTTPClientConfig(cfg httpclient.Config) *M3UHandler {
	h.fetcher = urlutil.NewResourceFetcher(cfg)
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
	if !urlutil.IsSupportedURL(source.URL) {
		return fmt.Errorf("source URL must be HTTP, HTTPS, or file:// URL")
	}
	return nil
}

// breakerNameFromURL extracts a circuit breaker name from the source URL.
func m3uBreakerNameFromURL(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Host == "" {
		return "m3u-ingestion"
	}
	return "ingestion-" + parsed.Host
}

// Ingest fetches and parses the M3U source, calling the callback for each channel.
func (h *M3UHandler) Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	breakerName := m3uBreakerNameFromURL(source.URL)
	breaker := httpclient.DefaultManager.GetOrCreate(breakerName)

	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultM3UTimeout
	fetcher := urlutil.NewResourceFetcherWithBreaker(cfg, breaker)

	body, err := fetcher.Fetch(ctx, source.URL)
	if err != nil {
		return fmt.Errorf("fetching M3U: %w", err)
	}
	defer body.Close()

	parser := &m3u.Parser{
		OnEntry: func(entry *m3u.Entry) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			channel := h.entryToChannel(entry, source.ID)

			return callback(channel)
		},
		OnError: func(lineNum int, err error) {
		},
	}

	if err := parser.ParseCompressed(body); err != nil {
		return fmt.Errorf("parsing M3U: %w", err)
	}

	return nil
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

	channel.ExtID = h.generateExtID(entry)

	if len(entry.Extra) > 0 {
		if extraJSON, err := json.Marshal(entry.Extra); err == nil {
			channel.Extra = string(extraJSON)
		}
	}

	return channel
}

// determineChannelName determines the best channel name from the entry.
func (h *M3UHandler) determineChannelName(entry *m3u.Entry) string {
	if entry.Title != "" {
		return entry.Title
	}
	if entry.TvgName != "" {
		return entry.TvgName
	}
	return extractNameFromURL(entry.URL)
}

// generateExtID generates a unique external ID for deduplication.
func (h *M3UHandler) generateExtID(entry *m3u.Entry) string {
	if entry.TvgID != "" {
		return entry.TvgID
	}
	return entry.URL
}

// extractNameFromURL extracts a channel name from a URL as a fallback.
func extractNameFromURL(url string) string {
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash >= 0 && lastSlash < len(url)-1 {
		name := url[lastSlash+1:]
		if qMark := strings.Index(name, "?"); qMark > 0 {
			name = name[:qMark]
		}
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
