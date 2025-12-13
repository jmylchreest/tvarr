package expression

import (
	"net"
	"net/http"
	"strings"
)

// RequestContextAccessor implements FieldValueAccessor for HTTP request context.
// It extracts field values from an HTTP request for client detection expressions.
type RequestContextAccessor struct {
	request *http.Request
}

// NewRequestContextAccessor creates a new accessor for the given HTTP request.
func NewRequestContextAccessor(r *http.Request) *RequestContextAccessor {
	return &RequestContextAccessor{request: r}
}

// GetFieldValue returns the value of a request context field.
// Returns the value and true if found, or empty string and false if not found.
//
// This accessor provides computed/URL-based fields only. For HTTP header access,
// use @dynamic(request.headers):header-name syntax in expressions.
//
// Available static fields:
//   - client_ip, ip, remote_addr: Client IP (computed from X-Forwarded-For, X-Real-IP, or RemoteAddr)
//   - request_path, path: URL path component
//   - request_url, url: Full request URL
//   - query_params, query: Raw query string
//   - method: HTTP method (GET, POST, etc.)
//   - host: Request host (from URL or Host header)
func (a *RequestContextAccessor) GetFieldValue(name string) (string, bool) {
	if a.request == nil {
		return "", false
	}

	// Normalize field name (lowercase, underscores)
	name = strings.ToLower(name)

	switch name {
	case "client_ip", "ip", "remote_addr":
		return a.getClientIP(), true

	case "request_path", "path":
		return a.request.URL.Path, true

	case "request_url", "url":
		return a.request.URL.String(), true

	case "query_params", "query":
		return a.request.URL.RawQuery, true

	case "method":
		return a.request.Method, true

	case "host":
		// Host is a URL component, not just a header
		if a.request.Host != "" {
			return a.request.Host, true
		}
		return a.request.URL.Host, true

	default:
		// Header-based fields have been moved to @dynamic(request.headers):header-name
		// Do not fall through to header lookup - return not found
		return "", false
	}
}

// getClientIP extracts the real client IP, considering proxy headers.
func (a *RequestContextAccessor) getClientIP() string {
	// Check X-Forwarded-For first (may contain multiple IPs)
	if xff := a.request.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP
	if xri := a.request.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(a.request.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have port
		return a.request.RemoteAddr
	}
	return ip
}

// GetHeader returns a specific HTTP header value.
// This is a convenience method for accessing headers directly.
// Note: For expression-based header access, use @dynamic(request.headers):header-name
func (a *RequestContextAccessor) GetHeader(name string) string {
	if a.request == nil {
		return ""
	}
	return a.request.Header.Get(name)
}

// GetRequest returns the underlying HTTP request.
func (a *RequestContextAccessor) GetRequest() *http.Request {
	return a.request
}
