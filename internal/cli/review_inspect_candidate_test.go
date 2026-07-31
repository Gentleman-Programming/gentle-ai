package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	repo, args, _, index := newCandidateInspectionReview(t, "candidate\n", true)
	attributes := repo + "/hostile attributes"
	writeReviewStartCandidate(t, repo, "hostile attributes", "tracked.txt binary\n", 0o644)
	config := repo + "/hostile.gitconfig"
	writeReviewStartCandidate(t, repo, "hostile.gitconfig", "[core]\n\tattributesFile = "+attributes+"\n[diff]\n\texternal = false\n\talgorithm = patience\n\tcontext = 0\n", 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_SYSTEM", config)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "diff.noprefix")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_DIFF_OPTS", "--unified=0")
	t.Setenv("GIT_ATTR_SOURCE", "HEAD")
	runReviewCLIGit(t, repo, "config", "core.attributesFile", attributes)
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
		{name: "wrong context", argv: replaceInspectionArg(valid, "--repository-context", "rctx1_"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "stale binding", argv: replaceInspectionArg(valid, "--expected-revision", "sha256:"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "wrong lineage", argv: replaceInspectionArg(valid, "--lineage", "wrong-lineage"), want: "repository_context_"},
		{name: "wrong target", argv: replaceInspectionArg(valid, "--target", "sha256:"+strings.Repeat("0", 64)), want: "repository_context_"},
		{name: "wrong binding", argv: replaceInspectionArg(valid, "--lens", "review-risk"), want: "binding does not match"},
		{name: "wrong order", argv: replaceInspectionArg(valid, "--order", "99"), want: "binding does not match"},
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
	_, args, _, index := newCandidateInspectionReview(t, string(bytes.Repeat([]byte("x"), reviewtransaction.MaxFrozenCandidateDiffBytes+1)), false)
	args = append(args, "--operation", "object", "--path-index", fmt.Sprint(index), "--side", "candidate")
	if err := RunReviewInspectCandidate(args, io.Discard); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("oversized object error = %v", err)
	}
}

func newCandidateInspectionReview(t *testing.T, tracked string, hostile bool) (string, []string, reviewtransaction.CompactRecord, int) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

func replaceInspectionArg(args []string, name, value string) []string {
	result := slices.Clone(args)
	result[slices.Index(result, name)+1] = value
	return result
}
