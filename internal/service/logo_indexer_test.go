package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestLogoIndexer(t *testing.T) (*LogoIndexer, *storage.LogoCache) {
	t.Helper()
	tempDir := t.TempDir()
	cache, err := storage.NewLogoCache(tempDir)
	require.NoError(t, err)

	indexer := NewLogoIndexer(cache)
	return indexer, cache
}

func TestLogoIndexer_New(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)
	require.NotNil(t, indexer)
}

func TestLogoIndexer_LoadFromDisk(t *testing.T) {
	indexer, cache := setupTestLogoIndexer(t)
	ctx := context.Background()

	// Store some logos on disk
	urls := []string{
		"https://example.com/logo1.png",
		"https://example.com/logo2.png",
		"https://example.com/logo3.png",
	}

	for _, url := range urls {
		meta := storage.NewCachedLogoMetadata(url)
		meta.ContentType = "image/png"
		err := cache.StoreWithMetadata(meta, bytes.NewReader([]byte("image data")))
		require.NoError(t, err)
	}

	// Load into index
	err := indexer.LoadFromDisk(ctx)
	require.NoError(t, err)

	// Verify all URLs are indexed
	stats := indexer.Stats()
	assert.Equal(t, 3, stats.TotalLogos)

	// Verify lookups work
	for _, url := range urls {
		meta := indexer.GetByURL(url)
		require.NotNil(t, meta, "URL %s should be in index", url)
		assert.Equal(t, url, meta.OriginalURL)
	}
}

func TestLogoIndexer_GetByURL(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Test non-existent URL
	meta := indexer.GetByURL("https://notfound.com/logo.png")
	assert.Nil(t, meta)

	// Add a logo to the index
	testMeta := storage.NewCachedLogoMetadata("https://example.com/test.png")
	testMeta.ContentType = "image/png"
	indexer.Add(testMeta)

	// Now it should be found
	found := indexer.GetByURL("https://example.com/test.png")
	require.NotNil(t, found)
	assert.Equal(t, testMeta.ID, found.ID)
}

func TestLogoIndexer_GetByURLHash(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add a logo
	testMeta := storage.NewCachedLogoMetadata("https://example.com/test.png")
	testMeta.ContentType = "image/png"
	indexer.Add(testMeta)

	// Lookup by hash
	found := indexer.GetByURLHash(testMeta.URLHash)
	require.NotNil(t, found)
	assert.Equal(t, testMeta.ID, found.ID)

	// Non-existent hash
	notFound := indexer.GetByURLHash("nonexistenthash")
	assert.Nil(t, notFound)
}

func TestLogoIndexer_GetByID(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add a logo
	testMeta := storage.NewCachedLogoMetadata("https://example.com/test.png")
	testMeta.ContentType = "image/png"
	indexer.Add(testMeta)

	// Lookup by ID
	found := indexer.GetByID(testMeta.ID)
	require.NotNil(t, found)
	assert.Equal(t, testMeta.OriginalURL, found.OriginalURL)

	// Non-existent ID
	notFound := indexer.GetByID("nonexistent")
	assert.Nil(t, notFound)
}

func TestLogoIndexer_Add(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	testMeta := storage.NewCachedLogoMetadata("https://example.com/add-test.png")
	testMeta.ContentType = "image/png"

	// Add to index
	indexer.Add(testMeta)

	// Verify it's indexed by all keys
	assert.NotNil(t, indexer.GetByURL(testMeta.OriginalURL))
	assert.NotNil(t, indexer.GetByURLHash(testMeta.URLHash))
	assert.NotNil(t, indexer.GetByID(testMeta.ID))

	stats := indexer.Stats()
	assert.Equal(t, 1, stats.TotalLogos)
}

func TestLogoIndexer_Remove(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add a logo
	testMeta := storage.NewCachedLogoMetadata("https://example.com/remove-test.png")
	testMeta.ContentType = "image/png"
	indexer.Add(testMeta)

	// Verify it's indexed
	assert.NotNil(t, indexer.GetByURL(testMeta.OriginalURL))

	// Remove it
	indexer.Remove(testMeta.ID)

	// Verify it's gone from all indices
	assert.Nil(t, indexer.GetByURL(testMeta.OriginalURL))
	assert.Nil(t, indexer.GetByURLHash(testMeta.URLHash))
	assert.Nil(t, indexer.GetByID(testMeta.ID))

	stats := indexer.Stats()
	assert.Equal(t, 0, stats.TotalLogos)
}

func TestLogoIndexer_Contains(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	url := "https://example.com/contains-test.png"

	// Initially not present
	assert.False(t, indexer.Contains(url))

	// Add it
	testMeta := storage.NewCachedLogoMetadata(url)
	testMeta.ContentType = "image/png"
	indexer.Add(testMeta)

	// Now it's present
	assert.True(t, indexer.Contains(url))
}

func TestLogoIndexer_Stats(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Initially empty
	stats := indexer.Stats()
	assert.Equal(t, 0, stats.TotalLogos)
	assert.Equal(t, int64(0), stats.TotalSize)

	// Add some logos with different sizes
	for i := range 5 {
		meta := storage.NewCachedLogoMetadata("https://example.com/logo" + string(rune('a'+i)) + ".png")
		meta.ContentType = "image/png"
		meta.FileSize = int64(1000 * (i + 1))
		indexer.Add(meta)
	}

	// Check stats
	stats = indexer.Stats()
	assert.Equal(t, 5, stats.TotalLogos)
	// 1000+2000+3000+4000+5000 = 15000
	assert.Equal(t, int64(15000), stats.TotalSize)
}

func TestLogoIndexer_ConcurrentAccess(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)
	ctx := context.Background()

	// Concurrent adds
	done := make(chan bool)
	for i := range 100 {
		go func(i int) {
			meta := storage.NewCachedLogoMetadata("https://example.com/concurrent-" + string(rune(i)) + ".png")
			meta.ContentType = "image/png"
			indexer.Add(meta)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 100 {
		<-done
	}

	// Verify index is consistent
	stats := indexer.Stats()
	assert.Equal(t, 100, stats.TotalLogos)

	// Test concurrent reads
	for range 100 {
		go func() {
			_ = indexer.Stats()
		}()
	}

	// Reload from disk (should not panic with concurrent access)
	_ = indexer.LoadFromDisk(ctx)
}

func TestLogoIndexer_GetAll(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add some logos
	urls := []string{
		"https://example.com/all1.png",
		"https://example.com/all2.png",
		"https://example.com/all3.png",
	}

	for _, url := range urls {
		meta := storage.NewCachedLogoMetadata(url)
		meta.ContentType = "image/png"
		indexer.Add(meta)
	}

	// Get all
	all := indexer.GetAll()
	assert.Len(t, all, 3)

	// Verify all URLs present
	foundURLs := make(map[string]bool)
	for _, meta := range all {
		foundURLs[meta.OriginalURL] = true
	}
	for _, url := range urls {
		assert.True(t, foundURLs[url])
	}
}

func TestLogoIndexer_Clear(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add some logos
	for i := range 5 {
		meta := storage.NewCachedLogoMetadata("https://example.com/clear-" + string(rune('a'+i)) + ".png")
		meta.ContentType = "image/png"
		indexer.Add(meta)
	}

	// Verify they're indexed
	stats := indexer.Stats()
	assert.Equal(t, 5, stats.TotalLogos)

	// Clear the index
	indexer.Clear()

	// Verify it's empty
	stats = indexer.Stats()
	assert.Equal(t, 0, stats.TotalLogos)
}

func TestLogoIndexer_LoadFromDiskWithOptions_NoPrune(t *testing.T) {
	indexer, cache := setupTestLogoIndexer(t)
	ctx := context.Background()

	// Store some logos on disk
	urls := []string{
		"https://example.com/logo1.png",
		"https://example.com/logo2.png",
	}

	for _, url := range urls {
		meta := storage.NewCachedLogoMetadata(url)
		meta.ContentType = "image/png"
		err := cache.StoreWithMetadata(meta, bytes.NewReader([]byte("image data")))
		require.NoError(t, err)
	}

	// Load without pruning
	result, err := indexer.LoadFromDiskWithOptions(ctx, LogoIndexerOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, result.TotalLoaded)
	assert.Equal(t, 2, result.CachedLoaded)
	assert.Equal(t, 0, result.UploadedLoaded)
	assert.Equal(t, 0, result.PrunedCount)
}

func TestLogoIndexer_LoadFromDiskWithOptions_PruneStaleLogos(t *testing.T) {
	indexer, cache := setupTestLogoIndexer(t)
	ctx := context.Background()

	// Create a recent logo (should not be pruned)
	recentMeta := storage.NewCachedLogoMetadata("https://example.com/recent.png")
	recentMeta.ContentType = "image/png"
	err := cache.StoreWithMetadata(recentMeta, bytes.NewReader([]byte("recent image")))
	require.NoError(t, err)

	// Create a stale logo (should be pruned)
	staleMeta := storage.NewCachedLogoMetadata("https://example.com/stale.png")
	staleMeta.ContentType = "image/png"
	// Set LastSeenAt to 60 days ago
	staleMeta.LastSeenAt = time.Now().Add(-60 * 24 * time.Hour)
	staleMeta.CreatedAt = staleMeta.LastSeenAt
	staleData := []byte("stale image data")
	err = cache.StoreWithMetadata(staleMeta, bytes.NewReader(staleData))
	require.NoError(t, err)

	// Load with pruning (30 day threshold)
	result, err := indexer.LoadFromDiskWithOptions(ctx, LogoIndexerOptions{
		PruneStaleLogos:    true,
		StalenessThreshold: 30 * 24 * time.Hour, // 30 days
	})
	require.NoError(t, err)

	// Recent logo should be loaded, stale logo should be pruned
	assert.Equal(t, 1, result.TotalLoaded)
	assert.Equal(t, 1, result.CachedLoaded)
	assert.Equal(t, 1, result.PrunedCount)
	assert.Equal(t, int64(len(staleData)), result.PrunedSize) // Actual stored size

	// Verify only recent logo is in index
	assert.NotNil(t, indexer.GetByURL("https://example.com/recent.png"))
	assert.Nil(t, indexer.GetByURL("https://example.com/stale.png"))
}

func TestLogoIndexer_LoadFromDiskWithOptions_UploadedLogosNeverPruned(t *testing.T) {
	indexer, cache := setupTestLogoIndexer(t)
	ctx := context.Background()

	// Create a stale cached logo (should be pruned)
	cachedMeta := storage.NewCachedLogoMetadata("https://example.com/stale-cached.png")
	cachedMeta.ContentType = "image/png"
	cachedMeta.LastSeenAt = time.Now().Add(-60 * 24 * time.Hour)
	cachedMeta.CreatedAt = cachedMeta.LastSeenAt
	err := cache.StoreWithMetadata(cachedMeta, bytes.NewReader([]byte("cached image")))
	require.NoError(t, err)

	// Create an old uploaded logo (should NOT be pruned)
	uploadedMeta := storage.NewUploadedLogoMetadata()
	uploadedMeta.ContentType = "image/png"
	uploadedMeta.CreatedAt = time.Now().Add(-60 * 24 * time.Hour) // Very old
	err = cache.StoreWithMetadata(uploadedMeta, bytes.NewReader([]byte("uploaded image")))
	require.NoError(t, err)

	// Load with aggressive pruning (7 day threshold)
	result, err := indexer.LoadFromDiskWithOptions(ctx, LogoIndexerOptions{
		PruneStaleLogos:    true,
		StalenessThreshold: 7 * 24 * time.Hour, // 7 days
	})
	require.NoError(t, err)

	// Cached logo should be pruned, uploaded should remain
	assert.Equal(t, 1, result.TotalLoaded)
	assert.Equal(t, 0, result.CachedLoaded)
	assert.Equal(t, 1, result.UploadedLoaded)
	assert.Equal(t, 1, result.PrunedCount)

	// Verify uploaded logo is still in index
	assert.NotNil(t, indexer.GetByID(uploadedMeta.GetID()))
}

func TestLogoIndexer_Stats_BySource(t *testing.T) {
	indexer, _ := setupTestLogoIndexer(t)

	// Add cached logos
	for i := range 3 {
		meta := storage.NewCachedLogoMetadata("https://example.com/cached-" + string(rune('a'+i)) + ".png")
		meta.ContentType = "image/png"
		meta.FileSize = 1000
		indexer.Add(meta)
	}

	// Add uploaded logos
	for range 2 {
		meta := storage.NewUploadedLogoMetadata()
		meta.ContentType = "image/png"
		meta.FileSize = 2000
		indexer.Add(meta)
	}

	stats := indexer.Stats()
	assert.Equal(t, 5, stats.TotalLogos)
	assert.Equal(t, int64(7000), stats.TotalSize) // 3*1000 + 2*2000
	assert.Equal(t, 3, stats.CachedLogos)
	assert.Equal(t, 2, stats.UploadedLogos)
	assert.Equal(t, int64(3000), stats.CachedSize)
	assert.Equal(t, int64(4000), stats.UploadedSize)
}
