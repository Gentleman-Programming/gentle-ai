package tui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestFinishAgentBuilderInstall_RegistryFailuresSkipSDD(t *testing.T) {
	registryErr := errors.New("registry unavailable")
	for _, tt := range []struct {
		name string
		fail func(*agentBuilderPostInstallOps)
	}{
		{
			name: "create registry directory",
			fail: func(ops *agentBuilderPostInstallOps) {
				ops.mkdirAll = func(string, os.FileMode) error { return registryErr }
			},
		},
		{
			name: "load registry",
			fail: func(ops *agentBuilderPostInstallOps) {
				ops.loadRegistry = func(string) (*agentbuilder.Registry, error) { return nil, registryErr }
			},
		},
		{
			name: "save registry",
			fail: func(ops *agentBuilderPostInstallOps) {
				ops.saveRegistry = func(string, *agentbuilder.Registry) error { return registryErr }
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			injections := 0
			ops := successfulAgentBuilderPostInstallOps(&injections)
			tt.fail(&ops)

			err := finishAgentBuilderInstall(sddAgent(), testAgentBuilderAdapters(), successfulInstallResults(), model.AgentClaudeCode, "/registry/custom-agents.json", ops)
			if !errors.Is(err, registryErr) {
				t.Fatalf("error = %v, want wrapped %v", err, registryErr)
			}
			if injections != 0 {
				t.Fatalf("SDD injections = %d, want 0 after registry failure", injections)
			}
			assertAgentBuilderInstallErrorScreen(t, err)
		})
	}
}

func TestFinishAgentBuilderInstall_SDDInjectionFailureShowsRetry(t *testing.T) {
	injectionErr := errors.New("prompt is read-only")
	injections := 0
	ops := successfulAgentBuilderPostInstallOps(&injections)
	ops.injectSDDReference = func(*agentbuilder.GeneratedAgent, string) error {
		injections++
		return injectionErr
	}

	err := finishAgentBuilderInstall(sddAgent(), testAgentBuilderAdapters(), successfulInstallResults(), model.AgentClaudeCode, "/registry/custom-agents.json", ops)
	if !errors.Is(err, injectionErr) {
		t.Fatalf("error = %v, want wrapped %v", err, injectionErr)
	}
	if injections != 1 {
		t.Fatalf("SDD injections = %d, want fail-fast 1", injections)
	}
	assertAgentBuilderInstallErrorScreen(t, err)
}

func TestFinishAgentBuilderInstall_SDDPathFailureStopsBeforeInjection(t *testing.T) {
	pathErr := errors.New("prompt path unavailable")
	injections := 0
	ops := successfulAgentBuilderPostInstallOps(&injections)
	ops.systemPromptPath = func(model.AgentID) (string, error) { return "", pathErr }

	err := finishAgentBuilderInstall(sddAgent(), testAgentBuilderAdapters(), successfulInstallResults(), model.AgentClaudeCode, "/registry/custom-agents.json", ops)
	if !errors.Is(err, pathErr) {
		t.Fatalf("error = %v, want wrapped %v", err, pathErr)
	}
	if injections != 0 {
		t.Fatalf("SDD injections = %d, want 0 after path failure", injections)
	}
}

func TestAgentBuilderInstallingErrorRetryRestartsInstall(t *testing.T) {
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenAgentBuilderInstalling
	m.Cursor = 0
	m.AgentBuilder = AgentBuilderState{
		Generated:  &agentbuilder.GeneratedAgent{Name: "test-agent"},
		InstallErr: errors.New("registry unavailable"),
	}

	updated, cmd := m.confirmSelection()
	state := updated.(Model)
	if cmd == nil {
		t.Fatal("Retry must start an installation command")
	}
	if state.Screen != ScreenAgentBuilderInstalling {
		t.Fatalf("screen = %v, want ScreenAgentBuilderInstalling", state.Screen)
	}
	if !state.AgentBuilder.Installing {
		t.Fatal("Retry must set Installing")
	}
	if state.AgentBuilder.InstallErr != nil {
		t.Fatalf("InstallErr = %v, want nil", state.AgentBuilder.InstallErr)
	}
}

func successfulAgentBuilderPostInstallOps(injections *int) agentBuilderPostInstallOps {
	return agentBuilderPostInstallOps{
		mkdirAll:     func(string, os.FileMode) error { return nil },
		loadRegistry: func(string) (*agentbuilder.Registry, error) { return &agentbuilder.Registry{Version: 1}, nil },
		saveRegistry: func(string, *agentbuilder.Registry) error { return nil },
		systemPromptPath: func(agentID model.AgentID) (string, error) {
			return "/prompts/" + string(agentID), nil
		},
		injectSDDReference: func(*agentbuilder.GeneratedAgent, string) error {
			(*injections)++
			return nil
		},
	}
}

func sddAgent() *agentbuilder.GeneratedAgent {
	return &agentbuilder.GeneratedAgent{
		Name:      "test-agent",
		Title:     "Test Agent",
		SDDConfig: &agentbuilder.SDDIntegration{Mode: agentbuilder.SDDPhaseSupport, TargetPhase: "apply"},
	}
}

func testAgentBuilderAdapters() []agentbuilder.AdapterInfo {
	return []agentbuilder.AdapterInfo{
		{AgentID: model.AgentClaudeCode},
		{AgentID: model.AgentOpenCode},
	}
}

func successfulInstallResults() []agentbuilder.InstallResult {
	return []agentbuilder.InstallResult{{AgentID: model.AgentClaudeCode, Success: true}}
}

func assertAgentBuilderInstallErrorScreen(t *testing.T, installErr error) {
	t.Helper()
	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenAgentBuilderInstalling
	m.AgentBuilder.Installing = true

	updated, _ := m.Update(AgentBuilderInstallDoneMsg{Results: successfulInstallResults(), Err: installErr})
	state := updated.(Model)
	if state.Screen != ScreenAgentBuilderInstalling {
		t.Fatalf("screen = %v, want ScreenAgentBuilderInstalling", state.Screen)
	}
	if state.AgentBuilder.Installing {
		t.Fatal("Installing should be false after AgentBuilderInstallDoneMsg")
	}
	if !errors.Is(state.AgentBuilder.InstallErr, installErr) {
		t.Fatalf("InstallErr = %v, want %v", state.AgentBuilder.InstallErr, installErr)
	}
	view := state.View()
	if !strings.Contains(view, "Retry") {
		t.Fatalf("error view = %q, want Retry", view)
	}
	if strings.Contains(view, "Installation Complete") {
		t.Fatalf("error view = %q, must not show success", view)
	}
}
