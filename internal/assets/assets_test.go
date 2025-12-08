package assets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetContentType(t *testing.T) {
	// Test that we get reasonable MIME types (exact values depend on system)
	tests := []struct {
		path           string
		expectedPrefix string
	}{
		{"index.html", "text/html"},
		{"style.css", "text/css"},
		{"app.js", "javascript"}, // Could be text/javascript or application/javascript
		{"data.json", "application/json"},
		{"logo.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"animation.gif", "image/gif"},
		{"icon.svg", "image/svg"},
		{"favicon.ico", "image/"}, // Could be image/x-icon or image/vnd.microsoft.icon
		{"font.woff", "font/woff"},
		{"font.woff2", "font/woff2"},
		{"readme.txt", "text/plain"},
		{"unknown", "application/octet-stream"},
		{"file.xyz", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := GetContentType(tt.path)
			assert.Contains(t, result, tt.expectedPrefix, "content type should contain %s, got %s", tt.expectedPrefix, result)
		})
	}
}

func TestHasStaticAssets(t *testing.T) {
	// In development, this will return false (only .gitkeep exists)
	// In production builds with frontend, this will return true
	hasAssets := HasStaticAssets()
	t.Logf("HasStaticAssets: %v", hasAssets)
	// We don't assert the value since it depends on build context
}

func TestGetStaticFS(t *testing.T) {
	staticFS, err := GetStaticFS()
	assert.NoError(t, err)
	assert.NotNil(t, staticFS)
}

func TestListAssets(t *testing.T) {
	assets, err := ListAssets()
	assert.NoError(t, err)
	t.Logf("Found %d assets", len(assets))
	for _, asset := range assets {
		t.Logf("  - %s", asset)
	}
}
