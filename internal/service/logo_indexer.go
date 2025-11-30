// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/storage"
)

// LogoIndexerOptions configures behavior when loading the logo index.
type LogoIndexerOptions struct {
	// PruneStaleLogos enables automatic deletion of stale cached logos during load.
	// Only affects cached logos; uploaded logos are never pruned.
	PruneStaleLogos bool

	// StalenessThreshold is how long since LastSeenAt before a cached logo is considered stale.
	// Only used if PruneStaleLogos is true.
	// Zero means no pruning (even if PruneStaleLogos is true).
	StalenessThreshold time.Duration
}

// LogoIndexerStats holds statistics about the logo index.
type LogoIndexerStats struct {
	TotalLogos    int
	TotalSize     int64
	CachedLogos   int   // URL-sourced logos (prunable)
	UploadedLogos int   // Manually uploaded logos (never pruned)
	CachedSize    int64 // Total size of cached logos
	UploadedSize  int64 // Total size of uploaded logos
}

// LogoIndexerLoadResult contains statistics from loading the index.
type LogoIndexerLoadResult struct {
	TotalLoaded   int
	CachedLoaded  int
	UploadedLoaded int
	PrunedCount   int   // Number of stale logos deleted
	PrunedSize    int64 // Total bytes freed from pruning
}

// LogoIndexer provides an in-memory index for cached logos.
// It maintains several hash maps for fast lookups by ID, URL hash, and URL.
// Thread-safe for concurrent access.
//
// For URL-sourced logos, the ID is deterministic (SHA256 of URL),
// ensuring the same URL always maps to the same cached file.
type LogoIndexer struct {
	cache  *storage.LogoCache
	logger *slog.Logger

	// mu protects all index maps
	mu sync.RWMutex

	// Primary index: ID -> metadata
	// ID is deterministic for URL-sourced logos (hash of URL)
	byID map[string]*storage.CachedLogoMetadata

	// Secondary indices for fast lookups
	byURLHash map[string]*storage.CachedLogoMetadata
	byURL     map[string]*storage.CachedLogoMetadata
}

// NewLogoIndexer creates a new LogoIndexer.
func NewLogoIndexer(cache *storage.LogoCache) *LogoIndexer {
	return &LogoIndexer{
		cache:     cache,
		logger:    slog.Default(),
		byID:      make(map[string]*storage.CachedLogoMetadata),
		byURLHash: make(map[string]*storage.CachedLogoMetadata),
		byURL:     make(map[string]*storage.CachedLogoMetadata),
	}
}

// WithLogger sets a custom logger.
func (idx *LogoIndexer) WithLogger(logger *slog.Logger) *LogoIndexer {
	idx.logger = logger
	return idx
}

// LoadFromDisk scans the logo cache directory and loads all metadata into the index.
// This should be called on startup to rebuild the in-memory index.
// For pruning stale logos during load, use LoadFromDiskWithOptions instead.
func (idx *LogoIndexer) LoadFromDisk(ctx context.Context) error {
	_, err := idx.LoadFromDiskWithOptions(ctx, LogoIndexerOptions{})
	return err
}

// LoadFromDiskWithOptions scans the logo cache directory and loads all metadata into the index.
// If options.PruneStaleLogos is true and StalenessThreshold > 0, stale cached logos
// (those with LastSeenAt older than the threshold) are deleted during load.
// Uploaded logos are never pruned regardless of settings.
func (idx *LogoIndexer) LoadFromDiskWithOptions(ctx context.Context, opts LogoIndexerOptions) (*LogoIndexerLoadResult, error) {
	idx.logger.Info("loading logo index from disk",
		"prune_enabled", opts.PruneStaleLogos,
		"staleness_threshold", opts.StalenessThreshold)

	logos, err := idx.cache.ScanLogos()
	if err != nil {
		return nil, err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Clear existing index
	idx.byID = make(map[string]*storage.CachedLogoMetadata, len(logos))
	idx.byURLHash = make(map[string]*storage.CachedLogoMetadata, len(logos))
	idx.byURL = make(map[string]*storage.CachedLogoMetadata, len(logos))

	result := &LogoIndexerLoadResult{}
	cutoff := time.Now().Add(-opts.StalenessThreshold)
	shouldPrune := opts.PruneStaleLogos && opts.StalenessThreshold > 0

	// Populate index, pruning stale cached logos if enabled
	for _, meta := range logos {
		// Check if this cached logo should be pruned
		if shouldPrune && meta.IsPrunable() && idx.isStale(meta, cutoff) {
			// Delete stale logo from disk
			if err := idx.cache.DeleteWithMetadata(meta.GetID(), meta.ContentType); err != nil {
				idx.logger.Warn("failed to prune stale logo",
					"id", meta.GetID(),
					"last_seen", meta.LastSeenAt,
					"error", err)
			} else {
				result.PrunedCount++
				result.PrunedSize += meta.FileSize
				idx.logger.Debug("pruned stale logo",
					"id", meta.GetID(),
					"last_seen", meta.LastSeenAt,
					"file_size", meta.FileSize)
			}
			continue // Don't add to index
		}

		// Add to index
		idx.byID[meta.GetID()] = meta
		if meta.URLHash != "" {
			idx.byURLHash[meta.URLHash] = meta
		}
		if meta.OriginalURL != "" {
			idx.byURL[meta.OriginalURL] = meta
		}

		// Track stats by source
		result.TotalLoaded++
		if meta.IsPrunable() {
			result.CachedLoaded++
		} else {
			result.UploadedLoaded++
		}
	}

	idx.logger.Info("loaded logo index",
		"total_logos", result.TotalLoaded,
		"cached_logos", result.CachedLoaded,
		"uploaded_logos", result.UploadedLoaded,
		"pruned_count", result.PrunedCount,
		"pruned_bytes", result.PrunedSize)

	return result, nil
}

// isStale checks if a logo's LastSeenAt is before the cutoff time.
// Handles backwards compatibility for logos without LastSeenAt set.
func (idx *LogoIndexer) isStale(meta *storage.CachedLogoMetadata, cutoff time.Time) bool {
	if !meta.LastSeenAt.IsZero() {
		return meta.LastSeenAt.Before(cutoff)
	}
	// For backwards compatibility: if LastSeenAt not set, use CreatedAt
	if !meta.CreatedAt.IsZero() {
		return meta.CreatedAt.Before(cutoff)
	}
	// No timestamp at all - don't prune (be conservative)
	return false
}

// Add adds a logo to the index.
func (idx *LogoIndexer) Add(meta *storage.CachedLogoMetadata) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.byID[meta.GetID()] = meta
	if meta.URLHash != "" {
		idx.byURLHash[meta.URLHash] = meta
	}
	if meta.OriginalURL != "" {
		idx.byURL[meta.OriginalURL] = meta
	}
}

// AddURLMapping adds an additional URL mapping to an existing logo.
// This handles cases where the same logo is referenced by different URL variants
// (e.g., http vs https, different query params) that normalize to the same hash.
func (idx *LogoIndexer) AddURLMapping(url string, meta *storage.CachedLogoMetadata) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.byURL[url] = meta
}

// Remove removes a logo from the index by its ID.
func (idx *LogoIndexer) Remove(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	meta, exists := idx.byID[id]
	if !exists {
		return
	}

	delete(idx.byID, id)
	if meta.URLHash != "" {
		delete(idx.byURLHash, meta.URLHash)
	}
	if meta.OriginalURL != "" {
		delete(idx.byURL, meta.OriginalURL)
	}
}

// GetByURL returns the logo metadata for a given URL.
// Returns nil if not found.
func (idx *LogoIndexer) GetByURL(url string) *storage.CachedLogoMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.byURL[url]
}

// GetByURLHash returns the logo metadata for a given URL hash.
// Returns nil if not found.
func (idx *LogoIndexer) GetByURLHash(hash string) *storage.CachedLogoMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.byURLHash[hash]
}

// GetByID returns the logo metadata for a given ID.
// Returns nil if not found.
func (idx *LogoIndexer) GetByID(id string) *storage.CachedLogoMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.byID[id]
}

// GetByULID returns the logo metadata for a given ID (alias for GetByID).
// Deprecated: Use GetByID instead.
func (idx *LogoIndexer) GetByULID(id string) *storage.CachedLogoMetadata {
	return idx.GetByID(id)
}

// Contains checks if a logo URL is already in the index.
func (idx *LogoIndexer) Contains(url string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	_, exists := idx.byURL[url]
	return exists
}

// GetAll returns all indexed logo metadata.
// Returns a copy to avoid mutation issues.
func (idx *LogoIndexer) GetAll() []*storage.CachedLogoMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]*storage.CachedLogoMetadata, 0, len(idx.byID))
	for _, meta := range idx.byID {
		result = append(result, meta)
	}
	return result
}

// Stats returns statistics about the logo index.
func (idx *LogoIndexer) Stats() LogoIndexerStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	stats := LogoIndexerStats{}
	for _, meta := range idx.byID {
		stats.TotalLogos++
		stats.TotalSize += meta.FileSize

		if meta.IsPrunable() {
			stats.CachedLogos++
			stats.CachedSize += meta.FileSize
		} else {
			stats.UploadedLogos++
			stats.UploadedSize += meta.FileSize
		}
	}

	return stats
}

// Clear removes all entries from the index.
// This does not delete files from disk.
func (idx *LogoIndexer) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.byID = make(map[string]*storage.CachedLogoMetadata)
	idx.byURLHash = make(map[string]*storage.CachedLogoMetadata)
	idx.byURL = make(map[string]*storage.CachedLogoMetadata)
}

// Count returns the number of logos in the index.
func (idx *LogoIndexer) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.byID)
}
