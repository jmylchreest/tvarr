// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DASHPassthroughConfig configures the DASH passthrough handler.
type DASHPassthroughConfig struct {
	// HTTPClient is the HTTP client for upstream requests.
	HTTPClient *http.Client

	// ManifestRefreshInterval is how often to refresh the manifest.
	ManifestRefreshInterval time.Duration

	// SegmentCacheSize is the maximum number of segments to cache.
	SegmentCacheSize int

	// UserAgent is the User-Agent header for upstream requests.
	UserAgent string
}

// DefaultDASHPassthroughConfig returns sensible defaults.
func DefaultDASHPassthroughConfig() DASHPassthroughConfig {
	return DASHPassthroughConfig{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ManifestRefreshInterval: 2 * time.Second,
		SegmentCacheSize:        10,
		UserAgent:               "tvarr/1.0",
	}
}

// DASHPassthroughHandler proxies DASH streams with URL rewriting and caching.
// It fetches manifests from upstream, rewrites segment URLs to point to the proxy,
// and caches segments locally to reduce upstream connections.
type DASHPassthroughHandler struct {
	config      DASHPassthroughConfig
	baseURL     string // Base URL for proxy
	upstreamURL string // Upstream manifest URL

	mu                sync.RWMutex
	cachedManifest    string // Cached and rewritten manifest
	lastManifestFetch time.Time
	segments          map[string]*cachedDASHSegment // segment URL -> cached data
	initSegments      map[string]*cachedDASHSegment // init segment URL -> cached data

	// Segment URL mapping: proxy ID -> upstream URL
	segmentMapping map[string]string
	initMapping    map[string]string
}

// cachedDASHSegment holds a cached segment and its metadata.
type cachedDASHSegment struct {
	data      []byte
	fetchedAt time.Time
}

// NewDASHPassthroughHandler creates a new DASH passthrough handler.
func NewDASHPassthroughHandler(upstreamURL, baseURL string, config DASHPassthroughConfig) *DASHPassthroughHandler {
	if config.HTTPClient == nil {
		config.HTTPClient = DefaultDASHPassthroughConfig().HTTPClient
	}
	if config.ManifestRefreshInterval == 0 {
		config.ManifestRefreshInterval = DefaultDASHPassthroughConfig().ManifestRefreshInterval
	}
	if config.SegmentCacheSize == 0 {
		config.SegmentCacheSize = DefaultDASHPassthroughConfig().SegmentCacheSize
	}

	return &DASHPassthroughHandler{
		config:         config,
		baseURL:        strings.TrimSuffix(baseURL, "/"),
		upstreamURL:    upstreamURL,
		segments:       make(map[string]*cachedDASHSegment),
		initSegments:   make(map[string]*cachedDASHSegment),
		segmentMapping: make(map[string]string),
		initMapping:    make(map[string]string),
	}
}

// ServeManifest fetches, rewrites, and serves the DASH manifest.
func (d *DASHPassthroughHandler) ServeManifest(ctx context.Context, w http.ResponseWriter) error {
	manifest, err := d.getRewrittenManifest(ctx)
	if err != nil {
		http.Error(w, "failed to fetch manifest", http.StatusBadGateway)
		return err
	}

	w.Header().Set("Content-Type", ContentTypeDASHManifest)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write([]byte(manifest))
	return err
}

// ServeSegment fetches (or returns cached) and serves a media segment.
func (d *DASHPassthroughHandler) ServeSegment(ctx context.Context, w http.ResponseWriter, segmentID string) error {
	d.mu.RLock()
	upstreamURL, ok := d.segmentMapping[segmentID]
	d.mu.RUnlock()

	if !ok {
		http.Error(w, "segment not found", http.StatusNotFound)
		return ErrSegmentNotFound
	}

	data, err := d.getSegment(ctx, upstreamURL)
	if err != nil {
		http.Error(w, "failed to fetch segment", http.StatusBadGateway)
		return err
	}

	w.Header().Set("Content-Type", ContentTypeDASHSegment)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "max-age=86400")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(data)
	return err
}

// ServeInitSegment fetches (or returns cached) and serves an initialization segment.
func (d *DASHPassthroughHandler) ServeInitSegment(ctx context.Context, w http.ResponseWriter, initID string) error {
	d.mu.RLock()
	upstreamURL, ok := d.initMapping[initID]
	d.mu.RUnlock()

	if !ok {
		http.Error(w, "init segment not found", http.StatusNotFound)
		return ErrSegmentNotFound
	}

	data, err := d.getInitSegment(ctx, upstreamURL)
	if err != nil {
		http.Error(w, "failed to fetch init segment", http.StatusBadGateway)
		return err
	}

	w.Header().Set("Content-Type", ContentTypeDASHInit)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "max-age=86400")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(data)
	return err
}

// getRewrittenManifest fetches the upstream manifest and rewrites URLs.
func (d *DASHPassthroughHandler) getRewrittenManifest(ctx context.Context) (string, error) {
	d.mu.RLock()
	if time.Since(d.lastManifestFetch) < d.config.ManifestRefreshInterval && d.cachedManifest != "" {
		manifest := d.cachedManifest
		d.mu.RUnlock()
		return manifest, nil
	}
	d.mu.RUnlock()

	// Fetch upstream manifest
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.upstreamURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	if d.config.UserAgent != "" {
		req.Header.Set("User-Agent", d.config.UserAgent)
	}

	resp, err := d.config.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading manifest: %w", err)
	}

	// Parse and rewrite manifest
	manifest, err := d.parseAndRewriteManifest(string(body))
	if err != nil {
		return "", fmt.Errorf("parsing manifest: %w", err)
	}

	// Update cache
	d.mu.Lock()
	d.cachedManifest = manifest
	d.lastManifestFetch = time.Now()
	d.mu.Unlock()

	return manifest, nil
}

// parseAndRewriteManifest parses a DASH manifest and rewrites segment URLs.
// This implementation uses regex-based rewriting for simplicity and robustness
// against different MPD structures.
func (d *DASHPassthroughHandler) parseAndRewriteManifest(manifestStr string) (string, error) {
	// Parse upstream base URL for resolving relative URLs
	upstreamBase, err := url.Parse(d.upstreamURL)
	if err != nil {
		return "", fmt.Errorf("parsing upstream URL: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Clear old mappings
	d.segmentMapping = make(map[string]string)
	d.initMapping = make(map[string]string)

	segmentCounter := 0
	initCounter := 0

	result := manifestStr

	// Rewrite initialization URLs
	initPattern := regexp.MustCompile(`initialization="([^"]+)"`)
	result = initPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := initPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		origURL := submatches[1]

		// Skip if it's already a template variable
		if strings.Contains(origURL, "$") {
			// Handle templates with $RepresentationID$ etc.
			initID := fmt.Sprintf("init_%d", initCounter)
			initCounter++
			// Store a placeholder - actual URL will be resolved at request time
			resolvedURL := resolveDASHURL(upstreamBase, origURL)
			d.initMapping[initID] = resolvedURL
			proxyURL := fmt.Sprintf("%s?%s=%s&%s=%s",
				d.baseURL,
				QueryParamFormat, FormatValueDASH,
				QueryParamInit, initID,
			)
			return fmt.Sprintf(`initialization="%s"`, proxyURL)
		}

		initID := fmt.Sprintf("init_%d", initCounter)
		initCounter++
		resolvedURL := resolveDASHURL(upstreamBase, origURL)
		d.initMapping[initID] = resolvedURL
		proxyURL := fmt.Sprintf("%s?%s=%s&%s=%s",
			d.baseURL,
			QueryParamFormat, FormatValueDASH,
			QueryParamInit, initID,
		)
		return fmt.Sprintf(`initialization="%s"`, proxyURL)
	})

	// Rewrite media URLs (SegmentTemplate media attribute)
	mediaPattern := regexp.MustCompile(`media="([^"]+)"`)
	result = mediaPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := mediaPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		origURL := submatches[1]

		// For SegmentTemplate, we keep the template pattern but rewrite the base
		if strings.Contains(origURL, "$") {
			// This is a template - we'll handle it by storing the template
			segID := fmt.Sprintf("seg_%d", segmentCounter)
			segmentCounter++
			resolvedURL := resolveDASHURL(upstreamBase, origURL)
			d.segmentMapping[segID] = resolvedURL
			// Create a proxy URL pattern
			proxyURL := fmt.Sprintf("%s?%s=%s&%s=%s",
				d.baseURL,
				QueryParamFormat, FormatValueDASH,
				QueryParamSegment, segID,
			)
			return fmt.Sprintf(`media="%s"`, proxyURL)
		}

		segID := fmt.Sprintf("seg_%d", segmentCounter)
		segmentCounter++
		resolvedURL := resolveDASHURL(upstreamBase, origURL)
		d.segmentMapping[segID] = resolvedURL
		proxyURL := fmt.Sprintf("%s?%s=%s&%s=%s",
			d.baseURL,
			QueryParamFormat, FormatValueDASH,
			QueryParamSegment, segID,
		)
		return fmt.Sprintf(`media="%s"`, proxyURL)
	})

	// Rewrite BaseURL elements
	baseURLPattern := regexp.MustCompile(`<BaseURL>([^<]+)</BaseURL>`)
	result = baseURLPattern.ReplaceAllStringFunc(result, func(match string) string {
		// For simplicity, we remove BaseURL as we're rewriting all segment URLs
		// This prevents double-resolution issues
		return ""
	})

	return result, nil
}

// getSegment fetches a segment from cache or upstream.
func (d *DASHPassthroughHandler) getSegment(ctx context.Context, segmentURL string) ([]byte, error) {
	// Check cache first
	d.mu.RLock()
	if cached, ok := d.segments[segmentURL]; ok {
		d.mu.RUnlock()
		return cached.data, nil
	}
	d.mu.RUnlock()

	// Fetch from upstream
	data, err := d.fetchFromUpstream(ctx, segmentURL)
	if err != nil {
		return nil, err
	}

	// Cache the segment
	d.mu.Lock()
	d.segments[segmentURL] = &cachedDASHSegment{
		data:      data,
		fetchedAt: time.Now(),
	}
	d.evictOldSegments()
	d.mu.Unlock()

	return data, nil
}

// getInitSegment fetches an init segment from cache or upstream.
func (d *DASHPassthroughHandler) getInitSegment(ctx context.Context, initURL string) ([]byte, error) {
	// Check cache first
	d.mu.RLock()
	if cached, ok := d.initSegments[initURL]; ok {
		d.mu.RUnlock()
		return cached.data, nil
	}
	d.mu.RUnlock()

	// Fetch from upstream
	data, err := d.fetchFromUpstream(ctx, initURL)
	if err != nil {
		return nil, err
	}

	// Cache the init segment (these are cached longer as they rarely change)
	d.mu.Lock()
	d.initSegments[initURL] = &cachedDASHSegment{
		data:      data,
		fetchedAt: time.Now(),
	}
	d.mu.Unlock()

	return data, nil
}

// fetchFromUpstream fetches data from an upstream URL.
func (d *DASHPassthroughHandler) fetchFromUpstream(ctx context.Context, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if d.config.UserAgent != "" {
		req.Header.Set("User-Agent", d.config.UserAgent)
	}

	resp, err := d.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading: %w", err)
	}

	return data, nil
}

// evictOldSegments removes old segments from cache (must hold write lock).
func (d *DASHPassthroughHandler) evictOldSegments() {
	if len(d.segments) <= d.config.SegmentCacheSize {
		return
	}

	// Remove oldest segments
	for len(d.segments) > d.config.SegmentCacheSize {
		var oldest string
		var oldestTime time.Time
		for url, seg := range d.segments {
			if oldest == "" || seg.fetchedAt.Before(oldestTime) {
				oldest = url
				oldestTime = seg.fetchedAt
			}
		}
		if oldest != "" {
			delete(d.segments, oldest)
		}
	}
}

// GetUpstreamURL returns the upstream manifest URL.
func (d *DASHPassthroughHandler) GetUpstreamURL() string {
	return d.upstreamURL
}

// CacheStats returns cache statistics.
func (d *DASHPassthroughHandler) CacheStats() DASHPassthroughStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var totalSize int64
	for _, seg := range d.segments {
		totalSize += int64(len(seg.data))
	}
	for _, seg := range d.initSegments {
		totalSize += int64(len(seg.data))
	}

	return DASHPassthroughStats{
		CachedSegments:     len(d.segments),
		CachedInitSegments: len(d.initSegments),
		TotalCacheSize:     totalSize,
		SegmentMappings:    len(d.segmentMapping),
		InitMappings:       len(d.initMapping),
		LastManifestFetch:  d.lastManifestFetch,
	}
}

// DASHPassthroughStats holds statistics for the passthrough handler.
type DASHPassthroughStats struct {
	CachedSegments     int       `json:"cached_segments"`
	CachedInitSegments int       `json:"cached_init_segments"`
	TotalCacheSize     int64     `json:"total_cache_size"`
	SegmentMappings    int       `json:"segment_mappings"`
	InitMappings       int       `json:"init_mappings"`
	LastManifestFetch  time.Time `json:"last_manifest_fetch"`
}

// resolveDASHURL resolves a potentially relative URL against a base URL.
func resolveDASHURL(base *url.URL, ref string) string {
	// Handle absolute URLs
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}

	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}

// Ensure xml package is available for potential future use
var _ = xml.Unmarshal
