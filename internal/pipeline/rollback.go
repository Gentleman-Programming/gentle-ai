package pipeline

import (
	"errors"
	"fmt"
)

type RollbackPolicy struct {
	OnApplyFailure bool
}

func DefaultRollbackPolicy() RollbackPolicy {
	return RollbackPolicy{OnApplyFailure: true}
}

func (p RollbackPolicy) ShouldRollback(stage Stage, err error) bool {
	if err == nil {
		return false
	}

	return stage == StageApply && p.OnApplyFailure
}

func ExecuteRollback(steps []StepResult, stepIndex map[string]Step) StageResult {
	result := StageResult{Stage: StageRollback, Success: true}
	var rollbackErrors []error

	for i := len(steps) - 1; i >= 0; i-- {
		stepResult := steps[i]
		if stepResult.Status != StepStatusSucceeded {
			continue
		}

		step, ok := stepIndex[stepResult.StepID]
		if !ok {
			continue
		}

		rollbackStep, ok := step.(RollbackStep)
		if !ok {
			continue
		}

		err := rollbackStep.Rollback()
		item := StepResult{StepID: rollbackStep.ID(), Status: StepStatusRolledBack}
		if err != nil {
			item.Status = StepStatusFailed
			item.Err = err
			result.Steps = append(result.Steps, item)
			result.Success = false
			rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback step %q: %w", rollbackStep.ID(), err))
			continue
		}

		result.Steps = append(result.Steps, item)
	}
	if len(rollbackErrors) > 0 {
		result.Err = errors.Join(rollbackErrors...)
	}

	return result
}
