// Package version provides build-time version information for tvarr.
//
// Version, Commit, and Date are injected at build time via ldflags:
//
//	go build -ldflags "-X github.com/jmylchreest/tvarr/internal/version.Version=x.y.z \
//	                   -X github.com/jmylchreest/tvarr/internal/version.Commit=$(git rev-parse HEAD) \
//	                   -X github.com/jmylchreest/tvarr/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

import (
	"fmt"
	"runtime"
	"strings"
)

// Build-time variables injected via ldflags.
var (
	// Version is the semantic version following SemVer 2.0.0.
	// Release format: "1.2.3"
	// Prerelease format: "1.2.3-SNAPSHOT.abc1234" (next patch + SNAPSHOT + short SHA)
	Version = "dev"

	// Commit is the full git commit SHA.
	Commit = "unknown"

	// Date is the build timestamp in RFC3339 format.
	Date = "unknown"
)

// Runtime constants.
var (
	// GoVersion is the Go runtime version.
	GoVersion = runtime.Version()
)

// ApplicationName is the canonical name of this application.
const ApplicationName = "tvarr"

// Info contains structured version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetInfo returns all version information as a structured type.
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: GoVersion,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a human-readable version string.
func String() string {
	info := GetInfo()
	if Commit != "unknown" && len(Commit) >= 8 {
		return fmt.Sprintf("%s version %s (commit: %s, built: %s, %s, %s)",
			ApplicationName, info.Version, info.Commit[:8], info.Date, info.GoVersion, info.Platform)
	}
	return fmt.Sprintf("%s version %s (%s, %s)", ApplicationName, info.Version, info.GoVersion, info.Platform)
}

// Short returns a short version string suitable for CLI --version output.
func Short() string {
	if Commit != "unknown" && len(Commit) >= 8 {
		return fmt.Sprintf("%s %s (%s)", ApplicationName, Version, Commit[:8])
	}
	return fmt.Sprintf("%s %s", ApplicationName, Version)
}

// UserAgent returns a User-Agent string for HTTP requests.
func UserAgent() string {
	return fmt.Sprintf("%s/%s", ApplicationName, Version)
}

// IsSnapshot returns true if this is a snapshot/prerelease build.
// Snapshots use SemVer prerelease format: X.Y.Z-SNAPSHOT.commitsha
func IsSnapshot() bool {
	return Version == "dev" || strings.Contains(Version, "-SNAPSHOT")
}

// IsRelease returns true if this is a tagged release build.
func IsRelease() bool {
	return !IsSnapshot() && Version != "dev"
}
