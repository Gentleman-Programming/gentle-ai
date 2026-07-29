package reviewtransaction

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These tests pin the fix for the second half of issue 1778: `treeBlobSizes`
// (git ls-tree) and processBoundaryRiskReasons's git grep call both expand
// one ":(literal)<path>" pathspec per changed file into argv, scaling with
// the size of a review candidate. Unlike the `git add` half of 1778, neither
// `git ls-tree` nor `git grep` accepts --pathspec-from-file, so both sites
// are fixed with bounded batching through the shared batchLiteralPathspecs
// helper instead of the stdin-transport technique.

func TestBatchLiteralPathspecsSplitsIntoMultipleBudgetBoundedBatches(t *testing.T) {
	original := gitPathspecArgvBudget
	gitPathspecArgvBudget = 200
	t.Cleanup(func() { gitPathspecArgvBudget = original })

	// Each pathspec is ":(literal)path-NNNN" plus a 1-character separator
	// budget, roughly 21 characters. Build enough paths to comfortably
	// exceed the 200-character budget several times over.
	count := (gitPathspecArgvBudget / 21) * 3
	paths := make([]string, count)
	for index := range paths {
		paths[index] = fmt.Sprintf("path-%04d", index)
	}

	batches := batchLiteralPathspecs(paths, 0)
	if len(batches) < 2 {
		t.Fatalf("batches = %d, want at least 2 for %d paths under budget %d", len(batches), count, gitPathspecArgvBudget)
	}
	for batchIndex, batch := range batches {
		length := 0
		for _, pathspec := range batch {
			length += len(pathspec) + 1
		}
		if length > gitPathspecArgvBudget {
			t.Fatalf("batch %d length %d exceeds budget %d", batchIndex, length, gitPathspecArgvBudget)
		}
	}
}

func TestBatchLiteralPathspecsCoversEveryPathExactlyOnce(t *testing.T) {
	original := gitPathspecArgvBudget
	gitPathspecArgvBudget = 150
	t.Cleanup(func() { gitPathspecArgvBudget = original })

	paths := make([]string, 500)
	for index := range paths {
		paths[index] = fmt.Sprintf("dir/file-%04d.go", index)
	}
	batches := batchLiteralPathspecs(paths, 0)
	seen := make(map[string]int, len(paths))
	for _, batch := range batches {
		for _, pathspec := range batch {
			seen[pathspec]++
		}
	}
	if len(seen) != len(paths) {
		t.Fatalf("covered %d distinct pathspecs, want %d", len(seen), len(paths))
	}
	for _, logicalPath := range paths {
		if count := seen[literalPathspec(logicalPath)]; count != 1 {
			t.Fatalf("pathspec for %q appeared %d times, want exactly 1", logicalPath, count)
		}
	}
}

func TestBatchLiteralPathspecsOversizedSinglePathGetsItsOwnBatchInsteadOfVanishing(t *testing.T) {
	original := gitPathspecArgvBudget
	gitPathspecArgvBudget = 50
	t.Cleanup(func() { gitPathspecArgvBudget = original })

	huge := strings.Repeat("a", 500)
	batches := batchLiteralPathspecs([]string{"short.txt", huge, "other.txt"}, 0)
	found := false
	for _, batch := range batches {
		for _, pathspec := range batch {
			if pathspec == literalPathspec(huge) {
				found = true
				if len(batch) != 1 {
					t.Fatalf("oversized path batch also carries %d other pathspecs, want it alone", len(batch)-1)
				}
			}
		}
	}
	if !found {
		t.Fatal("oversized single path vanished instead of being passed in a batch of its own")
	}
}

func TestBatchLiteralPathspecsSmallSetProducesExactlyOneBatch(t *testing.T) {
	batches := batchLiteralPathspecs([]string{"a.go", "b.go", "c.go"}, 100)
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want exactly 1 for a small path set", len(batches))
	}
}

func TestTreeBlobSizesSmallPathSetUsesExactlyOneInvocation(t *testing.T) {
	repo := initSnapshotRepo(t)
	paths := []string{"a.txt", "b.txt"}
	for _, logicalPath := range paths {
		writeSnapshotFile(t, repo, logicalPath, "x\n")
	}
	gitSnapshot(t, repo, "add", "--", ".")
	gitSnapshot(t, repo, "commit", "-m", "small blob fixture")
	tree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

	var invocations int
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "ls-tree") {
			invocations++
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	if _, err := treeBlobSizes(context.Background(), repo, tree, paths); err != nil {
		t.Fatalf("treeBlobSizes() error = %v", err)
	}
	if invocations != 1 {
		t.Fatalf("ls-tree invocations = %d, want exactly 1 for a small path set", invocations)
	}
}

// TestTreeBlobSizesBatchesAcrossMultipleGitInvocations forces a small budget
// so a moderate path set must split across multiple `git ls-tree`
// invocations, and proves no single invocation's argv exceeds the Windows
// 32767-character command-line limit.
func TestTreeBlobSizesBatchesAcrossMultipleGitInvocations(t *testing.T) {
	repo := initSnapshotRepo(t)
	const count = 40
	paths := make([]string, count)
	for index := range paths {
		name := fmt.Sprintf("blobs/file-%03d.txt", index)
		writeSnapshotFile(t, repo, name, fmt.Sprintf("content-%03d\n", index))
		paths[index] = name
	}
	gitSnapshot(t, repo, "add", "--", ".")
	gitSnapshot(t, repo, "commit", "-m", "blob batch fixture")
	tree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

	originalBudget := gitPathspecArgvBudget
	gitPathspecArgvBudget = 200
	t.Cleanup(func() { gitPathspecArgvBudget = originalBudget })

	var invocations int
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "ls-tree") {
			invocations++
			argvLength := 0
			for _, arg := range args {
				argvLength += len(arg) + 1
			}
			if argvLength > 32767 {
				t.Fatalf("ls-tree invocation argv length %d exceeds the Windows command-line limit", argvLength)
			}
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	blobs, err := treeBlobSizes(context.Background(), repo, tree, paths)
	if err != nil {
		t.Fatalf("treeBlobSizes() error = %v", err)
	}
	if invocations < 2 {
		t.Fatalf("ls-tree invocations = %d, want at least 2 for a batched scan", invocations)
	}
	if len(blobs) != count {
		t.Fatalf("blobs = %d, want %d", len(blobs), count)
	}
	seen := make(map[string]bool, len(blobs))
	for _, blob := range blobs {
		seen[blob.path] = true
	}
	for _, logicalPath := range paths {
		if !seen[logicalPath] {
			t.Fatalf("blob for %q missing from batched result", logicalPath)
		}
	}
}

// TestTreeBlobSizesSingleBatchMatchesUnbatchedGitLsTree is the behavioral
// equivalence regression: for a set small enough to fit in one batch, the
// batched result must equal a manually built unbatched
// `git ls-tree -r -l -z <tree> -- :(literal)<path>...` call.
func TestTreeBlobSizesSingleBatchMatchesUnbatchedGitLsTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	files := map[string]string{
		"a.txt":   "hello\n",
		"b.txt":   "hello world\n",
		"c/d.txt": "x\n",
	}
	paths := make([]string, 0, len(files))
	for name, content := range files {
		writeSnapshotFile(t, repo, name, content)
		paths = append(paths, name)
	}
	sort.Strings(paths)
	gitSnapshot(t, repo, "add", "--", ".")
	gitSnapshot(t, repo, "commit", "-m", "small blob fixture")
	tree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))

	batched, err := treeBlobSizes(context.Background(), repo, tree, paths)
	if err != nil {
		t.Fatalf("treeBlobSizes() error = %v", err)
	}

	args := append([]string{"ls-tree", "-r", "-l", "-z", tree, "--"}, literalPathspecs(paths)...)
	rawOutput, err := runGit(context.Background(), repo, nil, nil, args...)
	if err != nil {
		t.Fatalf("unbatched ls-tree: %v", err)
	}
	var want []treeBlob
	for _, entry := range bytes.Split(rawOutput, []byte{0}) {
		tab := bytes.IndexByte(entry, '\t')
		fields := strings.Fields(string(entry[:max(tab, 0)]))
		if tab < 0 || len(fields) != 4 {
			continue
		}
		size, parseErr := strconv.ParseInt(fields[3], 10, 64)
		if parseErr != nil {
			t.Fatalf("parse blob size: %v", parseErr)
		}
		want = append(want, treeBlob{path: string(entry[tab+1:]), size: size})
	}

	sortTreeBlobsForTest(batched)
	sortTreeBlobsForTest(want)
	if !reflect.DeepEqual(batched, want) {
		t.Fatalf("batched treeBlobSizes() = %#v, want %#v (unbatched)", batched, want)
	}
}

func sortTreeBlobsForTest(blobs []treeBlob) {
	sort.Slice(blobs, func(left, right int) bool { return blobs[left].path < blobs[right].path })
}

// TestProcessBoundaryRiskReasonsGrepBatchesMixedMatchAndNoMatch forces every
// changed path into its own grep batch and proves git grep's "no matches"
// exit code 1 in one batch does not abort the scan or get mistaken for a
// real failure, while a matching batch still surfaces its reason.
func TestProcessBoundaryRiskReasonsGrepBatchesMixedMatchAndNoMatch(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tools/spawn.py", "import subprocess\nsubprocess.run(argv)\n")
	writeSnapshotFile(t, repo, "tools/neutral.py", "def helper(value):\n    return value\n")
	untracked := []string{"tools/neutral.py", "tools/spawn.py"}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: untracked})
	if err != nil {
		t.Fatal(err)
	}

	originalBudget := gitPathspecArgvBudget
	// Small enough that the fixed grep argv prefix alone consumes the whole
	// budget, so every path deterministically lands in its own batch.
	gitPathspecArgvBudget = 40
	t.Cleanup(func() { gitPathspecArgvBudget = originalBudget })

	var grepInvocations int
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "grep") {
			grepInvocations++
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("AssessSnapshotRisk() error = %v", err)
	}
	if grepInvocations < 2 {
		t.Fatalf("grep invocations = %d, want at least 2 (one matching batch, one non-matching batch)", grepInvocations)
	}
	want := RiskReason{Code: RiskReasonProcessBoundary, Signal: SignalShellProcess, Path: "tools/spawn.py"}
	if assessment.Level != RiskHigh || !reflect.DeepEqual(assessment.Reasons, []RiskReason{want}) {
		t.Fatalf("AssessSnapshotRisk() = %#v, want high with %#v", assessment, want)
	}
}
