package devorchestrator

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// TestPrepareBatches_ConsumesProjectionReadOnly covers H-03: PrepareBatches
// (the wiring internal/cli/dev_orchestrator.go's status and dispatch
// operations actually call) must consume a real sddstatus.StatusV1Projection
// end-to-end -- not just batch.GenerateExecutionBatches in isolation -- and
// must never write back into it. `grep -rn "PrepareBatches"
// --include="*_test.go" internal/` returned zero hits before this test, even
// though the underlying batch.GenerateExecutionBatches seam was already
// covered by internal/devorchestrator/batch/generator_test.go: nothing had
// ever exercised the Orchestrator.PrepareBatches wrapper the CLI actually
// calls.
func TestPrepareBatches_ConsumesProjectionReadOnly(t *testing.T) {
	orch := New(t.TempDir())

	projection := sddstatus.StatusV1Projection{
		NextRecommended: "apply",
		ApplyState:      sddstatus.ApplyReady,
		RepoProgress: &sddstatus.RepoProgress{
			Repos: []sddstatus.RepoProgressEntry{
				{Slug: "frontend-web", ApplyProgress: sddstatus.ArtifactDone},
				{Slug: "backend-api", ApplyProgress: sddstatus.ArtifactPartial},
			},
			AllComplete: false,
		},
	}

	// Independent snapshot of the projection's observable fields, taken
	// BEFORE the call, so the post-call assertion below compares against a
	// value that cannot have been mutated by the same call it is guarding --
	// comparing the live projection to itself after mutation would trivially
	// pass no matter what PrepareBatches did to it.
	repoSlugsBefore := make([]string, len(projection.RepoProgress.Repos))
	repoStatesBefore := make([]sddstatus.ArtifactState, len(projection.RepoProgress.Repos))
	for i, r := range projection.RepoProgress.Repos {
		repoSlugsBefore[i] = r.Slug
		repoStatesBefore[i] = r.ApplyProgress
	}
	nextRecommendedBefore := projection.NextRecommended
	applyStateBefore := projection.ApplyState

	batches := orch.PrepareBatches(projection, "backend-implementer")

	// The resulting batches must reflect the projection's actual repo/apply
	// state: frontend-web is already ArtifactDone (not ready), backend-api
	// still needs apply (ready), both under NextRecommended == "apply".
	expected := []batch.ExecutionBatch{
		{RepoName: "frontend-web", AgentName: "backend-implementer", Ready: false},
		{RepoName: "backend-api", AgentName: "backend-implementer", Ready: true},
	}
	if !reflect.DeepEqual(batches, expected) {
		t.Fatalf("PrepareBatches() = %+v, want %+v", batches, expected)
	}

	// No sddstatus field was mutated by the call.
	if projection.NextRecommended != nextRecommendedBefore {
		t.Fatalf("PrepareBatches mutated NextRecommended: got %q, want %q", projection.NextRecommended, nextRecommendedBefore)
	}
	if projection.ApplyState != applyStateBefore {
		t.Fatalf("PrepareBatches mutated ApplyState: got %v, want %v", projection.ApplyState, applyStateBefore)
	}
	for i, r := range projection.RepoProgress.Repos {
		if r.Slug != repoSlugsBefore[i] || r.ApplyProgress != repoStatesBefore[i] {
			t.Fatalf("PrepareBatches mutated RepoProgress.Repos[%d]: got %+v, want slug=%q state=%v", i, r, repoSlugsBefore[i], repoStatesBefore[i])
		}
	}
}

// TestPrepareBatches_NilRepoProgressReadOnly covers the single-repo
// (RepoProgress == nil) branch of the same seam, so PrepareBatches is proven
// end-to-end for both shapes GenerateExecutionBatches branches on.
func TestPrepareBatches_NilRepoProgressReadOnly(t *testing.T) {
	orch := New(t.TempDir())

	projection := sddstatus.StatusV1Projection{
		NextRecommended: "apply",
		ApplyState:      sddstatus.ApplyReady,
		RepoProgress:    nil,
	}

	batches := orch.PrepareBatches(projection, "backend-implementer")

	expected := []batch.ExecutionBatch{
		{RepoName: "", AgentName: "backend-implementer", Ready: true},
	}
	if !reflect.DeepEqual(batches, expected) {
		t.Fatalf("PrepareBatches() = %+v, want %+v", batches, expected)
	}
	if projection.RepoProgress != nil {
		t.Fatalf("PrepareBatches mutated RepoProgress from nil to %+v", projection.RepoProgress)
	}
}
