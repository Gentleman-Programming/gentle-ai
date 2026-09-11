package agentbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestMain(m *testing.M) { saveRegistry = SaveRegistry; os.Exit(m.Run()) }

type uninstallCtx struct {
	registryPath string
	lookupName   string
	paths        map[string][]string
}

func TestUninstall_SuccessScenarios(t *testing.T) {
	tests := []struct {
		name, lookupName string
		entries          []RegistryEntry
		owned            map[string][]model.AgentID
		setup            func(*uninstallCtx)
		wantRemoved      []string
		wantRemaining    []string
		wantSkipped      []model.AgentID
		wantAbsent       []string
		wantPresent      []string
	}{
		{"removes owned skill files for installed agents", "css-a11y-reviewer", []RegistryEntry{entry("css-a11y-reviewer", model.AgentClaudeCode, model.AgentOpenCode, model.AgentGeminiCLI, model.AgentCodex)}, map[string][]model.AgentID{"owned": {model.AgentClaudeCode, model.AgentOpenCode, model.AgentGeminiCLI, model.AgentCodex}}, nil, []string{"owned"}, nil, nil, nil, nil},
		{"uses persisted registry name exactly", "reviewer-custom", []RegistryEntry{entry("reviewer-custom", model.AgentOpenCode)}, map[string][]model.AgentID{"owned": {model.AgentOpenCode}, "other": {model.AgentOpenCode}}, nil, []string{"owned"}, []string{"other"}, nil, nil, nil},
		{"removes registry entry after success", "remove-me", []RegistryEntry{entry("remove-me", model.AgentClaudeCode), entry("keep-me", model.AgentOpenCode)}, map[string][]model.AgentID{"owned": {model.AgentClaudeCode}}, nil, nil, nil, nil, []string{"remove-me"}, []string{"keep-me"}},
		{"removes first agent when multiple exist without mutating pointer", "first-agent", []RegistryEntry{entry("first-agent", model.AgentClaudeCode), entry("other-agent", model.AgentOpenCode)}, map[string][]model.AgentID{"owned": {model.AgentClaudeCode}, "other-agent": {model.AgentOpenCode}}, nil, []string{"owned"}, []string{"other-agent"}, nil, []string{"first-agent"}, []string{"other-agent"}},
		{"missing owned file does not fail", "missing-file-agent", []RegistryEntry{entry("missing-file-agent", model.AgentClaudeCode)}, nil, nil, nil, nil, nil, nil, nil},
		{"existing skill dir without skill file reports no removed paths", "empty-dir-agent", []RegistryEntry{entry("empty-dir-agent", model.AgentClaudeCode)}, nil, func(ctx *uninstallCtx) {
			skillsDir := supportedSkillsDirs(filepath.Dir(ctx.registryPath))[model.AgentClaudeCode]
			_ = os.MkdirAll(filepath.Join(skillsDir, "empty-dir-agent"), 0755)
		}, nil, nil, nil, []string{"empty-dir-agent"}, nil},
		{"unknown installed agent is skipped safely", "unknown-agent-reviewer", []RegistryEntry{entry("unknown-agent-reviewer", model.AgentClaudeCode, model.AgentID("unknown-agent"))}, map[string][]model.AgentID{"owned": {model.AgentClaudeCode}}, nil, []string{"owned"}, nil, []model.AgentID{model.AgentID("unknown-agent")}, nil, nil},
		{"does not delete parent or shared directories", "agent-a", []RegistryEntry{entry("agent-a", model.AgentOpenCode)}, map[string][]model.AgentID{"owned": {model.AgentOpenCode}, "shared": {model.AgentOpenCode}}, func(ctx *uninstallCtx) {
			sharedDir := filepath.Dir(ctx.paths["shared"][0])
			ctx.paths["shared-dir"] = []string{sharedDir}
			ctx.paths["parent-dir"] = []string{filepath.Dir(sharedDir)}
		}, nil, []string{"parent-dir", "shared-dir", "shared"}, nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			ctx := uninstallCtx{registryPath: writeRegistryForUninstall(t, home, tt.entries...), lookupName: tt.lookupName, paths: map[string][]string{}}
			for key, agents := range tt.owned {
				ctx.paths[key] = createOwnedSkillFiles(t, home, ownedName(key, tt.lookupName), agents)
			}
			if tt.setup != nil {
				tt.setup(&ctx)
			}
			result, err := Uninstall(ctx.registryPath, ctx.lookupName, home)
			assertUninstallOK(t, err)
			assertAgentIDs(t, result.SkippedAgents, tt.wantSkipped)
			assertPathSets(t, ctx.paths, false, tt.wantRemoved...)
			assertPathSets(t, ctx.paths, true, tt.wantRemaining...)
			if removed := countPaths(ctx.paths, tt.wantRemoved...); removed > 0 && len(result.RemovedPaths) != removed {
				t.Fatalf("RemovedPaths len = %d, want %d", len(result.RemovedPaths), removed)
			}
			reg := loadRegistryForTest(t, ctx.registryPath)
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
		setup     func(t *testing.T, home string) uninstallCtx
		stubSave  func(string, *Registry) error
		wantErr   string
		assertion func(t *testing.T, ctx uninstallCtx)
	}{
		{
			name: "save registry failure preserves registry entry on disk while files removed",
			setup: func(t *testing.T, home string) uninstallCtx {
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry("save-failure-agent", model.AgentClaudeCode, model.AgentOpenCode)),
					lookupName:   "save-failure-agent",
					paths:        map[string][]string{"owned": createOwnedSkillFiles(t, home, "save-failure-agent", []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode})},
				}
			},
			stubSave: func(string, *Registry) error { return fmt.Errorf("boom") },
			wantErr:  "uninstall: save registry: boom",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				assertPathSets(t, ctx.paths, false, "owned")
				if loadRegistryForTest(t, ctx.registryPath).FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry to remain on disk after save failure")
				}
			},
		},
		{
			name: "path traversal name is rejected",
			setup: func(t *testing.T, home string) uninstallCtx {
				outside := filepath.Join(home, "escaped-target", "SKILL.md")
				writeSkillFile(t, outside, "# Outside\n")
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry("../escaped-target", model.AgentOpenCode)),
					lookupName:   "../escaped-target",
					paths:        map[string][]string{"outside": {outside}},
				}
			},
			wantErr: "uninstall: invalid registry entry name ",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				assertPathSets(t, ctx.paths, true, "outside")
				reg := loadRegistryForTest(t, ctx.registryPath)
				if reg.FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry unmutated when validation fails")
				}
			},
		},
		{
			name: "path traversal name with empty installed agents is rejected",
			setup: func(t *testing.T, home string) uninstallCtx {
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry("../escaped-target")),
					lookupName:   "../escaped-target",
				}
			},
			wantErr: "uninstall: invalid registry entry name ",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				reg := loadRegistryForTest(t, ctx.registryPath)
				if reg.FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry unmutated when validation fails")
				}
			},
		},
		{
			name: "path traversal name with unsupported installed agents is rejected",
			setup: func(t *testing.T, home string) uninstallCtx {
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry("../escaped-target", model.AgentID("unsupported-agent"))),
					lookupName:   "../escaped-target",
				}
			},
			wantErr: "uninstall: invalid registry entry name ",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				reg := loadRegistryForTest(t, ctx.registryPath)
				if reg.FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry unmutated when validation fails")
				}
			},
		},
		{
			name: "file removal failure preserves registry entry",
			setup: func(t *testing.T, home string) uninstallCtx {
				skillsDir := supportedSkillsDirs(home)[model.AgentOpenCode]
				skillDir := filepath.Join(skillsDir, "blocked-agent")
				skillFile := filepath.Join(skillDir, "SKILL.md")
				if err := os.MkdirAll(filepath.Join(skillFile, "nested"), 0755); err != nil {
					t.Fatalf("setup non-empty dir blocker: %v", err)
				}
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry("blocked-agent", model.AgentOpenCode)),
					lookupName:   "blocked-agent",
					paths:        map[string][]string{"blocked": {skillFile}},
				}
			},
			wantErr: "uninstall: remove ",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				reg := loadRegistryForTest(t, ctx.registryPath)
				if reg.FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry to remain untouched after file removal failure")
				}
			},
		},
		{
			name: "absolute path name is rejected",
			setup: func(t *testing.T, home string) uninstallCtx {
				name := filepath.Join(home, "absolute-target")
				outside := filepath.Join(name, "SKILL.md")
				writeSkillFile(t, outside, "# Absolute\n")
				return uninstallCtx{
					registryPath: writeRegistryForUninstall(t, home, entry(name, model.AgentOpenCode)),
					lookupName:   name,
					paths:        map[string][]string{"outside": {outside}},
				}
			},
			wantErr: "uninstall: invalid registry entry name ",
			assertion: func(t *testing.T, ctx uninstallCtx) {
				assertPathSets(t, ctx.paths, true, "outside")
				reg := loadRegistryForTest(t, ctx.registryPath)
				if reg.FindByName(ctx.lookupName) == nil {
					t.Fatal("expected registry entry unmutated when validation fails")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			ctx := tt.setup(t, home)
			originalSave := saveRegistry
			if tt.stubSave != nil {
				t.Cleanup(func() { saveRegistry = originalSave })
				saveRegistry = tt.stubSave
			}
			result, err := Uninstall(ctx.registryPath, ctx.lookupName, home)
			if err == nil {
				t.Fatal("expected uninstall error, got nil")
			}
			if tt.wantErr == "uninstall: invalid registry entry name " || tt.wantErr == "uninstall: remove " {
				if got := err.Error(); len(got) < len(tt.wantErr) || got[:len(tt.wantErr)] != tt.wantErr {
					t.Fatalf("expected error prefix %q, got %v", tt.wantErr, err)
				}
			} else if err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
			if tt.name != "save registry failure preserves registry entry on disk while files removed" && len(result.RemovedPaths) != 0 {
				t.Fatalf("RemovedPaths = %v, want empty", result.RemovedPaths)
			}
			assertAgentIDs(t, result.SkippedAgents, nil)
			tt.assertion(t, ctx)
		})
	}
}

func entry(name string, agents ...model.AgentID) RegistryEntry {
	return RegistryEntry{Name: name, InstalledAgents: agents}
}

func ownedName(key, lookup string) string {
	if key == "other" {
		return "reviewer"
	}
	if key == "other-agent" {
		return "other-agent"
	}
	if key == "shared" {
		return "agent-b"
	}
	return lookup
}

func assertUninstallOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}
}

func assertAgentIDs(t *testing.T, got, want []model.AgentID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("SkippedAgents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SkippedAgents = %v, want %v", got, want)
		}
	}
}

func assertPathSets(t *testing.T, paths map[string][]string, exists bool, keys ...string) {
	t.Helper()
	for _, key := range keys {
		for _, path := range paths[key] {
			if exists {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("expected path to remain %s: %v", path, err)
				}
				continue
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("expected removed path %s, stat err = %v", path, err)
			}
		}
	}
}

func countPaths(paths map[string][]string, keys ...string) int {
	total := 0
	for _, key := range keys {
		total += len(paths[key])
	}
	return total
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

func TestUninstall_SymlinkedSkillDirDoesNotDeleteOutsideTarget(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "outside")
	outsideSkill := filepath.Join(outside, "SKILL.md")
	writeSkillFile(t, outsideSkill, "# Outside Skill\n")

	skillsDir := supportedSkillsDirs(home)[model.AgentOpenCode]
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("MkdirAll skillsDir: %v", err)
	}
	skillLink := filepath.Join(skillsDir, "symlink-agent")
	if err := os.Symlink(outside, skillLink); err != nil {
		t.Fatalf("Symlink %s -> %s: %v", skillLink, outside, err)
	}

	regPath := writeRegistryForUninstall(t, home, entry("symlink-agent", model.AgentOpenCode))
	result, err := Uninstall(regPath, "symlink-agent", home)
	assertUninstallOK(t, err)

	data, err := os.ReadFile(outsideSkill)
	if err != nil {
		t.Fatalf("expected outside SKILL.md to remain intact, got err = %v", err)
	}
	if string(data) != "# Outside Skill\n" {
		t.Fatalf("outside SKILL.md content altered: %q", string(data))
	}

	if _, err := os.Lstat(skillLink); !os.IsNotExist(err) {
		t.Fatalf("expected symlink %s to be removed, got err = %v", skillLink, err)
	}

	reg := loadRegistryForTest(t, regPath)
	if reg.FindByName("symlink-agent") != nil {
		t.Fatal("expected registry entry removed")
	}

	if len(result.RemovedPaths) != 1 || result.RemovedPaths[0] != skillLink {
		t.Fatalf("RemovedPaths = %v, want [%s]", result.RemovedPaths, skillLink)
	}
}
