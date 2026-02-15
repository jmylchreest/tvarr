// Package urlutil provides URL manipulation utilities.
package urlutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/jmylchreest/tvarr/pkg/httpclient"
)

// URL scheme constants.
const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
	SchemeFile  = "file"
)

// NormalizeBaseURL normalizes a base URL for consistent use:
//   - Adds http:// scheme if no scheme provided
//   - Removes trailing slash for clean path joining
//
// Examples:
//
//	"www.mysite.com"       -> "http://www.mysite.com"
//	"https://mysite.com/"  -> "https://mysite.com"
//	"http://localhost:8080/" -> "http://localhost:8080"
//	"mysite.com:8080"      -> "http://mysite.com:8080"
func NormalizeBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}

	// Trim whitespace
	baseURL = strings.TrimSpace(baseURL)

	// Add default http:// scheme if not present
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	// Remove trailing slash for clean path joining
	baseURL = strings.TrimSuffix(baseURL, "/")

	return baseURL
}

// JoinPath joins a base URL with a path, ensuring single slashes.
// The path should start with / for absolute paths.
func JoinPath(baseURL, path string) string {
	if baseURL == "" {
		return path
	}

	// Normalize base URL
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return baseURL + path
}

// IsRemoteURL checks if a URL is a remote URL that can be fetched.
// This includes:
//   - URLs with http:// or https:// scheme
//   - Protocol-relative URLs (//example.com/...)
//
// Returns false for relative paths, empty strings, or local paths.
func IsRemoteURL(u string) bool {
	return strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "//")
}

// IsFileURL checks if a URL uses the file:// scheme.
func IsFileURL(u string) bool {
	return strings.HasPrefix(u, "file://")
}

// IsSupportedURL checks if a URL uses a supported scheme (http, https, or file).
func IsSupportedURL(u string) bool {
	return IsRemoteURL(u) || IsFileURL(u)
}

// GetScheme returns the scheme of a URL (http, https, file) or empty string if unknown.
func GetScheme(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme)
}

// FilePathFromURL extracts the file path from a file:// URL.
// Returns the path and nil error on success.
// For non-file URLs, returns empty string and an error.
func FilePathFromURL(u string) (string, error) {
	if !IsFileURL(u) {
		return "", fmt.Errorf("not a file:// URL: %s", u)
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// For file:// URLs, the path is the file path
	// Handle both file:///path and file://localhost/path formats
	path := parsed.Path

	// On Windows, file:///C:/path becomes /C:/path, need to strip leading /
	// This is handled by the caller if needed

	if path == "" {
		return "", fmt.Errorf("empty path in file URL: %s", u)
	}

	return path, nil
}

// ResourceFetcher provides a unified interface for fetching resources from
// HTTP/HTTPS URLs and file:// URLs.
type ResourceFetcher struct {
	httpClient *httpclient.Client
}

// NewResourceFetcher creates a new ResourceFetcher with the given HTTP client config.
// Uses a per-host circuit breaker from the default manager.
func NewResourceFetcher(cfg httpclient.Config) *ResourceFetcher {
	breaker := httpclient.DefaultManager.GetOrCreate("resource-fetcher")
	return &ResourceFetcher{
		httpClient: httpclient.NewWithBreaker(cfg, breaker),
	}
}

// NewResourceFetcherWithBreaker creates a ResourceFetcher with a specific circuit breaker.
func NewResourceFetcherWithBreaker(cfg httpclient.Config, breaker *httpclient.CircuitBreaker) *ResourceFetcher {
	return &ResourceFetcher{
		httpClient: httpclient.NewWithBreaker(cfg, breaker),
	}
}

// NewDefaultResourceFetcher creates a ResourceFetcher with default settings.
func NewDefaultResourceFetcher() *ResourceFetcher {
	cfg := httpclient.DefaultConfig()
	breaker := httpclient.DefaultManager.GetOrCreate("resource-fetcher")
	return &ResourceFetcher{
		httpClient: httpclient.NewWithBreaker(cfg, breaker),
	}
}

// Fetch retrieves content from a URL (http://, https://, or file://).
// Returns an io.ReadCloser that must be closed by the caller.
func (f *ResourceFetcher) Fetch(ctx context.Context, u string) (io.ReadCloser, error) {
	scheme := GetScheme(u)

	switch scheme {
	case SchemeHTTP, SchemeHTTPS:
		return f.fetchHTTP(ctx, u)
	case SchemeFile:
		return f.fetchFile(u)
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (URL: %s)", scheme, u)
	}
}

// fetchHTTP fetches content from an HTTP/HTTPS URL.
func (f *ResourceFetcher) fetchHTTP(ctx context.Context, u string) (io.ReadCloser, error) {
	resp, err := f.httpClient.Get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// fetchFile fetches content from a file:// URL.
func (f *ResourceFetcher) fetchFile(u string) (io.ReadCloser, error) {
	path, err := FilePathFromURL(u)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// ValidateURL checks if a URL is valid and uses a supported scheme.
// Returns nil if valid, or an error describing the problem.
func ValidateURL(u string) error {
	if u == "" {
		return fmt.Errorf("URL is required")
	}

	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case SchemeHTTP, SchemeHTTPS:
		// Valid remote URL
		return nil
	case SchemeFile:
		// Valid file URL - check path exists
		path, err := FilePathFromURL(u)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file not found: %s", path)
			}
			return fmt.Errorf("cannot access file: %w", err)
		}
		return nil
	case "":
		return fmt.Errorf("URL must include a scheme (http://, https://, or file://)")
	default:
		return fmt.Errorf("unsupported URL scheme: %s (supported: http, https, file)", scheme)
	}
}
