package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestStatusRecoverTransitionExecutesExactBaseDiffSelectors(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add candidate")
	// The predecessor is compact-v2 fixture setup for the RECOVER selector
	// behavior below. Construct it through the test-only legacy seam; production
	// STATUS and RECOVER remain the subject operations under test.
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", "selector-recover", "--base-ref", base, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "helper.go", "package candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "helper.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "expand candidate scope")
	probe := selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--base-ref", base)
	reason, actor := "approved scope expansion", "maintainer"
	authorization := recoveryAuthorizationFromCollection(t, probe, "selector-recovered", actor, reason)
	status := selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--base-ref", "  "+base+"  ",
		"--recovery-successor-lineage", "selector-recovered", "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	if status.TargetIdentity != probe.TargetIdentity || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" || status.NextTransition.Execute.Binding.TargetIdentity != probe.TargetIdentity {
		t.Fatalf("authorized base-diff recovery transition = %#v, probe = %#v", status.NextTransition, probe)
	}
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
	mixedAliasArgs, err := selectorTransitionCommandArguments(repo, status)
	if err != nil {
		t.Fatal(err)
	}
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
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", "selector-accounting-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a larger correction",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, predecessor.State.CorrectionBudget)
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	correction := requestedCorrectionSnapshot(t, repo, predecessor.State)
	nativeLines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), correction)
	if err != nil || nativeLines <= 0 || nativeLines > predecessor.State.CorrectionBudget {
		t.Fatalf("accounting-only correction lines = %d budget = %d err=%v", nativeLines, predecessor.State.CorrectionBudget, err)
	}
	// Seed the historical/legacy persisted overflow directly. Current correction
	// completion rejects over-budget actuals before it mutates authority.
	policyHash, policyContent := predecessor.State.PolicyHash, predecessor.State.FrozenPolicyContent
	actual, fixHash := predecessor.State.CorrectionBudget+1, reviewtransaction.FixDeltaHashForSnapshot(correction)
	validation := reviewtransaction.ScopedValidationResult{
		LedgerIDs: predecessor.State.FixFindingIDs, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: []reviewtransaction.FollowUp{},
		OriginalCriteria:              reviewtransaction.ValidationCheck{EvidenceHash: facadePayloadHash([]byte("historical acceptance")), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression:          reviewtransaction.ValidationCheck{EvidenceHash: facadePayloadHash([]byte("historical regression")), FixDeltaHash: fixHash, Passed: true},
		TargetedValidationRequestHash: facadePayloadHash([]byte("historical targeted validation request")), CorrectionTargetIdentity: correction.Identity,
	}
	original, regression := validation.OriginalCriteria, validation.CorrectionRegression
	predecessor.State.CorrectionAttempts = []reviewtransaction.CompactCorrectionAttempt{{Snapshot: correction, ProposedLines: *predecessor.State.ProposedCorrectionLines, ActualLines: actual, FixDeltaHash: fixHash, OriginalCriteria: original, CorrectionRegression: regression, TargetedValidationRequestHash: validation.TargetedValidationRequestHash, CorrectionTargetIdentity: correction.Identity}}
	predecessor.State.CumulativeCorrectionLines, predecessor.State.CurrentSnapshot, predecessor.State.FixDeltaHash = actual, correction, fixHash
	predecessor.State.ActualCorrectionLines, predecessor.State.OriginalCriteria, predecessor.State.CorrectionRegression = &actual, &original, &regression
	predecessor.State.State = reviewtransaction.StateEscalated
	if err := predecessor.State.Validate(); err != nil {
		t.Fatalf("validate historical/legacy accounting-only authority: %v", err)
	}
	if policyHash != "" && (predecessor.State.PolicyHash != policyHash ||
		policyContent != nil && (predecessor.State.FrozenPolicyContent == nil || *predecessor.State.FrozenPolicyContent != *policyContent)) {
		t.Fatal("historical/legacy fixture changed frozen policy content")
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
	reviewEnabledHome(t)
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

func TestStatusRecoverTransitionExecutesCorrectionRequiredStagedScopeExpansion(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int {\n\treturn 1\n}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add reviewed candidate")

	// The predecessor is compact-v2 fixture setup for the staged RECOVER
	// selector behavior below. Build it directly through the test-only legacy
	// seam so real STATUS and RECOVER remain the production operations under test.
	const predecessorLineage = "correction-staged-root"
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", predecessorLineage, "--base-ref", base, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) == 0 {
		t.Fatal("base-diff fixture selected no reviewer lenses")
	}
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:4", Severity: "CRITICAL", Claim: "candidate returns the wrong value",
				ProofRefs: []string{"candidate.go:4 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, predecessorLineage, 3)

	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessorLineage)
	if err != nil {
		t.Fatal(err)
	}
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
	authorization := recoveryAuthorizationFromCollection(t, probe, successor, actor, reason)
	status := selectorTransitionStatus(t, repo, append(selectors,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)...)
	if status.TargetIdentity != probe.TargetIdentity || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.recover" || status.NextTransition.Execute.Binding.TargetIdentity != probe.TargetIdentity {
		t.Fatalf("authorized staged correction recovery transition = %#v, probe = %#v", status.NextTransition, probe)
	}
	arguments := selectorTransitionArguments(t, status)
	if arguments["base-ref"] != base || arguments["projection"] != "staged" ||
		arguments["workspace-overlay"] != "true" || arguments["committed-only"] != "" {
		t.Fatalf("staged correction RECOVER selectors = %#v", arguments)
	}
	payload := executeSelectorTransition(t, repo, status)
	var recoveredResult ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &recoveredResult)
	if recoveredResult.TargetIdentity != status.TargetIdentity {
		t.Fatalf("staged correction RECOVER target = %#v, want %q", recoveredResult, status.TargetIdentity)
	}

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
		state.ActualCorrectionLines != nil {
		t.Fatalf("recovered successor inherited review or verification evidence: %#v", state)
	}
	stateAfter, _ := os.ReadFile(predecessorStore.StatePath())
	if !bytes.Equal(stateBefore, stateAfter) || strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree")) != wantTree {
		t.Fatal("staged correction recovery mutated predecessor authority or index")
	}
}

func TestCurrentChangesRecoverSelectorPresenceSurvivesJSONRoundTrip(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", "selector-current",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
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
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", "selector-unrepresentable",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate requires a helper",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
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
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add candidate")
	// The invalidated compact predecessor is fixture setup for the STATUS stop
	// behavior below, not a production START assertion.
	if err := runLegacyFacadeStartForTest(t, []string{
		"--cwd", repo, "--lineage", "selector-unchanged", "--base-ref", base, "--committed-only",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "selector-unchanged")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("execute selector transition: %v\n%s", err, payload)
	}
	return payload
}

func runSelectorTransition(repo string, status ReviewTargetStatusResult) ([]byte, error) {
	args, err := selectorTransitionCommandArguments(repo, status)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

func selectorTransitionCommandArguments(repo string, status ReviewTargetStatusResult) ([]string, error) {
	if status.NextTransition == nil || status.NextTransition.Execute == nil {
		reason := "<no transition>"
		if status.NextTransition != nil {
			reason = status.NextTransition.Kind + "/" + status.NextTransition.ReasonCode
		}
		return nil, fmt.Errorf("selector transition is not executable: %s; collect recovery authorization and re-query STATUS before execution", reason)
	}
	operation := strings.TrimPrefix(status.NextTransition.Execute.Operation, "review.")
	args := []string{operation, "--cwd=" + repo}
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Token == "" {
			return nil, fmt.Errorf("selector transition argument %q has no executable token", argument.Name)
		}
		args = append(args, argument.Token)
	}
	return args, nil
}

func recoveryAuthorizationFromCollection(t *testing.T, status ReviewTargetStatusResult, successor, actor, reason string) string {
	t.Helper()
	if status.Action != reviewtransaction.TargetStatusActionRecover || status.Authority == nil ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "recovery_authorization_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("recovery authorization collection = %#v", status)
	}
	input := status.NextTransition.Collect.Inputs[0]
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"lineage": status.Authority.LineageID, "expected-revision": status.Authority.Revision,
		"target": status.TargetIdentity, "disposition": string(status.ActionDisposition),
	}
	if input.Name != "recovery_authorization" || input.Schema != "gentle-ai.review-recovery-authorization/v1" ||
		input.CaptureOperation != "external.authorize_recovery" || !reflect.DeepEqual(arguments, want) {
		t.Fatalf("recovery authorization provider binding = %#v, want %#v", input, want)
	}
	return strings.Join([]string{
		input.Schema,
		"predecessor_lineage=" + arguments["lineage"],
		"predecessor_revision=" + arguments["expected-revision"],
		"target_identity=" + arguments["target"],
		"successor_lineage=" + successor,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
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
			if arguments[index].Token != "" {
				arguments[index].Token = reviewTransitionArgumentToken(arguments[index])
			}
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
