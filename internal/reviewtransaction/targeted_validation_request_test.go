package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
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

func TestTargetedValidationRequestBindsFrozenPolicyAndCausalEvidence(t *testing.T) {
	repo, state, revision, _ := targetedValidationRequestFixture(t, "targeted-validation-frozen-semantics", true)
	request, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil {
		t.Fatal(err)
	}
	if state.FrozenPolicyContent == nil {
		t.Fatal("fixture did not preserve the frozen policy content")
	}
	if request.PolicyContent != *state.FrozenPolicyContent || len(request.FixFindings) != len(state.FixFindingIDs) ||
		len(request.FixClassifications) != len(state.FixFindingIDs) {
		t.Fatalf("targeted request semantic context = %#v", request)
	}
	for index, findingID := range state.FixFindingIDs {
		if request.FixFindings[index].ID != findingID || request.FixClassifications[index].FindingID != findingID ||
			!reflect.DeepEqual(request.FixFindings[index], state.Findings[index]) ||
			!reflect.DeepEqual(request.FixClassifications[index], state.Classifications[findingID]) {
			t.Fatalf("targeted request semantic context at %d = %#v / %#v, want finding %q and %#v", index, request.FixFindings[index], request.FixClassifications[index], findingID, state.Classifications[findingID])
		}
	}
	policyDrift := request
	policyDrift.PolicyContent += "\ndrift"
	if targetedValidationRequestHash(policyDrift) == request.RequestHash {
		t.Fatal("policy drift did not change the targeted validator request hash")
	}
	driftedState := state
	driftedState.FrozenPolicyContent = &policyDrift.PolicyContent
	if _, err := driftedState.FrozenPolicyForTargetedValidation(); err == nil {
		t.Fatal("mismatched frozen policy content was accepted")
	}
	findingDrift := request
	findingDrift.FixFindings = append([]Finding(nil), request.FixFindings...)
	findingDrift.FixFindings[0].Claim += " drift"
	if targetedValidationRequestHash(findingDrift) == request.RequestHash {
		t.Fatal("causal finding drift did not change the targeted validator request hash")
	}
}

func TestTargetedValidationRequestRefusesMissingDuplicateOrMismatchedCausalEvidence(t *testing.T) {
	repo, state, revision, _ := targetedValidationRequestFixture(t, "targeted-validation-refuse-semantics", true)
	request, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*TargetedValidationRequest){
		"missing finding": func(value *TargetedValidationRequest) { value.FixFindings = nil },
		"duplicate finding": func(value *TargetedValidationRequest) {
			value.FixFindings = append(value.FixFindings, value.FixFindings[0])
		},
		"missing classification": func(value *TargetedValidationRequest) { value.FixClassifications = nil },
		"duplicate classification": func(value *TargetedValidationRequest) {
			value.FixClassifications = append(value.FixClassifications, value.FixClassifications[0])
		},
		"mismatched classification": func(value *TargetedValidationRequest) {
			value.FixClassifications[0].FindingID = "R3-not-the-requested-finding"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			changed.FixFindings = append([]Finding(nil), request.FixFindings...)
			changed.FixClassifications = append([]FindingEvidence(nil), request.FixClassifications...)
			mutate(&changed)
			changed.RequestHash = targetedValidationRequestHash(changed)
			if err := ValidateTargetedValidationRequest(changed); err == nil {
				t.Fatalf("targeted request accepted %s causal evidence: %#v", name, changed)
			}
		})
	}
}

func TestTargetedValidationRequestFailsClosedWithoutFrozenPolicyContent(t *testing.T) {
	repo, state, revision, _ := targetedValidationRequestFixtureWithFrozenPolicy(t, "targeted-validation-legacy-policy", true, false)
	if _, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision); err == nil {
		t.Fatal("legacy authority with a policy hash but no frozen policy content built a validator request")
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

func TestResolveCorrectedCandidateInspectionUsesCapturedEvidenceAfterLiveDrift(t *testing.T) {
	passed := VerificationOutcomePassed
	repo, request, correction, handle, binding, store := correctedInspectionFixture(t, "corrected-inspection-immutable", &passed)
	ctx := context.Background()
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// The old context resolver reconstructs TargetFixDiff from this mutable file.
	writeSnapshotFile(t, repo, "tracked.txt", "base\ndecoy drift\n")
	if _, err := ResolveReviewRepositoryContext(ctx, handle, binding); err == nil {
		t.Fatal("live repository-context resolver accepted drifted correction")
	}

	resolved, err := ResolveCorrectedCandidateInspection(ctx, handle, request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotsEqual(resolved, correction) || !reflect.DeepEqual(after, before) {
		t.Fatalf("resolved=%#v authority changed=%t", resolved, !reflect.DeepEqual(after, before))
	}
	payload, err := (SnapshotBuilder{Repo: repo}).InspectCandidate(ctx, resolved, "object", 0, "candidate")
	if err != nil || string(payload) != "base\nfixed\n" {
		t.Fatalf("immutable corrected inspection = %q, %v", payload, err)
	}
}

func TestResolveCorrectedCandidateInspectionFailsClosed(t *testing.T) {
	passed := VerificationOutcomePassed
	t.Run("forged request hash", func(t *testing.T) {
		_, request, _, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-forged-hash", &passed)
		request.RequestHash = hash("forged-request")
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("forged request hash resolved")
		}
	})
	t.Run("locator target mismatch", func(t *testing.T) {
		_, request, _, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-target-mismatch", &passed)
		request.CorrectionTargetIdentity = hash("other-correction")
		request.RequestHash = targetedValidationRequestHash(request)
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("locator target mismatch resolved")
		}
	})
	t.Run("missing correction tree", func(t *testing.T) {
		_, request, _, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-missing-tree", &passed)
		request.CorrectionCandidateTree = strings.Repeat("a", 40)
		request.RequestHash = targetedValidationRequestHash(request)
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("missing correction tree resolved")
		}
	})
	t.Run("altered correction evidence tree", func(t *testing.T) {
		_, request, correction, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-altered-tree", &passed)
		request.CorrectionCandidateTree = correction.BaseTree
		request.RequestHash = targetedValidationRequestHash(request)
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("altered correction tree resolved")
		}
	})
	t.Run("altered correction evidence path digest", func(t *testing.T) {
		_, request, _, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-altered-paths", &passed)
		request.CorrectionPathsDigest = hash("altered-paths")
		request.RequestHash = targetedValidationRequestHash(request)
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("altered correction paths resolved")
		}
	})
	t.Run("propagates authority load error", func(t *testing.T) {
		_, request, _, handle, binding, store := correctedInspectionFixture(t, "corrected-inspection-load-error", &passed)
		if err := os.Remove(store.StatePath()); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ResolveCorrectedCandidateInspectionBinding(context.Background(), handle, binding, request.RequestHash); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("binding load error = %v, want %v", err, os.ErrNotExist)
		}
	})
	t.Run("missing evidence", func(t *testing.T) {
		_, request, _, handle, _, _ := correctedInspectionFixture(t, "corrected-inspection-missing-evidence", nil)
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatal("missing repository evidence resolved")
		}
	})
	t.Run("stale authority", func(t *testing.T) {
		repo, request, correction, handle, _, store := correctedInspectionFixture(t, "corrected-inspection-stale-authority", &passed)
		current, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		next := current.State
		fixHash := FixDeltaHashForSnapshot(correction)
		validation := bindTargetedValidationForTest(ScopedValidationResult{
			LedgerIDs: next.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
			OriginalCriteria:     ValidationCheck{Passed: true, EvidenceHash: hash("1"), FixDeltaHash: fixHash},
			CorrectionRegression: ValidationCheck{Passed: true, EvidenceHash: hash("2"), FixDeltaHash: fixHash},
		}, correction)
		payload := []byte("repository verification passed\n")
		evidence, err := NewVerificationEvidenceRecord(next.LineageID, current.Revision, correction, payload, passed)
		if err != nil {
			t.Fatal(err)
		}
		if err := next.CompleteCorrectionVerification(correction, 2, validation, evidence, payload); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Replace(current.Revision, "review/complete-correction-verification", next); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveCorrectedCandidateInspection(context.Background(), handle, request); err == nil {
			t.Fatalf("stale authority for %q resolved", repo)
		}
	})
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
	policy := "targeted validation frozen policy\n"
	state.PolicyHash = hashArtifactPayload([]byte(policy))
	state.FrozenPolicyContent = &policy
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
	return targetedValidationRequestFixtureWithFrozenPolicy(t, lineage, correct, true)
}

func targetedValidationRequestFixtureWithFrozenPolicy(t *testing.T, lineage string, correct, freezePolicy bool) (string, CompactState, string, CompactStore) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nwrong\n")
	state := newCompactTestState(t, repo, lineage)
	if freezePolicy {
		policy := "targeted validation frozen policy\n"
		state.PolicyHash = hashArtifactPayload([]byte(policy))
		state.FrozenPolicyContent = &policy
	}
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

func correctedInspectionFixture(t *testing.T, lineage string, outcome *VerificationOutcome) (string, TargetedValidationRequest, Snapshot, string, ReviewRepositoryContextBinding, CompactStore) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo, state, revision, store := targetedValidationRequestFixture(t, lineage, true)
	request, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil {
		t.Fatal(err)
	}
	correction, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: state.InitialSnapshot.Projection,
		BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
		LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if correction.Identity != request.CorrectionTargetIdentity {
		t.Fatalf("correction identity = %s, want request target %s", correction.Identity, request.CorrectionTargetIdentity)
	}
	binding := ReviewRepositoryContextBinding{LineageID: state.LineageID, TargetIdentity: request.CorrectionTargetIdentity, Revision: revision}
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != nil {
		if _, err := PublishCapturedVerificationEvidence(CaptureVerificationEvidenceRequest{
			StoreDir: store.Dir, LineageID: state.LineageID, AuthorityRevision: revision,
			Target: correction, Payload: []byte("repository verification passed\n"), Outcome: *outcome,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return repo, request, correction, handle, binding, store
}
