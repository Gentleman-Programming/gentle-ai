package engram

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
func (commandCodeTestAdapter) SystemPromptFile(home string) string {
	return filepath.Join(home, ".commandcode", "AGENTS.md")
}
func (commandCodeTestAdapter) SystemPromptStrategy() model.SystemPromptStrategy {
	return model.StrategyMarkdownSections
}
func (commandCodeTestAdapter) MCPConfigPath(home, _ string) string {
	return filepath.Join(home, ".commandcode", "mcp.json")
}

func TestCommandCodeEngramInjection(t *testing.T) {
	home := t.TempDir()
	adapter := commandCodeTestAdapter{cursor.NewAdapter()}
	mcpPath := adapter.MCPConfigPath(home, "engram")
	promptPath := adapter.SystemPromptFile(home)

	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"custom-server":{"command":"custom-bin"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(promptPath, []byte("# User Instructions\n\nPreserve me.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Inject(home, adapter)
	if err != nil || !first.Changed {
		t.Fatalf("first Inject() = (%v, %v), want (true, nil)", first.Changed, err)
	}

	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(mcpData, &doc); err != nil {
		t.Fatalf("unmarshal mcp.json: %v", err)
	}
	if _, ok := doc.MCPServers["custom-server"]; !ok {
		t.Fatalf("custom-server missing: %s", string(mcpData))
	}
	if _, ok := doc.MCPServers["engram"]; !ok {
		t.Fatalf("engram missing: %s", string(mcpData))
	}

	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	promptText := string(promptData)
	if !strings.Contains(promptText, "<!-- gentle-ai:engram-protocol -->") ||
		!strings.Contains(promptText, "<!-- /gentle-ai:engram-protocol -->") {
		t.Fatalf("AGENTS.md missing engram markers: %s", promptText)
	}
	if !strings.Contains(promptText, "# User Instructions") {
		t.Fatalf("AGENTS.md must preserve user content: %s", promptText)
	}

	second, err := Inject(home, adapter)
	if err != nil || second.Changed {
		t.Fatalf("second Inject() = (%v, %v), want (false, nil)", second.Changed, err)
	}
}
