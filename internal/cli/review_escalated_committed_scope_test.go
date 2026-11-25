package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func buildReviewProbeBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "gentle-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binary, "./cmd/gentle-ai")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return binary
}

func invokeReviewProbe(t *testing.T, binary, repo string, args ...string) ([]byte, string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, append([]string{"review"}, args...)...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "DO_NOT_TRACK=1", "GENTLE_AI_TELEMETRY=0", "GENTLE_AI_NO_SELF_UPDATE=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); cmd.ProcessState == nil {
		t.Fatalf("run review %q: %v", args, err)
	}
	return stdout.Bytes(), stderr.String(), cmd.ProcessState.ExitCode()
}

func TestRejectedCommittedCorrectionStatusFromLinkedWorktree(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GENTLE_AI_BOUND_SELECTION_CONTRACT", "")
	binary := buildReviewProbeBinary(t)
	main := initReviewCLIRepo(t)
	repo := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, main, "worktree", "add", "-qb", "probe-linked", repo)
	const lineage = "rejected-committed-probe"
	writeUndeclaredWorkspaceFile(t, repo, "scratch.txt", "unrelated\n", 0o644)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	digest, _ := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	closure := closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage,
		"--untracked-scope=exclude", "--expected-untracked-inventory="+digest)
	if closure == nil || closure.StatusContinuation == nil {
		t.Fatal("reviewer closure did not issue a bound STATUS")
	}
	statusArgs := []string{"status"}
	for _, arg := range closure.StatusContinuation.Arguments {
		statusArgs = append(statusArgs, arg.Token)
	}
	run := func(args ...string) []byte {
		t.Helper()
		stdout, stderr, exit := invokeReviewProbe(t, binary, repo, args...)
		if exit != 0 || stderr != "" {
			t.Fatalf("review %q: exit=%d stderr=%q\n%s", args, exit, stderr, stdout)
		}
		return stdout
	}
	status := func(args ...string) ReviewTargetStatusResult {
		t.Helper()
		var result ReviewTargetStatusResult
		decodeStrictReviewJSON(t, run(args...), &result)
		if result.NextTransition == nil {
			t.Fatalf("review %q did not return a transition: %+v", args, result)
		}
		t.Logf("review %q: exit=0 scope=%s/%s action=%s/%s next_transition=%s/%s", args,
			result.Projection.Kind, result.Projection.Projection, result.Action, result.ActionDisposition, result.NextTransition.Kind, result.NextTransition.ReasonCode)
		return result
	}
	plan := status(statusArgs...).NextTransition
	if plan.ReasonCode != "correction_plan_required" || plan.Collect == nil || len(plan.Collect.Inputs) != 1 {
		t.Fatalf("want correction plan, got %+v", plan)
	}
	run(append([]string{"capture-correction-plan", "--correction-lines=2"}, reviewTransitionInputTokens(t, repo, plan.Collect.Inputs[0])...)...)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate", "--", "candidate.go")

	validation := status(append(statusArgs, "--agent=pi")...)
	if validation.ValidationRequest == nil || validation.NextTransition.ReasonCode != "targeted_validation_required" ||
		validation.NextTransition.Collect == nil || len(validation.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("want a targeted validation request: %+v", validation.NextTransition)
	}
	submission := validation.NextTransition.Collect.Inputs[0].Submission
	if submission == nil || submission.Value == nil {
		t.Fatalf("want a validator submission: %+v", submission)
	}
	request := validation.ValidationRequest
	payload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria: facadeValidationCheck{Passed: true, Evidence: []string{"original criterion passed"}},
		CorrectionRegression: facadeValidationCheck{Evidence: []string{"correction introduced a regression"}, Regressions: []reviewtransaction.Regression{{
			Location: "candidate.go:3", Claim: "caller still needs the previous value", ProofRefs: []string{"frozen candidate.go diff"},
		}}}, FollowUps: []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{submission.OperationToken}, submission.ArgumentTokens...)
	index := 1 + submission.Value.SubstitutionLocation
	args[index] = strings.Replace(args[index], "{{value}}", writeReviewCLIRawInput(t, payload), 1)
	var rejected reviewLastEventClosureResult
	decodeStrictReviewJSON(t, run(args...), &rejected)
	if rejected.State != reviewtransaction.StateEscalated || rejected.Escalation == nil || rejected.Escalation.Cause != "targeted_validator_rejected" {
		t.Fatalf("expected validator rejection: %+v", rejected)
	}
	t.Logf("review %q: exit=0 state=%s cause=%s", args, rejected.State, rejected.Escalation.Cause)

	preflight := []string{"status", "--contract=" + ReviewIntegrationContractV2, "--agent=pi", "--next-transition=true"}
	bound := append(append([]string(nil), preflight...), "--lineage="+lineage)
	withRange := append(append([]string(nil), bound...), "--base-ref="+base, "--committed-only=true")
	before := run("status")
	explicit := status(withRange...)
	for _, args := range [][]string{withRange, bound} {
		got := status(args...)
		if got.NextTransition.ReasonCode == "intended_untracked_selection_required" {
			input := got.NextTransition.Collect.Inputs[0]
			if input.Submission == nil || input.Submission.Value == nil {
				t.Fatalf("selection has no submission: %+v", input)
			}
			digest, _ := intendedUntrackedSelection(t, got)
			answer := `{"schema":"` + input.Schema + `","untracked_scope":"exclude","expected_untracked_inventory":"` + digest + `","intended_untracked":[]}`
			submissionArgs := append([]string{input.Submission.OperationToken}, input.Submission.ArgumentTokens...)
			index := 1 + input.Submission.Value.SubstitutionLocation
			submissionArgs[index] = strings.Replace(submissionArgs[index], "{{value}}", answer, 1)
			status(submissionArgs...)
		}
		if got.NextTransition.Kind != reviewNextTransitionStop || got.NextTransition.ReasonCode != "native_stop_required" ||
			got.Projection.Kind != reviewtransaction.TargetBaseDiff || got.Projection.BaseTree != base || got.Authority == nil ||
			got.Authority.Revision != rejected.StoreRevision || !reflect.DeepEqual(got.Escalation, rejected.Escalation) ||
			!reflect.DeepEqual(got, explicit) {
			t.Errorf("review %q did not retain the same terminal committed correction as explicit STATUS", args)
		}
	}
	if fresh := status(preflight...); fresh.Authority != nil || fresh.Projection.Kind != reviewtransaction.TargetCurrentChanges ||
		fresh.NextTransition.Kind != reviewNextTransitionCollect || fresh.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Error("fresh selectorless STATUS must still collect an untracked selection without borrowing the rejected lineage")
	}

	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 3 }\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "change rejected candidate", "--", "candidate.go")
	changed := status(bound...)
	if !reflect.DeepEqual(changed, status(withRange...)) || changed.TargetIdentity == explicit.TargetIdentity ||
		changed.NextTransition.ReasonCode == "native_stop_required" {
		t.Error("selectorless STATUS must assess changed HEAD against the frozen range, not stop on the rejected candidate")
	}
	if after := run("status"); !bytes.Equal(before, after) {
		t.Errorf("STATUS changed the authority inventory:\nbefore: %s\nafter: %s", before, after)
	}
}

// The base-diff kind denotes HEAD (plus explicitly selected untracked paths),
// not a workspace overlay. Omitting --committed-only in STATUS does not
// change what it projects or authorize a different kind of START.
func TestBaseRefWithoutCommittedOnlyCannotFreezeTrackedWorkspace(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	binary := buildReviewProbeBinary(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, repo, "docs/committed.md", "# Committed\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "committed range")
	head := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	writeReviewStartCandidate(t, repo, "tracked.txt", "staged workspace content\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	writeReviewStartCandidate(t, repo, "tracked.txt", "unstaged workspace content\n", 0o644)

	run := func(args ...string) []byte {
		t.Helper()
		stdout, stderr, exit := invokeReviewProbe(t, binary, repo, args...)
		if exit != 0 || stderr != "" {
			t.Fatalf("review %q: exit=%d stderr=%q\n%s", args, exit, stderr, stdout)
		}
		return stdout
	}
	before := run("status")
	args := []string{"status", "--contract=" + ReviewIntegrationContractV2, "--next-transition=true", "--agent=pi", "--base-ref=" + base}
	without := run(args...)
	if with := run(append(args, "--committed-only=true")...); !bytes.Equal(without, with) {
		t.Fatalf("omitting --committed-only changed the response:\nwithout: %s\nwith: %s", without, with)
	}
	var committed ReviewTargetStatusResult
	decodeStrictReviewJSON(t, without, &committed)
	if committed.Authority != nil || committed.Projection.Kind != reviewtransaction.TargetBaseDiff ||
		committed.Projection.CurrentCandidateTree != head || !reflect.DeepEqual(committed.Projection.Paths, []string{"docs/committed.md"}) ||
		committed.NextTransition == nil || committed.NextTransition.ReasonCode != "fresh_target_ready" ||
		committed.NextTransition.Execute == nil || committed.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("base-ref STATUS did not offer a committed-only START: %s", without)
	}
	t.Log("STATUS with --base-ref, with or without --committed-only: exit=0 base-diff at HEAD, execute/fresh_target_ready")

	start := invocationArgs(t, committed.NextTransition.Execute.Command)
	withoutAcknowledgement := []string{start[0]}
	for _, token := range start[1:] {
		if token != "--committed-only" && token != "--committed-only=true" {
			withoutAcknowledgement = append(withoutAcknowledgement, token)
		}
	}
	if len(withoutAcknowledgement) != len(start)-1 {
		t.Fatalf("base-ref STATUS did not render exactly one --committed-only option: %q", start)
	}
	stdout, stderr, exit := invokeReviewProbe(t, binary, repo, withoutAcknowledgement...)
	failure := decodeReviewIntegrationFailure(t, stdout)
	if exit != 1 || stderr != "Error: The negotiated review request is invalid. [invalid_request]\n" ||
		failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.Cause != "negotiated base-diff START requires exactly --base-ref and --committed-only" {
		t.Fatalf("negotiated START without --committed-only: exit=%d stderr=%q\n%s", exit, stderr, stdout)
	}
	t.Logf("negotiated START with --base-ref alone: exit=%d %s: %s", exit, failure.Code, failure.Cause)

	stdout, stderr, exit = invokeReviewProbe(t, binary, repo, "start", "--base-ref="+base)
	// The plain dispatcher may append the path of a local diagnostic report.
	cause, _, _ := strings.Cut(strings.TrimSuffix(stderr, "\n"), " A defect report was saved at ")
	if exit != 1 || len(stdout) != 0 || cause != "Error: review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope" {
		t.Fatalf("plain START without --committed-only: exit=%d stderr=%q stdout=%s", exit, stderr, stdout)
	}
	t.Logf("plain START with --base-ref alone on a dirty worktree: exit=%d %s", exit, cause)

	var overlay ReviewTargetStatusResult
	decodeStrictReviewJSON(t, run(append(args, "--workspace-overlay=true")...), &overlay)
	if overlay.Authority != nil || overlay.Projection.Kind != reviewtransaction.TargetBaseWorkspaceOverlay ||
		overlay.Projection.CurrentCandidateTree == head || !reflect.DeepEqual(overlay.Projection.Paths, []string{"docs/committed.md", "tracked.txt"}) {
		t.Fatalf("explicit overlay did not project tracked workspace content: %+v", overlay.Projection)
	}
	t.Log("STATUS with --workspace-overlay: exit=0 base-workspace-overlay including the dirty tracked file")
	if after := run("status"); !bytes.Equal(after, before) {
		t.Errorf("scope probes changed the authority inventory:\nbefore: %s\nafter: %s", before, after)
	}

	// A plain zero-lens START can omit the acknowledgement on a clean
	// worktree. The resulting lineage still freezes only the committed range.
	clean := initReviewCLIRepo(t)
	cleanBase := strings.TrimSpace(runReviewCLIGit(t, clean, "rev-parse", "HEAD"))
	writeReviewStartCandidate(t, clean, "docs/committed.md", "# Committed\n", 0o644)
	runReviewCLIGit(t, clean, "commit", "-qm", "clean committed range")
	cleanHead := strings.TrimSpace(runReviewCLIGit(t, clean, "rev-parse", "HEAD^{tree}"))
	stdout, stderr, exit = invokeReviewProbe(t, binary, clean, "start", "--base-ref="+cleanBase)
	if exit != 0 || stderr != "" {
		t.Fatalf("plain START on a clean worktree: exit=%d stderr=%q\n%s", exit, stderr, stdout)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, stdout, &started)
	if started.LineageID == "" || started.State != reviewtransaction.StateApproved {
		t.Fatalf("plain START without --committed-only did not approve the clean committed range: %s", stdout)
	}
	stdout, stderr, exit = invokeReviewProbe(t, binary, clean, "status", "--contract="+ReviewIntegrationContractV2, "--next-transition=true", "--agent=pi",
		"--base-ref="+cleanBase, "--lineage="+started.LineageID)
	if exit != 0 || stderr != "" {
		t.Fatalf("bound STATUS after plain START: exit=%d stderr=%q\n%s", exit, stderr, stdout)
	}
	var frozen ReviewTargetStatusResult
	decodeStrictReviewJSON(t, stdout, &frozen)
	if frozen.Authority == nil || frozen.Authority.LineageID != started.LineageID || frozen.Authority.State != reviewtransaction.StateApproved ||
		frozen.TargetIdentity != started.TargetIdentity || frozen.Projection.InitialSnapshotIdentity != started.TargetIdentity ||
		frozen.Projection.Kind != reviewtransaction.TargetBaseDiff || frozen.Projection.CurrentCandidateTree != cleanHead {
		t.Fatalf("plain START without --committed-only did not freeze a base-diff at HEAD: %s", stdout)
	}
	t.Logf("plain START with --base-ref alone on a clean worktree and bound STATUS: exit=0 %s/%s at HEAD", frozen.Projection.Kind, started.State)
}

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
	runStatus := func(t *testing.T, args []string) (ReviewTargetStatusResult, []byte) {
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
		return status, stdout.Bytes()
	}
	readStatus := func(t *testing.T, args []string) ReviewTargetStatusResult {
		t.Helper()
		status, raw := runStatus(t, args)
		if err := status.Validate(); err != nil {
			t.Fatalf("invalid negotiated STATUS: %v\n%s", err, raw)
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
		const boundSelectionEnv = "GENTLE_AI_BOUND_SELECTION_CONTRACT"
		t.Setenv(boundSelectionEnv, "gentle-ai.review-bound-selection/v1")
		status, raw := runStatus(t, validatorStatusArgs)
		digest, eligible := intendedUntrackedSelection(t, status)
		schema := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
		manual, manualRaw := runStatus(t, continuation)
		intendedUntrackedSelection(t, manual)
		if input := manual.NextTransition.Collect.Inputs[0]; input.Submission != nil {
			t.Fatalf("no-agent manual collect includes a submission: %+v", input.Submission)
		}
		validatePublishedReviewSchema(t, schema, manualRaw)
		// Pi 0d181e872 requests --agent=pi. It decoded this descriptor from
		// e2381dbc8 but rejects the whole STATUS when submission is absent.
		wantLegacy := &ReviewTransitionSubmission{
			OperationToken: "status",
			ArgumentTokens: []string{"--contract=gentle-ai.review-integration/v2", "--next-transition=true", "--agent=pi",
				"--base-ref=" + baseTree, "--committed-only=true", "--intended-untracked-selection={{value}}"},
			Value: &ReviewTransitionSubmissionValue{Slot: "intended_untracked_selection", Domain: "schema_bound_json",
				Schema: "gentle-ai.review-intended-untracked-selection/v1", SubstitutionLocation: 5},
		}
		for _, tt := range []struct{ name, value string }{
			{name: "missing-opt-in"},
			{name: "unknown-opt-in", value: "gentle-ai.review-bound-selection/v2"},
			{name: "inexact-opt-in", value: "gentle-ai.review-bound-selection/v1 "},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Setenv(boundSelectionEnv, tt.value)
				if tt.value == "" {
					if err := os.Unsetenv(boundSelectionEnv); err != nil {
						t.Fatal(err)
					}
				}
				legacy, raw := runStatus(t, validatorStatusArgs)
				intendedUntrackedSelection(t, legacy)
				input := &legacy.NextTransition.Collect.Inputs[0]
				if !reflect.DeepEqual(input.Submission, wantLegacy) {
					t.Fatalf("legacy Pi STATUS submission = %+v, want %+v", input.Submission, wantLegacy)
				}
				validatePublishedReviewSchema(t, schema, raw)
				input.Submission = status.NextTransition.Collect.Inputs[0].Submission
				if !reflect.DeepEqual(legacy, status) {
					t.Fatal("opt-in changed STATUS beyond the selection submission")
				}
			})
		}
		if status.NextTransition.ReasonCode != "intended_untracked_selection_required" ||
			status.Authority == nil || status.Authority.LineageID != lineage || status.Authority.Revision != rejected.StoreRevision ||
			digest != status.EligibleUntrackedInventory || !reflect.DeepEqual(eligible, []string{scratchName}) {
			t.Fatalf("changed candidate must collect an explicit declaration for its current binding: %#v", status)
		}
		args := append(append([]string{}, validatorStatusArgs...), "--untracked-scope=exclude",
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
			t.Fatalf("changed candidate must offer recovery with the exact predecessor and new target: %#v", recovery)
		}

		answer, err := json.Marshal(reviewIntendedUntrackedSelection{
			Schema: reviewIntendedUntrackedSelectionSchema, UntrackedScope: "exclude", ExpectedInventory: digest, Intended: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		replay := func(t *testing.T, selection ReviewTargetStatusResult) ReviewTargetStatusResult {
			t.Helper()
			submission := selection.NextTransition.Collect.Inputs[0].Submission
			if submission == nil || submission.OperationToken != "status" || submission.Value == nil {
				t.Fatalf("missing selected-STATUS descriptor: %#v", submission)
			}
			args := append([]string{submission.OperationToken, "--cwd", repo}, submission.ArgumentTokens...)
			index := 3 + submission.Value.SubstitutionLocation
			if index != len(args)-1 || args[index] != "--intended-untracked-selection="+reviewSubmissionValuePlaceholder {
				t.Fatalf("selection substitution must replace only the last token's value: %v", args)
			}
			args[index] = strings.Replace(args[index], reviewSubmissionValuePlaceholder, string(answer), 1)
			// START validation needs in-process selection state that is not on
			// the wire. RunReview already validates it before writing stdout.
			result, _ := runStatus(t, args)
			return result
		}
		t.Run("replay-without-opt-in", func(t *testing.T) {
			t.Setenv(boundSelectionEnv, "")
			if got := replay(t, status); !reflect.DeepEqual(got, recovery) {
				t.Fatalf("literal selected STATUS = action %q, authority %+v; want manual bound STATUS action %q, authority %+v",
					got.Action, got.Authority, recovery.Action, recovery.Authority)
			}
		})

		submission := status.NextTransition.Collect.Inputs[0].Submission
		wantSelector := []string{"--contract=" + ReviewIntegrationContractV2, "--next-transition=true", "--agent=pi",
			"--lineage=" + lineage, "--base-ref=" + baseTree, "--committed-only=true", "--intended-untracked-selection=" + reviewSubmissionValuePlaceholder}
		if !reflect.DeepEqual(submission.ArgumentTokens, wantSelector) {
			t.Fatalf("bound selection tokens = %v, want %v", submission.ArgumentTokens, wantSelector)
		}
		validatePublishedReviewSchema(t, schema, raw)
		for _, tt := range []struct {
			name, token string
			index       int
		}{
			{name: "foreign-lineage", token: "--lineage=foreign-lineage", index: 3},
			{name: "foreign-base", token: "--base-ref=" + conflictingTree, index: 4},
			{name: "foreign-scope", token: "--workspace-overlay=true", index: 5},
		} {
			t.Run(tt.name, func(t *testing.T) {
				var altered ReviewTargetStatusResult
				decodeStrictReviewJSON(t, raw, &altered)
				altered.NextTransition.Collect.Inputs[0].Submission.ArgumentTokens[tt.index] = tt.token
				if err := altered.Validate(); err == nil || !strings.Contains(err.Error(), "selected STATUS submission differs from") {
					t.Fatalf("mutated selected-STATUS envelope was not refused as a binding mismatch: %v", err)
				}
			})
		}
		t.Run("foreign-operation", func(t *testing.T) {
			var altered ReviewTargetStatusResult
			decodeStrictReviewJSON(t, raw, &altered)
			descriptor := altered.NextTransition.Collect.Inputs[0].Submission
			descriptor.OperationToken = "capture-result"
			descriptor.ArgumentTokens[6] = "--input=" + reviewSubmissionValuePlaceholder
			descriptor.Value = &ReviewTransitionSubmissionValue{Slot: "reviewer_result", Domain: "artifact_path_or_stdin", Schema: reviewReviewerSchemaID, SubstitutionLocation: 6}
			if err := altered.Validate(); err == nil || !strings.Contains(err.Error(), "selected STATUS submission differs from") {
				t.Fatalf("selection envelope accepted a different operation: %v", err)
			}
		})
		for _, freshLineage := range []string{"", "fresh-committed-range"} {
			name := "unbound-fresh-control"
			if freshLineage != "" {
				name = "named-fresh-control"
			}
			t.Run(name, func(t *testing.T) {
				args := []string{"status", "--cwd", repo, "--contract=" + ReviewIntegrationContractV2,
					"--next-transition=true", "--agent=pi", "--base-ref=" + baseTree, "--committed-only=true"}
				if freshLineage != "" {
					args = append(args, "--lineage="+freshLineage)
				}
				fresh, raw := runStatus(t, args)
				t.Run("same-as-default", func(t *testing.T) {
					t.Setenv(boundSelectionEnv, "")
					if _, defaultRaw := runStatus(t, args); !bytes.Equal(raw, defaultRaw) {
						t.Fatal("opt-in changed fresh STATUS bytes")
					}
				})
				validatePublishedReviewSchema(t, schema, raw)
				intendedUntrackedSelection(t, fresh)
				freshSubmission := fresh.NextTransition.Collect.Inputs[0].Submission
				want := append(append([]string{}, wantSelector[:3]...), wantSelector[4:]...)
				if fresh.Authority != nil || freshSubmission == nil || !reflect.DeepEqual(freshSubmission.ArgumentTokens, want) {
					t.Fatalf("fresh selection must keep its unbound form: authority=%+v submission=%+v", fresh.Authority, freshSubmission)
				}
				if got := replay(t, fresh); got.Authority != nil || got.TargetIdentity != recovery.TargetIdentity ||
					got.Action != reviewtransaction.TargetStatusActionStart || got.NextTransition == nil || got.NextTransition.Execute == nil ||
					got.NextTransition.Execute.Operation != "review.start" {
					t.Fatalf("fresh selection must offer START for the same target without authority: %#v", got)
				}
				fresh.NextTransition.Collect.Inputs[0].Submission = submission
				if err := fresh.Validate(); err == nil || !strings.Contains(err.Error(), "selected STATUS submission differs from its authority lineage") {
					t.Fatalf("fresh selection accepted a foreign authority binding: %v", err)
				}
			})
		}
		if after := readStatus(t, validatorStatusArgs); !reflect.DeepEqual(after, status) {
			t.Fatalf("selection changed bound STATUS: before authority=%+v, after authority=%+v", status.Authority, after.Authority)
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
