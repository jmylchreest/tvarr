// Package service provides business logic layer for tvarr operations.
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/jmylchreest/tvarr/internal/storage"
)

// HTTPClient defines the interface for HTTP operations.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// LogoStats holds statistics about cached logos.
type LogoStats struct {
	TotalLogos    int
	TotalSize     int64
	CachedLogos   int   // URL-sourced logos (prunable)
	UploadedLogos int   // Manually uploaded logos (never pruned)
	CachedSize    int64 // Total size of cached logos
	UploadedSize  int64 // Total size of uploaded logos
}

// LogoService provides business logic for logo caching.
// It uses file-based storage with an in-memory index for fast lookups.
type LogoService struct {
	cache      *storage.LogoCache
	indexer    *LogoIndexer
	httpClient HTTPClient
	logger     *slog.Logger
}

// NewLogoService creates a new LogoService.
func NewLogoService(cache *storage.LogoCache) *LogoService {
	indexer := NewLogoIndexer(cache)
	return &LogoService{
		cache:      cache,
		indexer:    indexer,
		httpClient: http.DefaultClient,
		logger:     slog.Default(),
	}
}

// WithHTTPClient sets a custom HTTP client for testing.
func (s *LogoService) WithHTTPClient(client HTTPClient) *LogoService {
	s.httpClient = client
	return s
}

// WithLogger sets the logger for the service.
func (s *LogoService) WithLogger(logger *slog.Logger) *LogoService {
	s.logger = logger
	s.indexer = s.indexer.WithLogger(logger)
	return s
}

// LoadIndex loads the logo index from disk.
// Should be called on startup.
// For pruning stale logos during load, use LoadIndexWithOptions instead.
func (s *LogoService) LoadIndex(ctx context.Context) error {
	return s.indexer.LoadFromDisk(ctx)
}

// LoadIndexWithOptions loads the logo index from disk with configurable options.
// If opts.PruneStaleLogos is true and opts.StalenessThreshold > 0, stale cached logos
// are deleted during load. Uploaded logos are never pruned.
func (s *LogoService) LoadIndexWithOptions(ctx context.Context, opts LogoIndexerOptions) (*LogoIndexerLoadResult, error) {
	return s.indexer.LoadFromDiskWithOptions(ctx, opts)
}

// CacheLogo downloads a logo from the given URL and stores it in the cache.
// If the logo is already cached, updates LastSeenAt and returns existing metadata.
// This is the main entry point for caching logos.
//
// The logo ID is deterministic - same normalized URL always produces same ID,
// ensuring logos shared across multiple channels are only downloaded once.
func (s *LogoService) CacheLogo(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error) {
	// Check if already cached
	if existing := s.indexer.GetByURL(logoURL); existing != nil {
		// Touch to update LastSeenAt (for freshness-based pruning)
		if err := s.TouchLogo(existing); err != nil {
			s.logger.Warn("failed to touch logo",
				"url", logoURL,
				"error", err)
		}
		s.logger.Debug("logo already cached",
			"url", logoURL,
			"id", existing.GetID())
		return existing, nil
	}

	// Also check by normalized URL hash (in case URL varies slightly)
	normalizedHash := storage.NewCachedLogoMetadata(logoURL).GetID()
	if existing := s.indexer.GetByID(normalizedHash); existing != nil {
		// Touch to update LastSeenAt
		if err := s.TouchLogo(existing); err != nil {
			s.logger.Warn("failed to touch logo",
				"url", logoURL,
				"error", err)
		}
		// Also index by this URL variant for future lookups
		s.indexer.AddURLMapping(logoURL, existing)
		s.logger.Debug("logo already cached (different URL variant)",
			"url", logoURL,
			"id", existing.GetID())
		return existing, nil
	}

	// Download the logo
	meta, err := s.downloadAndStore(ctx, logoURL)
	if err != nil {
		return nil, fmt.Errorf("downloading logo: %w", err)
	}

	// Add to index
	s.indexer.Add(meta)

	s.logger.Debug("cached logo",
		"url", logoURL,
		"id", meta.GetID())

	return meta, nil
}

// TouchLogo updates the LastSeenAt timestamp for a logo.
// Called when a logo URL is encountered during pipeline processing.
// This enables time-based pruning of logos no longer in use.
func (s *LogoService) TouchLogo(meta *storage.CachedLogoMetadata) error {
	return s.cache.TouchMetadata(meta)
}

// downloadAndStore downloads a logo and stores it in the cache.
func (s *LogoService) downloadAndStore(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Read body into memory (logos should be reasonably small)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Get content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png" // Default to PNG
	}

	// Create metadata
	meta := storage.NewCachedLogoMetadata(logoURL)
	meta.ContentType = contentType

	// Store with metadata
	if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(body)); err != nil {
		return nil, fmt.Errorf("storing logo: %w", err)
	}

	return meta, nil
}

// GetCachedLogo retrieves a cached logo by URL.
// Returns nil if not cached.
func (s *LogoService) GetCachedLogo(logoURL string) *storage.CachedLogoMetadata {
	return s.indexer.GetByURL(logoURL)
}

// GetLogoByID retrieves a cached logo by its ID.
// Returns nil if not found.
func (s *LogoService) GetLogoByID(id string) *storage.CachedLogoMetadata {
	return s.indexer.GetByID(id)
}

// Contains checks if a logo URL is already cached.
func (s *LogoService) Contains(logoURL string) bool {
	return s.indexer.Contains(logoURL)
}

// GetLogoFile opens a logo file for reading.
func (s *LogoService) GetLogoFile(meta *storage.CachedLogoMetadata) (io.ReadCloser, error) {
	return s.cache.Get(meta.RelativeImagePath())
}

// GetLogoAbsolutePath returns the absolute filesystem path for a logo image.
func (s *LogoService) GetLogoAbsolutePath(meta *storage.CachedLogoMetadata) (string, error) {
	return s.cache.GetImageAbsolutePath(meta.GetID(), meta.ContentType)
}

// DeleteLogo removes a logo from both the cache and the index.
func (s *LogoService) DeleteLogo(id string) error {
	meta := s.indexer.GetByID(id)
	if meta == nil {
		return nil // Already doesn't exist
	}

	// Remove from index first
	s.indexer.Remove(id)

	// Delete files
	if err := s.cache.DeleteWithMetadata(meta.GetID(), meta.ContentType); err != nil {
		return fmt.Errorf("deleting logo files: %w", err)
	}

	return nil
}

// GetStats returns statistics about cached logos.
func (s *LogoService) GetStats() *LogoStats {
	stats := s.indexer.Stats()
	return &LogoStats{
		TotalLogos:    stats.TotalLogos,
		TotalSize:     stats.TotalSize,
		CachedLogos:   stats.CachedLogos,
		UploadedLogos: stats.UploadedLogos,
		CachedSize:    stats.CachedSize,
		UploadedSize:  stats.UploadedSize,
	}
}

// GetAllLogos returns all cached logo metadata.
func (s *LogoService) GetAllLogos() []*storage.CachedLogoMetadata {
	return s.indexer.GetAll()
}

// Indexer returns the underlying logo indexer.
// Useful for direct access when needed.
func (s *LogoService) Indexer() *LogoIndexer {
	return s.indexer
}

// EnqueueLogo is a compatibility method that immediately caches the logo.
// This replaces the previous async queue-based approach with synchronous caching.
func (s *LogoService) EnqueueLogo(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error) {
	return s.CacheLogo(ctx, logoURL)
}
