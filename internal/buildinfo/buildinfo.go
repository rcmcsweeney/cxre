// Package buildinfo exposes version metadata injected by release builds.
package buildinfo

import "fmt"

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info describes the current CXRE build.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Current returns metadata for the running binary.
func Current() Info {
	return Info{Version: Version, Commit: Commit, Date: BuildDate}
}

// String returns the user-facing version line.
func String() string {
	if Version == "dev" {
		return "cxre dev"
	}

	if Commit == "" || Commit == "unknown" {
		return fmt.Sprintf("cxre %s", Version)
	}

	return fmt.Sprintf("cxre %s (%s)", Version, Commit)
}
