package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/cursor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

type commandCodeTestAdapter struct{ *cursor.Adapter }

func (commandCodeTestAdapter) Agent() model.AgentID { return model.AgentID("command-code") }
func (commandCodeTestAdapter) MCPConfigPath(home, _ string) string {
	return filepath.Join(home, ".commandcode", "mcp.json")
}

func TestCommandCodeMCPInjection(t *testing.T) {
	home := t.TempDir()
	adapter := commandCodeTestAdapter{cursor.NewAdapter()}
	mcpPath := adapter.MCPConfigPath(home, "context7")

	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"user-tool":{"command":"bin"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Inject(home, home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first Inject() = (%v, %v), want (true, nil)", first.Changed, err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal mcp.json: %v", err)
	}
	if _, ok := doc.MCPServers["user-tool"]; !ok {
		t.Fatalf("user-tool missing: %s", string(data))
	}
	if _, ok := doc.MCPServers["context7"]; !ok {
		t.Fatalf("context7 missing: %s", string(data))
	}
	if !strings.Contains(string(data), "@upstash/context7-mcp") {
		t.Fatalf("context7 package missing: %s", string(data))
	}

	second, err := Inject(home, home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second Inject() = (%v, %v), want (false, nil)", second.Changed, err)
	}
}
