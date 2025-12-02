package middleware

import (
	"net/http"
	"strings"
)

// SkipCompressionForSSE wraps a compression middleware handler to skip
// compression for SSE (Server-Sent Events) endpoints.
// SSE requires unbuffered streaming; compression middleware interferes with flushing.
func SkipCompressionForSSE(compressionHandler func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// Create the compression-wrapped handler
		compressedHandler := compressionHandler(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this is an SSE request by looking at the Accept header
			// or the request path (SSE endpoints typically have specific paths)
			acceptHeader := r.Header.Get("Accept")
			if strings.Contains(acceptHeader, "text/event-stream") {
				// Skip compression for SSE - call the next handler directly
				next.ServeHTTP(w, r)
				return
			}

			// Also check if the request is to a known SSE endpoint path
			if strings.HasSuffix(r.URL.Path, "/events") && strings.Contains(r.URL.Path, "/progress/") {
				// Skip compression for progress SSE endpoints
				next.ServeHTTP(w, r)
				return
			}

			// Apply compression for all other requests
			compressedHandler.ServeHTTP(w, r)
		})
	}
}
