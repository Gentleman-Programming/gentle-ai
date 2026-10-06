package uninstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

const (
	jsoncManagedStop = `"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "gentle-ai telemetry runtime claude --json", "async": true}]}]`
	jsoncMixedStop   = `"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "gentle-ai telemetry runtime claude --json", "async": true}, {"type": "command", "command": "echo keep"}]}]`
)

// applyClaudeSettingsRewrite runs the full-agent uninstall rewrites planned
// for the Claude settings file over seed and returns the apply error.
func applyClaudeSettingsRewrite(t *testing.T, seed string) (string, error) {
	t.Helper()
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	adapter, _ := svc.registry.Get(model.AgentClaudeCode)
	path := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	built, err := svc.buildPlan([]model.AgentID{adapter.Agent()}, withoutComponent(fullAgentRemovalComponents, model.ComponentSDD))
	if err != nil {
		t.Fatal(err)
	}
	var applyErr error
	for _, op := range built.operations {
		if op.typeID == opRewriteFile && op.path == path {
			if _, _, err := op.apply(op.path); err != nil && applyErr == nil {
				applyErr = err
			}
		}
	}
	return path, applyErr
}

func TestFullAgentClaudeRemovesRetainedHooksFromJSONCSettings(t *testing.T) {
	const prefix = "{\n  // Personal settings.\n  \"model\": \"opus\", /* chosen by hand */\n"
	const plain = "{\n  // Personal settings.\n  \"model\": \"opus\",\n"
	for _, tc := range []struct {
		name, seed string
		keep       []string
		dropHooks  bool
	}{
		{"user hook kept", prefix + "  \"hooks\": {\n    " + jsoncMixedStop + ",\n  },\n}\n", []string{prefix, "echo keep"}, false},
		{"emptied hooks dropped", plain + "  \"hooks\": {\n    " + jsoncManagedStop + ",\n  },\n}\n", []string{plain}, true},
		{"comment attached to emptied hooks", prefix + "  \"hooks\": {\n    " + jsoncManagedStop + ",\n  },\n}\n", []string{prefix}, false},
		{"only a comment remains", "{\n  \"hooks\": {" + jsoncManagedStop + "}\n  // note\n}\n", []string{"// note"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, err := applyClaudeSettingsRewrite(t, tc.seed)
			if err != nil {
				t.Fatalf("uninstall over JSONC settings: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("settings with user text removed: %v", err)
			}
			if strings.Contains(string(raw), "telemetry runtime claude") {
				t.Fatalf("managed hook kept:\n%s", raw)
			}
			for _, want := range tc.keep {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("user text %q lost:\n%s", want, raw)
				}
			}
			if tc.dropHooks && strings.Contains(string(raw), "\"hooks\"") {
				t.Fatalf("emptied hooks object kept:\n%s", raw)
			}
		})
	}
}

func TestFullAgentClaudeRefusesUnsafeJSONCHooksWithoutWrite(t *testing.T) {
	for name, seed := range map[string]string{
		"comment beside a user hook":      "{\n  \"hooks\": {\n    // mine\n    " + jsoncMixedStop + ",\n  },\n}\n",
		"comment beside managed hooks":    "{\n  \"hooks\": {\n    // mine\n    " + jsoncManagedStop + ",\n  },\n}\n",
		"escaped hooks key":               "{\n  // note\n  \"\\u0068ooks\": {" + jsoncManagedStop + "},\n}\n",
		"escaped hooks key beside a user": "{\n  // note\n  \"\\u0068ooks\": {" + jsoncMixedStop + "},\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path, err := applyClaudeSettingsRewrite(t, seed)
			if err == nil {
				t.Fatal("unsafe JSONC hooks rewritten without refusal")
			}
			if after, readErr := os.ReadFile(path); readErr != nil || string(after) != seed {
				t.Fatalf("refused settings changed: %q %v", after, readErr)
			}
		})
	}
}
