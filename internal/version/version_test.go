package version

import (
	"encoding/json"
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
	// Save originals and restore after test
	originalVersion := Version
	defer func() { Version = originalVersion }()

	Version = "1.0.0"
	s := Short()

	// Short() does not include ApplicationName (Cobra adds it)
	if !strings.Contains(s, "1.0.0") {
		t.Errorf("expected short string to contain version, got %s", s)
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
	originalBranch := Branch
	originalTreeState := TreeState
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
		Branch = originalBranch
		TreeState = originalTreeState
	}()

	Version = "1.0.0"
	Commit = "abc123def456789"
	Date = "2024-01-15T10:30:00Z"
	Branch = "main"
	TreeState = "clean"

	s := String()

	if !strings.Contains(s, "abc123de") {
		t.Errorf("expected string to contain truncated commit hash, got %s", s)
	}
	if !strings.Contains(s, "2024-01-15") {
		t.Errorf("expected string to contain date, got %s", s)
	}
	if !strings.Contains(s, "branch: main") {
		t.Errorf("expected string to contain branch info, got %s", s)
	}
}

func TestStringWithDirtyTree(t *testing.T) {
	// Save originals and restore after test
	originalVersion := Version
	originalCommit := Commit
	originalTreeState := TreeState
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		TreeState = originalTreeState
	}()

	Version = "1.0.0"
	Commit = "abc123def456789"
	TreeState = "dirty"

	s := String()
	short := Short()

	// Should contain dirty indicator (*)
	if !strings.Contains(s, "abc123de*") {
		t.Errorf("expected string to contain dirty indicator, got %s", s)
	}
	// Short format: "1.0.0 (abc123de*)"
	if !strings.Contains(short, "(abc123de*)") {
		t.Errorf("expected short string to contain dirty indicator, got %s", short)
	}
}

func TestJSON(t *testing.T) {
	// Save originals and restore after test
	originalVersion := Version
	originalCommit := Commit
	originalDate := Date
	originalBranch := Branch
	originalTreeState := TreeState
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
		Branch = originalBranch
		TreeState = originalTreeState
	}()

	Version = "1.2.3"
	Commit = "abc123def456789"
	Date = "2024-01-15T10:30:00Z"
	Branch = "feature-branch"
	TreeState = "clean"

	jsonStr := JSON()

	// Verify it's valid JSON
	var info Info
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		t.Fatalf("JSON() did not produce valid JSON: %v", err)
	}

	// Verify fields
	if info.Version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", info.Version)
	}
	if info.Commit != "abc123def456789" {
		t.Errorf("expected full commit, got %s", info.Commit)
	}
	if info.CommitSHA != "abc123de" {
		t.Errorf("expected short commit sha abc123de, got %s", info.CommitSHA)
	}
	if info.Date != "2024-01-15T10:30:00Z" {
		t.Errorf("expected date 2024-01-15T10:30:00Z, got %s", info.Date)
	}
	if info.Branch != "feature-branch" {
		t.Errorf("expected branch feature-branch, got %s", info.Branch)
	}
	if info.TreeState != "clean" {
		t.Errorf("expected tree_state clean, got %s", info.TreeState)
	}
	if info.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, info.OS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %s, got %s", runtime.GOARCH, info.Arch)
	}
}

func TestGetInfoFields(t *testing.T) {
	// Save originals and restore after test
	originalBranch := Branch
	originalTreeState := TreeState
	defer func() {
		Branch = originalBranch
		TreeState = originalTreeState
	}()

	Branch = "test-branch"
	TreeState = "dirty"

	info := GetInfo()

	if info.Branch != "test-branch" {
		t.Errorf("expected branch test-branch, got %s", info.Branch)
	}
	if info.TreeState != "dirty" {
		t.Errorf("expected tree_state dirty, got %s", info.TreeState)
	}
	if info.OS == "" {
		t.Error("expected non-empty OS")
	}
	if info.Arch == "" {
		t.Error("expected non-empty Arch")
	}
}
