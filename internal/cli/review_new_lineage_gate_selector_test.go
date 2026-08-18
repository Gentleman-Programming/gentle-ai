package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestGoverningAuthorityLiveEvidenceUsesStagedProjectionAtPreCommit is Wave 3
// Slice 5's task 6.7 fix for S4's own documented scope cut
// (governingAuthorityLiveEvidence's doc comment, review_governing_authority.go):
// pre-commit's live evidence must be resolved from the STAGED (index)
// content, exactly like the legacy gate-target composition
// (buildCompactGateRequestWithPushBase's own `if input.Gate == GatePreCommit
// { projection = ProjectionStaged }` branch) — not from the full live
// workspace, which also includes unstaged edits that pre-commit does not
// commit.
//
// The RED shape: start freezes a tracked file's content exactly as staged,
// and finalize (C1/C5 remediation: now CLI-wired, and required for a v3
// lineage to allow anything under default-deny) reaches an approved
// receipt. After that, the SAME tracked file is edited again WITHOUT
// staging the edit — the index still matches the frozen content, only the
// working tree has drifted. A gate-accurate pre-commit selector reads the
// index and finds an exact match (allow); the old always-workspace selector
// reads the dirty working tree and finds a mismatch (a false
// scope-changed/collect denial for a candidate pre-commit would never
// actually commit).
func TestGoverningAuthorityLiveEvidenceUsesStagedProjectionAtPreCommit(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")

	trackedPath := filepath.Join(repo, "tracked.md")
	if err := os.WriteFile(trackedPath, []byte("frozen staged content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.md")

	var start bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "gate-selector-lineage"}, &start); err != nil {
		t.Fatalf("new-lineage start: %v\n%s", err, start.String())
	}
	var finalize bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "gate-selector-lineage"}, &finalize); err != nil {
		t.Fatalf("new-lineage finalize: %v\n%s", err, finalize.String())
	}

	// Dirty the SAME tracked file further WITHOUT staging the change: the
	// index (what pre-commit actually commits) still equals the frozen
	// content; only the untracked working tree has drifted.
	if err := os.WriteFile(trackedPath, []byte("unrelated unstaged noise, never committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", "gate-selector-lineage", "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("pre-commit must allow the exact staged content despite unrelated unstaged noise: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
}

// TestGoverningAuthorityLiveEvidenceUsesWorkspaceProjectionAtPostApply
// triangulates against the pre-commit case above: post-apply's live evidence
// must still resolve from the full live workspace (unchanged from before
// this fix), because post-apply's whole purpose is checking work that has
// not been staged yet. The same unstaged-drift fixture that pre-commit
// (correctly) allows must still be reported as changed at post-apply.
func TestGoverningAuthorityLiveEvidenceUsesWorkspaceProjectionAtPostApply(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")

	trackedPath := filepath.Join(repo, "tracked.md")
	if err := os.WriteFile(trackedPath, []byte("frozen staged content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.md")

	var start bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "gate-selector-post-apply-lineage"}, &start); err != nil {
		t.Fatalf("new-lineage start: %v\n%s", err, start.String())
	}
	var finalize bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "gate-selector-post-apply-lineage"}, &finalize); err != nil {
		t.Fatalf("new-lineage finalize: %v\n%s", err, finalize.String())
	}
	staleManagedReviewerAssets(t, home)
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", "gate-selector-post-apply-lineage", "--gate", string(reviewtransaction.GatePostApply)}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), managedAssetProvenanceRefusal) {
		t.Fatalf("exact approved receipt with stale assets error = %v, want %q", err, managedAssetProvenanceRefusal)
	}

	// Approved and receipted (default-deny no longer masks the denial this
	// test is actually pinning): the drift below must be denied because
	// post-apply reads the live workspace and finds a scope change, not
	// because the lineage was never approved.
	if err := os.WriteFile(trackedPath, []byte("unrelated unstaged noise, never committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", "gate-selector-post-apply-lineage", "--gate", string(reviewtransaction.GatePostApply)}, &output)
	if err == nil {
		t.Fatalf("post-apply must see the live workspace drift and deny, got allow:\n%s", output.String())
	}
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("post-apply denial = %T %v, want ReviewGateDeniedError", err, err)
	}
	// Now that the fixture is genuinely approved and receipted, the denial
	// must specifically be the scope-changed relation this test is pinning
	// — not merely "not allow", which a default-deny state mismatch would
	// also produce for the wrong reason.
	if denied.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("post-apply drift must deny as scope-changed, got %#v", denied)
	}
	if strings.Contains(err.Error(), managedAssetProvenanceRefusal) {
		t.Fatalf("post-apply drift was masked by stale asset provenance: %v", err)
	}
}
