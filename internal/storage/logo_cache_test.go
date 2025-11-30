package storage

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogoCache_New(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Check that logos directory was created
	logosDir := filepath.Join(tempDir, "logos")
	info, err := os.Stat(logosDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLogoCache_GeneratePath(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	tests := []struct {
		name        string
		urlHash     string
		contentType string
		wantExt     string
	}{
		{"PNG image", "abc123", "image/png", ".png"},
		{"JPEG image", "def456", "image/jpeg", ".jpg"},
		{"GIF image", "ghi789", "image/gif", ".gif"},
		{"WebP image", "jkl012", "image/webp", ".webp"},
		{"SVG image", "mno345", "image/svg+xml", ".svg"},
		{"Unknown type", "pqr678", "application/octet-stream", ""},
		{"Empty type", "stu901", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := cache.GeneratePath(tt.urlHash, tt.contentType)
			assert.Contains(t, path, tt.urlHash)
			if tt.wantExt != "" {
				assert.True(t, filepath.Ext(path) == tt.wantExt)
			}
		})
	}
}

func TestLogoCache_Store(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("fake image data")
	reader := bytes.NewReader(testData)

	path, size, err := cache.Store("test-hash", "image/png", reader)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Equal(t, int64(len(testData)), size)

	// Verify file exists and contains correct data
	fullPath, err := cache.sandbox.ResolvePath(path)
	require.NoError(t, err)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestLogoCache_Get(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("test logo content")
	reader := bytes.NewReader(testData)

	path, _, err := cache.Store("get-test", "image/png", reader)
	require.NoError(t, err)

	// Get the file
	file, err := cache.Get(path)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestLogoCache_GetNotFound(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	_, err = cache.Get("logos/nonexistent.png")
	assert.Error(t, err)
}

func TestLogoCache_Exists(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	path, _, err := cache.Store("exists-test", "image/png", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	exists, err := cache.Exists(path)
	require.NoError(t, err)
	assert.True(t, exists)

	notExists, err := cache.Exists("logos/nonexistent.png")
	require.NoError(t, err)
	assert.False(t, notExists)
}

func TestLogoCache_Delete(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	path, _, err := cache.Store("delete-test", "image/png", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	// Verify it exists
	exists, _ := cache.Exists(path)
	assert.True(t, exists)

	// Delete it
	err = cache.Delete(path)
	require.NoError(t, err)

	// Verify it's gone
	exists, _ = cache.Exists(path)
	assert.False(t, exists)
}

func TestLogoCache_Size(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("test data with known size")
	path, _, err := cache.Store("size-test", "image/png", bytes.NewReader(testData))
	require.NoError(t, err)

	size, err := cache.Size(path)
	require.NoError(t, err)
	assert.Equal(t, int64(len(testData)), size)
}

func TestLogoCache_HashSharding(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	// Store files with different hashes
	hashes := []string{"abcd1234", "efgh5678", "ijkl9012"}
	for _, hash := range hashes {
		_, _, err := cache.Store(hash, "image/png", bytes.NewReader([]byte("data")))
		require.NoError(t, err)
	}

	// Verify sharding structure (first 2 chars of hash as subdirectory)
	for _, hash := range hashes {
		path := cache.GeneratePath(hash, "image/png")
		// Path should be logos/{shard}/{hash}.ext
		shard := hash[:2]
		expectedPrefix := filepath.Join("logos", shard)
		assert.True(t, strings.HasPrefix(path, expectedPrefix), "path %s should have prefix %s", path, expectedPrefix)
	}
}

func TestLogoCache_CleanupEmpty(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	// Store and then delete a file
	path, _, err := cache.Store("cleanup-test", "image/png", bytes.NewReader([]byte("data")))
	require.NoError(t, err)

	err = cache.Delete(path)
	require.NoError(t, err)

	// Cleanup empty directories
	err = cache.CleanupEmptyDirs()
	require.NoError(t, err)
}

func TestLogoCache_StoreWithMetadata(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("fake PNG image data")
	meta := NewCachedLogoMetadata("https://example.com/test-logo.png")
	meta.ContentType = "image/png"
	meta.SourceHint = "channel:Test Channel"

	err = cache.StoreWithMetadata(meta, bytes.NewReader(testData))
	require.NoError(t, err)

	// Verify image file exists
	imgExists, err := cache.Exists(meta.RelativeImagePath())
	require.NoError(t, err)
	assert.True(t, imgExists)

	// Verify metadata file exists
	metaExists, err := cache.Exists(meta.RelativeMetadataPath())
	require.NoError(t, err)
	assert.True(t, metaExists)

	// Verify metadata can be read back
	loadedMeta, err := cache.LoadMetadata(meta.ID)
	require.NoError(t, err)
	assert.Equal(t, meta.ID, loadedMeta.ID)
	assert.Equal(t, meta.OriginalURL, loadedMeta.OriginalURL)
	assert.Equal(t, meta.URLHash, loadedMeta.URLHash)
	assert.Equal(t, meta.ContentType, loadedMeta.ContentType)
	assert.Equal(t, meta.SourceHint, loadedMeta.SourceHint)
	assert.Equal(t, int64(len(testData)), loadedMeta.FileSize)
}

func TestLogoCache_DeleteWithMetadata(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("image to delete")
	meta := NewCachedLogoMetadata("https://example.com/delete-test.png")
	meta.ContentType = "image/png"

	err = cache.StoreWithMetadata(meta, bytes.NewReader(testData))
	require.NoError(t, err)

	// Verify both files exist
	imgExists, _ := cache.Exists(meta.RelativeImagePath())
	metaExists, _ := cache.Exists(meta.RelativeMetadataPath())
	assert.True(t, imgExists)
	assert.True(t, metaExists)

	// Delete with metadata
	err = cache.DeleteWithMetadata(meta.ID, meta.ContentType)
	require.NoError(t, err)

	// Verify both files are gone
	imgExists, _ = cache.Exists(meta.RelativeImagePath())
	metaExists, _ = cache.Exists(meta.RelativeMetadataPath())
	assert.False(t, imgExists)
	assert.False(t, metaExists)
}

func TestLogoCache_ScanLogos(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	// Store multiple logos
	urls := []string{
		"https://example.com/logo1.png",
		"https://example.com/logo2.png",
		"https://example.com/logo3.png",
	}

	for _, url := range urls {
		meta := NewCachedLogoMetadata(url)
		meta.ContentType = "image/png"
		err = cache.StoreWithMetadata(meta, bytes.NewReader([]byte("image data")))
		require.NoError(t, err)
	}

	// Scan all logos
	logos, err := cache.ScanLogos()
	require.NoError(t, err)
	assert.Len(t, logos, 3)

	// Verify all URLs are present
	foundURLs := make(map[string]bool)
	for _, meta := range logos {
		foundURLs[meta.OriginalURL] = true
	}
	for _, url := range urls {
		assert.True(t, foundURLs[url], "URL %s should be found in scan", url)
	}
}

func TestLogoCache_GetImagePath(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := NewLogoCache(tempDir)
	require.NoError(t, err)

	testData := []byte("image data")
	meta := NewCachedLogoMetadata("https://example.com/path-test.png")
	meta.ContentType = "image/png"

	err = cache.StoreWithMetadata(meta, bytes.NewReader(testData))
	require.NoError(t, err)

	// Get absolute path for the image
	absPath, err := cache.GetImageAbsolutePath(meta.ID, meta.ContentType)
	require.NoError(t, err)
	assert.NotEmpty(t, absPath)

	// Verify the file exists at that path
	_, err = os.Stat(absPath)
	require.NoError(t, err)
}
