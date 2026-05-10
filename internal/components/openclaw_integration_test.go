package components_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/internal/components/persona"
	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestOpenClawSelectedAdapterRoutesToExpectedInjectors(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentOpenClaw)
	if err != nil {
		t.Fatalf("NewAdapter(openclaw) error = %v", err)
	}

	if _, err := engram.InjectWithPromptDir(home, workspace, adapter); err != nil {
		t.Fatalf("engram.Inject(openclaw) error = %v", err)
	}
	if _, err := sdd.Inject(workspace, adapter, model.SDDModeSingle, sdd.InjectOptions{StrictTDD: true, WorkspaceDir: workspace}); err != nil {
		t.Fatalf("sdd.Inject(openclaw) error = %v", err)
	}
	if _, err := persona.Inject(workspace, adapter, model.PersonaGentleman); err != nil {
		t.Fatalf("persona.Inject(openclaw) error = %v", err)
	}

	config := readText(t, filepath.Join(home, ".openclaw", "openclaw.json"))
	var root map[string]any
	if err := json.Unmarshal([]byte(config), &root); err != nil {
		t.Fatalf("Unmarshal openclaw.json error = %v; content:\n%s", err, config)
	}
	servers := objectAt(t, objectAt(t, root, "mcp"), "servers")
	engramServer := objectAt(t, servers, "engram")
	command, ok := engramServer["command"].(string)
	if !ok {
		t.Fatalf("OpenClaw Engram command = %#v, want string", engramServer["command"])
	}
	if got := filepath.Base(command); got != "engram" {
		t.Fatalf("OpenClaw Engram command = %q, want executable named engram", command)
	}

	agentsText := readText(t, filepath.Join(workspace, "AGENTS.md"))
	for _, want := range []string{"gentle-ai:engram-protocol", "gentle-ai:sdd-orchestrator", "gentle-ai:strict-tdd-mode"} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("OpenClaw AGENTS.md missing %q; got:\n%s", want, agentsText)
		}
	}
	if strings.Contains(agentsText, "Senior Architect") {
		t.Fatalf("OpenClaw AGENTS.md must not receive persona content; got:\n%s", agentsText)
	}

	soulText := readText(t, filepath.Join(workspace, "SOUL.md"))
	if !strings.Contains(soulText, "gentle-ai:persona") || !strings.Contains(soulText, "Senior Architect") {
		t.Fatalf("OpenClaw SOUL.md missing managed persona content; got:\n%s", soulText)
	}
	if _, err := os.Stat(filepath.Join(workspace, "TOOLS.md")); !os.IsNotExist(err) {
		t.Fatalf("OpenClaw injector chain must not create TOOLS.md for protocols; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".openclaw", "openclaw.json")); !os.IsNotExist(err) {
		t.Fatalf("OpenClaw injector chain must not create workspace OpenClaw MCP config; stat err=%v", err)
	}
}

func TestOpenClawInjectorChainRerunIsIdempotent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentOpenClaw)
	if err != nil {
		t.Fatalf("NewAdapter(openclaw) error = %v", err)
	}

	runOpenClawInjectorChain(t, home, workspace, adapter)
	beforeAgents := readText(t, filepath.Join(workspace, "AGENTS.md"))
	beforeSoul := readText(t, filepath.Join(workspace, "SOUL.md"))
	beforeConfig := readText(t, filepath.Join(home, ".openclaw", "openclaw.json"))

	runOpenClawInjectorChain(t, home, workspace, adapter)
	afterAgents := readText(t, filepath.Join(workspace, "AGENTS.md"))
	afterSoul := readText(t, filepath.Join(workspace, "SOUL.md"))
	afterConfig := readText(t, filepath.Join(home, ".openclaw", "openclaw.json"))

	if beforeAgents != afterAgents {
		t.Fatalf("OpenClaw AGENTS.md changed on rerun\nbefore:\n%s\nafter:\n%s", beforeAgents, afterAgents)
	}
	if beforeSoul != afterSoul {
		t.Fatalf("OpenClaw SOUL.md changed on rerun\nbefore:\n%s\nafter:\n%s", beforeSoul, afterSoul)
	}
	if beforeConfig != afterConfig {
		t.Fatalf("OpenClaw config changed on rerun\nbefore:\n%s\nafter:\n%s", beforeConfig, afterConfig)
	}
}

func runOpenClawInjectorChain(t *testing.T, home, workspace string, adapter agents.Adapter) {
	t.Helper()
	if _, err := engram.InjectWithPromptDir(home, workspace, adapter); err != nil {
		t.Fatalf("engram.Inject(openclaw) error = %v", err)
	}
	if _, err := sdd.Inject(workspace, adapter, model.SDDModeSingle, sdd.InjectOptions{StrictTDD: true, WorkspaceDir: workspace}); err != nil {
		t.Fatalf("sdd.Inject(openclaw) error = %v", err)
	}
	if _, err := persona.Inject(workspace, adapter, model.PersonaGentleman); err != nil {
		t.Fatalf("persona.Inject(openclaw) error = %v", err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func objectAt(t *testing.T, root map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := root[key]
	if !ok {
		t.Fatalf("missing object key %q in %#v", key, root)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("key %q has type %T, want object", key, value)
	}
	return object
}
