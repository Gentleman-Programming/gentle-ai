package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const testSDDSessionPreflightInitAnchor = "### SDD Init Guard (MANDATORY)"
const testSDDSessionPreflightEntryAnchor = "### SDD Entry Routing (MANDATORY)"

func TestOpenCodeSessionPreflightCompositionIsRuntimeScoped(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		content, err := composeOpenCodeOrchestratorPrompt(agent)
		if err != nil {
			t.Fatalf("composeOpenCodeOrchestratorPrompt(%s) error = %v", agent, err)
		}
		if !strings.Contains(content, sddSessionPreflightMarker) || !strings.Contains(content, "Both -> `hybrid`") {
			t.Fatalf("composeOpenCodeOrchestratorPrompt(%s) omitted canonical session preflight", agent)
		}
	}
	claude, err := composeOpenCodeOrchestratorPrompt(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("composeOpenCodeOrchestratorPrompt(claude) error = %v", err)
	}
	if strings.Contains(claude, sddSessionPreflightMarker) {
		t.Fatal("Claude composition unexpectedly received the OpenCode session preflight projection")
	}
}
func TestSDDSessionPreflightProjectionCanonicalAndBounded(t *testing.T) {
	block := sddSessionPreflightBlock()
	for _, want := range []string{"<!-- gentle-ai:sdd-session-preflight -->", "### SDD Session Preflight (HARD GATE)", "1. **Pace**", "2. **Artifacts**", "3. **PR strategy**", "Both -> `hybrid`", "fixed at 400 changed lines", "<!-- /gentle-ai:sdd-session-preflight -->"} {
		if !strings.Contains(block, want) {
			t.Fatalf("canonical block missing %q", want)
		}
	}
	for _, retired := range []string{"4. **", "Both -> `both`", "Review: 800 lines", "Other ->"} {
		if strings.Contains(block, retired) {
			t.Fatalf("canonical block retains retired content %q", retired)
		}
	}
	for _, newline := range []string{"\n", "\r\n"} {
		rendered := strings.Join([]string{"before", testSDDSessionPreflightEntryAnchor, testSDDSessionPreflightInitAnchor, "after"}, newline)
		got, err := projectSDDSessionPreflight(rendered, testSDDSessionPreflightEntryAnchor)
		if err != nil {
			t.Fatalf("projectSDDSessionPreflight() error = %v", err)
		}
		want := strings.Join([]string{"before", strings.ReplaceAll(block, "\n", newline), testSDDSessionPreflightEntryAnchor, testSDDSessionPreflightInitAnchor, "after"}, newline)
		if got != want {
			t.Fatalf("projected preflight = %q, want %q", got, want)
		}
	}
	for _, rendered := range []string{"prefix " + testSDDSessionPreflightEntryAnchor + "\n" + testSDDSessionPreflightInitAnchor, testSDDSessionPreflightEntryAnchor + " suffix\n" + testSDDSessionPreflightInitAnchor} {
		if _, err := projectSDDSessionPreflight(rendered, testSDDSessionPreflightEntryAnchor); err == nil {
			t.Fatal("projectSDDSessionPreflight() accepted an inline anchor")
		}
	}
}
