package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// lensContextBlock runs `review lens-context` for one lens and returns the
// finished reviewer block exactly as a runtime would inject it.
func lensContextBlock(t *testing.T, handle, lens string) string {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"lens-context", "--repository-context", handle, "--lens", lens}, &output); err != nil {
		t.Fatalf("lens-context %s: %v", lens, err)
	}
	return output.String()
}

// TestReviewLensContextEmitsFinishedReviewerBlockFromTwoTokens is the whole
// point of the surface: the caller supplies only the two opaque tokens the
// collect transition already carries, and receives the complete reviewer
// prompt prefix with nothing left to assemble.
func TestReviewLensContextEmitsFinishedReviewerBlockFromTwoTokens(t *testing.T) {
	reviewEnabledHome(t)
	_, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	block := lensContextBlock(t, handle, lens)

	lines := strings.SplitN(block, "\n", 3)
	if len(lines) < 3 {
		t.Fatalf("lens context is not a multi-line block:\n%s", block)
	}
	bindingJSON, found := strings.CutPrefix(lines[0], "GENTLE_AI_REVIEW_BINDING ")
	if !found {
		t.Fatalf("first line is not the binding: %q", lines[0])
	}
	var binding map[string]any
	decoder := json.NewDecoder(strings.NewReader(bindingJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&binding); err != nil {
		t.Fatalf("binding is not one-line JSON: %v", err)
	}
	wantKeys := []string{"lens", "lineage", "order", "repository_context", "revision", "subject_hash", "target"}
	gotKeys := make([]string, 0, len(binding))
	for key := range binding {
		gotKeys = append(gotKeys, key)
	}
	slices.Sort(gotKeys)
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("binding fields = %v, want %v", gotKeys, wantKeys)
	}
	// The defect this surface retires: a relayed prose instruction let an
	// orchestrator author "0" where the contract requires the number 0.
	if _, isNumber := binding["order"].(json.Number); !isNumber {
		t.Fatalf("binding order is %T, want a JSON number", binding["order"])
	}
	if binding["lens"] != lens || binding["repository_context"] != handle {
		t.Fatalf("binding does not echo the supplied tokens: %v", binding)
	}

	contextJSON, found := strings.CutPrefix(lines[1], "GENTLE_AI_REVIEW_CONTEXT ")
	if !found {
		t.Fatalf("second line is not the capture context: %q", lines[1])
	}
	var preflight map[string]any
	if err := json.Unmarshal([]byte(contextJSON), &preflight); err != nil {
		t.Fatalf("capture context is not one-line JSON: %v", err)
	}
	if preflight["schema"] != reviewCapturePreflightSchema {
		t.Fatalf("capture context schema = %v", preflight["schema"])
	}
	if _, leaked := preflight["repository_root"]; leaked {
		t.Fatal("capture context leaked a repository path into an opaque-bound block")
	}
	subject, _ := preflight["artifact_subject"].(map[string]any)
	if subject == nil || subject["subject_hash"] != binding["subject_hash"] {
		t.Fatalf("binding subject hash does not match the emitted subject: %v", preflight)
	}

	// Every materialized section is exactly what the provider's own bounded
	// inspection returns for the frozen candidate.
	inspection := []string{
		"--repository-context", handle, "--expected-revision", subject["authority_revision"].(string),
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0",
	}
	wantSections := map[string][]string{
		"GENTLE_AI_REVIEW_NAME_STATUS": {"--operation", "name-status"},
		"GENTLE_AI_REVIEW_NUMSTAT":     {"--operation", "numstat"},
	}
	for header, operation := range wantSections {
		var expected bytes.Buffer
		if err := RunReview(slices.Concat([]string{"inspect-candidate"}, inspection, operation), &expected); err != nil {
			t.Fatal(err)
		}
		want := header + "\n" + strings.TrimSpace(expected.String()) + "\n" + header + "_END\n"
		if !strings.Contains(block, want) {
			t.Fatalf("block omits %s section\nwant:\n%s\ngot:\n%s", header, want, block)
		}
	}
	manifest, _ := preflight["changed_path_manifest"].([]any)
	if len(manifest) == 0 {
		t.Fatal("capture context carries no changed path manifest")
	}
	for index, raw := range manifest {
		entry := raw.(map[string]any)
		var expected bytes.Buffer
		if err := RunReview(slices.Concat([]string{"inspect-candidate"}, inspection,
			[]string{"--operation", "patch", "--path-index", fmt.Sprint(index)}), &expected); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("GENTLE_AI_REVIEW_PATCH %d %s\n%s\nGENTLE_AI_REVIEW_PATCH_END\n",
			index, entry["path"], strings.TrimSpace(expected.String()))
		if !strings.Contains(block, want) {
			t.Fatalf("block omits patch %d\nwant:\n%s\ngot:\n%s", index, want, block)
		}
	}
	if !strings.HasSuffix(block, "GENTLE_AI_REVIEW_CONTEXT_END\n") {
		t.Fatalf("block is not terminated:\n%s", block)
	}
}

// TestReviewLensContextRefusesUnboundInput proves every refusal fails closed
// with no bytes on stdout: a partial reviewer block is the fabricate-a-clean-
// review shape this surface exists to prevent.
func TestReviewLensContextRefusesUnboundInput(t *testing.T) {
	reviewEnabledHome(t)
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "missing lens", argv: []string{"lens-context", "--repository-context", handle}, want: "requires the exact provider-issued"},
		{name: "missing context", argv: []string{"lens-context", "--lens", lens}, want: "requires the exact provider-issued"},
		{name: "malformed context", argv: []string{"lens-context", "--repository-context", "not-a-handle", "--lens", lens}, want: "repository_context_"},
		{name: "unknown context", argv: []string{"lens-context", "--repository-context", "rctx1_" + strings.Repeat("0", 64), "--lens", lens}, want: "repository_context_"},
		{name: "unselected lens", argv: []string{"lens-context", "--repository-context", handle, "--lens", "review-nonexistent"}, want: "lens_context_lens_not_selected"},
		{name: "positional", argv: []string{"lens-context", "--repository-context", handle, "--lens", lens, "HEAD"}, want: "requires the exact provider-issued"},
		{name: "unknown flag", argv: []string{"lens-context", "--repository-context", handle, "--lens", lens, "--order", "0"}, want: "flag provided but not defined"},
		{name: "unknown delivery", argv: []string{"lens-context", "--repository-context", handle, "--lens", lens, "--delivery", "hand-wave"}, want: "unknown reviewer context delivery"},
		{name: "caller provider contract", argv: []string{"lens-context", "--repository-context", handle, "--lens", lens, "--delivery", string(reviewtransaction.ReviewerContextLevelProviderContract)}, want: "reserved for Go-owned provider execution"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(test.argv, &output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(output.String(), "GENTLE_AI_REVIEW_") {
				t.Fatalf("refusal leaked partial reviewer context to stdout:\n%s", output.String())
			}
		})
	}
}

// writeOverByteBudgetLensContextCandidate writes two paths that each read well
// inside the per-command cap but together exceed the whole-context byte budget.
// That aggregate is what this surface owns, and no single native read catches
// it.
func writeOverByteBudgetLensContextCandidate(t *testing.T, repo string) {
	t.Helper()
	body := strings.Repeat("oversized evidence line\n", 100_000)
	for _, name := range []string{"bulk-one.txt", "bulk-two.txt"} {
		writeReviewStartCandidate(t, repo, name, body, 0o644)
	}
}

// TestNegotiatedStartRefusesOverBudgetCandidateWithoutPersistingAuthority
// proves the budget is enforced by outright refusal at the point of decision.
// Truncated candidate evidence could fabricate a false-clean lens result, so
// there is no partial outcome; and because START refuses before creating
// authority, there is no dead lineage to abandon afterwards either (#3367).
func TestNegotiatedStartRefusesOverBudgetCandidateWithoutPersistingAuthority(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeOverByteBudgetLensContextCandidate(t, repo)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "lens-context-budget",
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget START error = %v\n%s", err, output.String())
	}
	if !strings.Contains(err.Error(), "no review authority was created") {
		t.Fatalf("over-budget START does not say that nothing was persisted: %v", err)
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("over-budget START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" ||
		failure.Failure.NextAction != "stop" || failure.Failure.Code != "lens_context_budget_exceeded" {
		t.Fatalf("over-budget START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if !strings.Contains(failure.Failure.Cause, "chained sequence of smaller reviewable commits") ||
		!strings.Contains(failure.Failure.Cause, "gentle-ai review status") {
		t.Fatalf("over-budget START cause does not name the runnable continuation: %q", failure.Failure.Cause)
	}
	if strings.Contains(output.String(), "GENTLE_AI_REVIEW_") {
		t.Fatalf("over-budget START refusal emitted reviewer evidence:\n%s", output.String())
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("over-budget START changed authority storage before create:\nbefore:\n%s\nafter:\n%s", authorityBefore, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("over-budget START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestReviewLensContextRefusesEmptyPatchForContentChangingPath proves the one
// guard that cannot be provoked from a real repository: if a native read
// silently returns nothing for a path that changed content, the reviewer must
// not launch. A reviewer handed a path header with no diff would report it
// clean.
func TestReviewLensContextRefusesEmptyPatchForContentChangingPath(t *testing.T) {
	reviewEnabledHome(t)
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	deps := reviewLensContextDependencies()
	deps.inspect = func(ctx context.Context, inspector reviewLensCandidateInspector, operation string, pathIndex int, side string) ([]byte, error) {
		if operation == "patch" {
			return nil, nil
		}
		return inspector.Inspect(ctx, operation, pathIndex, side)
	}
	block, err := runReviewLensContext([]string{"--repository-context", handle, "--lens", lens}, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), "lens_context_empty_patch") {
		t.Fatalf("empty patch error = %v", err)
	}
	if len(block) != 0 {
		t.Fatalf("empty-patch refusal emitted %d bytes of reviewer context", len(block))
	}
}

// TestReviewLensContextCarriesAggregateDeadline proves the assembly-wide
// deadline reaches every phase, so a repository that stops answering partway
// through never yields a partial block.
func TestReviewLensContextCarriesAggregateDeadline(t *testing.T) {
	reviewEnabledHome(t)
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	deps := reviewLensContextDependencies()
	deps.timeout = 0
	block, err := runReviewLensContext([]string{"--repository-context", handle, "--lens", lens}, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), "lens_context_deadline_exceeded") {
		t.Fatalf("deadline error = %v", err)
	}
	if len(block) != 0 {
		t.Fatalf("deadline refusal emitted %d bytes of reviewer context", len(block))
	}
	want := "lens_context_deadline_exceeded: provider-owned reviewer lens context was not produced; " + reviewLensContextDeadlineAction
	if err.Error() != want {
		t.Fatalf("deadline guidance = %q, want %q", err, want)
	}
	if !strings.Contains(err.Error(), "execute the returned transition once") || !strings.Contains(err.Error(), "if the same lens slot reaches the same deadline again") ||
		!strings.Contains(err.Error(), "split the candidate into a chained sequence of smaller reviewable commits") || !strings.Contains(err.Error(), reviewNextTransitionRefreshCommandV21) {
		t.Fatalf("deadline guidance is not a single transition execution followed by a runnable reduced-scope exit: %q", err)
	}
}

// TestNegotiatedStatusStopsDeterministicLensContextBudgetWithoutMutation keeps
// STATUS's deterministic budget classification honest for a lineage that
// already exists over budget. Its candidate changed with issue #3367: the
// 33-path fixture it used to carry is now reviewable, because path count no
// longer decides anything, so the over-budget candidate has to be one whose
// evidence genuinely exceeds the byte budget.
//
// Its authority is created through StartCompactAuthority rather than the
// facade, which is exactly the shape a build without START's representability
// check leaves behind. That upgrade path is the only way this stop is still
// reachable, and it is why the classification is kept rather than deleted
// along with the count. Every no-mutation assertion below is unchanged.
func TestNegotiatedStatusStopsDeterministicLensContextBudgetWithoutMutation(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeOverByteBudgetLensContextCandidate(t, repo)
	record := startCompactAuthorityWithoutFacadeChecks(t, repo, "lens-context-status-budget")
	handle := deriveLensContextHandle(t, repo, record)
	before := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo))
	homeBefore := readLegacyAuthorityTree(t, home)
	worktreeBefore := runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all")

	var context bytes.Buffer
	err := RunReview([]string{
		"lens-context", "--repository-context", handle, "--lens", record.State.SelectedLenses[0],
	}, &context)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("lens-context refusal = %v, want deterministic budget exhaustion", err)
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
		t.Fatalf("STATUS reoffered a deterministically impossible reviewer slot: %#v", status)
	}
	if status.Authority == nil || status.Authority.Revision != record.Revision ||
		status.Authority.State != reviewtransaction.StateReviewing ||
		status.Frozen == nil || status.Frozen.CorrectionBudget != record.State.CorrectionBudget {
		t.Fatalf("terminal STATUS changed frozen review truth: %#v", status)
	}
	if after := readLegacyAuthorityTree(t, reviewCLIAuthorityRoot(t, repo)); !reflect.DeepEqual(before, after) {
		t.Fatalf("STATUS budget classification mutated authority: before=%#v after=%#v", before, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("STATUS budget classification persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
	if worktreeAfter := runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"); worktreeAfter != worktreeBefore {
		t.Fatalf("STATUS budget classification changed the worktree: before=%q after=%q", worktreeBefore, worktreeAfter)
	}
}

func TestNegotiatedStatusReoffersEmptyReviewerResultWithoutBudgetTerminal(t *testing.T) {
	reviewModeHome(t)
	repo, started, _, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "empty-reviewer-result.json")
	if err := os.WriteFile(input, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, io.Discard)
	if err == nil {
		t.Fatal("empty reviewer result was accepted")
	}

	status := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "reviewer_results_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		t.Fatalf("empty reviewer result STATUS = %#v, want fresh collect continuity", status)
	}
}

func TestReviewLensContextRecoveryGuidanceRefreshesThenExecutesNextTransition(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "evidence capacity", action: reviewLensContextBudgetAction},
		{name: "start capacity", action: reviewLensContextStartBudgetAction},
		{name: "deadline", action: reviewLensContextDeadlineAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(test.action, reviewNextTransitionRefreshCommandV21) ||
				!strings.Contains(test.action, "refresh the exact native next transition") ||
				!strings.Contains(test.action, "execute the returned transition") {
				t.Fatalf("recovery guidance = %q, want STATUS to refresh the exact transition and the caller to execute it", test.action)
			}
			if strings.Contains(test.action, "start a review") {
				t.Fatalf("recovery guidance = %q, must not claim STATUS itself starts a review", test.action)
			}
		})
	}
}

func TestReviewLensContextCleanupClassifiesCleanupFailureIndependentlyOfOperationContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithTimeout(context.Background(), 0)
	defer cancelExpired()

	for _, test := range []struct {
		name     string
		ctx      context.Context
		closeErr error
	}{
		{name: "canceled", ctx: canceled, closeErr: errors.New("close canceled inspector")},
		{name: "deadline exceeded", ctx: expired, closeErr: errors.New("close expired inspector")},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := reviewLensContextCleanup(test.ctx, "reviewer context", nil, func() error {
				return test.closeErr
			})
			if result != "" {
				t.Fatalf("cleanup-only result = %q, want zero result", result)
			}
			var refusal *reviewLensContextError
			if !errors.As(err, &refusal) || refusal.Code != "lens_context_inspection_failed" {
				t.Fatalf("cleanup-only error = %v, want ordinary inspection-failed refusal", err)
			}
			if !errors.Is(err, test.closeErr) {
				t.Fatalf("cleanup-only error = %v, want close error %v", err, test.closeErr)
			}
		})
	}
}

func TestReviewLensContextCallersFailClosedOnInspectorCleanupFailure(t *testing.T) {
	reviewEnabledHome(t)
	cleanupErr := errors.New("close inspector")
	for _, caller := range []struct {
		name string
		call func(reviewLensContextDeps, string, string) (error, bool)
	}{
		{"lens context", func(deps reviewLensContextDeps, handle, lens string) (error, bool) {
			payload, err := runReviewLensContext([]string{"--repository-context", handle, "--lens", lens}, io.Discard, deps)
			return err, payload == nil
		}},
		{"provider materialization", func(deps reviewLensContextDeps, handle, lens string) (error, bool) {
			request, err := reviewProviderMaterialize(context.Background(), deps, handle, lens)
			return err, request.Store.Dir == "" && request.Binding == (reviewLensContextBinding{}) && request.Subject == (reviewtransaction.ArtifactSubject{}) && len(request.Invocation.Prompt()) == 0
		}},
	} {
		for _, operation := range []bool{false, true} {
			t.Run(caller.name, func(t *testing.T) {
				_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
				handle, lens := args[slices.Index(args, "--repository-context")+1], args[slices.Index(args, "--lens")+1]
				deps := reviewLensContextDependencies()
				deps.close = func(inspector reviewLensCandidateInspector) error {
					if err := inspector.Close(); err != nil {
						return err
					}
					return cleanupErr
				}
				if operation {
					inspect := deps.inspect
					deps.inspect = func(ctx context.Context, inspector reviewLensCandidateInspector, kind string, index int, side string) ([]byte, error) {
						if kind == "patch" {
							return nil, nil
						}
						return inspect(ctx, inspector, kind, index, side)
					}
				}
				err, zero := caller.call(deps, handle, lens)
				if !zero {
					t.Fatal("cleanup did not zero the result")
				}
				if !operation {
					if !errors.Is(err, cleanupErr) {
						t.Fatalf("cleanup-only error = %v, want cleanup cause %v", err, cleanupErr)
					}
					return
				}
				var refusal *reviewLensContextError
				if !errors.As(err, &refusal) || refusal.Code != "lens_context_empty_patch" || !errors.Is(err, cleanupErr) {
					t.Fatalf("operation and cleanup error = %v, want empty-patch refusal and cleanup cause", err)
				}
			})
		}
	}
}

// TestReviewLensContextLeavesRepositoryUntouched proves the surface is a pure
// read of the frozen trees.
func TestReviewLensContextLeavesRepositoryUntouched(t *testing.T) {
	reviewEnabledHome(t)
	repo, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	before := [2]string{
		runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"),
		runReviewCLIGit(t, repo, "rev-parse", "HEAD"),
	}
	if _, err := io.WriteString(io.Discard, lensContextBlock(t, handle, lens)); err != nil {
		t.Fatal(err)
	}
	after := [2]string{
		runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"),
		runReviewCLIGit(t, repo, "rev-parse", "HEAD"),
	}
	if before != after {
		t.Fatalf("lens-context mutated the repository: before=%q after=%q", before, after)
	}
}

// TestReviewLensContextRecordsProviderEmissionForTheReceipt proves the level a
// receipt will carry is observed from what the provider actually produced, not
// declared by whoever closes the review. Absence stays absence when nothing produced a
// context.
func TestReviewLensContextRecordsProviderEmissionForTheReceipt(t *testing.T) {
	reviewEnabledHome(t)
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	state := record.State
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	subjects := make([]string, len(state.SelectedLenses))

	if level := reviewtransaction.DiscoverReviewerContextLevel(store.Dir, state.LineageID,
		state.InitialSnapshot.Identity, record.Revision, state.SelectedLenses, subjects); level != "" {
		t.Fatalf("level recorded before any context was produced: %q", level)
	}

	for order, lens := range state.SelectedLenses {
		block := lensContextBlock(t, handle, lens)
		binding, _, _ := strings.Cut(strings.TrimPrefix(strings.SplitN(block, "\n", 2)[0], "GENTLE_AI_REVIEW_BINDING "), "\n")
		var decoded map[string]any
		if err := json.Unmarshal([]byte(binding), &decoded); err != nil {
			t.Fatal(err)
		}
		subjects[order] = decoded["subject_hash"].(string)
		// Re-emitting the identical context converges instead of conflicting.
		lensContextBlock(t, handle, lens)
	}

	level := reviewtransaction.DiscoverReviewerContextLevel(store.Dir, state.LineageID,
		state.InitialSnapshot.Identity, record.Revision, state.SelectedLenses, subjects)
	if level != reviewtransaction.ReviewerContextLevelProviderCommand {
		t.Fatalf("recorded level = %q, want %q", level, reviewtransaction.ReviewerContextLevelProviderCommand)
	}

	// A record is bound to its exact candidate: read it against another
	// subject and it is absent, never reused.
	wrong := append([]string(nil), subjects...)
	wrong[0] = "sha256:" + strings.Repeat("0", 64)
	if level := reviewtransaction.DiscoverReviewerContextLevel(store.Dir, state.LineageID,
		state.InitialSnapshot.Identity, record.Revision, state.SelectedLenses, wrong); level != "" {
		t.Fatalf("emission was reused for a different candidate: %q", level)
	}
}

// TestReviewLensContextRefusesConflictingDeliveryForOneSlot proves the audit
// record cannot be rewritten: one frozen lens slot records one mechanism.
func TestReviewLensContextRefusesConflictingDeliveryForOneSlot(t *testing.T) {
	reviewEnabledHome(t)
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]
	lensContextBlock(t, handle, lens)
	var output bytes.Buffer
	err := RunReview([]string{
		"lens-context", "--repository-context", handle, "--lens", lens,
		"--delivery", string(reviewtransaction.ReviewerContextLevelRuntimeInterception),
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_emission_conflict") {
		t.Fatalf("conflicting delivery error = %v", err)
	}
	// The exit has to name the mechanism the slot actually recorded: "use the
	// same mechanism" is not runnable if the operator cannot tell which one
	// that was (issue #2850).
	for _, want := range []string{
		string(reviewtransaction.ReviewerContextLevelProviderCommand),
		"--delivery " + string(reviewtransaction.ReviewerContextLevelProviderCommand),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("conflict refusal does not name a runnable exit (%q): %v", want, err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("conflicting delivery emitted %d bytes", output.Len())
	}
}

// TestReviewLensContextStandsAloneAsTheReviewerInstruction is the level-2
// floor's load-bearing property: an orchestrator with no runtime adapter pastes
// this block in front of a generic subagent and the reviewer knows who it is,
// what it may look at, and exactly what to return. There is no runtime layer
// behind this output to fill a gap in it.
func TestReviewLensContextStandsAloneAsTheReviewerInstruction(t *testing.T) {
	reviewEnabledHome(t)
	_, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	for _, lens := range record.State.SelectedLenses {
		t.Run(lens, func(t *testing.T) {
			block := lensContextBlock(t, handle, lens)
			instruction, found := lensContextSection(block, "GENTLE_AI_REVIEW_INSTRUCTION")
			if !found {
				t.Fatalf("block carries no reviewer instruction:\n%s", block)
			}
			title, focus, ok := reviewtransaction.LensMandate(lens)
			if !ok {
				t.Fatalf("lens %q has no canonical mandate", lens)
			}
			for _, required := range []string{title, focus} {
				if !strings.Contains(instruction, required) {
					t.Fatalf("instruction omits the lens mandate %q:\n%s", required, instruction)
				}
			}
			// The reviewer must know the evidence in the block is the whole
			// candidate, and that reading anything else is not permitted.
			// The citation discipline must warn that free-text evidence and
			// proof_refs tokens shaped like path:line are validated against
			// the frozen repository, since one unknown path rejects the
			// entire result, and that a candidate-causal finding must anchor
			// its span entirely within candidate-changed lines, since one
			// unchanged context line in the span rejects the result too.
			for _, required := range []string{
				"complete and only", "working tree", "subject_hash", "complete unique unordered set", "path:line or path:start-end",
				"exactly as it appears in the changed-path manifest", "host:port", "validated against the frozen repository",
				"entirely within lines this candidate changed", "one unchanged context line in the span",
				"never reproduce that token", "describe it in words and cite the manifest file and line",
			} {
				if !strings.Contains(instruction, required) {
					t.Fatalf("instruction omits %q:\n%s", required, instruction)
				}
			}
			schema, found := lensContextSection(block, "GENTLE_AI_REVIEW_RESULT_SCHEMA")
			if !found {
				t.Fatalf("block carries no reviewer result schema:\n%s", block)
			}
			if strings.TrimSpace(schema) != strings.TrimSpace(reviewtransaction.ReviewerResultSchema) {
				t.Fatalf("result schema is not the product's published one:\n%s", schema)
			}
			// The instruction precedes the evidence, and the block still ends
			// where every existing consumer expects it to.
			if strings.Index(block, "GENTLE_AI_REVIEW_INSTRUCTION") > strings.Index(block, "GENTLE_AI_REVIEW_NAME_STATUS") {
				t.Fatal("instruction appears after the evidence")
			}
			if !strings.HasSuffix(block, "GENTLE_AI_REVIEW_CONTEXT_END\n") {
				t.Fatal("block terminator moved")
			}
		})
	}
}

func lensContextSection(block, header string) (string, bool) {
	_, after, found := strings.Cut(block, "\n"+header+"\n")
	if !found {
		return "", false
	}
	body, _, found := strings.Cut(after, "\n"+header+"_END\n")
	return body, found
}

// writeLensContextPathFixture writes `paths` small, distinct source files so a
// candidate's path count and its evidence byte size can be varied
// independently. Every file is a few dozen bytes, which is what makes the
// difference between counting entries and measuring bytes observable.
func writeLensContextPathFixture(t *testing.T, repo string, paths int) {
	t.Helper()
	for index := range paths {
		writeReviewStartCandidate(t, repo,
			fmt.Sprintf("internal/candidate/path%02d/source.go", index),
			"package path"+strconv.Itoa(index)+"\n\nfunc Exported"+strconv.Itoa(index)+"() bool { return true }\n", 0o644)
	}
}

// TestNegotiatedStartLeavesNoAuthorityStatusCanOnlyRefuse pins the documented
// START invariant from review_facade.go: "a candidate that starts here is a
// candidate STATUS can answer". A candidate whose only excess is its path count
// costs a few kilobytes of evidence, so START must not persist durable
// authority whose STATUS is a permanent stop (issue #3367 property 1).
func TestNegotiatedStartLeavesNoAuthorityStatusCanOnlyRefuse(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeLensContextPathFixture(t, repo, 60)
	started := runNegotiatedReviewStart(t, repo, "large-candidate-reviewable")
	if len(started.SelectedLenses) == 0 {
		t.Fatal("60-path candidate selected no lenses; the fixture no longer exercises reviewer evidence")
	}
	status := explicitFrozenReviewingStatus(t, repo, started.LineageID)
	if status.NextTransition == nil {
		t.Fatal("STATUS published no next transition for a durable authority START created")
	}
	if status.NextTransition.Kind == reviewNextTransitionStop {
		t.Fatalf("START persisted authority STATUS can only refuse: stop(%s) for a %d-path candidate",
			status.NextTransition.ReasonCode, len(started.SelectedLenses))
	}
}

// TestReviewLensContextAdmitsManyPathsFarUnderByteBudget pins that the limit
// tracks the thing that actually costs reviewer context. Thirty-three one-line
// files are a rounding error against a 4 MiB budget, so refusing them while
// admitting a much larger 32-path candidate is a count masquerading as a
// budget (issue #3367 property 3).
func TestReviewLensContextAdmitsManyPathsFarUnderByteBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeLensContextPathFixture(t, repo, 33)
	started := runNegotiatedReviewStart(t, repo, "lens-context-small-paths")
	var output bytes.Buffer
	if err := RunReview([]string{
		"lens-context", "--repository-context", started.RepositoryContext.Handle, "--lens", started.SelectedLenses[0],
	}, &output); err != nil {
		t.Fatalf("33 one-line paths were refused: %v", err)
	}
	if output.Len() > reviewLensContextByteBudget/8 {
		t.Fatalf("fixture is not far under budget: block = %d bytes of %d", output.Len(), reviewLensContextByteBudget)
	}
	t.Logf("33-path reviewer block = %d bytes of the %d byte budget", output.Len(), reviewLensContextByteBudget)
}

// startCompactAuthorityWithoutFacadeChecks persists reviewing authority the way
// a build predating START's representability check did: the same native state
// and the same store write, with none of the facade's pre-create refusals. It
// is the only remaining way to reach a lineage whose reviewer evidence cannot
// be assembled, which is exactly the upgrade path STATUS still has to classify.
func startCompactAuthorityWithoutFacadeChecks(t *testing.T, repo, lineage string) reviewtransaction.CompactRecord {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(t.Context(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := builder.AssessSnapshotRisk(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatal(err)
	}
	if len(lenses) == 0 {
		t.Fatal("fixture selected no lenses; it no longer exercises reviewer evidence")
	}
	request, err := prepareReviewFacadeCompactAtomicStart(t.Context(), repo, lineage, "", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	}, snapshot, assessment, assessment.ChangedLines, lenses)
	if err != nil {
		t.Fatal(err)
	}
	started, err := runReviewFacadeCompactAtomicStart(t.Context(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	return started.Record
}

// deriveLensContextHandle recovers the opaque repository context a collect
// transition would have carried for an already-persisted record.
func deriveLensContextHandle(t *testing.T, repo string, record reviewtransaction.CompactRecord) string {
	t.Helper()
	handle, err := reviewtransaction.PublishReviewRepositoryContext(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handle
}

// TestReviewLensContextBudgetProbeReportsFailureInsteadOfUnderBudget pins the
// third outcome. A probe that never reached the budget used to return the same
// false a comfortably-small candidate returns, so no caller could tell "fits"
// apart from "never measured" (issue #3367 property 2).
func TestReviewLensContextBudgetProbeReportsFailureInsteadOfUnderBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeLensContextPathFixture(t, repo, 4)
	started := runNegotiatedReviewStart(t, repo, "budget-probe-failure")
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if reviewLensContextStatusBudgetExhausted(t.Context(), repo, record.State, record.Revision) {
		t.Fatal("reachable small candidate was classified as over budget")
	}

	for _, test := range []struct {
		name string
		deps func(reviewLensContextDeps) reviewLensContextDeps
	}{
		{
			name: "candidate preparation fails",
			deps: func(deps reviewLensContextDeps) reviewLensContextDeps {
				deps.prepare = func(reviewtransaction.SnapshotBuilder, context.Context, reviewtransaction.Snapshot) (reviewLensCandidateInspector, error) {
					return nil, errors.New("frozen trees are unreachable")
				}
				return deps
			},
		},
		{
			name: "repository context is unreachable",
			deps: func(deps reviewLensContextDeps) reviewLensContextDeps {
				deps.timeout = time.Nanosecond
				return deps
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome, err := reviewLensContextBudgetProbe(t.Context(), test.deps(reviewLensContextDependencies()), repo, record.State, record.Revision)
			if err == nil {
				t.Fatalf("probe answered outcome=%v with no cause after it never evaluated the budget", outcome)
			}
			if outcome != reviewLensContextUnproven {
				t.Fatalf("probe that failed to evaluate the budget answered %v, inventing a verdict about the candidate: %v", outcome, err)
			}
		})
	}
}
