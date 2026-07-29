package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestActiveAttemptBlockerAdvertisesTheCompleteFlagSet closes the second half
// of the discoverability gap decode2 fell into. The active-attempt blocker is
// the surface an orchestrator reads before it decides how to close a charged
// attempt, and it used to advertise only the six ordinary finish fields. For a
// BOUND attempt whose candidate moved that flag set is incomplete: the finish
// is refused until it also carries the remediation trio. Advertising the short
// set is what routed the reporter into a refusal that named nothing.
//
// The blocker has to name the trio with the values the ledger already holds,
// and say the unobvious part out loud: the bound lineage is itself the
// successor once the corrected candidate is approved on it.
func TestActiveAttemptBlockerAdvertisesTheCompleteFlagSet(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "runtime-self-remediation"})
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "sdd-attempt finish") {
		t.Fatalf("active bound attempt published no finish blocker: %v", status.BlockedReasons)
	}
	for _, fragment := range []string{
		"--expected-binding-revision " + quoteForBlocker(fixture.binding.Revision),
		"--successor-lineage " + quoteForBlocker(fixture.lineage),
		"--remediates-evidence-revision " + quoteForBlocker(fixture.failedEvidence),
	} {
		if !strings.Contains(reasons, fragment) {
			t.Fatalf("active bound attempt blocker omits %s:\n%s", fragment, reasons)
		}
	}
	if !strings.Contains(reasons, "the bound lineage is itself the successor") {
		t.Fatalf("blocker never says the bound lineage is an acceptable successor:\n%s", reasons)
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
	if !strings.Contains(reasons, "sdd-attempt finish") {
		t.Fatalf("active unbound attempt published no finish blocker: %v", status.BlockedReasons)
	}
	if strings.Contains(reasons, "--successor-lineage") {
		t.Fatalf("unbound active attempt advertised remediation flags it cannot use:\n%s", reasons)
	}
}

func quoteForBlocker(value string) string { return "\"" + value + "\"" }
