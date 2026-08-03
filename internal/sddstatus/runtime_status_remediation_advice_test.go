package sddstatus

import (
	"context"
	"strings"
	"testing"
)

func TestActiveAttemptBlockerUsesOnlyOpaqueCompactContinuation(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "runtime-self-remediation"})
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "blocked(active_attempt)") || !strings.Contains(reasons, "opaque settle token") {
		t.Fatalf("active bound attempt published no compact blocker: %v", status.BlockedReasons)
	}
	for _, fragment := range []string{
		fixture.binding.Revision,
		fixture.lineage,
		fixture.failedEvidence,
		"sdd-attempt finish",
	} {
		if strings.Contains(reasons, fragment) {
			t.Fatalf("active bound attempt blocker leaked %s:\n%s", fragment, reasons)
		}
	}
}

// TestActiveAttemptBlockerStaysShortWithoutABinding keeps the addition
// proportional: an unbound attempt owes no review obligation, so naming
// remediation flags there would advertise a route that does not apply.
func TestActiveAttemptBlockerStaysShortWithoutABinding(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "unbound-active", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "unbound-active")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-unbound-active", WorkUnit: "apply-auth",
		EvidenceGoal: "prove the auth runtime", MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "unbound-active"})
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "blocked(active_attempt)") {
		t.Fatalf("active unbound attempt published no compact blocker: %v", status.BlockedReasons)
	}
	if strings.Contains(reasons, "--successor-lineage") {
		t.Fatalf("unbound active attempt advertised remediation flags it cannot use:\n%s", reasons)
	}
}
