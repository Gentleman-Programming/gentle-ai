package reviewtransaction

import (
	"encoding/json"
	"strings"
	"testing"
)

// requestContextFixtureState is a fully literal compact state, so its capture
// phase preimage depends on nothing but the fields written here.
func requestContextFixtureState() CompactState {
	paths := []string{"internal/a.go"}
	snapshot := Snapshot{
		Identity: "sha256:" + strings.Repeat("1", 64), BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40),
		PathsDigest: "sha256:" + strings.Repeat("3", 64), Paths: paths,
	}
	return CompactState{
		Schema: CompactStateSchema, LineageID: "request-context-fixture", Generation: 1, State: StateReviewing,
		InitialSnapshot: snapshot, CurrentSnapshot: snapshot, GenesisPaths: paths,
		PolicyHash: "sha256:" + strings.Repeat("4", 64), RiskLevel: RiskMedium, SelectedLenses: []string{LensReliability},
		OriginalChangedLines: 3, CorrectionBudget: 2, CorrectionBudgetPolicy: CorrectionBudgetPolicyFloorTwo,
		FixFindingIDs: []string{}, FixDeltaHash: EmptyFixDeltaHash,
	}
}

// TestCompactStateWithoutRequestContextKeepsItsBytes pins the PRESERVE
// contract: authority started without --request-context serializes and derives
// its capture phase exactly as it did before the field existed, so older
// binaries keep reading it and admitted subjects keep their identity.
func TestCompactStateWithoutRequestContextKeepsItsBytes(t *testing.T) {
	state := requestContextFixtureState()
	phase, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	// Captured before the request context fields were added.
	const historical = "sha256:29aaf1517a5ef6d67e5e40c1942977242cbc069c21aa924fd697a35c3a80813f"
	if phase != historical {
		t.Fatalf("capture phase without request context = %s, want the historical %s", phase, historical)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := json.Marshal(CompactAtomicStartBinding{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload)+string(binding), "request_context") {
		t.Fatalf("absent request context leaked into persisted JSON:\n%s\n%s", payload, binding)
	}
}

// TestFreezeRequestContextBindsHashContentAndCapturePhase is S10's frozen
// input: the verbatim request is stored with its hash, both are checked
// together like the frozen policy, and the capture phase moves with it.
func TestFreezeRequestContextBindsHashContentAndCapturePhase(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactFixtureStateForTarget(t, repo, "request-context-freeze", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	before, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	const request = "S1 budget set --year rejects years before 2000.\n"
	if err := state.FreezeRequestContext(request); err != nil {
		t.Fatal(err)
	}
	if state.FrozenRequestContext == nil || *state.FrozenRequestContext != request ||
		state.RequestContextHash != compactPolicyContentHash(request) {
		t.Fatalf("frozen request context = %q / %q", state.RequestContextHash, deref(state.FrozenRequestContext))
	}
	if state.CapturePhaseRevision == "" || state.CapturePhaseRevision == before {
		t.Fatalf("capture phase did not move with the request context: %s", state.CapturePhaseRevision)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("state with frozen request context: %v", err)
	}
	if err := state.FreezeRequestContext(request); err == nil {
		t.Fatal("a frozen request context was replaced")
	}
	tampered := state
	other := "S1 is out of scope.\n"
	tampered.FrozenRequestContext = &other
	if err := tampered.Validate(); err == nil {
		t.Fatal("request context that does not match its hash validated")
	}
	tampered = state
	tampered.FrozenRequestContext = nil
	if err := tampered.Validate(); err == nil {
		t.Fatal("request context hash without content validated")
	}
}

// TestFreezeAgentEscalationBindsCapturePhaseAndRequiresHigh is S14's frozen
// input: an agent escalation is frozen once, only on a high authority, and
// the capture phase moves with it so every artifact subject commits to it.
func TestFreezeAgentEscalationBindsCapturePhaseAndRequiresHigh(t *testing.T) {
	escalation := CompactAgentEscalation{Item: 2, Reason: "rewrites how service tokens are parsed"}
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	medium := newCompactFixtureStateForTarget(t, repo, "agent-escalation-freeze", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	state := medium
	if err := medium.FreezeAgentEscalation(escalation); err == nil {
		t.Fatal("an escalation froze on a medium authority")
	}
	state.RiskLevel, state.SelectedLenses = RiskHigh, append([]string(nil), supportedLenses...)
	before, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.FreezeAgentEscalation(escalation); err != nil {
		t.Fatal(err)
	}
	if state.AgentEscalation == nil || *state.AgentEscalation != escalation {
		t.Fatalf("frozen escalation = %#v", state.AgentEscalation)
	}
	if state.CapturePhaseRevision == "" || state.CapturePhaseRevision == before {
		t.Fatalf("capture phase did not move with the escalation: %s", state.CapturePhaseRevision)
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("state with frozen escalation: %v", err)
	}
	if err := state.FreezeAgentEscalation(escalation); err == nil {
		t.Fatal("a frozen escalation was replaced")
	}
	for name, tampered := range map[string]CompactAgentEscalation{
		"item out of range": {Item: 7, Reason: escalation.Reason},
		"blank reason":      {Item: 2, Reason: " "},
		"reason too long":   {Item: 2, Reason: strings.Repeat("x", AgentEscalationReasonMax+1)},
	} {
		copy := state
		copy.AgentEscalation = &tampered
		if err := copy.Validate(); err == nil {
			t.Fatalf("escalation with %s validated", name)
		}
	}
	lowered := state
	lowered.RiskLevel, lowered.SelectedLenses = RiskMedium, []string{LensReliability}
	if err := lowered.Validate(); err == nil {
		t.Fatal("an escalated authority below high validated")
	}
}

func deref(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}
