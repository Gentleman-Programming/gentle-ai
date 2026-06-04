package model

import "testing"

func TestPlanCompilability(t *testing.T) {
	// Verify that PlanStatus values are defined correctly
	statuses := []PlanStatus{
		PlanStatusPending,
		PlanStatusRunning,
		PlanStatusSucceeded,
		PlanStatusFailed,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("PlanStatus constant is empty")
		}
	}

	// Verify that RunResult values are defined correctly
	results := []RunResult{
		RunResultSkipped,
		RunResultSuccess,
		RunResultFailed,
	}

	for _, r := range results {
		if r == "" {
			t.Error("RunResult constant is empty")
		}
	}

	// Test PlanStep struct instantiation
	step := PlanStep{
		ID:     "step-1",
		Name:   "Init Step",
		Status: PlanStatusRunning,
		Result: RunResultSuccess,
		Error:  "",
	}

	if step.ID != "step-1" || step.Status != PlanStatusRunning || step.Result != RunResultSuccess {
		t.Errorf("PlanStep struct fields instantiated incorrectly: %+v", step)
	}

	// Test Plan struct instantiation
	plan := Plan{
		ID: "plan-123",
		Selection: Selection{
			Agents: []AgentID{AgentClaudeCode},
		},
		Status: PlanStatusPending,
		Steps:  []PlanStep{step},
	}

	if plan.ID != "plan-123" || len(plan.Steps) != 1 || plan.Selection.Agents[0] != AgentClaudeCode {
		t.Errorf("Plan struct fields instantiated incorrectly: %+v", plan)
	}
}
