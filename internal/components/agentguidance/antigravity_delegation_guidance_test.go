package agentguidance_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestAntigravityOrchestratorDelegationGuidance verifies that Antigravity orchestrator
// guidance correctly specifies delegation routing for both Gentle Shell bridge mode
// (routed to pi_subagent_run when pi-bridge is active) and standalone Antigravity mode.
func TestAntigravityOrchestratorDelegationGuidance(t *testing.T) {
	content, err := assets.Read("antigravity/orchestrator.md")
	if err != nil {
		t.Fatalf("assets.Read(antigravity/orchestrator.md) error = %v", err)
	}

	// 1. Bridge activity routing condition: verify pi-bridge activity triggers bridge delegation (#6011).
	if !strings.Contains(content, "When `pi-bridge` is active") {
		t.Errorf("orchestrator guidance must specify pi-bridge activity as the routing condition")
	}

	// 2. Bridge mode: routes delegation through pi_subagent_run on pi-bridge without native subagents.
	if !strings.Contains(content, "route all subagent delegation to `pi_subagent_run` on `pi-bridge`") {
		t.Errorf("orchestrator guidance must name pi_subagent_run on pi-bridge as the delegation tool")
	}
	if !strings.Contains(content, "~/.pi/gentle-ai/models.json") {
		t.Errorf("orchestrator guidance must mention models configured in ~/.pi/gentle-ai/models.json")
	}
	if !strings.Contains(content, "In Gentle Shell bridge mode, do NOT call `define_subagent` or `invoke_subagent`") {
		t.Errorf("orchestrator guidance must prohibit define_subagent and invoke_subagent in bridge mode")
	}

	// 3. Eager vs lazy MCP tool invocation: direct eager mcp_* vs call_mcp_tool for lazy.
	if !strings.Contains(content, "registered direct `mcp_<serverName>_<toolName>` tool") {
		t.Errorf("orchestrator guidance must permit direct invocation for eager MCP tools")
	}
	if !strings.Contains(content, "`call_mcp_tool` with `ServerName: \"pi-bridge\"` and `ToolName: \"pi_subagent_run\"`") {
		t.Errorf("orchestrator guidance must describe call_mcp_tool usage for lazy MCP tools")
	}

	// 4. Standalone mode: requires define_subagent before invoke_subagent when pi-bridge is not active.
	if !strings.Contains(content, "when `pi-bridge` is not active (native standalone Antigravity bounded ODD work)") {
		t.Errorf("orchestrator guidance must distinguish native standalone sessions when pi-bridge is not active")
	}
	if !strings.Contains(content, "call `define_subagent` to create a general-purpose worker with a scoped mission, then `invoke_subagent` with the defined name") {
		t.Errorf("orchestrator guidance must instruct standalone mode to call define_subagent before invoke_subagent")
	}

	// 5. Deduplication check applies across both bridge and standalone modes.
	if !strings.Contains(content, "Before launching any dynamic bounded worker across both delegation paths") {
		t.Errorf("deduplication guidance must apply across both delegation paths")
	}
	if !strings.Contains(content, "`pi_subagent_run`") {
		t.Errorf("deduplication guidance must name pi_subagent_run for bridge mode")
	}
	if !strings.Contains(content, "The one-writer rule does not replace this check") {
		t.Errorf("deduplication guidance must clarify that one-writer rule does not replace the check")
	}
}

// TestAntigravityPublishedRulesCarryPiBridgeSubagentBinding verifies that
// when Antigravity rule files are rendered from installed guidance
// (via InjectRoutingWithOptions), the resulting rule files (03-orchestrator parts)
// carry the pi_subagent_run binding and routing condition for Gentle Shell / pi-bridge
// delegation across the collected parts (#6011).
func TestAntigravityPublishedRulesCarryPiBridgeSubagentBinding(t *testing.T) {
	home := t.TempDir()
	_, err := agentguidance.InjectRoutingWithOptions(home, model.AgentAntigravity, agentguidance.RoutingOptions{})
	if err != nil {
		t.Fatalf("InjectRoutingWithOptions(antigravity) error = %v", err)
	}

	stagingPath := antigravityrules.StagingPath(home)
	stagingBytes, err := os.ReadFile(stagingPath)
	if err != nil {
		t.Fatalf("os.ReadFile(stagingPath) error = %v", err)
	}

	rules := antigravityrules.Render(string(stagingBytes))

	var orchestratorParts []string
	for _, rule := range rules {
		if strings.Contains(rule.Name, "orchestrator") {
			orchestratorParts = append(orchestratorParts, rule.Content)
		}
	}
	if len(orchestratorParts) == 0 {
		t.Fatalf("expected to find rendered orchestrator rule files, got none")
	}

	combined := strings.Join(orchestratorParts, "\n")
	if !strings.Contains(combined, "pi_subagent_run") {
		t.Errorf("collected orchestrator parts must contain pi_subagent_run binding")
	}
	if !strings.Contains(combined, "When `pi-bridge` is active") {
		t.Errorf("collected orchestrator parts must specify pi-bridge activity as the routing condition")
	}
	if !strings.Contains(combined, "~/.pi/gentle-ai/models.json") {
		t.Errorf("collected orchestrator parts must mention ~/.pi/gentle-ai/models.json")
	}
}
