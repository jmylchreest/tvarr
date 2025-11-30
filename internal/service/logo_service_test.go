package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHTTPClient implements HTTPClient for testing.
type mockHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func newMockHTTPClient() *mockHTTPClient {
	return &mockHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	if err, ok := m.errors[url]; ok {
		return nil, err
	}
	if resp, ok := m.responses[url]; ok {
		return resp, nil
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

func (m *mockHTTPClient) withResponse(url string, contentType string, body []byte) *mockHTTPClient {
	m.responses[url] = &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	return m
}

func setupTestLogoService(t *testing.T) (*LogoService, *storage.LogoCache) {
	t.Helper()
	tempDir := t.TempDir()
	cache, err := storage.NewLogoCache(tempDir)
	require.NoError(t, err)

	svc := NewLogoService(cache)
	return svc, cache
}

func TestLogoService_New(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	require.NotNil(t, svc)
}

func TestLogoService_CacheLogo(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", []byte("PNG image data"))
	svc = svc.WithHTTPClient(mockClient)

	// Cache a logo
	meta, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, "https://example.com/logo.png", meta.OriginalURL)
	assert.Equal(t, "image/png", meta.ContentType)
	assert.NotEmpty(t, meta.ULID)
	assert.True(t, meta.FileSize > 0)
}

func TestLogoService_CacheLogo_AlreadyCached(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", []byte("PNG image data"))
	svc = svc.WithHTTPClient(mockClient)

	// Cache a logo
	meta1, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)

	// Cache same logo again - should return existing
	meta2, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)

	// Should be same ULID
	assert.Equal(t, meta1.ULID, meta2.ULID)
}

func TestLogoService_CacheLogo_HTTPError(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	// No mock response = 404
	mockClient := newMockHTTPClient()
	svc = svc.WithHTTPClient(mockClient)

	_, err := svc.CacheLogo(ctx, "https://example.com/notfound.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestLogoService_GetCachedLogo(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	// Initially not cached
	meta := svc.GetCachedLogo("https://example.com/logo.png")
	assert.Nil(t, meta)

	// Cache it
	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", []byte("PNG data"))
	svc = svc.WithHTTPClient(mockClient)

	_, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)

	// Now should be cached
	meta = svc.GetCachedLogo("https://example.com/logo.png")
	require.NotNil(t, meta)
	assert.Equal(t, "https://example.com/logo.png", meta.OriginalURL)
}

func TestLogoService_Contains(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	url := "https://example.com/contains.png"

	// Initially not present
	assert.False(t, svc.Contains(url))

	// Cache it
	mockClient := newMockHTTPClient().
		withResponse(url, "image/png", []byte("PNG data"))
	svc = svc.WithHTTPClient(mockClient)

	_, err := svc.CacheLogo(ctx, url)
	require.NoError(t, err)

	// Now it's present
	assert.True(t, svc.Contains(url))
}

func TestLogoService_GetLogoFile(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	imageData := []byte("test PNG image content")
	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", imageData)
	svc = svc.WithHTTPClient(mockClient)

	meta, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)

	// Get the file
	file, err := svc.GetLogoFile(meta)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, imageData, data)
}

func TestLogoService_DeleteLogo(t *testing.T) {
	svc, cache := setupTestLogoService(t)
	ctx := context.Background()

	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", []byte("PNG data"))
	svc = svc.WithHTTPClient(mockClient)

	meta, err := svc.CacheLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)

	// Verify it's cached
	assert.True(t, svc.Contains("https://example.com/logo.png"))

	// Delete it
	err = svc.DeleteLogo(meta.ULID)
	require.NoError(t, err)

	// Verify it's gone from index
	assert.False(t, svc.Contains("https://example.com/logo.png"))

	// Verify file is gone from disk
	exists, _ := cache.Exists(meta.RelativeImagePath())
	assert.False(t, exists)
}

func TestLogoService_GetStats(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	// Initially empty
	stats := svc.GetStats()
	assert.Equal(t, 0, stats.TotalLogos)
	assert.Equal(t, int64(0), stats.TotalSize)

	// Add some logos
	for i, url := range []string{
		"https://example.com/logo1.png",
		"https://example.com/logo2.png",
		"https://example.com/logo3.png",
	} {
		mockClient := newMockHTTPClient().
			withResponse(url, "image/png", bytes.Repeat([]byte{byte(i)}, 1000*(i+1)))
		svc = svc.WithHTTPClient(mockClient)
		_, err := svc.CacheLogo(ctx, url)
		require.NoError(t, err)
	}

	stats = svc.GetStats()
	assert.Equal(t, 3, stats.TotalLogos)
	// 1000 + 2000 + 3000 = 6000
	assert.Equal(t, int64(6000), stats.TotalSize)
}

func TestLogoService_LoadIndex(t *testing.T) {
	tempDir := t.TempDir()
	cache, err := storage.NewLogoCache(tempDir)
	require.NoError(t, err)

	ctx := context.Background()

	// First, store some logos directly
	urls := []string{
		"https://example.com/persist1.png",
		"https://example.com/persist2.png",
	}

	for _, url := range urls {
		meta := storage.NewCachedLogoMetadata(url)
		meta.ContentType = "image/png"
		err := cache.StoreWithMetadata(meta, bytes.NewReader([]byte("image data")))
		require.NoError(t, err)
	}

	// Create a new service instance (simulating restart)
	svc := NewLogoService(cache)

	// Initially index is empty (not loaded)
	assert.Equal(t, 0, svc.GetStats().TotalLogos)

	// Load index from disk
	err = svc.LoadIndex(ctx)
	require.NoError(t, err)

	// Now logos should be indexed
	assert.Equal(t, 2, svc.GetStats().TotalLogos)

	for _, url := range urls {
		assert.True(t, svc.Contains(url))
	}
}

func TestLogoService_EnqueueLogo_Compatibility(t *testing.T) {
	svc, _ := setupTestLogoService(t)
	ctx := context.Background()

	mockClient := newMockHTTPClient().
		withResponse("https://example.com/logo.png", "image/png", []byte("PNG data"))
	svc = svc.WithHTTPClient(mockClient)

	// EnqueueLogo should work same as CacheLogo
	meta, err := svc.EnqueueLogo(ctx, "https://example.com/logo.png")
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, "https://example.com/logo.png", meta.OriginalURL)
}
