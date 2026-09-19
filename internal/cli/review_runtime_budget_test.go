package cli

import (
	"bytes"
	"context"
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
