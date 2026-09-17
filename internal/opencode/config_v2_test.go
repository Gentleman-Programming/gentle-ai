package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestConfigV2Assignments(t *testing.T) {
	for _, tt := range []struct {
		name, fields string
		want         model.ModelAssignment
	}{
		{"legacy", `"agent":{"worker":{"model":"old/model","variant":"low"}}`, model.ModelAssignment{ProviderID: "old", ModelID: "model", Effort: "low"}},
		{"native string", `"agents":{"worker":{"model":"openrouter/vendor/model#high"}}`, model.ModelAssignment{ProviderID: "openrouter", ModelID: "vendor/model", Effort: "high"}},
		{"native expanded", `"agents":{"worker":{"model":{"providerID":"openai","model":"coder","variant":"high"}}}`, model.ModelAssignment{ProviderID: "openai", ModelID: "coder", Effort: "high"}},
		{"native wins", `"agents":{"worker":{"model":"new/model#high"}},"agent":{"worker":{"model":"old/model","variant":"low"}}`, model.ModelAssignment{ProviderID: "new", ModelID: "model", Effort: "high"}},
		{"invalid native preserves legacy", `"agent":{"worker":{"model":"old/model"}},"agents":{"worker":{"model":{"providerID":"bad","model":42}}}`, model.ModelAssignment{ProviderID: "old", ModelID: "model"}},
		{"absent native model preserves legacy", `"agent":{"worker":{"model":"old/model"}},"agents":{"worker":{"system":"instructions"}}`, model.ModelAssignment{ProviderID: "old", ModelID: "model"}},
		{"malformed native entry", `"agent":{"worker":{"model":"old/model"}},"agents":{"worker":false}`, model.ModelAssignment{ProviderID: "old", ModelID: "model"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "opencode.jsonc")
			if err := os.WriteFile(path, []byte("{"+tt.fields+"}"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadConfigSnapshot(path)
			if err != nil {
				t.Fatal(err)
			}
			if assignment := got.Assignments["worker"]; !assignment.Present || assignment.Cleared || assignment.Assignment != tt.want {
				t.Fatalf("assignment = %+v, want %+v", assignment, tt.want)
			}
		})
	}
}

func TestConfigV2Providers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	raw := `{"provider":{"local":{"name":"Legacy","options":{"baseURL":"https://old.invalid"},"models":{"legacy":{"tool_call":true}}},"retained":{}},"providers":{"local":{"name":"Native","settings":{"baseURL":"https://new.invalid"},"models":{"alias":{"modelID":"actual","capabilities":{"tools":true}},"bad":42}},"invalid":false}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfigSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Providers["local"]
	if p.Name != "Native" || p.URL != "https://new.invalid" {
		t.Fatalf("provider = %+v", p)
	}
	if m := p.Models["alias"]; m.ID != "alias" || !m.ToolCall {
		t.Fatalf("catalog alias = %+v", m)
	}
	if _, ok := p.Models["bad"]; ok {
		t.Fatal("malformed model admitted")
	}
	if _, ok := got.Providers["retained"]; !ok {
		t.Fatal("unrelated legacy provider lost")
	}
	if _, ok := got.Providers["invalid"]; ok {
		t.Fatal("malformed provider admitted")
	}
}

func TestConfigV2VariantsAndMalformedFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	raw := `{"provider":{"local":{"name":"Legacy","options":{"baseURL":"https://old.invalid"},"models":{"coder":{"tool_call":true},"disabled":{}}}},"providers":{"local":{"name":42,"settings":{"baseURL":false},"models":{"coder":{"capabilities":{"tools":false},"variants":[{"id":"high"},{"id":"low"},{"id":"high"},false,{"id":42}]},"disabled":{"disabled":true}}}},"agents":{"worker":{"disabled":true},"cleared":{},"invalid":{"model":42}}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfigSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Providers["local"]
	if p.Name != "Legacy" || p.URL != "https://old.invalid" {
		t.Fatalf("malformed native fields replaced legacy: %+v", p)
	}
	m := p.Models["coder"]
	if m.ToolCall || len(m.Variants) != 2 || m.Variants[0] != "low" || m.Variants[1] != "high" {
		t.Fatalf("model = %+v", m)
	}
	if _, ok := p.Models["disabled"]; ok {
		t.Fatal("disabled native model retained")
	}
	for _, key := range []string{"worker", "cleared"} {
		if a := got.Assignments[key]; !a.Present || !a.Cleared {
			t.Fatalf("%s = %+v", key, a)
		}
	}
	if a := got.Assignments["invalid"]; !a.Present || a.Cleared {
		t.Fatalf("malformed assignment treated as cleared: %+v", a)
	}
}

func TestConfigV2LayerPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	project := filepath.Join(home, "repo")
	global := filepath.Join(home, ".config", "opencode")
	for _, dir := range []string{global, filepath.Join(project, ".git")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(global, "opencode.json"), []byte(`{"agents":{"worker":{"model":"global/model#high"}},"providers":{"local":{"name":"Global"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "opencode.json"), []byte(`{"agent":{"worker":{"model":"project/model","variant":"low"}},"provider":{"local":{"name":"Project"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRuntimeConfigForHome(home, project)
	if err != nil {
		t.Fatal(err)
	}
	if a := got.Assignments["worker"].Assignment; a.ProviderID != "project" || a.Effort != "low" {
		t.Fatalf("lower-layer native shadowed project: %+v", a)
	}
	if p := got.Providers["local"]; p.Name != "Project" {
		t.Fatalf("lower-layer native shadowed project: %+v", p)
	}
}

func TestConfigLegacyPartialLayerPreservesFields(t *testing.T) {
	providers := configuredProviders(map[string]any{"provider": map[string]any{"local": map[string]any{"models": map[string]any{"coder": map[string]any{"name": "Coder", "tool_call": true, "reasoning": true}}}}})
	overlayConfiguredProviders(providers, map[string]any{"provider": map[string]any{"local": map[string]any{"models": map[string]any{"coder": map[string]any{"name": "Renamed"}}}}})
	if m := providers["local"].Models["coder"]; m.Name != "Renamed" || !m.ToolCall || !m.Reasoning {
		t.Fatalf("partial model layer lost fields: %+v", m)
	}
	assignments := configuredAssignments(map[string]any{"agent": map[string]any{"worker": map[string]any{"model": "local/coder", "variant": "high"}}})
	overlayConfiguredAssignments(assignments, map[string]any{"agent": map[string]any{"worker": map[string]any{"variant": "low"}}})
	if a := assignments["worker"].Assignment; a.ModelID != "coder" || a.Effort != "low" {
		t.Fatalf("partial assignment layer lost fields: %+v", a)
	}
}
