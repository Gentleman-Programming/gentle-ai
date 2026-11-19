package cli

import (
	"bytes"
	"testing"
)

// Issue #6156 companion coverage: the selectorless untracked-declaration
// collect must validate against the published status-v10 schema both with a
// runtime agent (workspace submission branch) and without one. The agentless
// envelope carries no submission, which is why the v10 input keeps
// `submission` optional: a current-changes STATUS with no --agent has no
// selected-STATUS submission to re-enter.
func TestSelectorlessUntrackedCollectValidatesAgainstPublishedSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/chosen.md", "# Chosen\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "docs/other.md", "# Other\n", 0o644)
	schema := compileWholeNativeStatusSchema(t, "status-v10.schema.json")

	var withAgent bytes.Buffer
	if err := RunReviewStatus([]string{"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo, "--agent", "pi"}, &withAgent); err != nil {
		t.Fatalf("agent STATUS: %v\n%s", err, withAgent.String())
	}
	validatePublishedReviewSchema(t, schema, withAgent.Bytes())

	var noAgent bytes.Buffer
	if err := RunReviewStatus([]string{"--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo}, &noAgent); err != nil {
		t.Fatalf("agentless STATUS: %v\n%s", err, noAgent.String())
	}
	validatePublishedReviewSchema(t, schema, noAgent.Bytes())
	if bytes.Contains(noAgent.Bytes(), []byte(`"submission"`)) {
		t.Fatal("agentless envelope unexpectedly carries a submission")
	}
}
