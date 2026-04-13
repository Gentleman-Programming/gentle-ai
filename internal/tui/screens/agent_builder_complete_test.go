package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestRenderABComplete_NonEmpty(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Trigger: "When user asks for help.",
		Content: "# My Agent\n",
	}
	results := []agentbuilder.InstallResult{
		{AgentID: model.AgentClaudeCode, Path: "/path/SKILL.md", Success: true},
	}
	out := RenderABComplete(agent, results)
	if out == "" {
		t.Fatal("RenderABComplete returned empty string")
	}
}

func TestRenderABComplete_SuccessIndicatorPresent(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:  "my-agent",
		Title: "My Agent",
	}
	out := RenderABComplete(agent, nil)
	if !strings.Contains(out, "Agent Created") {
		t.Errorf("success indicator not found; output:\n%s", out)
	}
}

func TestRenderABComplete_AgentTitleShown(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:  "my-agent",
		Title: "Super Special Agent",
	}
	out := RenderABComplete(agent, nil)
	if !strings.Contains(out, "Super Special Agent") {
		t.Errorf("agent title not found; output:\n%s", out)
	}
}

func TestRenderABComplete_SuccessfulInstallResult(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Trigger: "When needed.",
	}
	results := []agentbuilder.InstallResult{
		{AgentID: model.AgentClaudeCode, Path: "/path/to/SKILL.md", Success: true},
	}
	out := RenderABComplete(agent, results)
	if !strings.Contains(out, string(model.AgentClaudeCode)) {
		t.Errorf("agent ID not found in results; output:\n%s", out)
	}
	if !strings.Contains(out, "/path/to/SKILL.md") {
		t.Errorf("install path not found; output:\n%s", out)
	}
}

func TestRenderABComplete_FailedInstallResult(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:  "my-agent",
		Title: "My Agent",
	}
	results := []agentbuilder.InstallResult{
		{AgentID: model.AgentOpenCode, Path: "/path/SKILL.md", Success: false},
	}
	out := RenderABComplete(agent, results)
	if !strings.Contains(out, string(model.AgentOpenCode)) {
		t.Errorf("failed agent ID not found; output:\n%s", out)
	}
}

func TestRenderABComplete_TriggerHintShown(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Trigger: "When you want to lint CSS",
	}
	out := RenderABComplete(agent, nil)
	if !strings.Contains(out, "When you want to lint CSS") {
		t.Errorf("trigger hint not shown; output:\n%s", out)
	}
	if !strings.Contains(out, "How to use") {
		t.Errorf("'How to use' section not found; output:\n%s", out)
	}
}

func TestRenderABComplete_DoneOptionPresent(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:  "my-agent",
		Title: "My Agent",
	}
	out := RenderABComplete(agent, nil)
	if !strings.Contains(out, "Done") {
		t.Errorf("Done option not found; output:\n%s", out)
	}
}
