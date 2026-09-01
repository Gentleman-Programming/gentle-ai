package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewFacadeStartDirectRouteRefusesUncompletableReview reproduces the
// exact sequence from issue #2447 (reported by @danizafa on 2.2.4): a direct
// (non-negotiated) `review start --base-ref <base> --committed-only=true`
// used to create a lineage no reviewer lens could ever capture against,
// because the direct route's own response type (ReviewFacadeStartResult)
// never carries repository_context and the negotiated facade never
// rediscovers a lineage it did not create. The maintainer decision recorded
// on #2447 made refusal the permanent destination for the direct route, not
// a stopgap: it must refuse before anything is persisted, naming the
// negotiated form as the exit.
func TestReviewFacadeStartDirectRouteRefusesUncompletableReview(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	runReviewCLIGit(t, repo, "checkout", "-qb", "feature/change")
	changeFile := filepath.Join(repo, "change.go")
	if err := os.WriteFile(changeFile, []byte("package main\n\nfunc doSomething(x int) int {\n\tif x > 10 {\n\t\treturn x * 2\n\t}\n\treturn x + 1\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "change.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add a method")

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base, "--committed-only=true"}, &output)
	if err == nil {
		t.Fatalf("direct route start with lenses required succeeded = %s, want an up-front refusal", output.String())
	}
	if !strings.Contains(err.Error(), "gentle-ai review start --contract") {
		t.Fatalf("refusal error = %v, want it to name the exact negotiated review start continuation", err)
	}
	if !strings.Contains(err.Error(), "--base-ref") || !strings.Contains(err.Error(), "--committed-only") {
		t.Fatalf("refusal error = %v, want it to carry the base-diff continuation flags", err)
	}

	stores, storesErr := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo)
	if storesErr != nil {
		t.Fatal(storesErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused direct start persisted authority = %v, want none created", stores)
	}
}
