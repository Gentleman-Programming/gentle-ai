package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agentbuilder"
)

func TestRenderABPreview_NonEmpty(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:        "my-agent",
		Title:       "My Agent",
		Description: "Does useful things.",
		Trigger:     "When you need help.",
		Content:     "# My Agent\n\n## Description\nDoes useful things.\n",
	}
	out := RenderABPreview(agent, []string{"/tmp/skills"}, 0, 40, 0, nil, "")
	if out == "" {
		t.Fatal("RenderABPreview returned empty string")
	}
}

func TestRenderABPreview_HeadingContainsTitle(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "cool-agent",
		Title:   "Cool Agent",
		Content: "# Cool Agent\n",
	}
	out := RenderABPreview(agent, nil, 0, 40, 0, nil, "")
	if !strings.Contains(out, "Cool Agent") {
		t.Errorf("title not found in heading; output:\n%s", out)
	}
}

func TestRenderABPreview_MetadataPresent(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:        "my-agent",
		Title:       "My Agent",
		Description: "A helpful assistant.",
		Trigger:     "When triggered by user.",
		Content:     "# My Agent\n",
	}
	out := RenderABPreview(agent, nil, 0, 40, 0, nil, "")
	if !strings.Contains(out, "my-agent") {
		t.Errorf("agent name not found; output:\n%s", out)
	}
	if !strings.Contains(out, "A helpful assistant.") {
		t.Errorf("description not found; output:\n%s", out)
	}
}

func TestRenderABPreview_InstallTargetsShown(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Content: "# My Agent\n",
	}
	targets := []string{"/home/user/.config/Claude/skills", "/home/user/.config/opencode/skills"}
	out := RenderABPreview(agent, targets, 0, 40, 0, nil, "")
	if !strings.Contains(out, "Will be installed to") {
		t.Errorf("install targets section not found; output:\n%s", out)
	}
	if !strings.Contains(out, "Claude/skills") {
		t.Errorf("claude skills path not found; output:\n%s", out)
	}
}

func TestRenderABPreview_ActionBarPresent(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Content: "# My Agent\n",
	}
	out := RenderABPreview(agent, nil, 0, 40, 0, nil, "")
	if !strings.Contains(out, "Install") {
		t.Errorf("Install action not found; output:\n%s", out)
	}
	if !strings.Contains(out, "Regenerate") {
		t.Errorf("Regenerate action not found; output:\n%s", out)
	}
	if !strings.Contains(out, "Back") {
		t.Errorf("Back action not found; output:\n%s", out)
	}
}

func TestRenderABPreview_NilAgent_ShowsErrorMessage(t *testing.T) {
	out := RenderABPreview(nil, nil, 0, 40, 0, nil, "")
	if !strings.Contains(out, "No agent generated") {
		t.Errorf("expected 'No agent generated' for nil agent; output:\n%s", out)
	}
}

func TestRenderABPreview_InstallError_ShowsError(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Content: "# My Agent\n",
	}
	installErr := errors.New("permission denied: /home/user/.claude/skills")
	out := RenderABPreview(agent, nil, 0, 40, 0, installErr, "")
	if !strings.Contains(out, "Installation failed") {
		t.Errorf("install error banner not found; output:\n%s", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("error message not found; output:\n%s", out)
	}
}

func TestRenderABPreview_ConflictWarning_Shown(t *testing.T) {
	agent := &agentbuilder.GeneratedAgent{
		Name:    "my-agent",
		Title:   "My Agent",
		Content: "# My Agent\n",
	}
	out := RenderABPreview(agent, nil, 0, 40, 0, nil, "Warning: conflict detected.")
	if !strings.Contains(out, "Warning: conflict detected.") {
		t.Errorf("conflict warning not found in output:\n%s", out)
	}
}

func TestABPreviewActions_HasThreeItems(t *testing.T) {
	actions := ABPreviewActions()
	if len(actions) != 3 {
		t.Errorf("ABPreviewActions() len = %d, want 3", len(actions))
	}
	expected := []string{"Install", "Regenerate", "Back"}
	for i, action := range expected {
		if actions[i] != action {
			t.Errorf("actions[%d] = %q, want %q", i, actions[i], action)
		}
	}
}
