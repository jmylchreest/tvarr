package handlers

import (
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
	// Always return 404 for API routes that weren't matched by registered handlers
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	if !h.hasAssets {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
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

	// Try to find the file
	staticFS, _ := assets.GetStaticFS()
	_, err := fs.Stat(staticFS, filePath)

	if err != nil {
		// File not found - try serving index.html for SPA routing
		// Check if it's a directory path (might have index.html)
		indexPath := path.Join(filePath, "index.html")
		if _, err := fs.Stat(staticFS, indexPath); err == nil {
			// Serve the directory's index.html
			r.URL.Path = "/" + indexPath
			h.setHeaders(w, indexPath)
			h.fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve root index.html for non-file paths
		// (paths without file extensions that aren't API routes)
		if !strings.Contains(path.Base(urlPath), ".") && !strings.HasPrefix(urlPath, "/api/") {
			r.URL.Path = "/index.html"
			h.setHeaders(w, "index.html")
			h.fileServer.ServeHTTP(w, r)
			return
		}

		// Return 404 for actual file requests that don't exist
		http.NotFound(w, r)
		return
	}

	// Serve the file
	h.setHeaders(w, filePath)
	h.fileServer.ServeHTTP(w, r)
}

// setHeaders sets appropriate cache and content-type headers.
func (h *StaticHandler) setHeaders(w http.ResponseWriter, filePath string) {
	contentType := assets.GetContentType(filePath)
	w.Header().Set("Content-Type", contentType)

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
