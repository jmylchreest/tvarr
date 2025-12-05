// Package service provides business logic layer for tvarr operations.
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

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
// All raster images are converted to PNG format for consistent display.
type LogoService struct {
	cache              *storage.LogoCache
	indexer            *LogoIndexer
	httpClient         HTTPClient
	logger             *slog.Logger
	converter          *ImageConverter
	stalenessThreshold time.Duration // For pruning during maintenance
}

// NewLogoService creates a new LogoService.
func NewLogoService(cache *storage.LogoCache) *LogoService {
	indexer := NewLogoIndexer(cache)
	return &LogoService{
		cache:              cache,
		indexer:            indexer,
		httpClient:         http.DefaultClient,
		logger:             slog.Default(),
		converter:          NewImageConverter(),
		stalenessThreshold: 7 * 24 * time.Hour, // Default: 7 days
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

// WithStalenessThreshold sets the threshold for pruning stale logos during maintenance.
func (s *LogoService) WithStalenessThreshold(d time.Duration) *LogoService {
	s.stalenessThreshold = d
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
// Raster images (JPEG, GIF, WebP) are converted to PNG for consistent display.
// The original image is also stored alongside the converted version.
// SVG images are stored as-is since they are vector graphics.
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

	// Get content type from header
	contentType := resp.Header.Get("Content-Type")

	// Create metadata
	meta := storage.NewCachedLogoMetadata(logoURL)
	meta.OriginalContentType = contentType

	// Convert to PNG if it's a supported raster format (not SVG)
	if s.converter.IsSupportedFormat(contentType) {
		pngData, width, height, err := s.converter.ConvertToPNG(body)
		if err != nil {
			s.logger.Warn("failed to convert image to PNG, storing original only",
				"url", logoURL,
				"content_type", contentType,
				"error", err)
			// Fall back to storing original only
			meta.ContentType = contentType
			meta.OriginalContentType = "" // No separate original
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(body)); err != nil {
				return nil, fmt.Errorf("storing logo: %w", err)
			}
			return meta, nil
		}

		// Store both converted PNG and original
		meta.ContentType = "image/png"
		meta.Width = width
		meta.Height = height

		// Store with original if format differs from PNG
		if contentType != "image/png" {
			if err := s.cache.StoreWithMetadataAndOriginal(meta, bytes.NewReader(pngData), bytes.NewReader(body)); err != nil {
				return nil, fmt.Errorf("storing logo with original: %w", err)
			}
			s.logger.Debug("converted and cached logo with original",
				"url", logoURL,
				"original_type", contentType,
				"converted_size", len(pngData),
				"original_size", len(body),
				"width", width,
				"height", height)
		} else {
			// Already PNG, no need for separate original
			meta.OriginalContentType = ""
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(pngData)); err != nil {
				return nil, fmt.Errorf("storing logo: %w", err)
			}
			s.logger.Debug("cached PNG logo",
				"url", logoURL,
				"width", width,
				"height", height)
		}
	} else if s.converter.IsSVG(contentType) {
		// Store SVG as-is (vector format)
		meta.ContentType = contentType
		meta.OriginalContentType = "" // SVG is the original
		if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(body)); err != nil {
			return nil, fmt.Errorf("storing logo: %w", err)
		}
		s.logger.Debug("cached SVG logo",
			"url", logoURL)
	} else {
		// Unknown format - try to convert anyway
		pngData, width, height, err := s.converter.ConvertToPNG(body)
		if err != nil {
			// Store as-is with detected or default content type
			if contentType == "" {
				contentType = "image/png"
			}
			meta.ContentType = contentType
			meta.OriginalContentType = "" // No conversion
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(body)); err != nil {
				return nil, fmt.Errorf("storing logo: %w", err)
			}
			return meta, nil
		}

		// Conversion succeeded - store both
		meta.ContentType = "image/png"
		meta.Width = width
		meta.Height = height

		if contentType != "" && contentType != "image/png" {
			if err := s.cache.StoreWithMetadataAndOriginal(meta, bytes.NewReader(pngData), bytes.NewReader(body)); err != nil {
				return nil, fmt.Errorf("storing logo with original: %w", err)
			}
			s.logger.Debug("converted unknown format and cached with original",
				"url", logoURL,
				"original_type", contentType,
				"converted_size", len(pngData),
				"original_size", len(body))
		} else {
			meta.OriginalContentType = ""
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(pngData)); err != nil {
				return nil, fmt.Errorf("storing logo: %w", err)
			}
		}
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

// UploadLogo stores a manually uploaded logo and returns its metadata.
// Raster images (JPEG, GIF, WebP) are converted to PNG for consistent display.
// The original image is also stored alongside the converted version.
// SVG images are stored as-is since they are vector graphics.
func (s *LogoService) UploadLogo(ctx context.Context, name string, contentType string, data io.Reader) (*storage.CachedLogoMetadata, error) {
	// Create metadata for the uploaded logo (generates ULID internally)
	meta := storage.NewUploadedLogoMetadata()
	meta.OriginalContentType = contentType

	// Read data into buffer so we can process it
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, data); err != nil {
		return nil, fmt.Errorf("reading upload data: %w", err)
	}
	originalData := buf.Bytes()

	// Convert to PNG if it's a supported raster format (not SVG)
	if s.converter.IsSupportedFormat(contentType) {
		pngData, width, height, err := s.converter.ConvertToPNG(originalData)
		if err != nil {
			s.logger.Warn("failed to convert uploaded image to PNG, storing original only",
				"name", name,
				"content_type", contentType,
				"error", err)
			// Fall back to storing original only
			meta.ContentType = contentType
			meta.OriginalContentType = "" // No separate original
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(originalData)); err != nil {
				return nil, fmt.Errorf("storing uploaded logo: %w", err)
			}
		} else {
			// Store both converted PNG and original
			meta.ContentType = "image/png"
			meta.Width = width
			meta.Height = height

			if contentType != "image/png" {
				if err := s.cache.StoreWithMetadataAndOriginal(meta, bytes.NewReader(pngData), bytes.NewReader(originalData)); err != nil {
					return nil, fmt.Errorf("storing uploaded logo with original: %w", err)
				}
				s.logger.Info("uploaded logo converted to PNG with original",
					"id", meta.GetID(),
					"name", name,
					"original_type", contentType,
					"converted_size", len(pngData),
					"original_size", len(originalData),
					"width", width,
					"height", height,
				)
			} else {
				// Already PNG, no need for separate original
				meta.OriginalContentType = ""
				if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(pngData)); err != nil {
					return nil, fmt.Errorf("storing uploaded logo: %w", err)
				}
				s.logger.Info("uploaded PNG logo",
					"id", meta.GetID(),
					"name", name,
					"width", width,
					"height", height,
					"size", len(pngData),
				)
			}
		}
	} else if s.converter.IsSVG(contentType) {
		// Store SVG as-is (vector format)
		meta.ContentType = contentType
		meta.OriginalContentType = "" // SVG is the original
		if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(originalData)); err != nil {
			return nil, fmt.Errorf("storing uploaded logo: %w", err)
		}
		s.logger.Info("uploaded SVG logo",
			"id", meta.GetID(),
			"name", name,
			"size", len(originalData),
		)
	} else {
		// Unknown format - try to convert anyway
		pngData, width, height, err := s.converter.ConvertToPNG(originalData)
		if err != nil {
			// Store as-is
			meta.ContentType = contentType
			meta.OriginalContentType = "" // No conversion
			if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(originalData)); err != nil {
				return nil, fmt.Errorf("storing uploaded logo: %w", err)
			}
			s.logger.Info("uploaded logo (unknown format)",
				"id", meta.GetID(),
				"name", name,
				"content_type", contentType,
				"size", len(originalData),
			)
		} else {
			// Conversion succeeded - store both
			meta.ContentType = "image/png"
			meta.Width = width
			meta.Height = height

			if contentType != "" && contentType != "image/png" {
				if err := s.cache.StoreWithMetadataAndOriginal(meta, bytes.NewReader(pngData), bytes.NewReader(originalData)); err != nil {
					return nil, fmt.Errorf("storing uploaded logo with original: %w", err)
				}
				s.logger.Info("uploaded logo converted to PNG with original",
					"id", meta.GetID(),
					"name", name,
					"original_type", contentType,
					"converted_size", len(pngData),
					"original_size", len(originalData),
					"width", width,
					"height", height,
				)
			} else {
				meta.OriginalContentType = ""
				if err := s.cache.StoreWithMetadata(meta, bytes.NewReader(pngData)); err != nil {
					return nil, fmt.Errorf("storing uploaded logo: %w", err)
				}
				s.logger.Info("uploaded logo",
					"id", meta.GetID(),
					"name", name,
					"width", width,
					"height", height,
					"size", len(pngData),
				)
			}
		}
	}

	// Add to index
	s.indexer.Add(meta)

	return meta, nil
}

// ReplaceLogo replaces an existing logo's images with new data.
// The old linked assets are deleted and new images are stored.
func (s *LogoService) ReplaceLogo(ctx context.Context, id string, name string, contentType string, data io.Reader) (*storage.CachedLogoMetadata, error) {
	// Get existing metadata
	existingMeta := s.indexer.GetByID(id)
	if existingMeta == nil {
		return nil, fmt.Errorf("logo not found: %s", id)
	}

	// Delete all existing linked assets
	if err := s.cache.DeleteAllLinkedAssets(existingMeta); err != nil {
		s.logger.Warn("failed to delete old linked assets",
			"id", id,
			"error", err)
	}

	// Read new data
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, data); err != nil {
		return nil, fmt.Errorf("reading replacement data: %w", err)
	}
	originalData := buf.Bytes()

	// Update metadata with new content info
	existingMeta.OriginalContentType = contentType
	existingMeta.ClearLinkedAssets()

	// Convert and store new images
	if s.converter.IsSupportedFormat(contentType) {
		pngData, width, height, err := s.converter.ConvertToPNG(originalData)
		if err != nil {
			// Fall back to storing original only
			existingMeta.ContentType = contentType
			existingMeta.OriginalContentType = ""
			existingMeta.Width = 0
			existingMeta.Height = 0
			if err := s.cache.StoreWithMetadata(existingMeta, bytes.NewReader(originalData)); err != nil {
				return nil, fmt.Errorf("storing replacement logo: %w", err)
			}
		} else {
			existingMeta.ContentType = "image/png"
			existingMeta.Width = width
			existingMeta.Height = height

			if contentType != "image/png" {
				if err := s.cache.StoreWithMetadataAndOriginal(existingMeta, bytes.NewReader(pngData), bytes.NewReader(originalData)); err != nil {
					return nil, fmt.Errorf("storing replacement logo with original: %w", err)
				}
			} else {
				existingMeta.OriginalContentType = ""
				if err := s.cache.StoreWithMetadata(existingMeta, bytes.NewReader(pngData)); err != nil {
					return nil, fmt.Errorf("storing replacement logo: %w", err)
				}
			}
		}
	} else if s.converter.IsSVG(contentType) {
		existingMeta.ContentType = contentType
		existingMeta.OriginalContentType = ""
		existingMeta.Width = 0
		existingMeta.Height = 0
		if err := s.cache.StoreWithMetadata(existingMeta, bytes.NewReader(originalData)); err != nil {
			return nil, fmt.Errorf("storing replacement logo: %w", err)
		}
	} else {
		// Unknown format - try to convert
		pngData, width, height, err := s.converter.ConvertToPNG(originalData)
		if err != nil {
			existingMeta.ContentType = contentType
			existingMeta.OriginalContentType = ""
			existingMeta.Width = 0
			existingMeta.Height = 0
			if err := s.cache.StoreWithMetadata(existingMeta, bytes.NewReader(originalData)); err != nil {
				return nil, fmt.Errorf("storing replacement logo: %w", err)
			}
		} else {
			existingMeta.ContentType = "image/png"
			existingMeta.Width = width
			existingMeta.Height = height

			if contentType != "" && contentType != "image/png" {
				if err := s.cache.StoreWithMetadataAndOriginal(existingMeta, bytes.NewReader(pngData), bytes.NewReader(originalData)); err != nil {
					return nil, fmt.Errorf("storing replacement logo with original: %w", err)
				}
			} else {
				existingMeta.OriginalContentType = ""
				if err := s.cache.StoreWithMetadata(existingMeta, bytes.NewReader(pngData)); err != nil {
					return nil, fmt.Errorf("storing replacement logo: %w", err)
				}
			}
		}
	}

	s.logger.Info("replaced logo image",
		"id", id,
		"name", name,
		"content_type", existingMeta.ContentType,
		"original_type", existingMeta.OriginalContentType,
		"linked_assets", len(existingMeta.LinkedAssets),
	)

	return existingMeta, nil
}

// RunMaintenance performs logo index reload and optional pruning.
// This is called by the scheduled logo maintenance job.
// Returns the number of logos scanned and pruned.
func (s *LogoService) RunMaintenance(ctx context.Context) (scanned int, pruned int, err error) {
	s.logger.Info("starting logo maintenance",
		"staleness_threshold", s.stalenessThreshold.String())

	opts := LogoIndexerOptions{
		PruneStaleLogos:    s.stalenessThreshold > 0,
		StalenessThreshold: s.stalenessThreshold,
	}

	result, err := s.indexer.LoadFromDiskWithOptions(ctx, opts)
	if err != nil {
		return 0, 0, fmt.Errorf("loading logo index: %w", err)
	}

	s.logger.Info("logo maintenance completed",
		"total_scanned", result.TotalLoaded,
		"cached_logos", result.CachedLoaded,
		"uploaded_logos", result.UploadedLoaded,
		"pruned_count", result.PrunedCount,
		"pruned_bytes", result.PrunedSize)

	return result.TotalLoaded, result.PrunedCount, nil
}
