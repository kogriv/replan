// Package buildinfo resolves metadata embedded in release binaries and the
// module and VCS metadata recorded by the Go toolchain.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// These values are set by release builds with -ldflags -X. They intentionally
// stay strings so the same linker mechanism supplies every release value.
var (
	Version   string
	Commit    string
	BuildDate string
)

// Info is the build metadata exposed by the CLI.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OSArch    string `json:"os_arch"`
}

// Current returns metadata for the running binary.
func Current() Info {
	bi, ok := debug.ReadBuildInfo()
	return Resolve(bi, ok, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// Resolve factors the toolchain fallback so tests can provide BuildInfo.
func Resolve(bi *debug.BuildInfo, ok bool, goVersion, goos, goarch string) Info {
	info := Info{
		Version: Version, Commit: Commit, BuildDate: BuildDate,
		GoVersion: goVersion, OSArch: goos + "/" + goarch,
	}
	modified := false
	if ok && bi != nil {
		if info.Version == "" {
			info.Version = bi.Main.Version
		}
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = setting.Value
				}
			case "vcs.time":
				if info.BuildDate == "" {
					info.BuildDate = setting.Value
				}
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = "dev"
	}
	if modified && !strings.HasSuffix(info.Commit, "+dirty") {
		info.Commit += "+dirty"
	}
	if info.BuildDate == "" {
		info.BuildDate = "dev"
	}
	return info
}

// Date returns the stable YYYY-MM-DD display form where possible.
func (i Info) Date() string {
	if parsed, err := time.Parse(time.RFC3339, i.BuildDate); err == nil {
		return parsed.UTC().Format(time.DateOnly)
	}
	if len(i.BuildDate) >= len(time.DateOnly) {
		if _, err := time.Parse(time.DateOnly, i.BuildDate[:len(time.DateOnly)]); err == nil {
			return i.BuildDate[:len(time.DateOnly)]
		}
	}
	return i.BuildDate
}

// ShortCommit is the seven-character commit used in human output. A dirty
// suffix is preserved.
func (i Info) ShortCommit() string {
	dirty := strings.HasSuffix(i.Commit, "+dirty")
	commit := strings.TrimSuffix(i.Commit, "+dirty")
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if dirty {
		commit += "+dirty"
	}
	return commit
}
