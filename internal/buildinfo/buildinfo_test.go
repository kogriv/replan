package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolveInjectedValues(t *testing.T) {
	t.Cleanup(func() { Version, Commit, BuildDate = "", "", "" })
	Version, Commit, BuildDate = "v0.1.0", "abcdef012345", "2026-11-03T10:20:30Z"
	got := Resolve(nil, false, "go1.24.9", "linux", "amd64")
	if got.Version != Version || got.Commit != Commit || got.BuildDate != BuildDate || got.Date() != "2026-11-03" || got.ShortCommit() != "abcdef0" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveBuildInfoFallbackAndDirtyMarker(t *testing.T) {
	t.Cleanup(func() { Version, Commit, BuildDate = "", "", "" })
	Version, Commit, BuildDate = "", "", ""
	bi := &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}, Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "123456789abc"},
		{Key: "vcs.time", Value: "2026-10-28T01:02:03Z"},
		{Key: "vcs.modified", Value: "true"},
	}}
	got := Resolve(bi, true, "go1.24.7", "darwin", "arm64")
	if got.Version != "v0.2.0" || got.Commit != "123456789abc+dirty" || got.BuildDate != "2026-10-28T01:02:03Z" || got.ShortCommit() != "1234567+dirty" {
		t.Fatalf("Resolve() = %#v", got)
	}
}

func TestResolveUnknownValues(t *testing.T) {
	t.Cleanup(func() { Version, Commit, BuildDate = "", "", "" })
	Version, Commit, BuildDate = "", "", ""
	got := Resolve(nil, false, "go-test", "test", "arch")
	if got.Version != "dev" || got.Commit != "dev" || got.BuildDate != "dev" {
		t.Fatalf("Resolve() = %#v", got)
	}
}
