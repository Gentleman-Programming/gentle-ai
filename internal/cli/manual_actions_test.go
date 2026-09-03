package cli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// TestRenderInstallManualActions pins the CLI renderer. It previously read only
// the Pi CodeGraph result, so an action any other apply step raised through the
// pipeline was collected, propagated, and then silently dropped before reaching
// the user. Pi CodeGraph actions are copied into ExecutionResult.ManualActions
// during propagation, so the two sources overlap and must not print twice.
func TestRenderInstallManualActions(t *testing.T) {
	piAction := "Pi CodeGraph runtime verification is pending."
	stepAction := "Something optional was skipped; here is how to finish it."

	tests := []struct {
		name   string
		result InstallResult
		action string
		want   int
	}{
		{
			name:   "renders pipeline actions the renderer used to drop",
			result: InstallResult{Execution: pipeline.ExecutionResult{ManualActions: []string{stepAction}}},
			action: stepAction,
			want:   1,
		},
		{
			name: "deduplicates an action present in both sources",
			result: InstallResult{
				PiCodeGraph: &communitytool.PiCodeGraphResult{ManualActions: []string{piAction}},
				Execution:   pipeline.ExecutionResult{ManualActions: []string{piAction}},
			},
			action: piAction,
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderInstallManualActions(tt.result)
			if got := strings.Count(out, tt.action); got != tt.want {
				t.Fatalf("action rendered %d times, want %d: %q", got, tt.want, out)
			}
		})
	}
}

// TestRenderInstallManualActionsOrdersPiCodeGraphFirst pins the order the two
// sources are merged in. Pi CodeGraph's actions were the only ones this
// renderer emitted before, so they keep the lead and an existing install's
// output stays byte-identical; the actions apply steps raise are appended
// after. The sibling table above asserts occurrence counts, which a reordered
// append leaves untouched, so without this the order is unpinned.
func TestRenderInstallManualActionsOrdersPiCodeGraphFirst(t *testing.T) {
	piAction := "Pi CodeGraph runtime verification is pending."
	stepAction := "Something optional was skipped; here is how to finish it."

	out := RenderInstallManualActions(InstallResult{
		PiCodeGraph: &communitytool.PiCodeGraphResult{ManualActions: []string{piAction}},
		Execution:   pipeline.ExecutionResult{ManualActions: []string{stepAction}},
	})

	pi := strings.Index(out, piAction)
	step := strings.Index(out, stepAction)
	if pi < 0 || step < 0 {
		t.Fatalf("both actions must render, got %q", out)
	}
	if pi > step {
		t.Fatalf("Pi CodeGraph action must render before pipeline actions, got %q", out)
	}
}

// TestRenderInstallManualActionsEmptyRendersNothing keeps the renderer silent
// when there is nothing to say, so an ordinary run gains no trailing section.
func TestRenderInstallManualActionsEmptyRendersNothing(t *testing.T) {
	if out := RenderInstallManualActions(InstallResult{}); out != "" {
		t.Fatalf("RenderInstallManualActions() = %q, want empty", out)
	}
}

// manualActionProbeStep is a stand-in apply step that raises one manual action.
// It keeps the propagation from runtimeState into the execution result pinned
// on its own, independently of componentApplyStep, so a change to the GGA
// producer cannot quietly take the propagation down with it.
type manualActionProbeStep struct {
	state  *runtimeState
	action string
}

func (s manualActionProbeStep) ID() string { return "test:manual-action-probe" }

func (s manualActionProbeStep) Run() error {
	s.state.manualActions = append(s.state.manualActions, s.action)
	return nil
}

// TestExecuteTUIInstallPropagatesManualActions pins the TUI half of the
// propagation. The TUI completion screen reads ExecutionResult.ManualActions,
// so an action a step collects in runtimeState reaches a user only if
// executeTUIInstallWithBackground copies it across. Without this test the copy
// could be deleted and every other test in the package would stay green.
func TestExecuteTUIInstallPropagatesManualActions(t *testing.T) {
	home := t.TempDir()
	action := "Something optional was skipped; here is how to finish it."

	// The probe is the only thing under test here, so the pipeline must not
	// reach the machine: without these seams the run would consult the host's
	// PATH and could execute a real installer.
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreHome := osUserHomeDir
	t.Cleanup(func() {
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		osUserHomeDir = restoreHome
	})
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(string) (string, error) { return "", errors.New("not found") }
	osUserHomeDir = func() (string, error) { return home, nil }

	previous := tuiInstallStagePlan
	t.Cleanup(func() { tuiInstallStagePlan = previous })
	tuiInstallStagePlan = func(runtime *installRuntime) pipeline.StagePlan {
		plan := previous(runtime)
		plan.Apply = append(plan.Apply, manualActionProbeStep{state: runtime.state, action: action})
		return plan
	}

	selection := model.Selection{Components: []model.ComponentID{model.ComponentSkills}}
	resolved := planner.ResolvedPlan{OrderedComponents: selection.Components}
	profile := system.PlatformProfile{OS: "linux", PackageManager: "apt", Supported: true}

	result, _ := ExecuteTUIInstallWithBackgroundAndOrchestrator(
		home, selection, resolved, profile,
		model.OpenCodeBackgroundAuto, model.PiBackgroundIntent(""), nil,
	)

	if !slices.Contains(result.ManualActions, action) {
		t.Fatalf("TUI execution result dropped the manual action: %v", result.ManualActions)
	}
}
