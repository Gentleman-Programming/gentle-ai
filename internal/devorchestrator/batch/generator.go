package batch

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// ExecutionBatch represents a single unit of execution (e.g. for one repository)
// that the Orchestrator will dispatch to an agent.
type ExecutionBatch struct {
	RepoName  string
	AgentName string
	Ready     bool
}

// GenerateExecutionBatches takes the resolved SDD status and splits the pending
// apply tasks into individual batches per repository.
func GenerateExecutionBatches(status sddstatus.StatusV1Projection, defaultAgent string) []ExecutionBatch {
	// If RepoProgress is nil, this change involves zero or exactly one repository,
	// so it uses the traditional flat apply-progress state. We yield a single batch.
	if status.RepoProgress == nil {
		ready := false
		if status.NextRecommended == "apply" && status.ApplyState != sddstatus.ApplyAllDone {
			ready = true
		}
		return []ExecutionBatch{
			{
				RepoName:  "", // Implies the workspace or implicit single repo
				AgentName: defaultAgent,
				Ready:     ready,
			},
		}
	}

	// For multi-repo changes, we generate a batch for each repository.
	var batches []ExecutionBatch
	for _, repo := range status.RepoProgress.Repos {
		ready := false
		// A repo batch is only ready if the global next recommended phase is "apply"
		// and this specific repo is not done yet.
		if status.NextRecommended == "apply" && repo.ApplyProgress != sddstatus.ArtifactDone {
			ready = true
		}
		batches = append(batches, ExecutionBatch{
			RepoName:  repo.Slug,
			AgentName: defaultAgent,
			Ready:     ready,
		})
	}

	return batches
}
