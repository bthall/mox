// Package version exposes build-time version metadata.
//
// The values are populated via -ldflags -X at build time. See the project
// Makefile and .goreleaser.yml for the canonical ldflag set.
package version

import "fmt"

var (
	// Version is the semantic version (e.g. "1.2.3"). "dev" for unreleased builds.
	Version = "dev"

	// GitCommit is the full git SHA of the build, or "unknown".
	GitCommit = "unknown"

	// BuildDate is an RFC3339 timestamp, or "unknown".
	BuildDate = "unknown"
)

// BuildInfo is the structured form of the build metadata, suitable for
// programmatic access by importers of pkg/version.
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Info returns a copy of the current build info.
func Info() BuildInfo {
	return BuildInfo{Version: Version, GitCommit: GitCommit, BuildDate: BuildDate}
}

// String returns a short version string suitable for cobra's Version field.
// Cobra will prefix it with the binary name itself, so we do not include it here.
func String() string {
	if GitCommit == "unknown" || GitCommit == "" {
		return fmt.Sprintf("%s (dev build)", Version)
	}
	short := GitCommit
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("%s (%s) built %s", Version, short, BuildDate)
}
