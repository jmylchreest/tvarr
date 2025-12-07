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
func (a *RequestContextAccessor) GetFieldValue(name string) (string, bool) {
	if a.request == nil {
		return "", false
	}

	// Normalize field name (lowercase, underscores)
	name = strings.ToLower(name)

	switch name {
	case "user_agent", "ua":
		return a.request.UserAgent(), true

	case "client_ip", "ip", "remote_addr":
		return a.getClientIP(), true

	case "request_path", "path":
		return a.request.URL.Path, true

	case "request_url", "url":
		return a.request.URL.String(), true

	case "query_params", "query":
		return a.request.URL.RawQuery, true

	case "x_forwarded_for":
		return a.request.Header.Get("X-Forwarded-For"), true

	case "x_real_ip":
		return a.request.Header.Get("X-Real-IP"), true

	case "accept":
		return a.request.Header.Get("Accept"), true

	case "accept_language":
		return a.request.Header.Get("Accept-Language"), true

	case "host":
		return a.request.Host, true

	case "referer", "referrer":
		return a.request.Header.Get("Referer"), true

	case "content_type":
		return a.request.Header.Get("Content-Type"), true

	case "method":
		return a.request.Method, true

	default:
		// Check if it's an x_ prefixed header (dynamic header access)
		if strings.HasPrefix(name, "x_") {
			// Convert x_custom_header to X-Custom-Header
			headerName := a.toHTTPHeader(name)
			return a.request.Header.Get(headerName), true
		}

		// Check if it's a direct header name
		if val := a.request.Header.Get(name); val != "" {
			return val, true
		}

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

// toHTTPHeader converts an underscore-separated field name to HTTP header format.
// e.g., "x_custom_header" -> "X-Custom-Header"
func (a *RequestContextAccessor) toHTTPHeader(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(string(part[0])) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}

// GetHeader returns a specific HTTP header value.
// This is a convenience method for accessing headers directly.
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
