package skills

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// skillFiles drops the support files (skills/_shared references and skill
// commands) so assertions can count only the per-skill files.
func skillFiles(files []string) []string {
	var out []string
	sep := string(filepath.Separator)
	for _, file := range files {
		if strings.Contains(file, sep+sharedDirName+sep) || strings.Contains(file, sep+"commands"+sep) {
			continue
		}
		out = append(out, file)
	}
	return out
}

func TestInjectWritesSupportFiles(t *testing.T) {
	shared, err := assets.SharedSkillFileNames()
	if err != nil {
		t.Fatal(err)
	}
	shared = slices.DeleteFunc(shared, func(name string) bool { return internalSharedSources[name] })
	for _, tt := range []struct {
		agent        model.AgentID
		wantCommands []string
	}{
		{agent: model.AgentOpenCode, wantCommands: []string{"skill-creator.md"}},
		{agent: model.AgentQwenCode, wantCommands: []string{"skill-creator.md"}},
		{agent: model.AgentClaudeCode},
		{agent: model.AgentCodex},
	} {
		t.Run(string(tt.agent), func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(tt.agent)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(adapter.SkillsDir(home), sharedDirName), 0o755); err != nil {
				t.Fatal(err)
			}
			marker := LegacySharedMarkerPath(adapter.SkillsDir(home))
			if err := os.WriteFile(marker, []byte("legacy"), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := Inject(home, adapter, []model.SkillID{model.SkillCreator, model.SkillJudgmentDay})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range shared {
				path := filepath.Join(adapter.SkillsDir(home), sharedDirName, name)
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("shared reference %s missing: %v", name, err)
				}
				if strings.Contains(string(content), runtimeAgentIDPlaceholder) {
					t.Fatalf("shared reference %s keeps the runtime placeholder", name)
				}
			}
			if _, err := os.Stat(filepath.Join(adapter.SkillsDir(home), sharedDirName, "odd-orchestrator-sections.md")); !os.IsNotExist(err) {
				t.Fatalf("internal odd-orchestrator-sections.md installed: %v", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("legacy _shared/SKILL.md marker not removed: %v", err)
			}
			commands, err := SkillCommandPaths(home, adapter, []model.SkillID{model.SkillCreator, model.SkillJudgmentDay})
			if err != nil {
				t.Fatal(err)
			}
			if len(commands) != len(tt.wantCommands) {
				t.Fatalf("commands = %v, want %v", commands, tt.wantCommands)
			}
			for i, name := range tt.wantCommands {
				if filepath.Base(commands[i]) != name {
					t.Fatalf("commands = %v, want %v", commands, tt.wantCommands)
				}
				if _, err := os.Stat(commands[i]); err != nil {
					t.Fatalf("command %s missing: %v", name, err)
				}
			}
			declared, err := SupportFilePaths(home, adapter, []model.SkillID{model.SkillCreator, model.SkillJudgmentDay})
			if err != nil {
				t.Fatal(err)
			}
			written := map[string]bool{}
			for _, file := range result.Files {
				written[file] = true
			}
			for _, path := range declared {
				if !written[path] {
					t.Fatalf("declared support path %s was not reported as written", path)
				}
			}

			again, err := Inject(home, adapter, []model.SkillID{model.SkillCreator, model.SkillJudgmentDay})
			if err != nil || again.Changed {
				t.Fatalf("second inject changed=%v err=%v", again.Changed, err)
			}
		})
	}
}

func TestInjectDirectoryWithWriterBindsGenericRuntimeSlot(t *testing.T) {
	skillDir := t.TempDir()
	result, err := InjectDirectoryWithWriter(skillDir, []model.SkillID{model.SkillCreator}, filemerge.WriteFileAtomic, removeRegularTestFile)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := os.ReadFile(filepath.Join(skillDir, sharedDirName, "review-ledger-contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ledger), "--agent "+compatibilityRuntimeSlot) {
		t.Fatal("compatibility review ledger contract is not bound to the generic runtime slot")
	}
	declared, err := SharedReferencePaths(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range declared {
		if !slices.Contains(result.Files, path) {
			t.Fatalf("declared shared reference %s was not written: %v", path, result.Files)
		}
	}
}

func removeRegularTestFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	return true, os.Remove(path)
}

// TestInjectDirectoryWithWriterRemovesLegacySharedMarker proves the shared
// compatibility operation, used by every transaction implementation, removes
// the obsolete _shared/SKILL.md through the caller's remover (#4471).
func TestInjectDirectoryWithWriterRemovesLegacySharedMarker(t *testing.T) {
	skillDir := t.TempDir()
	marker := LegacySharedMarkerPath(skillDir)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("legacy generated marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	var removals []string
	remove := func(path string) (bool, error) {
		removals = append(removals, path)
		return removeRegularTestFile(path)
	}

	result, err := InjectDirectoryWithWriter(skillDir, []model.SkillID{model.SkillCreator}, filemerge.WriteFileAtomic, remove)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removals, []string{marker}) {
		t.Fatalf("remover calls = %v, want only the legacy marker %s", removals, marker)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("legacy marker still present: %v", err)
	}
	if !result.Changed || !slices.Contains(result.Files, marker) {
		t.Fatalf("result changed=%v files=%v, want the removed marker reported", result.Changed, result.Files)
	}

	again, err := InjectDirectoryWithWriter(skillDir, []model.SkillID{model.SkillCreator}, filemerge.WriteFileAtomic, remove)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed || slices.Contains(again.Files, marker) {
		t.Fatalf("second refresh changed=%v files=%v, want idempotent", again.Changed, again.Files)
	}
}
