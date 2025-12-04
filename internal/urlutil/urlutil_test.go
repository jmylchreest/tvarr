package urlutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no scheme", "example.com", "http://example.com"},
		{"http", "http://example.com", "http://example.com"},
		{"https", "https://example.com", "https://example.com"},
		{"trailing slash", "http://example.com/", "http://example.com"},
		{"with port", "localhost:8080", "http://localhost:8080"},
		{"whitespace", "  http://example.com  ", "http://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeBaseURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJoinPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{"empty base", "", "/path", "/path"},
		{"with leading slash", "http://example.com", "/api/v1", "http://example.com/api/v1"},
		{"without leading slash", "http://example.com", "api/v1", "http://example.com/api/v1"},
		{"base with trailing slash", "http://example.com/", "/api", "http://example.com/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPath(tt.baseURL, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"http", "http://example.com", true},
		{"https", "https://example.com", true},
		{"protocol-relative", "//example.com", true},
		{"file", "file:///path/to/file", false},
		{"relative", "/path/to/file", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRemoteURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsFileURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"file url", "file:///path/to/file.m3u", true},
		{"file url windows", "file:///C:/path/to/file.m3u", true},
		{"http url", "http://example.com/file.m3u", false},
		{"https url", "https://example.com/file.m3u", false},
		{"relative path", "/path/to/file.m3u", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsFileURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSupportedURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"http", "http://example.com", true},
		{"https", "https://example.com", true},
		{"file", "file:///path/to/file", true},
		{"ftp", "ftp://example.com", false},
		{"relative", "/path/to/file", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSupportedURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetScheme(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"http", "http://example.com", "http"},
		{"https", "https://example.com", "https"},
		{"file", "file:///path/to/file", "file"},
		{"ftp", "ftp://example.com", "ftp"},
		{"invalid", "not-a-url", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetScheme(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilePathFromURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expected    string
		expectError bool
	}{
		{"unix path", "file:///home/user/file.m3u", "/home/user/file.m3u", false},
		{"unix path with spaces", "file:///home/user/my%20file.m3u", "/home/user/my file.m3u", false},
		{"root path", "file:///file.xml", "/file.xml", false},
		{"http url", "http://example.com/file.m3u", "", true},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FilePathFromURL(tt.url)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestResourceFetcher_FetchFile(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.m3u")
	testContent := "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com/stream.m3u8\n"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	fetcher := NewDefaultResourceFetcher()

	t.Run("fetch existing file", func(t *testing.T) {
		fileURL := "file://" + testFile
		reader, err := fetcher.Fetch(context.Background(), fileURL)
		require.NoError(t, err)
		defer reader.Close()

		content := make([]byte, len(testContent))
		n, err := reader.Read(content)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(content[:n]))
	})

	t.Run("fetch non-existent file", func(t *testing.T) {
		fileURL := "file:///nonexistent/path/file.m3u"
		_, err := fetcher.Fetch(context.Background(), fileURL)
		assert.Error(t, err)
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, err := fetcher.Fetch(context.Background(), "ftp://example.com/file.m3u")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported URL scheme")
	})
}

func TestValidateURL(t *testing.T) {
	// Create a temporary file for file:// URL testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.m3u")
	err := os.WriteFile(testFile, []byte("#EXTM3U\n"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		{"valid http", "http://example.com/playlist.m3u", false, ""},
		{"valid https", "https://example.com/playlist.m3u", false, ""},
		{"valid file", "file://" + testFile, false, ""},
		{"empty url", "", true, "URL is required"},
		{"no scheme", "example.com/playlist.m3u", true, "URL must include a scheme"},
		{"unsupported scheme", "ftp://example.com/playlist.m3u", true, "unsupported URL scheme"},
		{"file not found", "file:///nonexistent/path/file.m3u", true, "file not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewResourceFetcher(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		fetcher := NewDefaultResourceFetcher()
		assert.NotNil(t, fetcher)
		assert.NotNil(t, fetcher.httpClient)
	})
}
