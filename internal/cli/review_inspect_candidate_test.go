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
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type inspectionCase struct {
	name string
	argv []string
	want string
}

func TestReviewInspectCandidateReadsOnlyBoundFrozenTrees(t *testing.T) {
	reviewEnabledHome(t)
	repo, args, _, index := newCandidateInspectionReview(t, "candidate\n", true)
	attributes := filepath.Join(repo, "hostile attributes")
	writeReviewStartCandidate(t, repo, "hostile attributes", "tracked.txt binary\n", 0o644)
	config := filepath.Join(repo, "hostile.gitconfig")
	writeReviewStartCandidate(t, repo, "hostile.gitconfig", "[core]\n\tattributesFile = "+filepath.ToSlash(attributes)+"\n[diff]\n\texternal = false\n\talgorithm = patience\n\tcontext = 0\n", 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_SYSTEM", config)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "diff.noprefix")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_DIFF_OPTS", "--unified=0")
	t.Setenv("GIT_ATTR_SOURCE", "HEAD")
	runReviewCLIGit(t, repo, "config", "core.attributesFile", filepath.ToSlash(attributes))
	runReviewCLIGit(t, repo, "config", "diff.external", "false")
	before := [2]string{runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"), runReviewCLIGit(t, repo, "rev-parse", "HEAD")}

	tests := []inspectionCase{
		{name: "name-status", argv: []string{"--operation", "name-status"}, want: "A\tdrive name 'quote';$.txt\nM\ttracked.txt\n"},
		{name: "numstat", argv: []string{"--operation", "numstat"}, want: "1\t0\tdrive name 'quote';$.txt\n1\t1\ttracked.txt\n"},
		{name: "stat", argv: []string{"--operation", "stat", "--path-index", fmt.Sprint(index)}, want: " tracked.txt | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n"},
		{name: "patch", argv: []string{"--operation", "patch", "--path-index", fmt.Sprint(index)}, want: "diff --git a/tracked.txt b/tracked.txt\nindex df967b96a579e45a18b8251732d16804b2e56a55..02e8acdcc44da4d68465994d590e44bcab3296b2 100644\n--- a/tracked.txt\n+++ b/tracked.txt\n@@ -1 +1 @@\n-base\n+candidate\n"},
		{name: "object-base", argv: []string{"--operation", "object", "--path-index", fmt.Sprint(index), "--side", "base"}, want: "base\n"},
		{name: "object-candidate", argv: []string{"--operation", "object", "--path-index", fmt.Sprint(index), "--side", "candidate"}, want: "candidate\n"},
	}
	for _, test := range tests {
		var output bytes.Buffer
		if err := RunReview(slices.Concat([]string{"inspect-candidate"}, args, test.argv), &output); err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if output.String() != test.want {
			t.Fatalf("%s output mismatch\ngot:  %q\nwant: %q", test.name, output.String(), test.want)
		}
	}
	after := [2]string{runReviewCLIGit(t, repo, "status", "--porcelain=v2", "--untracked-files=all"), runReviewCLIGit(t, repo, "rev-parse", "HEAD")}
	if after != before {
		t.Fatalf("inspection mutated worktree, index, or HEAD: before=%q after=%q", before, after)
	}
}

func TestReviewInspectCandidateCarriesAggregateDeadlineAndCancels(t *testing.T) {
	reviewEnabledHome(t)
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	args = append(args, "--operation", "name-status")
	contexts := map[error]func(*reviewInspectCandidateDeps){
		context.DeadlineExceeded: func(deps *reviewInspectCandidateDeps) { deps.timeout = 0 },
		context.Canceled:         func(deps *reviewInspectCandidateDeps) { deps.operationContext = canceledInspectionContext },
	}
	for want, configure := range contexts {
		for _, phase := range []string{"resolver", "discovery", "inspector"} {
			t.Run(want.Error()+"/"+phase, func(t *testing.T) {
				deps := failingInspectionDeps(repo, record, phase)
				configure(&deps)
				_, err := runReviewInspectCandidate(args, io.Discard, deps)
				if !errors.Is(err, want) {
					t.Fatalf("%s %s error = %T %v", want, phase, err, err)
				}
			})
		}
	}

	independent := errors.New("independent provider refusal")
	deps := failingInspectionDeps(repo, record, "discovery")
	deps.timeout = 0
	deps.discover = func(ctx context.Context, _, _ string, _ bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
		<-ctx.Done()
		return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, independent
	}
	_, err := runReviewInspectCandidate(args, io.Discard, deps)
	if !errors.Is(err, independent) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("independent error was masked: %T %v", err, err)
	}
}

func failingInspectionDeps(repo string, record reviewtransaction.CompactRecord, phase string) reviewInspectCandidateDeps {
	deps := reviewInspectCandidateDependencies()
	deps.resolve = func(context.Context, string, reviewtransaction.ReviewRepositoryContextBinding) (string, error) {
		return repo, nil
	}
	deps.discover = func(context.Context, string, string, bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
		return reviewtransaction.CompactStore{}, record, nil
	}
	switch phase {
	case "resolver":
		deps.resolve = func(ctx context.Context, _ string, _ reviewtransaction.ReviewRepositoryContextBinding) (string, error) {
			return "", inspectionContextError(ctx)
		}
	case "discovery":
		deps.discover = func(ctx context.Context, _, _ string, _ bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
			return reviewtransaction.CompactStore{}, reviewtransaction.CompactRecord{}, inspectionContextError(ctx)
		}
	case "inspector":
		deps.inspect = func(_ reviewtransaction.SnapshotBuilder, ctx context.Context, _ reviewtransaction.Snapshot, _ string, _ int, _ string) ([]byte, error) {
			return nil, inspectionContextError(ctx)
		}
	}
	return deps
}

func inspectionContextError(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

func canceledInspectionContext(context.Context, time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, func() {}
}

func TestReviewInspectCandidateRejectsUnboundInput(t *testing.T) {
	reviewEnabledHome(t)
	var help bytes.Buffer
	if err := RunReview([]string{"inspect-candidate", "--help"}, &help); err != nil {
		t.Fatal(err)
	}
	wantHelp := "Global form: --operation name-status|numstat\nPath form: --operation stat|patch --path-index <n>\nObject form: --operation object --path-index <n> --side base|candidate"
	if !strings.Contains(help.String(), wantHelp) {
		t.Fatalf("inspect-candidate help omits closed forms:\n%s", help.String())
	}
	_, valid, _, index := newCandidateInspectionReview(t, "candidate\n", true)
	valid = append(valid, "--operation", "name-status")
	pathIndex := fmt.Sprint(index)
	tests := []inspectionCase{
		{name: "unknown operation", argv: append(slices.Clone(valid), "--operation", "show"), want: "unknown candidate inspection operation"},
		{name: "unknown flag", argv: append(slices.Clone(valid), "--operation", "patch", "--path-index", pathIndex, "--binary"), want: "flag provided but not defined"},
		{name: "positional", argv: append(slices.Clone(valid), "--operation", "name-status", "HEAD"), want: "requires the exact provider-issued"},
		{name: "global with selector", argv: append(slices.Clone(valid), "--operation", "name-status", "--path-index", pathIndex), want: "global candidate inspection"},
		{name: "path operation without selector", argv: append(slices.Clone(valid), "--operation", "patch"), want: "path candidate inspection"},
		{name: "patch with side", argv: append(slices.Clone(valid), "--operation", "patch", "--path-index", pathIndex, "--side", "base"), want: "path candidate inspection"},
		{name: "object without side", argv: append(slices.Clone(valid), "--operation", "object", "--path-index", pathIndex), want: "object candidate inspection"},
		{name: "invalid side", argv: append(slices.Clone(valid), "--operation", "object", "--path-index", pathIndex, "--side", "worktree"), want: "object candidate inspection"},
		{name: "out of range selector", argv: append(slices.Clone(valid), "--operation", "patch", "--path-index", "99"), want: "exact canonical path index"},
		{name: "wrong context", argv: replaceInspectionArg(t, valid, "--repository-context", "rctx1_"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "stale binding", argv: replaceInspectionArg(t, valid, "--expected-revision", "sha256:"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "wrong lineage", argv: replaceInspectionArg(t, valid, "--lineage", "wrong-lineage"), want: "repository_context_"},
		{name: "wrong target", argv: replaceInspectionArg(t, valid, "--target", "sha256:"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "wrong binding", argv: replaceInspectionArg(t, valid, "--lens", "review-risk"), want: "binding does not match"},
		{name: "wrong order", argv: replaceInspectionArg(t, valid, "--order", "99"), want: "binding does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := RunReviewInspectCandidate(test.argv, io.Discard); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid request error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReviewInspectCandidateRejectsOversizedObject(t *testing.T) {
	reviewEnabledHome(t)
	_, args, _, index := newCandidateInspectionReview(t, string(bytes.Repeat([]byte("x"), reviewtransaction.MaxFrozenCandidateDiffBytes+1)), false)
	args = append(args, "--operation", "object", "--path-index", fmt.Sprint(index), "--side", "candidate")
	if err := RunReviewInspectCandidate(args, io.Discard); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized object error = %v", err)
	}
}

func TestReviewInspectCandidateInspectsProviderBoundCorrectedTree(t *testing.T) {
	reviewEnabledHome(t)
	repo, args, ready, store, index := newTargetedCandidateInspectionReview(t)
	nonTerminal, err := store.Load()
	if err != nil || nonTerminal.State.State != reviewtransaction.StateCorrectionRequired ||
		nonTerminal.Revision != ready.Authority.Revision || nonTerminal.State.CorrectionAttemptConsumed() {
		t.Fatalf("corrected inspection must use current unconsumed correction authority: %#v, %v", nonTerminal, err)
	}
	before := readReviewOperationFile(t, store.StatePath())
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 3 }\n", 0o644)
	t.Chdir(t.TempDir())
	var output bytes.Buffer
	if err := RunReviewInspectCandidate(append(args, "--operation", "object", "--path-index", fmt.Sprint(index), "--side", "candidate"), &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "package candidate\n\nfunc value() int { return 2 }\n" {
		t.Fatalf("corrected immutable inspection = %q", output.String())
	}
	if after := readReviewOperationFile(t, store.StatePath()); !bytes.Equal(after, before) {
		t.Fatal("inspection changed authority bytes")
	}

	validation := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validation, facadeValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", ready.Authority.LineageID, "--validation", validation, "--captured-evidence=true"}, io.Discard); err == nil {
		t.Fatal("FINALIZE accepted live correction drift")
	}
	stillOpen, err := store.Load()
	if err != nil || stillOpen.State.State != reviewtransaction.StateCorrectionRequired || len(stillOpen.State.CorrectionAttempts) != 0 {
		t.Fatalf("drifted FINALIZE changed correction authority: %#v, %v", stillOpen.State, err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	var finalizedOutput bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", ready.Authority.LineageID, "--validation", validation, "--captured-evidence=true"}, &finalizedOutput); err != nil {
		t.Fatalf("FINALIZE after restoring corrected candidate: %v", err)
	}
	var finalized ReviewFacadeFinalizeResult
	if err := json.Unmarshal(finalizedOutput.Bytes(), &finalized); err != nil || finalized.State != reviewtransaction.StateApproved {
		t.Fatalf("corrected FINALIZE terminal approval = %#v, %v", finalized, err)
	}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal FINALIZE did not burn corrected authority: %v", err)
	}
	if _, err := os.Stat(store.StatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal FINALIZE left corrected authority state: %v", err)
	}
	if err := RunReviewInspectCandidate(append(args, "--operation", "name-status"), io.Discard); err == nil {
		t.Fatal("terminal corrected inspection unexpectedly regained authority")
	}
	if _, err := os.Stat(store.StatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal corrected inspection recreated authority: %v", err)
	}
}

func TestReviewInspectCandidatePreservesCorrectedEvidenceError(t *testing.T) {
	reviewEnabledHome(t)
	_, args, ready, store, _ := newTargetedCandidateInspectionReview(t)
	evidenceDir := filepath.Join(store.Dir, reviewtransaction.CompactFinalEvidenceDir,
		strings.TrimPrefix(ready.ValidationRequest.CorrectionTargetIdentity, "sha256:"))
	if err := os.RemoveAll(evidenceDir); err != nil {
		t.Fatal(err)
	}
	err := RunReviewInspectCandidate(append(args, "--operation", "name-status"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires passed repository verification evidence") || strings.Contains(err.Error(), "repository_context_") {
		t.Fatalf("missing corrected evidence error = %v", err)
	}
}

func TestReviewInspectCandidateRejectsTargetedBindingDecoys(t *testing.T) {
	reviewEnabledHome(t)
	_, lens, _, _ := newCandidateInspectionReview(t, "candidate\n", false)
	_, targeted, ready, _, _ := newTargetedCandidateInspectionReview(t)
	targeted = append(targeted, "--operation", "name-status")
	lens = append(lens, "--operation", "name-status", "--request-hash", ready.ValidationRequest.RequestHash)
	tests := []inspectionCase{
		{name: "missing context", argv: removeInspectionArg(targeted, "--repository-context"), want: "requires the exact provider-issued"},
		{name: "forged context", argv: replaceInspectionArg(t, targeted, "--repository-context", "rctx1_"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "original lens context", argv: replaceInspectionArg(t, targeted, "--repository-context", lens[slices.Index(lens, "--repository-context")+1]), want: "repository_context_"},
		{name: "stale revision", argv: replaceInspectionArg(t, targeted, "--expected-revision", "sha256:"+strings.Repeat("0", 64)), want: "context does not match binding"},
		{name: "forged target", argv: replaceInspectionArg(t, targeted, "--target", "sha256:"+strings.Repeat("0", 64)), want: "context does not match binding"},
		{name: "forged request hash", argv: replaceInspectionArg(t, targeted, "--request-hash", "sha256:"+strings.Repeat("0", 64)), want: "request hash does not match authority"},
		{name: "lens supplied", argv: append(slices.Clone(targeted), "--lens", "review-risk"), want: "does not accept --lens or --order"},
		{name: "order supplied", argv: append(slices.Clone(targeted), "--order", "0"), want: "does not accept --lens or --order"},
		{name: "targeted-only hash on lens mode", argv: lens, want: "request hash is valid only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := RunReviewInspectCandidate(test.argv, io.Discard); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("targeted inspection error = %v, want %q", err, test.want)
			}
		})
	}
}

func newCandidateInspectionReview(t *testing.T, tracked string, hostile bool) (string, []string, reviewtransaction.CompactRecord, int) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", tracked, 0o644)
	if hostile {
		writeReviewStartCandidate(t, repo, `drive name 'quote';$.txt`, "hostile path\n", 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "inspect-candidate")
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--repository-context", started.RepositoryContext.Handle,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--lens", started.SelectedLenses[0], "--order", "0",
	}
	index := slices.Index(record.State.InitialSnapshot.Paths, "tracked.txt")
	if index < 0 {
		t.Fatalf("snapshot paths omit tracked.txt: %v", record.State.InitialSnapshot.Paths)
	}
	return repo, args, record, index
}

func newTargetedCandidateInspectionReview(t *testing.T) (string, []string, ReviewTargetStatusResult, reviewtransaction.CompactStore, int) {
	t.Helper()
	reviewEnabledHome(t)
	repo, started, store := submissionDescriptorCorrectionFixture(t)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
	waiting := submissionDescriptorStatus(t, repo, started.LineageID)
	evidence := filepath.Join(t.TempDir(), "correction-evidence.txt")
	if err := os.WriteFile(evidence, []byte("repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", waiting.ValidationRequest.CorrectionTargetIdentity, "--expected-revision", waiting.Authority.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidence}, io.Discard); err != nil {
		t.Fatal(err)
	}
	ready := submissionDescriptorStatus(t, repo, started.LineageID)
	input := submissionDescriptorInput(t, ready)
	arguments := requiredReviewTransitionArguments(t, input.Arguments, "repository-context", "expected-revision", "lineage", "target", "purpose", "request-hash")
	index := slices.Index(ready.Projection.Paths, "candidate.go")
	if index < 0 {
		t.Fatalf("corrected target manifest omits candidate.go: %v", ready.Projection.Paths)
	}
	return repo, []string{"--repository-context", arguments["repository-context"], "--expected-revision", arguments["expected-revision"],
		"--lineage", arguments["lineage"], "--target", arguments["target"], "--purpose", arguments["purpose"], "--request-hash", arguments["request-hash"]}, ready, store, index
}

func requiredReviewTransitionArguments(t *testing.T, arguments []ReviewTransitionArgument, names ...string) map[string]string {
	t.Helper()
	values, err := reviewTransitionArgumentMap(arguments)
	if err != nil {
		t.Fatalf("transition arguments = %#v: %v", arguments, err)
	}
	for _, name := range names {
		if value, found := values[name]; !found || strings.TrimSpace(value) == "" {
			t.Fatalf("transition arguments omit required %q: %#v", name, arguments)
		}
	}
	return values
}

func replaceInspectionArg(t *testing.T, args []string, name, value string) []string {
	t.Helper()
	result := slices.Clone(args)
	for index, argument := range result {
		if argument == name && index+1 < len(result) {
			result[index+1] = value
			return result
		}
	}
	t.Fatalf("inspection arguments omit %q: %v", name, args)
	return nil
}

func removeInspectionArg(args []string, name string) []string {
	result := slices.Clone(args)
	index := slices.Index(result, name)
	return append(result[:index], result[index+2:]...)
}
