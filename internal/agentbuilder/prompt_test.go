package agentbuilder

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestComposePrompt_StandaloneMode_NoSDDContext(t *testing.T) {
	prompt := ComposePrompt("build a css linter", nil, nil)

	// No SDD Integration Context block for standalone.
	if strings.Contains(prompt, "SDD Integration Context") {
		t.Errorf("standalone mode should not include SDD context block; got:\n%s", prompt)
	}
}

func TestComposePrompt_StandaloneSDDConfig_NoSDDContext(t *testing.T) {
	cfg := &SDDIntegration{Mode: SDDStandalone}
	prompt := ComposePrompt("build a css linter", cfg, nil)

	// Standalone mode should not include SDD context, even when config is passed.
	if strings.Contains(prompt, "SDD Integration Context") {
		t.Errorf("SDDStandalone mode should not include SDD context block; got:\n%s", prompt)
	}
}

func TestComposePrompt_PhaseSupportMode_SDDContextPresent(t *testing.T) {
	cfg := &SDDIntegration{
		Mode:        SDDPhaseSupport,
		TargetPhase: "apply",
	}
	prompt := ComposePrompt("help with apply phase", cfg, nil)

	if !strings.Contains(prompt, "SDD Integration Context") {
		t.Errorf("phase-support mode should include SDD context block; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "apply") {
		t.Errorf("phase-support should reference target phase 'apply'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "sdd-apply") {
		t.Errorf("phase-support should reference sdd-apply trigger pattern; got:\n%s", prompt)
	}
}

func TestComposePrompt_NewPhaseMode_SDDContextPresent(t *testing.T) {
	cfg := &SDDIntegration{
		Mode:      SDDNewPhase,
		PhaseName: "review",
	}
	prompt := ComposePrompt("create a review phase", cfg, nil)

	if !strings.Contains(prompt, "SDD Integration Context") {
		t.Errorf("new-phase mode should include SDD context block; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "review") {
		t.Errorf("new-phase should reference phase name 'review'; got:\n%s", prompt)
	}
	// New phase references the dependency graph integration
	if !strings.Contains(prompt, "dependency graph") {
		t.Errorf("new-phase should mention dependency graph; got:\n%s", prompt)
	}
}

func TestComposePrompt_InstalledAgentsIncluded(t *testing.T) {
	agents := []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode}
	prompt := ComposePrompt("build an agent", nil, agents)

	if !strings.Contains(prompt, "Installed Agents Context") {
		t.Errorf("should include installed agents context; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, string(model.AgentClaudeCode)) {
		t.Errorf("should include claude-code agent; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, string(model.AgentOpenCode)) {
		t.Errorf("should include opencode agent; got:\n%s", prompt)
	}
}

func TestComposePrompt_NoInstalledAgents_NoAgentContext(t *testing.T) {
	prompt := ComposePrompt("build an agent", nil, nil)

	if strings.Contains(prompt, "Installed Agents Context") {
		t.Errorf("should not include agents context when list is empty; got:\n%s", prompt)
	}
}

func TestComposePrompt_UserInputPresent(t *testing.T) {
	userInput := "build a unique custom validator for database migrations"
	prompt := ComposePrompt(userInput, nil, nil)

	if !strings.Contains(prompt, userInput) {
		t.Errorf("user input %q not found in prompt;\ngot:\n%s", userInput, prompt)
	}
}

func TestComposePrompt_SystemPromptHeader(t *testing.T) {
	prompt := ComposePrompt("test", nil, nil)

	// The prompt should start with the system prompt base content.
	if !strings.Contains(prompt, "You are an expert AI agent skill designer") {
		t.Errorf("system prompt header not found;\ngot:\n%s", prompt)
	}
}

func TestComposePrompt_UserRequestSection(t *testing.T) {
	prompt := ComposePrompt("my special request", nil, nil)

	if !strings.Contains(prompt, "## User Request") {
		t.Errorf("should contain '## User Request' section;\ngot:\n%s", prompt)
	}
}
