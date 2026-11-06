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

func compactSevereFindingForTest(id string, class EvidenceClass) Finding {
	return Finding{
		ID: id, Lens: LensReliability, Location: "tracked.txt:1", Severity: "CRITICAL",
		Claim: "observable failure " + id, ProofRefs: []string{"defect reproduced " + id},
		EvidenceClass: class, CausalDisposition: CausalIntroduced,
	}
}

// completeAndReloadCompactReviewForTest persists CompleteReview through the
// store, so successor validation and a fresh load re-derive the same view.
func completeAndReloadCompactReviewForTest(t *testing.T, findings []Finding, refuter []EvidenceResult) (CompactState, CompactReviewView) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "deterministic-refuter"))
	classifications := make([]FindingEvidence, 0, len(findings))
	for _, finding := range findings {
		if isSevereSeverity(finding.Severity) {
			classifications = append(classifications, FindingEvidence{FindingID: finding.ID, Class: finding.EvidenceClass, Causality: finding.CausalDisposition, Proof: "defect reproduced"})
		}
	}
	completed, record := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: LensReliability, Findings: findings, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: classifications, RefuterOutcomes: refuter,
	})
	if _, err := store.Replace(record.Revision, "review/complete-review", completed); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	view, err := reloaded.State.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	return reloaded.State, view
}

// TestCompactReviewAppliesRefuterVerdictToDeterministicSevereFindings pins
// L20/S11: a severe deterministic finding the refuter answered is decided by
// that verdict exactly like an inferential one -- refuted drops it to an
// advisory, corroborated and inconclusive route it to the bounded correction.
func TestCompactReviewAppliesRefuterVerdictToDeterministicSevereFindings(t *testing.T) {
	for _, tt := range []struct {
		outcome   EvidenceOutcome
		wantState State
		wantFix   bool
	}{
		{outcome: OutcomeRefuted, wantState: StateValidating},
		{outcome: OutcomeCorroborated, wantState: StateCorrectionRequired, wantFix: true},
		{outcome: OutcomeInconclusive, wantState: StateCorrectionRequired, wantFix: true},
	} {
		t.Run(string(tt.outcome), func(t *testing.T) {
			finding := compactSevereFindingForTest("R3-001", EvidenceDeterministic)
			state, view := completeAndReloadCompactReviewForTest(t, []Finding{finding}, []EvidenceResult{{FindingID: finding.ID, Outcome: tt.outcome, Proof: "probe: go test ./... -> baseline already fails"}})
			if state.State != tt.wantState || view.Outcomes[finding.ID] != tt.outcome || (len(view.FixFindingIDs) == 1) != tt.wantFix {
				t.Fatalf("deterministic finding with %s refuter verdict = state %q, outcome %q, fixes %v", tt.outcome, state.State, view.Outcomes[finding.ID], view.FixFindingIDs)
			}
			if len(view.RefuterOutcomes) != 1 || view.RefuterOutcomes[0].FindingID != finding.ID {
				t.Fatalf("refuter outcomes = %#v, want the one admitted verdict", view.RefuterOutcomes)
			}
		})
	}
}

// TestCompactReviewKeepsDeterministicFindingsWithoutRefuterCorroborated is the
// compatibility PRESERVE: an authority admitted before deterministic findings
// reached the refuter -- no refuter at all, or a batch that answered only the
// inferential findings -- still loads, replays, and closes with every
// deterministic severe finding corroborated and correction-bound.
func TestCompactReviewKeepsDeterministicFindingsWithoutRefuterCorroborated(t *testing.T) {
	deterministic := compactSevereFindingForTest("R3-001", EvidenceDeterministic)
	inferential := compactSevereFindingForTest("R3-002", EvidenceInferential)
	for _, tt := range []struct {
		name     string
		findings []Finding
		refuter  []EvidenceResult
		wantFix  []string
	}{
		{name: "no refuter", findings: []Finding{deterministic}, wantFix: []string{"R3-001"}},
		{name: "inferential-only batch", findings: []Finding{deterministic, inferential},
			refuter: []EvidenceResult{{FindingID: inferential.ID, Outcome: OutcomeRefuted, Proof: "baseline already fails"}}, wantFix: []string{"R3-001"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state, view := completeAndReloadCompactReviewForTest(t, tt.findings, tt.refuter)
			if state.State != StateCorrectionRequired || view.Outcomes[deterministic.ID] != OutcomeCorroborated || strings.Join(view.FixFindingIDs, ",") != strings.Join(tt.wantFix, ",") {
				t.Fatalf("historical deterministic route = state %q, outcomes %#v, fixes %v", state.State, view.Outcomes, view.FixFindingIDs)
			}
		})
	}
}

// TestCompactRefuterNeverAnswersNonSevereOrInsufficientFindings is the
// routing PRESERVE: a refuter result naming a non-severe finding or a severe
// finding with insufficient evidence never becomes review semantics. The
// capture merge does not derive the view (the CLI admission only accepts
// issued claims), so the refusal is observed where authority is read: the
// record no longer loads.
func TestCompactRefuterNeverAnswersNonSevereOrInsufficientFindings(t *testing.T) {
	warning := compactSevereFindingForTest("R3-001", "")
	warning.Severity, warning.EvidenceClass, warning.CausalDisposition = "WARNING", "", ""
	insufficient := compactSevereFindingForTest("R3-002", EvidenceInsufficient)
	for _, finding := range []Finding{warning, insufficient} {
		t.Run(finding.Severity+"/"+string(finding.EvidenceClass), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			state, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "refuter-scope"))
			captureCompactLens(t, store, state, 0, finding)
			record := requireCompactRoleCount(t, store, 1)
			payload, err := json.Marshal(compactAdmittedRefuterValue{Results: []EvidenceResult{{FindingID: finding.ID, Outcome: OutcomeRefuted, Proof: "not reproduced"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.CaptureAdmittedRefuterResult(t.Context(), CompactAdmittedRefuterResultRequest{
				ExpectedRevision: record.State.CapturePhaseRevision, TargetIdentity: record.State.InitialSnapshot.Identity,
				RequestHash: hash("f"), Payload: payload,
			}); err != nil {
				return
			}
			if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "refuter result does not match") {
				t.Fatalf("authority with a refuter result for a %s %s finding loaded: %v", finding.Severity, finding.EvidenceClass, err)
			}
		})
	}
}
