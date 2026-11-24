package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// A rejected correction does not reopen the committed range's frozen empty
// untracked selection, even when the live workspace has untracked material.
func TestBoundEscalatedCommittedStatusPreservesFrozenUntrackedExclusion(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises the native review lifecycle in a disposable Git repository")
	}
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	t.Setenv("DO_NOT_TRACK", "1")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_OPTIONAL_LOCKS", "0")
	repo := initReviewCLIRepo(t)
	const lineage = "escalated-committed-scope-6221"
	baseCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	closure := closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage)
	if closure == nil || closure.StatusContinuation == nil || closure.StatusContinuation.Operation != "review.status" {
		t.Fatal("fixture did not emit a bound STATUS continuation")
	}
	continuation := []string{"status"}
	for _, argument := range closure.StatusContinuation.Arguments {
		continuation = append(continuation, argument.Token)
	}
	if !reviewArgsContain(continuation, "--base-ref="+baseTree) || !reviewArgsContain(continuation, "--committed-only=true") {
		t.Fatalf("emitted STATUS continuation = %v, want the frozen committed selector", continuation)
	}
	readStatus := func(t *testing.T, args []string) ReviewTargetStatusResult {
		t.Helper()
		stderr := captureReviewProcessStderr(t)
		var stdout bytes.Buffer
		if err := RunReview(args, &stdout); err != nil {
			t.Fatalf("negotiated STATUS %v: %v\n%s\n%s", args, err, stdout.String(), stderr())
		}
		if got := stderr(); got != "" {
			t.Fatalf("negotiated STATUS stderr = %q, want empty", got)
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, stdout.Bytes(), &status)
		if err := status.Validate(); err != nil {
			t.Fatalf("invalid negotiated STATUS: %v\n%s", err, stdout.String())
		}
		return status
	}

	plan := readStatus(t, continuation).NextTransition
	if plan == nil || plan.Kind != reviewNextTransitionCollect || plan.ReasonCode != "correction_plan_required" ||
		plan.Collect == nil || len(plan.Collect.Inputs) != 1 {
		t.Fatalf("fixture must offer a bound correction plan, got %#v", plan)
	}
	planArgs := append([]string{"capture-correction-plan"}, reviewTransitionInputTokens(t, repo, plan.Collect.Inputs[0])...)
	var stdout bytes.Buffer
	if err := RunReview(append(planArgs, "--correction-lines", "2"), &stdout); err != nil {
		t.Fatalf("capture offered two-line plan: %v\n%s", err, stdout.String())
	}
	conflictingBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	conflictingTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate", "--", "candidate.go")
	correctedHead := runReviewCLIGit(t, repo, "rev-parse", "HEAD")
	correctedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	// The shared range fixture predates a frozen runtime. Negotiate the Pi
	// host relay explicitly to obtain the validator request without a spawn.
	validatorStatusArgs := append(append([]string(nil), continuation...), "--agent=pi")
	validation := readStatus(t, validatorStatusArgs)
	request := validation.ValidationRequest
	if request == nil || validation.NextTransition == nil || validation.NextTransition.Kind != reviewNextTransitionCollect ||
		validation.NextTransition.ReasonCode != "targeted_validation_required" || request.LineageID != lineage ||
		request.CorrectionCandidateTree != correctedTree || !reflect.DeepEqual(request.CorrectionPaths, []string{"candidate.go"}) {
		t.Fatalf("fixture must offer targeted validation of the committed correction, got request=%#v transition=%#v", request, validation.NextTransition)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria: facadeValidationCheck{Passed: true, Evidence: []string{"original correction criteria pass"}},
		CorrectionRegression: facadeValidationCheck{Evidence: []string{"named correction regression"}, Regressions: []reviewtransaction.Regression{{
			ID: "value-regression", Location: "candidate.go:3", Claim: "the corrected value breaks its caller's expectation", ProofRefs: []string{"corrected candidate inspection"},
		}}}, FollowUps: []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, rejected, err := reviewProviderCloseTargetedValidatorRaw(t.Context(), repo, store, record.State, request.ExpectedRevision, payload)
	if err != nil || rejected == nil || rejected.State != reviewtransaction.StateEscalated ||
		rejected.StoreRevision == "" || rejected.TargetedValidatorEvidence == nil || rejected.Escalation == nil {
		t.Fatalf("fixture did not close with a canonical targeted validator rejection: %#v, %v", rejected, err)
	}

	const scratchName = "unrelated-scratch.txt"
	const scratchContents = "local-only notes\n"
	assertWorkspace := func(t *testing.T, wantHead, wantPorcelain string) {
		t.Helper()
		if got := runReviewCLIGit(t, repo, "rev-parse", "HEAD"); got != wantHead {
			t.Errorf("STATUS changed HEAD: got %q, want %q", got, wantHead)
		}
		if got := runReviewCLIGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all"); got != wantPorcelain {
			t.Errorf("STATUS changed the worktree or index: got %q, want %q", got, wantPorcelain)
		}
		if wantPorcelain != "" {
			if got, err := os.ReadFile(filepath.Join(repo, scratchName)); err != nil || string(got) != scratchContents {
				t.Errorf("STATUS changed the untracked file: got %q, %v", got, err)
			}
		}
	}
	assertBoundStatus := func(t *testing.T, status ReviewTargetStatusResult) {
		t.Helper()
		wantStop := ReviewNextTransition{Kind: reviewNextTransitionStop, ReasonCode: "native_stop_required"}
		if status.Action != reviewtransaction.TargetStatusActionStop || status.ActionDisposition != "" ||
			status.NextTransition == nil || *status.NextTransition != wantStop || status.ValidationRequest != nil {
			t.Errorf("unchanged rejected correction: action=%q disposition=%q transition=%+v; want stop/native_stop_required without an actionable continuation", status.Action, status.ActionDisposition, status.NextTransition)
		}
		if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
			status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateEscalated ||
			status.Authority.Revision != rejected.StoreRevision || !reflect.DeepEqual(status.Escalation, rejected.Escalation) {
			t.Errorf("STATUS changed escalated authority: applicability=%q authority=%+v escalation=%+v; want %s at %s with original escalation evidence", status.Applicability, status.Authority, status.Escalation, lineage, rejected.StoreRevision)
		}
		if status.TargetIdentity == "" || status.TargetIdentity != validation.TargetIdentity ||
			status.AuthorityTargetIdentity != request.CorrectionTargetIdentity ||
			status.Projection.Kind != reviewtransaction.TargetBaseDiff || status.Projection.BaseTree != baseTree ||
			status.Projection.CurrentCandidateTree != correctedTree || len(status.Projection.IntendedUntracked) != 0 ||
			len(status.Projection.Paths) != 1 || status.Projection.Paths[0] != "candidate.go" {
			t.Errorf("STATUS changed target or scope: live=%q authority=%q projection=%+v; want live=%q and rejected fix=%q", status.TargetIdentity, status.AuthorityTargetIdentity, status.Projection, validation.TargetIdentity, request.CorrectionTargetIdentity)
		}
	}

	readInventory := func() string {
		t.Helper()
		var inventory bytes.Buffer
		if err := RunReview([]string{"status", "--cwd", repo}, &inventory); err != nil {
			t.Fatal(err)
		}
		return inventory.String()
	}
	before := readInventory()
	clean := readStatus(t, continuation)
	t.Run("without-untracked", func(t *testing.T) {
		assertBoundStatus(t, clean)
		assertWorkspace(t, correctedHead, "")
	})
	writeUndeclaredWorkspaceFile(t, repo, scratchName, scratchContents, 0o644)
	for _, entry := range []struct {
		name string
		args []string
	}{
		{name: "emitted-continuation", args: continuation},
		{name: "pinned-base-commit", args: replaceReviewStatusBaseRef(continuation, baseCommit)},
	} {
		t.Run(entry.name, func(t *testing.T) {
			status := readStatus(t, entry.args)
			assertBoundStatus(t, status)
			if status.EligibleUntrackedInventory == "" || status.EligibleUntrackedInventory == clean.EligibleUntrackedInventory {
				t.Error("fixture's unrelated file did not change the live untracked inventory")
			}
			assertWorkspace(t, correctedHead, "?? "+scratchName+"\n")
		})
	}

	t.Run("conflicting-base", func(t *testing.T) {
		status := readStatus(t, replaceReviewStatusBaseRef(continuation, conflictingBase))
		if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated || status.Authority != nil ||
			status.AuthorityTargetIdentity != "" || status.Projection.BaseTree != conflictingTree || status.TargetIdentity == clean.TargetIdentity ||
			status.Action != reviewtransaction.TargetStatusActionStart || status.NextTransition == nil ||
			status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
			t.Errorf("conflicting base: applicability=%q authority=%+v base=%q target=%q action=%q transition=%+v; want a different, unrelated target with its own untracked selection", status.Applicability, status.Authority, status.Projection.BaseTree, status.TargetIdentity, status.Action, status.NextTransition)
		}
		if after := readStatus(t, continuation); !reflect.DeepEqual(after.Authority, clean.Authority) ||
			after.AuthorityTargetIdentity != clean.AuthorityTargetIdentity || !reflect.DeepEqual(after.Escalation, clean.Escalation) {
			t.Errorf("conflicting-base STATUS changed authority: before=%+v after=%+v", clean.Authority, after.Authority)
		}
		assertWorkspace(t, correctedHead, "?? "+scratchName+"\n")
	})

	t.Run("fresh-lineage", func(t *testing.T) {
		status := readStatus(t, []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
			"--next-transition", "--lineage", "fresh-committed-range", "--base-ref", baseCommit, "--committed-only"})
		if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated || status.Authority != nil ||
			status.AuthorityTargetIdentity != "" || status.TargetIdentity != clean.TargetIdentity || status.Projection.BaseTree != baseTree ||
			status.Action != reviewtransaction.TargetStatusActionStart || status.NextTransition == nil ||
			status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
			t.Errorf("fresh lineage must collect its own selection for the same candidate: %#v", status)
		}
		assertWorkspace(t, correctedHead, "?? "+scratchName+"\n")
	})

	t.Run("changed-committed-candidate", func(t *testing.T) {
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 3 }\n", 0o644)
		runReviewCLIGit(t, repo, "commit", "-qm", "change rejected candidate", "--", "candidate.go")
		changedHead := runReviewCLIGit(t, repo, "rev-parse", "HEAD")
		changedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
		status := readStatus(t, continuation)
		if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
			t.Fatalf("changed candidate must not inherit an untracked exclusion: %#v", status.NextTransition)
		}
		args := append(append([]string{}, continuation...), "--untracked-scope=exclude",
			"--expected-untracked-inventory="+status.EligibleUntrackedInventory)
		recovery := readStatus(t, args)
		wantBinding := ReviewTransitionBinding{LineageID: lineage, Revision: rejected.StoreRevision, TargetIdentity: status.TargetIdentity}
		if recovery.Action != reviewtransaction.TargetStatusActionRecover || recovery.ActionDisposition != reviewtransaction.RecoveryEscalated ||
			recovery.NextTransition == nil || recovery.NextTransition.Kind != reviewNextTransitionExecute ||
			recovery.NextTransition.ReasonCode != "recovery_authorized" || recovery.NextTransition.Execute == nil ||
			recovery.NextTransition.Execute.Operation != "review.recover" || recovery.NextTransition.Execute.Binding != wantBinding ||
			recovery.TargetIdentity != status.TargetIdentity || recovery.TargetIdentity == clean.TargetIdentity ||
			recovery.Projection.BaseTree != baseTree || recovery.Projection.CurrentCandidateTree != changedTree ||
			!reflect.DeepEqual(recovery.Authority, clean.Authority) || !reflect.DeepEqual(recovery.Escalation, clean.Escalation) {
			t.Errorf("changed candidate must offer recovery with the exact predecessor and new target: %#v", recovery)
		}
		assertWorkspace(t, changedHead, "?? "+scratchName+"\n")
	})

	t.Run("restored-original-tree", func(t *testing.T) {
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
		runReviewCLIGit(t, repo, "commit", "-qm", "restore original candidate", "--", "candidate.go")
		originalHead := runReviewCLIGit(t, repo, "rev-parse", "HEAD")
		status := readStatus(t, continuation)
		if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || !reflect.DeepEqual(status.Authority, clean.Authority) ||
			status.Projection.CurrentCandidateTree != conflictingTree || status.TargetIdentity == clean.TargetIdentity ||
			status.Action != reviewtransaction.TargetStatusActionCollect || status.NextTransition == nil ||
			status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
			t.Errorf("restored original tree must not inherit the rejected candidate's exclusion: %#v", status)
		}
		assertWorkspace(t, originalHead, "?? "+scratchName+"\n")
	})
	if after := readInventory(); after != before {
		t.Errorf("STATUS changed authority inventory:\nbefore: %s\nafter: %s", before, after)
	}
}
