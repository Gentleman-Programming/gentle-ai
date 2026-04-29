package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestInjectPIAgentsPromptUsesConcreteModelIDs(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(workspace, ".pi", "extensions", "pi-subagents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(pi-subagents) error = %v", err)
	}

	assignments := map[string]model.ModelAssignment{
		"sdd-design":  {ProviderID: "anthropic", ModelID: "claude-opus-4-1"},
		"sdd-archive": {ProviderID: "anthropic", ModelID: "claude-haiku-3-5-20241022"},
		"default":     {ProviderID: "anthropic", ModelID: "claude-sonnet-4-20250514"},
	}

	_, err := Inject(home, pi.NewAdapter(), model.SDDModeMulti, InjectOptions{
		WorkspaceDir:             workspace,
		OpenCodeModelAssignments: assignments,
	})
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	agentsPath := filepath.Join(home, ".pi", "agent", "AGENTS.md")
	raw, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	text := string(raw)

	for _, alias := range []string{"| orchestrator | opus |", "| sdd-explore | sonnet |", "| sdd-archive | haiku |"} {
		if strings.Contains(text, alias) {
			t.Fatalf("AGENTS.md contains Claude alias row %q; want concrete model IDs", alias)
		}
	}

	for _, expected := range []string{
		"| orchestrator | anthropic/claude-sonnet-4-20250514 |",
		"| sdd-design | anthropic/claude-opus-4-1 |",
		"| sdd-archive | anthropic/claude-haiku-3-5-20241022 |",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("AGENTS.md missing concrete model assignment row %q", expected)
		}
	}
}
