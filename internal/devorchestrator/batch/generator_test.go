package batch

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestGenerateExecutionBatches_NilRepoProgress(t *testing.T) {
	status := sddstatus.StatusV1Projection{
		NextRecommended: "apply",
		ApplyState:      sddstatus.ApplyReady,
		RepoProgress:    nil,
	}

	batches := GenerateExecutionBatches(status, "backend-implementer")

	expected := []ExecutionBatch{
		{
			RepoName:  "",
			AgentName: "backend-implementer",
			Ready:     true,
		},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("expected %+v, got %+v", expected, batches)
	}
}

func TestGenerateExecutionBatches_NilRepoProgressNotReady(t *testing.T) {
	status := sddstatus.StatusV1Projection{
		NextRecommended: "verify",
		ApplyState:      sddstatus.ApplyAllDone,
		RepoProgress:    nil,
	}

	batches := GenerateExecutionBatches(status, "backend-implementer")

	expected := []ExecutionBatch{
		{
			RepoName:  "",
			AgentName: "backend-implementer",
			Ready:     false,
		},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("expected %+v, got %+v", expected, batches)
	}
}

func TestGenerateExecutionBatches_MultiRepo(t *testing.T) {
	status := sddstatus.StatusV1Projection{
		NextRecommended: "apply",
		RepoProgress: &sddstatus.RepoProgress{
			Repos: []sddstatus.RepoProgressEntry{
				{Slug: "frontend-web", ApplyProgress: sddstatus.ArtifactDone},
				{Slug: "backend-api", ApplyProgress: sddstatus.ArtifactPartial},
				{Slug: "db-schema", ApplyProgress: sddstatus.ArtifactMissing},
			},
			AllComplete: false,
		},
	}

	batches := GenerateExecutionBatches(status, "backend-implementer")

	expected := []ExecutionBatch{
		{
			RepoName:  "frontend-web",
			AgentName: "backend-implementer",
			Ready:     false, // Already done
		},
		{
			RepoName:  "backend-api",
			AgentName: "backend-implementer",
			Ready:     true, // Needs apply
		},
		{
			RepoName:  "db-schema",
			AgentName: "backend-implementer",
			Ready:     true, // Needs apply
		},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("expected %+v, got %+v", expected, batches)
	}
}

func TestGenerateExecutionBatches_MultiRepoNotApplyPhase(t *testing.T) {
	status := sddstatus.StatusV1Projection{
		NextRecommended: "verify",
		RepoProgress: &sddstatus.RepoProgress{
			Repos: []sddstatus.RepoProgressEntry{
				{Slug: "frontend-web", ApplyProgress: sddstatus.ArtifactDone},
				{Slug: "backend-api", ApplyProgress: sddstatus.ArtifactDone},
			},
			AllComplete: true,
		},
	}

	batches := GenerateExecutionBatches(status, "backend-implementer")

	expected := []ExecutionBatch{
		{
			RepoName:  "frontend-web",
			AgentName: "backend-implementer",
			Ready:     false,
		},
		{
			RepoName:  "backend-api",
			AgentName: "backend-implementer",
			Ready:     false,
		},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("expected %+v, got %+v", expected, batches)
	}
}
