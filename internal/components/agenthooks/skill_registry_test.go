package agenthooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestSkillRegistryPreservesExistingClaudeHooks(t *testing.T) {
	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	path := adapter.SettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo keep"}]}],"UserPromptSubmit":[{"matcher":"startup","hooks":[{"type":"command","command":"echo existing"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := InstallSkillRegistry(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first install: %+v %v", first, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"echo keep", "echo existing"} {
		if !strings.Contains(string(before), expected) {
			t.Fatalf("existing hook %q lost: %s", expected, before)
		}
	}
	if strings.Count(string(before), "gentle-ai skill-registry refresh") != 1 {
		t.Fatalf("duplicate refresh: %s", before)
	}
	second, err := InstallSkillRegistry(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second install: %+v %v", second, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("repeat changed bytes: %v", err)
	}
}

func TestSkillRegistryRejectsSymlinkWithoutChangingTarget(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentCodex, model.AgentClaudeCode} {
		t.Run(string(agent), func(t *testing.T) {
			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			home := t.TempDir()
			path := adapter.SettingsPath(home)
			if agent == model.AgentCodex {
				path = filepath.Join(adapter.GlobalConfigDir(home), "hooks.json")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "target.json")
			const sentinel = `{"hooks":{}}`
			if err := os.WriteFile(target, []byte(sentinel), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if result, err := InstallSkillRegistry(home, adapter); err == nil || result.Changed {
				t.Fatalf("symlink accepted: %+v %v", result, err)
			}
			if after, err := os.ReadFile(target); err != nil || string(after) != sentinel {
				t.Fatalf("target changed: %q %v", after, err)
			}
		})
	}
}

func TestSkillRegistryHooksWithoutSDD(t *testing.T) {
	for _, tc := range []struct {
		agent         model.AgentID
		file, command string
	}{
		{model.AgentCodex, "hooks.json", `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true`},
		{model.AgentClaudeCode, "settings.json", `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			home := t.TempDir()
			adapter, err := agents.NewAdapter(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			result, err := InstallSkillRegistry(home, adapter)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Changed || len(result.Files) != 1 {
				t.Fatalf("first install: %+v", result)
			}
			content, err := os.ReadFile(result.Files[0])
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(content, &document); err != nil {
				t.Fatal(err)
			}
			if filepath.Base(result.Files[0]) != tc.file || !strings.Contains(string(content), strings.ReplaceAll(tc.command, `"`, `\"`)) {
				t.Fatalf("unexpected hook: %s", content)
			}
			again, err := InstallSkillRegistry(home, adapter)
			if err != nil {
				t.Fatal(err)
			}
			if again.Changed {
				t.Fatalf("not idempotent: %+v", again)
			}
			if tc.agent == model.AgentCodex {
				if !strings.Contains(string(content), `startup|resume|clear|compact`) || !strings.Contains(string(content), `"SessionStart"`) {
					t.Fatalf("Codex startup sources missing: %s", content)
				}
			}
			for _, malformed := range []string{`{"hooks":[]}`, `{"hooks":{"` + map[model.AgentID]string{model.AgentCodex: "SessionStart", model.AgentClaudeCode: "UserPromptSubmit"}[tc.agent] + `":{}}}`, `{"permissions":`} {
				if err := os.WriteFile(result.Files[0], []byte(malformed), 0o644); err != nil {
					t.Fatal(err)
				}
				if got, err := InstallSkillRegistry(home, adapter); err == nil || got.Changed {
					t.Fatalf("accepted malformed hook settings %q: %+v %v", malformed, got, err)
				}
				if after, err := os.ReadFile(result.Files[0]); err != nil || string(after) != malformed {
					t.Fatalf("malformed settings changed: %q %v", after, err)
				}
			}
		})
	}
}
