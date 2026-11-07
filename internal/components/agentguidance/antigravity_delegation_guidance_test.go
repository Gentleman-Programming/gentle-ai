package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
)

// TestAntigravityOrchestratorDelegationGuidance verifies that Antigravity orchestrator
// guidance correctly specifies delegation routing for both Gentle Shell bridge mode
// and standalone Antigravity mode.
func TestAntigravityOrchestratorDelegationGuidance(t *testing.T) {
	content, err := assets.Read("antigravity/orchestrator.md")
	if err != nil {
		t.Fatalf("assets.Read(antigravity/orchestrator.md) error = %v", err)
	}

	// 1. Session confirmation: verify it checks session context rather than pi-bridge presence alone.
	if !strings.Contains(content, "session context confirms the current session is a Gentle Shell turn") {
		t.Errorf("orchestrator guidance must require session context to confirm Gentle Shell turn")
	}
	if !strings.Contains(content, "Do not use `pi-bridge` availability or activity alone") {
		t.Errorf("orchestrator guidance must warn against relying on pi-bridge availability alone")
	}

	// 2. Bridge mode: routes delegation through subagent_run on pi-bridge without native subagents.
	if !strings.Contains(content, "route all subagent delegation to `subagent_run` on `pi-bridge`") {
		t.Errorf("orchestrator guidance must name subagent_run on pi-bridge as the delegation tool")
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
	if !strings.Contains(content, "`call_mcp_tool` with server `pi-bridge` and tool `subagent_run`") {
		t.Errorf("orchestrator guidance must describe call_mcp_tool usage for lazy MCP tools")
	}

	// 4. Standalone mode: requires define_subagent before invoke_subagent.
	if !strings.Contains(content, "native standalone Antigravity bounded ODD work without Gentle Shell turn context") {
		t.Errorf("orchestrator guidance must distinguish native standalone sessions")
	}
	if !strings.Contains(content, "call `define_subagent` to create a general-purpose worker with a scoped mission, then `invoke_subagent` with the defined name") {
		t.Errorf("orchestrator guidance must instruct standalone mode to call define_subagent before invoke_subagent")
	}

	// 5. Deduplication check applies across both bridge and standalone modes.
	if !strings.Contains(content, "Before launching any dynamic bounded worker across both delegation paths") {
		t.Errorf("deduplication guidance must apply across both delegation paths")
	}
	if !strings.Contains(content, "The one-writer rule does not replace this check") {
		t.Errorf("deduplication guidance must clarify that one-writer rule does not replace the check")
	}
}
