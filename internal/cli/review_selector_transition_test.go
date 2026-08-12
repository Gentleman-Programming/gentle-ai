package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestStatusValidateTransitionPreservesCustomPublicationBase(t *testing.T) {
	repo := initReviewCLIRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, remote, "init", "--bare", "-q")
	runReviewCLIGit(t, repo, "branch", "-M", "main")
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "push", "-qu", "origin", "main")
	runReviewCLIGit(t, repo, "branch", "release", "HEAD")
	runReviewCLIGit(t, repo, "push", "-q", "origin", "release")
	writeReviewStartCandidate(t, repo, "main-only.txt", "main movement\n", 0o644)
	runReviewCLIGit(t, repo, "add", "main-only.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance main")
	runReviewCLIGit(t, repo, "push", "-q", "origin", "main")
	runReviewCLIGit(t, repo, "switch", "-q", "release")
	writeReviewStartCandidate(t, repo, "docs/release.md", "# Release candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/release.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "add release candidate")

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "selector-validate", "--base-ref", "origin/release", "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "selector-validate"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	status := selectorTransitionStatus(t, repo, "--lineage", "selector-validate", "--gate", "pre-pr", "--base-ref", "  origin/release  ")
	arguments := selectorTransitionArguments(t, status)
	if arguments["base-ref"] != "origin/release" || arguments["committed-only"] != "" || arguments["projection"] != "" {
		t.Fatalf("VALIDATE selectors = %#v", arguments)
	}
	prePush := newReviewNextTransition(ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCurrent,
		Action:         reviewtransaction.TargetStatusActionValidate,
		TargetIdentity: "approved-target",
		Authority:      &ReviewTargetStatusAuthority{LineageID: "selector-validate", State: reviewtransaction.StateApproved},
		Receipt:        ReviewTargetStatusReceipt{Status: ReviewReceiptPresent},
	}, nil, nil, nil, nil, reviewNextTransitionInput{
		Gate: reviewtransaction.GatePrePush, Selector: &reviewTransitionSelector{BaseRef: "origin/release"},
	})
	prePushStatus := ReviewTargetStatusResult{NextTransition: &prePush}
	prePushArguments := selectorTransitionArguments(t, prePushStatus)
	if prePushStatus.NextTransition == nil || prePushStatus.NextTransition.Execute == nil || prePushStatus.NextTransition.Execute.Operation != "review.validate" ||
		prePushArguments["base-ref"] != "origin/release" || prePushArguments["committed-only"] != "" || prePushArguments["projection"] != "" {
		t.Fatalf("pre-push VALIDATE selectors = %#v", prePushStatus.NextTransition)
	}
	if err := validateReviewTransitionExecution(*prePushStatus.NextTransition.Execute, prePushArguments); err != nil {
		t.Fatalf("pre-push VALIDATE contract = %v", err)
	}
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return arguments[:len(arguments)-1]
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "base-ref", "origin/main")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "base-ref", filepath.Join(t.TempDir(), "main"))
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "base-ref", " origin/release")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: "origin/release"})
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return removeSelectorTransitionArgument(arguments, "gate")
	})
	unbound, transition, execution := status, *status.NextTransition, *status.NextTransition.Execute
	execution.SelectorArguments = nil
	transition.Execute, unbound.NextTransition = &execution, &transition
	if err := unbound.Validate(); err == nil {
		t.Fatal("status accepted a missing normalized selector")
	}
	duplicate := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "origin/release"))
	runReviewCLIGit(t, repo, "push", "-q", "origin", "release:duplicate")
	raw := selectorTransitionStatus(t, repo, "--lineage", "selector-validate", "--gate", "pre-pr", "--base-ref", duplicate)
	// Root 7 (#2471): a raw SHA at the pre-PR gate is a missing input, not a
	// terminal state. The reason code is unchanged and the kind now collects
	// the symbolic ref only the caller can choose.
	if raw.NextTransition.Kind != reviewNextTransitionCollect || raw.NextTransition.ReasonCode != "pre_pr_selector_unrepresentable" ||
		raw.NextTransition.Collect == nil || len(raw.NextTransition.Collect.Inputs) != 1 ||
		raw.NextTransition.Collect.Inputs[0].Name != "base_ref" {
		t.Fatalf("raw SHA pre-PR transition = %#v", raw.NextTransition)
	}
	assertReviewGateResult(t, executeSelectorTransition(t, repo, status), reviewtransaction.GateAllow)
}

func TestStatusAndValidateShareMergeBaseBoundPrePRTarget(t *testing.T) {
	repo := initReviewCLIRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, remote, "init", "--bare", "-q")
	runReviewCLIGit(t, repo, "branch", "-M", "main")
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "push", "-qu", "origin", "main")
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", base+"^{tree}"))
	runReviewCLIGit(t, repo, "checkout", "-qb", "feature")
	writeReviewStartCandidate(t, repo, "docs/feature.md", "# Feature\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/feature.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "docs: feature")
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "moving-base", "--base-ref", base, "--committed-only"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "moving-base"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	side := filepath.Join(t.TempDir(), "main-advance")
	runReviewCLIGit(t, repo, "clone", "-q", "--no-checkout", remote, side)
	runReviewCLIGit(t, side, "checkout", "-q", "-B", "main", "origin/main")
	runReviewCLIGit(t, side, "config", "user.email", "side@example.test")
	runReviewCLIGit(t, side, "config", "user.name", "Side")
	writeReviewStartCandidate(t, side, "docs/base.md", "# Base\n", 0o644)
	runReviewCLIGit(t, side, "add", "docs/base.md")
	runReviewCLIGit(t, side, "commit", "-qm", "docs: advance main")
	runReviewCLIGit(t, side, "push", "-q", "origin", "main")
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--gate", "pre-pr", "--base-ref", "origin/main"}, &bytes.Buffer{}); err == nil {
		t.Fatalf("unfetched advertised base STATUS error = %v", err)
	}
	runReviewCLIGit(t, repo, "fetch", "-q", "origin", "main")

	status := selectorTransitionStatus(t, repo, "--gate", "pre-pr", "--base-ref", "origin/main")
	if status.Action != reviewtransaction.TargetStatusActionValidate || status.Projection.BaseTree != baseTree ||
		status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.validate" {
		t.Fatalf("merge-base-bound pre-PR status = %#v", status)
	}
	assertReviewGateResult(t, executeSelectorTransition(t, repo, status), reviewtransaction.GateAllow)
	var direct bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-pr", "--base-ref", "origin/main"}, &direct); err != nil {
		t.Fatal(err)
	}
	assertReviewGateResult(t, direct.Bytes(), reviewtransaction.GateAllow)
}

func TestStatusRecoverTransitionExecutesExactBaseDiffSelectors(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add candidate")
	// This is SETUP for the RECOVER selector-transition behavior under test,
	// not the start refusal itself; the committed candidate selects a lens,
	// so a direct base-diff start now hits issue #2447's up-front refusal.
	// The negotiated envelope carries extra fields ReviewFacadeStartResult
	// does not declare, so a plain (non-strict) decode is used here.
	var output bytes.Buffer
	startArgs := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "selector-recover", "--base-ref", base,
	})
	if err := RunReview(startArgs, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "blocking.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
		ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed exact base diff"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "helper.go", "package candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "helper.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "expand candidate scope")
	probe := selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--base-ref", base)
	reason, actor := "approved scope expansion", "maintainer"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + started.LineageID + "\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity + "\nsuccessor_lineage=selector-recovered\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--base-ref", "  "+base+"  ",
		"--recovery-successor-lineage", "selector-recovered", "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	arguments := selectorTransitionArguments(t, status)
	if arguments["base-ref"] != base || arguments["committed-only"] != "true" || arguments["projection"] != "" {
		t.Fatalf("RECOVER selectors = %#v", arguments)
	}
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "committed-only", "false")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "base-ref", "HEAD")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "base-ref", " "+base)
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return append(arguments, ReviewTransitionArgument{Name: "base-ref", Value: base})
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return removeSelectorTransitionArgument(arguments, "committed-only")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "predecessor-lineage", "wrong-lineage")
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return append(arguments, ReviewTransitionArgument{Name: "projection", Value: "staged"})
	})
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return removeSelectorTransitionArgument(arguments, "reason")
	})
	before, _ := os.ReadFile(store.StatePath())
	storesBefore, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	substituted := status
	transition, execution := *status.NextTransition, *status.NextTransition.Execute
	execution.Arguments = setSelectorTransitionArgument(append([]ReviewTransitionArgument(nil), execution.Arguments...), "successor-lineage", "selector-substituted")
	transition.Execute, substituted.NextTransition = &execution, &transition
	if _, err := runSelectorTransition(repo, substituted); err == nil {
		t.Fatal("RECOVER accepted successor substitution")
	}
	storesAfter, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	afterRejected, _ := os.ReadFile(store.StatePath())
	if len(storesAfter) != len(storesBefore) || !bytes.Equal(before, afterRejected) {
		t.Fatal("rejected RECOVER mutated authority")
	}
	mixedAliasArgs := selectorTransitionCommandArguments(repo, status)
	mixedAliasArgs = append(mixedAliasArgs, "-base-ref=HEAD")
	if err := RunReview(mixedAliasArgs, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "repeats --base-ref") {
		t.Fatalf("mixed selector aliases error = %v", err)
	}
	storesAfterMixedAlias, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	afterMixedAlias, _ := os.ReadFile(store.StatePath())
	if len(storesAfterMixedAlias) != len(storesBefore) || !bytes.Equal(before, afterMixedAlias) {
		t.Fatal("mixed-alias RECOVER mutated authority")
	}
	payload := executeSelectorTransition(t, repo, status)
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &recovered)
	if recovered.LineageID != "selector-recovered" || recovered.TargetIdentity != status.TargetIdentity {
		t.Fatalf("RECOVER = %#v, want target %q", recovered, status.TargetIdentity)
	}
	after, _ := os.ReadFile(store.StatePath())
	if !bytes.Equal(before, after) || predecessor.Revision != probe.Authority.Revision {
		t.Fatal("RECOVER changed predecessor authority")
	}
}

// TestStatusRecoverTransitionExecutesAccountingOnlyRecoveryWithoutSelectors
// drives the core decision through negotiated STATUS. The evidence-bound
// accounting-only edge deliberately carries no target selector, unlike an
// absent selector from an unrepresentable recovery.
func TestStatusRecoverTransitionExecutesAccountingOnlyRecoveryWithoutSelectors(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, "selector-accounting-only")
	result := filepath.Join(t.TempDir(), "blocking.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a larger correction",
		ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed exact candidate"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", strconv.Itoa(predecessor.State.CorrectionBudget)}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	request := capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
	correction := requestedCorrectionSnapshot(t, repo, predecessor.State)
	nativeLines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), correction)
	if err != nil || nativeLines <= 0 || nativeLines > predecessor.State.CorrectionBudget {
		t.Fatalf("accounting-only correction lines = %d budget = %d err=%v", nativeLines, predecessor.State.CorrectionBudget, err)
	}
	validation, err := (facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	}).compact(reviewtransaction.FixDeltaHashForSnapshot(correction), predecessor.State.FixFindingIDs, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := predecessor.State.CompleteCorrection(correction, predecessor.State.CorrectionBudget+1, validation); err != nil {
		t.Fatal(err)
	}
	revision, err := reviewtransaction.CompactRevisionForState(predecessor.State)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := json.MarshalIndent(reviewtransaction.CompactRecord{
		Schema: "gentle-ai.review-state-record/v2", Revision: revision, State: predecessor.State,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), append(persisted, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := predecessor.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	predecessor, err = store.Load()
	if err != nil || predecessor.State.State != reviewtransaction.StateEscalated {
		t.Fatalf("accounting-only predecessor = %#v, %v", predecessor, err)
	}
	if _, err := reviewtransaction.AssessTargetStatus(context.Background(), repo, reviewtransaction.TargetStatusRequest{
		Target: reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: started.LineageID,
	}); err != nil {
		t.Fatalf("native accounting-only status: %v", err)
	}

	probe := selectorTransitionStatus(t, repo, "--lineage", started.LineageID)
	if probe.Action != reviewtransaction.TargetStatusActionRecover || probe.ActionDisposition != reviewtransaction.RecoveryEscalated {
		t.Fatalf("accounting-only status = %#v", probe)
	}
	const successor, actor, reason = "selector-accounting-successor", "maintainer", "recover accounting-only escalation"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + started.LineageID +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, "--lineage", started.LineageID,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	arguments := selectorTransitionArguments(t, status)
	for _, selector := range []string{"base-ref", "committed-only", "projection", "workspace-overlay"} {
		if value, ok := arguments[selector]; ok {
			t.Fatalf("accounting-only recovery emitted selector %q=%q in %#v", selector, value, arguments)
		}
	}
	if status.NextTransition.Execute.SelectorArguments != nil {
		t.Fatalf("accounting-only recovery selectors = %#v, want no selector arguments", status.NextTransition.Execute.SelectorArguments)
	}
	payload := executeSelectorTransition(t, repo, status)
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &recovered)
	if recovered.LineageID != successor || recovered.State != reviewtransaction.StateValidating || recovered.Recovery.Evidence == nil {
		t.Fatalf("accounting-only recovery = %#v", recovered)
	}
}

func TestStatusStopsFreshStagedWorkspaceOverlay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "docs/fresh.md", "# Fresh\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/fresh.md")
	status := selectorTransitionStatus(t, repo, "--action-eligibility", "--base-ref", base, "--projection", "staged", "--workspace-overlay")
	if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated || status.Action != reviewtransaction.TargetStatusActionStop ||
		status.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "staged_workspace_overlay_recovery_unavailable" ||
		status.Eligibility == nil || status.Eligibility.AllowedActions[0].Action != "stop" {
		t.Fatalf("fresh staged overlay status = %#v", status)
	}
}

func TestStatusRecoverTransitionExecutesApprovedStagedScopeExpansion(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "docs/candidate.md", "# Candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/candidate.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "add reviewed candidate")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "staged-scope-root", "--base-ref", base, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "staged-scope-root"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "staged-scope-root")
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	stateBefore, _ := os.ReadFile(store.StatePath())
	receiptBefore, _ := os.ReadFile(store.ReceiptPath())

	writeReviewStartCandidate(t, repo, "docs/extra.md", "# Extra\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/extra.md")
	writeReviewStartCandidate(t, repo, "tracked.txt", "unstaged divergence\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "scratch.txt", "untracked noise\n", 0o644)
	wantTree := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree"))
	selectors := []string{"--lineage", predecessor.State.LineageID, "--base-ref", base, "--projection", "staged", "--workspace-overlay"}
	probe := selectorTransitionStatus(t, repo, selectors...)
	if probe.Action != reviewtransaction.TargetStatusActionRecover ||
		probe.ActionDisposition != reviewtransaction.RecoveryScopeChanged ||
		probe.NextTransition == nil || probe.NextTransition.Collect == nil {
		t.Fatalf("staged scope probe = %#v", probe)
	}
	reason, actor, successor := "include staged release notes", "maintainer", "staged-scope-successor"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, append(selectors,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)...)
	arguments := selectorTransitionArguments(t, status)
	if arguments["base-ref"] != base || arguments["projection"] != "staged" ||
		arguments["workspace-overlay"] != "true" || arguments["committed-only"] != "" {
		t.Fatalf("staged RECOVER selectors = %#v", arguments)
	}
	for _, name := range []string{"base-ref", "projection", "workspace-overlay"} {
		name := name
		assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
			return removeSelectorTransitionArgument(arguments, name)
		})
	}
	assertSelectorTransitionMutationRejected(t, status, func(arguments []ReviewTransitionArgument) []ReviewTransitionArgument {
		return setSelectorTransitionArgument(arguments, "workspace-overlay", "false")
	})

	payload := executeSelectorTransition(t, repo, status)
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &recovered)
	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
	successorRecord, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.TargetIdentity != status.TargetIdentity ||
		successorRecord.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseWorkspaceOverlay ||
		successorRecord.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged ||
		successorRecord.State.InitialSnapshot.CandidateTree != wantTree ||
		!reflect.DeepEqual(successorRecord.State.GenesisPaths, []string{"docs/candidate.md", "docs/extra.md"}) {
		t.Fatalf("staged successor = %#v", successorRecord.State)
	}
	if _, err := os.Stat(successorStore.ReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh staged successor receipt = %v", err)
	}
	if stateAfter, _ := os.ReadFile(store.StatePath()); !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("staged RECOVER changed predecessor state")
	}
	if receiptAfter, _ := os.ReadFile(store.ReceiptPath()); !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("staged RECOVER changed predecessor receipt")
	}
	if got := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree")); got != wantTree {
		t.Fatalf("staged RECOVER changed index tree: got %s want %s", got, wantTree)
	}

	if err := RunReviewInvalidate([]string{
		"--cwd", repo, "--lineage", successor, "--expected-revision", successorRecord.Revision,
		"--reason", "replace invalidated staged review",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "docs/later.md", "# Later\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/later.md")
	laterSelectors := []string{"--lineage", successor, "--base-ref", base, "--projection", "staged", "--workspace-overlay"}
	laterProbe := selectorTransitionStatus(t, repo, laterSelectors...)
	laterLineage, laterReason := "staged-scope-later", "replace invalidated staged review"
	laterAuthorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + successor +
		"\npredecessor_revision=" + laterProbe.Authority.Revision + "\ntarget_identity=" + laterProbe.TargetIdentity +
		"\nsuccessor_lineage=" + laterLineage + "\nactor=" + actor + "\nreason=" + laterReason
	later := selectorTransitionStatus(t, repo, append(laterSelectors,
		"--recovery-successor-lineage", laterLineage, "--recovery-reason", laterReason,
		"--recovery-actor", actor, "--recovery-authorization", laterAuthorization)...)
	laterArguments := selectorTransitionArguments(t, later)
	if later.ActionDisposition != reviewtransaction.RecoveryInvalidated ||
		laterArguments["base-ref"] != base || laterArguments["projection"] != "staged" ||
		laterArguments["workspace-overlay"] != "true" {
		t.Fatalf("later staged recovery = %#v, selectors %#v", later, laterArguments)
	}
	executeSelectorTransition(t, repo, later)
	laterStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, laterLineage)
	laterRecord, err := laterStore.Load()
	if err != nil || laterRecord.State.Recovery == nil ||
		laterRecord.State.Recovery.Disposition != reviewtransaction.RecoveryInvalidated {
		t.Fatalf("later staged successor = %#v, %v", laterRecord, err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", laterLineage}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	approved := selectorTransitionStatus(t, repo, "--lineage", laterLineage, "--base-ref", base, "--projection", "staged", "--workspace-overlay")
	if approved.Action != reviewtransaction.TargetStatusActionValidate {
		t.Fatalf("finalized staged status = %#v", approved)
	}
	assertReviewGateResult(t, executeSelectorTransition(t, repo, approved), reviewtransaction.GateAllow)
	var postApply bytes.Buffer
	if err := RunReviewFacadeValidate([]string{
		"--cwd", repo, "--lineage", laterLineage, "--gate", string(reviewtransaction.GatePostApply),
	}, &postApply); err != nil {
		t.Fatal(err)
	}
	assertReviewGateResult(t, postApply.Bytes(), reviewtransaction.GateAllow)
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "direct-staged-start", "--base-ref", base, "--workspace-overlay", "--projection", "staged"}, &bytes.Buffer{}); err == nil {
		t.Fatal("direct staged workspace-overlay START succeeded")
	}
}

func TestStatusRecoverTransitionExecutesCorrectionRequiredStagedScopeExpansion(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int {\n\treturn 1\n}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add reviewed candidate")

	// This is SETUP for the RECOVER selector-transition behavior under test,
	// not the start refusal itself; the committed candidate selects a lens,
	// so a direct base-diff start now hits issue #2447's up-front refusal.
	// The negotiated envelope carries extra fields ReviewFacadeStartResult
	// does not declare, so a plain (non-strict) decode is used here.
	const predecessorLineage = "correction-staged-root"
	var startedOut bytes.Buffer
	startArgs := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", predecessorLineage, "--base-ref", base,
	})
	if err := RunReview(startArgs, &startedOut); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startedOut.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if len(started.SelectedLenses) == 0 {
		t.Fatal("base-diff fixture selected no reviewer lenses")
	}
	finalizeArgs := []string{"--cwd", repo, "--lineage", predecessorLineage}
	for index, lens := range started.SelectedLenses {
		result := facadeReviewerResult{Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact base diff"}}
		if index == 0 {
			result.Findings = []facadeFinding{{
				Location: "candidate.go:4", Severity: "CRITICAL", Claim: "candidate returns the wrong value",
				ProofRefs: []string{"candidate.go:4 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		path := filepath.Join(t.TempDir(), "reviewer.json")
		writeReviewCLIJSON(t, path, result)
		finalizeArgs = append(finalizeArgs, "--result", path)
	}
	if err := finalizeReviewCLIArgs(t, repo, finalizeArgs, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", predecessorLineage, "--correction-lines", "3",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	predecessorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessorLineage)
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	stateBefore, _ := os.ReadFile(predecessorStore.StatePath())
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int {\n\treturn 2\n}\n", 0o644)
	writeReviewStartCandidate(t, repo, "migration.sql", "CREATE TABLE recovered (id INTEGER);\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go", "migration.sql")
	writeReviewStartCandidate(t, repo, "tracked.txt", "unstaged divergence\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "scratch.txt", "untracked noise\n", 0o644)
	wantTree := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree"))

	selectors := []string{
		"--lineage", predecessorLineage, "--base-ref", base, "--projection", "staged", "--workspace-overlay",
	}
	probe := selectorTransitionStatus(t, repo, selectors...)
	if probe.Action != reviewtransaction.TargetStatusActionRecover || probe.ActionDisposition != reviewtransaction.RecoveryScopeChanged ||
		probe.NextTransition == nil || probe.NextTransition.Collect == nil {
		t.Fatalf("correction-required staged scope probe = %#v", probe)
	}
	const successor, actor, reason = "correction-staged-successor", "maintainer", "authorize staged correction scope expansion"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessorLineage +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, append(selectors,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)...)
	executeSelectorTransition(t, repo, status)

	successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
	recovered, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := recovered.State
	if state.State != reviewtransaction.StateReviewing || state.InitialSnapshot.Kind != reviewtransaction.TargetBaseWorkspaceOverlay ||
		state.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged || state.InitialSnapshot.CandidateTree != wantTree ||
		!reflect.DeepEqual(state.GenesisPaths, []string{"candidate.go", "migration.sql"}) {
		t.Fatalf("recovered staged successor = %#v", state)
	}
	if state.CorrectionBudget != predecessor.State.CorrectionBudget ||
		!state.CorrectionAttemptConsumed() || len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 ||
		state.Recovery == nil || state.Recovery.ConsumedCorrectionAttempts != 1 || state.Recovery.ConsumedCorrectionLines != 3 {
		t.Fatalf("recovered correction accounting = %#v, predecessor = %#v", state, predecessor.State)
	}
	if len(state.LensResults) != 0 || len(state.Findings) != 0 || state.EvidenceHash != "" || state.ProposedCorrectionLines != nil ||
		state.ActualCorrectionLines != nil || state.CorrectionVerificationTarget != nil {
		t.Fatalf("recovered successor inherited review or verification evidence: %#v", state)
	}
	if _, err := os.Stat(successorStore.ReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh correction successor receipt = %v", err)
	}
	stateAfter, _ := os.ReadFile(predecessorStore.StatePath())
	if !bytes.Equal(stateBefore, stateAfter) || strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree")) != wantTree {
		t.Fatal("staged correction recovery mutated predecessor authority or index")
	}
}

func TestCurrentChangesRecoverSelectorPresenceSurvivesJSONRoundTrip(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "selector-current"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	result := filepath.Join(t.TempDir(), "blocking.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
		ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed exact current changes"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "helper.go", "package candidate\n", 0o644)
	probe := selectorTransitionStatus(t, repo, "--lineage", record.State.LineageID)
	if probe.Authority == nil {
		t.Fatalf("current-changes recovery probe lacks authority: %#v", probe)
	}
	if probe.Action != reviewtransaction.TargetStatusActionRecover {
		t.Fatalf("current-changes recovery probe action = %q, target=%s authority=%s projection=%#v", probe.Action, probe.TargetIdentity, probe.AuthorityTargetIdentity, probe.Projection)
	}
	reason, actor, successor := "approved current scope", "maintainer", "selector-current-successor"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + record.State.LineageID +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + successor + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo,
		"--lineage", record.State.LineageID,
		"--recovery-successor-lineage", successor,
		"--recovery-reason", reason,
		"--recovery-actor", actor,
		"--recovery-authorization", authorization,
	)
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" {
		t.Fatalf("current-changes recovery transition = %#v", status.NextTransition)
	}
	selectors := status.NextTransition.Execute.SelectorArguments
	if selectors == nil || len(*selectors) != 0 {
		t.Fatalf("current-changes selectors = %#v, want explicit empty selector contract", selectors)
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte(`"selector_arguments":[]`)) {
		t.Fatalf("status JSON omitted explicit empty selectors: %s", payload)
	}
	var decoded ReviewTargetStatusResult
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.NextTransition == nil || decoded.NextTransition.Execute == nil ||
		decoded.NextTransition.Execute.SelectorArguments == nil ||
		len(*decoded.NextTransition.Execute.SelectorArguments) != 0 {
		t.Fatalf("round-tripped selectors = %#v", decoded.NextTransition)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped status validation: %v", err)
	}
	before, _ := os.ReadFile(store.StatePath())
	recoveredPayload := executeSelectorTransition(t, repo, decoded)
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, recoveredPayload, &recovered)
	if recovered.LineageID != successor || recovered.TargetIdentity != decoded.TargetIdentity {
		t.Fatalf("current-changes RECOVER = %#v", recovered)
	}
	after, _ := os.ReadFile(store.StatePath())
	if !bytes.Equal(before, after) {
		t.Fatal("current-changes RECOVER changed predecessor authority")
	}
}

func TestStatusStopsUnrepresentableRecoveryWithoutMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "selector-unrepresentable"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	result := filepath.Join(t.TempDir(), "blocking.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
		ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed exact current changes"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "helper.go", "package candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go", "helper.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "commit candidate")
	before, _ := os.ReadFile(store.StatePath())
	storesBefore, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	status := selectorTransitionStatus(t, repo, "--lineage", record.State.LineageID, "--base-ref", base)
	if status.Action != reviewtransaction.TargetStatusActionRecover {
		t.Fatalf("unrepresentable recovery status action = %q, target=%s authority=%s projection=%#v", status.Action, status.TargetIdentity, status.AuthorityTargetIdentity, status.Projection)
	}
	// Root 7 (#2471): an unrepresentable recovery selector is a missing input.
	// The mutation assertions below still hold: collecting an input persists
	// nothing, exactly as the stop it replaces did not.
	if status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "recovery_target_unrepresentable" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].Name != "recovery_target_selector" {
		t.Fatalf("unrepresentable recovery transition = %#v", status.NextTransition)
	}
	after, _ := os.ReadFile(store.StatePath())
	storesAfter, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if !bytes.Equal(before, after) || len(storesAfter) != len(storesBefore) {
		t.Fatal("unrepresentable recovery mutated authority")
	}
}

func TestTransitionSelectorFlagsRejectMixedAliases(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		args      []string
	}{
		{name: "validate base", operation: "review.validate", args: []string{"--base-ref=origin/release", "-base-ref", "origin/main"}},
		{name: "recover base", operation: "review.recover", args: []string{"-base-ref=HEAD^", "--base-ref=HEAD"}},
		{name: "recover committed", operation: "review.recover", args: []string{"--committed-only", "-committed-only=true"}},
		{name: "recover projection", operation: "review.recover", args: []string{"-projection=workspace", "--projection", "staged"}},
		{name: "recover workspace overlay", operation: "review.recover", args: []string{"--workspace-overlay", "-workspace-overlay=true"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateReviewTransitionSelectorFlagCounts(test.args, test.operation); err == nil {
				t.Fatal("mixed selector aliases accepted")
			}
		})
	}
}

func TestStatusStopsUnchangedBaseDiffRecoveryWithoutSuccessor(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add candidate")
	// This is SETUP for the STATUS/recover stop behavior under test, not the
	// start refusal itself; the committed candidate selects a lens, so a
	// direct base-diff start now hits issue #2447's up-front refusal.
	var output bytes.Buffer
	startArgs := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "selector-unchanged", "--base-ref", base,
	})
	if err := RunReview(startArgs, &output); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "selector-unchanged")
	record, _ := store.Load()
	if err := RunReviewInvalidate([]string{"--cwd", repo, "--lineage", record.State.LineageID, "--expected-revision", record.Revision, "--reason", "invalidate unchanged target"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Load()
	before, _ := os.ReadFile(store.StatePath())
	probe := selectorTransitionStatus(t, repo, "--lineage", record.State.LineageID, "--base-ref", base)
	reason, actor := "unchanged recovery", "maintainer"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + record.State.LineageID + "\npredecessor_revision=" + record.Revision + "\ntarget_identity=" + probe.TargetIdentity + "\nactor=" + actor + "\nreason=" + reason
	status := selectorTransitionStatus(t, repo, "--lineage", record.State.LineageID, "--base-ref", base,
		"--recovery-successor-lineage", "selector-unchanged-successor", "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	if status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "recovery_scope_unchanged" {
		t.Fatalf("unchanged recovery transition = %#v", status.NextTransition)
	}
	after, _ := os.ReadFile(store.StatePath())
	stores, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if !bytes.Equal(before, after) || len(stores) != 1 {
		t.Fatalf("unchanged recovery mutated authority: stores=%d", len(stores))
	}
}

func selectorTransitionStatus(t *testing.T, repo string, selectors ...string) ReviewTargetStatusResult {
	t.Helper()
	args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}
	args = append(args, selectors...)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func selectorTransitionArguments(t *testing.T, status ReviewTargetStatusResult) map[string]string {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Execute == nil {
		t.Fatalf("status lacks execute transition: %#v", status.NextTransition)
	}
	arguments, err := reviewTransitionArgumentMap(status.NextTransition.Execute.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	return arguments
}

func executeSelectorTransition(t *testing.T, repo string, status ReviewTargetStatusResult) []byte {
	t.Helper()
	payload, err := runSelectorTransition(repo, status)
	if err != nil {
		t.Fatalf("execute %s: %v\n%s", status.NextTransition.Execute.Operation, err, payload)
	}
	return payload
}

func runSelectorTransition(repo string, status ReviewTargetStatusResult) ([]byte, error) {
	args := selectorTransitionCommandArguments(repo, status)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

func selectorTransitionCommandArguments(repo string, status ReviewTargetStatusResult) []string {
	operation := strings.TrimPrefix(status.NextTransition.Execute.Operation, "review.")
	args := []string{operation, "--cwd=" + repo}
	for _, argument := range status.NextTransition.Execute.Arguments {
		args = append(args, "--"+argument.Name+"="+argument.Value)
	}
	return args
}

func assertSelectorTransitionMutationRejected(t *testing.T, status ReviewTargetStatusResult, mutate func([]ReviewTransitionArgument) []ReviewTransitionArgument) {
	t.Helper()
	invalid := status
	transition := *status.NextTransition
	execution := *status.NextTransition.Execute
	execution.Arguments = mutate(append([]ReviewTransitionArgument(nil), execution.Arguments...))
	transition.Execute, invalid.NextTransition = &execution, &transition
	if err := invalid.Validate(); err == nil {
		t.Fatalf("status accepted invalid transition arguments: %#v", execution.Arguments)
	}
}

func setSelectorTransitionArgument(arguments []ReviewTransitionArgument, name, value string) []ReviewTransitionArgument {
	for index := range arguments {
		if arguments[index].Name == name {
			arguments[index].Value = value
		}
	}
	return arguments
}

func removeSelectorTransitionArgument(arguments []ReviewTransitionArgument, name string) []ReviewTransitionArgument {
	filtered := arguments[:0]
	for _, argument := range arguments {
		if argument.Name != name {
			filtered = append(filtered, argument)
		}
	}
	return filtered
}

func requestedCorrectionSnapshot(t *testing.T, repo string, state reviewtransaction.CompactState) reviewtransaction.Snapshot {
	t.Helper()
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, Projection: state.InitialSnapshot.Projection,
		BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked,
		LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
