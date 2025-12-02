package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// LogoSource indicates whether a logo was cached from a URL or manually uploaded.
type LogoSource string

const (
	// LogoSourceCached indicates the logo was downloaded from a remote URL.
	// These logos can be pruned if not seen in channel/program data.
	LogoSourceCached LogoSource = "cached"

	// LogoSourceUploaded indicates the logo was manually uploaded.
	// These logos are never automatically pruned.
	LogoSourceUploaded LogoSource = "uploaded"
)

// LinkedAsset represents a single file linked to a logo.
type LinkedAsset struct {
	// Type identifies the asset variant: "converted", "original", etc.
	Type string `json:"type"`

	// Path is the relative path to the asset file from the data directory.
	Path string `json:"path"`

	// ContentType is the MIME type of this specific asset.
	ContentType string `json:"content_type"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`
}

// CachedLogoMetadata represents metadata stored alongside a cached logo file.
// Each logo has a hash-based filename derived from the normalized URL.
//
// The ID is deterministic: the same URL always produces the same ID,
// ensuring logos shared across multiple channels are only downloaded once.
//
// Directory structure:
//   - logos/cached/{hash}.{ext} - for URL-sourced logos (can be pruned)
//   - logos/uploaded/{ulid}.{ext} - for manually uploaded (never auto-pruned)
type CachedLogoMetadata struct {
	// ID is the unique identifier for this cached logo.
	// For URL-sourced logos, this is a SHA256 hash of the normalized URL.
	// For uploaded logos (no URL), this is a ULID.
	ID string `json:"id"`

	// Source indicates whether this logo was cached from URL or uploaded.
	// Used for determining pruning eligibility.
	Source LogoSource `json:"source,omitempty"`

	// OriginalURL is the source URL where the logo was fetched from (before normalization).
	// Empty for uploaded logos.
	OriginalURL string `json:"original_url"`

	// NormalizedURL is the URL after normalization (scheme removed, params sorted, etc).
	// The ID is derived from this, not OriginalURL.
	NormalizedURL string `json:"normalized_url,omitempty"`

	// URLHash is kept for backwards compatibility.
	// For new entries, this equals ID when logo was fetched from URL.
	URLHash string `json:"url_hash,omitempty"`

	// ContentType is the MIME type of the display image (e.g., "image/png").
	// This is the converted/display format, typically PNG.
	ContentType string `json:"content_type,omitempty"`

	// OriginalContentType is the MIME type of the original source image.
	// May differ from ContentType if the image was converted.
	OriginalContentType string `json:"original_content_type,omitempty"`

	// FileSize is the size of the display/converted image in bytes.
	FileSize int64 `json:"file_size,omitempty"`

	// OriginalFileSize is the size of the original image in bytes.
	// Zero if original was not retained (e.g., already PNG).
	OriginalFileSize int64 `json:"original_file_size,omitempty"`

	// Width is the image width in pixels (if known).
	Width int `json:"width,omitempty"`

	// Height is the image height in pixels (if known).
	Height int `json:"height,omitempty"`

	// CreatedAt is when the logo was first cached.
	CreatedAt time.Time `json:"created_at"`

	// LastSeenAt is when this logo was last seen in channel/program data.
	// Updated each time the logo URL appears during pipeline processing.
	// Used for time-based pruning of stale logos.
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`

	// SourceHint is optional context about where this logo came from.
	// Examples: "channel:BBC One", "program:News at Ten"
	SourceHint string `json:"source_hint,omitempty"`

	// LinkedAssets is the list of all files associated with this logo.
	// Includes the converted/display image and optionally the original.
	LinkedAssets []LinkedAsset `json:"linked_assets,omitempty"`
}

// NewCachedLogoMetadata creates a new metadata entry for a logo URL.
// The ID is deterministic - same normalized URL always produces same ID.
// This ensures logos shared across channels are only downloaded once.
func NewCachedLogoMetadata(originalURL string) *CachedLogoMetadata {
	normalized := normalizeURL(originalURL)
	urlHash := computeURLHash(normalized)
	now := time.Now().UTC()
	return &CachedLogoMetadata{
		ID:            urlHash, // Deterministic ID from normalized URL
		Source:        LogoSourceCached,
		OriginalURL:   originalURL,
		NormalizedURL: normalized,
		URLHash:       urlHash,
		CreatedAt:     now,
		LastSeenAt:    now, // Initially seen at creation time
	}
}

// NewUploadedLogoMetadata creates a new metadata entry for an uploaded logo.
// Since uploaded logos have no URL, a ULID is used as the unique identifier.
// Uploaded logos are never automatically pruned.
func NewUploadedLogoMetadata() *CachedLogoMetadata {
	id := ulid.Make().String()
	return &CachedLogoMetadata{
		ID:        id,
		Source:    LogoSourceUploaded,
		CreatedAt: time.Now().UTC(),
	}
}

// GetID returns the canonical identifier for this logo.
func (m *CachedLogoMetadata) GetID() string {
	return m.ID
}

// GetSource returns the logo source, defaulting to Cached for backwards compatibility.
func (m *CachedLogoMetadata) GetSource() LogoSource {
	if m.Source != "" {
		return m.Source
	}
	// Backwards compatibility: assume cached if source not set but has URL
	if m.OriginalURL != "" {
		return LogoSourceCached
	}
	return LogoSourceUploaded
}

// IsPrunable returns true if this logo can be automatically pruned.
// Only cached logos (from URLs) can be pruned; uploaded logos are permanent.
func (m *CachedLogoMetadata) IsPrunable() bool {
	return m.GetSource() == LogoSourceCached
}

// MarkSeen updates LastSeenAt to current time.
// Called when the logo URL is encountered during pipeline processing.
func (m *CachedLogoMetadata) MarkSeen() {
	m.LastSeenAt = time.Now().UTC()
}

// normalizeURL normalizes a URL for consistent hashing.
// This ensures that equivalent URLs produce the same hash:
//   - Removes scheme (http/https treated as equivalent)
//   - Lowercases hostname
//   - Removes default ports (80, 443)
//   - Sorts query parameters alphabetically
//   - Removes trailing slashes from path
//   - Removes common image extensions from path (for CDN variants)
func normalizeURL(rawURL string) string {
	// Handle empty URL
	if rawURL == "" {
		return ""
	}

	// Parse the URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, just lowercase and return
		return strings.ToLower(rawURL)
	}

	// Lowercase the host
	host := strings.ToLower(parsed.Host)

	// Remove default ports
	host = strings.TrimSuffix(host, ":80")
	host = strings.TrimSuffix(host, ":443")

	// Get path, remove trailing slash
	path := strings.TrimSuffix(parsed.Path, "/")

	// Sort query parameters for consistent ordering
	query := parsed.Query()
	var sortedParams []string
	for key := range query {
		for _, val := range query[key] {
			sortedParams = append(sortedParams, key+"="+val)
		}
	}
	sort.Strings(sortedParams)

	// Build normalized URL (without scheme)
	result := host + path
	if len(sortedParams) > 0 {
		result += "?" + strings.Join(sortedParams, "&")
	}

	return result
}

// ImagePath returns just the filename for the image file.
func (m *CachedLogoMetadata) ImagePath() string {
	return m.GetID() + m.extension()
}

// MetadataPath returns just the filename for the metadata JSON file.
func (m *CachedLogoMetadata) MetadataPath() string {
	return m.GetID() + ".json"
}

// SourceDir returns the source-based directory name ("cached" or "uploaded").
// This enables different pruning policies for each type.
func (m *CachedLogoMetadata) SourceDir() string {
	return string(m.GetSource())
}

// RelativeImagePath returns the full relative path for the image file.
// Format: logos/{source}/{id}.{ext}
// - logos/cached/{hash}.png for URL-sourced logos
// - logos/uploaded/{ulid}.png for manually uploaded logos
func (m *CachedLogoMetadata) RelativeImagePath() string {
	return filepath.Join("logos", m.SourceDir(), m.ImagePath())
}

// RelativeMetadataPath returns the full relative path for the metadata file.
// Format: logos/{source}/{id}.json
func (m *CachedLogoMetadata) RelativeMetadataPath() string {
	return filepath.Join("logos", m.SourceDir(), m.MetadataPath())
}

// extension returns the file extension based on content type.
// Defaults to .png if content type is unknown.
func (m *CachedLogoMetadata) extension() string {
	ext := extensionFromContentType(m.ContentType)
	if ext == "" {
		return ".png" // Default to PNG
	}
	return ext
}

// originalExtension returns the file extension for the original image.
func (m *CachedLogoMetadata) originalExtension() string {
	if m.OriginalContentType == "" {
		return ""
	}
	return extensionFromContentType(m.OriginalContentType)
}

// OriginalImagePath returns just the filename for the original image file.
// Returns empty string if no original was stored.
func (m *CachedLogoMetadata) OriginalImagePath() string {
	ext := m.originalExtension()
	if ext == "" || ext == m.extension() {
		return "" // No separate original (same format or not stored)
	}
	return m.GetID() + "_original" + ext
}

// RelativeOriginalImagePath returns the full relative path for the original image file.
// Returns empty string if no original was stored.
func (m *CachedLogoMetadata) RelativeOriginalImagePath() string {
	originalPath := m.OriginalImagePath()
	if originalPath == "" {
		return ""
	}
	return filepath.Join("logos", m.SourceDir(), originalPath)
}

// HasOriginalImage returns true if an original image file was stored separately.
func (m *CachedLogoMetadata) HasOriginalImage() bool {
	return m.OriginalContentType != "" && m.OriginalContentType != m.ContentType
}

// AddLinkedAsset adds or updates a linked asset in the list.
func (m *CachedLogoMetadata) AddLinkedAsset(asset LinkedAsset) {
	// Check if this asset type already exists and update it
	for i, existing := range m.LinkedAssets {
		if existing.Type == asset.Type {
			m.LinkedAssets[i] = asset
			return
		}
	}
	// Add new asset
	m.LinkedAssets = append(m.LinkedAssets, asset)
}

// GetLinkedAsset returns the linked asset of the given type, or nil if not found.
func (m *CachedLogoMetadata) GetLinkedAsset(assetType string) *LinkedAsset {
	for i, asset := range m.LinkedAssets {
		if asset.Type == assetType {
			return &m.LinkedAssets[i]
		}
	}
	return nil
}

// ClearLinkedAssets removes all linked assets.
func (m *CachedLogoMetadata) ClearLinkedAssets() {
	m.LinkedAssets = nil
}

// BuildLinkedAssets populates the LinkedAssets list from current metadata.
// This ensures the linked_assets field reflects the actual stored files.
func (m *CachedLogoMetadata) BuildLinkedAssets() {
	m.LinkedAssets = nil

	// Add converted/display image
	if m.ContentType != "" {
		m.LinkedAssets = append(m.LinkedAssets, LinkedAsset{
			Type:        "converted",
			Path:        m.RelativeImagePath(),
			ContentType: m.ContentType,
			Size:        m.FileSize,
		})
	}

	// Add original image if stored separately
	if m.HasOriginalImage() {
		m.LinkedAssets = append(m.LinkedAssets, LinkedAsset{
			Type:        "original",
			Path:        m.RelativeOriginalImagePath(),
			ContentType: m.OriginalContentType,
			Size:        m.OriginalFileSize,
		})
	}
}

// computeURLHash creates a SHA256 hash of a URL for fast lookups.
func computeURLHash(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// extensionFromContentType is already defined in logo_cache.go but we need it here too.
// This is a helper that maps MIME types to file extensions.
func logoMetadataExtensionFromContentType(contentType string) string {
	// Handle content type with parameters (e.g., "image/png; charset=utf-8")
	contentType = strings.Split(contentType, ";")[0]
	contentType = strings.TrimSpace(contentType)
	contentType = strings.ToLower(contentType)

	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	default:
		return ""
	}
}
