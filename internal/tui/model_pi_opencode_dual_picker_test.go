package tui

import (
	"context"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/agents/capabilities"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/modelcatalog"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestSDDModeMulti_MixedAgentsShowsOpenCodeThenPIModelPicker(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	clearAgentCapabilitiesCache()
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		if agentID == model.AgentOpenCode {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() {
		resolveAgentCapabilitiesFn = origResolve
		clearAgentCapabilitiesCache()
	})

	origPILoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{
			Providers: map[string]modelcatalog.Provider{"openai": {ID: "openai", Name: "OpenAI"}},
			AvailableProviderIDs: []string{"openai"},
			SDDModels: map[string][]modelcatalog.Model{"openai": {{ID: "gpt-5", Name: "GPT-5"}}},
		}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origPILoader })

	origStat := osStatModelCache
	osStatModelCache = func(string) (os.FileInfo, error) { return nil, nil }
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode, model.AgentPiCodingAgent}
	m.Selection.Components = []model.ComponentID{model.ComponentEngram, model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.Screen != ScreenModelPicker || state.ModelPicker.Source != "opencode" {
		t.Fatalf("first picker = (%v, %q), want (ScreenModelPicker, opencode)", state.Screen, state.ModelPicker.Source)
	}

	state.Cursor = len(screens.ModelPickerRows())
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)
	if state.Screen != ScreenModelPicker || state.ModelPicker.Source != "pi" {
		t.Fatalf("second picker = (%v, %q), want (ScreenModelPicker, pi)", state.Screen, state.ModelPicker.Source)
	}

	state.Cursor = len(screens.ModelPickerRows())
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)
	if state.Screen != ScreenStrictTDD {
		t.Fatalf("after second picker continue: screen = %v, want %v", state.Screen, ScreenStrictTDD)
	}
}

func TestSDDModeMulti_MixedAssignmentsAreSeparatedBetweenOpenCodeAndPI(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	clearAgentCapabilitiesCache()
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent || agentID == model.AgentOpenCode {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() {
		resolveAgentCapabilitiesFn = origResolve
		clearAgentCapabilitiesCache()
	})

	origPILoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{
			Providers:            map[string]modelcatalog.Provider{"openai": {ID: "openai", Name: "OpenAI"}},
			AvailableProviderIDs: []string{"openai"},
			SDDModels:            map[string][]modelcatalog.Model{"openai": {{ID: "gpt-5", Name: "GPT-5"}}},
		}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origPILoader })

	origStat := osStatModelCache
	osStatModelCache = func(string) (os.FileInfo, error) { return nil, nil }
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode, model.AgentPiCodingAgent}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	state.Selection.ModelAssignments = map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
	}
	state.Cursor = len(screens.ModelPickerRows())
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)

	state.ModelPicker.Mode = screens.ModeModelSelect
	state.ModelPicker.SelectedProvider = "openai"
	state.ModelPicker.SelectedPhaseIdx = 2 // sdd-init
	updated, _ = state.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state = updated.(Model)

	if got := state.Selection.ModelAssignments["sdd-apply"].FullID(); got != "anthropic/claude-sonnet-4" {
		t.Fatalf("OpenCode assignment overwritten: got %q", got)
	}
	if got := state.Selection.PIModelAssignments["sdd-init"].FullID(); got != "openai/gpt-5" {
		t.Fatalf("PI assignment missing/wrong: got %q, want %q", got, "openai/gpt-5")
	}
}

func TestSDDModeMulti_PIOnlyStillUsesPIPicker(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	clearAgentCapabilitiesCache()
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() {
		resolveAgentCapabilitiesFn = origResolve
		clearAgentCapabilitiesCache()
	})
	origPILoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{AvailableProviderIDs: []string{"openai"}}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origPILoader })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentPiCodingAgent}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.ModelPicker.Source != "pi" {
		t.Fatalf("source = %q, want pi", state.ModelPicker.Source)
	}
}

func TestSDDModeMulti_OpenCodeOnlyStillUsesOpenCodePicker(t *testing.T) {
	origStat := osStatModelCache
	osStatModelCache = func(string) (os.FileInfo, error) { return nil, nil }
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Cursor = sddMultiCursor(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)
	if state.ModelPicker.Source != "opencode" {
		t.Fatalf("source = %q, want opencode", state.ModelPicker.Source)
	}
}
