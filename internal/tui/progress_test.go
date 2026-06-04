package tui

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
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
		Rollback: pipeline.StageResult{Steps: []pipeline.StepResult{{StepID: "rollback", Status: pipeline.StepStatusPending}}},
	}

	progress := ProgressFromExecution(result)
	if len(progress.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(progress.Items))
	}

	if got, want := progress.Percent(), 66; got != want {
		t.Fatalf("Percent() = %d, want %d", got, want)
	}
}

func TestProgressState_Start_Boundaries(t *testing.T) {
	progress := NewProgressState([]string{"a", "b"})

	// Negative index
	progress.Start(-1)
	if progress.Current != -1 {
		t.Errorf("expected Current to remain -1, got %d", progress.Current)
	}

	// Out of bounds index
	progress.Start(2)
	if progress.Current != -1 {
		t.Errorf("expected Current to remain -1, got %d", progress.Current)
	}

	// Valid index
	progress.Start(0)
	if progress.Current != 0 {
		t.Errorf("expected Current to be 0, got %d", progress.Current)
	}
	if progress.Items[0].Status != ProgressStatusRunning {
		t.Errorf("expected item 0 status to be running, got %q", progress.Items[0].Status)
	}
}

func TestProgressState_Mark_Boundaries(t *testing.T) {
	progress := NewProgressState([]string{"a", "b"})

	// Negative index
	progress.Mark(-1, string(pipeline.StepStatusSucceeded))
	if progress.Current != -1 {
		t.Errorf("expected Current to remain -1, got %d", progress.Current)
	}

	// Out of bounds index
	progress.Mark(2, string(pipeline.StepStatusSucceeded))
	if progress.Current != -1 {
		t.Errorf("expected Current to remain -1, got %d", progress.Current)
	}

	// Valid index 0 -> advances current to 1
	progress.Mark(0, string(pipeline.StepStatusSucceeded))
	if progress.Current != 1 {
		t.Errorf("expected Current to advance to 1, got %d", progress.Current)
	}
	if progress.Items[0].Status != string(pipeline.StepStatusSucceeded) {
		t.Errorf("expected item 0 status to be succeeded, got %q", progress.Items[0].Status)
	}

	// Valid index 1 (last index) -> advances current to len(Items) which is 2
	progress.Mark(1, string(pipeline.StepStatusFailed))
	if progress.Current != 2 {
		t.Errorf("expected Current to advance to 2, got %d", progress.Current)
	}
}

func TestProgressState_AppendLog(t *testing.T) {
	progress := NewProgressState([]string{"a"})
	progress.AppendLog("hello %s %d", "world", 42)

	if len(progress.Logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(progress.Logs))
	}
	expectedLog := "hello world 42"
	if progress.Logs[0] != expectedLog {
		t.Errorf("expected log to be %q, got %q", expectedLog, progress.Logs[0])
	}
}

func TestProgressState_DoneAndPercentEmpty(t *testing.T) {
	emptyProgress := NewProgressState(nil)
	if !emptyProgress.Done() {
		t.Error("empty progress should be Done()")
	}
	if pct := emptyProgress.Percent(); pct != 100 {
		t.Errorf("empty progress Percent() = %d, want 100", pct)
	}
}

func TestProgressState_HasFailures(t *testing.T) {
	progress := NewProgressState([]string{"a", "b"})

	if progress.HasFailures() {
		t.Error("initially progress should not have failures")
	}

	progress.Mark(0, string(pipeline.StepStatusSucceeded))
	if progress.HasFailures() {
		t.Error("after success, progress should not have failures")
	}

	progress.Mark(1, string(pipeline.StepStatusFailed))
	if !progress.HasFailures() {
		t.Error("after failure, progress should have failures")
	}
}

func TestProgressState_ViewModel(t *testing.T) {
	progress := NewProgressState([]string{"step-a", "step-b"})
	progress.AppendLog("Log line 1")

	// Current is initially -1
	vm1 := progress.ViewModel()
	if vm1.CurrentStep != "" {
		t.Errorf("expected empty CurrentStep, got %q", vm1.CurrentStep)
	}
	if vm1.Done {
		t.Error("expected Done to be false")
	}
	if vm1.Failed {
		t.Error("expected Failed to be false")
	}

	// Current is set to index 0
	progress.Start(0)
	vm2 := progress.ViewModel()
	if vm2.CurrentStep != "step-a" {
		t.Errorf("expected CurrentStep step-a, got %q", vm2.CurrentStep)
	}

	// Out of bounds current (after completing everything)
	progress.Mark(0, string(pipeline.StepStatusSucceeded))
	progress.Mark(1, string(pipeline.StepStatusSucceeded))
	vm3 := progress.ViewModel()
	if vm3.CurrentStep != "" {
		t.Errorf("expected empty CurrentStep on completed progress, got %q", vm3.CurrentStep)
	}
	if !vm3.Done {
		t.Error("expected Done to be true")
	}
	if vm3.Percent != 100 {
		t.Errorf("expected percent 100, got %d", vm3.Percent)
	}

	// Verify logs array copy safety
	vm3.Logs[0] = "Mutated"
	if progress.Logs[0] != "Log line 1" {
		t.Error("ViewModel Logs should be a copy, not share the underlying array slice")
	}
}
