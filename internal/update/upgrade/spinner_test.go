package upgrade

import (
	"bytes"
	"strings"
	"testing"
)

// TestSpinner_FinishSuccess verifies that Finish(true) writes the ✓ success icon.
func TestSpinner_FinishSuccess(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Upgrading engram")
	s.Finish(true)

	got := buf.String()
	if !strings.Contains(got, "✓") {
		t.Errorf("Finish(true) output = %q, want output containing '✓'", got)
	}
}

// TestSpinner_FinishFailure verifies that Finish(false) writes the ✗ failure icon.
func TestSpinner_FinishFailure(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Upgrading engram")
	s.Finish(false)

	got := buf.String()
	if !strings.Contains(got, "✗") {
		t.Errorf("Finish(false) output = %q, want output containing '✗'", got)
	}
}

// TestSpinner_FinishSkipped verifies that FinishSkipped writes a skip marker
// (-- or ⊘) instead of the failure marker (✗).
//
// RED: This test must fail before the fix because FinishSkipped does not exist yet.
func TestSpinner_FinishSkipped(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Upgrading gentle-ai")
	s.FinishSkipped()

	got := buf.String()

	// Must NOT show the failure marker.
	if strings.Contains(got, "✗") {
		t.Errorf("FinishSkipped() output = %q, must NOT contain '✗' (failure marker)", got)
	}

	// Must show a skip marker — either "--" or "⊘".
	if !strings.Contains(got, "--") && !strings.Contains(got, "⊘") {
		t.Errorf("FinishSkipped() output = %q, want skip marker '--' or '⊘'", got)
	}
}

// TestSpinner_FinishSkipped_NotSuccess verifies FinishSkipped does not
// write the success icon (✓) either.
func TestSpinner_FinishSkipped_NotSuccess(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "Upgrading gentle-ai")
	s.FinishSkipped()

	got := buf.String()
	if strings.Contains(got, "✓") {
		t.Errorf("FinishSkipped() output = %q, must NOT contain '✓' (success marker)", got)
	}
}
