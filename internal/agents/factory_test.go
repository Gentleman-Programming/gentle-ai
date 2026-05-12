package agents

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestFactoryResolvesPiAdapter(t *testing.T) {
	adapter, err := NewAdapter(model.AgentPi)
	if err != nil {
		t.Fatalf("NewAdapter(%q) returned error: %v", model.AgentPi, err)
	}

	if got := adapter.Agent(); got != model.AgentPi {
		t.Fatalf("adapter.Agent() = %q, want %q", got, model.AgentPi)
	}
}

func TestDefaultRegistryIncludesPi(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() returned error: %v", err)
	}

	adapter, ok := registry.Get(model.AgentPi)
	if !ok {
		t.Fatalf("registry missing %s adapter", model.AgentPi)
	}

	if got := adapter.Agent(); got != model.AgentPi {
		t.Fatalf("registry adapter.Agent() = %q, want %q", got, model.AgentPi)
	}
}

func TestDefaultRegistrySupportedAgentsMatchesFactoryAgents(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() returned error: %v", err)
	}

	want := []model.AgentID{
		model.AgentAntigravity,
		model.AgentClaudeCode,
		model.AgentCodex,
		model.AgentCursor,
		model.AgentGeminiCLI,
		model.AgentKilocode,
		model.AgentKimi,
		model.AgentKiroIDE,
		model.AgentOpenClaw,
		model.AgentOpenCode,
		model.AgentPi,
		model.AgentQwenCode,
		model.AgentVSCodeCopilot,
		model.AgentWindsurf,
	}

	if got := registry.SupportedAgents(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedAgents() = %v, want %v", got, want)
	}
}

func TestFactoryRejectsUnsupportedOpenClawLookalike(t *testing.T) {
	_, err := NewAdapter(model.AgentID("openclaw-beta"))
	if err == nil {
		t.Fatalf("NewAdapter() expected unsupported agent error")
	}

	if !errors.Is(err, ErrAgentNotSupported) {
		t.Fatalf("NewAdapter() error = %v, want ErrAgentNotSupported", err)
	}
}
