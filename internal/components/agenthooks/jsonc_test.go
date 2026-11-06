package agenthooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type hookWriter struct {
	name    string
	agent   model.AgentID
	command string
	install func(string, agents.Adapter) (Result, error)
}

func hookWriters() []hookWriter {
	return []hookWriter{
		{"claude retained hooks", model.AgentClaudeCode, "gentle-ai review stop-hook --agent claude-code", InstallRetainedClaudeHooks},
		{"claude skill registry", model.AgentClaudeCode, "gentle-ai skill-registry refresh", InstallSkillRegistry},
		{"codex skill registry", model.AgentCodex, "gentle-ai skill-registry refresh", InstallSkillRegistry},
		{"codex telemetry", model.AgentCodex, "gentle-ai telemetry runtime codex --json", InstallCodexTelemetry},
	}
}

func hookSettingsPath(t *testing.T, home string, agent model.AgentID) (agents.Adapter, string) {
	t.Helper()
	adapter, err := agents.NewAdapter(agent)
	if err != nil {
		t.Fatal(err)
	}
	path := adapter.SettingsPath(home)
	if agent == model.AgentCodex {
		path = filepath.Join(adapter.GlobalConfigDir(home), "hooks.json")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return adapter, path
}

func TestHookWritersPreserveJSONCSettings(t *testing.T) {
	const prefix = "{\n  // Personal settings.\n  \"model\": \"opus\", /* chosen by hand */\n  \"permissions\": {\n    \"allow\": [\"Bash(ls)\",], // trailing comma kept\n  },\n"
	seeds := map[string]string{
		"hooks present": prefix + "  \"hooks\": {\n    \"PreToolUse\": [{\"matcher\": \"Bash\", \"hooks\": [{\"type\": \"command\", \"command\": \"echo keep\"}]}],\n  },\n}\n",
		"hooks absent":  prefix + "}\n",
	}
	for _, writer := range hookWriters() {
		for seedName, seed := range seeds {
			t.Run(writer.name+"/"+seedName, func(t *testing.T) {
				home := t.TempDir()
				adapter, path := hookSettingsPath(t, home, writer.agent)
				if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
					t.Fatal(err)
				}
				first, err := writer.install(home, adapter)
				if err != nil || !first.Changed {
					t.Fatalf("install over JSONC settings = %+v, %v", first, err)
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(string(got), prefix) {
					t.Fatalf("untouched JSONC text was not preserved:\n%s", got)
				}
				root, err := filemerge.UnmarshalJSONObject(got)
				if err != nil {
					t.Fatalf("result is not JSONC: %v\n%s", err, got)
				}
				if root["model"] != "opus" || !strings.Contains(string(got), writer.command) {
					t.Fatalf("settings lost or hook missing:\n%s", got)
				}
				if strings.Contains(seed, "echo keep") && !strings.Contains(string(got), "echo keep") {
					t.Fatalf("existing hook lost:\n%s", got)
				}
				second, err := writer.install(home, adapter)
				if err != nil || second.Changed {
					t.Fatalf("repeat install = %+v, %v", second, err)
				}
				if again, err := os.ReadFile(path); err != nil || string(again) != string(got) {
					t.Fatalf("repeat install changed bytes: %v", err)
				}
			})
		}
	}
}

func TestHookWritersRefuseUnsafeJSONCHooksWithoutWrite(t *testing.T) {
	seeds := map[string]string{
		"comment inside hooks": "{\n  \"model\": \"opus\",\n  \"hooks\": {\n    // mine\n    \"PreToolUse\": [],\n  },\n}\n",
		"escaped hooks key":    "{\n  // note\n  \"\\u0068ooks\": {\"PreToolUse\": []},\n}\n",
		"duplicate hooks key":  "{\n  // note\n  \"hooks\": {},\n  \"hooks\": {\"PreToolUse\": []},\n}\n",
	}
	for _, writer := range hookWriters() {
		for seedName, seed := range seeds {
			t.Run(writer.name+"/"+seedName, func(t *testing.T) {
				home := t.TempDir()
				adapter, path := hookSettingsPath(t, home, writer.agent)
				if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
					t.Fatal(err)
				}
				if result, err := writer.install(home, adapter); err == nil || result.Changed {
					t.Fatalf("unsafe JSONC hooks accepted: %+v %v", result, err)
				}
				if after, err := os.ReadFile(path); err != nil || string(after) != seed {
					t.Fatalf("refused settings changed: %q %v", after, err)
				}
			})
		}
	}
}
