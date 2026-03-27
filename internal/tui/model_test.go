package tui

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestNavigationWelcomeToDetection(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenDetection {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenDetection)
	}
}

func TestNavigationBackWithEscape(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenPersona

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	state := updated.(Model)

	if state.Screen != ScreenAgents {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenAgents)
	}
}

func TestAgentSelectionToggleAndContinue(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenAgents
	m.Selection.Agents = []model.AgentID{model.AgentClaudeCode}
	m.Cursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	state := updated.(Model)

	if len(state.Selection.Agents) != 0 {
		t.Fatalf("agents = %v, want empty", state.Selection.Agents)
	}

	state.Cursor = len(screensAgentOptions())
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)

	if state.Screen != ScreenAgents {
		t.Fatalf("screen changed with no selected agents: %v", state.Screen)
	}

	state.Selection.Agents = []model.AgentID{model.AgentOpenCode}
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)

	if state.Screen != ScreenPersona {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenPersona)
	}
}

func TestReviewToInstallingInitializesProgress(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenReview

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenInstalling {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenInstalling)
	}

	if state.Progress.Current != 0 {
		t.Fatalf("progress current = %d, want 0", state.Progress.Current)
	}
}

func TestStepProgressMsgUpdatesProgressState(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.Progress = NewProgressState([]string{"step-a", "step-b"})

	// Send running event for step-a.
	updated, _ := m.Update(StepProgressMsg{StepID: "step-a", Status: pipeline.StepStatusRunning})
	state := updated.(Model)
	if state.Progress.Items[0].Status != ProgressStatusRunning {
		t.Fatalf("step-a status = %q, want running", state.Progress.Items[0].Status)
	}

	// Send succeeded event for step-a.
	updated, _ = state.Update(StepProgressMsg{StepID: "step-a", Status: pipeline.StepStatusSucceeded})
	state = updated.(Model)
	if state.Progress.Items[0].Status != string(pipeline.StepStatusSucceeded) {
		t.Fatalf("step-a status = %q, want succeeded", state.Progress.Items[0].Status)
	}

	// Send failed event for step-b.
	updated, _ = state.Update(StepProgressMsg{StepID: "step-b", Status: pipeline.StepStatusFailed, Err: fmt.Errorf("oops")})
	state = updated.(Model)
	if state.Progress.Items[1].Status != string(pipeline.StepStatusFailed) {
		t.Fatalf("step-b status = %q, want failed", state.Progress.Items[1].Status)
	}

	if !state.Progress.HasFailures() {
		t.Fatalf("expected HasFailures() = true")
	}
}

func TestPipelineDoneMsgMarksCompletion(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.pipelineRunning = true
	m.Progress = NewProgressState([]string{"step-x"})
	m.Progress.Start(0)

	// Simulate pipeline completion with a real step result.
	result := pipeline.ExecutionResult{
		Apply: pipeline.StageResult{
			Success: true,
			Steps: []pipeline.StepResult{
				{StepID: "step-x", Status: pipeline.StepStatusSucceeded},
			},
		},
	}
	updated, _ := m.Update(PipelineDoneMsg{Result: result})
	state := updated.(Model)

	if state.pipelineRunning {
		t.Fatalf("expected pipelineRunning = false")
	}

	if !state.Progress.Done() {
		t.Fatalf("expected progress to be done")
	}
}

func TestPipelineDoneMsgSurfacesFailedSteps(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.pipelineRunning = true
	m.Progress = NewProgressState([]string{"step-ok", "step-bad"})

	result := pipeline.ExecutionResult{
		Apply: pipeline.StageResult{
			Success: false,
			Err:     fmt.Errorf("step-bad failed"),
			Steps: []pipeline.StepResult{
				{StepID: "step-ok", Status: pipeline.StepStatusSucceeded},
				{StepID: "step-bad", Status: pipeline.StepStatusFailed, Err: fmt.Errorf("skill inject: write failed")},
			},
		},
		Err: fmt.Errorf("step-bad failed"),
	}
	updated, _ := m.Update(PipelineDoneMsg{Result: result})
	state := updated.(Model)

	if !state.Progress.HasFailures() {
		t.Fatalf("expected HasFailures() = true")
	}

	// Verify that the error message appears in the logs.
	found := false
	for _, log := range state.Progress.Logs {
		if contains(log, "skill inject: write failed") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error detail in logs, got: %v", state.Progress.Logs)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestInstallingScreenManualFallbackWithoutExecuteFn(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.Progress = NewProgressState([]string{"step-1", "step-2"})
	m.Progress.Start(0)
	// ExecuteFn is nil — manual fallback should work.

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	// First enter advances step-1 to succeeded.
	if state.Progress.Items[0].Status != "succeeded" {
		t.Fatalf("step-1 status = %q, want succeeded", state.Progress.Items[0].Status)
	}
}

func TestEscBlockedWhilePipelineRunning(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.pipelineRunning = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	state := updated.(Model)

	if state.Screen != ScreenInstalling {
		t.Fatalf("screen = %v, want ScreenInstalling (esc should be blocked)", state.Screen)
	}
}

func TestInstallingDoneToComplete(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenInstalling
	m.Progress = NewProgressState([]string{"only-step"})
	m.Progress.Mark(0, string(pipeline.StepStatusSucceeded))

	// Progress is at 100%, enter should go to complete.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenComplete {
		t.Fatalf("screen = %v, want ScreenComplete", state.Screen)
	}
}

func TestBuildProgressLabelsFromResolvedPlan(t *testing.T) {
	resolved := planner.ResolvedPlan{
		Agents:            []model.AgentID{model.AgentClaudeCode},
		OrderedComponents: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
	}

	labels := buildProgressLabels(resolved)

	want := []string{
		"prepare:check-dependencies",
		"prepare:backup-snapshot",
		"apply:rollback-restore",
		"agent:claude-code",
		"component:engram",
		"component:sdd",
	}

	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
}

func TestBackupRestoreMsgHandledGracefully(t *testing.T) {
	// Error case: BackupRestoreMsg with error navigates to ScreenRestoreResult
	// and stores the error in RestoreErr.
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenRestoreConfirm

	updated, _ := m.Update(BackupRestoreMsg{Err: fmt.Errorf("restore-error")})
	state := updated.(Model)

	if state.Screen != ScreenRestoreResult {
		t.Fatalf("error case: expected ScreenRestoreResult, got %v", state.Screen)
	}
	if state.RestoreErr == nil {
		t.Fatalf("expected RestoreErr to be set on error")
	}

	// Success case: BackupRestoreMsg with no error navigates to ScreenRestoreResult
	// with nil RestoreErr.
	m2 := NewModel(system.DetectionResult{}, "dev")
	m2.Screen = ScreenRestoreConfirm
	updated2, _ := m2.Update(BackupRestoreMsg{})
	state2 := updated2.(Model)

	if state2.Screen != ScreenRestoreResult {
		t.Fatalf("success case: expected ScreenRestoreResult, got %v", state2.Screen)
	}
	if state2.RestoreErr != nil {
		t.Fatalf("unexpected RestoreErr on success: %v", state2.RestoreErr)
	}
}

func TestShouldShowSDDModeScreen(t *testing.T) {
	tests := []struct {
		name       string
		agents     []model.AgentID
		components []model.ComponentID
		want       bool
	}{
		{
			name:       "OpenCode + SDD = true",
			agents:     []model.AgentID{model.AgentOpenCode},
			components: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
			want:       true,
		},
		{
			name:       "Claude only + SDD = false",
			agents:     []model.AgentID{model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
			want:       false,
		},
		{
			name:       "OpenCode + no SDD = false",
			agents:     []model.AgentID{model.AgentOpenCode},
			components: []model.ComponentID{model.ComponentEngram},
			want:       false,
		},
		{
			name:       "multiple agents including OpenCode + SDD = true",
			agents:     []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode},
			components: []model.ComponentID{model.ComponentSDD, model.ComponentEngram},
			want:       true,
		},
		{
			name:       "no agents + SDD = false",
			agents:     []model.AgentID{},
			components: []model.ComponentID{model.ComponentSDD},
			want:       false,
		},
		{
			name:       "OpenCode + empty components = false",
			agents:     []model.AgentID{model.AgentOpenCode},
			components: []model.ComponentID{},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Selection.Agents = tt.agents
			m.Selection.Components = tt.components

			got := m.shouldShowSDDModeScreen()
			if got != tt.want {
				t.Fatalf("shouldShowSDDModeScreen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldShowClaudeModelPickerScreen(t *testing.T) {
	tests := []struct {
		name       string
		agents     []model.AgentID
		components []model.ComponentID
		want       bool
	}{
		{
			name:       "Claude + SDD = true",
			agents:     []model.AgentID{model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
			want:       true,
		},
		{
			name:       "OpenCode + SDD = false",
			agents:     []model.AgentID{model.AgentOpenCode},
			components: []model.ComponentID{model.ComponentEngram, model.ComponentSDD},
			want:       false,
		},
		{
			name:       "Claude + no SDD = false",
			agents:     []model.AgentID{model.AgentClaudeCode},
			components: []model.ComponentID{model.ComponentEngram},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{}, "dev")
			m.Selection.Agents = tt.agents
			m.Selection.Components = tt.components

			if got := m.shouldShowClaudeModelPickerScreen(); got != tt.want {
				t.Fatalf("shouldShowClaudeModelPickerScreen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPresetFlowShowsClaudeModelPickerBeforeDependencyTree(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenPreset
	m.Selection.Agents = []model.AgentID{model.AgentClaudeCode}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram, model.ComponentSDD}
	m.Cursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenClaudeModelPicker {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenClaudeModelPicker)
	}
	if state.ClaudeModelPicker.Preset != screens.ClaudePresetBalanced {
		t.Fatalf("preset = %v, want %v", state.ClaudeModelPicker.Preset, screens.ClaudePresetBalanced)
	}
}

func TestClaudeModelPickerBalancedSelectionStoresAssignments(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenClaudeModelPicker
	m.Selection.Agents = []model.AgentID{model.AgentClaudeCode}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram, model.ComponentSDD}
	m.ClaudeModelPicker = screens.NewClaudeModelPickerState()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenDependencyTree {
		t.Fatalf("screen = %v, want %v", state.Screen, ScreenDependencyTree)
	}
	if got := state.Selection.ClaudeModelAssignments["orchestrator"]; got != model.ClaudeModelOpus {
		t.Fatalf("orchestrator = %q, want %q", got, model.ClaudeModelOpus)
	}
	if got := state.Selection.ClaudeModelAssignments["default"]; got != model.ClaudeModelSonnet {
		t.Fatalf("default = %q, want %q", got, model.ClaudeModelSonnet)
	}
	if got := state.Selection.ClaudeModelAssignments["sdd-archive"]; got != model.ClaudeModelHaiku {
		t.Fatalf("sdd-archive = %q, want %q", got, model.ClaudeModelHaiku)
	}
}

// ─── SDDMode → ModelPicker / DependencyTree transition (issue #106 Bug 2) ──

// sddMultiCursor returns the cursor index for SDDModeMulti in SDDModeOptions.
func sddMultiCursor(t *testing.T) int {
	t.Helper()
	for i, opt := range screens.SDDModeOptions() {
		if opt == model.SDDModeMulti {
			return i
		}
	}
	t.Fatal("SDDModeMulti not found in SDDModeOptions()")
	return -1
}

// TestSDDModeMultiSkipModelPickerWhenCacheMissing verifies that when SDDModeMulti
// is selected and the OpenCode model cache does NOT exist on disk, the TUI skips
// the model picker and goes directly to ScreenDependencyTree.
// This is the "fresh install" path where OpenCode has not been run yet.
func TestSDDModeMultiSkipModelPickerWhenCacheMissing(t *testing.T) {
	origStat := osStatModelCache
	osStatModelCache = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram, model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenDependencyTree {
		t.Fatalf("screen = %v, want ScreenDependencyTree (cache missing → skip model picker)", state.Screen)
	}
	if len(state.ModelPicker.AvailableIDs) != 0 {
		t.Fatalf("ModelPicker.AvailableIDs should be empty when cache missing, got: %v", state.ModelPicker.AvailableIDs)
	}
}

// TestSDDModeMultiShowsModelPickerWhenCacheExists verifies that when SDDModeMulti
// is selected and the OpenCode model cache EXISTS on disk, the TUI transitions to
// ScreenModelPicker so the user can assign models to SDD phases.
func TestSDDModeMultiShowsModelPickerWhenCacheExists(t *testing.T) {
	// Write a minimal valid models.json so NewModelPickerState can parse it.
	tmpDir := t.TempDir()
	cacheFile := tmpDir + "/models.json"
	if err := os.WriteFile(cacheFile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origStat := osStatModelCache
	osStatModelCache = func(name string) (os.FileInfo, error) {
		return os.Stat(cacheFile) // stat succeeds → cache present
	}
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram, model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenModelPicker {
		t.Fatalf("screen = %v, want ScreenModelPicker (cache present → show picker)", state.Screen)
	}
}

func screensAgentOptions() []model.AgentID {
	return screens.AgentOptions()
}

// ─── Detection-default consumer regression tests ───────────────────────────

// makeDetectionWithAgents builds a DetectionResult with the specified agents
// marked as Exists=true. All other agents are absent.
func makeDetectionWithAgents(present ...string) system.DetectionResult {
	known := []string{"claude-code", "opencode", "gemini-cli", "cursor", "vscode-copilot", "codex"}
	presentSet := make(map[string]bool, len(present))
	for _, p := range present {
		presentSet[p] = true
	}
	var configs []system.ConfigState
	for _, agent := range known {
		configs = append(configs, system.ConfigState{
			Agent:       agent,
			Path:        "/tmp/fake/" + agent,
			Exists:      presentSet[agent],
			IsDirectory: presentSet[agent],
		})
	}
	return system.DetectionResult{Configs: configs}
}

// TestPreselectedAgents_CodexIsIncludedWhenPresent is a regression guard:
// when the codex config dir is detected, preselectedAgents must include
// model.AgentCodex. Previously the switch statement omitted codex, so
// detection-driven TUI preselection silently dropped it.
func TestPreselectedAgents_CodexIsIncludedWhenPresent(t *testing.T) {
	detection := makeDetectionWithAgents("codex")
	selected := preselectedAgents(detection)

	found := false
	for _, id := range selected {
		if id == model.AgentCodex {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("preselectedAgents() did not include codex even though config dir is present; got %v", selected)
	}
}

// TestPreselectedAgents_AllSixAgentsMappedCorrectly verifies every canonical
// agent string maps to its model.AgentID constant in preselectedAgents.
// This prevents silent drops when new agents are added to ScanConfigs without
// updating the TUI switch statement.
func TestPreselectedAgents_AllSixAgentsMappedCorrectly(t *testing.T) {
	tests := []struct {
		configAgent string
		wantID      model.AgentID
	}{
		{"claude-code", model.AgentClaudeCode},
		{"opencode", model.AgentOpenCode},
		{"gemini-cli", model.AgentGeminiCLI},
		{"cursor", model.AgentCursor},
		{"vscode-copilot", model.AgentVSCodeCopilot},
		{"codex", model.AgentCodex},
	}

	for _, tt := range tests {
		t.Run(tt.configAgent, func(t *testing.T) {
			detection := makeDetectionWithAgents(tt.configAgent)
			selected := preselectedAgents(detection)

			found := false
			for _, id := range selected {
				if id == tt.wantID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("preselectedAgents() missing %q → %q mapping; got %v",
					tt.configAgent, tt.wantID, selected)
			}
			// Exactly one agent should be in the result (only one dir exists).
			if len(selected) != 1 {
				t.Errorf("preselectedAgents() returned %d agents, want 1 (only %q detected); got %v",
					len(selected), tt.configAgent, selected)
			}
		})
	}
}
