package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestActiveCompactRecordOmitsResultDispositionsAndStablyDigestsAdmittedRoles(t *testing.T) {
	repo := initSnapshotRepo(t)
	record := CompactRecord{Schema: compactRecordSchema, Revision: hash("active-compact-record"), State: newCompactTestState(t, repo, "active-compact-record")}

	// The reflection guard makes a future reintroduction of the retired persisted
	// field observable without coupling this post-deletion test to its Go type.
	if field := reflect.ValueOf(&record.State).Elem().FieldByName("ResultDispositions"); field.IsValid() {
		field.Set(reflect.MakeSlice(field.Type(), 1, 1))
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var serialized struct {
		State map[string]json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(payload, &serialized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"lens_results", "findings", "classifications", "outcomes", "follow_ups", "result_dispositions",
	} {
		if _, found := serialized.State[key]; found {
			t.Fatalf("new compact record serialized retired %s state", key)
		}
	}

	const wantDigest = "sha256:ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356"
	if got := compactPreservedPayloadDigest([]byte("{}\n")); got != wantDigest {
		t.Fatalf("admitted-role payload digest = %q, want %q", got, wantDigest)
	}
}

func TestCompactHistoricalFailedValidatorRequiresLocalAttempt(t *testing.T) {
	state := CompactState{
		State: StateCorrectionRequired,
		Recovery: &CompactRecoveryProvenance{
			ConsumedCorrectionAttempts: MaxCompactCorrectionAttempts,
		},
	}
	if compactHistoricalFailedValidator(state) {
		t.Fatal("recovery accounting without a local correction attempt must not be treated as a historical failed validator")
	}
}

func TestNewCompactStateDerivesNonCircularCapturePhaseBeforeRecordCreation(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "capture-phase-before-record")
	if !validSHA256(state.CapturePhaseRevision) {
		t.Fatalf("capture phase revision = %q, want canonical SHA-256", state.CapturePhaseRevision)
	}

	preimage := state
	preimage.CapturePhaseRevision = ""
	derived, err := deriveCompactCapturePhaseRevision(preimage)
	if err != nil {
		t.Fatal(err)
	}
	if derived != state.CapturePhaseRevision {
		t.Fatalf("derived capture phase = %q, want %q", derived, state.CapturePhaseRevision)
	}

	record, _, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision == state.CapturePhaseRevision {
		t.Fatal("live record revision must remain distinct from the stable capture phase")
	}
}

func TestCompactCapturePhasePreimageIncludesAtomicWorktreeIdentity(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "capture-phase-worktree")
	binding := CompactAtomicStartBinding{
		LineageID: state.LineageID, WorktreeIdentity: hash("worktree-one"),
		TargetIdentity: state.InitialSnapshot.Identity, Selector: Target{Kind: state.InitialSnapshot.Kind},
		PolicyHash: state.PolicyHash, Tier: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
		OriginalChangedLines: state.OriginalChangedLines, CorrectionBudget: state.CorrectionBudget,
		CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
	}
	state.InitialAtomicStart = &binding
	first, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	state.InitialAtomicStart.WorktreeIdentity = hash("worktree-two")
	second, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("capture phase did not bind the frozen atomic START worktree identity")
	}
}

func TestCompactRecordBoundsRefuseMaxPlusOneBeforePersistence(t *testing.T) {
	entry := CompactAdmittedRoleResult{Value: json.RawMessage(`{}`)}
	tooMany := make([]CompactAdmittedRoleResult, compactMaxAdmittedRoleResults+1)
	for index := range tooMany {
		tooMany[index] = entry
	}
	if err := validateCompactRoleResultBounds(tooMany); err == nil {
		t.Fatal("accepted more than six admitted role values")
	}

	tooLargeRole := entry
	tooLargeRole.Value = make(json.RawMessage, compactReviewerResultSizeLimit+1)
	if err := validateCompactRoleResultBounds([]CompactAdmittedRoleResult{tooLargeRole}); err == nil {
		t.Fatal("accepted a role value above the current four MiB limit")
	}

	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "compact-non-role-bound")
	state.InvalidationReason = strings.Repeat("x", compactNonRoleStateSizeLimit)
	if err := validateCompactNonRoleStateBounds(state); err == nil {
		t.Fatal("accepted non-role compact state at the seven MiB max-plus-one boundary")
	}
	if err := validateCompactRecordWritePayload(make([]byte, compactRecordSizeLimit+1)); err == nil {
		t.Fatal("accepted compact record bytes above the 32 MiB write limit")
	}
}

func TestCompactTargetedValidatorAttemptLedgerReplaysAndRefusesFourthWithoutMutation(t *testing.T) {
	repo, _, _, store := targetedValidationRequestFixture(t, "targeted-validator-attempt-ledger", true)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	fix, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{
		Kind: TargetFixDiff, Projection: record.State.InitialSnapshot.Projection,
		BaseRef: record.State.CurrentSnapshot.CandidateTree, IntendedUntracked: record.State.InitialSnapshot.IntendedUntracked,
		LedgerIDs: record.State.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(t.Context(), repo, record.State, record.State.CapturePhaseRevision, fix)
	if err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{hash("a"), hash("b"), hash("c")} {
		replayed, err := store.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, digest)
		if err != nil || replayed {
			t.Fatalf("record distinct validator attempt %q: replayed=%t err=%v", digest, replayed, err)
		}
	}
	restarted, err := CompactAuthoritativeStore(t.Context(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, hash("c"))
	if err != nil || !replayed {
		t.Fatalf("exact validator replay after restart: replayed=%t err=%v", replayed, err)
	}
	after, err := restarted.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.State.TargetedValidatorAttempts) != 3 {
		t.Fatalf("validator attempt count = %d, want 3", len(after.State.TargetedValidatorAttempts))
	}
	beforeFourth := after.Revision
	if _, err := store.RecordInconclusiveTargetedValidatorAttempt(t.Context(), request, hash("d")); !errors.Is(err, ErrCompactTargetedValidatorAttemptsExhausted) {
		t.Fatalf("fourth validator attempt error = %v, want exhaustion", err)
	}
	afterFourth, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterFourth.Revision != beforeFourth || len(afterFourth.State.TargetedValidatorAttempts) != 3 {
		t.Fatal("fourth validator attempt mutated the authority")
	}
}

func TestCompactStateValidatesCanonicalAdmittedLensBindings(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "canonical-admitted-lens-bindings")
	state, store := startReviewingCompactAuthority(t, repo, state)
	captureCompactLens(t, store, state, 0)
	record := requireCompactRoleCount(t, store, 1)
	state = record.State
	entry := state.AdmittedRoleResults[0]
	if err := state.Validate(); err != nil {
		t.Fatalf("valid admitted lens binding: %v", err)
	}

	state.AdmittedRoleResults = append(state.AdmittedRoleResults, entry)
	if err := state.Validate(); err == nil {
		t.Fatal("accepted duplicate admitted lens tuple")
	}
}

func newCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return newCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func newCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// TestReplaceContextGuardedCommitsAndReportsUnwritableTraceOutcome is issue
// #1854, reworked to the issue's own accepted scope: the transition
// genuinely succeeds, so it must commit exactly as it would without a
// requested trace. A trace that cannot be persisted is represented as a
// typed, committed-but-degraded outcome -- never a rolled-back commit and
// never a stderr-only warning. The reproduction matches the issue exactly:
// an existing directory supplied as the trace target, which can never be
// opened as a file.
func TestReplaceContextGuardedCommitsAndReportsUnwritableTraceOutcome(t *testing.T) {
	const lineage = "compact-trace-unwritable-commits-and-reports"
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	store.TracePath = t.TempDir() // an existing directory: never openable as a file
	var outcome CompactTraceOutcome
	store.TraceOutcome = &outcome

	var warned []string
	originalWarn := compactTraceWarn
	compactTraceWarn = func(operation, path string, err error) { warned = append(warned, operation) }
	t.Cleanup(func() { compactTraceWarn = originalWarn })

	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatalf("Replace() with an unwritable requested trace: %v", err)
	}
	if _, statErr := os.Stat(store.StatePath()); statErr != nil {
		t.Fatalf("authority did not commit despite the trace failure: stat(review-state.json) = %v", statErr)
	}
	if outcome.Persisted || outcome.ErrorClass == "" || outcome.Revision != revision || outcome.Operation != "review/start" {
		t.Fatalf("trace outcome = %#v, want persisted=false, a non-empty error class, and the committed revision %q", outcome, revision)
	}
	if outcome.Identity() == "" {
		t.Fatal("trace outcome carries no event identity")
	}
	// A caller that wires TraceOutcome already has a home for this failure:
	// the stderr fallback must not also fire, or an operator would see the
	// same failure reported twice.
	if len(warned) != 0 {
		t.Fatalf("a projected trace outcome must suppress the stderr fallback, got warnings = %v", warned)
	}
}

// TestReplaceContextGuardedWarnsWhenNoTraceOutcomeIsProjected is the R4
// finding this test closes: a caller that sets TracePath without also
// wiring TraceOutcome (no machine result exists to carry the outcome on that
// call path) must still learn about a trace failure through the stderr
// fallback, never total silence.
func TestReplaceContextGuardedWarnsWhenNoTraceOutcomeIsProjected(t *testing.T) {
	const lineage = "compact-trace-unwritable-warns-without-outcome"
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	store.TracePath = t.TempDir() // an existing directory: never openable as a file
	// store.TraceOutcome is deliberately left nil: no result on this call
	// path can carry the outcome.

	var warned []string
	originalWarn := compactTraceWarn
	compactTraceWarn = func(operation, path string, err error) {
		if path != store.TracePath || err == nil {
			t.Errorf("compactTraceWarn(%q, %q, %v)", operation, path, err)
		}
		warned = append(warned, operation)
	}
	t.Cleanup(func() { compactTraceWarn = originalWarn })

	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatalf("Replace() with an unwritable requested trace: %v", err)
	}
	if _, statErr := os.Stat(store.StatePath()); statErr != nil {
		t.Fatalf("authority did not commit despite the trace failure: stat(review-state.json) = %v", statErr)
	}
	if !reflect.DeepEqual(warned, []string{"review/start"}) {
		t.Fatalf("stderr fallback warnings = %v, want exactly one for review/start", warned)
	}
}

// TestReplaceContextGuardedCommitsAndReportsPersistedTraceOutcome is the
// positive-path sibling: a writable trace path commits normally and reports
// persisted=true with the committed revision.
func TestReplaceContextGuardedCommitsAndReportsPersistedTraceOutcome(t *testing.T) {
	const lineage = "compact-trace-writable-commits-and-reports"
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	store.TracePath = filepath.Join(t.TempDir(), "trace.jsonl")
	var outcome CompactTraceOutcome
	store.TraceOutcome = &outcome

	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatalf("Replace() with a writable requested trace: %v", err)
	}
	if !outcome.Persisted || outcome.ErrorClass != "" || outcome.Revision != revision || outcome.Operation != "review/start" {
		t.Fatalf("trace outcome = %#v, want persisted=true, no error class, and the committed revision %q", outcome, revision)
	}
	payload, err := os.ReadFile(store.TracePath)
	if err != nil {
		t.Fatalf("requested trace was not written: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"review/start"`)) {
		t.Fatalf("trace payload missing the committed operation: %s", payload)
	}
}

// TestReplaceContextGuardedCommitsWhenNoTraceIsRequested pins the "no trace
// requested preserves current behavior" bound from #1854: an empty
// TracePath must never attempt a write or affect the commit.
func TestReplaceContextGuardedCommitsWhenNoTraceIsRequested(t *testing.T) {
	const lineage = "compact-trace-absent-commits-normally"
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatalf("Replace() with no requested trace: %v", err)
	}
	if _, statErr := os.Stat(store.StatePath()); statErr != nil {
		t.Fatalf("authority did not commit: stat(review-state.json) = %v", statErr)
	}
}

func pendingCompactCorrection(t *testing.T, repo, lineage string) (CompactState, Snapshot) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, lineage)
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed once"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	return state, fix
}

func correctedCompactTestState(t *testing.T, repo, lineage string) CompactState {
	return correctedCompactTestStateWithIntended(t, repo, lineage, []string{})
}

func correctedCompactTestStateWithIntended(t *testing.T, repo, lineage string, intended []string) CompactState {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	for _, path := range intended {
		writeSnapshotFile(t, repo, path, "initial intended content\n")
	}
	state := newCompactTestStateWithIntended(t, repo, lineage, intended)
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate returns the wrong terminal value", ProofRefs: []string{"differential test fails only on candidate"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	result := LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"focused differential test failed"}}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{result},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes the failure"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.InitialSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}
	if err := state.CompleteCorrectionVerification(fix, 2, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatal(err)
	}
	return state
}

func bindTargetedValidationForTest(validation ScopedValidationResult, fix Snapshot) ScopedValidationResult {
	validation.TargetedValidationRequestHash = hash("9")
	validation.CorrectionTargetIdentity = fix.Identity
	return validation
}
