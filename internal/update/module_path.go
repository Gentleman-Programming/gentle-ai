package update

import (
	"runtime/debug"
	"strconv"
	"strings"
)

// ModulePathForVersion returns the Go module path for base at the given
// version. Go requires a /vN suffix on the module path for major >= 2 and
// forbids it for v0/v1. The suffix is derived from the version being
// installed, never from the running binary, so cross-major upgrades compose
// a resolvable target. The suffix is inserted immediately after the /<repo>
// segment of base (e.g. github.com/o/r + v4 + subpath → github.com/o/r/v4/subpath);
// when base has no /<repo> segment, the suffix is appended. When version is
// empty or cannot be parsed, the running binary's major is used
// (debug.ReadBuildInfo). Beta installs must instead use the validated module
// directive at the checked commit, never this fallback.
func ModulePathForVersion(base, repo, version string) string {
	major := parseMajorVersion(version)
	if major < 0 {
		major = runningGoMajor()
	}
	if major < 2 {
		return base
	}
	suffix := "/v" + strconv.Itoa(major)
	if insertAt := repoSegmentInsertPoint(base, repo); insertAt >= 0 {
		return base[:insertAt] + suffix + base[insertAt:]
	}
	return base + suffix
}

// repoSegmentInsertPoint returns the byte offset at which to insert the /vN
// suffix into base so it lands immediately after the /<repo> segment that
// follows the owner (e.g. github.com/<owner>/<repo>), or -1 when no such
// segment exists. The repo segment is determined by Go's own module-path
// layout: github.com/<owner>/<repo>[/<subpath>...] — the third path
// segment, right after the owner. Earlier occurrences of "/<repo>" would be
// substring matches inside the owner (e.g. "guardian-angel" inside
// "gentleman-guardian-angel"), and later occurrences are subpath components
// that happen to share the repo name (e.g. the final "/gentle-ai" in
// ".../cmd/gentle-ai").
func repoSegmentInsertPoint(base, repo string) int {
	if repo == "" {
		return -1
	}
	// Walk segments to find the third one (after "github.com" and "<owner>").
	// Splitting by "/" lets us treat each segment atomically instead of
	// chasing substring matches that span segment boundaries.
	parts := strings.Split(base, "/")
	// Need at least github.com/<owner>/<repo>.
	if len(parts) < 3 {
		return -1
	}
	if parts[2] != repo {
		return -1
	}
	// The byte offset immediately after "/<repo>" is the offset of the "/"
	// that begins the next segment (or len(base) when /<repo> is terminal).
	// Walk through github.com/<owner>/<repo> summing the segment widths and
	// the single "/" that follows each; for the last one (<repo> itself),
	// only add the segment width and stop at the trailing "/" boundary of
	// the third segment.
	offset := 0
	for i := 0; i < 2; i++ {
		offset += len(parts[i]) + 1 // include the trailing "/"
	}
	offset += len(parts[2]) // "/<repo>" itself, no trailing "/" yet
	// If there is a trailing "/" or another segment after <repo>, the offset
	// already lands at the start of that separator/segment, which is the
	// insertion point. If <repo> is terminal, len(base) is the natural
	// append-at-end position.
	if offset > len(base) {
		offset = len(base)
	}
	return offset
}

// runningGoMajor returns the major-version suffix of the running binary's own
// module path, parsed from debug.ReadBuildInfo().Main.Path. Major 1 (or
// unsuffixed) returns 0 so callers using ModulePathForVersion produce an
// unsuffixed path; major >= 2 returns the parsed integer.
//
// Package-level var so tests can pin the running-major seam (e.g. to 3 in
// the repo's v3 unit tests) without rebuilding. Beta upgrade paths do not
// use this seam; they require a checked commit's validated go.mod directive.
var runningGoMajor = func() int {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil || info.Main.Path == "" {
		return 0
	}
	return parseMajorFromModulePath(info.Main.Path)
}

// parseMajorFromModulePath returns the trailing /vN integer in a Go module
// path, or 0 when the path has no /vN suffix (so v0/v1 callers compose an
// unsuffixed import path).
func parseMajorFromModulePath(modulePath string) int {
	i := strings.LastIndex(modulePath, "/v")
	if i < 0 || i+2 == len(modulePath) {
		return 0
	}
	tail := modulePath[i+2:]
	end := strings.IndexAny(tail, "/")
	if end >= 0 {
		tail = tail[:end]
	}
	n, err := strconv.Atoi(tail)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// parseMajorVersion returns the leading integer of a version string, or -1
// when the version cannot be parsed. Accepts "3.4.0", "v3.4.0", "4.0.0-rc.1",
// "10.0.0" — the leading integer up to the first dot, regardless of any
// preceding v/V.
func parseMajorVersion(version string) int {
	v := strings.TrimSpace(version)
	if v == "" {
		return -1
	}
	if len(v) > 0 && (v[0] == 'v' || v[0] == 'V') {
		v = v[1:]
	}
	if v == "" {
		return -1
	}
	end := strings.IndexAny(v, ".")
	if end < 0 {
		end = len(v)
	}
	digits := v[:end]
	if digits == "" {
		return -1
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return -1
		}
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return -1
	}
	return n
}
