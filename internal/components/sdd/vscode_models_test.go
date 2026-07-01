package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestVSCodeModelAssignmentKeysIncludeCoordinatorAndPhases locks the native VS
// Code assignment surface to the coordinator plus real SDD phase agents.
func TestVSCodeModelAssignmentKeysIncludeCoordinatorAndPhases(t *testing.T) {
	keys := vscodeModelAssignmentKeys()

	for _, want := range append([]string{"sdd-orchestrator"}, "sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard") {
		if !containsString(keys, want) {
			t.Fatalf("vscodeModelAssignmentKeys() missing %q in %v", want, keys)
		}
	}
}

// TestVSCodeAgentKeyRejectsUnknownNativeFiles proves the assignment key list is
// an enforced allowlist, not just documentation for expected assets.
func TestVSCodeAgentKeyRejectsUnknownNativeFiles(t *testing.T) {
	if _, ok := vscodeAgentKey("experimental.agent.md"); ok {
		t.Fatal("unknown VS Code native agent file should not accept model assignments")
	}
}

// TestResolveVSCodeModelAssignment covers the resolver contract: valid dynamic
// Copilot cache entries render, while stale or unsafe assignments warn and omit.
func TestResolveVSCodeModelAssignment(t *testing.T) {
	cachePath := writeVSCodeModelCache(t, "gpt-4.1", "GPT-4.1", true)

	tests := []struct {
		name         string
		assignments  map[string]model.ModelAssignment
		cachePath    string
		wantLabel    string
		wantWarnings []string
	}{
		{
			name:         "missing assignment inherits parent silently",
			assignments:  nil,
			cachePath:    cachePath,
			wantLabel:    "",
			wantWarnings: nil,
		},
		{
			name: "valid github copilot cache entry renders display label",
			assignments: map[string]model.ModelAssignment{
				"sdd-apply": {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			},
			cachePath:    cachePath,
			wantLabel:    "GPT-4.1",
			wantWarnings: nil,
		},
		{
			name: "provider mismatch warns and omits",
			assignments: map[string]model.ModelAssignment{
				"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-opus-4"},
			},
			cachePath:    cachePath,
			wantWarnings: []string{"github-copilot"},
		},
		{
			name: "missing cache warns and omits",
			assignments: map[string]model.ModelAssignment{
				"sdd-apply": {ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			},
			cachePath:    filepath.Join(t.TempDir(), "missing-models.json"),
			wantWarnings: []string{"models cache"},
		},
		{
			name: "non tool capable model warns and omits",
			assignments: map[string]model.ModelAssignment{
				"sdd-apply": {ProviderID: "github-copilot", ModelID: "text-only"},
			},
			cachePath:    writeVSCodeModelCache(t, "text-only", "Text Only", false),
			wantWarnings: []string{"tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, warnings := resolveVSCodeModelAssignment("sdd-apply", tt.assignments, tt.cachePath, "")
			if label != tt.wantLabel {
				t.Fatalf("label = %q, want %q", label, tt.wantLabel)
			}
			for _, want := range tt.wantWarnings {
				if !containsWarning(warnings, want) {
					t.Fatalf("warnings = %v, want substring %q", warnings, want)
				}
			}
		})
	}
}

// TestResolveVSCodeModelAssignmentEffortMetadata proves effort assignments are
// only accepted when variant metadata can validate the requested effort.
func TestResolveVSCodeModelAssignmentEffortMetadata(t *testing.T) {
	cachePath := writeVSCodeModelCache(t, "gpt-4.1", "GPT-4.1", true)

	_, warnings := resolveVSCodeModelAssignment("sdd-apply", map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "github-copilot", ModelID: "gpt-4.1", Effort: "low"},
	}, cachePath, filepath.Join(t.TempDir(), "missing-variants.json"))
	if !containsWarning(warnings, "effort metadata") {
		t.Fatalf("warnings = %v, want missing effort metadata warning", warnings)
	}

	variantsPath := writeVSCodeModelVariants(t, "gpt-4.1", []string{"low", "medium"})
	_, warnings = resolveVSCodeModelAssignment("sdd-apply", map[string]model.ModelAssignment{
		"sdd-apply": {ProviderID: "github-copilot", ModelID: "gpt-4.1", Effort: "high"},
	}, cachePath, variantsPath)
	if !containsWarning(warnings, "effort") {
		t.Fatalf("warnings = %v, want invalid effort warning", warnings)
	}
}

func TestRenderVSCodeAgentModelAssignmentStripsStaleModelPlaceholdersWithoutValidAssignment(t *testing.T) {
	staleContent := strings.Join([]string{
		"---",
		"name: sdd-orchestrator",
		"target: vscode",
		"user-invocable: true",
		"model: {{VSC_MODEL}}",
		"---",
		"",
		"Body",
	}, "\n")

	tests := []struct {
		name        string
		opts        InjectOptions
		wantWarning string
	}{
		{
			name: "missing assignments strips placeholder",
			opts: InjectOptions{},
		},
		{
			name: "invalid provider strips placeholder",
			opts: InjectOptions{VSCodeModelAssignments: map[string]model.ModelAssignment{
				"sdd-orchestrator": {ProviderID: "anthropic", ModelID: "claude-opus-4"},
			}},
			wantWarning: "github-copilot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings := renderVSCodeAgentModelAssignment(staleContent, "sdd-orchestrator.agent.md", tt.opts)
			if strings.Contains(got, "{{VSC_MODEL}}") {
				t.Fatalf("rendered VS Code agent leaked raw placeholder:\n%s", got)
			}
			if strings.Contains(got, "model:") {
				t.Fatalf("rendered VS Code agent should inherit parent model without model frontmatter:\n%s", got)
			}
			if tt.wantWarning != "" && !containsWarning(warnings, tt.wantWarning) {
				t.Fatalf("warnings = %v, want substring %q", warnings, tt.wantWarning)
			}
		})
	}
}

// writeVSCodeModelCache creates the smallest OpenCode-compatible model cache
// needed to test Copilot label resolution and tool-call filtering.
func writeVSCodeModelCache(t *testing.T, modelID, name string, toolCall bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	content := `{"github-copilot":{"id":"github-copilot","name":"GitHub Copilot","models":{"` + modelID + `":{"id":"` + modelID + `","name":"` + name + `","tool_call":` + boolJSON(toolCall) + `}}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// writeVSCodeModelVariants writes provider variant metadata so effort validation
// can be tested without depending on the user's real cache.
func writeVSCodeModelVariants(t *testing.T, modelID string, efforts []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "variants.json")
	quoted := make([]string, 0, len(efforts))
	for _, effort := range efforts {
		quoted = append(quoted, `"`+effort+`"`)
	}
	content := `{"github-copilot":{"` + modelID + `":[` + strings.Join(quoted, ",") + `]}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// boolJSON keeps generated fixture JSON readable while avoiding fmt noise in
// table setup.
func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}



// containsWarning checks warning substrings so tests assert behavior without
// coupling to exact full diagnostic wording.
func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}
