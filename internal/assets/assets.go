// Package assets provides embedded static assets for the tvarr web UI.
//
// The static/ directory contains the built Next.js frontend. During development,
// this directory may be empty (containing only .gitkeep). For production builds,
// the frontend must be built first:
//
//	cd frontend && npm run build
//	cp -r frontend/out/* internal/assets/static/
//	go build ./cmd/tvarr
//
// The embed directive will include all files from static/ at compile time.
package assets

import (
	"embed"
	"io/fs"
	"mime"
	"path/filepath"
	"strings"
)

// StaticFS embeds the static/ directory containing the built Next.js frontend.
// This will be empty in development builds but populated in production.
//
//go:embed all:static
var StaticFS embed.FS

// GetStaticFS returns a sub-filesystem rooted at "static/" for easier access.
// Returns an error if the static directory doesn't exist.
func GetStaticFS() (fs.FS, error) {
	return fs.Sub(StaticFS, "static")
}

// HasStaticAssets returns true if the static directory contains actual assets
// (more than just .gitkeep).
func HasStaticAssets() bool {
	entries, err := fs.ReadDir(StaticFS, "static")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}

// GetContentType returns the MIME type for a given file path based on extension.
func GetContentType(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return "application/octet-stream"
	}

	// Try standard mime type lookup first
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		return mimeType
	}

	// Fallback for common web extensions
	switch strings.ToLower(ext) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml; charset=utf-8"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".map":
		return "application/json"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// ListAssets returns a list of all embedded asset paths.
func ListAssets() ([]string, error) {
	var assets []string

	err := fs.WalkDir(StaticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() != ".gitkeep" {
			// Remove "static/" prefix for cleaner paths
			cleanPath := strings.TrimPrefix(path, "static/")
			assets = append(assets, cleanPath)
		}
		return nil
	})

	return assets, err
}
