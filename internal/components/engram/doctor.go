package engram

import (
	"context"
	"fmt"
	"strings"
)

// MinEngramVersionForHealthyLifecycle is the lowest engram binary version
// whose MCP stdio lifecycle survives Claude Code's handshake timeout window.
//
// TODO: set this to the engram release that first shipped the
// `notifications/initialized` notification once that release is published.
// Until then the default is "0.0.0" so no warning is emitted (spec R8 / S8).
//
// Declared as a `var` so tests can override the threshold via
// withThresholdForTest. In production the value is set once at init and
// never mutated.
var MinEngramVersionForHealthyLifecycle = "0.0.0"

// mcpLifecycleVersionCheckName is the stable name surfaced to
// cli/doctor.go when aggregating findings (spec R5).
const mcpLifecycleVersionCheckName = "engram-mcp-lifecycle-version"

// mcpLifecycleIssueRef is the GitHub issue that motivated this check.
// Surfaced in the doctor detail so operators can grep triage from one place.
const mcpLifecycleIssueRef = "Gentleman-Programming/gentle-ai#1019"

// CheckStatus is the doctor finding status emitted by CheckEngramMCPVersion.
// The underlying values mirror internal/cli/doctor.go's CheckStatus 1-1 so
// the cli package can convert via cli.CheckStatus(c.Status) without
// creating an import cycle.
type CheckStatus string

const (
	CheckStatusPass CheckStatus = "pass"
	CheckStatusWarn CheckStatus = "warn"
	CheckStatusFail CheckStatus = "fail"
)

// MCPVersionCheck is the doctor finding produced by CheckEngramMCPVersion.
// It carries the full payload the cli/doctor.go aggregator needs:
// Name, Status, Detail, and Remedy. Status is a string-typed enum so the
// conversion to cli.CheckStatus is a one-line cast.
type MCPVersionCheck struct {
	Name   string
	Status CheckStatus
	Detail string
	Remedy string
}

// CheckEngramMCPVersion probes the installed engram binary version via the
// existing VerifyVersionCommand seam (verify.go:31-37) and reports a doctor
// finding for the MCP handshake lifecycle.
//
// Behaviour matrix (spec R5 + R7 + R8):
//
//	parse failure OR version command failure → WARN (never FAIL)
//	installed < MinEngramVersionForHealthyLifecycle   → WARN (with #1019 + remedy)
//	installed >= MinEngramVersionForHealthyLifecycle  → PASS
//
// The threshold default is "0.0.0" so the check is dormant in production
// until the upstream engram fix is published and the constant is flipped.
func CheckEngramMCPVersion(ctx context.Context) MCPVersionCheck {
	// ctx is reserved for downstream seam reuse; VerifyVersionCommand (which
	// verify.go locks byte-identical) builds its own timeout context, so we
	// pass only "engram". When verify.go ever accepts ctx, drop the discard.
	_ = ctx
	raw, err := VerifyVersionCommand("engram")
	if err != nil {
		// Version command failed (binary missing, exec error, timeout).
		// Emit WARN — never FAIL — so an inaccessible binary cannot brick
		// the doctor exit code (spec R7).
		return MCPVersionCheck{
			Name:   mcpLifecycleVersionCheckName,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("could not read engram version: %v", err),
			Remedy: "Install engram (go install github.com/gentleman-programming/engram/cmd/engram@latest) " +
				"and ensure it is on $PATH so the MCP lifecycle version can be inspected.",
		}
	}

	thresholdMajor, thresholdMinor, thresholdPatch, thresholdOK := parseStrictSemver(MinEngramVersionForHealthyLifecycle)
	if !thresholdOK {
		// Defensive: a malformed threshold value should not produce FAIL.
		return MCPVersionCheck{
			Name:   mcpLifecycleVersionCheckName,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("MinEngramVersionForHealthyLifecycle=%q is not strict semver; comparison skipped (%s)",
				MinEngramVersionForHealthyLifecycle, mcpLifecycleIssueRef),
		}
	}

	// Dormant threshold ("0.0.0"): the check is silent per spec S8. Even an
	// unparseable version output is PASS because there is no comparison to
	// make — the operator has not flipped the switch yet, so we do not
	// surface parse warnings either.
	if thresholdMajor == 0 && thresholdMinor == 0 && thresholdPatch == 0 {
		return MCPVersionCheck{
			Name:   mcpLifecycleVersionCheckName,
			Status: CheckStatusPass,
			Detail: "engram-mcp-lifecycle-version check is dormant (threshold 0.0.0); set MinEngramVersionForHealthyLifecycle to the engram release that shipped notifications/initialized to enable it",
		}
	}

	installedMajor, installedMinor, installedPatch, ok := parseStrictSemver(extractSemverToken(raw))
	if !ok {
		return MCPVersionCheck{
			Name:   mcpLifecycleVersionCheckName,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("engram version output %q has no strict semver token (X.Y.Z); cannot compare against threshold %q (%s)",
				strings.TrimSpace(raw), MinEngramVersionForHealthyLifecycle, mcpLifecycleIssueRef),
			Remedy: "Update engram to a release that prints strict semver " +
				"(go install github.com/gentleman-programming/engram/cmd/engram@latest).",
		}
	}

	if semverLess(installedMajor, installedMinor, installedPatch,
		thresholdMajor, thresholdMinor, thresholdPatch) {
		return MCPVersionCheck{
			Name:   mcpLifecycleVersionCheckName,
			Status: CheckStatusWarn,
			Detail: fmt.Sprintf("engram %d.%d.%d is below the known-good threshold %s; "+
				"the MCP server may be terminated by Claude Code after the notifications/initialized handshake (%s)",
				installedMajor, installedMinor, installedPatch,
				MinEngramVersionForHealthyLifecycle, mcpLifecycleIssueRef),
			Remedy: fmt.Sprintf("Upgrade engram to %s or later: "+
				"go install github.com/gentleman-programming/engram/cmd/engram@v%s",
				MinEngramVersionForHealthyLifecycle, MinEngramVersionForHealthyLifecycle),
		}
	}

	return MCPVersionCheck{
		Name:   mcpLifecycleVersionCheckName,
		Status: CheckStatusPass,
		Detail: fmt.Sprintf("engram %d.%d.%d is at or above the known-good threshold %s",
			installedMajor, installedMinor, installedPatch, MinEngramVersionForHealthyLifecycle),
	}
}

// parseStrictSemver parses the strict semver string "X.Y.Z" into its three
// numeric components. Strict rules: exactly three dot-separated segments,
// each non-empty and composed only of ASCII digits. No leading "v", no
// surrounding prefix, no "engram 1.5.0"-style suffix. Empty / whitespace-
// only input returns ok=false.
//
// The doctor uses this to fail loud on unparseable version output (e.g. a
// binary that prints "engram dev") so the operator sees a WARN rather than
// silently getting a PASS on garbage. The caller is expected to pre-extract
// the X.Y.Z token from the raw `engram version` output — see
// extractSemverToken.
func parseStrictSemver(raw string) (major, minor, patch int, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, 0, 0, false
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}

	major, ok = parseStrictSemverSegment(parts[0])
	if !ok {
		return 0, 0, 0, false
	}
	minor, ok = parseStrictSemverSegment(parts[1])
	if !ok {
		return 0, 0, 0, false
	}
	patch, ok = parseStrictSemverSegment(parts[2])
	if !ok {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// extractSemverToken scans raw for the first whitespace-delimited token
// that parses as strict semver. The engram binary prints lines like
// "engram 1.5.0" or "engram v1.4.2 (commit deadbeef)"; this helper isolates
// the numeric triple so parseStrictSemver can stay strict.
//
// Returns "" when no candidate token parses — the doctor surfaces this as a
// WARN so the operator gets actionable feedback instead of a silent PASS.
func extractSemverToken(raw string) string {
	for _, field := range strings.Fields(raw) {
		if _, _, _, ok := parseStrictSemver(field); ok {
			return field
		}
	}
	return ""
}

func parseStrictSemverSegment(segment string) (int, bool) {
	if segment == "" {
		return 0, false
	}
	for i := 0; i < len(segment); i++ {
		if segment[i] < '0' || segment[i] > '9' {
			return 0, false
		}
	}
	n := 0
	for i := 0; i < len(segment); i++ {
		n = n*10 + int(segment[i]-'0')
	}
	return n, true
}

func semverLess(aMajor, aMinor, aPatch, bMajor, bMinor, bPatch int) bool {
	if aMajor != bMajor {
		return aMajor < bMajor
	}
	if aMinor != bMinor {
		return aMinor < bMinor
	}
	return aPatch < bPatch
}
