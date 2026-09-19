package triageevidence

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ReleaseChannel is the coarse bucket for the version a report names. The four
// values are the only ones the report may produce for a version, so stable
// releases, prereleases, unreleased main, and unknown reports never collapse
// into one another.
type ReleaseChannel int

const (
	// ChannelUnknown means no version could be parsed and no main signal found.
	ChannelUnknown ReleaseChannel = iota
	// ChannelStable is an exact stable release, e.g. v3.4.0.
	ChannelStable
	// ChannelPrerelease is a prerelease of an exact version, e.g. v2.2.0-rc.3.
	ChannelPrerelease
	// ChannelMain is an unreleased build, e.g. "build from main".
	ChannelMain
)

func (c ReleaseChannel) String() string {
	switch c {
	case ChannelStable:
		return "stable"
	case ChannelPrerelease:
		return "prerelease"
	case ChannelMain:
		return "main"
	default:
		return "unknown"
	}
}

// ReportedVersion is the best-effort structured reading of the version string a
// bug report names. Raw always preserves the original text so the report can
// quote it verbatim (escaped); the structured fields are empty/false when the
// input carried no parseable version.
type ReportedVersion struct {
	Raw     string
	Channel ReleaseChannel
	Major   int
	Minor   int
	Patch   int
	Pre     string // prerelease identifier, e.g. "rc.3"; empty for stable
}

// IsVersion reports whether a parseable exact version (stable or prerelease) was
// found. ChannelMain and ChannelUnknown carry no numeric identity.
func (v ReportedVersion) IsVersion() bool {
	return v.Channel == ChannelStable || v.Channel == ChannelPrerelease
}

// Compare orders exact versions. It returns -1, 0, or 1 following semver
// precedence for the fields this report's template exposes (X.Y.Z with an
// optional prerelease): a prerelease of the same X.Y.Z sorts before its stable.
// Versions without a numeric identity (main, unknown) compare as 0 only against
// each other and are otherwise "older" so a report can never claim a main build
// to be current proof by version alone.
func (v ReportedVersion) Compare(other ReportedVersion) int {
	switch {
	case v.IsVersion() && other.IsVersion():
		if c := compareTriplet(v, other); c != 0 {
			return c
		}
		switch {
		case v.Pre == "" && other.Pre == "":
			return 0
		case v.Pre == "":
			return 1 // stable beats prerelease of the same version
		case other.Pre == "":
			return -1
		case v.Pre == other.Pre:
			return 0
		default:
			return strings.Compare(v.Pre, other.Pre)
		}
	case v.IsVersion():
		return 1 // an exact version is never older than main/unknown
	case other.IsVersion():
		return -1
	default:
		return 0
	}
}

func compareTriplet(a, b ReportedVersion) int {
	if a.Major != b.Major {
		return sign(a.Major - b.Major)
	}
	if a.Minor != b.Minor {
		return sign(a.Minor - b.Minor)
	}
	return sign(a.Patch - b.Patch)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// versionPattern matches an optional leading v followed by X.Y.Z with an optional
// semver-ish prerelease suffix. The optional v makes "**v3.4.0**" in prose match
// (a plain \b before the digit would fail because both v and a digit are word
// characters). It intentionally does not try to validate the full semver grammar:
// the goal is a deterministic bucket for report text, not a semver library.
var versionPattern = regexp.MustCompile(`\b(?:v)?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z][0-9A-Za-z.-]*))?\b`)

// ParseReportedVersion reads a version string the way a maintainer would: it
// strips the common product prefixes ("gentle-ai", a leading "v", "version",
// "public release source"), takes the first X.Y.Z triplet, and buckets the
// remainder. "build from main" style reports are recognized as ChannelMain.
func ParseReportedVersion(raw string) ReportedVersion {
	out := ReportedVersion{Raw: strings.TrimSpace(raw)}
	normalized := normalizeVersionText(out.Raw)
	if matched := versionPattern.FindStringSubmatch(normalized); matched != nil {
		out.Channel = ChannelStable
		out.Major = mustAtoi(matched[1])
		out.Minor = mustAtoi(matched[2])
		out.Patch = mustAtoi(matched[3])
		out.Pre = matched[4]
		if out.Pre != "" {
			out.Channel = ChannelPrerelease
		}
		return out
	}
	if strings.Contains(normalized, "main") {
		out.Channel = ChannelMain
		return out
	}
	out.Channel = ChannelUnknown
	return out
}

// normalizeVersionText strips common wrapper text so the first triplet search
// agrees with what a human would read as the version.
func normalizeVersionText(raw string) string {
	s := strings.ToLower(raw)
	for _, prefix := range []string{
		"public release source",
		"build from",
		"release channel:",
		"gentle-ai",
		"gentle ai",
		"version",
		"release",
		"v",
	} {
		s = strings.TrimPrefix(s, prefix+" ")
		s = strings.TrimPrefix(s, prefix)
	}
	return s
}

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		// Unreachable for a \d+ capture; keep the signature total.
		panic(fmt.Sprintf("triageevidence: non-digit captured as %q", s))
	}
	return n
}
