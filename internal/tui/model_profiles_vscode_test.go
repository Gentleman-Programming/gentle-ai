package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

// newModelWithVSCodeDetected returns a Model that reports VS Code Copilot as detected.
func newModelWithVSCodeDetected() Model {
	m := NewModel(system.DetectionResult{
		Configs: []system.ConfigState{
			{Agent: string(model.AgentVSCodeCopilot), Exists: true},
		},
	}, "dev")
	return m
}

// newModelWithBothDetected returns a Model with both OpenCode and VS Code detected.
func newModelWithBothDetected() Model {
	m := NewModel(system.DetectionResult{
		Configs: []system.ConfigState{
			{Agent: string(model.AgentOpenCode), Exists: true},
			{Agent: string(model.AgentVSCodeCopilot), Exists: true},
		},
	}, "dev")
	return m
}

// TestHasDetectedVSCode verifies the detection flag based on Detection.Configs.
func TestHasDetectedVSCode(t *testing.T) {
	tests := []struct {
		name    string
		configs []system.ConfigState
		want    bool
	}{
		{
			name:    "vscode detected and exists",
			configs: []system.ConfigState{{Agent: string(model.AgentVSCodeCopilot), Exists: true}},
			want:    true,
		},
		{
			name:    "vscode present but not exists",
			configs: []system.ConfigState{{Agent: string(model.AgentVSCodeCopilot), Exists: false}},
			want:    false,
		},
		{
			name:    "opencode detected, not vscode",
			configs: []system.ConfigState{{Agent: string(model.AgentOpenCode), Exists: true}},
			want:    false,
		},
		{
			name:    "empty configs",
			configs: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(system.DetectionResult{Configs: tt.configs}, "dev")
			if got := m.hasDetectedVSCode(); got != tt.want {
				t.Errorf("hasDetectedVSCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWelcomeMenuShowsVSCodeProfiles verifies that the VS Code profile entry
// appears in the welcome menu when VS Code is detected.
func TestWelcomeMenuShowsVSCodeProfiles(t *testing.T) {
	m := newModelWithVSCodeDetected()
	m.VSCodeProfileList = []model.Profile{{Name: "cheap"}, {Name: "fast"}}

	view := m.View()

	if !strings.Contains(view, "VS Code SDD Profiles (2)") {
		t.Errorf("welcome view missing 'VS Code SDD Profiles (2)', got:\n%s", view)
	}
}

// TestWelcomeMenuHidesVSCodeProfilesWhenNotDetected ensures the entry is absent
// when VS Code is not detected.
func TestWelcomeMenuHidesVSCodeProfilesWhenNotDetected(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.VSCodeProfileList = nil

	view := m.View()

	if strings.Contains(view, "VS Code SDD Profiles") {
		t.Errorf("welcome view should NOT contain 'VS Code SDD Profiles' when not detected, got:\n%s", view)
	}
}

// TestActiveProfileAdapter_SetOnWelcomeClick verifies that clicking the VS Code profiles
// menu item sets ActiveProfileAdapter to AgentVSCodeCopilot and transitions to ScreenProfiles.
func TestActiveProfileAdapter_SetOnWelcomeClick(t *testing.T) {
	m := newModelWithVSCodeDetected()
	m.Screen = ScreenWelcome

	// Compute which cursor index is the VS Code profiles entry.
	// Menu: 0=Install, 1=Upgrade, 2=Sync, 3=Upgrade+Sync, 4=ModelConfig,
	// 5=AgentBuilder, 6=Plugins, 7=VSCodeProfiles(since OpenCode not detected), 8=Backups, 9=Uninstall, 10=Quit
	opts := screens.WelcomeOptions(nil, false, false, 0, false, true, 0)
	vscodeIdx := -1
	for i, opt := range opts {
		if strings.HasPrefix(opt, "VS Code SDD Profiles") {
			vscodeIdx = i
			break
		}
	}
	if vscodeIdx == -1 {
		t.Fatal("VS Code SDD Profiles not found in WelcomeOptions")
	}

	m.Cursor = vscodeIdx
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenProfiles {
		t.Errorf("screen = %v, want ScreenProfiles", state.Screen)
	}
	if state.ActiveProfileAdapter != model.AgentVSCodeCopilot {
		t.Errorf("ActiveProfileAdapter = %q, want %q", state.ActiveProfileAdapter, model.AgentVSCodeCopilot)
	}
}

// TestVSCodeProfileCreate_GeneratesFiles verifies that confirming a VS Code profile
// create writes 10 agent files and refreshes VSCodeProfileList (no sync triggered).
func TestVSCodeProfileCreate_GeneratesFiles(t *testing.T) {
	agentsDir := t.TempDir()

	// Override the readVSCodeProfilesFn so it reads from our temp dir.
	restore := overrideReadVSCodeProfilesFn(agentsDir)
	defer restore()

	// Override vscodeAgentsDirFn to point at temp dir.
	restoreDir := overrideVSCodeAgentsDirFn(agentsDir)
	defer restoreDir()

	m := newModelWithVSCodeDetected()
	m.Screen = ScreenProfileCreate
	m.ActiveProfileAdapter = model.AgentVSCodeCopilot
	m.ProfileCreateStep = 2
	m.ProfileEditMode = false
	m.ProfileDraft = model.Profile{
		Name: "testprofile",
		PhaseAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
		},
	}
	m.Cursor = 0 // "Create"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	// Must stay on ScreenProfiles, not ScreenSync
	if state.Screen != ScreenProfiles {
		t.Errorf("screen = %v, want ScreenProfiles (no sync for VS Code)", state.Screen)
	}

	// 10 files must exist in agentsDir
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", agentsDir, err)
	}
	if len(entries) != 10 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected 10 agent files, got %d: %v", len(entries), names)
	}

	// Profile list should be refreshed
	if len(state.VSCodeProfileList) == 0 {
		t.Error("VSCodeProfileList should be refreshed after create, but is empty")
	}
}

// TestVSCodeProfileDelete_RemovesFiles_NoSync verifies that delete removes agent files
// and does NOT trigger a sync operation.
func TestVSCodeProfileDelete_RemovesFiles_NoSync(t *testing.T) {
	agentsDir := t.TempDir()

	// Write 10 sdd-*-cheap.agent.md files
	sddPhases := []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	}
	for _, phase := range sddPhases {
		fname := phase + "-cheap.agent.md"
		if err := os.WriteFile(filepath.Join(agentsDir, fname), []byte("content"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", fname, err)
		}
	}

	restoreDir := overrideVSCodeAgentsDirFn(agentsDir)
	defer restoreDir()
	restoreRead := overrideReadVSCodeProfilesFn(agentsDir)
	defer restoreRead()

	m := newModelWithVSCodeDetected()
	m.Screen = ScreenProfileDelete
	m.ActiveProfileAdapter = model.AgentVSCodeCopilot
	m.ProfileDeleteTarget = "cheap"
	m.Cursor = 0 // "Delete" button

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	// Must return to ScreenProfiles, not ScreenSync
	if state.Screen != ScreenProfiles {
		t.Errorf("screen = %v, want ScreenProfiles (no sync for VS Code delete)", state.Screen)
	}
	// OperationRunning must NOT be set (no sync launched)
	if state.OperationRunning {
		t.Error("OperationRunning should be false after VS Code delete (no sync)")
	}
	// Files must be gone
	for _, phase := range sddPhases {
		fname := phase + "-cheap.agent.md"
		if _, err := os.Stat(filepath.Join(agentsDir, fname)); !os.IsNotExist(err) {
			t.Errorf("file %q should have been removed", fname)
		}
	}
}

// TestRenderProfiles_AdapterLabel verifies that the profiles screen title reflects
// the active adapter.
func TestRenderProfiles_AdapterLabel(t *testing.T) {
	tests := []struct {
		name        string
		adapterLabel string
		wantTitle   string
	}{
		{"opencode adapter", "OpenCode", "OpenCode SDD Profiles"},
		{"vscode adapter", "VS Code", "VS Code SDD Profiles"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := screens.RenderProfiles(nil, 0, nil, tt.adapterLabel)
			if !strings.Contains(view, tt.wantTitle) {
				t.Errorf("RenderProfiles with adapterLabel=%q missing %q in output:\n%s",
					tt.adapterLabel, tt.wantTitle, view)
			}
		})
	}
}

// TestRenderProfileDelete_VSCodeWording verifies the wording adapts for VS Code.
func TestRenderProfileDelete_VSCodeWording(t *testing.T) {
	t.Run("opencode wording", func(t *testing.T) {
		view := screens.RenderProfileDelete("myprofile", 0, false)
		if !strings.Contains(view, "Delete & Sync") {
			t.Errorf("OpenCode delete should show 'Delete & Sync', got:\n%s", view)
		}
	})

	t.Run("vscode wording", func(t *testing.T) {
		view := screens.RenderProfileDelete("myprofile", 0, true)
		if !strings.Contains(view, "10 agent files") {
			t.Errorf("VS Code delete should mention '10 agent files', got:\n%s", view)
		}
		if strings.Contains(view, "Delete & Sync") {
			t.Errorf("VS Code delete should NOT show 'Delete & Sync', got:\n%s", view)
		}
	})
}

// TestWelcomeMenuBothDetected verifies both profile entries appear when both adapters detected.
func TestWelcomeMenuBothDetected(t *testing.T) {
	m := newModelWithBothDetected()
	m.ProfileList = []model.Profile{{Name: "oc-profile"}}
	m.VSCodeProfileList = []model.Profile{{Name: "vsc-profile"}}

	view := m.View()

	if !strings.Contains(view, "OpenCode SDD Profiles (1)") {
		t.Errorf("welcome view missing 'OpenCode SDD Profiles (1)', got:\n%s", view)
	}
	if !strings.Contains(view, "VS Code SDD Profiles (1)") {
		t.Errorf("welcome view missing 'VS Code SDD Profiles (1)', got:\n%s", view)
	}
}

// --- helpers for test injection ---

func overrideReadVSCodeProfilesFn(agentsDir string) func() {
	original := readVSCodeProfilesFn
	readVSCodeProfilesFn = func(dir string) ([]model.Profile, error) {
		// count sdd-*-{name}.agent.md files and return profile names
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		seen := make(map[string]struct{})
		phases := []string{
			"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
			"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".agent.md") || !strings.HasPrefix(name, "sdd-") {
				continue
			}
			base := name[:len(name)-len(".agent.md")]
			for _, phase := range phases {
				prefix := phase + "-"
				if strings.HasPrefix(base, prefix) {
					profileName := base[len(prefix):]
					if profileName != "" {
						seen[profileName] = struct{}{}
					}
				}
			}
		}
		result := make([]model.Profile, 0, len(seen))
		for n := range seen {
			result = append(result, model.Profile{Name: n})
		}
		return result, nil
	}
	return func() { readVSCodeProfilesFn = original }
}

func overrideVSCodeAgentsDirFn(dir string) func() {
	original := vscodeAgentsDirFn
	vscodeAgentsDirFn = func() string { return dir }
	return func() { vscodeAgentsDirFn = original }
}
