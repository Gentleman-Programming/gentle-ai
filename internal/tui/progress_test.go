package tui

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
)

func TestProgressPercentTracksCompletedSteps(t *testing.T) {
	progress := NewProgressState([]string{"a", "b", "c", "d"})

	progress.Mark(0, string(pipeline.StepStatusSucceeded))
	progress.Mark(1, string(pipeline.StepStatusFailed))

	if got, want := progress.Percent(), 50; got != want {
		t.Fatalf("Percent() = %d, want %d", got, want)
	}
}

func TestProgressFromExecutionIncludesAllStages(t *testing.T) {
	result := pipeline.ExecutionResult{
		Prepare: pipeline.StageResult{Steps: []pipeline.StepResult{{StepID: "prepare", Status: pipeline.StepStatusSucceeded}}},
		Apply:   pipeline.StageResult{Steps: []pipeline.StepResult{{StepID: "apply", Status: pipeline.StepStatusSucceeded}}},
	}

	progress := ProgressFromExecution(result)
	if len(progress.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(progress.Items))
	}

	if got, want := progress.Percent(), 100; got != want {
		t.Fatalf("Percent() = %d, want %d", got, want)
	}
}

func TestProgressSkippedIsTerminalNotSuccess(t *testing.T) {
	progress := NewProgressState([]string{"logo"})
	progress.Mark(0, string(pipeline.StepStatusSkipped))
	if !progress.Done() || progress.HasFailures() || progress.Items[0].Status != "skipped" {
		t.Fatalf("skip progress: %#v", progress)
	}
}
