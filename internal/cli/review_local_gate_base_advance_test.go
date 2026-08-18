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

func TestValidateLocalParentMergeAllowsExplicitBase(t *testing.T) {
	reviewEnabledHome(t)
	tests := []struct {
		name      string
		gate      reviewtransaction.GateKind
		mergeArgs []string
	}{
		{name: "staged pre-commit", gate: reviewtransaction.GatePreCommit, mergeArgs: []string{"merge", "-q", "--no-commit", "--no-ff"}},
		{name: "committed pre-push", gate: reviewtransaction.GatePrePush, mergeArgs: []string{"merge", "-q", "--no-ff", "--no-edit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
			remote := filepath.Join(t.TempDir(), "remote.git")
			runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
			runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
			runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
			runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
			runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

			lineage := "local-gate-" + strings.ReplaceAll(tt.name, " ", "-")
			approveCommittedBaseDiff(t, repo, lineage)

			client := filepath.Join(t.TempDir(), "advance-client")
			runReviewCLIGit(t, repo, "clone", "-q", remote, client)
			runReviewCLIGit(t, client, "config", "user.email", "advance@example.com")
			runReviewCLIGit(t, client, "config", "user.name", "advance")
			writeReviewCLIFile(t, client, "docs/unrelated.md", "parent advance\n")
			runReviewCLIGit(t, client, "add", "-A")
			runReviewCLIGit(t, client, "commit", "-qm", "advance the parent")
			runReviewCLIGit(t, client, "push", "-q", "origin", "HEAD:"+branch)
			runReviewCLIGit(t, repo, "fetch", "-q", "origin")
			runReviewCLIGit(t, repo, append(tt.mergeArgs, "origin/"+branch)...)

			var output bytes.Buffer
			err := RunReview([]string{"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
				"--lineage", lineage, "--gate", string(tt.gate), "--base-ref", "origin/" + branch}, &output)
			if err != nil {
				t.Fatalf("explicit-base validation failed: %v\n%s", err, output.String())
			}
			var response struct {
				Result ReviewValidateResult `json:"result"`
			}
			if err := json.Unmarshal(output.Bytes(), &response); err != nil || !response.Result.Allowed {
				t.Fatalf("explicit-base validation = %#v, unmarshal = %v\n%s", response.Result, err, output.String())
			}
		})
	}
}

func approveCommittedBaseDiff(t *testing.T, repo, lineage string) {
	t.Helper()
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeReviewCLIFile(t, repo, "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed delivery")

	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := builder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.StartCompactAuthority(ctx, repo, reviewtransaction.CompactStartRequest{State: state, ExplicitLineage: true}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func writeReviewCLIFile(t *testing.T, repo, logicalPath, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
