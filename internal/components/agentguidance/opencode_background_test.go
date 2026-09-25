package agentguidance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyOpenCodeBackgroundPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if result, err := ApplyOpenCodeBackgroundPolicy(path, true); err != nil || result.Changed {
		t.Fatalf("absent settings must stay absent: %+v %v", result, err)
	}
	original := `{"permission":{"bash":"ask"},"agent":{"gentle-orchestrator":{"prompt":"user prompt","permission":{"task":"deny"}},"other":{"prompt":"custom"}}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := ApplyOpenCodeBackgroundPolicy(path, true)
	if err != nil || !first.Changed {
		t.Fatalf("on: %+v %v", first, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "gentle-ai:opencode-background-subagents") || !strings.Contains(string(content), `"bash": "ask"`) || !strings.Contains(string(content), `"task": "deny"`) || !strings.Contains(string(content), `"other"`) {
		t.Fatalf("policy or user settings lost: %s", content)
	}
	again, err := ApplyOpenCodeBackgroundPolicy(path, true)
	if err != nil || again.Changed {
		t.Fatalf("repeat on must converge: %+v %v", again, err)
	}
	off, err := ApplyOpenCodeBackgroundPolicy(path, false)
	if err != nil || !off.Changed {
		t.Fatalf("off: %+v %v", off, err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "gentle-ai:opencode-background-subagents") || !strings.Contains(string(content), "user prompt") || !strings.Contains(string(content), `"bash": "ask"`) {
		t.Fatalf("off damaged settings: %s", content)
	}
	again, err = ApplyOpenCodeBackgroundPolicy(path, false)
	if err != nil || again.Changed {
		t.Fatalf("repeat off must converge: %+v %v", again, err)
	}
}
