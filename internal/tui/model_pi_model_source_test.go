package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/internal/agents/capabilities"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/modelcatalog"
	"github.com/gentleman-programming/gentle-ai/internal/system"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func modelConfigOptionCursor(t *testing.T, opts []string, target string) int {
	t.Helper()
	for i, opt := range opts {
		if opt == target {
			return i
		}
	}
	t.Fatalf("option %q not found in %v", target, opts)
	return -1
}

func TestModelConfig_PIOptionLoadsPickerFromPIRPC(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() { resolveAgentCapabilitiesFn = origResolve })

	origLoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{
			Providers: map[string]modelcatalog.Provider{
				"openai": {
					ID:   "openai",
					Name: "OpenAI",
					Models: map[string]modelcatalog.Model{
						"gpt-5": {ID: "gpt-5", Name: "GPT-5"},
					},
				},
			},
			AvailableProviderIDs: []string{"openai"},
			SDDModels: map[string][]modelcatalog.Model{
				"openai": {{ID: "gpt-5", Name: "GPT-5"}},
			},
		}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentPiCodingAgent}
	m.Screen = ScreenModelConfig
	m.Cursor = modelConfigOptionCursor(t, screens.ModelConfigOptionsForCapabilities(true), screens.ModelConfigOptionPI)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenModelPicker {
		t.Fatalf("screen = %v, want ScreenModelPicker", state.Screen)
	}
	if state.ModelPicker.Source != "pi" {
		t.Fatalf("ModelPicker.Source = %q, want %q", state.ModelPicker.Source, "pi")
	}
	if len(state.ModelPicker.AvailableIDs) != 1 || state.ModelPicker.AvailableIDs[0] != "openai" {
		t.Fatalf("unexpected PI picker providers: %v", state.ModelPicker.AvailableIDs)
	}
}

func TestModelConfig_PIOptionRPCFailureBlocksPicker(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() { resolveAgentCapabilitiesFn = origResolve })

	origLoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{}, errors.New("rpc unavailable")
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentPiCodingAgent}
	m.Screen = ScreenModelConfig
	m.Cursor = modelConfigOptionCursor(t, screens.ModelConfigOptionsForCapabilities(true), screens.ModelConfigOptionPI)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if !state.ModelPicker.CapabilityBlocked {
		t.Fatal("expected PI picker to be blocked when RPC model load fails")
	}
	if strings.Contains(state.ModelPicker.CapabilityMessage, "signal: killed") {
		t.Fatalf("CapabilityMessage should hide raw process signal details, got %q", state.ModelPicker.CapabilityMessage)
	}
	if !strings.Contains(state.ModelPicker.CapabilityMessage, "Run `pi --mode rpc` manually") {
		t.Fatalf("CapabilityMessage should include actionable manual verification guidance, got %q", state.ModelPicker.CapabilityMessage)
	}
}

func TestModelConfig_PIOptionRPCFailureKilled_ShowsActionableMessage(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() { resolveAgentCapabilitiesFn = origResolve })

	origLoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		return modelcatalog.Catalog{}, errors.New("PI RPC command failed: signal: killed")
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Selection.Agents = []model.AgentID{model.AgentPiCodingAgent}
	m.Screen = ScreenModelConfig
	m.Cursor = modelConfigOptionCursor(t, screens.ModelConfigOptionsForCapabilities(true), screens.ModelConfigOptionPI)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if !state.ModelPicker.CapabilityBlocked {
		t.Fatal("expected PI picker to be blocked when RPC model load fails")
	}
	if strings.Contains(state.ModelPicker.CapabilityMessage, "signal: killed") {
		t.Fatalf("expected actionable message without raw signal details, got %q", state.ModelPicker.CapabilityMessage)
	}
	if !strings.Contains(state.ModelPicker.CapabilityMessage, "Run `pi --mode rpc` manually") {
		t.Fatalf("expected manual verification guidance, got %q", state.ModelPicker.CapabilityMessage)
	}
}

func TestModelConfig_PIUnsupportedCapabilityShowsCanonicalBlockedMessage(t *testing.T) {
	origResolve := resolveAgentCapabilitiesFn
	clearAgentCapabilitiesCache()
	resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
		if agentID == model.AgentPiCodingAgent {
			return capabilities.ResolvedCapabilities{}, nil
		}
		return capabilities.ResolvedCapabilities{}, nil
	}
	t.Cleanup(func() {
		resolveAgentCapabilitiesFn = origResolve
		clearAgentCapabilitiesCache()
	})

	origLoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		t.Fatal("PI RPC loader should not run when capability is blocked")
		return modelcatalog.Catalog{}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

	m := NewModel(system.DetectionResult{
		Configs: []system.ConfigState{{Agent: string(model.AgentPiCodingAgent), Exists: true, IsDirectory: true}},
	}, "dev")
	m.Selection.Agents = nil // regression guard: detected PI must still expose model config option
	m.Screen = ScreenModelConfig
	m.Cursor = modelConfigOptionCursor(t, screens.ModelConfigOptionsForCapabilities(true), screens.ModelConfigOptionPI)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenModelPicker {
		t.Fatalf("screen = %v, want ScreenModelPicker", state.Screen)
	}
	if state.ModelPicker.Source != "pi" {
		t.Fatalf("ModelPicker.Source = %q, want %q", state.ModelPicker.Source, "pi")
	}
	if !state.ModelPicker.CapabilityBlocked {
		t.Fatal("expected PI model picker to be blocked when capability is unsupported")
	}
	if state.ModelPicker.CapabilityMessage != capabilities.PiMultiModelRequiresPiSubagentsMessage {
		t.Fatalf("CapabilityMessage = %q, want %q", state.ModelPicker.CapabilityMessage, capabilities.PiMultiModelRequiresPiSubagentsMessage)
	}
}

func TestSDDModeMulti_MixedAgentsKeepsOpenCodeSource(t *testing.T) {
	origLoader := loadPIModelCatalogFn
	loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
		t.Fatal("PI loader must not be called when OpenCode is selected")
		return modelcatalog.Catalog{}, nil
	}
	t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

	origStat := osStatModelCache
	osStatModelCache = func(string) (os.FileInfo, error) { return nil, nil }
	t.Cleanup(func() { osStatModelCache = origStat })

	m := NewModel(system.DetectionResult{}, "dev")
	m.Screen = ScreenSDDMode
	m.Selection.Agents = []model.AgentID{model.AgentOpenCode, model.AgentPiCodingAgent}
	m.Selection.Components = []model.ComponentID{model.ComponentSDD}
	m.Cursor = 1 // multi

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	state := updated.(Model)

	if state.Screen != ScreenModelPicker {
		t.Fatalf("screen = %v, want ScreenModelPicker", state.Screen)
	}
	if state.ModelPicker.Source != "opencode" {
		t.Fatalf("ModelPicker.Source = %q, want %q", state.ModelPicker.Source, "opencode")
	}
}

func TestSDDModeMulti_ModelSourceMatrix(t *testing.T) {
	tests := []struct {
		name       string
		agents     []model.AgentID
		wantSource string
		piLoads    int
	}{
		{
			name:       "mixed opencode+pi prefers opencode model source",
			agents:     []model.AgentID{model.AgentOpenCode, model.AgentPiCodingAgent},
			wantSource: "opencode",
			piLoads:    0,
		},
		{
			name:       "pi-only uses pi rpc model source",
			agents:     []model.AgentID{model.AgentPiCodingAgent},
			wantSource: "pi",
			piLoads:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origResolve := resolveAgentCapabilitiesFn
			clearAgentCapabilitiesCache()
			resolveAgentCapabilitiesFn = func(_, _ string, agentID model.AgentID) (capabilities.ResolvedCapabilities, error) {
				if agentID == model.AgentPiCodingAgent {
					return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true, SupportsModelPicker: true}, nil
				}
				if agentID == model.AgentOpenCode {
					return capabilities.ResolvedCapabilities{SupportsSDDMultiMode: true}, nil
				}
				return capabilities.ResolvedCapabilities{}, nil
			}
			t.Cleanup(func() {
				resolveAgentCapabilitiesFn = origResolve
				clearAgentCapabilitiesCache()
			})

			piLoadCalls := 0
			origLoader := loadPIModelCatalogFn
			loadPIModelCatalogFn = func(context.Context) (modelcatalog.Catalog, error) {
				piLoadCalls++
				return modelcatalog.Catalog{
					Providers: map[string]modelcatalog.Provider{
						"openai": {
							ID:   "openai",
							Name: "OpenAI",
							Models: map[string]modelcatalog.Model{
								"gpt-5": {ID: "gpt-5", Name: "GPT-5"},
							},
						},
					},
					AvailableProviderIDs: []string{"openai"},
				}, nil
			}
			t.Cleanup(func() { loadPIModelCatalogFn = origLoader })

			origStat := osStatModelCache
			osStatModelCache = func(string) (os.FileInfo, error) { return nil, nil }
			t.Cleanup(func() { osStatModelCache = origStat })

			m := NewModel(system.DetectionResult{}, "dev")
			m.Screen = ScreenSDDMode
			m.Selection.Agents = tt.agents
			m.Selection.Components = []model.ComponentID{model.ComponentSDD}
			m.Cursor = 1 // multi

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			state := updated.(Model)

			if state.Screen != ScreenModelPicker {
				t.Fatalf("screen = %v, want %v", state.Screen, ScreenModelPicker)
			}
			if state.ModelPicker.Source != tt.wantSource {
				t.Fatalf("ModelPicker.Source = %q, want %q", state.ModelPicker.Source, tt.wantSource)
			}
			if piLoadCalls != tt.piLoads {
				t.Fatalf("PI model catalog calls = %d, want %d", piLoadCalls, tt.piLoads)
			}
		})
	}
}
