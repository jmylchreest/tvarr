// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HLSPassthroughConfig configures the HLS passthrough handler.
type HLSPassthroughConfig struct {
	// HTTPClient is the HTTP client for upstream requests.
	HTTPClient *http.Client

	// PlaylistRefreshInterval is how often to refresh the playlist.
	PlaylistRefreshInterval time.Duration

	// SegmentCacheSize is the maximum number of segments to cache.
	SegmentCacheSize int

	// UserAgent is the User-Agent header for upstream requests.
	UserAgent string
}

// DefaultHLSPassthroughConfig returns sensible defaults.
func DefaultHLSPassthroughConfig() HLSPassthroughConfig {
	return HLSPassthroughConfig{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		PlaylistRefreshInterval: 2 * time.Second,
		SegmentCacheSize:        10,
		UserAgent:               "tvarr/1.0",
	}
}

// HLSPassthroughHandler proxies HLS streams with URL rewriting and caching.
// It fetches playlists from upstream, rewrites segment URLs to point to the proxy,
// and caches segments locally to reduce upstream connections.
type HLSPassthroughHandler struct {
	config      HLSPassthroughConfig
	baseURL     string // Base URL for proxy (e.g., http://localhost:8080/stream/123)
	upstreamURL string // Upstream playlist URL

	mu                sync.RWMutex
	cachedPlaylist    string // Cached and rewritten playlist
	lastPlaylistFetch time.Time
	segments          map[string]*cachedSegment // segment URL -> cached data

	// Upstream playlist parsing state
	mediaSequence  uint64
	targetDuration int
	segmentURLs    []string // Ordered list of segment URLs from upstream
}

// cachedSegment holds a cached segment and its metadata.
type cachedSegment struct {
	data      []byte
	fetchedAt time.Time
}

// NewHLSPassthroughHandler creates a new HLS passthrough handler.
func NewHLSPassthroughHandler(upstreamURL, baseURL string, config HLSPassthroughConfig) *HLSPassthroughHandler {
	if config.HTTPClient == nil {
		config.HTTPClient = DefaultHLSPassthroughConfig().HTTPClient
	}
	if config.PlaylistRefreshInterval == 0 {
		config.PlaylistRefreshInterval = DefaultHLSPassthroughConfig().PlaylistRefreshInterval
	}
	if config.SegmentCacheSize == 0 {
		config.SegmentCacheSize = DefaultHLSPassthroughConfig().SegmentCacheSize
	}

	return &HLSPassthroughHandler{
		config:      config,
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		upstreamURL: upstreamURL,
		segments:    make(map[string]*cachedSegment),
	}
}

// ServePlaylist fetches, rewrites, and serves the HLS playlist.
func (h *HLSPassthroughHandler) ServePlaylist(ctx context.Context, w http.ResponseWriter) error {
	playlist, err := h.getRewrittenPlaylist(ctx)
	if err != nil {
		http.Error(w, "failed to fetch playlist", http.StatusBadGateway)
		return err
	}

	w.Header().Set("Content-Type", ContentTypeHLSPlaylist)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write([]byte(playlist))
	return err
}

// ServeSegment fetches (or returns cached) and serves a segment.
func (h *HLSPassthroughHandler) ServeSegment(ctx context.Context, w http.ResponseWriter, segmentIndex int) error {
	h.mu.RLock()
	if segmentIndex < 0 || segmentIndex >= len(h.segmentURLs) {
		h.mu.RUnlock()
		http.Error(w, "segment not found", http.StatusNotFound)
		return ErrSegmentNotFound
	}
	segmentURL := h.segmentURLs[segmentIndex]
	h.mu.RUnlock()

	data, err := h.getSegment(ctx, segmentURL)
	if err != nil {
		http.Error(w, "failed to fetch segment", http.StatusBadGateway)
		return err
	}

	w.Header().Set("Content-Type", ContentTypeHLSSegment)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "max-age=86400")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(data)
	return err
}

// getRewrittenPlaylist fetches the upstream playlist and rewrites URLs.
func (h *HLSPassthroughHandler) getRewrittenPlaylist(ctx context.Context) (string, error) {
	h.mu.RLock()
	if time.Since(h.lastPlaylistFetch) < h.config.PlaylistRefreshInterval && h.cachedPlaylist != "" {
		playlist := h.cachedPlaylist
		h.mu.RUnlock()
		return playlist, nil
	}
	h.mu.RUnlock()

	// Fetch upstream playlist
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.upstreamURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	if h.config.UserAgent != "" {
		req.Header.Set("User-Agent", h.config.UserAgent)
	}

	resp, err := h.config.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	// Parse and rewrite playlist
	playlist, segmentURLs, err := h.parseAndRewritePlaylist(resp.Body)
	if err != nil {
		return "", fmt.Errorf("parsing playlist: %w", err)
	}

	// Update cache
	h.mu.Lock()
	h.cachedPlaylist = playlist
	h.segmentURLs = segmentURLs
	h.lastPlaylistFetch = time.Now()
	h.mu.Unlock()

	return playlist, nil
}

// parseAndRewritePlaylist parses an HLS playlist and rewrites segment URLs.
func (h *HLSPassthroughHandler) parseAndRewritePlaylist(r io.Reader) (string, []string, error) {
	var sb strings.Builder
	var segmentURLs []string
	segmentIndex := 0

	// Parse upstream base URL for resolving relative URLs
	upstreamBase, err := url.Parse(h.upstreamURL)
	if err != nil {
		return "", nil, fmt.Errorf("parsing upstream URL: %w", err)
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			// Copy tag lines as-is
			sb.WriteString(line)
			sb.WriteString("\n")

			// Parse target duration
			if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
				fmt.Sscanf(line, "#EXT-X-TARGETDURATION:%d", &h.targetDuration)
			}
			// Parse media sequence
			if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
				fmt.Sscanf(line, "#EXT-X-MEDIA-SEQUENCE:%d", &h.mediaSequence)
			}
		} else if line != "" {
			// This is a segment URL - resolve and rewrite
			segmentURL := resolveURL(upstreamBase, line)
			segmentURLs = append(segmentURLs, segmentURL)

			// Rewrite to proxy URL
			proxyURL := fmt.Sprintf("%s?%s=%s&%s=%d",
				h.baseURL,
				QueryParamFormat, FormatValueHLS,
				QueryParamSegment, segmentIndex,
			)
			sb.WriteString(proxyURL)
			sb.WriteString("\n")
			segmentIndex++
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("scanning playlist: %w", err)
	}

	return sb.String(), segmentURLs, nil
}

// getSegment fetches a segment from cache or upstream.
func (h *HLSPassthroughHandler) getSegment(ctx context.Context, segmentURL string) ([]byte, error) {
	// Check cache first
	h.mu.RLock()
	if cached, ok := h.segments[segmentURL]; ok {
		h.mu.RUnlock()
		return cached.data, nil
	}
	h.mu.RUnlock()

	// Fetch from upstream
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, segmentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if h.config.UserAgent != "" {
		req.Header.Set("User-Agent", h.config.UserAgent)
	}

	resp, err := h.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching segment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading segment: %w", err)
	}

	// Cache the segment
	h.mu.Lock()
	h.segments[segmentURL] = &cachedSegment{
		data:      data,
		fetchedAt: time.Now(),
	}
	// Evict old segments if needed
	h.evictOldSegments()
	h.mu.Unlock()

	return data, nil
}

// evictOldSegments removes old segments from cache (must hold write lock).
func (h *HLSPassthroughHandler) evictOldSegments() {
	if len(h.segments) <= h.config.SegmentCacheSize {
		return
	}

	// Find oldest segments not in current playlist
	currentSegments := make(map[string]bool)
	for _, url := range h.segmentURLs {
		currentSegments[url] = true
	}

	// Remove segments not in current playlist first
	for url := range h.segments {
		if !currentSegments[url] {
			delete(h.segments, url)
			if len(h.segments) <= h.config.SegmentCacheSize {
				return
			}
		}
	}

	// If still over limit, remove oldest
	if len(h.segments) > h.config.SegmentCacheSize {
		var oldest string
		var oldestTime time.Time
		for url, seg := range h.segments {
			if oldest == "" || seg.fetchedAt.Before(oldestTime) {
				oldest = url
				oldestTime = seg.fetchedAt
			}
		}
		if oldest != "" {
			delete(h.segments, oldest)
		}
	}
}

// GetUpstreamURL returns the upstream playlist URL.
func (h *HLSPassthroughHandler) GetUpstreamURL() string {
	return h.upstreamURL
}

// GetMediaSequence returns the current media sequence number.
func (h *HLSPassthroughHandler) GetMediaSequence() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mediaSequence
}

// GetTargetDuration returns the target duration.
func (h *HLSPassthroughHandler) GetTargetDuration() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.targetDuration
}

// CacheStats returns cache statistics.
func (h *HLSPassthroughHandler) CacheStats() HLSPassthroughStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var totalSize int64
	for _, seg := range h.segments {
		totalSize += int64(len(seg.data))
	}

	return HLSPassthroughStats{
		CachedSegments:    len(h.segments),
		TotalCacheSize:    totalSize,
		PlaylistSegments:  len(h.segmentURLs),
		MediaSequence:     h.mediaSequence,
		TargetDuration:    h.targetDuration,
		LastPlaylistFetch: h.lastPlaylistFetch,
	}
}

// HLSPassthroughStats holds statistics for the passthrough handler.
type HLSPassthroughStats struct {
	CachedSegments    int       `json:"cached_segments"`
	TotalCacheSize    int64     `json:"total_cache_size"`
	PlaylistSegments  int       `json:"playlist_segments"`
	MediaSequence     uint64    `json:"media_sequence"`
	TargetDuration    int       `json:"target_duration"`
	LastPlaylistFetch time.Time `json:"last_playlist_fetch"`
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(base *url.URL, ref string) string {
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}
