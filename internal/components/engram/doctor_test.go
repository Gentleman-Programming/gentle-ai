package engram

import (
	"context"
	"strings"
	"testing"
)

// TestParseStrictSemver exercises the strict semver parser used by
// CheckEngramMCPVersion. Strict means exactly three dot-separated non-empty
// numeric segments with no leading "v" or surrounding text — a binary that
// prints "engram 1.5.0" must NOT parse (the doctor relies on this to flag
// unparseable version output).
func TestParseStrictSemver(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantMajor int
		wantMinor int
		wantPatch int
		wantOK    bool
	}{
		{name: "valid_three_segment", raw: "1.5.0", wantMajor: 1, wantMinor: 5, wantPatch: 0, wantOK: true},
		{name: "valid_zero_zero_zero", raw: "0.0.0", wantMajor: 0, wantMinor: 0, wantPatch: 0, wantOK: true},
		{name: "valid_large_numbers", raw: "12.345.6789", wantMajor: 12, wantMinor: 345, wantPatch: 6789, wantOK: true},
		{name: "invalid_two_segment", raw: "1.5", wantOK: false},
		{name: "invalid_leading_v", raw: "v1.5.0", wantOK: false},
		{name: "invalid_with_prefix", raw: "engram 1.5.0", wantOK: false},
		{name: "invalid_empty", raw: "", wantOK: false},
		{name: "invalid_whitespace", raw: "   ", wantOK: false},
		{name: "invalid_non_numeric", raw: "1.5.x", wantOK: false},
		{name: "invalid_extra_segment", raw: "1.5.0.1", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, ok := parseStrictSemver(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("parseStrictSemver(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if major != tt.wantMajor || minor != tt.wantMinor || patch != tt.wantPatch {
				t.Fatalf("parseStrictSemver(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.raw, major, minor, patch, tt.wantMajor, tt.wantMinor, tt.wantPatch)
			}
		})
	}
}

// withThresholdForTest swaps MinEngramVersionForHealthyLifecycle for the
// duration of the test. Returns the restore function for explicit t.Cleanup.
func withThresholdForTest(t *testing.T, value string) {
	t.Helper()
	orig := MinEngramVersionForHealthyLifecycle
	MinEngramVersionForHealthyLifecycle = value
	t.Cleanup(func() { MinEngramVersionForHealthyLifecycle = orig })
}

// TestCheckEngramMCPVersion_VersionBelowThreshold covers S5: when the parsed
// version is strictly below MinEngramVersionForHealthyLifecycle, the check
// returns a WARN whose Detail mentions Gentleman-Programming/gentle-ai#1019
// and whose Remedy names an actionable upgrade command.
func TestCheckEngramMCPVersion_VersionBelowThreshold(t *testing.T) {
	withThresholdForTest(t, "1.5.0")
	CountVersionCallsForTest(t, "engram 0.5.0")

	got := CheckEngramMCPVersion(context.Background())

	if got.Name != "engram-mcp-lifecycle-version" {
		t.Fatalf("Name = %q, want %q", got.Name, "engram-mcp-lifecycle-version")
	}
	if got.Status != CheckStatusWarn {
		t.Errorf("Status = %q, want %q (got Detail=%q)", got.Status, CheckStatusWarn, got.Detail)
	}
	if !strings.Contains(got.Detail, "0.5.0") {
		t.Errorf("Detail should mention installed version 0.5.0; got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "1.5.0") {
		t.Errorf("Detail should mention threshold 1.5.0; got %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "Gentleman-Programming/gentle-ai#1019") {
		t.Errorf("Detail must reference #1019; got %q", got.Detail)
	}
	if got.Remedy == "" {
		t.Error("Remedy must carry an actionable upgrade command")
	}
	if !strings.Contains(got.Remedy, "upgrade") && !strings.Contains(got.Remedy, "install") {
		t.Errorf("Remedy should guide the user to upgrade/install; got %q", got.Remedy)
	}
}

// TestCheckEngramMCPVersion_VersionAtOrAboveThreshold covers S7: when the
// parsed version is at or above the threshold, the check returns PASS with
// no WARN. The version gate is inclusive at the floor.
func TestCheckEngramMCPVersion_VersionAtOrAboveThreshold(t *testing.T) {
	withThresholdForTest(t, "1.5.0")
	CountVersionCallsForTest(t, "engram 1.5.0")

	got := CheckEngramMCPVersion(context.Background())

	if got.Status != CheckStatusPass {
		t.Errorf("Status = %q, want %q (Detail=%q)", got.Status, CheckStatusPass, got.Detail)
	}
	if strings.Contains(got.Detail, "#1019") {
		t.Errorf("PASS finding must not reference #1019; got %q", got.Detail)
	}

	// Above the floor — also PASS.
	withThresholdForTest(t, "1.5.0")
	CountVersionCallsForTest(t, "engram 2.0.0")

	got = CheckEngramMCPVersion(context.Background())
	if got.Status != CheckStatusPass {
		t.Errorf("Status for 2.0.0 = %q, want %q", got.Status, CheckStatusPass)
	}
}

// TestCheckEngramMCPVersion_DefaultThresholdDormant covers S8: the shipped
// constant is "0.0.0", so no installed version triggers a WARN. This is the
// safety net while the upstream engram fix is still in flight.
func TestCheckEngramMCPVersion_DefaultThresholdDormant(t *testing.T) {
	// Force the shipped constant. Do NOT use withThresholdForTest — this test
	// guards the default value itself.
	if MinEngramVersionForHealthyLifecycle != "0.0.0" {
		t.Skipf("test guards default threshold; skip when override is active (current=%q)", MinEngramVersionForHealthyLifecycle)
	}

	// Even an ancient version must NOT warn when the threshold is "0.0.0".
	CountVersionCallsForTest(t, "engram 0.0.1")
	got := CheckEngramMCPVersion(context.Background())
	if got.Status != CheckStatusPass {
		t.Errorf("dormant threshold with version 0.0.1: Status = %q, want pass; Detail=%q",
			got.Status, got.Detail)
	}

	CountVersionCallsForTest(t, "engram dev")
	got = CheckEngramMCPVersion(context.Background())
	if got.Status != CheckStatusPass {
		t.Errorf("dormant threshold with unparseable version: Status = %q, want pass; Detail=%q",
			got.Status, got.Detail)
	}
}

// TestCheckEngramMCPVersion_ParseFailureReturnsWarn covers the R5 parse-warning
// contract: when VerifyVersionCommand returns output we cannot parse, the
// check emits WARN — never FAIL. A FAIL would brick the doctor exit code on
// unparseable binary output (e.g. a future "engram dev" string).
func TestCheckEngramMCPVersion_ParseFailureReturnsWarn(t *testing.T) {
	withThresholdForTest(t, "1.5.0")
	CountVersionCallsForTest(t, "engram dev snapshot 2026-07-21")

	got := CheckEngramMCPVersion(context.Background())

	if got.Status != CheckStatusWarn {
		t.Errorf("parse-failure Status = %q, want %q (Detail=%q)",
			got.Status, CheckStatusWarn, got.Detail)
	}
	if got.Status == CheckStatusFail {
		t.Fatal("parse failure must NEVER escalate to FAIL (R5 + R7)")
	}
	if got.Detail == "" {
		t.Error("parse-failure Detail must explain why the version is unparseable")
	}
}

// TestCheckEngramMCPVersion_VersionCommandFailure covers the case where the
// version probe itself fails (binary missing on PATH or exec error). The
// check MUST emit WARN — never FAIL — for the same reason as parse failures:
// unparseable/inaccessible binaries must not brick the doctor exit code.
func TestCheckEngramMCPVersion_VersionCommandFailure(t *testing.T) {
	withThresholdForTest(t, "1.5.0")

	orig := runVersionCommand
	t.Cleanup(func() { runVersionCommand = orig })
	runVersionCommand = func(context.Context, string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	}

	got := CheckEngramMCPVersion(context.Background())

	if got.Status != CheckStatusWarn {
		t.Errorf("version-command-failure Status = %q, want %q", got.Status, CheckStatusWarn)
	}
	if got.Status == CheckStatusFail {
		t.Fatal("version-command failure must NEVER escalate to FAIL")
	}
}
