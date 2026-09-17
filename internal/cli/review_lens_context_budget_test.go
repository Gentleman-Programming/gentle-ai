package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func reviewLensContextProbeState(t *testing.T, repo string, runtime model.AgentID) (reviewtransaction.CompactState, string) {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(t.Context(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assessment, err := builder.AssessSnapshotRisk(t.Context(), snapshot)
	if err != nil {
		t.Fatalf("AssessSnapshotRisk() error = %v", err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("facadeSelectedLenses() error = %v", err)
	}
	request, err := prepareReviewFacadeCompactAtomicStart(t.Context(), repo, "lens-context-runtime-budget", "", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	}, snapshot, assessment, assessment.ChangedLines, lenses, runtime)
	if err != nil {
		t.Fatalf("prepareReviewFacadeCompactAtomicStart() error = %v", err)
	}
	state := request.State
	state.InitialAtomicStart = &request.Binding
	revision, err := reviewtransaction.CompactRevisionForState(state)
	if err != nil {
		t.Fatalf("CompactRevisionForState() error = %v", err)
	}
	return state, revision
}

func TestReviewLensContextBudgetProbeUsesFrozenRuntimeBound(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "internal/large-source.txt", strings.Repeat("ordinary source line for immutable reviewer context\n", 14_000), 0o644)

	for _, test := range []struct {
		name    string
		runtime model.AgentID
		want    reviewLensContextProbeOutcome
	}{
		{name: "claude runtime refuses candidate above 512 KiB", runtime: model.AgentClaudeCode, want: reviewLensContextOverBudget},
		{name: "empty historical runtime keeps 4 MiB budget", runtime: "", want: reviewLensContextRepresentable},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, revision := reviewLensContextProbeState(t, repo, test.runtime)
			outcome, err := reviewLensContextBudgetProbe(t.Context(), reviewLensContextDependencies(), repo, state, revision)
			if err != nil {
				t.Fatalf("reviewLensContextBudgetProbe() error = %v", err)
			}
			if outcome != test.want {
				t.Fatalf("reviewLensContextBudgetProbe() = %v, want %v for runtime %q", outcome, test.want, test.runtime)
			}
		})
	}
}
