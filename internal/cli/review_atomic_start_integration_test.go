package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestAtomicStartIgnoresSiblingAuthorityAcrossVersions(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)

	for _, version := range []string{"v1", "v3"} {
		writeAtomicStartCorruptSibling(t, repo, version, "shared-lineage")
	}
	started := atomicStartV2(t, repo, "shared-lineage")
	store := atomicCompactStartStore(t, repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatalf("START did not create compact authority beside v1/v3 siblings: %v", err)
	}
	if record.State.InitialAtomicStart == nil {
		t.Fatal("atomic START did not persist its immutable compact binding")
	}
	if record.State.InitialAtomicStart.WorktreeIdentity == "" {
		t.Fatal("atomic START omitted the compact worktree binding")
	}
	for _, version := range []string{"v1", "v2", "v3"} {
		writeAtomicStartCorruptSibling(t, repo, version, "corrupt-"+version)
	}

	unrelated := atomicStartV2(t, repo, "atomic-target")
	if _, err := atomicCompactStartStore(t, repo, unrelated.LineageID).Load(); err != nil {
		t.Fatalf("START did not ignore unrelated corrupt siblings: %v", err)
	}
}

func TestOrdinaryCompactLifecycleIgnoresHistoricalSiblings(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)

	for _, version := range []string{"v1", "v3"} {
		writeAtomicStartCorruptSibling(t, repo, version, started.LineageID)
	}

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo,
		"--lineage", started.LineageID,
	}, &statusOutput); err != nil {
		t.Fatalf("ordinary STATUS beside historical siblings: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.Version != reviewtransaction.AuthorityVersionCompact ||
		status.Authority.State != reviewtransaction.StateReviewing || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("ordinary STATUS did not select the active compact authority: %#v", status)
	}

	for order := range started.SelectedLenses {
		captureCLIReviewerResult(t, repo, started, order)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true",
	}, io.Discard); err != nil {
		t.Fatalf("ordinary FINALIZE beside historical siblings: %v", err)
	}
}

func TestAtomicStartPlainRouteAlwaysCreatesCompact(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "plain-compact"}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Operation != "review/start" || result.Action != "created" || result.LineageID != "plain-compact" || result.State != reviewtransaction.StateReviewing {
		t.Fatalf("plain compact START = %#v", result)
	}
	if _, err := atomicCompactStartStore(t, repo, result.LineageID).Load(); err != nil {
		t.Fatalf("plain START did not create compact authority: %v", err)
	}
	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, result.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.LoadChain(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("real CLI START reached legacy v1 authority: %v", err)
	}
}

func TestAtomicStartLinkedWorktreesAreIndependentAndReplayExactly(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "--detach", linked, "HEAD")
	for _, root := range []string{repo, linked} {
		if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("same overlapping candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	first := atomicStartV2(t, repo, "")
	second := atomicStartV2(t, linked, "")
	if first.Action != "created" || second.Action != "created" {
		t.Fatalf("initial negotiated START actions = %q, %q, want created", first.Action, second.Action)
	}
	if first.LineageID == second.LineageID {
		t.Fatalf("linked worktrees reused lineage %q for the same bytes", first.LineageID)
	}
	if first.RepositoryContext == nil || second.RepositoryContext == nil {
		t.Fatalf("linked-worktree negotiated START omitted repository context: %#v %#v", first.RepositoryContext, second.RepositoryContext)
	}
	firstStore := atomicCompactStartStore(t, repo, first.LineageID)
	secondStore := atomicCompactStartStore(t, linked, second.LineageID)
	firstRecord, err := firstStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	secondRecord, err := secondStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if firstRecord.State.InitialAtomicStart == nil || secondRecord.State.InitialAtomicStart == nil {
		t.Fatal("linked worktree compact authorities omitted immutable START bindings")
	}
	if firstRecord.State.InitialAtomicStart.WorktreeIdentity == secondRecord.State.InitialAtomicStart.WorktreeIdentity {
		t.Fatal("linked worktree compact authorities share a worktree binding")
	}
	assertAtomicStartRepositoryContext(t, repo, first)
	assertAtomicStartRepositoryContext(t, linked, second)

	before, err := os.ReadFile(firstStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	replayed := atomicStartV2(t, repo, "")
	after, err := os.ReadFile(firstStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Action != "replayed" || replayed.LineageID != first.LineageID {
		t.Fatalf("exact active retry = %#v, want replay of %q", replayed, first.LineageID)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("exact active retry changed authority bytes")
	}
}

func TestAtomicStartBurnRecreateAndCrossWorktreeConflictStayScoped(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "--detach", linked, "HEAD")
	for _, root := range []string{repo, linked} {
		writeAtomicStartDocumentationCandidate(t, root)
	}

	first := atomicStartV2(t, repo, "")
	second := atomicStartV2(t, linked, "")
	firstStore := atomicCompactStartStore(t, repo, first.LineageID)
	secondStore := atomicCompactStartStore(t, linked, second.LineageID)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", first.LineageID}, &bytes.Buffer{}); err != nil {
		t.Fatalf("approve and burn first linked-worktree authority: %v", err)
	}
	if _, err := firstStore.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("approved first authority remains readable: %v", err)
	}
	if record, err := secondStore.Load(); err != nil || record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("burning first authority changed sibling authority: %#v, %v", record.State, err)
	}
	recreated := atomicStartV2(t, repo, first.LineageID)
	if recreated.Action != "created" || recreated.LineageID != first.LineageID {
		t.Fatalf("START after burn = %#v, want a fresh compact authority", recreated)
	}
	if _, err := firstStore.Load(); err != nil {
		t.Fatalf("recreated compact authority: %v", err)
	}

	before, err := os.ReadFile(secondStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", second.LineageID,
	}), &output)
	if err == nil {
		t.Fatal("same explicit lineage across linked worktrees unexpectedly started")
	}
	after, err := os.ReadFile(secondStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("cross-worktree lineage conflict mutated the active compact authority")
	}
}

func TestAtomicStartStatusLineageIsPureAndStableBeforeExecution(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)
	for _, version := range []string{"v1", "v2", "v3"} {
		writeAtomicStartCorruptSibling(t, repo, version, "unrelated-corrupt-"+version)
	}

	before := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
	first, firstOutput := atomicStatusStartLineage(t, repo)
	second, secondOutput := atomicStatusStartLineage(t, repo)
	after := snapshotAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
	if first == "" || first != second {
		t.Fatalf("STATUS fresh START lineages = %q, %q", first, second)
	}
	if !bytes.Equal(firstOutput, secondOutput) {
		t.Fatalf("repeated fresh STATUS changed bytes:\nfirst:\n%s\nsecond:\n%s", firstOutput, secondOutput)
	}
	if before != after {
		t.Fatalf("STATUS mutated authority storage before START:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestAtomicStatusSelectorlessIgnoresCorruptAuthorityButExplicitLineageReportsIt
// keeps selectorless STATUS bound to the current fresh candidate: unrelated
// corrupt authority must neither alter its fresh response nor be read or
// mutated. Naming that exact lineage is the explicit opt-in to report its
// corruption.
func TestAtomicStatusSelectorlessIgnoresCorruptAuthorityButExplicitLineageReportsIt(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)

	_, fresh := atomicStatusStartLineage(t, repo)
	const corruptLineage = "selectorless-status-corrupt"
	writeAtomicStartCorruptSibling(t, repo, "v2", corruptLineage)
	statePath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", corruptLineage, "review-state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	_, selectorless := atomicStatusStartLineage(t, repo)
	if !bytes.Equal(selectorless, fresh) {
		t.Fatalf("selectorless STATUS changed with unrelated corrupt authority:\nbefore:\n%s\nafter:\n%s", fresh, selectorless)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("selectorless STATUS mutated unrelated corrupt authority")
	}

	var explicit bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo,
		"--lineage", corruptLineage,
	}, &explicit); err != nil {
		t.Fatalf("explicit corrupt-lineage STATUS: %v\n%s", err, explicit.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, explicit.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "corrupted_or_unverifiable_authority" {
		t.Fatalf("explicit corrupt-lineage STATUS = %#v\n%s", status.NextTransition, explicit.String())
	}
}

func TestAtomicStartExplicitStatusCollectsCompactMediumAndHighLenses(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		path       string
		contents   string
		wantLenses int
	}{
		{name: "medium", path: "internal/atomic.go", contents: "package atomic\n", wantLenses: 1},
		{name: "high", path: "scripts/atomic.sh", contents: "#!/bin/sh\necho atomic\n", wantLenses: 4},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, testCase.path, testCase.contents, 0o644)
			started := atomicStartV2(t, repo, "compact-status-"+testCase.name)
			if len(started.SelectedLenses) != testCase.wantLenses {
				t.Fatalf("START selected lenses = %v, want %d", started.SelectedLenses, testCase.wantLenses)
			}

			var output bytes.Buffer
			if err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo, "--lineage", started.LineageID,
			}, &output); err != nil {
				t.Fatalf("explicit compact STATUS: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.Authority == nil || status.Authority.State != reviewtransaction.StateReviewing || status.NextTransition == nil ||
				status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" ||
				len(status.NextTransition.Collect.Inputs) != testCase.wantLenses {
				t.Fatalf("explicit compact STATUS = %#v", status)
			}
		})
	}
}

func TestAtomicStartDisabledPersistsNothing(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	writeAtomicStartDocumentationCandidate(t, repo)
	if err := RunReviewMode([]string{"disable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	lineage := "disabled-compact-start"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &bytes.Buffer{}); err == nil {
		t.Fatal("disabled START unexpectedly succeeded")
	}
	compact := atomicCompactStartStore(t, repo, lineage)
	if _, err := compact.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled START persisted compact authority: %v", err)
	}
}

func assertAtomicStartRepositoryContext(t *testing.T, wantRoot string, started ReviewIntegrationStartResult) {
	t.Helper()
	if started.Schema != ReviewIntegrationStartSchema || started.RepositoryContext == nil {
		t.Fatalf("atomic negotiated START schema/context = %#v", started)
	}
	writeAtomicStartCorruptSibling(t, wantRoot, "v3", started.LineageID)
	resolved, err := reviewtransaction.ResolveReviewRepositoryContext(context.Background(), started.RepositoryContext.Handle, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, Revision: started.RepositoryContext.Revision,
	})
	if err != nil {
		t.Fatalf("resolve compact repository context beside v3 sibling: %v", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(wantRoot) {
		t.Fatalf("repository context resolved %q, want selected worktree %q", resolved, wantRoot)
	}
}

func atomicStartV2(t *testing.T, repo, lineage string) ReviewIntegrationStartResult {
	t.Helper()
	args := []string{"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo}
	if lineage != "" {
		args = append(args, "--lineage", lineage)
	}
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, args), &output); err != nil {
		t.Fatalf("atomic START: %v\n%s", err, output.String())
	}
	return decodeNegotiatedReviewStart(t, output.Bytes())
}

func atomicCompactStartStore(t *testing.T, repo, lineage string) reviewtransaction.CompactStore {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func atomicStatusStartLineage(t *testing.T, repo string) (string, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo}, &output); err != nil {
		t.Fatalf("STATUS: %v\n%s", err, output.String())
	}
	var result ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.NextTransition == nil || result.NextTransition.Execute == nil || result.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("STATUS did not issue fresh START: %#v", result.NextTransition)
	}
	return result.NextTransition.Execute.Binding.LineageID, output.Bytes()
}

func writeAtomicStartDocumentationCandidate(t *testing.T, repo string) {
	t.Helper()
	lines := make([]byte, 0, 4096)
	for index := 0; index < 129; index++ {
		lines = append(lines, "ordinary documentation line\n"...)
	}
	writeReviewStartCandidate(t, repo, "docs/atomic-start.md", string(lines), 0o644)
}

func writeAtomicStartCorruptSibling(t *testing.T, repo, version, lineage string) {
	t.Helper()
	dir := filepath.Join(reviewCLIAuthorityRoot(t, repo), version, lineage)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
