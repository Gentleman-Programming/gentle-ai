package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/screens"
)

// TestTuiSyncOpenCodeSequentialProfilesPersist drives Configure Models ->
// Configure OpenCode Models -> provider/model/effort selection -> Continue ->
// confirmed Sync. Catalog discovery, runtime version, and SDK prerequisites are
// local fixtures; model selection and assignment persistence use real code.
func TestTuiSyncOpenCodeSequentialProfilesPersist(t *testing.T) {
	home := t.TempDir()
	t.Chdir(home)
	for _, key := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(key, home)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, key := range []string{"OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT"} {
		t.Setenv(key, "")
	}
	// Keep runtime-version discovery local as well: do not invoke the user's
	// installed OpenCode while testing model-config persistence.
	oldVersionRunner := opencode.VersionRunnerOverride
	t.Cleanup(func() { opencode.VersionRunnerOverride = oldVersionRunner })
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("2.0.4")}, nil
	}

	assignments := map[string]model.ModelAssignment{
		"gentle-orchestrator":         {ProviderID: "openai", ModelID: "gpt-5.6-sol", Effort: "low"},
		"sdd-orchestrator-claude-low": {ProviderID: "anthropic", ModelID: "claude-opus-5"},
		"sdd-research-claude-low":     {ProviderID: "anthropic", ModelID: "fable-5-1"},
		"sdd-orchestrator-mix":        {ProviderID: "anthropic", ModelID: "claude-opus-5"},
		"sdd-orchestrator-deepseek":   {ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
	}
	persisted := make(map[string]state.ModelAssignmentState, len(assignments))
	agents := make(map[string]map[string]string, len(assignments))
	for name, assignment := range assignments {
		persisted[name] = state.ModelAssignmentState{ProviderID: assignment.ProviderID, ModelID: assignment.ModelID, Effort: assignment.Effort}
		agents[name] = map[string]string{"model": assignment.FullID(), "description": "existing profile agent", "mode": "subagent"}
		if assignment.Effort != "" {
			agents[name]["variant"] = assignment.Effort
		}
	}
	if err := state.Write(home, state.InstallState{
		InstalledAgents:          []string{string(model.AgentOpenCode)},
		SelectionConfigured:      true,
		Components:               []model.ComponentID{},
		CommunityToolsConfigured: true,
		BackgroundIntent:         model.OpenCodeBackgroundOff,
		ModelAssignments:         persisted,
	}); err != nil {
		t.Fatal(err)
	}
	settingsPath := opencodeagent.NewAdapter().SettingsPath(home)
	// Satisfy the current V2 SDK prerequisite without npm or network access.
	// This test checks assignment persistence, not SDK execution.
	sdkManifest := filepath.Join(filepath.Dir(settingsPath), "node_modules", "@opencode", "plugin", "package.json")
	if err := os.MkdirAll(filepath.Dir(sdkManifest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sdkManifest, []byte(`{"version":"2.0.4"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	initial, err := json.Marshal(map[string]any{"agent": agents})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, initial, 0600); err != nil {
		t.Fatal(err)
	}

	assertFiles := func(t *testing.T) {
		t.Helper()
		stored, err := state.Read(home)
		if err != nil {
			t.Fatalf("read state.json: %v", err)
		}
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("read opencode.json: %v", err)
		}
		var settings struct {
			Agent map[string]struct {
				Model   string `json:"model"`
				Variant string `json:"variant"`
			} `json:"agent"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatalf("decode opencode.json: %v", err)
		}
		for name, want := range assignments {
			got, ok := stored.ModelAssignments[name]
			if !ok || got.ProviderID != want.ProviderID || got.ModelID != want.ModelID || got.Effort != want.Effort {
				t.Errorf("state.json assignment %s = %+v (present=%v), want %+v", name, got, ok, want)
			}
			entry, ok := settings.Agent[name]
			if !ok || entry.Model != want.FullID() || entry.Variant != want.Effort {
				t.Errorf("opencode.json agent %s = %+v (present=%v), want model=%s variant=%s", name, entry, ok, want.FullID(), want.Effort)
			}
		}
	}

	steps := []struct {
		name    string
		changes map[string]model.ModelAssignment
	}{
		{"configure claude-low and principal", map[string]model.ModelAssignment{
			"sdd-orchestrator-claude-low": {ProviderID: "anthropic", ModelID: "claude-opus-5-5"},
			"sdd-research-claude-low":     {ProviderID: "anthropic", ModelID: "claude-opus-5-5"},
			"gentle-orchestrator":         {ProviderID: "openai", ModelID: "gpt-5.6-sol", Effort: "high"},
		}},
		{"configure mix and deepseek without reverting claude-low", map[string]model.ModelAssignment{
			"sdd-orchestrator-mix":      {ProviderID: "anthropic", ModelID: "claude-opus-5-5"},
			"sdd-orchestrator-deepseek": {ProviderID: "deepseek", ModelID: "deepseek-v4-flash"},
		}},
		{"fresh sync without overrides", nil},
	}
	catalog := map[string]opencode.Provider{}
	for name, models := range map[string][]opencode.Model{
		"openai":    {{ID: "gpt-5.6-sol", Name: "gpt-5.6-sol", ToolCall: true, Reasoning: true, Variants: []string{"low", "high"}}},
		"anthropic": {{ID: "claude-opus-5-5", Name: "claude-opus-5-5", ToolCall: true}},
		"deepseek":  {{ID: "deepseek-v4-pro", Name: "deepseek-v4-pro", ToolCall: true}, {ID: "deepseek-v4-flash", Name: "deepseek-v4-flash", ToolCall: true}},
	} {
		provider := opencode.Provider{ID: name, Name: name, Models: map[string]opencode.Model{}}
		for _, entry := range models {
			provider.Models[entry.ID] = entry
		}
		catalog[name] = provider
	}
	newUI := func() tui.Model {
		m := tui.NewModel(system.DetectionResult{}, "test")
		m.SyncDetailedFn = tuiSyncDetailed(home)
		return m
	}
	m := newUI()
	press := func(key tea.KeyType) tea.Cmd {
		next, cmd := m.Update(tea.KeyMsg{Type: key})
		m = next.(tui.Model)
		return cmd
	}
	confirmSync := func(t *testing.T) {
		t.Helper()
		if m.Screen != tui.ScreenSync {
			t.Fatalf("Continue screen = %v, want Sync", m.Screen)
		}
		cmd := press(tea.KeyEnter)
		if cmd == nil || !m.OperationRunning {
			t.Fatal("confirming Sync did not start the operation")
		}
		message := cmd()
		commands, ok := message.(tea.BatchMsg)
		if !ok {
			t.Fatalf("Sync command returned %T, want BatchMsg", message)
		}
		completed := 0
		for _, command := range commands {
			if done, ok := command().(tui.SyncDoneMsg); ok {
				if done.Err != nil {
					t.Fatalf("TUI Sync: %v", done.Err)
				}
				next, _ := m.Update(done)
				m = next.(tui.Model)
				completed++
			}
		}
		if completed != 1 || !m.HasSyncRun || m.OperationRunning || m.PendingSyncOverrides != nil {
			t.Fatalf("Sync did not complete and clear overrides: completions=%d, ran=%v, running=%v, overrides=%v", completed, m.HasSyncRun, m.OperationRunning, m.PendingSyncOverrides)
		}
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) {
			if step.changes == nil {
				m = newUI()
				m.Screen = tui.ScreenSync
			} else {
				m.Cursor = 4 // Welcome: Configure Models.
				press(tea.KeyEnter)
				if m.Screen != tui.ScreenModelConfig {
					t.Fatalf("Configure Models screen = %v", m.Screen)
				}
				m.Cursor = 1        // Configure OpenCode Models.
				press(tea.KeyEnter) // Do not execute external catalog discovery.
				if m.Screen != tui.ScreenModelPicker {
					t.Fatalf("OpenCode menu screen = %v", m.Screen)
				}
				next, _ := m.Update(screens.RuntimeCatalogDiscoveryMsg{
					RequestID:  m.ModelPicker.CatalogRequestID,
					ProjectDir: m.ModelPicker.CatalogProjectDir,
					Providers:  catalog,
				})
				m = next.(tui.Model)
				for name, assignment := range step.changes {
					rowIndex := -1
					for i := range screens.ModelPickerRowsForState(m.ModelPicker) {
						if row, ok := screens.ModelPickerRowAt(m.ModelPicker, i); ok && row.AgentID == name {
							rowIndex = i
							break
						}
					}
					if rowIndex < 0 {
						t.Fatalf("picker did not discover agent %s", name)
					}
					m.Cursor = rowIndex
					press(tea.KeyEnter)
					providerIndex := -1
					for i, id := range m.ModelPicker.AvailableIDs {
						if id == assignment.ProviderID {
							providerIndex = i
						}
					}
					if providerIndex < 0 {
						t.Fatalf("provider %s unavailable", assignment.ProviderID)
					}
					for i := 0; i < providerIndex; i++ {
						press(tea.KeyDown)
					}
					press(tea.KeyEnter)
					modelIndex := -1
					for i, entry := range screens.FilteredModelEntries(m.ModelPicker) {
						if entry.ID == assignment.ModelID {
							modelIndex = i
						}
					}
					if modelIndex < 0 {
						t.Fatalf("model %s unavailable", assignment.ModelID)
					}
					for i := 0; i < modelIndex; i++ {
						press(tea.KeyDown)
					}
					press(tea.KeyEnter)
					if m.ModelPicker.Mode == screens.ModeEffortSelect {
						effortIndex := 0 // Provider default.
						for i, effort := range m.ModelPicker.SelectedModelEffortLevels {
							if effort == assignment.Effort {
								effortIndex = i + 1
							}
						}
						for i := 0; i < effortIndex; i++ {
							press(tea.KeyDown)
						}
						press(tea.KeyEnter)
					}
					if got := m.Selection.ModelAssignments[name]; got != assignment {
						t.Fatalf("picker selection %s = %+v, want %+v", name, got, assignment)
					}
					assignments[name] = assignment // Independent expected result only.
				}
				m.Cursor = len(screens.ModelPickerRowsForState(m.ModelPicker))
				press(tea.KeyEnter) // Continue.
			}
			confirmSync(t)
			assertFiles(t)
			press(tea.KeyEnter) // Return from completed Sync to Welcome.
		}) {
			break
		}
	}
}
