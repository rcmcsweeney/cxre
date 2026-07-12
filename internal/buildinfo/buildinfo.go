// Package buildinfo exposes version metadata injected by release builds or
// embedded by the Go toolchain.
package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

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
	embedded, ok := debug.ReadBuildInfo()
	if !ok {
		embedded = nil
	}
	return resolve(Info{Version: Version, Commit: Commit, Date: BuildDate}, embedded)
}

// String returns the user-facing version line.
func String() string {
	return format(Current())
}

// resolve combines release linker values with metadata embedded by the Go
// toolchain. Linker values remain authoritative, while the module version lets
// `go install module@version` builds identify the release they came from.
// Keeping the merge pure makes all fallback behavior testable without
// replacing runtime/debug state.
func resolve(linked Info, embedded *debug.BuildInfo) Info {
	resolved := Info{
		Version: normalizeVersion(linked.Version),
		Commit:  strings.TrimSpace(linked.Commit),
		Date:    strings.TrimSpace(linked.Date),
	}
	if resolved.Version == "" {
		resolved.Version = "dev"
	}
	if resolved.Commit == "" {
		resolved.Commit = "unknown"
	}
	if resolved.Date == "" {
		resolved.Date = "unknown"
	}
	if embedded == nil {
		return resolved
	}

	if resolved.Version == "dev" {
		if version := normalizeVersion(embedded.Main.Version); version != "" && version != "dev" {
			resolved.Version = version
		}
	}

	for _, setting := range embedded.Settings {
		switch setting.Key {
		case "vcs.revision":
			if resolved.Commit == "unknown" && strings.TrimSpace(setting.Value) != "" {
				resolved.Commit = strings.TrimSpace(setting.Value)
			}
		case "vcs.time":
			if resolved.Date == "unknown" && strings.TrimSpace(setting.Value) != "" {
				resolved.Date = strings.TrimSpace(setting.Value)
			}
		}
	}

	return resolved
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "(devel)" || version == "dev" {
		return "dev"
	}
	return strings.TrimPrefix(version, "v")
}

func format(info Info) string {
	if info.Version == "dev" {
		return "cxre dev"
	}

	if info.Commit == "" || info.Commit == "unknown" {
		return fmt.Sprintf("cxre %s", info.Version)
	}

	return fmt.Sprintf("cxre %s (%s)", info.Version, info.Commit)
}
