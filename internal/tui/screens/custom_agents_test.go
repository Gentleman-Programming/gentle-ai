package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
)

func TestRenderCustomAgents_EmptyAndPopulated(t *testing.T) {
	out := RenderCustomAgents(nil, 0, nil, true)
	if !strings.Contains(out, "Manage Custom Agents") || !strings.Contains(out, "No custom agents created yet. Use 'Create new agent' to build one.") {
		t.Errorf("expected empty message with create instruction, got: %s", out)
	}

	outErrOnly := RenderCustomAgents(nil, 0, errors.New("cannot read registry"), true)
	if !strings.Contains(outErrOnly, "Error: cannot read registry") {
		t.Errorf("expected error displayed on load failure, got: %s", outErrOnly)
	}
	if strings.Contains(outErrOnly, "No custom agents created yet") {
		t.Errorf("did not expect empty message when err is non-nil, got: %s", outErrOnly)
	}

	agents := []agentbuilder.RegistryEntry{
		{Name: "agent-one", Title: "Agent One"},
		{Name: "agent-two", Title: "Agent Two"},
	}
	out = RenderCustomAgents(agents, 0, errors.New("sample error"), false)
	if !strings.Contains(out, "agent-one ─── Agent One") || !strings.Contains(out, "Error: sample error") {
		t.Errorf("expected agent listed and error displayed, got: %s", out)
	}
	if !strings.Contains(out, "no engine available") {
		t.Errorf("expected disabled create label when hasEngines=false, got: %s", out)
	}
	if count := CustomAgentsOptionCount(agents); count != 4 {
		t.Errorf("CustomAgentsOptionCount = %d, want 4", count)
	}
}

func TestRenderCustomAgentDelete(t *testing.T) {
	out := RenderCustomAgentDelete("my-custom-agent", 0)
	if !strings.Contains(out, "Delete Custom Agent") || !strings.Contains(out, "my-custom-agent") {
		t.Errorf("expected title and agent name, got: %s", out)
	}
	if !strings.Contains(out, "supported installed agent skill directories") {
		t.Errorf("expected accurate deletion scope description, got: %s", out)
	}
	if count := CustomAgentDeleteOptionCount(); count != 2 {
		t.Errorf("CustomAgentDeleteOptionCount = %d, want 2", count)
	}
}

func TestRenderCustomAgents_SanitizesControlCharacters(t *testing.T) {
	agents := []agentbuilder.RegistryEntry{
		{Name: "agent\x1b[31m-name\nwith-newline", Title: "Title\x1b[0m\r\nInjected"},
	}
	out := RenderCustomAgents(agents, 0, nil, true)
	if strings.Contains(out, "\nwith-newline") || strings.Contains(out, "\x1b[31m") {
		t.Errorf("expected control characters and newlines sanitized from Name, got: %q", out)
	}
	if strings.Contains(out, "\r") || strings.Contains(out, "\x1b[0m") {
		t.Errorf("expected control characters sanitized from Title, got: %q", out)
	}
	if !strings.Contains(out, "agent-namewith-newline ─── TitleInjected") {
		t.Errorf("expected sanitized label in output, got: %s", out)
	}

	deleteOut := RenderCustomAgentDelete("agent\nname\x1b[31m", 0)
	if strings.Contains(deleteOut, "\nname") || strings.Contains(deleteOut, "\x1b[31m") {
		t.Errorf("expected delete screen to sanitize agent name, got: %q", deleteOut)
	}
}
