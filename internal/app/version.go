package app

import (
	"runtime/debug"
	"strings"
)

// buildInfoReader is swappable for testing — prevents reading real BuildInfo in tests.
var buildInfoReader = debug.ReadBuildInfo

// ResolveVersion determines the effective version string.
// Priority: ldflags override > debug.BuildInfo.Main.Version > "dev".
//
// When the binary is built via GoReleaser, ldflagsVersion is a real semver
// (e.g. "1.8.3") and BuildInfo is never consulted. When built via `go install`,
// ldflagsVersion defaults to "dev" and BuildInfo.Main.Version contains the
// tagged version (e.g. "v1.8.3") or "(devel)" for untagged builds.
func ResolveVersion(ldflagsVersion string) string {
	if ldflagsVersion != "dev" {
		return ldflagsVersion
	}

	info, ok := buildInfoReader()
	if !ok {
		return "dev"
	}

	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return "dev"
	}

	return strings.TrimPrefix(v, "v")
}
