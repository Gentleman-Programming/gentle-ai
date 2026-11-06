package pipeline

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

type fixtureSkip string

func (s fixtureSkip) Error() string      { return string(s) }
func (s fixtureSkip) SkipReason() string { return string(s) }

func TestRunnerSkipContinuesAndDoesNotRollback(t *testing.T) {
	order := []string{}
	events := []ProgressEvent{}
	skip := newRollbackStep("skip", &order, fmt.Errorf("wrapped: %w", fixtureSkip("unsupported")))
	fail := newRollbackStep("fail", &order, errors.New("ordinary failure"))
	o := NewOrchestrator(DefaultRollbackPolicy(), WithProgressFunc(func(e ProgressEvent) { events = append(events, e) }))
	result := o.Execute(StagePlan{Apply: []Step{skip, fail, newTestStep("later", &order)}})
	if result.Err == nil || result.Apply.Steps[0].Status != StepStatusSkipped || result.Apply.Steps[1].Status != StepStatusFailed {
		t.Fatalf("result: %#v", result)
	}
	if !reflect.DeepEqual(order, []string{"run:skip", "run:fail"}) {
		t.Fatalf("order: %v", order)
	}
	if events[1].Status != StepStatusSkipped || events[1].Err == nil {
		t.Fatalf("skip event: %#v", events)
	}
}
