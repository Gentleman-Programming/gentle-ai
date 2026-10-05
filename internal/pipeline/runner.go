package pipeline

import (
	"errors"
	"time"
)

// Runner executes a list of steps for a given stage.
type Runner struct {
	FailurePolicy FailurePolicy
	OnProgress    ProgressFunc
}

func (r Runner) Run(stage Stage, steps []Step) StageResult {
	result := StageResult{Stage: stage, Success: true, Steps: make([]StepResult, 0, len(steps))}
	var errs []error

	for _, step := range steps {
		r.emitProgress(ProgressEvent{StepID: step.ID(), Stage: stage, Status: StepStatusRunning})

		started := time.Now().UTC()
		err := step.Run()
		finished := time.Now().UTC()

		stepResult := StepResult{
			StepID:     step.ID(),
			StartedAt:  started,
			FinishedAt: finished,
		}

		// Only explicit, nonempty skip outcomes are nonfatal. Wrapped ordinary
		// errors retain the existing failure and rollback policy.
		var skipped interface{ SkipReason() string }
		if errors.As(err, &skipped) && skipped.SkipReason() != "" {
			stepResult.Status = StepStatusSkipped
			stepResult.Err = err
			result.Steps = append(result.Steps, stepResult)
			r.emitProgress(ProgressEvent{StepID: step.ID(), Stage: stage, Status: StepStatusSkipped, Err: err})
			continue
		}

		if err != nil {
			stepResult.Status = StepStatusFailed
			stepResult.Err = err
			result.Steps = append(result.Steps, stepResult)

			r.emitProgress(ProgressEvent{StepID: step.ID(), Stage: stage, Status: StepStatusFailed, Err: err})

			errs = append(errs, err)
			result.Success = false

			if r.FailurePolicy == StopOnError {
				result.Err = err
				return result
			}

			continue
		}

		stepResult.Status = StepStatusSucceeded
		result.Steps = append(result.Steps, stepResult)
		r.emitProgress(ProgressEvent{StepID: step.ID(), Stage: stage, Status: StepStatusSucceeded})
	}

	if len(errs) > 0 {
		result.Err = errors.Join(errs...)
	}

	return result
}

func (r Runner) emitProgress(event ProgressEvent) {
	if r.OnProgress != nil {
		r.OnProgress(event)
	}
}
