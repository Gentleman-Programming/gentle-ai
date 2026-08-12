package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestPrePushValidationPreservesForkBaseSelectorAndRejectsInvalidSelectors(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	origin := filepath.Join(t.TempDir(), "origin.git")
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, origin)
	runReviewCLIGit(t, repo, "clone", "--bare", repo, upstream)
	runReviewCLIGit(t, repo, "remote", "add", "origin", origin)
	runReviewCLIGit(t, repo, "remote", "add", "upstream", upstream)

	parent := filepath.Join(t.TempDir(), "upstream-work")
	runReviewCLIGit(t, repo, "clone", "--no-local", upstream, parent)
	runReviewCLIGit(t, parent, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, parent, "config", "user.name", "Test")
	writeReviewStartCandidate(t, parent, "docs/base.md", "# Upstream base\n", 0o644)
	runReviewCLIGit(t, parent, "commit", "-qm", "advance upstream base")
	runReviewCLIGit(t, parent, "push", "-q", "origin", "HEAD:refs/heads/main")
	runReviewCLIGit(t, repo, "fetch", "-q", "upstream", "main")

	upstreamBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "upstream/main"))
	runReviewCLIGit(t, repo, "checkout", "-q", "-B", "feature", upstreamBase)
	writeReviewStartCandidate(t, repo, "docs/feature.md", "# Feature\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "add feature")
	runReviewCLIGit(t, repo, "--git-dir", origin, "update-ref", "refs/heads/feature", base)
	runReviewCLIGit(t, repo, "config", "branch.feature.remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch.feature.merge", "refs/heads/feature")
	runReviewCLIGit(t, repo, "config", "branch.feature.pushRemote", "origin")
	runReviewCLIGit(t, repo, "--git-dir", origin, "update-ref", "refs/heads/main", base)

	const lineage = "fork-symbolic-pre-push"
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", "upstream/main", "--committed-only",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	candidateBefore, err := os.ReadFile(filepath.Join(repo, "docs", "feature.md"))
	if err != nil {
		t.Fatal(err)
	}

	status := selectorTransitionStatus(t, repo, "--lineage", lineage, "--gate", "pre-push", "--base-ref", "upstream/main")
	if status.NextTransition == nil || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.validate" ||
		selectorTransitionArguments(t, status)["base-ref"] != "upstream/main" {
		t.Fatalf("symbolic pre-push STATUS = %#v", status.NextTransition)
	}
	assertReviewGateResult(t, executeSelectorTransition(t, repo, status), reviewtransaction.GateAllow)

	rawBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "upstream/main"))
	for _, test := range []struct {
		name     string
		selector string
		want     string
	}{
		{name: "raw nontracking commit", selector: rawBase, want: "advertised tracking branch"},
		{name: "ambiguous unqualified branch", selector: "main", want: "missing or ambiguous"},
		{name: "unadvertised qualified branch", selector: "upstream/missing", want: "missing or ambiguous"},
		{name: "local-only branch", selector: "local-only", want: "missing or ambiguous"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.selector == "local-only" {
				runReviewCLIGit(t, repo, "branch", test.selector, "HEAD")
			}
			output.Reset()
			err := RunReviewFacadeValidate([]string{
				"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePrePush), "--base-ref", test.selector,
			}, &output)
			if err == nil || !strings.Contains(err.Error(), test.want) && !strings.Contains(output.String(), test.want) {
				t.Fatalf("selector %q error=%v output=%s, want %q", test.selector, err, output.String(), test.want)
			}
			var result ReviewValidateResult
			if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
				t.Fatalf("decode denied selector %q: %v\n%s", test.selector, decodeErr, output.String())
			}
			if result.Allowed || result.Result == reviewtransaction.GateAllow {
				t.Fatalf("selector %q was allowed: %#v", test.selector, result)
			}
			stateAfter, stateErr := os.ReadFile(store.StatePath())
			receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
			candidateAfter, candidateErr := os.ReadFile(filepath.Join(repo, "docs", "feature.md"))
			if stateErr != nil || receiptErr != nil || candidateErr != nil ||
				!bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) ||
				!bytes.Equal(candidateBefore, candidateAfter) {
				t.Fatalf("denied selector %q mutated candidate or authority: state=%v receipt=%v candidate=%v", test.selector, stateErr, receiptErr, candidateErr)
			}
		})
	}
}
