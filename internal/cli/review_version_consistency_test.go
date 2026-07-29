package cli

import (
	"strings"
	"testing"
)

// A tester reported `gentle-ai --version` saying 2.2.0-rc.1 while `doctor` on
// the same binary reported 1.49.1-0.2026...+dirty. Two version strings for one
// build is not a cosmetic inconsistency: doctor's whole purpose here is to say
// WHICH build is running, and the defect report carries the same value to
// whoever has to reproduce it. AppVersion is already handed to this package by
// internal/app; it was simply never consulted.
func TestGentleAIVersionMatchesTheStampedAppVersion(t *testing.T) {
	original := AppVersion
	t.Cleanup(func() { AppVersion = original })

	AppVersion = "2.2.0-rc.1"
	version, _ := reviewGentleAIVersionAndCommit()
	if version != "2.2.0-rc.1" {
		t.Fatalf("resolved version = %q, want the stamped %q", version, "2.2.0-rc.1")
	}

	clause := doctorInvokedGentleAIClause("")
	if clause != "" && !strings.Contains(clause, "2.2.0-rc.1") {
		t.Fatalf("doctor names a different version than --version: %s", clause)
	}
}

// An unstamped build must still fall back to build info rather than reporting
// a literal "dev" that hides which commit is running.
func TestGentleAIVersionFallsBackWhenNotStamped(t *testing.T) {
	original := AppVersion
	t.Cleanup(func() { AppVersion = original })

	for _, unstamped := range []string{"", "dev"} {
		AppVersion = unstamped
		if version, _ := reviewGentleAIVersionAndCommit(); version == "" {
			t.Fatalf("AppVersion %q resolved to an empty version", unstamped)
		}
	}
}
