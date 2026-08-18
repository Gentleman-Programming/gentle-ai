package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeReviewStartLargeCandidate(t *testing.T, repo string, lines int) []byte {
	t.Helper()
	var builder strings.Builder
	for index := 0; index < lines; index++ {
		fmt.Fprintf(&builder, "line %08d payload aaaaaaaaaaaaaaaaaaaa\n", index)
	}
	if err := os.WriteFile(filepath.Join(repo, "big.txt"), []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "big.txt")
	return []byte(runReviewCLIGitOutput(t, repo, "diff", "--cached", "--binary", "--full-index",
		"--no-color", "--no-renames", "--no-ext-diff", "--no-textconv"))
}

func TestLargeCandidateSTARTAndStatusCarryOnlyNativeGitReferences(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	fullDiff := writeReviewStartLargeCandidate(t, repo, 110000)
	if len(fullDiff) <= 4<<20 {
		t.Fatalf("fixture diff = %d bytes, want larger than the retired 4 MiB inline limit", len(fullDiff))
	}

	var started bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "review-start-large-native-git",
	}), &started); err != nil {
		t.Fatalf("START refused a large Git-addressable candidate: %v\n%s", err, started.String())
	}
	assertNoEagerCandidateDiff(t, "START", started.Bytes(), fullDiff)

	var start ReviewIntegrationStartResult
	if err := json.Unmarshal(started.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	if start.BaseTree == "" || start.CandidateTree == "" || start.ChangedPathManifest == nil ||
		len(*start.ChangedPathManifest) != 1 || (*start.ChangedPathManifest)[0].Path != "big.txt" {
		t.Fatalf("START native Git context = %#v", start)
	}

	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--next-transition",
	}, &status); err != nil {
		t.Fatal(err)
	}
	assertNoEagerCandidateDiff(t, "STATUS", status.Bytes(), fullDiff)

	var result ReviewTargetStatusResult
	if err := json.Unmarshal(status.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.NextTransition == nil || result.NextTransition.Collect == nil || len(result.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("large-candidate STATUS produced no reviewer input: %#v", result.NextTransition)
	}
	for _, input := range result.NextTransition.Collect.Inputs {
		if input.BaseTree != start.BaseTree || input.CandidateTree != start.CandidateTree ||
			input.ChangedPathManifest == nil || input.ArtifactSubject == nil {
			t.Fatalf("collect input lost native Git context: %#v", input)
		}
	}
	// The reviewer recipe is plain read-only native Git against the frozen
	// trees: compact discovery first, then a literal-pathspec selective diff.
	discovery := runReviewCLIGitOutput(t, repo, "--no-pager", "diff", "--name-status", "--no-renames",
		start.BaseTree, start.CandidateTree)
	if strings.TrimSpace(discovery) != "A\tbig.txt" {
		t.Fatalf("compact discovery = %q, want the single added path", discovery)
	}
	numstat := runReviewCLIGitOutput(t, repo, "--no-pager", "diff", "--numstat", "--no-renames",
		start.BaseTree, start.CandidateTree)
	if !strings.Contains(numstat, "big.txt") {
		t.Fatalf("numstat discovery = %q, want big.txt", numstat)
	}
	selective := runReviewCLIGitOutput(t, repo, "--no-pager", "diff", "--no-ext-diff", "--no-textconv",
		"--full-index", "--no-renames", "--diff-algorithm=myers",
		start.BaseTree, start.CandidateTree, "--", ":(literal)big.txt")
	if len(selective) <= 4<<20 || !strings.Contains(selective, "line 00000000 payload") ||
		!strings.Contains(selective, "line 00109999 payload") {
		t.Fatalf("selective native Git diff did not reconstruct the oversized candidate (%d bytes)", len(selective))
	}
}

func assertNoEagerCandidateDiff(t *testing.T, surface string, payload, fullDiff []byte) {
	t.Helper()
	if bytes.Contains(payload, []byte(`"candidate_diff"`)) || bytes.Contains(payload, fullDiff) ||
		bytes.Contains(payload, []byte(base64.StdEncoding.EncodeToString(fullDiff))) {
		t.Fatalf("%s eagerly contains the full candidate diff or its Base64 encoding", surface)
	}
}
