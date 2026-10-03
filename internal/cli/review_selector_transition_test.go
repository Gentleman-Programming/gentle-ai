package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
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
	authorization := recoveryAuthorizationFromStatus(t, probe, "selector-recovered", actor, reason)
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
	nativeStatus := selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--base-ref", base, "--recovery-successor-lineage", "selector-recovered")
	if len(nativeStatus.NextTransition.Execute.Arguments) != 6 {
		t.Fatal("base-diff recovery did not omit the authorization tuple")
	}
	payload := executeSelectorTransition(t, repo, nativeStatus)
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

type accountingRecoveryScenario struct {
	name, focus                                                                                string
	customPolicy, implicit, compatible, conflictFocus, conflictPolicy, changedTarget, hashOnly bool
	emptyFocus, wrongAuthorization, emptyAuthorization                                         bool
}

// TestStatusRecoverTransitionExecutesAccountingOnlyRecoveryWithoutSelectors
// drives the core decision through negotiated STATUS. The evidence-bound
// accounting-only edge deliberately carries no target selector, unlike an
// absent selector from an unrepresentable recovery.
func TestStatusRecoverTransitionExecutesAccountingOnlyRecoveryWithoutSelectors(t *testing.T) {
	for _, tc := range []accountingRecoveryScenario{
		{name: "default"},
		{name: "nondefault-focus", focus: "resilience"},
		{name: "custom-policy", customPolicy: true},
		{name: "implicit-frozen-shape", focus: "resilience", customPolicy: true, implicit: true},
		{name: "compatible-overrides", focus: "resilience", customPolicy: true, compatible: true},
		{name: "incompatible-focus", focus: "resilience", conflictFocus: true},
		{name: "incompatible-policy", customPolicy: true, conflictPolicy: true},
		{name: "changed-target", focus: "resilience", customPolicy: true, changedTarget: true},
		{name: "nullable-policy", focus: "resilience", hashOnly: true},
		{name: "empty-focus", focus: "resilience", emptyFocus: true},
		{name: "wrong-authorization", focus: "resilience", customPolicy: true, wrongAuthorization: true},
		{name: "empty-authorization", focus: "resilience", customPolicy: true, emptyAuthorization: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testAccountingOnlyRecoveryShape(t, tc)
		})
	}
}

func testAccountingOnlyRecoveryShape(t *testing.T, tc accountingRecoveryScenario) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n\n// historical candidate\n", 0o644)
	startArgs := []string{"--cwd", repo, "--lineage", "selector-accounting-only"}
	if tc.focus != "" {
		startArgs = append(startArgs, "--focus", tc.focus)
	}
	policyPath := t.TempDir() + "/policy.txt"
	if tc.customPolicy {
		if err := os.WriteFile(policyPath, []byte("private fixture policy bytes\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		startArgs = append(startArgs, "--policy", policyPath)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, startArgs)
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
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n\n// historical candidate\n// corrected candidate\n", 0o644)
	correction := requestedCorrectionSnapshot(t, repo, predecessor.State)
	nativeLines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), correction)
	if err != nil || nativeLines <= 0 || nativeLines > predecessor.State.CorrectionBudget {
		t.Fatalf("accounting-only correction lines = %d budget = %d err=%v", nativeLines, predecessor.State.CorrectionBudget, err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(
		context.Background(), repo, predecessor.State, predecessor.State.CapturePhaseRevision, correction,
	)
	if err != nil {
		t.Fatalf("build canonical targeted validation request: %v", err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(context.Background(), reviewtransaction.CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request, Payload: []byte(`{"outcome":"passed"}`),
	}); err != nil {
		t.Fatalf("capture canonical targeted validation result: %v", err)
	}
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Seed the historical/legacy persisted overflow directly. Current correction
	// completion rejects over-budget actuals before it mutates authority.
	policyHash, policyContent := predecessor.State.PolicyHash, predecessor.State.FrozenPolicyContent
	actual, fixHash := predecessor.State.CorrectionBudget+1, reviewtransaction.FixDeltaHashForSnapshot(correction)
	validation := reviewtransaction.ScopedValidationResult{
		LedgerIDs: predecessor.State.FixFindingIDs, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: []reviewtransaction.FollowUp{},
		OriginalCriteria:              reviewtransaction.ValidationCheck{EvidenceHash: facadePayloadHash([]byte("historical acceptance")), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression:          reviewtransaction.ValidationCheck{EvidenceHash: facadePayloadHash([]byte("historical regression")), FixDeltaHash: fixHash, Passed: true},
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: correction.Identity,
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
	if tc.hashOnly {
		predecessor.State.FrozenPolicyContent = nil
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

	if tc.changedTarget {
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 3 }\n", 0o644)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if tc.hashOnly {
		var output bytes.Buffer
		err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", started.LineageID}, &output)
		var failure ReviewIntegrationFailure
		decodeStrictReviewJSON(t, output.Bytes(), &failure)
		if err == nil || !strings.Contains(failure.Cause, "no frozen policy content") {
			t.Fatalf("impossible accounting recovery was advertised: %v, %#v", err, failure)
		}
		err = RunReviewRecover([]string{"--cwd", repo, "--predecessor-lineage", started.LineageID, "--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "hash-only-successor", "--disposition", "escalated"}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "no frozen policy content") {
			t.Fatalf("hash-only native recovery = %v", err)
		}
		after, _ := os.ReadFile(store.StatePath())
		stores, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
		if !bytes.Equal(before, after) || len(stores) != 1 {
			t.Fatal("hash-only refusal mutated authority")
		}
		return
	}
	probe := selectorTransitionStatus(t, repo, "--lineage", started.LineageID)
	if probe.NextTransition == nil || probe.NextTransition.Execute == nil {
		t.Fatalf("native STATUS must execute recovery: %#v", probe.NextTransition)
	}
	if probe.Action != reviewtransaction.TargetStatusActionRecover || probe.ActionDisposition != reviewtransaction.RecoveryEscalated {
		t.Fatalf("accounting-only status = %#v", probe)
	}
	const successor, actor, reason = "selector-accounting-successor", "maintainer", "recover accounting-only escalation"
	if tc.changedTarget {
		if err := RunReviewRecover([]string{"--cwd", repo, "--predecessor-lineage", started.LineageID,
			"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", successor, "--disposition", "escalated"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		successorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
		next, err := successorStore.Load()
		after, _ := os.ReadFile(store.StatePath())
		if err != nil || next.State.Recovery.Evidence != nil || next.State.PolicyHash == predecessor.State.PolicyHash || reflect.DeepEqual(next.State.SelectedLenses, predecessor.State.SelectedLenses) || !bytes.Equal(before, after) {
			t.Fatalf("changed target inherited frozen shape or mutated predecessor: %#v, %v", next, err)
		}
		return
	}
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
	recoverArgs, err := selectorTransitionCommandArguments(repo, status)
	if err != nil {
		t.Fatal(err)
	}
	if tc.implicit {
		status = selectorTransitionStatus(t, repo, "--lineage", started.LineageID, "--recovery-successor-lineage", successor)
		recoverArgs, err = selectorTransitionCommandArguments(repo, status)
		if err != nil || len(status.NextTransition.Execute.Arguments) != 4 {
			t.Fatalf("native accounting-only recovery argv: %v, %v", recoverArgs, err)
		}
	}
	if tc.compatible {
		recoverArgs = append(recoverArgs, "--focus", tc.focus, "--policy", policyPath)
	}
	conflictFlag := ""
	if tc.conflictFocus {
		recoverArgs, conflictFlag = append(recoverArgs, "--focus", "reliability"), "--focus"
	}
	if tc.conflictPolicy {
		recoverArgs, conflictFlag = append(recoverArgs, "--policy", ""), "--policy"
	}
	if tc.emptyFocus {
		t.Chdir(repo)
		for index, argument := range recoverArgs {
			if strings.HasPrefix(argument, "--cwd=") {
				recoverArgs = append(recoverArgs[:index], recoverArgs[index+1:]...)
				break
			}
		}
		recoverArgs, conflictFlag = append(recoverArgs, "--focus", ""), "--focus"
	}
	if tc.conflictFocus || tc.conflictPolicy || tc.emptyFocus {
		recoverArgs = append(recoverArgs, "--projection", "workspace", "--untracked-scope", "exclude", "--expected-untracked-inventory", probe.EligibleUntrackedInventory)
	}
	if tc.wrongAuthorization || tc.emptyAuthorization {
		arguments := setSelectorTransitionArgument(append([]ReviewTransitionArgument(nil), status.NextTransition.Execute.Arguments...), "maintainer-authorization", "")
		if tc.wrongAuthorization {
			arguments = setSelectorTransitionArgument(arguments, "maintainer-authorization", "wrong")
		}
		recoverArgs = []string{"recover", "--cwd", repo}
		for _, argument := range arguments {
			recoverArgs = append(recoverArgs, "--"+argument.Name, argument.Value)
		}
		conflictFlag = "--maintainer-authorization"
		if tc.emptyAuthorization {
			conflictFlag = "maintainer authorization"
		}
	}
	if tc.hashOnly {
		// The nullable pair must remain hash-only; core targeted validation
		// refuses it rather than letting RECOVER inject default policy bytes.
		conflictFlag = "no frozen policy content"
	}
	var output bytes.Buffer
	err = RunReview(recoverArgs, &output)
	after, _ := os.ReadFile(store.StatePath())
	if !bytes.Equal(before, after) {
		t.Fatal("RECOVER changed predecessor authority")
	}
	if conflictFlag != "" {
		if err == nil || !strings.Contains(err.Error(), conflictFlag) || strings.Contains(err.Error(), "private fixture policy bytes") {
			t.Fatalf("incompatible override refusal = %v", err)
		}
		stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
		if discoverErr != nil || len(stores) != 1 {
			t.Fatalf("refused override created successor: %v, %v", stores, discoverErr)
		}
		if !(tc.conflictFocus || tc.conflictPolicy || tc.emptyFocus) {
			return
		}
		_, command, named := strings.Cut(err.Error(), "re-run: ")
		if !named {
			// Exercise the old bare advice in RED, rather than just checking
			// whether the improved diagnostic's marker exists.
			_, command, named = strings.Cut(err.Error(), "rerun `")
			command = strings.TrimSuffix(command, "`")
		}
		if !named {
			t.Fatalf("conflict refusal names no continuation: %v", err)
		}
		words := reviewShellWords(t, command)
		if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" {
			t.Fatalf("conflict continuation is not a review command: %q", command)
		}
		recoverArgs = words[2:]
		output.Reset()
		if err := RunReview(recoverArgs, &output); err != nil {
			t.Fatalf("printed conflict continuation did not run: %v; command=%q", err, command)
		}
		want := []string{"recover", "--predecessor-lineage", started.LineageID,
			"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", successor,
			"--disposition", "escalated"}
		if !tc.emptyFocus {
			want = append(want, "--cwd", repo)
		}
		want = append(want, "--expected-untracked-inventory="+probe.EligibleUntrackedInventory,
			"--projection=workspace", "--untracked-scope=exclude")
		if !reflect.DeepEqual(recoverArgs, want) {
			t.Fatalf("conflict continuation lost bindings/selectors or retained overrides: %v, want %v", recoverArgs, want)
		}
		after, _ := os.ReadFile(store.StatePath())
		if !bytes.Equal(before, after) {
			t.Fatal("printed conflict continuation mutated predecessor")
		}
	} else if err != nil {
		t.Fatal(err)
	}
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, output.Bytes(), &recovered)
	if recovered.LineageID != successor || recovered.State != reviewtransaction.StateValidating || recovered.Recovery.Evidence == nil {
		t.Fatalf("accounting-only recovery = %#v", recovered)
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
	if err != nil {
		t.Fatal(err)
	}
	successorRecord, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), successorRecord.State.InitialSnapshot)
	if err != nil || successorRecord.State.RiskLevel != assessment.Level || successorRecord.State.OriginalChangedLines != assessment.ChangedLines || successorRecord.State.OriginalChangedLines == predecessor.State.OriginalChangedLines {
		t.Fatalf("recovery risk/lines are not fresh: %#v, %v", successorRecord.State, err)
	}
	if successorRecord.State.PolicyHash != predecessor.State.PolicyHash || !reflect.DeepEqual(successorRecord.State.FrozenPolicyContent, predecessor.State.FrozenPolicyContent) || !reflect.DeepEqual(successorRecord.State.SelectedLenses, predecessor.State.SelectedLenses) {
		t.Fatal("recovery did not preserve frozen policy/lenses")
	}
	if len(successorRecord.State.AdmittedRoleResults) != len(predecessor.State.AdmittedRoleResults) {
		t.Fatalf("accounting-only successor did not retain the canonical role references: %#v", successorRecord.State)
	}
	for index, reference := range recovered.Recovery.Evidence.AdmittedRoleReferences {
		admitted := successorRecord.State.AdmittedRoleResults[index]
		if admitted.Role != reference.Role || admitted.Lens != reference.Lens || admitted.SelectedOrder != reference.SelectedOrder ||
			admitted.TargetIdentity != reference.TargetIdentity || admitted.CapturePhaseRevision != reference.CapturePhaseRevision ||
			admitted.RequestHash != reference.RequestHash || admitted.ArtifactDigest != reference.ArtifactDigest {
			t.Fatalf("accounting-only reference %d does not resolve one canonical admitted entry: %#v", index, admitted)
		}
	}
	beforeReplay, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReview(recoverArgs, &output); err != nil {
		t.Fatal(err)
	}
	var replay ReviewRecoverResult
	decodeStrictReviewJSON(t, output.Bytes(), &replay)
	afterReplay, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if replay.StoreRevision != recovered.StoreRevision || !bytes.Equal(beforeReplay, afterReplay) {
		t.Fatalf("accounting-only recovery replay mutated authority: first=%#v replay=%#v", recovered, replay)
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
		probe.NextTransition == nil || probe.NextTransition.Execute == nil {
		t.Fatalf("correction-required staged scope probe = %#v", probe)
	}
	const successor, actor, reason = "correction-staged-successor", "maintainer", "authorize staged correction scope expansion"
	authorization := recoveryAuthorizationFromStatus(t, probe, successor, actor, reason)
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
	nativeStatus := selectorTransitionStatus(t, repo, append(selectors, "--recovery-successor-lineage", successor)...)
	if len(nativeStatus.NextTransition.Execute.Arguments) != 7 {
		t.Fatal("staged recovery did not retain exactly four core arguments and three selectors")
	}
	payload := executeSelectorTransition(t, repo, nativeStatus)
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
	if len(state.AdmittedRoleResults) != 0 || state.EvidenceHash != "" || state.ProposedCorrectionLines != nil ||
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

func TestStatusExecutesInvalidatedSameTargetCurrentChangesRecovery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "selector-current-invalidated"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewInvalidate([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", record.Revision,
		"--reason", "replace invalidated current-changes review",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.StatePath())
	storesBefore, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)

	status := selectorTransitionStatus(t, repo, "--lineage", started.LineageID)
	if status.Action != reviewtransaction.TargetStatusActionRecover ||
		status.ActionDisposition != reviewtransaction.RecoveryInvalidated ||
		status.TargetIdentity != reviewAuthorityTargetIdentity(status) {
		t.Fatalf("invalidated current-changes status = %#v", status)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.recover" {
		t.Fatalf("invalidated same-target current-changes transition = %#v", status.NextTransition)
	}
	arguments := selectorTransitionArguments(t, status)
	if len(arguments) != 4 || arguments["actor"] != "" || arguments["reason"] != "" || arguments["maintainer-authorization"] != "" {
		t.Fatalf("self-derived recovery arguments = %#v", arguments)
	}
	repeated := selectorTransitionStatus(t, repo, "--lineage", started.LineageID)
	if !reflect.DeepEqual(status.NextTransition, repeated.NextTransition) {
		t.Fatal("repeated STATUS changed the bound recovery")
	}
	after, _ := os.ReadFile(store.StatePath())
	storesAfter, _ := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if !bytes.Equal(before, after) || len(storesAfter) != len(storesBefore) {
		t.Fatalf("invalidated same-target STATUS mutated authority: stores before=%d after=%d", len(storesBefore), len(storesAfter))
	}
	payload := executeSelectorTransition(t, repo, status)
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &recovered)
	after, _ = os.ReadFile(store.StatePath())
	if recovered.LineageID != arguments["successor-lineage"] || recovered.TargetIdentity != status.TargetIdentity || !bytes.Equal(before, after) {
		t.Fatalf("printed recovery did not preserve its binding/predecessor: %#v", recovered)
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

// Explicit compatibility callers bind the same target as the native command;
// they no longer have to manufacture an external authorization collection.
func recoveryAuthorizationFromStatus(t *testing.T, status ReviewTargetStatusResult, successor, actor, reason string) string {
	t.Helper()
	arguments := selectorTransitionArguments(t, status)
	binding := status.NextTransition.Execute.Binding
	if status.Action != reviewtransaction.TargetStatusActionRecover || status.Authority == nil ||
		binding.LineageID != status.Authority.LineageID || binding.Revision != status.Authority.Revision ||
		binding.TargetIdentity != status.TargetIdentity || arguments["disposition"] != string(status.ActionDisposition) {
		t.Fatalf("recovery provider binding = %#v", status)
	}
	return strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + binding.LineageID,
		"predecessor_revision=" + binding.Revision,
		"target_identity=" + binding.TargetIdentity,
		"successor_lineage=" + successor,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
}

func TestNativeRecoveryIgnoresUnrelatedAuthorityDamage(t *testing.T) {
	reviewEnabledHome(t)
	for _, tc := range []struct{ name, version, file string }{
		{"ambiguous-legacy-lock", "v1", "LOCK"},
		{"malformed-compact-record", "v2", "review-state.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "healthy-recovery")
			store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, predecessor.State.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			foreignDir := filepath.Join(filepath.Dir(filepath.Dir(store.Dir)), tc.version, "unrelated-damaged")
			if err := os.MkdirAll(foreignDir, 0o755); err != nil {
				t.Fatal(err)
			}
			foreignPath := filepath.Join(foreignDir, tc.file)
			if err := os.WriteFile(foreignPath, []byte("not-json\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.version == "v1" {
				report, err := reviewtransaction.InventoryAuthority(t.Context(), repo)
				if err != nil || report.Complete {
					t.Fatalf("fixture must make the global inventory incomplete: %v, complete=%t", err, report.Complete)
				}
			} else {
				foreign, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, "unrelated-damaged")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := foreign.LoadContext(t.Context()); err == nil || reviewtransaction.IsCompactAuthorityOperationalFailure(err) {
					t.Fatalf("fixture must be quarantinable content damage: %v", err)
				}
			}
			status := selectorTransitionStatus(t, repo, "--lineage", predecessor.State.LineageID)
			if status.Action != reviewtransaction.TargetStatusActionRecover || status.NextTransition.Execute == nil || len(status.NextTransition.Execute.Arguments) != 4 {
				t.Fatalf("unrelated damage blocked healthy native recovery: %#v", status.NextTransition)
			}
			var recovered ReviewRecoverResult
			decodeStrictReviewJSON(t, executeSelectorTransition(t, repo, status), &recovered)
			after, err := os.ReadFile(store.StatePath())
			foreignAfter, foreignErr := os.ReadFile(foreignPath)
			if err != nil || foreignErr != nil || !bytes.Equal(before, after) || string(foreignAfter) != "not-json\n" || recovered.TargetIdentity != status.TargetIdentity {
				t.Fatalf("printed recovery changed predecessor/damaged authority or target: %#v, %v, %v", recovered, err, foreignErr)
			}
			var output bytes.Buffer
			err = RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", predecessor.State.LineageID}, &output)
			var failure ReviewIntegrationFailure
			decodeStrictReviewJSON(t, output.Bytes(), &failure)
			if err == nil || !strings.Contains(failure.Cause, "predecessor already has successor "+recovered.LineageID) {
				t.Fatalf("unrelated damage hid a valid child: %v, %#v", err, failure)
			}
		})
	}
}

func TestNativeRecoveryPropagatesOperationalAuthorityFailure(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "healthy-operational-probe")
	status := selectorTransitionStatus(t, repo, "--lineage", predecessor.State.LineageID)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, "unobservable-foreign")
	if err != nil {
		t.Fatal(err)
	}
	// A directory at the record path deterministically makes ReadFile fail;
	// unlike chmod this is reliable even when tests have elevated permissions.
	if err := os.MkdirAll(store.StatePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadContext(t.Context()); err == nil || !reviewtransaction.IsCompactAuthorityOperationalFailure(err) {
		t.Fatalf("fixture must prevent observation: %v", err)
	}
	successor, err := reviewStatusRecoverySuccessor(t.Context(), repo, status.NextTransition.Execute.Binding, "")
	if successor != "" || err == nil || !reviewtransaction.IsCompactAuthorityOperationalFailure(err) {
		t.Fatalf("ownership scan ignored operational failure: successor=%q err=%v", successor, err)
	}
}

func TestNativeRecoveryStatusRefusalsAreReadOnlyAndDiagnosticRuns(t *testing.T) {
	reviewEnabledHome(t)
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "native-recovery-refusals")
	t.Chdir(repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, predecessor.State.LineageID)
	before, _ := os.ReadFile(store.StatePath())
	assertRefusal := func(extra ...string) {
		t.Helper()
		var output bytes.Buffer
		err := RunReview(append([]string{"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", predecessor.State.LineageID}, extra...), &output)
		if err == nil {
			t.Fatalf("STATUS accepted %v: %s", extra, output.String())
		}
		var failure ReviewIntegrationFailure
		decodeStrictReviewJSON(t, output.Bytes(), &failure)
		if err := failure.Validate(); err != nil || failure.Code != "invalid_request" || failure.Phase != "preflight" {
			t.Fatalf("invalid refusal envelope: %#v, %v", failure, err)
		}
		_, command, named := strings.Cut(failure.Cause, "re-run: ")
		if !named || command != "gentle-ai review inspect-authority" {
			t.Fatalf("refusal lacks read-only diagnostic: %v", err)
		}
		if err := RunReview(reviewShellWords(t, command)[2:], io.Discard); err != nil {
			t.Fatalf("printed diagnostic refused: %v", err)
		}
		after, _ := os.ReadFile(store.StatePath())
		if !bytes.Equal(before, after) {
			t.Fatal("refused STATUS or diagnostic mutated predecessor")
		}
	}
	for _, extra := range [][]string{
		{"--recovery-authorization="}, {"--recovery-authorization=wrong"},
		{"--recovery-actor=maintainer"}, {"--recovery-reason=only-reason"}, {"--recovery-actor="},
		{"--recovery-successor-lineage=" + predecessor.State.LineageID},
	} {
		assertRefusal(extra...)
	}
	status := selectorTransitionStatus(t, repo, "--lineage", predecessor.State.LineageID)
	derived := selectorTransitionArguments(t, status)["successor-lineage"]
	// An unrelated authority at the derived name must not trigger a suffix search.
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", derived}, io.Discard); err != nil {
		t.Fatal(err)
	}
	assertRefusal()
	assertRefusal("--recovery-successor-lineage=" + derived)
	available := selectorTransitionStatus(t, repo, "--lineage", predecessor.State.LineageID, "--recovery-successor-lineage=explicit-available")
	executeSelectorTransition(t, repo, available)
	assertRefusal("--recovery-successor-lineage=another-name")
	stores, err := reviewtransaction.DiscoverCompactStores(t.Context(), repo)
	if err != nil || len(stores) != 3 {
		t.Fatalf("refusals forked authority: %d stores, %v", len(stores), err)
	}
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
