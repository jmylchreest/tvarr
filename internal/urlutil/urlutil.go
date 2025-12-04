// Package urlutil provides URL manipulation utilities.
package urlutil

import (
	"strings"
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
func IsRemoteURL(url string) bool {
	return strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "//")
}
