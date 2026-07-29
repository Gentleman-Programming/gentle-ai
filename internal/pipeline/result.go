package pipeline

import "time"

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusRunning    StepStatus = "running"
	StepStatusSucceeded  StepStatus = "succeeded"
	StepStatusFailed     StepStatus = "failed"
	StepStatusRolledBack StepStatus = "rolled-back"
	StepStatusSkipped    StepStatus = "skipped"
)

type StepResult struct {
	StepID     string
	Status     StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Err        error
}

type StageResult struct {
	Stage   Stage
	Steps   []StepResult
	Success bool
	Err     error
}

type ExecutionResult struct {
	Prepare  StageResult
	Apply    StageResult
	Rollback StageResult
	Err      error
	// ManualActions are non-fatal but actionable outcomes propagated by runtime
	// steps to both CLI and TUI completion renderers.
	ManualActions []string
}
