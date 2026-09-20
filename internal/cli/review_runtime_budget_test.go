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

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
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

// TestRecoveredOverBudgetLineageStopsTypedAndArrivesWithItsExitIntact answers
// question this issue's guard cannot answer on its own: `review recover` mints
// a successor authority from a new snapshot with new lenses, and it runs no
// budget check (the START guard has exactly one call site,
// review_facade.go:2193). So recover really can create a REVIEWING authority
// for a candidate no runtime can carry.
//
// That is not the dead-end this issue closes AT THE MOMENT IT ARRIVES, and the
// difference is the exit, not the refusal. The #4680 shape is: authority
// frozen, a lens result already persisted, compactPristineReviewing therefore
// false, `review invalidate` gone, every capture failing forever. A recovered
// lineage lands at the next generation with zero admitted role results, so on
// arrival it is pristine and `review invalidate` accepts it, and STATUS
// classifies it with the same typed stop rather than reoffering a slot nothing
// can fill.
//
// What this test proves is exactly that arrival, and NOT an invariant. Two
// conditions hold here and are enforced nowhere, so the exit is losable:
//
//   - Nothing is captured in between. RunReviewCaptureResult is dispatched
//     straight from runReviewCommand and never consults this budget guard,
//     whose only production call site is STATUS (review_facade.go:1287). A
//     hand-built --input result admits on an over-budget candidate, and the
//     admitted result makes compactPristineReviewing false — the #4680
//     dead-end, reached on a recovered lineage.
//   - The worktree does not move. `review invalidate` also runs
//     rebuildCurrentSnapshotEvidence (compact_store.go:1691), so a pristine
//     zero-result lineage whose tree drifted cannot be invalidated at all. An
//     over-budget candidate is a large change the author keeps editing, which
//     makes that the ordinary case rather than the exotic one.
//
// Both are tracked separately. Naming them here keeps the guarantee this test
// actually carries — an exit on arrival — from being read as a durable one.
//
// This is written as execution rather than prose on purpose: the claim "recover
// is out of scope" is only worth as much as the exit it depends on, and that
// exit is a behavior, not a comment.
func TestRecoveredOverBudgetLineageStopsTypedAndArrivesWithItsExitIntact(t *testing.T) {
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

	// The exit, on arrival. This is what keeps a freshly recovered lineage out
	// of the dead-end class; see the conditions named above for what loses it.
	var invalidated bytes.Buffer
	if err := RunReview([]string{
		"invalidate", "--cwd", repo, "--lineage", "recover-over-budget-successor",
		"--expected-revision", record.Revision, "--reason", "candidate cannot fit the runtime budget",
	}, &invalidated); err != nil {
		t.Fatalf("the recovered over-budget lineage arrived without a non-destructive exit, which makes it the same dead-end this issue closes: %v\n%s", err, invalidated.String())
	}
}

// driveReviewToOpenTargetedValidationWithCorrection is
// driveReviewToOpenTargetedValidation's parameterized sibling: same drive to
// an open forecasted correction, but the corrected content is supplied by the
// caller so one fixture can produce both a correction whose validator request
// fits the runtime budget and one whose request cannot.
//
// The asymmetry is the whole #4680 shape and is deliberate here: START froze a
// two-line candidate and measured it, and the correction written afterwards is
// what the targeted validator's evidence is materialized from. Nothing START
// could have measured predicts it.
func driveReviewToOpenTargetedValidationWithCorrection(t *testing.T, lineage, correctedAlpha string) (repo string, record reviewtransaction.CompactRecord) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	for name, content := range map[string]string{
		"alpha.go": "package candidate\n\nfunc Alpha() int { return 1 }\n",
		"beta.go":  "package candidate\n\nfunc Beta() int { return 2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "alpha.go", "beta.go")

	var startOut bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", lineage, "--projection", "staged",
	}), &startOut); err != nil {
		t.Fatalf("start: %v\n%s", err, startOut.String())
	}
	started := decodeNegotiatedReviewStart(t, startOut.Bytes())
	captureStarted := ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity,
		SelectedLenses: started.SelectedLenses,
	}
	for index := range started.SelectedLenses {
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "alpha.go:3", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
				ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
				EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, captureStarted, index, findings, &bytes.Buffer{})
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 2)

	if err := os.WriteFile(filepath.Join(repo, "alpha.go"), []byte(correctedAlpha), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "alpha.go")

	_, record, err := discoverCompactFacadeReview(context.Background(), repo, started.LineageID, false)
	if err != nil {
		t.Fatalf("discover open correction authority: %v", err)
	}
	if record.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("fixture state = %q, want correction_required", record.State.State)
	}
	return repo, record
}

// The two corrections below are the #4680 asymmetry made concrete. START froze
// and measured a two-line candidate and admitted it honestly; what the
// targeted validator is finally handed is materialized from the CORRECTED
// snapshot instead, which no START-time probe measured. Both corrections
// change the same single line, so both stay inside the forecasted correction
// budget -- only their byte volume differs, which is exactly the property the
// correction-line budget does not bound.
const reviewCorrectionBudgetInBudgetCorrection = "package candidate\n\nfunc Alpha() int { return 10 }\n"

func reviewCorrectionBudgetOverBudgetCorrection(t *testing.T) string {
	t.Helper()
	corrected := "package candidate\n\nfunc Alpha() int { return 10 } // " +
		strings.Repeat("correction evidence no runtime can be asked to hold ", 6_000) + "\n"
	if len(corrected) <= reviewRuntimeBudgetTestCapBytes || len(corrected) >= reviewRuntimeBudgetTestCeilingBytes {
		t.Fatalf("over-budget correction is %d bytes; it must sit over the %d byte runtime cap and under the %d byte Git ceiling", len(corrected), reviewRuntimeBudgetTestCapBytes, reviewRuntimeBudgetTestCeilingBytes)
	}
	return corrected
}

// TestOverBudgetCorrectionStopsTypedInsteadOfReofferingTargetedValidation is
// the #4680 defect itself, by execution. START admitted a two-line candidate
// honestly; the correction written afterwards makes the targeted validator
// request unassemblable. Before this change STATUS kept returning
// targeted_validation_required forever while `review capture-validation`
// answered with untyped prose, so the one exit that still works was never
// named.
func TestOverBudgetCorrectionStopsTypedInsteadOfReofferingTargetedValidation(t *testing.T) {
	reviewEnabledHome(t)
	repo, record := driveReviewToOpenTargetedValidationWithCorrection(t,
		"correction-budget-over", reviewCorrectionBudgetOverBudgetCorrection(t))

	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", record.State.LineageID, "--next-transition", "--projection", "staged",
		"--agent", string(model.AgentClaudeCode),
	}, &status); err != nil {
		t.Fatalf("status on the over-budget correction failed: %v\n%s", err, status.String())
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
	if parsed.NextTransition.Kind != "stop" || parsed.NextTransition.ReasonCode != reviewCorrectionContextBudgetCode {
		t.Fatalf("over-budget correction next transition = %+v, want a typed correction budget stop instead of a targeted validation offer", parsed.NextTransition)
	}
}

// TestOverBudgetCorrectionNamesAbandonAndTheLineageIsActuallyAbandonable
// proves the continuation this stop names is the one that works. The
// narration is checked against the real eligibility probe, and then the real
// `review abandon` is executed with its real maintainer authorization: a
// named exit that a live authority refuses is the same dead end with better
// prose.
func TestOverBudgetCorrectionNamesAbandonAndTheLineageIsActuallyAbandonable(t *testing.T) {
	reviewEnabledHome(t)
	repo, record := driveReviewToOpenTargetedValidationWithCorrection(t,
		"correction-budget-abandon", reviewCorrectionBudgetOverBudgetCorrection(t))
	lineage := record.State.LineageID

	statement := reviewStopReasonNarration[reviewCorrectionContextBudgetCode]
	for _, want := range []string{"gentle-ai review abandon", "cannot fit the runtime context budget"} {
		if !strings.Contains(statement, want) {
			t.Fatalf("correction budget narration does not name %q: %q", want, statement)
		}
	}

	eligibility, err := reviewtransaction.InspectCompactPristineAbandonment(context.Background(), repo, lineage)
	if err != nil || !eligibility.Eligible {
		t.Fatalf("over-budget correction abandonment eligibility = %#v, %v; the stop names an exit the authority refuses", eligibility, err)
	}
	rendered := reviewCorrectionContextBudgetAction(eligibility, repo, lineage)
	if !strings.Contains(rendered, eligibility.Revision) || !strings.Contains(rendered, lineage) {
		t.Fatalf("rendered correction budget exit does not carry the concrete values the operator would otherwise look up: %q", rendered)
	}

	var abandoned bytes.Buffer
	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", eligibility.Revision,
		"--reason", reviewtransaction.CompactAbandonReasonOperatorDisposition, "--actor", "maintainer@example.com",
		"--maintainer-authorization", reviewtransaction.RenderCompactAbandonAuthorization(
			lineage, eligibility.Revision, eligibility.SnapshotIdentity, "maintainer@example.com",
			reviewtransaction.CompactAbandonReasonOperatorDisposition, eligibility.DiscardedWork),
	}, &abandoned); err != nil {
		t.Fatalf("the exit this stop names was refused by the real operation: %v\n%s", err, abandoned.String())
	}
}

// TestOverBudgetCorrectionKeepsInvalidateRefusing documents which exit is the
// real one. `review invalidate` refuses here because lens results are already
// admitted, so compactPristineReviewing is false. That protection is
// deliberate and is NOT widened by this change; the point of the new stop is
// that it stops naming a continuation that cannot work.
func TestOverBudgetCorrectionKeepsInvalidateRefusing(t *testing.T) {
	reviewEnabledHome(t)
	repo, record := driveReviewToOpenTargetedValidationWithCorrection(t,
		"correction-budget-invalidate", reviewCorrectionBudgetOverBudgetCorrection(t))

	var invalidated bytes.Buffer
	err := RunReview([]string{
		"invalidate", "--cwd", repo, "--lineage", record.State.LineageID,
		"--expected-revision", record.Revision, "--reason", "candidate cannot fit the runtime budget",
	}, &invalidated)
	if err == nil {
		t.Fatalf("invalidate accepted a lineage with admitted lens results; that protection must stay:\n%s", invalidated.String())
	}
}

// TestCorrectionWithinBudgetStillOffersTargetedValidation is the positive
// control: the probe must classify, not refuse. A correction whose validator
// request assembles keeps its ordinary targeted validation offer.
func TestCorrectionWithinBudgetStillOffersTargetedValidation(t *testing.T) {
	reviewEnabledHome(t)
	repo, record := driveReviewToOpenTargetedValidationWithCorrection(t,
		"correction-budget-under", reviewCorrectionBudgetInBudgetCorrection)

	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", record.State.LineageID, "--next-transition", "--projection", "staged",
		"--agent", string(model.AgentClaudeCode),
	}, &status); err != nil {
		t.Fatalf("status on the in-budget correction failed: %v\n%s", err, status.String())
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
	if parsed.NextTransition.ReasonCode == reviewCorrectionContextBudgetCode {
		t.Fatalf("the correction budget probe refused a correction that fits: %+v", parsed.NextTransition)
	}
	if parsed.NextTransition.Kind != "collect" || parsed.NextTransition.ReasonCode != "targeted_validation_required" {
		t.Fatalf("in-budget correction next transition = %+v, want targeted_validation_required", parsed.NextTransition)
	}
}

// TestOverBudgetCorrectionStopCarriesTheReleaseContinuation closes the Pi dead
// end. The shipped Pi ledger row for correction_context_budget_exceeded tells
// the maintainer to run "the release command the stop's `continuation` names",
// and the Pi facade contract may not name a raw `gentle-ai review ` route at
// all, so the continuation is the only channel that route can travel. This
// asserts the stop actually carries it, that the command is the executable
// -anchored abandon route bound to this repository, and that it is runnable
// exactly as printed -- the flagless form whose refusal prints the binding
// template every remaining value is read from.
func TestOverBudgetCorrectionStopCarriesTheReleaseContinuation(t *testing.T) {
	reviewEnabledHome(t)
	// Pi is eligible for immutable receipt-review transport only while its
	// host relay contract is exported; the gentle-pi host exports it on every
	// invocation it relays.
	t.Setenv("GENTLE_PI_REVIEW_RELAY_CONTRACT", "gentle-pi.review-relay/v1")
	repo, record := driveReviewToOpenTargetedValidationWithCorrection(t,
		"correction-budget-continuation", reviewCorrectionBudgetOverBudgetCorrection(t))

	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", record.State.LineageID, "--next-transition", "--projection", "staged",
		"--agent", string(model.AgentPi),
	}, &status); err != nil {
		t.Fatalf("status on the over-budget correction failed: %v\n%s", err, status.String())
	}
	var parsed struct {
		NextTransition struct {
			Kind         string `json:"kind"`
			ReasonCode   string `json:"reason_code"`
			Continuation *struct {
				Operation string `json:"operation"`
				Command   string `json:"command"`
				Agent     string `json:"agent"`
				Detail    string `json:"detail"`
			} `json:"continuation"`
		} `json:"next_transition"`
	}
	if err := json.Unmarshal(status.Bytes(), &parsed); err != nil {
		t.Fatalf("decode status: %v\n%s", err, status.String())
	}
	if parsed.NextTransition.ReasonCode != reviewCorrectionContextBudgetCode {
		t.Fatalf("next transition = %+v, want the correction budget stop", parsed.NextTransition)
	}
	continuation := parsed.NextTransition.Continuation
	if continuation == nil {
		t.Fatalf("the correction budget stop carries no continuation, so a Pi maintainer following the shipped ledger row has no release command to run:\n%s", status.String())
	}
	if continuation.Operation != "abandon" {
		t.Fatalf("continuation operation = %q, want abandon", continuation.Operation)
	}
	if continuation.Agent != string(model.AgentPi) {
		t.Fatalf("continuation agent = %q, want the runtime STATUS was asked for", continuation.Agent)
	}
	wantCommand := managedAssetsContinuationExecutable() + " review abandon --cwd " + reviewTransitionShellWord(repo)
	if continuation.Command != wantCommand {
		t.Fatalf("continuation command = %q, want %q", continuation.Command, wantCommand)
	}
	if !strings.Contains(continuation.Detail, "binding template") {
		t.Fatalf("continuation detail does not say what running the command produces: %q", continuation.Detail)
	}

	// Runnable exactly as printed: the flagless form refuses with the binding
	// template rather than silently doing nothing.
	var template bytes.Buffer
	err := RunReview([]string{"abandon", "--cwd", repo}, &template)
	if err == nil {
		t.Fatalf("the named release command did not print the binding template:\n%s", template.String())
	}
	if !strings.Contains(err.Error(), "--maintainer-authorization") {
		t.Fatalf("the named release command is not the binding-template step the continuation claims: %v", err)
	}
}

// TestCorrectionBudgetStopOmitsTheContinuationWhenReleaseIsRefused preserves
// the honesty property the narration already has: where the authority is not
// eligible for abandonment, no command is named at all, because an unrunnable
// command is worse than an honest "ask a maintainer".
func TestCorrectionBudgetStopOmitsTheContinuationWhenReleaseIsRefused(t *testing.T) {
	ineligible := reviewtransaction.CompactAbandonEligibility{}
	if continuation := reviewCorrectionReleaseContinuation("/repo", "pi", &ineligible); continuation != nil {
		t.Fatalf("an ineligible authority was handed a release command that would be refused: %+v", continuation)
	}
	if continuation := reviewCorrectionReleaseContinuation("/repo", "pi", nil); continuation != nil {
		t.Fatalf("an unprobed authority was handed a release command: %+v", continuation)
	}
}
