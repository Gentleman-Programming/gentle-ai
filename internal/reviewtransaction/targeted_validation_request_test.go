package reviewtransaction

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTargetedValidationRequestBindsCurrentAuthorityAndCorrectedCandidate(t *testing.T) {
	repo, state, revision, _ := targetedValidationRequestFixture(t, "targeted-validation-current", true)
	first, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatalf("restarted request = %#v, %v; want %#v", second, err, first)
	}
	if first.Schema != TargetedValidationRequestSchema || first.LineageID != state.LineageID ||
		first.ExpectedRevision != revision || first.TargetIdentity != state.InitialSnapshot.Identity ||
		!reflect.DeepEqual(first.FixFindingIDs, state.FixFindingIDs) ||
		!reflect.DeepEqual(first.CorrectionPaths, []string{"tracked.txt"}) ||
		first.CorrectionPathsDigest != digestPaths(first.CorrectionPaths) ||
		first.CorrectionCandidateTree == state.CurrentSnapshot.CandidateTree ||
		ValidateTargetedValidationRequest(first) != nil {
		t.Fatalf("targeted request = %#v", first)
	}

	for name, mutate := range map[string]func(*TargetedValidationRequest){
		"hash": func(value *TargetedValidationRequest) { value.RequestHash = "sha256:" + strings.Repeat("a", 64) },
		"finding IDs": func(value *TargetedValidationRequest) {
			value.FixFindingIDs = append(value.FixFindingIDs, value.FixFindingIDs[0])
		},
		"candidate":  func(value *TargetedValidationRequest) { value.CorrectionCandidateTree = strings.Repeat("a", 40) },
		"projection": func(value *TargetedValidationRequest) { value.Projection = Projection("invented") },
		"correction paths": func(value *TargetedValidationRequest) {
			value.CorrectionPaths = append(value.CorrectionPaths, value.CorrectionPaths[0])
		},
		"correction path digest": func(value *TargetedValidationRequest) {
			value.CorrectionPathsDigest = "sha256:" + strings.Repeat("b", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := first
			changed.FixFindingIDs = append([]string(nil), first.FixFindingIDs...)
			changed.CorrectionPaths = append([]string(nil), first.CorrectionPaths...)
			mutate(&changed)
			if err := ValidateTargetedValidationRequest(changed); err == nil {
				t.Fatalf("tampered targeted request was accepted: %#v", changed)
			}
		})
	}
}

func TestTargetedValidationRequestRejectsUnchangedAndStaleAuthority(t *testing.T) {
	repo, unchanged, unchangedRevision, _ := targetedValidationRequestFixture(t, "targeted-validation-unchanged", false)
	if _, err := BuildTargetedValidationRequest(context.Background(), repo, unchanged, unchangedRevision); err == nil {
		t.Fatal("unchanged correction candidate produced a validator request")
	}

	repo, stale, staleRevision, store := targetedValidationRequestFixture(t, "targeted-validation-stale", true)
	next := stale
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: next.InitialSnapshot.Projection,
		BaseRef: next.CurrentSnapshot.CandidateTree, IntendedUntracked: next.InitialSnapshot.IntendedUntracked,
		LedgerIDs: next.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: next.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash},
		CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("3"), FixDeltaHash: fixHash},
	}
	if err := next.CompleteCorrection(fix, 2, bindTargetedValidationForTest(validation, fix)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(staleRevision, "review/complete-fix", next); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTargetedValidationRequest(context.Background(), repo, stale, staleRevision); err == nil {
		t.Fatal("stale compact authority produced a validator request")
	}
}

func TestTargetedValidationRequestFromSnapshotIgnoresLaterWorkspaceChanges(t *testing.T) {
	repo, state, revision, _ := targetedValidationRequestFixture(t, "targeted-validation-coherent-snapshot", true)
	live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: state.InitialSnapshot.Projection,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\nlater\n")

	request, err := BuildTargetedValidationRequestFromSnapshot(context.Background(), repo, state, revision, live)
	if err != nil {
		t.Fatal(err)
	}
	if request.CorrectionCandidateTree != live.CandidateTree {
		t.Fatalf("request candidate = %s, want captured live tree %s", request.CorrectionCandidateTree, live.CandidateTree)
	}
	if got := gitSnapshot(t, repo, "show", request.CorrectionCandidateTree+":tracked.txt"); got != "base\nfixed\n" {
		t.Fatalf("request candidate content = %q, want captured content", got)
	}
}

func TestTargetedValidationRequestCountsOnlyPartialCorrectionAcrossIntendedUntracked(t *testing.T) {
	repo := initSnapshotRepo(t)
	var tracked, intended strings.Builder
	for index := 0; index < 120; index++ {
		fmt.Fprintf(&tracked, "candidate tracked line %03d\n", index)
		fmt.Fprintf(&intended, "var CandidateValue%03d = %d\n", index, index)
	}
	writeSnapshotFile(t, repo, "tracked.txt", tracked.String())
	writeSnapshotFile(t, repo, "intended.go", intended.String())

	state := newCompactTestStateWithIntended(t, repo, "targeted-validation-partial-intended", []string{"intended.go"})
	if state.OriginalChangedLines <= 200 || state.RiskLevel != RiskMedium || len(state.SelectedLenses) != 1 {
		t.Fatalf("original review scope = lines:%d risk:%q lenses:%v", state.OriginalChangedLines, state.RiskLevel, state.SelectedLenses)
	}
	store := storeCompactStartAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:61", Severity: "CRITICAL",
		Claim: "candidate values require a paired correction", ProofRefs: []string{"candidate-only differential failure"},
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact initial candidate"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(4); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/begin-fix", state)
	if err != nil {
		t.Fatal(err)
	}

	correctedTracked := strings.Replace(tracked.String(), "candidate tracked line 060", "corrected tracked line 060", 1)
	correctedIntended := strings.Replace(intended.String(), "var CandidateValue060 = 60", "var CorrectedValue060 = 60", 1)
	writeSnapshotFile(t, repo, "tracked.txt", correctedTracked)
	writeSnapshotFile(t, repo, "intended.go", correctedIntended)
	live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: state.InitialSnapshot.Projection,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildTargetedValidationRequestFromSnapshot(context.Background(), repo, state, revision, live)
	if err != nil {
		t.Fatal(err)
	}
	nativeFix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: state.InitialSnapshot.Projection,
		BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
		LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	nativeLines, err := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), nativeFix)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"intended.go", "tracked.txt"}
	if nativeLines != 4 || !reflect.DeepEqual(nativeFix.Paths, wantPaths) ||
		request.CorrectionCandidateTree != nativeFix.CandidateTree || request.CorrectionTargetIdentity != nativeFix.Identity ||
		!reflect.DeepEqual(request.CorrectionPaths, wantPaths) || request.CorrectionPathsDigest != nativeFix.PathsDigest {
		t.Fatalf("partial correction = lines:%d fix:%#v request:%#v", nativeLines, nativeFix, request)
	}
}

func targetedValidationRequestFixture(t *testing.T, lineage string, correct bool) (string, CompactState, string, CompactStore) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	state := newCompactTestState(t, repo, lineage)
	store := storeCompactStartAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:2", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: []FindingEvidence{{
			FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes failure",
		}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/begin-fix", state)
	if err != nil {
		t.Fatal(err)
	}
	if correct {
		writeSnapshotFile(t, repo, "tracked.txt", "base\nfixed\n")
	}
	return repo, state, revision, store
}
