package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelectorlessCommittedBaseDiffCorrectionsRequireMatchingPredecessor pins
// issue #2345: a small, unrelated new candidate must never bind to a stale
// historical correction_required lineage merely because that lineage's
// genesis paths happen to be broad enough to admit the live diff. Binding is
// legitimate only when the correction lineage's own frozen current candidate
// is the exact predecessor the live candidate continues from.
//
// The fixture builds three lineages that all happen to touch the same file
// ("candidate.go"):
//   - "stale-correction": reviewed the file at commit1 (base commit0), found
//     a severe finding, reached correction_required, and was then abandoned.
//     Its own recorded OriginalChangedLines reflects that original,
//     substantially larger diff.
//   - "approved-lineage": a later, unrelated, properly approved review of the
//     same file (base commit1, candidate commit2). This is already excluded
//     by RebuildCommittedBaseDiffCorrectionCandidate's own state check (it is
//     not correction_required); it is included only to prove its presence
//     does not change the outcome.
//   - the live candidate: one small additional one-line edit committed
//     directly on top of the approved delivery (commit3, parent commit2).
//
// Before the fix, selectorlessCommittedBaseDiffCorrections rebuilt
// "stale-correction" from ITS OWN frozen initial base (commit0) to the
// current HEAD (commit3); since only candidate.go ever changed across the
// whole history, that rebuilt diff stayed a subset of the stale lineage's
// genesis paths and bound successfully, incorrectly attaching the live
// candidate to it and reporting its unrelated, much larger changed-line
// count instead of the live candidate's own tiny diff.
func TestSelectorlessCommittedBaseDiffCorrectionsRequireMatchingPredecessor(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	builder := SnapshotBuilder{Repo: repo}

	commit0 := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	commit0Tree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

	// commit1: the stale lineage's own original ("wrong") candidate: a
	// substantially sized change so its recorded line count is unmistakably
	// its own, not the live candidate's.
	writeSnapshotFile(t, repo, "candidate.go", "package candidate\n\n"+
		"func value() int {\n\treturn 1\n}\n\n"+
		"func other() int {\n\treturn 2\n}\n\n"+
		"func third() int {\n\treturn 3\n}\n\n"+
		"func fourth() int {\n\treturn 4\n}\n")
	gitSnapshot(t, repo, "add", "candidate.go")
	gitSnapshot(t, repo, "commit", "-qm", "stale candidate")

	staleState := newCompactFixtureStateForTarget(t, repo, "stale-correction", Target{
		Kind: TargetBaseDiff, BaseRef: commit0Tree, IntendedUntracked: []string{},
	})
	if len(staleState.SelectedLenses) == 0 {
		t.Fatal("stale correction fixture unexpectedly selected no lenses")
	}
	staleOriginalLines := staleState.OriginalChangedLines
	staleState, staleStore := startReviewingCompactFixture(t, repo, staleState)
	staleFinding := Finding{
		ID: "R3-001", Lens: staleState.SelectedLenses[0], Location: "candidate.go:4", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate.go:4 changed hunk"},
	}
	staleResults := make([]LensResult, len(staleState.SelectedLenses))
	for index, lens := range staleState.SelectedLenses {
		staleResults[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	staleResults[0].Findings = []Finding{staleFinding}
	staleState, staleStarted := captureAndCompleteCompactReview(t, staleStore, staleState, CompactReviewInput{
		LensResults: staleResults,
		Classifications: []FindingEvidence{{
			FindingID: staleFinding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk",
		}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if staleState.State != StateCorrectionRequired {
		t.Fatalf("stale fixture state = %q, want %q", staleState.State, StateCorrectionRequired)
	}
	staleRevision, err := staleStore.Replace(staleStarted.Revision, "review/complete-review", staleState)
	if err != nil {
		t.Fatal(err)
	}
	// RebuildCommittedBaseDiffCorrectionCandidate is only eligible once a
	// correction has been forecasted (ProposedCorrectionLines set): a bare
	// correction_required lineage that never began its correction is
	// naturally excluded before the predecessor-binding policy is even
	// reached, so exercising that policy requires forecasting first, exactly
	// as an abandoned in-progress correction would.
	if err := staleState.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	if _, err := staleStore.Replace(staleRevision, "review/begin-fix", staleState); err != nil {
		t.Fatal(err)
	}

	// Abandon commit1 entirely: reset back to commit0 so the stale lineage's
	// frozen candidate tree leaves HEAD's history for good.
	gitSnapshot(t, repo, "reset", "--hard", commit0)

	// commit2: a later, unrelated, properly approved delivery of the same
	// file, built directly from commit0 (never through the abandoned commit1).
	writeSnapshotFile(t, repo, "candidate.go", "package candidate\n\n"+
		"func value() int {\n\treturn 5\n}\n\n"+
		"func other() int {\n\treturn 2\n}\n\n"+
		"func third() int {\n\treturn 3\n}\n\n"+
		"func fourth() int {\n\treturn 4\n}\n")
	gitSnapshot(t, repo, "add", "candidate.go")
	gitSnapshot(t, repo, "commit", "-qm", "approved delivery")

	approvedState := newCompactFixtureStateForTarget(t, repo, "approved-lineage", Target{
		Kind: TargetBaseDiff, BaseRef: commit0Tree, IntendedUntracked: []string{},
	})
	if len(approvedState.SelectedLenses) == 0 {
		t.Fatal("approved fixture unexpectedly selected no lenses")
	}
	approvedState, approvedStore := startReviewingCompactFixture(t, repo, approvedState)
	approvedResults := make([]LensResult, len(approvedState.SelectedLenses))
	for index, lens := range approvedState.SelectedLenses {
		approvedResults[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	approvedState, approvedStarted := captureAndCompleteCompactReview(t, approvedStore, approvedState, CompactReviewInput{
		LensResults:     approvedResults,
		Classifications: []FindingEvidence{},
		RefuterOutcomes: []EvidenceResult{},
	})
	if approvedState.State == StateValidating {
		if err := approvedState.CloseCleanReviewOnLastEvent(); err != nil {
			t.Fatal(err)
		}
	}
	if approvedState.State != StateApproved {
		t.Fatalf("approved fixture state = %q, want %q", approvedState.State, StateApproved)
	}
	if _, err := approvedStore.Replace(approvedStarted.Revision, "review/complete-review", approvedState); err != nil {
		t.Fatal(err)
	}

	// commit3: the small, live, unrelated follow-up candidate, committed
	// directly on top of the approved delivery: a single one-line edit.
	writeSnapshotFile(t, repo, "candidate.go", "package candidate\n\n"+
		"func value() int {\n\treturn 6\n}\n\n"+
		"func other() int {\n\treturn 2\n}\n\n"+
		"func third() int {\n\treturn 3\n}\n\n"+
		"func fourth() int {\n\treturn 4\n}\n")
	gitSnapshot(t, repo, "add", "candidate.go")
	gitSnapshot(t, repo, "commit", "-qm", "small unrelated follow-up")

	candidates, err := selectorlessCommittedBaseDiffCorrections(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		lineages := make([]string, len(candidates))
		for index, candidate := range candidates {
			lineages[index] = candidate.lineage
		}
		t.Fatalf("selectorless committed-base-diff corrections bound to %v, want no match for an unrelated predecessor", lineages)
	}

	status, err := AssessTargetStatus(ctx, repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Applicability != TargetApplicabilityUnrelated || status.Action != TargetStatusActionStart || status.LineageID != "" {
		t.Fatalf("status = %#v, want a fresh unrelated start with no bound lineage", status)
	}

	// The candidate's own diff (against its real immediate predecessor,
	// commit2) is a single one-line change, nowhere near the stale lineage's
	// much larger recorded OriginalChangedLines.
	freshSnapshot, err := builder.Build(ctx, Target{Kind: TargetBaseDiff, BaseRef: "HEAD~1", IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_, freshLines, err := builder.ClassifySnapshotRisk(ctx, freshSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if freshLines != 2 {
		t.Fatalf("candidate's own changed lines = %d, want 2 (one removed, one added line)", freshLines)
	}
	if freshLines >= staleOriginalLines {
		t.Fatalf("candidate's own changed lines (%d) is not distinctly smaller than the stale lineage's unrelated OriginalChangedLines (%d)", freshLines, staleOriginalLines)
	}
}

// TestSelectorlessCommittedBaseDiffCorrectionsPredecessorResolve pins
// R3-predecessor-resolve-fails-open: only the unambiguous "root commit, no
// parent" case cleanly reports no candidates; a real resolve failure must
// surface as an error instead.
func TestSelectorlessCommittedBaseDiffCorrectionsPredecessorResolve(t *testing.T) {
	requireSnapshotGit(t)
	ctx := context.Background()

	t.Run("root commit has no predecessor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		if hasParent, err := selectorlessCommittedBaseDiffHasParentCommit(ctx, repo); err != nil || hasParent {
			t.Fatalf("fixture precondition: hasParent=%v err=%v, want a true root commit", hasParent, err)
		}
		candidates, err := selectorlessCommittedBaseDiffCorrections(ctx, repo)
		if err != nil {
			t.Fatalf("root commit produced an error, want a clean empty result: %v", err)
		}
		if len(candidates) != 0 {
			t.Fatalf("root commit unexpectedly matched candidates: %#v", candidates)
		}
	})

	t.Run("real resolve failure fails closed", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "extra.txt", "extra\n")
		gitSnapshot(t, repo, "add", "extra.txt")
		gitSnapshot(t, repo, "commit", "-qm", "second")

		headPath := filepath.Join(repo, ".git", "HEAD")
		original, err := os.ReadFile(headPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(headPath, []byte("this is not a valid ref\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.WriteFile(headPath, original, 0o644) })

		if _, err := selectorlessCommittedBaseDiffAncestors(ctx, repo); err == nil {
			t.Fatal("corrupted HEAD produced no error, want the resolve failure to surface")
		}
	})
}

// TestSelectorlessCommittedBaseDiffMatchedPredecessorWalksAncestry pins
// R3-single-commit-only-resumption: a lineage's frozen candidate several
// commits back from HEAD still matches, stopping at its own frozen base; a
// candidate tree that never appears in the ancestry does not match.
func TestSelectorlessCommittedBaseDiffMatchedPredecessorWalksAncestry(t *testing.T) {
	ancestors := []selectorlessCommittedBaseDiffAncestor{{commit: "c2", tree: "t2"}, {commit: "c1", tree: "t1"}, {commit: "c0", tree: "t0"}}
	resumed := CompactState{CurrentSnapshot: Snapshot{CandidateTree: "t1"}, InitialSnapshot: Snapshot{BaseTree: "t0"}}
	if got := selectorlessCommittedBaseDiffMatchedPredecessor(ancestors, resumed); got != "t1" {
		t.Fatalf("matched predecessor = %q, want t1 (two commits back)", got)
	}
	unrelated := CompactState{CurrentSnapshot: Snapshot{CandidateTree: "unrelated"}, InitialSnapshot: Snapshot{BaseTree: "t0"}}
	if got := selectorlessCommittedBaseDiffMatchedPredecessor(ancestors, unrelated); got != "" {
		t.Fatalf("matched predecessor = %q, want no match for a candidate tree absent from the ancestry", got)
	}
}

// TestSelectorlessCommittedBaseDiffAncestorsWalksMultipleCommits pins the
// ancestry-resolution half of the same fix: HEAD's first-parent ancestry
// resolves several commits deep, nearest to HEAD first.
func TestSelectorlessCommittedBaseDiffAncestorsWalksMultipleCommits(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	ctx := context.Background()
	var trees []string
	for _, name := range []string{"a", "b", "c"} {
		writeSnapshotFile(t, repo, name+".txt", name+"\n")
		gitSnapshot(t, repo, "add", name+".txt")
		gitSnapshot(t, repo, "commit", "-qm", name)
		trees = append(trees, strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}")))
	}
	ancestors, err := selectorlessCommittedBaseDiffAncestors(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ancestors) < 2 || ancestors[0].tree != trees[1] || ancestors[1].tree != trees[0] {
		t.Fatalf("ancestors = %#v, want [b, a] nearest-to-HEAD first", ancestors)
	}
}
