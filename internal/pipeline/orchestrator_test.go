package pipeline

import (
	"errors"
	"reflect"
	"testing"
)

func TestOrchestratorRunsPrepareThenApply(t *testing.T) {
	order := []string{}
	orchestrator := NewOrchestrator(DefaultRollbackPolicy())

	result := orchestrator.Execute(StagePlan{
		Prepare: []Step{
			newTestStep("prepare-1", &order),
		},
		Apply: []Step{
			newTestStep("apply-1", &order),
		},
	})

	if result.Err != nil {
		t.Fatalf("Execute() error = %v", result.Err)
	}

	if !reflect.DeepEqual(order, []string{"run:prepare-1", "run:apply-1"}) {
		t.Fatalf("execution order = %v", order)
	}

	if !result.Prepare.Success || !result.Apply.Success {
		t.Fatalf("stage result = prepare:%v apply:%v", result.Prepare.Success, result.Apply.Success)
	}
}

func TestOrchestratorRollsBackApplyStepsOnFailure(t *testing.T) {
	order := []string{}
	orchestrator := NewOrchestrator(DefaultRollbackPolicy())

	result := orchestrator.Execute(StagePlan{
		Apply: []Step{
			newRollbackStep("apply-1", &order, nil),
			newRollbackStep("apply-2", &order, errors.New("boom")),
		},
	})

	if result.Err == nil {
		t.Fatalf("Execute() expected apply error")
	}

	wantOrder := []string{"run:apply-1", "run:apply-2", "rollback:apply-1"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}

	if result.Rollback.Stage != StageRollback {
		t.Fatalf("rollback stage = %q", result.Rollback.Stage)
	}

	if !result.Rollback.Success {
		t.Fatalf("rollback expected success, got err = %v", result.Rollback.Err)
	}
}

func TestOrchestratorSkipsRollbackWhenPolicyDisabled(t *testing.T) {
	order := []string{}
	orchestrator := NewOrchestrator(RollbackPolicy{OnApplyFailure: false})

	result := orchestrator.Execute(StagePlan{
		Apply: []Step{
			newRollbackStep("apply-1", &order, errors.New("boom")),
		},
	})

	if result.Err == nil {
		t.Fatalf("Execute() expected apply error")
	}

	if len(result.Rollback.Steps) != 0 {
		t.Fatalf("rollback steps = %d, want 0", len(result.Rollback.Steps))
	}

	if !reflect.DeepEqual(order, []string{"run:apply-1"}) {
		t.Fatalf("execution order = %v", order)
	}
}

func TestRunnerContinueOnErrorExecutesAllSteps(t *testing.T) {
	order := []string{}
	runner := Runner{FailurePolicy: ContinueOnError}

	steps := []Step{
		newRollbackStep("step-1", &order, nil),
		newRollbackStep("step-2", &order, errors.New("fail-2")),
		newRollbackStep("step-3", &order, nil),
	}

	result := runner.Run(StageApply, steps)

	wantOrder := []string{"run:step-1", "run:step-2", "run:step-3"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}

	if result.Success {
		t.Fatalf("expected result.Success = false")
	}

	if result.Err == nil {
		t.Fatalf("expected aggregated error")
	}

	if len(result.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(result.Steps))
	}

	if result.Steps[0].Status != StepStatusSucceeded {
		t.Fatalf("step-1 status = %q", result.Steps[0].Status)
	}
	if result.Steps[1].Status != StepStatusFailed {
		t.Fatalf("step-2 status = %q", result.Steps[1].Status)
	}
	if result.Steps[2].Status != StepStatusSucceeded {
		t.Fatalf("step-3 status = %q", result.Steps[2].Status)
	}
}

func TestRunnerStopOnErrorHaltsExecution(t *testing.T) {
	order := []string{}
	runner := Runner{FailurePolicy: StopOnError}

	steps := []Step{
		newRollbackStep("step-1", &order, nil),
		newRollbackStep("step-2", &order, errors.New("fail")),
		newRollbackStep("step-3", &order, nil),
	}

	result := runner.Run(StageApply, steps)

	wantOrder := []string{"run:step-1", "run:step-2"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}

	if result.Success {
		t.Fatalf("expected failure")
	}

	if len(result.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(result.Steps))
	}
}

func TestRunnerProgressCallbackEmitsEvents(t *testing.T) {
	order := []string{}
	events := []ProgressEvent{}

	runner := Runner{
		FailurePolicy: StopOnError,
		OnProgress: func(e ProgressEvent) {
			events = append(events, e)
		},
	}

	steps := []Step{
		newTestStep("step-a", &order),
		newTestStep("step-b", &order),
	}

	result := runner.Run(StageApply, steps)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// 2 steps × 2 events each (running + succeeded) = 4 events
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}

	if events[0].Status != StepStatusRunning || events[0].StepID != "step-a" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Status != StepStatusSucceeded || events[1].StepID != "step-a" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	if events[2].Status != StepStatusRunning || events[2].StepID != "step-b" {
		t.Fatalf("event[2] = %+v", events[2])
	}
	if events[3].Status != StepStatusSucceeded || events[3].StepID != "step-b" {
		t.Fatalf("event[3] = %+v", events[3])
	}
}

func TestRunnerProgressCallbackEmitsFailedEvents(t *testing.T) {
	order := []string{}
	events := []ProgressEvent{}

	runner := Runner{
		FailurePolicy: ContinueOnError,
		OnProgress: func(e ProgressEvent) {
			events = append(events, e)
		},
	}

	steps := []Step{
		newRollbackStep("ok-step", &order, nil),
		newRollbackStep("bad-step", &order, errors.New("oops")),
	}

	result := runner.Run(StageApply, steps)

	if result.Success {
		t.Fatalf("expected failure")
	}

	// ok-step: running, succeeded; bad-step: running, failed = 4 events
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}

	if events[3].Status != StepStatusFailed || events[3].Err == nil {
		t.Fatalf("last event expected failed with error, got %+v", events[3])
	}
}

func TestOrchestratorContinueOnErrorWithRollback(t *testing.T) {
	order := []string{}
	orchestrator := NewOrchestrator(
		DefaultRollbackPolicy(),
		WithFailurePolicy(ContinueOnError),
	)

	result := orchestrator.Execute(StagePlan{
		Apply: []Step{
			newRollbackStep("apply-1", &order, nil),
			newRollbackStep("apply-2", &order, errors.New("boom")),
			newRollbackStep("apply-3", &order, nil),
		},
	})

	// All 3 steps should run due to ContinueOnError.
	wantRunOrder := []string{"run:apply-1", "run:apply-2", "run:apply-3"}
	runOrder := order[:3]
	if !reflect.DeepEqual(runOrder, wantRunOrder) {
		t.Fatalf("run order = %v, want %v", runOrder, wantRunOrder)
	}

	if result.Err == nil {
		t.Fatalf("expected error")
	}

	if result.Apply.Success {
		t.Fatalf("expected apply stage to report failure")
	}

	// Rollback should fire because apply failed and policy is enabled.
	if result.Rollback.Stage != StageRollback {
		t.Fatalf("rollback stage = %q, want rollback", result.Rollback.Stage)
	}
}

func TestExecuteRollbackAttemptsEveryStepAfterFailures(t *testing.T) {
	order := []string{}
	first := &testStep{id: "first", order: &order}
	failed := &testStep{id: "failed", order: &order, rollErr: errors.New("restore failed")}
	last := &testStep{id: "last", order: &order}
	result := ExecuteRollback([]StepResult{{StepID: "first", Status: StepStatusSucceeded}, {StepID: "failed", Status: StepStatusSucceeded}, {StepID: "last", Status: StepStatusSucceeded}}, map[string]Step{"first": first, "failed": failed, "last": last})
	if result.Success || len(result.Steps) != 3 || result.Steps[1].Status != StepStatusFailed {
		t.Fatalf("rollback = %#v, want all three results with failed middle step", result)
	}
	if !reflect.DeepEqual(order, []string{"rollback:last", "rollback:failed", "rollback:first"}) {
		t.Fatalf("order = %v", order)
	}
}

func TestOrchestratorWithProgressFunc(t *testing.T) {
	order := []string{}
	events := []ProgressEvent{}

	orchestrator := NewOrchestrator(
		RollbackPolicy{OnApplyFailure: false},
		WithProgressFunc(func(e ProgressEvent) {
			events = append(events, e)
		}),
	)

	result := orchestrator.Execute(StagePlan{
		Prepare: []Step{newTestStep("prep", &order)},
		Apply:   []Step{newTestStep("act", &order)},
	})

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// prep: running+succeeded, act: running+succeeded = 4
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4", len(events))
	}

	if events[0].Stage != StagePrepare || events[0].StepID != "prep" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[2].Stage != StageApply || events[2].StepID != "act" {
		t.Fatalf("event[2] = %+v", events[2])
	}
}

type testStep struct {
	id      string
	order   *[]string
	runErr  error
	rollErr error
}

func newTestStep(id string, order *[]string) *testStep {
	return &testStep{id: id, order: order}
}

func newRollbackStep(id string, order *[]string, runErr error) *testStep {
	return &testStep{id: id, order: order, runErr: runErr}
}

func (s *testStep) ID() string {
	return s.id
}

func (s *testStep) Run() error {
	*s.order = append(*s.order, "run:"+s.id)
	return s.runErr
}

func (s *testStep) Rollback() error {
	*s.order = append(*s.order, "rollback:"+s.id)
	return s.rollErr
}
