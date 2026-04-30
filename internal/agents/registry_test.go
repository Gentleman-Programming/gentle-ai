package agents

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

type mockAdapter struct {
	agent model.AgentID
}

func (m mockAdapter) Agent() model.AgentID      { return m.agent }
func (m mockAdapter) Tier() model.SupportTier   { return model.TierFull }
func (m mockAdapter) SupportsAutoInstall() bool { return true }
func (m mockAdapter) Detect(_ context.Context, _ string) (bool, string, string, bool, error) {
	return false, "", "", false, nil
}
func (m mockAdapter) InstallCommand(system.PlatformProfile) ([][]string, error) { return nil, nil }
func (m mockAdapter) GlobalConfigDir(_ string) string                           { return "" }
func (m mockAdapter) SystemPromptDir(_ string) string                           { return "" }
func (m mockAdapter) SystemPromptFile(_ string) string                          { return "" }
func (m mockAdapter) SkillsDir(_ string) string                                 { return "" }
func (m mockAdapter) SettingsPath(_ string) string                              { return "" }
func (m mockAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}
func (m mockAdapter) MCPStrategy() model.MCPStrategy          { return model.StrategySeparateMCPFiles }
func (m mockAdapter) MCPConfigPath(_ string, _ string) string { return "" }
func (m mockAdapter) SupportsOutputStyles() bool              { return false }
func (m mockAdapter) OutputStyleDir(_ string) string          { return "" }
func (m mockAdapter) SupportsSlashCommands() bool             { return false }
func (m mockAdapter) CommandsDir(_ string) string             { return "" }
func (m mockAdapter) SupportsSubAgents() bool                 { return false }
func (m mockAdapter) SubAgentsDir(_ string) string            { return "" }
func (m mockAdapter) EmbeddedSubAgentsDir() string            { return "" }
func (m mockAdapter) SupportsSkills() bool                    { return true }
func (m mockAdapter) SupportsSystemPrompt() bool              { return true }
func (m mockAdapter) SupportsMCP() bool                       { return true }

func TestRegistrySupportedAgentsSorted(t *testing.T) {
	r, err := NewRegistry(
		mockAdapter{agent: model.AgentOpenCode},
		mockAdapter{agent: model.AgentClaudeCode},
	)
	if err != nil {
		t.Fatalf("NewRegistry() returned error: %v", err)
	}

	if !reflect.DeepEqual(r.SupportedAgents(), []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}) {
		t.Fatalf("SupportedAgents() = %v", r.SupportedAgents())
	}
}

func TestRegistryRejectsDuplicateAgent(t *testing.T) {
	_, err := NewRegistry(
		mockAdapter{agent: model.AgentClaudeCode},
		mockAdapter{agent: model.AgentClaudeCode},
	)
	if err == nil {
		t.Fatalf("NewRegistry() expected duplicate error")
	}

	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("NewRegistry() error = %v, want ErrDuplicateAdapter", err)
	}
}

func TestFactoryReturnsMVPAdapters(t *testing.T) {
	registry, err := NewMVPRegistry()
	if err != nil {
		t.Fatalf("NewMVPRegistry() returned error: %v", err)
	}

	if _, ok := registry.Get(model.AgentClaudeCode); !ok {
		t.Fatalf("registry missing claude adapter")
	}

	if _, ok := registry.Get(model.AgentOpenCode); !ok {
		t.Fatalf("registry missing opencode adapter")
	}
}

func TestDefaultRegistryIncludesAllAgents(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() returned error: %v", err)
	}

	for _, agent := range []model.AgentID{
		model.AgentClaudeCode,
		model.AgentOpenCode,
		model.AgentGeminiCLI,
		model.AgentCursor,
		model.AgentVSCodeCopilot,
		model.AgentCodex,
		model.AgentAntigravity,
		model.AgentWindsurf,
		model.AgentQwenCode,
	} {
		if _, ok := registry.Get(agent); !ok {
			t.Fatalf("registry missing %s adapter", agent)
		}
	}
}

func TestFactoryRejectsUnsupportedAgent(t *testing.T) {
	_, err := NewAdapter(model.AgentID("unknown-agent-xyz"))
	if err == nil {
		t.Fatalf("NewAdapter() expected unsupported agent error")
	}

	if !errors.Is(err, ErrAgentNotSupported) {
		t.Fatalf("NewAdapter() error = %v, want ErrAgentNotSupported", err)
	}
}
