package storage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedLogoMetadata_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	meta := &CachedLogoMetadata{
		ID:          "abc123def456",
		Source:      LogoSourceCached,
		OriginalURL: "https://example.com/logo.png",
		URLHash:     "abc123def456",
		ContentType: "image/png",
		FileSize:    1024,
		Width:       100,
		Height:      100,
		CreatedAt:   now,
		LastSeenAt:  now,
		SourceHint:  "channel:Test Channel",
	}

	// Marshal to JSON
	data, err := json.Marshal(meta)
	require.NoError(t, err)

	// Unmarshal back
	var decoded CachedLogoMetadata
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, meta.ID, decoded.ID)
	assert.Equal(t, meta.Source, decoded.Source)
	assert.Equal(t, meta.OriginalURL, decoded.OriginalURL)
	assert.Equal(t, meta.URLHash, decoded.URLHash)
	assert.Equal(t, meta.ContentType, decoded.ContentType)
	assert.Equal(t, meta.FileSize, decoded.FileSize)
	assert.Equal(t, meta.Width, decoded.Width)
	assert.Equal(t, meta.Height, decoded.Height)
	assert.Equal(t, meta.SourceHint, decoded.SourceHint)
	// Times may differ slightly due to precision, just ensure it's set
	assert.False(t, decoded.CreatedAt.IsZero())
	assert.False(t, decoded.LastSeenAt.IsZero())
}

func TestCachedLogoMetadata_ImagePath(t *testing.T) {
	meta := &CachedLogoMetadata{
		ID: "abc123def456789",
	}

	// PNG
	meta.ContentType = "image/png"
	assert.Equal(t, "abc123def456789.png", meta.ImagePath())

	// JPEG
	meta.ContentType = "image/jpeg"
	assert.Equal(t, "abc123def456789.jpg", meta.ImagePath())

	// Default to .png if unknown
	meta.ContentType = "application/octet-stream"
	assert.Equal(t, "abc123def456789.png", meta.ImagePath())
}

func TestCachedLogoMetadata_MetadataPath(t *testing.T) {
	meta := &CachedLogoMetadata{
		ID: "abc123def456789",
	}

	assert.Equal(t, "abc123def456789.json", meta.MetadataPath())
}

func TestCachedLogoMetadata_SourceDir(t *testing.T) {
	// Cached logo
	meta := &CachedLogoMetadata{
		ID:          "abc123",
		Source:      LogoSourceCached,
		OriginalURL: "https://example.com/logo.png",
	}
	assert.Equal(t, "cached", meta.SourceDir())

	// Uploaded logo
	meta2 := &CachedLogoMetadata{
		ID:     "01HXYZ",
		Source: LogoSourceUploaded,
	}
	assert.Equal(t, "uploaded", meta2.SourceDir())
}

func TestCachedLogoMetadata_RelativeImagePath(t *testing.T) {
	// Cached logo
	meta := &CachedLogoMetadata{
		ID:          "abc123def456789",
		Source:      LogoSourceCached,
		ContentType: "image/png",
	}
	assert.Equal(t, "logos/cached/abc123def456789.png", meta.RelativeImagePath())

	// Uploaded logo
	meta2 := &CachedLogoMetadata{
		ID:          "01HXYZ123456",
		Source:      LogoSourceUploaded,
		ContentType: "image/jpeg",
	}
	assert.Equal(t, "logos/uploaded/01HXYZ123456.jpg", meta2.RelativeImagePath())
}

func TestCachedLogoMetadata_RelativeMetadataPath(t *testing.T) {
	// Cached logo
	meta := &CachedLogoMetadata{
		ID:     "abc123def456789",
		Source: LogoSourceCached,
	}
	assert.Equal(t, "logos/cached/abc123def456789.json", meta.RelativeMetadataPath())

	// Uploaded logo
	meta2 := &CachedLogoMetadata{
		ID:     "01HXYZ123456",
		Source: LogoSourceUploaded,
	}
	assert.Equal(t, "logos/uploaded/01HXYZ123456.json", meta2.RelativeMetadataPath())
}

func TestNewCachedLogoMetadata(t *testing.T) {
	meta := NewCachedLogoMetadata("https://example.com/test.png")

	assert.NotEmpty(t, meta.GetID())
	assert.Equal(t, "https://example.com/test.png", meta.OriginalURL)
	assert.NotEmpty(t, meta.URLHash)
	assert.NotEmpty(t, meta.NormalizedURL)
	assert.Equal(t, LogoSourceCached, meta.Source)
	assert.False(t, meta.CreatedAt.IsZero())
	assert.False(t, meta.LastSeenAt.IsZero())
}

func TestNewCachedLogoMetadata_DeterministicID(t *testing.T) {
	// Same URL should produce same ID
	url := "https://example.com/channel/logo.png"

	meta1 := NewCachedLogoMetadata(url)
	meta2 := NewCachedLogoMetadata(url)

	// IDs should be identical for same URL
	assert.Equal(t, meta1.GetID(), meta2.GetID())
	assert.Equal(t, meta1.URLHash, meta2.URLHash)

	// ID should equal URLHash for URL-sourced logos
	assert.Equal(t, meta1.URLHash, meta1.GetID())
}

func TestNewCachedLogoMetadata_URLNormalization(t *testing.T) {
	tests := []struct {
		name string
		url1 string
		url2 string
	}{
		{
			name: "http vs https produces same ID",
			url1: "http://example.com/logo.png",
			url2: "https://example.com/logo.png",
		},
		{
			name: "query param ordering produces same ID",
			url1: "https://example.com/logo.png?a=1&b=2",
			url2: "https://example.com/logo.png?b=2&a=1",
		},
		{
			name: "trailing slash normalization",
			url1: "https://example.com/logos/",
			url2: "https://example.com/logos",
		},
		{
			name: "case insensitive hostname",
			url1: "https://EXAMPLE.COM/logo.png",
			url2: "https://example.com/logo.png",
		},
		{
			name: "default port removal",
			url1: "https://example.com:443/logo.png",
			url2: "https://example.com/logo.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta1 := NewCachedLogoMetadata(tt.url1)
			meta2 := NewCachedLogoMetadata(tt.url2)

			// Same normalized ID despite different URL forms
			assert.Equal(t, meta1.GetID(), meta2.GetID(),
				"URLs should normalize to same ID:\n  %s\n  %s", tt.url1, tt.url2)

			// But original URLs are preserved
			assert.Equal(t, tt.url1, meta1.OriginalURL)
			assert.Equal(t, tt.url2, meta2.OriginalURL)
		})
	}
}

func TestNewCachedLogoMetadata_DifferentURLs(t *testing.T) {
	// Different URLs should produce different IDs
	meta1 := NewCachedLogoMetadata("https://example.com/logo1.png")
	meta2 := NewCachedLogoMetadata("https://example.com/logo2.png")

	assert.NotEqual(t, meta1.GetID(), meta2.GetID())
}

func TestNewUploadedLogoMetadata(t *testing.T) {
	// Uploaded logos (no URL) get unique IDs
	meta1 := NewUploadedLogoMetadata()
	meta2 := NewUploadedLogoMetadata()

	assert.NotEmpty(t, meta1.GetID())
	assert.NotEmpty(t, meta2.GetID())
	// Each upload gets unique ID
	assert.NotEqual(t, meta1.GetID(), meta2.GetID())
	// No URL for uploaded logos
	assert.Empty(t, meta1.OriginalURL)
	// Source is uploaded
	assert.Equal(t, LogoSourceUploaded, meta1.Source)
}

func TestCachedLogoMetadata_IsPrunable(t *testing.T) {
	// Cached logos can be pruned
	cached := NewCachedLogoMetadata("https://example.com/logo.png")
	assert.True(t, cached.IsPrunable())

	// Uploaded logos cannot be pruned
	uploaded := NewUploadedLogoMetadata()
	assert.False(t, uploaded.IsPrunable())
}

func TestCachedLogoMetadata_MarkSeen(t *testing.T) {
	meta := NewCachedLogoMetadata("https://example.com/logo.png")
	originalSeen := meta.LastSeenAt

	// Wait a tiny bit and mark seen
	time.Sleep(time.Millisecond)
	meta.MarkSeen()

	assert.True(t, meta.LastSeenAt.After(originalSeen))
}

func TestCachedLogoMetadata_ExtensionFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/jpg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/svg+xml", ".svg"},
		{"image/x-icon", ".ico"},
		{"image/bmp", ".bmp"},
		{"image/tiff", ".tiff"},
		{"application/octet-stream", ".png"}, // Default
		{"", ".png"},                          // Default
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			meta := &CachedLogoMetadata{
				ID:          "TEST",
				ContentType: tt.contentType,
			}
			ext := meta.extension()
			assert.Equal(t, tt.expected, ext)
		})
	}
}
