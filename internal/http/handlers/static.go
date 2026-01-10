package handlers

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/jmylchreest/tvarr/internal/assets"
)

// StaticHandler serves embedded static assets for the web UI.
type StaticHandler struct {
	fileServer http.Handler
	hasAssets  bool
}

// NewStaticHandler creates a new static asset handler.
// If no static assets are embedded, it will serve a "UI not available" message.
func NewStaticHandler() *StaticHandler {
	hasAssets := assets.HasStaticAssets()

	var fileServer http.Handler
	if hasAssets {
		staticFS, err := assets.GetStaticFS()
		if err == nil {
			fileServer = http.FileServer(http.FS(staticFS))
		}
	}

	return &StaticHandler{
		fileServer: fileServer,
		hasAssets:  hasAssets,
	}
}

// ServeHTTP handles HTTP requests for static assets.
// It implements SPA (Single Page Application) routing by serving index.html
// for paths that don't match actual files.
func (h *StaticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Always return 404 for API routes that weren't matched by registered handlers.
	// This includes /api/* and backend streaming routes like /channel/*, /proxy/*, /relay/*, /live/*
	if strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/channel/") ||
		strings.HasPrefix(r.URL.Path, "/proxy/") ||
		strings.HasPrefix(r.URL.Path, "/relay/") ||
		strings.HasPrefix(r.URL.Path, "/live/") {
		http.NotFound(w, r)
		return
	}

	if !h.hasAssets {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>tvarr</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 600px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        p { color: #666; line-height: 1.6; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>tvarr API Server</h1>
    <p>The web UI is not available in this build.</p>
    <p>To build with the web UI:</p>
    <ol>
        <li>Build the frontend: <code>cd frontend && npm run build</code></li>
        <li>Copy assets: <code>cp -r frontend/out/* internal/assets/static/</code></li>
        <li>Rebuild the binary: <code>go build ./cmd/tvarr</code></li>
    </ol>
    <p>API documentation is available at <a href="/docs">/docs</a></p>
</body>
</html>`))
		return
	}

	// Clean the request path
	urlPath := path.Clean(r.URL.Path)
	if urlPath == "" {
		urlPath = "/"
	}

	// Remove leading slash for file lookup
	filePath := strings.TrimPrefix(urlPath, "/")
	if filePath == "" {
		filePath = "index.html"
	}

	staticFS, _ := assets.GetStaticFS()

	// Try to find the file
	fileInfo, err := fs.Stat(staticFS, filePath)
	if err != nil {
		// File not found - check if it's a directory path with index.html
		indexPath := path.Join(filePath, "index.html")
		if _, err := fs.Stat(staticFS, indexPath); err == nil {
			// Serve the directory's index.html directly (no redirect)
			h.serveFile(w, r, staticFS, indexPath)
			return
		}

		// File doesn't exist - return 404
		http.NotFound(w, r)
		return
	}

	// If it's a directory, serve its index.html
	if fileInfo.IsDir() {
		indexPath := path.Join(filePath, "index.html")
		if _, err := fs.Stat(staticFS, indexPath); err == nil {
			h.serveFile(w, r, staticFS, indexPath)
			return
		}
		// Directory without index.html - return 404
		http.NotFound(w, r)
		return
	}

	// Serve the file directly
	h.serveFile(w, r, staticFS, filePath)
}

// serveFile serves a file from the filesystem with proper headers.
func (h *StaticHandler) serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, filePath string) {
	file, err := fsys.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	// Get file info for size
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set Content-Type explicitly before ServeContent can guess it
	// This must be done before any writes to the response
	contentType := assets.GetContentType(filePath)
	w.Header().Set("Content-Type", contentType)

	// Set cache headers
	h.setCacheHeaders(w, filePath)

	// For seekable files, use http.ServeContent for range support
	// Pass empty string for name so it doesn't try to sniff content type
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "", stat.ModTime(), seeker)
		return
	}

	// Fallback: just copy the content
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

// setCacheHeaders sets appropriate cache headers based on file type.
func (h *StaticHandler) setCacheHeaders(w http.ResponseWriter, filePath string) {
	// Set cache headers based on file type
	if strings.Contains(filePath, "_next/static/") || strings.HasSuffix(filePath, ".woff2") {
		// Hashed Next.js assets and fonts can be cached for a long time
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if strings.HasSuffix(filePath, ".js") || strings.HasSuffix(filePath, ".css") {
		// JS and CSS files - moderate cache
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else if strings.HasSuffix(filePath, ".html") {
		// HTML files should not be cached (SPA routing)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	} else {
		// Default cache for other assets
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
}
