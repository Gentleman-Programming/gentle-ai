//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestActiveAttemptGuidanceUsesOnlyOpaqueCompactContinuation keeps the original
// non-leak property while following the guidance to the channel it now belongs
// to. An active attempt stopped being a blocker in #2463 (status has no
// standing to refuse a launch compact acquire admits, and it cannot know
// whether its reader holds the token), so the continuation is published as
// runtime instructions instead. What it may say is unchanged: the opaque token
// and nothing else.
func TestActiveAttemptGuidanceUsesOnlyOpaqueCompactContinuation(t *testing.T) {
	fixture := newRuntimeSelfRemediationFixture(t)
	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "runtime-self-remediation", IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	guidance := activeAttemptGuidance(t, status)
	if !strings.Contains(guidance, "--token ") {
		t.Fatalf("active bound attempt published no compact continuation:\n%s", guidance)
	}
	if reasons := strings.Join(status.BlockedReasons, "\n"); strings.Contains(reasons, "active_attempt") {
		t.Fatalf("an attempt compact acquire admits was published as a blocker: %v", status.BlockedReasons)
	}
	for _, fragment := range []string{
		fixture.binding.Revision,
		fixture.lineage,
		fixture.failedEvidence,
		"sdd-attempt finish",
	} {
		if strings.Contains(guidance, fragment) {
			t.Fatalf("active bound attempt guidance leaked %s:\n%s", fragment, guidance)
		}
	}
}

// TestActiveAttemptGuidanceStaysShortWithoutABinding keeps the addition
// proportional: an unbound attempt owes no review obligation, so naming
// remediation flags there would advertise a route that does not apply.
func TestActiveAttemptGuidanceStaysShortWithoutABinding(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "unbound-active", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "unbound-active")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-unbound-active", WorkUnit: "apply-auth",
		EvidenceGoal: "prove the auth runtime", MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "unbound-active", IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	guidance := activeAttemptGuidance(t, status)
	if strings.Contains(guidance, "--successor-lineage") {
		t.Fatalf("unbound active attempt advertised remediation flags it cannot use:\n%s", guidance)
	}
}

// activeAttemptGuidance returns the single apply instruction that names a live
// attempt's continuation. Scoping to that line keeps these assertions about
// what the readiness predicate published, not about the unconditional
// acquire/settle boilerplate that surrounds it.
func activeAttemptGuidance(t *testing.T, status Status) string {
	t.Helper()
	if status.PhaseInstructions == nil {
		t.Fatal("status carries no phase instructions")
	}
	for _, instruction := range status.PhaseInstructions.Apply {
		if strings.HasPrefix(instruction, "An attempt is already active") {
			return instruction
		}
	}
	t.Fatalf("apply instructions name no live attempt continuation:\n%s", strings.Join(status.PhaseInstructions.Apply, "\n"))
	return ""
}
