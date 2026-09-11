package agentbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestMain(m *testing.M) { saveRegistry = SaveRegistry; os.Exit(m.Run()) }

func TestUninstall_SuccessScenarios(t *testing.T) {
	all := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentGeminiCLI, model.AgentCodex}
	tests := []struct {
		name        string
		lookup      string
		entries     []RegistryEntry
		files       map[string][]model.AgentID
		setup       func(t *testing.T, home string)
		wantRemoved int
		wantSkipped []model.AgentID
		wantAbsent  []string
		wantPresent []string
	}{
		{
			name:   "removes owned skill files for installed agents",
			lookup: "css-a11y-reviewer",
			entries: []RegistryEntry{
				entry("css-a11y-reviewer", all...),
			},
			files:       map[string][]model.AgentID{"css-a11y-reviewer": all},
			wantRemoved: 4,
			wantAbsent:  []string{"css-a11y-reviewer"},
		},
		{
			name:   "uses persisted registry name exactly",
			lookup: "reviewer-custom",
			entries: []RegistryEntry{
				entry("reviewer-custom", model.AgentOpenCode),
			},
			files: map[string][]model.AgentID{
				"reviewer-custom": {model.AgentOpenCode},
				"reviewer":        {model.AgentOpenCode},
			},
			wantRemoved: 1,
			wantAbsent:  []string{"reviewer-custom"},
		},
		{
			name:   "removes registry entry after success",
			lookup: "remove-me",
			entries: []RegistryEntry{
				entry("remove-me", model.AgentClaudeCode),
				entry("keep-me", model.AgentOpenCode),
			},
			files:       map[string][]model.AgentID{"remove-me": {model.AgentClaudeCode}},
			wantRemoved: 1,
			wantAbsent:  []string{"remove-me"},
			wantPresent: []string{"keep-me"},
		},
		{
			name:   "removes first agent when multiple exist without mutating pointer",
			lookup: "first-agent",
			entries: []RegistryEntry{
				entry("first-agent", model.AgentClaudeCode),
				entry("other-agent", model.AgentOpenCode),
			},
			files: map[string][]model.AgentID{
				"first-agent": {model.AgentClaudeCode},
				"other-agent": {model.AgentOpenCode},
			},
			wantRemoved: 1,
			wantAbsent:  []string{"first-agent"},
			wantPresent: []string{"other-agent"},
		},
		{
			name:       "missing owned file does not fail",
			lookup:     "missing-file-agent",
			entries:    []RegistryEntry{entry("missing-file-agent", model.AgentClaudeCode)},
			wantAbsent: []string{"missing-file-agent"},
		},
		{
			name:    "existing skill dir without skill file reports no removed paths",
			lookup:  "empty-dir-agent",
			entries: []RegistryEntry{entry("empty-dir-agent", model.AgentClaudeCode)},
			setup: func(t *testing.T, home string) {
				_ = os.MkdirAll(filepath.Join(supportedSkillsDirs(home)[model.AgentClaudeCode], "empty-dir-agent"), 0755)
			},
			wantAbsent: []string{"empty-dir-agent"},
		},
		{
			name:   "unknown installed agent is skipped safely",
			lookup: "unknown-agent-reviewer",
			entries: []RegistryEntry{
				entry("unknown-agent-reviewer", model.AgentClaudeCode, "unknown-agent"),
			},
			files:       map[string][]model.AgentID{"unknown-agent-reviewer": {model.AgentClaudeCode}},
			wantRemoved: 1,
			wantSkipped: []model.AgentID{"unknown-agent"},
			wantAbsent:  []string{"unknown-agent-reviewer"},
		},
		{
			name:   "does not delete parent or shared directories",
			lookup: "agent-a",
			entries: []RegistryEntry{
				entry("agent-a", model.AgentOpenCode),
			},
			files: map[string][]model.AgentID{
				"agent-a": {model.AgentOpenCode},
				"agent-b": {model.AgentOpenCode},
			},
			wantRemoved: 1,
			wantAbsent:  []string{"agent-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			regPath := writeRegistryForUninstall(t, home, tt.entries...)
			var removedExpected, remainingExpected []string
			for name, agents := range tt.files {
				paths := createOwnedSkillFiles(t, home, name, agents)
				if name == tt.lookup {
					removedExpected = append(removedExpected, paths...)
				} else {
					remainingExpected = append(remainingExpected, paths...)
				}
			}
			if tt.setup != nil {
				tt.setup(t, home)
			}

			result, err := Uninstall(regPath, tt.lookup, home)
			if err != nil {
				t.Fatalf("Uninstall error: %v", err)
			}
			if len(result.RemovedPaths) != tt.wantRemoved {
				t.Fatalf("RemovedPaths len = %d, want %d", len(result.RemovedPaths), tt.wantRemoved)
			}
			if len(result.SkippedAgents) != len(tt.wantSkipped) {
				t.Fatalf("SkippedAgents = %v, want %v", result.SkippedAgents, tt.wantSkipped)
			}
			for _, path := range removedExpected {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected removed path %s", path)
				}
			}
			for _, path := range remainingExpected {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("expected remaining path %s: %v", path, err)
				}
			}
			reg := loadRegistryForTest(t, regPath)
			for _, name := range tt.wantAbsent {
				if reg.FindByName(name) != nil {
					t.Fatalf("expected registry entry %q absent", name)
				}
			}
			for _, name := range tt.wantPresent {
				if reg.FindByName(name) == nil {
					t.Fatalf("expected registry entry %q present", name)
				}
			}
		})
	}
}

func TestUninstall_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
		agents    []model.AgentID
		stubSave  bool
		blockFile bool
		outside   bool
		wantErr   string
	}{
		{
			name:      "save registry failure preserves registry entry on disk while files removed",
			agentName: "save-failure-agent",
			agents:    []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode},
			stubSave:  true,
			wantErr:   "uninstall: save registry: boom",
		},
		{
			name:      "path traversal name is rejected",
			agentName: "../escaped-target",
			agents:    []model.AgentID{model.AgentOpenCode},
			outside:   true,
			wantErr:   "uninstall: invalid registry entry name ",
		},
		{
			name:      "path traversal name with empty installed agents is rejected",
			agentName: "../escaped-target",
			wantErr:   "uninstall: invalid registry entry name ",
		},
		{
			name:      "path traversal name with unsupported installed agents is rejected",
			agentName: "../escaped-target",
			agents:    []model.AgentID{"unsupported-agent"},
			wantErr:   "uninstall: invalid registry entry name ",
		},
		{
			name:      "file removal failure preserves registry entry",
			agentName: "blocked-agent",
			agents:    []model.AgentID{model.AgentOpenCode},
			blockFile: true,
			wantErr:   "uninstall: remove ",
		},
		{
			name:      "absolute path name is rejected",
			agentName: "absolute-target",
			agents:    []model.AgentID{model.AgentOpenCode},
			outside:   true,
			wantErr:   "uninstall: invalid registry entry name ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			name := tt.agentName
			var outsidePath string
			if tt.name == "absolute path name is rejected" {
				name = filepath.Join(home, tt.agentName)
			}
			if tt.outside {
				outsidePath = filepath.Join(home, filepath.Base(tt.agentName), "SKILL.md")
				writeSkillFile(t, outsidePath, "# Outside\n")
			}

			regPath := writeRegistryForUninstall(t, home, entry(name, tt.agents...))
			var ownedFiles []string
			if len(tt.agents) > 0 && !tt.outside && !tt.blockFile {
				ownedFiles = createOwnedSkillFiles(t, home, name, tt.agents)
			}
			if tt.blockFile {
				skillFile := filepath.Join(supportedSkillsDirs(home)[model.AgentOpenCode], name, "SKILL.md")
				if err := os.MkdirAll(filepath.Join(skillFile, "nested"), 0755); err != nil {
					t.Fatalf("setup non-empty dir blocker: %v", err)
				}
			}
			if tt.stubSave {
				orig := saveRegistry
				t.Cleanup(func() { saveRegistry = orig })
				saveRegistry = func(string, *Registry) error { return fmt.Errorf("boom") }
			}

			result, err := Uninstall(regPath, name, home)
			if err == nil {
				t.Fatal("expected uninstall error, got nil")
			}
			if !strings.HasPrefix(err.Error(), tt.wantErr) {
				t.Fatalf("expected error prefix %q, got %v", tt.wantErr, err)
			}
			if !tt.stubSave && len(result.RemovedPaths) != 0 {
				t.Fatalf("RemovedPaths = %v, want empty", result.RemovedPaths)
			}
			if len(result.SkippedAgents) != 0 {
				t.Fatalf("SkippedAgents = %v, want empty", result.SkippedAgents)
			}
			if loadRegistryForTest(t, regPath).FindByName(name) == nil {
				t.Fatal("expected registry entry unmutated when validation or removal fails")
			}
			if tt.stubSave {
				for _, f := range ownedFiles {
					if _, err := os.Stat(f); !os.IsNotExist(err) {
						t.Fatalf("expected file removed: %s", f)
					}
				}
			}
			if outsidePath != "" {
				if _, err := os.Stat(outsidePath); err != nil {
					t.Fatalf("expected outside file to remain: %v", err)
				}
			}
		})
	}
}

func TestUninstall_SymlinkedSkillDirDoesNotDeleteOutsideTarget(t *testing.T) {
	home := t.TempDir()
	outsideSkill := filepath.Join(home, "outside", "SKILL.md")
	writeSkillFile(t, outsideSkill, "# Outside Skill\n")

	skillsDir := supportedSkillsDirs(home)[model.AgentOpenCode]
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("MkdirAll skillsDir: %v", err)
	}
	skillLink := filepath.Join(skillsDir, "symlink-agent")
	if err := os.Symlink(filepath.Dir(outsideSkill), skillLink); err != nil {
		t.Fatalf("Symlink %s: %v", skillLink, err)
	}

	regPath := writeRegistryForUninstall(t, home, entry("symlink-agent", model.AgentOpenCode))
	result, err := Uninstall(regPath, "symlink-agent", home)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	data, err := os.ReadFile(outsideSkill)
	if err != nil || string(data) != "# Outside Skill\n" {
		t.Fatalf("outside SKILL.md altered or missing: %v, %q", err, string(data))
	}
	if _, err := os.Lstat(skillLink); !os.IsNotExist(err) {
		t.Fatalf("expected symlink %s removed, got err = %v", skillLink, err)
	}
	if loadRegistryForTest(t, regPath).FindByName("symlink-agent") != nil {
		t.Fatal("expected registry entry removed")
	}
	if len(result.RemovedPaths) != 1 || result.RemovedPaths[0] != skillLink {
		t.Fatalf("RemovedPaths = %v, want [%s]", result.RemovedPaths, skillLink)
	}
}

func entry(name string, agents ...model.AgentID) RegistryEntry {
	return RegistryEntry{Name: name, InstalledAgents: agents}
}

func loadRegistryForTest(t *testing.T, registryPath string) *Registry {
	t.Helper()
	reg, err := LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}
	return reg
}

func writeRegistryForUninstall(t *testing.T, home string, entries ...RegistryEntry) string {
	t.Helper()
	registryPath := filepath.Join(home, "custom-agents.json")
	registry := &Registry{Version: 1, Agents: make([]RegistryEntry, 0, len(entries))}
	for _, item := range entries {
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now().UTC().Truncate(time.Second)
		}
		if item.GenerationEngine == "" {
			item.GenerationEngine = model.AgentClaudeCode
		}
		registry.Add(item)
	}
	if err := SaveRegistry(registryPath, registry); err != nil {
		t.Fatalf("SaveRegistry error: %v", err)
	}
	return registryPath
}

func createOwnedSkillFiles(t *testing.T, home, agentName string, installedAgents []model.AgentID) []string {
	t.Helper()
	skillsDirs, paths := supportedSkillsDirs(home), make([]string, 0, len(installedAgents))
	for _, agentID := range installedAgents {
		skillsDir, ok := skillsDirs[agentID]
		if !ok {
			continue
		}
		path := filepath.Join(skillsDir, agentName, "SKILL.md")
		writeSkillFile(t, path, "# Test\n")
		paths = append(paths, path)
	}
	return paths
}

func writeSkillFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
