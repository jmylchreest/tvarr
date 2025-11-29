package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetInfo(t *testing.T) {
	info := GetInfo()

	if info.Version == "" {
		t.Error("expected non-empty version")
	}
	if info.GoVersion == "" {
		t.Error("expected non-empty go version")
	}
	if info.Platform == "" {
		t.Error("expected non-empty platform")
	}
	if !strings.Contains(info.Platform, runtime.GOOS) {
		t.Errorf("expected platform to contain %s, got %s", runtime.GOOS, info.Platform)
	}
	if !strings.Contains(info.Platform, runtime.GOARCH) {
		t.Errorf("expected platform to contain %s, got %s", runtime.GOARCH, info.Platform)
	}
}

func TestString(t *testing.T) {
	s := String()

	if !strings.Contains(s, ApplicationName) {
		t.Errorf("expected string to contain %s, got %s", ApplicationName, s)
	}
	if !strings.Contains(s, "version") {
		t.Errorf("expected string to contain 'version', got %s", s)
	}
}

func TestShort(t *testing.T) {
	s := Short()

	if !strings.Contains(s, ApplicationName) {
		t.Errorf("expected short string to contain %s, got %s", ApplicationName, s)
	}
}

func TestUserAgent(t *testing.T) {
	ua := UserAgent()

	if !strings.HasPrefix(ua, ApplicationName+"/") {
		t.Errorf("expected user agent to start with %s/, got %s", ApplicationName, ua)
	}
}

func TestIsSnapshot(t *testing.T) {
	// Save original and restore after test
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		version  string
		expected bool
	}{
		{"dev", true},
		{"1.0.0", false},
		{"1.0.1-SNAPSHOT.abc1234", true}, // SemVer 2.0.0 prerelease format
		{"0.1.0", false},
		{"2.0.0-SNAPSHOT.def5678", true}, // Another snapshot
		{"1.2.3-alpha.1", false},         // Other prerelease, not snapshot
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			Version = tt.version
			if got := IsSnapshot(); got != tt.expected {
				t.Errorf("IsSnapshot() = %v for version %q, want %v", got, tt.version, tt.expected)
			}
		})
	}
}

func TestIsRelease(t *testing.T) {
	// Save original and restore after test
	originalVersion := Version
	defer func() { Version = originalVersion }()

	tests := []struct {
		version  string
		expected bool
	}{
		{"dev", false},
		{"1.0.0", true},
		{"1.0.1-SNAPSHOT.abc1234", false}, // SemVer 2.0.0 prerelease format
		{"0.1.0", true},
		{"1.2.3-alpha.1", true}, // Other prerelease is still a "release"
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			Version = tt.version
			if got := IsRelease(); got != tt.expected {
				t.Errorf("IsRelease() = %v for version %q, want %v", got, tt.version, tt.expected)
			}
		})
	}
}

func TestStringWithCommit(t *testing.T) {
	// Save originals and restore after test
	originalVersion := Version
	originalCommit := Commit
	originalDate := Date
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	}()

	Version = "1.0.0"
	Commit = "abc123def456789"
	Date = "2024-01-15T10:30:00Z"

	s := String()

	if !strings.Contains(s, "abc123de") {
		t.Errorf("expected string to contain truncated commit hash, got %s", s)
	}
	if !strings.Contains(s, "2024-01-15") {
		t.Errorf("expected string to contain date, got %s", s)
	}
}
