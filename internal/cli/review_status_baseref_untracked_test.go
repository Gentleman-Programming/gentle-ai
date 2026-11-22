package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// Issue #6156: a base-ref STATUS (`--base-ref <ref> --committed-only`) over a
// consumed (acknowledged) target with an unrelated untracked file present and
// no untracked declaration must answer with the typed
// intended_untracked_selection_required collect transition the selectorless
// branch already emits, not fail pre_native with
// "unrelated target status is inconsistent".
func TestBaseRefStatusUntrackedDeclarationRequiredOverAcknowledgedTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START: %#v", offered)
	}
	runTransitionTokens(t, "start", offered.NextTransition.Execute.Arguments)
	pending := derivedRangeTerminalStatus(t, repo)
	if pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("no pending acknowledgement: %#v", pending.NextTransition)
	}
	runTransitionTokens(t, "acknowledge-approved", pending.NextTransition.Execute.Arguments)
	acknowledged := derivedRangeTerminalStatus(t, repo)
	if acknowledged.NextTransition == nil || acknowledged.NextTransition.ReasonCode != "target_already_acknowledged" {
		t.Fatalf("no acknowledged stop: %#v", acknowledged.NextTransition)
	}

	// The #6156 shape: one unrelated untracked file, no declaration.
	if err := os.WriteFile(filepath.Join(repo, "unrelated-helper.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The selectorless STATUS over the same repository state must keep
	// answering with the typed collect transition (issue table, row 1).
	selectorless := derivedRangeTerminalStatus(t, repo)
	if selectorless.NextTransition == nil ||
		selectorless.NextTransition.Kind != reviewNextTransitionCollect ||
		selectorless.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("selectorless STATUS must emit the typed untracked-declaration collect, got: %#v", selectorless.NextTransition)
	}
	digest := selectorless.EligibleUntrackedInventory
	if digest == "" {
		t.Fatal("selectorless STATUS did not publish the canonical untracked inventory digest")
	}

	// The exact invocation shape Gentle Shell's pre-delivery preflight issues.
	var output bytes.Buffer
	args := []string{
		"status",
		"--cwd", repo,
		"--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode),
		"--next-transition",
		"--base-ref", acknowledged.Projection.BaseTree,
		"--committed-only=true",
	}
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("base-ref STATUS with undeclared untracked failed: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), output.Bytes())
	if status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("base-ref STATUS must emit the typed untracked-declaration collect, got: %#v\n%s", status.NextTransition, output.String())
	}
	if status.EligibleUntrackedInventory != digest {
		t.Fatalf("base-ref STATUS inventory digest = %q, want the selectorless digest %q", status.EligibleUntrackedInventory, digest)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Submission == nil {
		t.Fatal("base-ref selection collect lacks its selected-STATUS submission")
	}
	wantSelector := []string{"--base-ref=" + status.Projection.BaseTree, "--committed-only=true"}
	if got := input.Submission.ArgumentTokens[3 : len(input.Submission.ArgumentTokens)-1]; !reflect.DeepEqual(got, wantSelector) {
		t.Fatalf("submission selector = %v, want %v", got, wantSelector)
	}

	// Issue table, row 5: the declared exclusion over the same target must
	// still reach the acknowledged terminal stop, not an inconsistent status.
	var declaredOutput bytes.Buffer
	declaredArgs := append(append([]string{}, args...),
		"--untracked-scope=exclude", "--expected-untracked-inventory="+digest)
	if err := RunReview(declaredArgs, &declaredOutput); err != nil {
		t.Fatalf("base-ref STATUS with declared exclusion failed: %v\n%s", err, declaredOutput.String())
	}
	var declared ReviewTargetStatusResult
	decodeStrictReviewJSON(t, declaredOutput.Bytes(), &declared)
	if declared.NextTransition == nil || declared.NextTransition.Kind != reviewNextTransitionStop ||
		declared.NextTransition.ReasonCode != "target_already_acknowledged" {
		t.Fatalf("declared base-ref STATUS = %#v, want the target_already_acknowledged stop", declared.NextTransition)
	}
}
