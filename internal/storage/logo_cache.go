// Package storage provides sandboxed file operations for tvarr.
package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogoCache provides cached logo storage operations.
// Directory structure:
//   - logos/cached/  - URL-sourced logos (can be pruned based on LastSeenAt)
//   - logos/uploaded/ - manually uploaded logos (never auto-pruned)
type LogoCache struct {
	sandbox *Sandbox
}

// NewLogoCache creates a new LogoCache in the given base directory.
func NewLogoCache(baseDir string) (*LogoCache, error) {
	sandbox, err := NewSandbox(baseDir)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}

	// Create logos directories for each source type
	if err := sandbox.MkdirAll(filepath.Join("logos", string(LogoSourceCached))); err != nil {
		return nil, fmt.Errorf("creating cached logos directory: %w", err)
	}
	if err := sandbox.MkdirAll(filepath.Join("logos", string(LogoSourceUploaded))); err != nil {
		return nil, fmt.Errorf("creating uploaded logos directory: %w", err)
	}

	return &LogoCache{sandbox: sandbox}, nil
}

// GeneratePath generates a relative file path for a logo based on its URL hash.
// Uses first 2 characters of hash for sharding to avoid too many files in one directory.
func (c *LogoCache) GeneratePath(urlHash string, contentType string) string {
	ext := extensionFromContentType(contentType)

	// Use first 2 chars of hash as shard directory
	shard := urlHash
	if len(shard) > 2 {
		shard = shard[:2]
	}

	return filepath.Join("logos", shard, urlHash+ext)
}

// Store stores a logo from a reader and returns the relative path and file size.
func (c *LogoCache) Store(urlHash string, contentType string, reader io.Reader) (string, int64, error) {
	path := c.GeneratePath(urlHash, contentType)

	if err := c.sandbox.AtomicWriteReader(path, reader); err != nil {
		return "", 0, fmt.Errorf("writing logo file: %w", err)
	}

	size, err := c.sandbox.Size(path)
	if err != nil {
		return "", 0, fmt.Errorf("getting file size: %w", err)
	}

	return path, size, nil
}

// StoreBytes stores logo data from a byte slice and returns the relative path.
func (c *LogoCache) StoreBytes(urlHash string, contentType string, data []byte) (string, error) {
	path := c.GeneratePath(urlHash, contentType)

	if err := c.sandbox.AtomicWrite(path, data); err != nil {
		return "", fmt.Errorf("writing logo file: %w", err)
	}

	return path, nil
}

// Get retrieves a logo file by its relative path.
func (c *LogoCache) Get(relativePath string) (*os.File, error) {
	return c.sandbox.OpenFile(relativePath, os.O_RDONLY, 0)
}

// GetBytes reads all bytes from a logo file.
func (c *LogoCache) GetBytes(relativePath string) ([]byte, error) {
	return c.sandbox.ReadFile(relativePath)
}

// Exists checks if a logo file exists.
func (c *LogoCache) Exists(relativePath string) (bool, error) {
	return c.sandbox.Exists(relativePath)
}

// Delete deletes a logo file.
func (c *LogoCache) Delete(relativePath string) error {
	return c.sandbox.Remove(relativePath)
}

// Size returns the size of a logo file in bytes.
func (c *LogoCache) Size(relativePath string) (int64, error) {
	return c.sandbox.Size(relativePath)
}

// AbsolutePath returns the absolute filesystem path for a relative logo path.
func (c *LogoCache) AbsolutePath(relativePath string) (string, error) {
	return c.sandbox.ResolvePath(relativePath)
}

// CleanupEmptyDirs removes empty subdirectories from the logos directory.
func (c *LogoCache) CleanupEmptyDirs() error {
	logosPath, err := c.sandbox.ResolvePath("logos")
	if err != nil {
		return err
	}

	// Walk the directory tree and collect empty directories
	emptyDirs := make([]string, 0)

	err = filepath.Walk(logosPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() || path == logosPath {
			return nil
		}

		// Check if directory is empty
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		if len(entries) == 0 {
			emptyDirs = append(emptyDirs, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("walking directory: %w", err)
	}

	// Remove empty directories (in reverse order to handle nested dirs)
	for i := len(emptyDirs) - 1; i >= 0; i-- {
		if err := os.Remove(emptyDirs[i]); err != nil {
			// Ignore errors - directory might have been populated
			continue
		}
	}

	return nil
}

// BaseDir returns the absolute path to the cache base directory.
func (c *LogoCache) BaseDir() string {
	return c.sandbox.BaseDir()
}

// LogosDir returns the absolute path to the logos directory.
func (c *LogoCache) LogosDir() (string, error) {
	return c.sandbox.ResolvePath("logos")
}

// extensionFromContentType returns the file extension for a content type.
func extensionFromContentType(contentType string) string {
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
		return "" // No extension for unknown types
	}
}

// ContentTypeFromPath guesses the content type from a file path extension.
func ContentTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	default:
		return "application/octet-stream"
	}
}

// StoreWithMetadata stores a logo with its metadata.
// The image is stored at logos/{source}/{id}.{ext} and
// metadata at logos/{source}/{id}.json.
func (c *LogoCache) StoreWithMetadata(meta *CachedLogoMetadata, imageData io.Reader) error {
	// Store the image file
	if err := c.sandbox.AtomicWriteReader(meta.RelativeImagePath(), imageData); err != nil {
		return fmt.Errorf("writing logo image: %w", err)
	}

	// Get and set file size
	size, err := c.sandbox.Size(meta.RelativeImagePath())
	if err != nil {
		return fmt.Errorf("getting file size: %w", err)
	}
	meta.FileSize = size

	// Build linked assets list
	meta.BuildLinkedAssets()

	// Marshal metadata to JSON
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	// Store the metadata file
	if err := c.sandbox.AtomicWrite(meta.RelativeMetadataPath(), metaJSON); err != nil {
		// Try to clean up image file on error
		_ = c.sandbox.Remove(meta.RelativeImagePath())
		return fmt.Errorf("writing metadata: %w", err)
	}

	return nil
}

// StoreWithMetadataAndOriginal stores both the converted and original images with metadata.
// The converted image is stored at logos/{source}/{id}.{ext}
// The original image is stored at logos/{source}/{id}_original.{orig_ext}
// Metadata is stored at logos/{source}/{id}.json with linked_assets list.
func (c *LogoCache) StoreWithMetadataAndOriginal(meta *CachedLogoMetadata, convertedData, originalData io.Reader) error {
	// Store the converted/display image file
	if err := c.sandbox.AtomicWriteReader(meta.RelativeImagePath(), convertedData); err != nil {
		return fmt.Errorf("writing converted logo image: %w", err)
	}

	// Get and set converted file size
	convertedSize, err := c.sandbox.Size(meta.RelativeImagePath())
	if err != nil {
		return fmt.Errorf("getting converted file size: %w", err)
	}
	meta.FileSize = convertedSize

	// Store original image if present and different format
	if originalData != nil && meta.HasOriginalImage() {
		originalPath := meta.RelativeOriginalImagePath()
		if originalPath != "" {
			if err := c.sandbox.AtomicWriteReader(originalPath, originalData); err != nil {
				// Clean up converted file on error
				_ = c.sandbox.Remove(meta.RelativeImagePath())
				return fmt.Errorf("writing original logo image: %w", err)
			}

			// Get and set original file size
			originalSize, err := c.sandbox.Size(originalPath)
			if err != nil {
				// Clean up on error
				_ = c.sandbox.Remove(meta.RelativeImagePath())
				_ = c.sandbox.Remove(originalPath)
				return fmt.Errorf("getting original file size: %w", err)
			}
			meta.OriginalFileSize = originalSize
		}
	}

	// Build linked assets list
	meta.BuildLinkedAssets()

	// Marshal metadata to JSON
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	// Store the metadata file
	if err := c.sandbox.AtomicWrite(meta.RelativeMetadataPath(), metaJSON); err != nil {
		// Try to clean up all files on error
		_ = c.sandbox.Remove(meta.RelativeImagePath())
		if origPath := meta.RelativeOriginalImagePath(); origPath != "" {
			_ = c.sandbox.Remove(origPath)
		}
		return fmt.Errorf("writing metadata: %w", err)
	}

	return nil
}

// LoadMetadataByPath loads metadata from a specific path (relative to logos dir).
// Use this when you know the exact location, e.g., from scanning.
func (c *LogoCache) LoadMetadataByPath(metaPath string) (*CachedLogoMetadata, error) {
	data, err := c.sandbox.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("reading metadata file: %w", err)
	}

	var meta CachedLogoMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshaling metadata: %w", err)
	}

	// Ensure linked_assets is populated from current metadata fields
	meta.BuildLinkedAssets()

	return &meta, nil
}

// LoadMetadata loads the metadata for a logo by searching both source directories.
// Searches cached/ first, then uploaded/.
func (c *LogoCache) LoadMetadata(id string) (*CachedLogoMetadata, error) {
	// Try cached directory first (more common)
	for _, source := range []LogoSource{LogoSourceCached, LogoSourceUploaded} {
		metaPath := filepath.Join("logos", string(source), id+".json")
		exists, _ := c.sandbox.Exists(metaPath)
		if exists {
			return c.LoadMetadataByPath(metaPath)
		}
	}
	return nil, fmt.Errorf("metadata not found for id: %s", id)
}

// DeleteWithMetadata deletes both the logo image and its metadata file.
// Uses the metadata's source to determine the correct directory.
// Also deletes any linked assets (original image, etc.).
func (c *LogoCache) DeleteWithMetadata(id string, contentType string) error {
	ext := extensionFromContentType(contentType)
	if ext == "" {
		ext = ".png" // Default
	}

	// Try both directories
	for _, source := range []LogoSource{LogoSourceCached, LogoSourceUploaded} {
		imagePath := filepath.Join("logos", string(source), id+ext)
		metaPath := filepath.Join("logos", string(source), id+".json")

		// Check if metadata exists in this directory
		exists, _ := c.sandbox.Exists(metaPath)
		if !exists {
			continue
		}

		// Load metadata to find all linked assets
		meta, _ := c.LoadMetadataByPath(metaPath)

		// Delete all linked assets if metadata was loaded
		if meta != nil && len(meta.LinkedAssets) > 0 {
			for _, asset := range meta.LinkedAssets {
				if asset.Path != "" {
					_ = c.sandbox.Remove(asset.Path)
				}
			}
		}

		// Delete main image file (in case it wasn't in linked assets)
		if err := c.sandbox.Remove(imagePath); err != nil && !os.IsNotExist(err) {
			// Log but continue - file might have been deleted via linked assets
		}

		// Delete original image file if present (legacy cleanup)
		for _, origExt := range []string{".webp", ".jpg", ".jpeg", ".gif"} {
			origPath := filepath.Join("logos", string(source), id+"_original"+origExt)
			_ = c.sandbox.Remove(origPath)
		}

		// Delete metadata file
		if err := c.sandbox.Remove(metaPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("deleting metadata file: %w", err)
		}

		return nil
	}

	return nil // Nothing to delete
}

// DeleteAllLinkedAssets deletes all files associated with a logo's linked assets.
// Does not delete the metadata file itself.
func (c *LogoCache) DeleteAllLinkedAssets(meta *CachedLogoMetadata) error {
	if meta == nil {
		return nil
	}

	for _, asset := range meta.LinkedAssets {
		if asset.Path != "" {
			if err := c.sandbox.Remove(asset.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("deleting linked asset %s: %w", asset.Path, err)
			}
		}
	}

	// Also try standard paths in case linked assets list is incomplete
	if imagePath := meta.RelativeImagePath(); imagePath != "" {
		_ = c.sandbox.Remove(imagePath)
	}
	if origPath := meta.RelativeOriginalImagePath(); origPath != "" {
		_ = c.sandbox.Remove(origPath)
	}

	return nil
}

// TouchMetadata updates the LastSeenAt timestamp in metadata and the file's mtime.
// This is called when a logo URL is encountered during pipeline processing.
// Returns the updated metadata, or nil if the logo doesn't exist.
func (c *LogoCache) TouchMetadata(meta *CachedLogoMetadata) error {
	// Update LastSeenAt
	meta.MarkSeen()

	// Re-serialize and write the metadata file
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	metaPath := meta.RelativeMetadataPath()
	if err := c.sandbox.AtomicWrite(metaPath, metaJSON); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	// Also touch the image file mtime for filesystem-based pruning tools
	imagePath := meta.RelativeImagePath()
	absPath, err := c.sandbox.ResolvePath(imagePath)
	if err != nil {
		return nil // Image might not exist yet, that's okay
	}

	now := time.Now()
	_ = os.Chtimes(absPath, now, now) // Ignore errors - file might not exist

	return nil
}

// SaveMetadata saves the logo metadata to disk without updating timestamps.
func (c *LogoCache) SaveMetadata(meta *CachedLogoMetadata) error {
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	metaPath := meta.RelativeMetadataPath()
	if err := c.sandbox.AtomicWrite(metaPath, metaJSON); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	return nil
}

// GetStaleLogos returns logos that haven't been seen since the cutoff time.
// Only returns cached (URL-sourced) logos; uploaded logos are never stale.
func (c *LogoCache) GetStaleLogos(cutoff time.Time) ([]*CachedLogoMetadata, error) {
	cachedDir := filepath.Join("logos", string(LogoSourceCached))
	absDir, err := c.sandbox.ResolvePath(cachedDir)
	if err != nil {
		return nil, fmt.Errorf("resolving cached logos directory: %w", err)
	}

	var staleLogos []*CachedLogoMetadata

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Only process .json metadata files
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		// Read and parse the metadata
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var meta CachedLogoMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil
		}

		// Check if stale (LastSeenAt before cutoff)
		if !meta.LastSeenAt.IsZero() && meta.LastSeenAt.Before(cutoff) {
			staleLogos = append(staleLogos, &meta)
		} else if meta.LastSeenAt.IsZero() {
			// For backwards compatibility: if LastSeenAt not set, use file mtime
			if info.ModTime().Before(cutoff) {
				staleLogos = append(staleLogos, &meta)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking cached logos directory: %w", err)
	}

	return staleLogos, nil
}

// ScanLogos scans the logos directory and returns all cached logo metadata.
// This is used for rebuilding the in-memory index on startup.
func (c *LogoCache) ScanLogos() ([]*CachedLogoMetadata, error) {
	logosDir, err := c.sandbox.ResolvePath("logos")
	if err != nil {
		return nil, fmt.Errorf("resolving logos directory: %w", err)
	}

	var logos []*CachedLogoMetadata

	err = filepath.Walk(logosDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Only process .json metadata files
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		// Read and parse the metadata
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}

		var meta CachedLogoMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil // Skip invalid JSON
		}

		// Ensure linked_assets is populated from current metadata fields
		// This handles older metadata files that may not have this field stored
		meta.BuildLinkedAssets()

		logos = append(logos, &meta)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking logos directory: %w", err)
	}

	return logos, nil
}

// GetImageAbsolutePath returns the absolute filesystem path for a logo image.
// Searches both cached and uploaded directories.
func (c *LogoCache) GetImageAbsolutePath(id string, contentType string) (string, error) {
	ext := extensionFromContentType(contentType)
	if ext == "" {
		ext = ".png" // Default
	}

	// Try both directories
	for _, source := range []LogoSource{LogoSourceCached, LogoSourceUploaded} {
		imagePath := filepath.Join("logos", string(source), id+ext)
		exists, _ := c.sandbox.Exists(imagePath)
		if exists {
			return c.sandbox.ResolvePath(imagePath)
		}
	}

	// Return cached path as default (for new files)
	imagePath := filepath.Join("logos", string(LogoSourceCached), id+ext)
	return c.sandbox.ResolvePath(imagePath)
}

// GetImageAbsolutePathForMeta returns the absolute path using metadata's known path.
func (c *LogoCache) GetImageAbsolutePathForMeta(meta *CachedLogoMetadata) (string, error) {
	return c.sandbox.ResolvePath(meta.RelativeImagePath())
}
