package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

type installerFailureStep struct {
	id    string
	err   error
	order *[]string
}

func (s installerFailureStep) ID() string { return s.id }
func (s installerFailureStep) Run() error {
	*s.order = append(*s.order, s.id)
	return s.err
}

type installerFailureRollbackStep struct {
	installerFailureStep
	rolledBack *[]string
}

func (s installerFailureRollbackStep) Rollback() error {
	*s.rolledBack = append(*s.rolledBack, s.id)
	return nil
}

// Reproduce #4509 without installing tools or modifying user configuration.
func TestInstallerFailureRollbackReturnsControl(t *testing.T) {
	labels := []string{
		"prepare:check-dependencies", "prepare:backup-snapshot",
		"apply:rollback-restore", "opencode:background-activation",
		"agent:claude-code", "agent:codex", "agent:hermes", "agent:opencode",
		"community-tool:codegraph", "opencode-plugin:sdd-engram-plugin",
		"opencode-plugin:sub-agent-statusline", "component:claude-theme",
		"component:context7", "component:persona", "component:engram",
		"component:gga", "component:opencode-gentle-logo", "component:permissions",
		"component:sdd", "component:skills", "agent-guidance:claude-code",
		"agent-guidance:codex", "agent-guidance:hermes", "agent-guidance:opencode",
		"component:compatibility-skills-refresh",
	}
	rollbackIDs := []string{
		"component:compatibility-skills-refresh",
		"opencode:background-activation",
		"apply:rollback-restore",
	}
	failure := errors.New("controlled CodeGraph probe failure")
	var order, rolledBack []string
	var plan pipeline.StagePlan
	for i, id := range labels {
		step := installerFailureStep{id: id, order: &order}
		if id == "community-tool:codegraph" {
			step.err = failure
		}
		if i < 2 {
			plan.Prepare = append(plan.Prepare, step)
			continue
		}
		switch id {
		case "apply:rollback-restore", "opencode:background-activation", "component:compatibility-skills-refresh":
			plan.Apply = append(plan.Apply, installerFailureRollbackStep{step, &rolledBack})
		default:
			plan.Apply = append(plan.Apply, step)
		}
	}

	const runID uint64 = 1
	m := installingModel(labels, runID)
	if !m.pipelineRunning || m.progressRun == nil || m.installRunID != runID {
		t.Fatal("fixture must represent an active install with a nonzero run ID")
	}
	// Keep an executor configured to disable the manual development fallback.
	m.ExecuteFn = func(model.Selection, planner.ResolvedPlan, system.DetectionResult,
		model.OpenCodeBackgroundIntent, model.OpenCodeBackgroundIntent,
		model.PiBackgroundIntent, model.PiBackgroundIntent, pipeline.ProgressFunc,
	) pipeline.ExecutionResult {
		panic("executor must not restart after completion")
	}
	orchestrator := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy(),
		pipeline.WithFailurePolicy(pipeline.ContinueOnError),
		pipeline.WithProgressFunc(func(event pipeline.ProgressEvent) {
			next, _ := m.Update(StepProgressMsg{RunID: runID, StepID: event.StepID, Status: event.Status, Err: event.Err})
			m = next.(Model)
		}),
	)
	result := orchestrator.Execute(plan)
	if !errors.Is(result.Err, failure) || result.Apply.Success {
		t.Fatalf("execution did not preserve controlled failure: %+v", result)
	}
	if !reflect.DeepEqual(order, labels) {
		t.Fatalf("execution order = %v, want all independent steps %v", order, labels)
	}
	if !result.Rollback.Success || !reflect.DeepEqual(rolledBack, rollbackIDs) {
		t.Fatalf("rollback = %+v, calls = %v, want successful rollback %v", result.Rollback, rolledBack, rollbackIDs)
	}
	if !m.pipelineRunning || m.progressRun == nil {
		t.Fatal("install became inactive before matching completion")
	}
	m.progressRun.complete(result)
	doneValue := m.nextProgressCommand()()
	doneMsg, ok := doneValue.(PipelineDoneMsg)
	if !ok || doneMsg.RunID != runID {
		t.Fatalf("progress command returned %#v, want PipelineDoneMsg for run %d", doneValue, runID)
	}
	next, completionCmd := m.Update(doneMsg)
	m = next.(Model)
	if m.pipelineRunning || m.progressRun != nil {
		t.Fatal("matching failed completion did not clear active install state")
	}
	if completionCmd != nil {
		t.Fatal("failed completion scheduled another command")
	}
	if !errors.Is(m.Execution.Err, failure) {
		t.Fatal("TUI discarded the execution error")
	}
	if !strings.Contains(strings.Join(m.Progress.Logs, "\n"), "pipeline completed with errors") {
		t.Fatalf("missing terminal error log: %v", m.Progress.Logs)
	}
	if len(m.Progress.Items) != len(labels)+len(rollbackIDs) {
		t.Fatalf("progress rows = %d, want %d", len(m.Progress.Items), len(labels)+len(rollbackIDs))
	}
	t.Logf("after completion: percent=%d, rollback rows=%+v", m.Progress.Percent(), m.Progress.Items[len(labels):])
	if m.Progress.Percent() != 100 || !m.Progress.Done() {
		t.Errorf("terminal progress = %d%%, want 100%% after completed rollback", m.Progress.Percent())
	}
	output := m.View()
	for _, id := range rollbackIDs {
		if !strings.Contains(output, "↶ "+id+" (rolled back)") || strings.Contains(output, "· "+id) {
			t.Errorf("completed rollback %q not rendered as reverted: %q", id, output)
		}
	}
	if !strings.Contains(output, "Completed with errors:") || !strings.Contains(output, "Press Enter to continue.") || strings.Contains(output, "completed successfully") {
		t.Errorf("terminal view missing error or continuation: %q", output)
	}
	next, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if enterCmd != nil || m.pipelineRunning || m.progressRun != nil || m.installRunID != runID {
		t.Fatal("Enter restarted or scheduled execution after failed completion")
	}
	if !reflect.DeepEqual(order, labels) || !reflect.DeepEqual(rolledBack, rollbackIDs) {
		t.Fatal("Enter changed execution or rollback calls")
	}
	if m.Screen != ScreenComplete {
		t.Errorf("screen after Enter = %v, want ScreenComplete", m.Screen)
	}
	if output := m.View(); !strings.Contains(output, "Installation completed with errors.") || !strings.Contains(output, failure.Error()) {
		t.Errorf("completion view missing failure: %q", output)
	}
}
