package agentbuilder

import (
	"errors"
	"strings"
	"testing"
)

// TestRunRestorers_JoinsFailures pins that restore failures are never
// swallowed: they surface joined onto the uninstall error, restorers only run
// on failure, and an empty restorer list is a no-op.
func TestRunRestorers_JoinsFailures(t *testing.T) {
	base := errors.New("save registry: permission denied")
	failing := func() error { return errors.New("restore: disk unavailable") }
	ok := func() error { return nil }

	got := runRestorers(base, []func() error{ok, failing})
	if got == nil || !strings.Contains(got.Error(), "save registry") || !strings.Contains(got.Error(), "restore: disk unavailable") {
		t.Fatalf("expected joined save+restore errors, got: %v", got)
	}

	if ran := false; runRestorers(nil, []func() error{func() error { ran = true; return nil }}) != nil || ran {
		t.Fatal("restorers must not run when err is nil")
	}

	if got := runRestorers(base, nil); got != base {
		t.Fatalf("no restorers should return the original error, got: %v", got)
	}
}
