package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// decisionFindingFixture builds the same reviewing authority the escalated
// fixture uses: one CRITICAL finding classified insufficient/unknown with no
// refuter outcome. Completing that review is the canonical pause population.
func decisionFindingFixture(t *testing.T, repo, lineage string) (CompactState, CompactStore, CompactRecord) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) == 0 {
		t.Fatal("decision fixture unexpectedly selected no lenses")
	}
	state, store := startReviewingCompactFixture(t, repo, state)
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	finding := Finding{
		ID: "R3-001", Lens: state.SelectedLenses[0], Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "reviewer evidence remains inconclusive", ProofRefs: []string{"candidate inspection was inconclusive"},
	}
	results[0].Findings = []Finding{finding}
	paused, startedRecord := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults: results,
		Classifications: []FindingEvidence{{
			FindingID: finding.ID, Class: EvidenceInsufficient, Causality: CausalUnknown, Proof: "insufficient evidence",
		}},
		RefuterOutcomes: []EvidenceResult{},
	})
	return paused, store, startedRecord
}

func decisionEntry(decision, actor, reason, revision string) CompactDecisionEntry {
	return CompactDecisionEntry{Decision: decision, Actor: actor, Reason: reason, Revision: revision}
}

func decideSuccessor(paused CompactState, decision, actor, reason, revision string) CompactState {
	next := paused
	next.DecisionHistory = append(append([]CompactDecisionEntry{}, paused.DecisionHistory...), decisionEntry(decision, actor, reason, revision))
	next.DecisionEpoch = paused.DecisionEpoch + 1
	switch decision {
	case CompactDecisionContinue:
		setCompactStateExit(&next, StateReviewing)
		next.FixFindingIDs = []string{}
		next.Decision = nil
	case CompactDecisionStop:
		setCompactStateExit(&next, StateEscalated)
	}
	return next
}

func TestCompleteReviewUnresolvedEvidencePausesForDecision(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-pause")

	if paused.State != StateDecisionRequired {
		t.Fatalf("state = %q, want %q", paused.State, StateDecisionRequired)
	}
	decision := paused.Decision
	if decision == nil {
		t.Fatal("decision_required state must carry its frozen decision block")
	}
	if decision.Reason != "evidence_inconclusive" {
		t.Fatalf("decision reason = %q, want evidence_inconclusive", decision.Reason)
	}
	if decision.Cause != "unknown_causality" {
		t.Fatalf("decision cause = %q, want unknown_causality (same classification the escalated rendering would derive)", decision.Cause)
	}
	if !reflect.DeepEqual(decision.FindingIDs, []string{"R3-001"}) {
		t.Fatalf("decision finding_ids = %#v, want the unresolved finding", decision.FindingIDs)
	}
	for _, field := range []struct {
		name  string
		value []string
	}{
		{"attempted_evidence", decision.AttemptedEvidence},
		{"missing_evidence", decision.MissingEvidence},
	} {
		if len(field.value) == 0 {
			t.Fatalf("decision %s is empty, want the transition-time evidence attribution", field.name)
		}
	}
	for _, field := range []struct{ name, value string }{
		{"recommendation", decision.Recommendation},
		{"next_action", decision.NextAction},
		{"bounded_cost", decision.BoundedCost},
	} {
		if strings.TrimSpace(field.value) == "" {
			t.Fatalf("decision %s is empty", field.name)
		}
	}
	if !strings.Contains(decision.NextAction, "review decide") {
		t.Fatalf("decision next_action = %q, want the review decide continuation", decision.NextAction)
	}
	if paused.DecisionEpoch != 1 {
		t.Fatalf("decision epoch = %d, want 1 after the first pause", paused.DecisionEpoch)
	}
	wantHistory := []CompactDecisionEntry{{Decision: CompactDecisionPause, Actor: "review/complete-review", Reason: "evidence_inconclusive"}}
	if !reflect.DeepEqual(paused.DecisionHistory, wantHistory) {
		t.Fatalf("decision history = %#v, want the engine-authored pause entry %#v", paused.DecisionHistory, wantHistory)
	}
	if err := paused.Validate(); err != nil {
		t.Fatalf("paused state refuses validation: %v", err)
	}
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the paused state: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != StateDecisionRequired || record.State.DecisionEpoch != 1 {
		t.Fatalf("persisted pause = %q epoch %d, want decision_required epoch 1", record.State.State, record.State.DecisionEpoch)
	}
}

func TestDecisionEvidenceAttributesEachUnresolvedFinding(t *testing.T) {
	for _, withRefuter := range []bool{false, true} {
		view := CompactReviewView{Outcomes: map[string]EvidenceOutcome{"A": OutcomeInconclusive, "B": OutcomeInconclusive}, Classifications: map[string]FindingEvidence{
			"A": {FindingID: "A", Class: EvidenceInsufficient, Causality: CausalUnknown},
			"B": {FindingID: "B", Class: EvidenceInferential, Causality: CausalIntroduced},
		}}
		if withRefuter {
			view.RefuterOutcomes = []EvidenceResult{{FindingID: "B"}}
		}
		evidence := deriveCompactDecisionEvidence(view)
		want := []string{"causal_disposition:A", "concrete_evidence:A"}
		if !withRefuter {
			want = append(want, "refuter_outcome:B")
		} else {
			want = append(want, "conclusive_outcome:B")
		}
		if !reflect.DeepEqual(evidence.MissingEvidence, want) {
			t.Fatalf("refuter=%v missing=%v want=%v", withRefuter, evidence.MissingEvidence, want)
		}
	}
}

func TestDecisionEvidenceFallsBackOnlyWithoutSpecificFacet(t *testing.T) {
	view := CompactReviewView{Outcomes: map[string]EvidenceOutcome{"A": OutcomeInconclusive, "B": OutcomeInconclusive}, Classifications: map[string]FindingEvidence{
		"A": {FindingID: "A", Class: EvidenceInsufficient, Causality: CausalIntroduced},
	}}
	evidence := deriveCompactDecisionEvidence(view)
	want := []string{"concrete_evidence:A", "conclusive_outcome:B"}
	if !reflect.DeepEqual(evidence.MissingEvidence, want) {
		t.Fatalf("missing=%v want=%v", evidence.MissingEvidence, want)
	}
}

func TestCompleteReviewCleanPathStaysValidating(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "decision-clean")
	state, store := startReviewingCompactFixture(t, repo, state)
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	completed, _ := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{LensResults: results, RefuterOutcomes: []EvidenceResult{}})
	if completed.State != StateValidating {
		t.Fatalf("clean review state = %q, want %q", completed.State, StateValidating)
	}
	if completed.Decision != nil || completed.DecisionEpoch != 0 || len(completed.DecisionHistory) != 0 {
		t.Fatalf("clean review must stay decision-free: block=%#v epoch=%d history=%#v", completed.Decision, completed.DecisionEpoch, completed.DecisionHistory)
	}
}

func TestCompleteReviewSuccessorRejectsUnresolvedValidatingRoute(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, _, started := decisionFindingFixture(t, repo, "unresolved-validating-forgery")
	forged := paused
	setCompactStateExit(&forged, StateValidating)
	forged.Decision = nil
	forged.DecisionEpoch = started.State.DecisionEpoch
	forged.DecisionHistory = started.State.DecisionHistory
	if err := forged.Validate(); err != nil {
		t.Fatalf("forgery must be structurally valid: %v", err)
	}
	if err := validateCompactSuccessor(started.Revision, started.State, forged, "review/complete-review"); err == nil {
		t.Fatal("accepted unresolved review routed to validating")
	}
}

func TestCompleteReviewSuccessorRejectsForgedDecision(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "completion-forgery")
	previous := started.State
	if err := validateCompactSuccessor(started.Revision, previous, paused, "review/complete-review"); err != nil {
		view, viewErr := previous.CompactReviewView()
		t.Fatalf("legitimate pause: %v; viewErr=%v unresolved=%v expected=%#v actual=%#v", err, viewErr, compactReviewViewHasUnresolvedFindings(view), deriveCompactDecisionEvidence(view), paused.Decision)
	}
	for _, tc := range []struct {
		name   string
		change func(*CompactState)
	}{
		{"escalated", func(s *CompactState) {
			setCompactStateExit(s, StateEscalated)
			s.Decision = nil
			s.DecisionEpoch = 0
			s.DecisionHistory = nil
		}},
		{"unrelated question", func(s *CompactState) {
			altered := *s.Decision
			altered.Cause = "insufficient_evidence"
			s.Decision = &altered
		}},
		{"forged history", func(s *CompactState) {
			s.DecisionHistory = append(s.DecisionHistory, decisionEntry(CompactDecisionPause, CompactDecisionSystemActor, CompactDecisionReasonEvidenceInconclusive, ""))
			s.DecisionEpoch++
		}},
		{"forged epoch", func(s *CompactState) { s.DecisionEpoch++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := paused
			tc.change(&next)
			if err := validateCompactSuccessor(started.Revision, previous, next, "review/complete-review"); err == nil {
				t.Fatal("accepted forged completion")
			}
		})
	}
	// A re-paused lineage must keep its preceding human decisions intact.
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatal(err)
	}
}

func TestCorrectionStartPreservesDecisionJournal(t *testing.T) {
	repo := initSnapshotRepo(t)
	previous, _, record := correctionRequiredCompactAuthority(t, repo, "correction-decision-continuity")
	next := previous
	if err := next.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	if err := validateCompactSuccessor(record.Revision, previous, next, "review/begin-fix"); err != nil {
		t.Fatalf("legitimate correction: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*CompactState)
	}{
		{"epoch", func(s *CompactState) { s.DecisionEpoch++ }},
		{"history", func(s *CompactState) {
			s.DecisionEpoch++
			s.DecisionHistory = []CompactDecisionEntry{decisionEntry(CompactDecisionPause, CompactDecisionSystemActor, CompactDecisionReasonEvidenceInconclusive, "")}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forged := next
			tc.change(&forged)
			if err := validateCompactSuccessor(record.Revision, previous, forged, "review/begin-fix"); err == nil {
				t.Fatal("accepted decision mutation")
			}
		})
	}
}

func TestCompleteReviewPreservesUnownedAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, _, started := decisionFindingFixture(t, repo, "completion-unowned-fields")
	for _, tc := range []struct {
		name   string
		change func(*CompactState)
	}{
		{"result reopens", func(s *CompactState) { s.ResultReopens = []CompactResultReopen{{}} }},
		{"correction attempts", func(s *CompactState) { s.CorrectionAttempts = []CompactCorrectionAttempt{{}} }},
		{"cumulative lines", func(s *CompactState) { s.CumulativeCorrectionLines++ }},
		{"recovery", func(s *CompactState) { s.Recovery = &CompactRecoveryProvenance{} }},
		{"added paths", func(s *CompactState) { s.CorrectionAddedPaths = []string{"extra.txt"} }},
		{"invalidation reason", func(s *CompactState) { s.InvalidationReason = "forged" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := paused
			tc.change(&next)
			if err := validateCompactSuccessor(started.Revision, started.State, next, "review/complete-review"); err == nil {
				t.Fatal("accepted unowned completion mutation")
			}
		})
	}
}

func TestCorrectionCompletionsPreserveDecisionAuthority(t *testing.T) {
	repo := initSnapshotRepo(t)
	previous, fix := pendingCompactCorrection(t, repo, "completion-decision-continuity")
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: previous.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	for _, tc := range []struct {
		name, operation string
		complete        func(*CompactState) error
	}{
		{"complete-fix", "review/complete-fix", func(s *CompactState) error { return s.CompleteCorrection(fix, 1, validation) }},
		{"complete-correction-verification", "review/complete-correction-verification", func(s *CompactState) error { return s.CompleteCorrectionVerification(fix, 1, validation) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := previous
			if err := tc.complete(&next); err != nil {
				t.Fatal(err)
			}
			if err := validateCompactSuccessor(hash("decision-predecessor"), previous, next, tc.operation); err != nil {
				t.Fatalf("legitimate completion refused: %v", err)
			}
			for _, mutation := range []struct {
				name   string
				change func(*CompactState)
			}{
				{"epoch", func(s *CompactState) { s.DecisionEpoch++ }},
				{"history", func(s *CompactState) {
					s.DecisionEpoch++
					s.DecisionHistory = []CompactDecisionEntry{decisionEntry(CompactDecisionPause, CompactDecisionSystemActor, CompactDecisionReasonEvidenceInconclusive, "")}
				}},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					forged := next
					mutation.change(&forged)
					if err := validateCompactSuccessor(hash("decision-predecessor"), previous, forged, tc.operation); err == nil {
						t.Fatal("accepted decision mutation")
					}
				})
			}
		})
	}
}

func TestDecisionSuccessorRejectsUnownedAuthorityMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-unowned")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*CompactState)
	}{
		{"result reopens", func(s *CompactState) { s.ResultReopens = append(s.ResultReopens, CompactResultReopen{}) }},
		{"correction attempts", func(s *CompactState) { s.CorrectionAttempts = append(s.CorrectionAttempts, CompactCorrectionAttempt{}) }},
		{"recovery", func(s *CompactState) { s.Recovery = &CompactRecoveryProvenance{} }},
		{"correction lines", func(s *CompactState) { s.CumulativeCorrectionLines++ }},
		{"invalidation reason", func(s *CompactState) { s.InvalidationReason = "forged" }},
		{"added paths", func(s *CompactState) { s.CorrectionAddedPaths = []string{"forged.txt"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := decideSuccessor(record.State, CompactDecisionStop, "maintainer", "stop", record.Revision)
			tc.mutate(&next)
			if err := validateCompactSuccessor(record.Revision, record.State, next, "review/decide"); err == nil {
				t.Fatal("accepted unowned mutation")
			}
		})
	}
}

func TestCompactDecisionSuccessorTruthRows(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-rows")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the pause: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	paused = record.State

	// Every refusal below must trip the successor validator, not the revision
	// CAS, so they all run against the live expected revision before any
	// accepted decision moves it.
	refused := func(name string, next CompactState, operation string) {
		t.Helper()
		if _, err := store.Replace(record.Revision, operation, next); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	decide := func(mutate func(*CompactState)) CompactState {
		next := decideSuccessor(paused, CompactDecisionStop, "maintainer@example.com", "stop the review", record.Revision)
		if mutate != nil {
			mutate(&next)
		}
		return next
	}
	refused("decide into validating", decide(func(next *CompactState) { setCompactStateExit(next, StateValidating) }), "review/decide")
	refused("continue entry into escalated", decide(func(next *CompactState) {
		next.DecisionHistory[len(next.DecisionHistory)-1].Decision = CompactDecisionContinue
		setCompactStateExit(next, StateEscalated)
	}), "review/decide")
	refused("stop entry into reviewing", decide(func(next *CompactState) {
		setCompactStateExit(next, StateReviewing)
		next.FixFindingIDs = []string{}
		next.Decision = nil
	}), "review/decide")
	refused("decision without a journaled entry", decide(func(next *CompactState) {
		next.DecisionHistory = next.DecisionHistory[:len(next.DecisionHistory)-1]
	}), "review/decide")
	refused("decision rewriting prior history", decide(func(next *CompactState) {
		next.DecisionHistory[0].Reason = "rewritten"
	}), "review/decide")
	refused("decision without an actor", decide(func(next *CompactState) {
		next.DecisionHistory[len(next.DecisionHistory)-1].Actor = " "
	}), "review/decide")
	refused("decision without a reason", decide(func(next *CompactState) {
		next.DecisionHistory[len(next.DecisionHistory)-1].Reason = ""
	}), "review/decide")
	refused("decision outside the closed vocabulary", decide(func(next *CompactState) {
		next.DecisionHistory[len(next.DecisionHistory)-1].Decision = "maybe"
	}), "review/decide")
	refused("decision advancing the capture phase", decide(func(next *CompactState) {
		next.CapturePhaseEpoch = next.CapturePhaseEpoch + 1
	}), "review/decide")
	refused("decision without the epoch step", decide(func(next *CompactState) {
		next.DecisionEpoch = paused.DecisionEpoch
	}), "review/decide")
	refused("stop retaining a cleared decision block", decide(func(next *CompactState) {
		next.Decision = nil
	}), "review/decide")
	refused("complete-review from decision_required", decide(nil), "review/complete-review")

	// The accepted rows each own a fresh lineage so their expected revisions
	// are live when they commit.
	for _, tc := range []struct {
		name, lineage, decision string
	}{
		{"decide continue row", "decision-rows-continue", CompactDecisionContinue},
		{"decide stop row", "decision-rows-stop", CompactDecisionStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lineagePaused, lineageStore, lineageStarted := decisionFindingFixture(t, repo, tc.lineage)
			if _, err := lineageStore.Replace(lineageStarted.Revision, "review/complete-review", lineagePaused); err != nil {
				t.Fatalf("persist the pause: %v", err)
			}
			lineageRecord, err := lineageStore.Load()
			if err != nil {
				t.Fatal(err)
			}
			actor, reason := "maintainer@example.com", "decide here"
			if tc.decision == CompactDecisionContinue {
				reason = "answer the model's own refutation"
			}
			successor := decideSuccessor(lineageRecord.State, tc.decision, actor, reason, lineageRecord.Revision)
			if _, err := lineageStore.Replace(lineageRecord.Revision, "review/decide", successor); err != nil {
				t.Fatalf("decide %s row refused: %v", tc.decision, err)
			}
		})
	}
}

func TestCompactDecisionValidateContract(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, _, _ := decisionFindingFixture(t, repo, "decision-validate")
	broken := func(name string, mutate func(*CompactState)) {
		t.Helper()
		state := paused
		// Clone the journal slice so a broken entry cannot alias — and corrupt —
		// the shared backing array of the fixture's history.
		state.DecisionHistory = append([]CompactDecisionEntry(nil), paused.DecisionHistory...)
		mutate(&state)
		if err := state.Validate(); err == nil {
			t.Fatalf("%s must refuse validation", name)
		}
	}
	broken("decision_required without its block", func(state *CompactState) { state.Decision = nil })
	broken("decision_required without an epoch", func(state *CompactState) { state.DecisionEpoch = 0 })
	broken("decision_required without history", func(state *CompactState) { state.DecisionHistory = nil })
	broken("decision history outside the closed vocabulary", func(state *CompactState) {
		state.DecisionHistory[0].Decision = "maybe"
	})
	broken("decision history without an actor", func(state *CompactState) {
		state.DecisionHistory[0].Actor = ""
	})
	broken("decision block on a reviewing state", func(state *CompactState) {
		state.State = StateReviewing
	})

	// The decide-stop successor retains the frozen block and passes the
	// existing escalated lifecycle validation, with escalation evidence that
	// renders the same cause the pause classified.
	stopped := decideSuccessor(paused, CompactDecisionStop, "maintainer@example.com", "stop the review", "")
	if err := stopped.Validate(); err != nil {
		t.Fatalf("decide-stop escalated record refuses validation: %v", err)
	}
	if !reflect.DeepEqual(stopped.Decision, paused.Decision) {
		t.Fatalf("decide-stop must retain the frozen decision block")
	}
	evidence := stopped.EscalationEvidence()
	if evidence == nil || evidence.Cause != paused.Decision.Cause {
		t.Fatalf("escalated rendering = %#v, want cause %q", evidence, paused.Decision.Cause)
	}

	// The decide-continue successor clears the answered question and returns
	// to a reviewing authority that validates with the decision provenance
	// retained.
	continued := decideSuccessor(paused, CompactDecisionContinue, "maintainer@example.com", "answer the model's own refutation", "")
	if err := continued.Validate(); err != nil {
		t.Fatalf("decide-continue reviewing record refuses validation: %v", err)
	}
	if continued.Decision != nil || len(continued.FixFindingIDs) != 0 {
		t.Fatalf("decide-continue must clear the answered question: block=%#v fix=%#v", continued.Decision, continued.FixFindingIDs)
	}
	if continued.DecisionEpoch != 2 && continued.DecisionEpoch != paused.DecisionEpoch+1 {
		t.Fatalf("decide-continue epoch = %d, want %d", continued.DecisionEpoch, paused.DecisionEpoch+1)
	}
}

func TestDecideCompactStoreFlow(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-flow")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the pause: %v", err)
	}
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	stopRequest := CompactDecisionRequest{
		LineageID: "decision-flow", ExpectedRevision: persisted.Revision,
		Decision: CompactDecisionStop, Actor: "maintainer@example.com", Reason: "stop the review here",
	}
	stopped, err := DecideCompactStore(context.Background(), repo, stopRequest)
	if err != nil {
		t.Fatalf("review decide stop: %v", err)
	}
	if stopped.State.State != StateEscalated {
		t.Fatalf("decide stop state = %q, want %q", stopped.State.State, StateEscalated)
	}
	if err := stopped.State.Validate(); err != nil {
		t.Fatalf("decide stop record refuses validation: %v", err)
	}
	if stopped.State.DecisionEpoch != 2 || len(stopped.State.DecisionHistory) != 2 {
		t.Fatalf("decide stop epoch=%d history=%d, want 2/2", stopped.State.DecisionEpoch, len(stopped.State.DecisionHistory))
	}
	last := stopped.State.DecisionHistory[len(stopped.State.DecisionHistory)-1]
	if last.Decision != CompactDecisionStop || last.Actor != "maintainer@example.com" || last.Reason != "stop the review here" {
		t.Fatalf("decide stop entry = %#v, want the human actor and reason", last)
	}
	if !reflect.DeepEqual(stopped.State.Decision, persisted.State.Decision) {
		t.Fatalf("decide stop must retain the frozen pause block")
	}
	if evidence := stopped.State.EscalationEvidence(); evidence == nil || evidence.Cause != "unknown_causality" {
		t.Fatalf("decide stop escalation rendering = %#v", evidence)
	}
	if stopped.Revision == persisted.Revision {
		t.Fatal("decide stop must advance the store revision")
	}

	replayed, err := DecideCompactStore(context.Background(), repo, stopRequest)
	if err != nil {
		t.Fatalf("idempotent decide replay: %v", err)
	}
	if replayed.Revision != stopped.Revision || replayed.State.State != StateEscalated {
		t.Fatalf("decide replay = %s@%s, want the committed stop", replayed.State.State, replayed.Revision)
	}
	_, err = DecideCompactStore(context.Background(), repo, CompactDecisionRequest{
		LineageID: "decision-flow", ExpectedRevision: persisted.Revision,
		Decision: CompactDecisionStop, Actor: "maintainer@example.com", Reason: "a different reason",
	})
	if err == nil {
		t.Fatal("a differing decide replay must be refused")
	}
}

func TestDecideCompactStoreContinueRepausesWithNewEpoch(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-continue")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the pause: %v", err)
	}
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	continued, err := DecideCompactStore(context.Background(), repo, CompactDecisionRequest{
		LineageID: "decision-continue", ExpectedRevision: persisted.Revision,
		Decision: CompactDecisionContinue, Actor: "maintainer@example.com", Reason: "answer the model's own refutation",
	})
	if err != nil {
		t.Fatalf("review decide continue: %v", err)
	}
	if continued.State.State != StateReviewing {
		t.Fatalf("decide continue state = %q, want %q", continued.State.State, StateReviewing)
	}
	if err := continued.State.Validate(); err != nil {
		t.Fatalf("decide continue record refuses validation: %v", err)
	}

	// The resumed review pauses again through the same engine path, and the
	// decision epoch proves this is the second decision.
	// The resumed review completes again from the same admitted evidence:
	// the captures survive the continue, so the second completion re-derives
	// the same unresolved view and the engine pauses again with epoch 2.
	view, err := continued.State.CompactReviewView()
	if err != nil {
		t.Fatalf("derive resumed review view: %v", err)
	}
	repaised := continued.State
	if err := repaised.CompleteReview(CompactReviewInput{
		LensResults: view.LensResults, Classifications: compactReviewViewClassifications(view), RefuterOutcomes: view.RefuterOutcomes,
	}); err != nil {
		t.Fatalf("resumed review completion: %v", err)
	}
	if _, err := store.Replace(continued.Revision, "review/complete-review", repaised); err != nil {
		t.Fatalf("persist the second pause: %v", err)
	}
	if repaised.State != StateDecisionRequired {
		t.Fatalf("resumed review state = %q, want %q", repaised.State, StateDecisionRequired)
	}
	if repaised.DecisionEpoch != 3 {
		t.Fatalf("second pause epoch = %d, want 3 (pause, continue, pause)", repaised.DecisionEpoch)
	}
	if len(repaised.DecisionHistory) != 3 || repaised.DecisionHistory[2].Decision != CompactDecisionPause {
		t.Fatalf("second pause history = %#v, want a second engine-authored pause entry", repaised.DecisionHistory)
	}
}

func TestDecideCompactStoreRefusals(t *testing.T) {
	repo := initSnapshotRepo(t)
	paused, store, started := decisionFindingFixture(t, repo, "decision-refusals")
	if _, err := store.Replace(started.Revision, "review/complete-review", paused); err != nil {
		t.Fatalf("persist the pause: %v", err)
	}
	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := CompactDecisionRequest{
		LineageID: "decision-refusals", ExpectedRevision: persisted.Revision,
		Decision: CompactDecisionStop, Actor: "maintainer@example.com", Reason: "stop the review here",
	}
	if _, err := DecideCompactStore(context.Background(), repo, CompactDecisionRequest{
		LineageID: "decision-refusals", ExpectedRevision: persisted.Revision,
		Decision: "maybe", Actor: "maintainer@example.com", Reason: "stop the review here",
	}); err == nil {
		t.Fatal("a decision outside the closed vocabulary must be refused")
	}
	for _, mutate := range []func(*CompactDecisionRequest){
		func(r *CompactDecisionRequest) { r.Actor = " " },
		func(r *CompactDecisionRequest) { r.Reason = "" },
		func(r *CompactDecisionRequest) { r.LineageID = "absent-lineage" },
	} {
		request := base
		mutate(&request)
		if _, err := DecideCompactStore(context.Background(), repo, request); err == nil {
			t.Fatalf("decide request %#v must be refused", request)
		}
	}
	// A stale expected revision must surface the typed CAS conflict, proving
	// the losing caller mutated nothing.
	conflicted := base
	conflicted.ExpectedRevision = "sha256:" + strings.Repeat("ab", 32)
	if _, err := DecideCompactStore(context.Background(), repo, conflicted); !errors.As(err, new(*CompactRevisionConflictError)) {
		t.Fatalf("stale expected revision error = %v, want a typed revision conflict", err)
	}
	// A lineage that never paused has no decision to make: the refusal must
	// name the decision gate instead of a generic state error.
	writeSnapshotFile(t, repo, "other.txt", "reviewing\n")
	other := newCompactTestState(t, repo, "decision-reviewing")
	_, otherStore := startReviewingCompactFixture(t, repo, other)
	otherRecord, err := otherStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecideCompactStore(context.Background(), repo, CompactDecisionRequest{
		LineageID: "decision-reviewing", ExpectedRevision: otherRecord.Revision,
		Decision: CompactDecisionStop, Actor: "maintainer@example.com", Reason: "stop the review here",
	})
	if err == nil || !strings.Contains(err.Error(), "decision") {
		t.Fatalf("decide on a reviewing lineage = %v, want a decision-gate refusal", err)
	}
}
