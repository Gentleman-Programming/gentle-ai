package model

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// codexDebugModelsFixture mirrors the shape `codex debug models` prints for
// Codex CLI 0.146.0 (issue #2218): Luna advertises max but not ultra.
const codexDebugModelsFixture = `{"models":[
{"slug":"gpt-5.6-sol","visibility":"list","supported_in_api":true,
 "supported_reasoning_levels":[{"effort":"low","description":""},{"effort":"medium","description":""},{"effort":"high","description":""},{"effort":"xhigh","description":""},{"effort":"max","description":""},{"effort":"ultra","description":""}],
 "additional_speed_tiers":["fast"],
 "service_tiers":[{"id":"priority","name":"Fast","description":"Faster responses"}]},
{"slug":"gpt-5.6-luna","visibility":"list","supported_in_api":true,
 "supported_reasoning_levels":[{"effort":"max","description":""},{"effort":"low","description":""},{"effort":"medium","description":""},{"effort":"high","description":""},{"effort":"xhigh","description":""},{"effort":"low","description":""}],
 "additional_speed_tiers":["fast"],
 "service_tiers":[{"id":"priority","name":"Fast","description":""},{"id":"default","name":"Standard","description":""},{"id":"bad tier","name":"x","description":""}]},
{"slug":"gpt-legacy","visibility":"list",
 "supported_reasoning_levels":[{"effort":"none","description":""},{"effort":"minimal","description":""},{"effort":"future-effort","description":""}]}
]}`

func stubCodexDebugModels(t *testing.T, output string) {
	t.Helper()
	originalLookPath, originalCommand := codexLookPath, codexCommand
	t.Cleanup(func() { codexLookPath, codexCommand = originalLookPath, originalCommand })
	codexLookPath = func(string) (string, error) { return "/fake/bin/codex", nil }
	codexCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return codexHelperCommand(ctx, "output", strings.ReplaceAll(output, "\n", ""))
	}
}

func TestDiscoverCodexModelsKeepsRuntimeAdvertisedCapabilitiesPerModel(t *testing.T) {
	stubCodexDebugModels(t, codexDebugModelsFixture)

	got := DiscoverCodexModels(context.Background())

	if want := []string{"gpt-5.6-sol", "gpt-5.6-luna", "gpt-legacy"}; !reflect.DeepEqual(got.Models, want) {
		t.Fatalf("Models = %v, want %v", got.Models, want)
	}
	fast := []CodexServiceTier{{ID: "priority", Name: "Fast"}}
	want := map[string]CodexModelCapabilities{
		"gpt-5.6-sol": {
			Efforts:      []CodexEffort{CodexEffortLow, CodexEffortMedium, CodexEffortHigh, CodexEffortXHigh, CodexEffortMax, CodexEffortUltra},
			ServiceTiers: fast,
		},
		// Order is canonical and duplicates collapse; the explicit "default"
		// sentinel and malformed IDs are never offered as tiers.
		"gpt-5.6-luna": {
			Efforts:      []CodexEffort{CodexEffortLow, CodexEffortMedium, CodexEffortHigh, CodexEffortXHigh, CodexEffortMax},
			ServiceTiers: fast,
		},
	}
	if !reflect.DeepEqual(got.Capabilities, want) {
		t.Fatalf("Capabilities = %#v, want %#v", got.Capabilities, want)
	}
}

func TestDiscoverCodexModelsFallbackHasNoCapabilities(t *testing.T) {
	stubCodexDebugModels(t, "not-json")

	got := DiscoverCodexModels(context.Background())

	if !reflect.DeepEqual(got.Models, CodexAvailableModels()) || got.Capabilities != nil {
		t.Fatalf("fallback = %#v, want curated models without capabilities", got)
	}
}

func TestCodexEffortMaxAndUltraAreValidAndRendered(t *testing.T) {
	for _, effort := range []CodexEffort{"max", "ultra"} {
		if !effort.Valid() {
			t.Fatalf("CodexEffort(%q).Valid() = false, want true", effort)
		}
	}
	if CodexEffort("future-effort").Valid() {
		t.Fatal("unknown effort reported valid")
	}

	table := RenderCodexODDAssignments(
		map[string]string{"odd-worker": "gpt-5.6-luna", "odd-verify": "gpt-5.6-sol"},
		map[string]CodexEffort{"odd-worker": "max", "odd-verify": "ultra"},
		nil,
	)
	for _, row := range []string{"| `odd-worker` | `gpt-5.6-luna` | `max` |", "| `odd-verify` | `gpt-5.6-sol` | `ultra` |"} {
		if !strings.Contains(table, row) {
			t.Fatalf("ODD table missing %q:\n%s", row, table)
		}
	}
}
