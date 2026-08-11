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
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
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

// TestReviewLensContextRefusesOverBudgetCandidateWithoutTruncating proves the
// budget is enforced by outright refusal. Truncated candidate evidence could
// fabricate a false-clean lens result, so there is no partial outcome.
func TestReviewLensContextRefusesOverBudgetCandidateWithoutTruncating(t *testing.T) {
	// Two paths that each read well inside the per-command cap but together
	// exceed the whole-context budget: this is the aggregate the surface owns
	// and no single native read would ever catch.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	body := strings.Repeat("oversized evidence line\n", 100_000)
	for _, name := range []string{"bulk-one.txt", "bulk-two.txt"} {
		writeReviewStartCandidate(t, repo, name, body, 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "lens-context-budget")
	var output bytes.Buffer
	err := RunReview([]string{
		"lens-context", "--repository-context", started.RepositoryContext.Handle, "--lens", started.SelectedLenses[0],
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("over-budget refusal emitted %d bytes of truncated evidence", output.Len())
	}
}

// TestReviewLensContextRefusesEmptyPatchForContentChangingPath proves the one
// guard that cannot be provoked from a real repository: if a native read
// silently returns nothing for a path that changed content, the reviewer must
// not launch. A reviewer handed a path header with no diff would report it
// clean.
func TestReviewLensContextRefusesEmptyPatchForContentChangingPath(t *testing.T) {
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

func TestReviewLensContextRefusesManifestOverAdvisoryCapacityBeforePatchInspection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	for index := range 33 {
		writeReviewStartCandidate(t, repo, fmt.Sprintf("path-%02d.txt", index), "candidate\n", 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "lens-context-entry-capacity")
	deps := reviewLensContextDependencies()
	inspect := deps.inspect
	patchReads := 0
	deps.inspect = func(ctx context.Context, inspector reviewLensCandidateInspector, operation string, pathIndex int, side string) ([]byte, error) {
		if operation == "patch" {
			patchReads++
		}
		return inspect(ctx, inspector, operation, pathIndex, side)
	}
	block, err := runReviewLensContext([]string{
		"--repository-context", started.RepositoryContext.Handle, "--lens", started.SelectedLenses[0],
	}, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("entry-capacity error = %v", err)
	}
	if strings.Contains(err.Error(), "advisory review") || !strings.Contains(err.Error(), "provider-owned reviewer context accepts at most") ||
		!strings.Contains(err.Error(), "33") || !strings.Contains(err.Error(), "32") || !strings.Contains(err.Error(), "retrying this candidate cannot succeed") ||
		!strings.Contains(err.Error(), "chained sequence of smaller reviewable commits") || !strings.Contains(err.Error(), reviewNextTransitionRefreshCommandV21) {
		t.Fatalf("entry-capacity guidance does not name its limit and runnable continuation: %q", err)
	}
	if patchReads != 0 {
		t.Fatalf("patch inspections before entry-capacity refusal = %d, want 0", patchReads)
	}
	if len(block) != 0 {
		t.Fatalf("entry-capacity refusal emitted %d bytes of reviewer context", len(block))
	}
}

func TestReviewLensContextRecoveryGuidanceRefreshesThenExecutesNextTransition(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "evidence capacity", action: reviewLensContextBudgetAction},
		{name: "entry capacity", action: reviewLensContextCapacityAction(advisoryreview.MaxEvidenceEntries + 1)},
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
	cleanupErr := errors.New("close inspector")
	for _, caller := range []struct {
		name string
		call func(reviewLensContextDeps, string, string) (error, bool)
	}{
		{"lens context", func(deps reviewLensContextDeps, handle, lens string) (error, bool) {
			payload, err := runReviewLensContext([]string{"--repository-context", handle, "--lens", lens}, io.Discard, deps)
			return err, payload == nil
		}},
		{"advisory request", func(deps reviewLensContextDeps, handle, lens string) (error, bool) {
			request, err := resolveAdvisoryRequest(context.Background(), deps, handle, lens)
			return err, request.ArtifactSubject == (reviewtransaction.ArtifactSubject{}) && request.ChangedPathManifest == nil && request.Evidence == nil
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
// declared by whoever finalizes. Absence stays absence when nothing produced a
// context.
func TestReviewLensContextRecordsProviderEmissionForTheReceipt(t *testing.T) {
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
	if output.Len() != 0 {
		t.Fatalf("conflicting delivery emitted %d bytes", output.Len())
	}
}

// TestReviewReceiptRecordsLensContextLevel is the end of the chain: a review
// whose every lens context came from the provider command carries that fact on
// its terminal receipt, and a review that never used the surface carries no
// level at all rather than a guessed one.
func TestReviewReceiptRecordsLensContextLevel(t *testing.T) {
	for _, test := range []struct {
		name           string
		produceContext bool
		want           reviewtransaction.ReviewerContextLevel
	}{
		{name: "provider produced every lens context", produceContext: true, want: reviewtransaction.ReviewerContextLevelProviderCommand},
		{name: "nothing produced a lens context", produceContext: false, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
			handle := args[slices.Index(args, "--repository-context")+1]
			state := record.State
			if test.produceContext {
				for _, lens := range state.SelectedLenses {
					lensContextBlock(t, handle, lens)
				}
			}
			finalizeArgs := []string{"--cwd", repo, "--lineage", state.LineageID}
			for range state.SelectedLenses {
				resultPath := filepath.Join(t.TempDir(), "review.json")
				writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
					Findings: []facadeFinding{}, Evidence: []string{"reviewed the complete candidate scope"},
				})
				finalizeArgs = append(finalizeArgs, "--result", resultPath)
			}
			evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
			if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			finalizeArgs = append(finalizeArgs, "--evidence", evidencePath)
			if err := finalizeReviewCLIArgs(t, repo, finalizeArgs, io.Discard); err != nil {
				t.Fatal(err)
			}
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := os.ReadFile(store.ReceiptPath())
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := reviewtransaction.ParseCompactReceipt(payload)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.ReviewerContextLevel != test.want {
				t.Fatalf("receipt reviewer context level = %q, want %q", receipt.ReviewerContextLevel, test.want)
			}
		})
	}
}

// TestReviewLensContextStandsAloneAsTheReviewerInstruction is the level-2
// floor's load-bearing property: an orchestrator with no runtime adapter pastes
// this block in front of a generic subagent and the reviewer knows who it is,
// what it may look at, and exactly what to return. There is no runtime layer
// behind this output to fill a gap in it.
func TestReviewLensContextStandsAloneAsTheReviewerInstruction(t *testing.T) {
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
			for _, required := range []string{"complete and only", "working tree", "subject_hash", "complete unique unordered set", "path:line or path:start-end"} {
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
