package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The approved runtime-context policy caps the complete reviewer context one
// runtime is handed at 200 KiB per supported runtime, independently of the
// unchanged native per-command Git diff ceiling
// (reviewtransaction.MaxFrozenCandidateDiffBytes). The fixtures below are
// pinned by size against both bounds so every test proves it exercises the
// narrow band the runtime policy owns: over the runtime cap, under the Git
// ceiling. The two numbers are restated here on purpose: the fixture must
// stay inside the band even if either bound later moves, and a silent drift
// would turn these tests into no-ops.
const (
	reviewRuntimeBudgetTestCapBytes     = 200 << 10
	reviewRuntimeBudgetTestCeilingBytes = reviewtransaction.MaxFrozenCandidateDiffBytes
)

// writeRuntimeBudgetOverCandidate writes one tracked candidate path whose
// complete patch is over the approved 200 KiB runtime cap but far under the
// 4 MiB Git ceiling.
func writeRuntimeBudgetOverCandidate(t *testing.T, repo string) int64 {
	t.Helper()
	size := writeRuntimeBudgetCandidate(t, repo, "runtime-budget-over.txt", 14_000)
	if size <= int64(reviewRuntimeBudgetTestCapBytes) || size >= int64(reviewRuntimeBudgetTestCeilingBytes) {
		t.Fatalf("over-budget fixture is %d bytes; it must sit over the %d byte runtime cap and under the %d byte Git ceiling to exercise the runtime policy", size, reviewRuntimeBudgetTestCapBytes, reviewRuntimeBudgetTestCeilingBytes)
	}
	return size
}

// writeRuntimeBudgetUnderCandidate writes one tracked candidate path whose
// complete block stays comfortably under the approved runtime cap.
func writeRuntimeBudgetUnderCandidate(t *testing.T, repo string) int64 {
	t.Helper()
	return writeRuntimeBudgetCandidate(t, repo, "runtime-budget-under.txt", 4_000)
}

func writeRuntimeBudgetCandidate(t *testing.T, repo, name string, lines int) int64 {
	t.Helper()
	body := strings.Repeat("runtime budget evidence line\n", lines)
	writeReviewStartCandidate(t, repo, name, body, 0o644)
	info, err := os.Stat(filepath.Join(repo, name))
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// TestNegotiatedStartRuntimeBudgetRefusesOver200KiBCandidateWithoutAuthority
// proves the runtime-context policy at the START decision point: a candidate
// whose complete reviewer evidence fits the unchanged Git ceiling but exceeds
// the approved per-runtime cap refuses before any authority exists, exactly
// like the over-ceiling refusal, because a reviewer that cannot hold the
// complete candidate must never launch on a partial view of it.
func TestNegotiatedStartRuntimeBudgetRefusesOver200KiBCandidateWithoutAuthority(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeRuntimeBudgetOverCandidate(t, repo)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "runtime-budget-start",
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-runtime-budget START error = %v\n%s", err, output.String())
	}
	if !strings.Contains(err.Error(), "no review authority was created") {
		t.Fatalf("over-runtime-budget START does not say that nothing was persisted: %v", err)
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("over-runtime-budget START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" ||
		failure.Failure.NextAction != "stop" || failure.Failure.Code != "lens_context_budget_exceeded" {
		t.Fatalf("over-runtime-budget START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if strings.Contains(output.String(), "GENTLE_AI_REVIEW_") {
		t.Fatalf("over-runtime-budget START refusal emitted reviewer evidence:\n%s", output.String())
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("over-runtime-budget START changed authority storage before create:\nbefore:\n%s\nafter:\n%s", authorityBefore, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("over-runtime-budget START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestNegotiatedStatusRuntimeBudgetClassifiesLegacyOverBudgetLineage proves
// the same runtime policy reaches STATUS and lens-context materialization for
// a lineage an older build already persisted: the shared probe classifies the
// candidate deterministically, refuses to materialize a partial view, and
// mutates nothing.
func TestNegotiatedStatusRuntimeBudgetClassifiesLegacyOverBudgetLineage(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeRuntimeBudgetOverCandidate(t, repo)
	record := startCompactAuthorityWithoutFacadeChecks(t, repo, "runtime-budget-status-legacy")
	handle := deriveLensContextHandle(t, repo, record)
	before := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
	homeBefore := readLegacyAuthorityTree(t, home)

	var context bytes.Buffer
	err := RunReview([]string{
		"lens-context", "--cwd", repo, "--repository-context", handle,
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--expected-revision", record.State.CapturePhaseRevision, "--lens", record.State.SelectedLenses[0],
	}, &context)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("lens-context refusal = %v, want deterministic runtime budget exhaustion", err)
	}
	if context.Len() != 0 {
		t.Fatalf("lens-context emitted %d bytes after budget refusal", context.Len())
	}
	if after := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); !reflect.DeepEqual(before, after) {
		t.Fatalf("lens-context budget refusal mutated authority: before=%#v after=%#v", before, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("lens-context budget refusal persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}

	status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
	if status.Action != reviewtransaction.TargetStatusActionStop ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "lens_context_budget_exceeded" ||
		status.NextTransition.Execute != nil || status.NextTransition.Collect != nil ||
		status.Forecast == nil || status.Forecast.Horizon != ForecastHorizonTerminal {
		t.Fatalf("STATUS reoffered a reviewer slot the runtime budget refuses: %#v", status)
	}
	if after := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); !reflect.DeepEqual(before, after) {
		t.Fatalf("STATUS budget classification mutated authority: before=%#v after=%#v", before, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("STATUS budget classification persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestNegotiatedStartRuntimeBudgetAdmitsCandidateUnder200KiB is the positive
// control for the policy: a candidate whose complete block fits the approved
// runtime cap still starts, still materializes, and its emitted block stays
// inside the cap.
func TestNegotiatedStartRuntimeBudgetAdmitsCandidateUnder200KiB(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeRuntimeBudgetUnderCandidate(t, repo)
	started := runNegotiatedReviewStart(t, repo, "runtime-budget-under")

	var output bytes.Buffer
	if err := RunReview([]string{
		"lens-context", "--cwd", repo, "--repository-context", started.RepositoryContext.Handle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision, "--lens", started.SelectedLenses[0],
	}, &output); err != nil {
		t.Fatalf("under-runtime-budget candidate was refused: %v", err)
	}
	if block := output.Len(); block == 0 || block > reviewRuntimeBudgetTestCapBytes {
		t.Fatalf("under-runtime-budget block = %d bytes, want a complete block inside the %d byte runtime cap", block, reviewRuntimeBudgetTestCapBytes)
	}
}

// writeRuntimeBudgetManyPathCandidate reproduces the shape #4680 reported: a
// candidate whose individual paths are all small and whose aggregate reviewer
// context sits between the runtime cap and the Git ceiling.
func writeRuntimeBudgetManyPathCandidate(t *testing.T, repo string, paths int) int64 {
	t.Helper()
	var total int64
	body := strings.Repeat("runtime budget evidence line\n", 200)
	// The reported candidate was mostly plain-text planning files with a
	// handful of code paths, and that code is what earns a lens plan at all:
	// an entirely non-executable candidate is classified low and never
	// materializes reviewer context.
	for index := range paths {
		name := fmt.Sprintf("docs/plan-%02d.md", index)
		if index < 13 {
			name = fmt.Sprintf("internal/plan%02d/plan.go", index)
		}
		writeReviewStartCandidate(t, repo, name, body, 0o644)
		info, err := os.Stat(filepath.Join(repo, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if total <= int64(reviewRuntimeBudgetTestCapBytes) || total >= int64(reviewRuntimeBudgetTestCeilingBytes) {
		t.Fatalf("many-path fixture is %d bytes across %d paths; it must sit over the %d byte runtime cap and under the %d byte Git ceiling", total, paths, reviewRuntimeBudgetTestCapBytes, reviewRuntimeBudgetTestCeilingBytes)
	}
	return total
}

// The reported candidate was 84 paths of ordinary text, no single one of them
// large. A per-path bound would admit it and strand the lineage exactly as
// #4680 describes, so the aggregate bound must refuse it before START
// persists anything.
func TestNegotiatedStartRuntimeBudgetRefusesManySmallPathsInAggregate(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeRuntimeBudgetManyPathCandidate(t, repo, 84)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "runtime-budget-many-paths",
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("many-small-path START error = %v\n%s", err, output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("many-small-path START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" {
		t.Fatalf("many-small-path START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("many-small-path START changed authority storage before create:\nbefore:\n%s\nafter:\n%s", authorityBefore, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("many-small-path START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// A candidate dominated by generated content is admitted on metadata
// summaries, so every later role must be materializable from those same
// summaries. If refuter evidence still read the complete lockfile patch, this
// candidate would pass START and then dead-end with authority already frozen,
// which is the unexecutable lineage #3367 closed and #4680 reopened.
func TestNegotiatedStartAdmitsGeneratedDominatedCandidateEveryRoleCanMaterialize(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lockfile := strings.Repeat("example.com/module v1.2.3 h1:0000000000000000000000000000000000000000000=\n", 5_000)
	writeReviewStartCandidate(t, repo, "go.sum", lockfile, 0o644)
	writeReviewStartCandidate(t, repo, "internal/auth/token.go", "package auth\n\nfunc Token() string { return \"candidate\" }\n", 0o644)
	info, err := os.Stat(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= int64(reviewRuntimeBudgetTestCapBytes) {
		t.Fatalf("generated fixture is %d bytes; its raw content must exceed the %d byte runtime cap for this test to mean anything", info.Size(), reviewRuntimeBudgetTestCapBytes)
	}

	started := runNegotiatedReviewStart(t, repo, "runtime-budget-generated")
	if len(started.SelectedLenses) == 0 {
		t.Fatal("generated-dominated candidate selected no lenses")
	}

	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := reviewProviderMaterializeEvidence(context.Background(), repo, "claude-code", snapshot)
	if err != nil {
		t.Fatalf("START admitted a candidate whose refuter evidence cannot be materialized: %v", err)
	}
	var total int
	for _, item := range evidence {
		total += len(item.Path) + len(item.Content)
	}
	if total > reviewRuntimeBudgetTestCapBytes {
		t.Fatalf("refuter evidence is %d bytes, over the %d byte runtime cap START admitted on", total, reviewRuntimeBudgetTestCapBytes)
	}
}

// writeRuntimeBudgetEscapedCandidate writes one tracked candidate whose raw
// bytes fit the runtime cap and whose JSON-escaped bytes do not. Every line is
// quote-dense on purpose: json.Marshal doubles each quote, so the same content
// that assembles as a raw lens block well under the cap serializes into a
// refuter or validator prompt well over it.
func writeRuntimeBudgetEscapedCandidate(t *testing.T, repo string) int64 {
	t.Helper()
	body := strings.Repeat(strings.Repeat(`"`, 28)+"\n", 5_000)
	writeReviewStartCandidate(t, repo, "runtime-budget-escaped.txt", body, 0o644)
	info, err := os.Stat(filepath.Join(repo, "runtime-budget-escaped.txt"))
	if err != nil {
		t.Fatal(err)
	}
	size := info.Size()
	if size >= int64(reviewRuntimeBudgetTestCapBytes) {
		t.Fatalf("escaped fixture is %d raw bytes; it must stay under the %d byte runtime cap so only escaping pushes it over", size, reviewRuntimeBudgetTestCapBytes)
	}
	return size
}

// TestNegotiatedStartRuntimeBudgetRefusesEscapedOverBudgetCandidate closes the
// envelope door into the unexecutable lineage of #3367 and #4680.
//
// START's probe assembles the lens block, which carries patch bytes raw. The
// refuter and targeted validator prompts carry the same bytes through
// json.Marshal, which doubles every quote, backslash, newline and tab before
// the role instruction and result schema are added. A quote-dense candidate
// therefore passed the raw probe and dead-ended at role materialization with
// authority already frozen -- and once one lens result is persisted,
// compactPristineReviewing is false, so review invalidate is gone too. The
// probe must measure the envelope those roles are actually held to.
func TestNegotiatedStartRuntimeBudgetRefusesEscapedOverBudgetCandidate(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeRuntimeBudgetEscapedCandidate(t, repo)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "runtime-budget-escaped",
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("escaped over-budget START error = %v; the candidate fits raw and cannot fit any role prompt\n%s", err, output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("escaped over-budget START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" ||
		failure.Failure.NextAction != "stop" || failure.Failure.Code != "lens_context_budget_exceeded" {
		t.Fatalf("escaped over-budget START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("escaped over-budget START changed authority storage before create:\nbefore:\n%s\nafter:\n%s", authorityBefore, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("escaped over-budget START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestNegotiatedStartRuntimeBudgetRefusesOverBudgetFrozenPolicy proves the
// envelope floor charges the frozen policy body.
//
// START reads the policy before it derives anything and freezes it into the
// authority, and the real targeted-validator prompt carries it twice: appended
// raw to the instruction and again JSON-escaped inside the marshalled request.
// The policy file itself has no size bound. A floor that charged it zero let a
// large --policy freeze authority on candidate evidence that fits, and then
// fail every validator capture deterministically -- with the lens result
// already persisted, so review invalidate was gone. That is the dead-end this
// issue closes, reached through the validator role.
func TestNegotiatedStartRuntimeBudgetRefusesOverBudgetFrozenPolicy(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	// Small candidate: its own evidence is nowhere near the cap, so only the
	// policy can push the validator envelope over it.
	writeRuntimeBudgetCandidate(t, repo, "runtime-budget-policy.txt", 200)
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte(strings.Repeat("frozen review policy line\n", 5_000)), 0o644); err != nil {
		t.Fatal(err)
	}
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "runtime-budget-policy", "--policy", policy,
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget frozen policy START error = %v; the validator prompt carries this policy twice and cannot fit\n%s", err, output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("over-budget frozen policy START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" {
		t.Fatalf("over-budget frozen policy START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("over-budget frozen policy START changed authority storage before create")
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("over-budget frozen policy START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestRecoveredOverBudgetLineageStopsTypedAndKeepsItsExit answers the one
// question this issue's guard cannot answer on its own: `review recover` mints
// a successor authority from a new snapshot with new lenses, and it runs no
// budget check (the START guard has exactly one call site,
// review_facade.go:2193). So recover really can create a REVIEWING authority
// for a candidate no runtime can carry.
//
// That is not the dead-end this issue closes, and the difference is the exit,
// not the refusal. The #4680 shape is: authority frozen, a lens result already
// persisted, compactPristineReviewing therefore false, `review invalidate`
// gone, every capture failing forever. A recovered lineage lands at the next
// generation with zero admitted role results, so it stays pristine and keeps
// its non-destructive exit, and STATUS classifies it with the same typed stop
// rather than reoffering a slot nothing can fill.
//
// This is written as execution rather than prose on purpose: the claim "recover
// is out of scope" is only worth as much as the exit it depends on, and that
// exit is a behavior, not a comment.
func TestRecoveredOverBudgetLineageStopsTypedAndKeepsItsExit(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-over-budget")

	// Push the successor scope far past the 200 KiB runtime cap while staying
	// under the Git ceiling, so the budget is what classifies it.
	if err := os.WriteFile(filepath.Join(repo, "huge.txt"),
		[]byte(strings.Repeat("runtime budget evidence line\n", 14_000)), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "huge.txt")
	runReviewCLIGit(t, repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "oversized successor scope")

	successorIdentity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, baseRef)
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, successorIdentity,
		"maintainer", "recover into oversized scope")

	var recovered bytes.Buffer
	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-over-budget-successor",
		"--disposition", "escalated", "--reason", "recover into oversized scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization", authorization,
	}, &recovered); err != nil {
		t.Fatalf("recover refused the oversized successor: %v\nIf recover ever grows its own budget refusal, this test documents the behavior that changed.", err)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-over-budget-successor")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("recovered successor state = %q, want reviewing", record.State.State)
	}
	if len(record.State.AdmittedRoleResults) != 0 {
		t.Fatalf("recovered successor already carries %d admitted role results, so it is not pristine and the exit below is not guaranteed", len(record.State.AdmittedRoleResults))
	}

	// STATUS must classify it deterministically instead of reoffering a slot
	// nothing can fill.
	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "recover-over-budget-successor", "--next-transition",
	}, &status); err != nil {
		t.Fatalf("status on the recovered over-budget lineage failed: %v\n%s", err, status.String())
	}
	var parsed struct {
		NextTransition struct {
			Kind       string `json:"kind"`
			ReasonCode string `json:"reason_code"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal(status.Bytes(), &parsed); err != nil {
		t.Fatalf("decode status: %v\n%s", err, status.String())
	}
	if parsed.NextTransition.Kind != "stop" || parsed.NextTransition.ReasonCode != "lens_context_budget_exceeded" {
		t.Fatalf("recovered over-budget lineage next transition = %+v, want a typed budget stop", parsed.NextTransition)
	}

	// The exit. This is what keeps recover out of the dead-end class.
	var invalidated bytes.Buffer
	if err := RunReview([]string{
		"invalidate", "--cwd", repo, "--lineage", "recover-over-budget-successor",
		"--expected-revision", record.Revision, "--reason", "candidate cannot fit the runtime budget",
	}, &invalidated); err != nil {
		t.Fatalf("the recovered over-budget lineage has no non-destructive exit, which makes it the same dead-end this issue closes: %v\n%s", err, invalidated.String())
	}
}
