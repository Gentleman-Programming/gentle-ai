package screens_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestRenderAgents_PICapabilityWarningGolden(t *testing.T) {
	out := screens.RenderAgents([]model.AgentID{model.AgentPiCodingAgent}, 2, []string{"PI is experimental and non-parity: multi-model is enabled only when `pi-subagents` is installed."})
	snippet := extractLinesContaining(out, []string{"pi-coding-agent", "multi-model is enabled only"})
	assertGoldenString(t, "agents_pi_warning.golden.txt", snippet)
}

func TestRenderAgents_ShowsPICapabilityLabels(t *testing.T) {
	out := screens.RenderAgents([]model.AgentID{model.AgentPiCodingAgent}, 0, []string{"PI is experimental and non-parity: multi-model is enabled only when `pi-subagents` is installed."})
	if !strings.Contains(out, "experimental") {
		t.Fatalf("expected PI capability label 'experimental' in output")
	}
	if !strings.Contains(out, "non-parity") {
		t.Fatalf("expected PI capability label 'non-parity' in output")
	}
	if !strings.Contains(out, "multi-model is enabled only when `pi-subagents` is installed") {
		t.Fatalf("expected PI warning about conditional multi-model requirement")
	}
}

func assertGoldenString(t *testing.T, name, got string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", name)
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", goldenPath, err)
	}
	want := string(wantBytes)
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func extractLinesContaining(text string, needles []string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				kept = append(kept, strings.TrimSpace(line))
				break
			}
		}
	}
	return strings.Join(kept, "\n")
}
