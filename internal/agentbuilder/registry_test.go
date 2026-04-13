package agentbuilder

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestLoadRegistry_NonExistentFile_ReturnsEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-agents.json")

	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Version != 1 {
		t.Errorf("Version = %d, want 1", reg.Version)
	}
	if len(reg.Agents) != 0 {
		t.Errorf("Agents = %v, want empty", reg.Agents)
	}
}

func TestSaveAndLoadRegistry_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-agents.json")

	original := &Registry{
		Version: 1,
		Agents: []RegistryEntry{
			{
				Name:             "my-agent",
				Title:            "My Agent",
				Description:      "Does things",
				CreatedAt:        time.Now().UTC().Truncate(time.Second),
				GenerationEngine: model.AgentClaudeCode,
				InstalledAgents:  []model.AgentID{model.AgentClaudeCode},
			},
		},
	}

	if err := SaveRegistry(path, original); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	loaded, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if loaded.Version != original.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, original.Version)
	}
	if len(loaded.Agents) != 1 {
		t.Fatalf("Agents len = %d, want 1", len(loaded.Agents))
	}
	if loaded.Agents[0].Name != "my-agent" {
		t.Errorf("Name = %q, want %q", loaded.Agents[0].Name, "my-agent")
	}
	if loaded.Agents[0].Title != "My Agent" {
		t.Errorf("Title = %q, want %q", loaded.Agents[0].Title, "My Agent")
	}
	if loaded.Agents[0].GenerationEngine != model.AgentClaudeCode {
		t.Errorf("GenerationEngine = %q, want %q", loaded.Agents[0].GenerationEngine, model.AgentClaudeCode)
	}
}

func TestRegistry_Add_EntryPresent(t *testing.T) {
	reg := &Registry{Version: 1, Agents: []RegistryEntry{}}

	entry := RegistryEntry{
		Name:  "test-agent",
		Title: "Test Agent",
	}
	reg.Add(entry)

	if len(reg.Agents) != 1 {
		t.Fatalf("Agents len = %d, want 1", len(reg.Agents))
	}
	if reg.Agents[0].Name != "test-agent" {
		t.Errorf("Name = %q, want %q", reg.Agents[0].Name, "test-agent")
	}
}

func TestRegistry_FindByName_ReturnsCorrectEntry(t *testing.T) {
	reg := &Registry{
		Version: 1,
		Agents: []RegistryEntry{
			{Name: "agent-a", Title: "Agent A"},
			{Name: "agent-b", Title: "Agent B"},
		},
	}

	found := reg.FindByName("agent-b")
	if found == nil {
		t.Fatal("expected to find 'agent-b', got nil")
	}
	if found.Title != "Agent B" {
		t.Errorf("Title = %q, want %q", found.Title, "Agent B")
	}
}

func TestRegistry_FindByName_ReturnsNilWhenNotFound(t *testing.T) {
	reg := &Registry{
		Version: 1,
		Agents: []RegistryEntry{
			{Name: "agent-a"},
		},
	}

	found := reg.FindByName("does-not-exist")
	if found != nil {
		t.Errorf("expected nil for non-existent name, got %+v", found)
	}
}

func TestRegistry_RemoveByName_EntryRemoved(t *testing.T) {
	reg := &Registry{
		Version: 1,
		Agents: []RegistryEntry{
			{Name: "keep-me"},
			{Name: "remove-me"},
		},
	}

	removed := reg.RemoveByName("remove-me")
	if !removed {
		t.Fatal("RemoveByName returned false, expected true")
	}
	if len(reg.Agents) != 1 {
		t.Fatalf("Agents len = %d, want 1", len(reg.Agents))
	}
	if reg.Agents[0].Name != "keep-me" {
		t.Errorf("remaining agent = %q, want %q", reg.Agents[0].Name, "keep-me")
	}
}

func TestRegistry_RemoveByName_NonExistentReturnsFalse(t *testing.T) {
	reg := &Registry{
		Version: 1,
		Agents: []RegistryEntry{
			{Name: "keep-me"},
		},
	}

	removed := reg.RemoveByName("does-not-exist")
	if removed {
		t.Fatal("RemoveByName returned true for non-existent entry")
	}
	if len(reg.Agents) != 1 {
		t.Errorf("Agents len = %d, want 1 (unchanged)", len(reg.Agents))
	}
}

func TestHasConflictWithBuiltin_TrueForBuiltin(t *testing.T) {
	builtins := []string{"sdd-init", "sdd-apply", "sdd-verify", "go-testing", "skill-creator", "judgment-day"}
	for _, name := range builtins {
		if !HasConflictWithBuiltin(name) {
			t.Errorf("HasConflictWithBuiltin(%q) = false, want true", name)
		}
	}
}

func TestHasConflictWithBuiltin_FalseForCustom(t *testing.T) {
	customs := []string{"my-custom-agent", "a11y-reviewer", "db-migrator", "css-linter"}
	for _, name := range customs {
		if HasConflictWithBuiltin(name) {
			t.Errorf("HasConflictWithBuiltin(%q) = true, want false", name)
		}
	}
}

func TestRegistry_VersionPreservedAcrossSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	reg := &Registry{Version: 42, Agents: []RegistryEntry{}}
	if err := SaveRegistry(path, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	loaded, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if loaded.Version != 42 {
		t.Errorf("Version = %d, want 42", loaded.Version)
	}
}
