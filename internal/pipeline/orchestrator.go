package pipeline

import "errors"

// OrchestratorOption configures the orchestrator.
type OrchestratorOption func(*Orchestrator)

// WithFailurePolicy sets the failure policy for the apply stage runner.
func WithFailurePolicy(policy FailurePolicy) OrchestratorOption {
	return func(o *Orchestrator) {
		o.runner.FailurePolicy = policy
	}
}

// WithProgressFunc sets a callback that receives progress events during execution.
func WithProgressFunc(fn ProgressFunc) OrchestratorOption {
	return func(o *Orchestrator) {
		o.runner.OnProgress = fn
	}
}

// WithTerminalCleanup releases transaction-only resources once the caller has
// finished post-apply work. Cleanup also runs after any pipeline rollback.
func WithTerminalCleanup(cleanup func()) OrchestratorOption {
	return func(o *Orchestrator) { o.terminalCleanup = cleanup }
}

type Orchestrator struct {
	runner          Runner
	policy          RollbackPolicy
	stepByID        map[string]Step
	terminalCleanup func()
}

// Finish settles a successful execution after downstream verification and
// persistence. It is safe to call again after a rollback.
func (o *Orchestrator) Finish() {
	if o != nil && o.terminalCleanup != nil {
		cleanup := o.terminalCleanup
		o.terminalCleanup = nil
		cleanup()
	}
}

func NewOrchestrator(policy RollbackPolicy, opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		runner:   Runner{},
		policy:   policy,
		stepByID: map[string]Step{},
	}

	for _, opt := range opts {
		opt(o)
	}

	return o
}

func (o *Orchestrator) Execute(plan StagePlan) ExecutionResult {
	o.indexSteps(plan.Prepare)
	o.indexSteps(plan.Apply)

	prepareResult := o.runner.Run(StagePrepare, plan.Prepare)
	if !prepareResult.Success {
		o.Finish()
		return ExecutionResult{Prepare: prepareResult, Err: prepareResult.Err}
	}

	applyResult := o.runner.Run(StageApply, plan.Apply)
	result := ExecutionResult{Prepare: prepareResult, Apply: applyResult}
	if applyResult.Success {
		return result
	}

	result.Err = applyResult.Err
	if o.policy.ShouldRollback(StageApply, applyResult.Err) {
		result.Rollback = ExecuteRollback(applyResult.Steps, o.stepByID)
		if !result.Rollback.Success {
			result.Err = errors.Join(result.Err, result.Rollback.Err)
		}
	}

	o.Finish()
	return result
}

// Rollback compensates successful apply steps after a downstream consumer,
// such as state persistence, fails. It must not be called after apply failure,
// because Execute already performs that rollback.
func (o *Orchestrator) Rollback(result ExecutionResult) StageResult {
	defer o.Finish()
	return ExecuteRollback(result.Apply.Steps, o.stepByID)
}

func (o *Orchestrator) indexSteps(steps []Step) {
	for _, step := range steps {
		o.stepByID[step.ID()] = step
	}
}
