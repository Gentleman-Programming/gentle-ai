package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestActiveAttemptGuidanceStaysShortWithoutRemediationNeed keeps the guidance
// proportional: a directly continuable attempt must not advertise remediation
// flags that do not apply.
func TestActiveAttemptGuidanceStaysShortWithoutRemediationNeed(t *testing.T) {
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
		t.Fatalf("directly continuable active attempt advertised remediation flags it cannot use:\n%s", guidance)
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
