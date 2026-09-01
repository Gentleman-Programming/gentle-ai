package sddstatus

import (
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// This file mirrors internal/cli's review-mode home fixtures for the SDD
// status surface. internal/sddstatus resolves the kill switch from the
// user's real home directory, so without these helpers a test's verdict
// depends on whatever Gentle AI install state the machine running it happens
// to carry: green on a developer box that once enabled reviews, red on a
// clean CI runner. Pinning the home directory makes the precondition part of
// the fixture instead of part of the environment.

func reviewModeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// reviewEnabledHome is reviewModeHome for a user who opted in. Receipt-driven
// development is off until someone explicitly enables it, so a test whose
// subject is the review lifecycle -- rather than the switch itself -- has to
// opt in the way a real user does before an offer, a gate, or a review will
// exist at all. It writes the same explicit global "on" that
// `gentle-ai review mode enable` persists, rather than reaching past the
// switch, so these fixtures keep exercising the resolution path they are
// meant to run through.
//
// The opinion lives in the user's home directory, which is process-wide state
// reached through t.Setenv. Go forbids t.Setenv in a test that also calls
// t.Parallel, so a test that opts in cannot be parallel: there is no
// repository-scoped way to assert "on" (a clone may only ever assert "off").
func reviewEnabledHome(t *testing.T) string {
	t.Helper()
	home := reviewModeHome(t)
	recordedAt := time.Now().UTC()
	if err := state.Write(home, state.InstallState{
		RDDMode:           string(reviewtransaction.RDDModeOn),
		RDDModeRecordedAt: &recordedAt,
	}); err != nil {
		t.Fatalf("enable review mode for this test: %v", err)
	}
	return home
}
