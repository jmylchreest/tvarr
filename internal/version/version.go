// Package version provides build-time version information for tvarr.
//
// Build-time variables are injected via ldflags:
//
//	go build -ldflags "
//	  -X github.com/jmylchreest/tvarr/internal/version.Version=x.y.z
//	  -X github.com/jmylchreest/tvarr/internal/version.Commit=$(git rev-parse HEAD)
//	  -X github.com/jmylchreest/tvarr/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//	  -X github.com/jmylchreest/tvarr/internal/version.Branch=$(git rev-parse --abbrev-ref HEAD)
//	  -X github.com/jmylchreest/tvarr/internal/version.TreeState=$(if git diff --quiet; then echo clean; else echo dirty; fi)
//	"
package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Build-time variables injected via ldflags.
var (
	// Version is the semantic version following SemVer 2.0.0.
	// Release format: "1.2.3"
	// Dev format: "1.2.3-dev.N-HASH" (next patch + dev + commits since release + short SHA)
	// Uses "-" instead of "+" for GitHub releases compatibility
	Version = "dev"

	// Commit is the full git commit SHA.
	Commit = "unknown"

	// Date is the build timestamp in RFC3339 format.
	Date = "unknown"

	// Branch is the git branch name at build time.
	Branch = "unknown"

	// TreeState indicates if the git tree was clean or dirty at build.
	TreeState = "unknown"
)

// Runtime constants.
var (
	// GoVersion is the Go runtime version.
	GoVersion = runtime.Version()
)

func init() {
	// If ldflags weren't provided, try to get VCS info from build info
	if Commit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					Commit = setting.Value
				case "vcs.time":
					Date = setting.Value
				case "vcs.modified":
					if setting.Value == "true" {
						TreeState = "dirty"
					} else {
						TreeState = "clean"
					}
				}
			}
		}
	}
}

// ApplicationName is the canonical name of this application.
const ApplicationName = "tvarr"

// Info contains structured version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	CommitSHA string `json:"commit_sha,omitempty"` // Short SHA for display
	Date      string `json:"date"`
	Branch    string `json:"branch"`
	TreeState string `json:"tree_state"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo returns all version information as a structured type.
func GetInfo() Info {
	commitSHA := ""
	if Commit != "unknown" && len(Commit) >= 8 {
		commitSHA = Commit[:8]
	}

	return Info{
		Version:   Version,
		Commit:    Commit,
		CommitSHA: commitSHA,
		Date:      Date,
		Branch:    Branch,
		TreeState: TreeState,
		GoVersion: GoVersion,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a human-readable version string.
func String() string {
	info := GetInfo()
	if Commit != "unknown" && len(Commit) >= 8 {
		treeIndicator := ""
		if TreeState == "dirty" {
			treeIndicator = "*"
		}
		branchInfo := ""
		if Branch != "unknown" {
			branchInfo = fmt.Sprintf(" branch: %s,", Branch)
		}
		return fmt.Sprintf("%s version %s (commit: %s%s,%s built: %s, %s, %s)",
			ApplicationName, info.Version, info.CommitSHA, treeIndicator, branchInfo, info.Date, info.GoVersion, info.Platform)
	}
	return fmt.Sprintf("%s version %s (%s, %s)", ApplicationName, info.Version, info.GoVersion, info.Platform)
}

// Short returns a short version string suitable for CLI --version output.
// Does not include application name as Cobra adds it automatically.
func Short() string {
	if Commit != "unknown" && len(Commit) >= 8 {
		treeIndicator := ""
		if TreeState == "dirty" {
			treeIndicator = "*"
		}
		return fmt.Sprintf("%s (%s%s)", Version, Commit[:8], treeIndicator)
	}
	return Version
}

// JSON returns the version info as a JSON string for machine parsing.
func JSON() string {
	info := GetInfo()
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// UserAgent returns a User-Agent string for HTTP requests.
func UserAgent() string {
	return fmt.Sprintf("%s/%s", ApplicationName, Version)
}

// IsSnapshot returns true if this is a snapshot/prerelease build.
// Dev builds use format: X.Y.Z-dev.N-HASH
func IsSnapshot() bool {
	return Version == "dev" || strings.Contains(Version, "-dev.")
}

// IsRelease returns true if this is a tagged release build.
func IsRelease() bool {
	return !IsSnapshot() && Version != "dev"
}
