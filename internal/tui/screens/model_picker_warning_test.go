package screens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderModelPicker_EmptyStateIncludesCapabilityGatedWarning(t *testing.T) {
	out := RenderModelPicker(nil, ModelPickerState{}, 0)
	if !strings.Contains(out, "capability-gated per agent") {
		t.Fatalf("expected capability-gated warning in model picker empty state")
	}
	snippet := extractModelPickerLines(out)
	assertGoldenModelPicker(t, "model_picker_empty_warning.golden.txt", snippet)
}

func TestRenderModelPicker_PIEnabledOmitsLegacyOpenCodeOnlyWarning(t *testing.T) {
	state := ModelPickerState{
		AvailableIDs: []string{"openai"},
	}

	out := RenderModelPicker(nil, state, 0)

	if strings.Contains(out, "OpenCode-only") {
		t.Fatalf("did not expect legacy OpenCode-only warning when picker is capability-enabled")
	}
}

func TestRenderModelPicker_PIUnsupportedBlocksMultiModelControlsWithCanonicalWarning(t *testing.T) {
	state := ModelPickerState{
		CapabilityBlocked: true,
		CapabilityMessage: "PI multi-model requires installing the `pi-subagents` extension.",
	}
	out := RenderModelPicker(nil, state, 0)

	if !strings.Contains(out, state.CapabilityMessage) {
		t.Fatalf("expected canonical PI warning in blocked model picker render")
	}

	if strings.Contains(out, "Set all phases") {
		t.Fatalf("unexpected multi-model controls when picker is capability blocked")
	}
}

func TestRenderModelPicker_CapabilityBlockedWithoutCustomMessageUsesFallback(t *testing.T) {
	out := RenderModelPicker(nil, ModelPickerState{CapabilityBlocked: true}, 0)

	if !strings.Contains(out, "Multi-model picker is unavailable for this agent setup.") {
		t.Fatalf("expected fallback blocked message when capability message is empty")
	}

	if !strings.Contains(out, "← Back to SDD mode") {
		t.Fatalf("expected back option when picker is capability blocked")
	}
}

func assertGoldenModelPicker(t *testing.T, name, got string) {
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

func extractModelPickerLines(out string) string {
	lines := strings.Split(out, "\n")
	var kept []string
	for _, line := range lines {
		if strings.Contains(line, "capability-gated per agent") || strings.Contains(line, "run 'opencode' once") {
			kept = append(kept, strings.TrimSpace(line))
		}
	}
	return strings.Join(kept, "\n")
}
