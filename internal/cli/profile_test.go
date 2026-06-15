package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"gopkg.in/yaml.v3"
)

// --- T18: CLI profile command tests ---

func TestParseProfileYAML_Developer(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/developer.yaml")
	if err != nil {
		t.Fatalf("failed to read developer.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "developer" {
		t.Errorf("ID = %q, want %q", p.ID, "developer")
	}
	if p.Role != model.RoleDeveloper {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleDeveloper)
	}
	if p.Name == "" {
		t.Error("Name should not be empty")
	}
	if p.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestParseProfileYAML_Cybersecurity(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/cybersecurity.yaml")
	if err != nil {
		t.Fatalf("failed to read cybersecurity.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "cybersecurity" {
		t.Errorf("ID = %q, want %q", p.ID, "cybersecurity")
	}
	if p.Role != model.RoleCybersecurity {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleCybersecurity)
	}
	if len(p.Skills) < 3 {
		t.Errorf("expected >= 3 skills, got %d", len(p.Skills))
	}
	if len(p.MCPConfig) < 2 {
		t.Errorf("expected >= 2 MCP servers, got %d", len(p.MCPConfig))
	}
}

func TestParseProfileYAML_Marketing(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/marketing.yaml")
	if err != nil {
		t.Fatalf("failed to read marketing.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "marketing" {
		t.Errorf("ID = %q, want %q", p.ID, "marketing")
	}
	if p.Role != model.RoleMarketing {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleMarketing)
	}
}

func TestParseProfileYAML_Education(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/education.yaml")
	if err != nil {
		t.Fatalf("failed to read education.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "education" {
		t.Errorf("ID = %q, want %q", p.ID, "education")
	}
	if p.Role != model.RoleEducation {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleEducation)
	}
}

func TestParseProfileYAML_Design(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/design.yaml")
	if err != nil {
		t.Fatalf("failed to read design.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "design" {
		t.Errorf("ID = %q, want %q", p.ID, "design")
	}
	if p.Role != model.RoleDesign {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleDesign)
	}
}

func TestParseProfileYAML_DataScience(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/data-science.yaml")
	if err != nil {
		t.Fatalf("failed to read data-science.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	if p.ID != "data-science" {
		t.Errorf("ID = %q, want %q", p.ID, "data-science")
	}
	if p.Role != model.RoleDataScience {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleDataScience)
	}
}

func TestParseProfileYAML_InvalidYAML(t *testing.T) {
	_, err := ParseProfileYAML([]byte("not: valid: yaml: ["))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseProfileYAML_EmptyInput(t *testing.T) {
	_, err := ParseProfileYAML([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseProfileYAML_MissingRequiredFields(t *testing.T) {
	yamlData := []byte(`
id: test
name: Test
`)
	_, err := ParseProfileYAML(yamlData)
	if err == nil {
		t.Fatal("expected error for missing role field")
	}
}

func TestParseProfileYAML_AllFieldsPreserved(t *testing.T) {
	yamlData := []byte(`
id: test-profile
name: Test Profile
description: A test profile for unit testing
role: developer
focus:
  - testing
  - quality

persona:
  base: gentleman
  tone: test-tone
  style: test-style
  focus: test-focus
  rules:
    - rule1
    - rule2

skills:
  - id: sdd-apply
    priority: primary
    relevance: 0.95
  - id: go-testing
    priority: secondary

mcp_config:
  - name: test-mcp
    command: npx
    args:
      - test-mcp
    category: search
    priority: required
    quality_score: 90
    free_tier: true

sdd_adaptations:
  phase_gates:
    sdd-apply:
      - phase: sdd-apply
        rule: qa-pass
        severity: CRITICAL
        source: qa
        action: block
  phase_skip:
    - sdd-explore

triggers:
  events:
    - pre-commit
    - pre-pr
  bindings:
    - on: pre-commit
      when:
        path_globs:
          - "**/*.go"
      run:
        - review-risk
      mode: strong

metadata:
  author: test-author
  version: "1.0.0"
  tags:
    - test
`)

	p, err := ParseProfileYAML(yamlData)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	// Identity
	if p.ID != "test-profile" {
		t.Errorf("ID = %q, want %q", p.ID, "test-profile")
	}
	if p.Role != model.RoleDeveloper {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleDeveloper)
	}
	if len(p.Focus) != 2 {
		t.Errorf("Focus len = %d, want 2", len(p.Focus))
	}

	// Persona
	if p.Persona.Base != model.PersonaGentleman {
		t.Errorf("Persona.Base = %q, want %q", p.Persona.Base, model.PersonaGentleman)
	}
	if p.Persona.Tone != "test-tone" {
		t.Errorf("Persona.Tone = %q, want %q", p.Persona.Tone, "test-tone")
	}
	if len(p.Persona.Rules) != 2 {
		t.Errorf("Persona.Rules len = %d, want 2", len(p.Persona.Rules))
	}

	// Skills
	if len(p.Skills) != 2 {
		t.Errorf("Skills len = %d, want 2", len(p.Skills))
	}
	if p.Skills[0].ID != "sdd-apply" {
		t.Errorf("Skills[0].ID = %q, want %q", p.Skills[0].ID, "sdd-apply")
	}

	// MCP
	if len(p.MCPConfig) != 1 {
		t.Errorf("MCPConfig len = %d, want 1", len(p.MCPConfig))
	}
	if p.MCPConfig[0].Name != "test-mcp" {
		t.Errorf("MCPConfig[0].Name = %q, want %q", p.MCPConfig[0].Name, "test-mcp")
	}
	if !p.MCPConfig[0].FreeTier {
		t.Error("MCPConfig[0].FreeTier should be true")
	}

	// SDD
	if len(p.SDDAdapt.PhaseGates) != 1 {
		t.Errorf("SDDAdapt.PhaseGates len = %d, want 1", len(p.SDDAdapt.PhaseGates))
	}
	if len(p.SDDAdapt.PhaseSkip) != 1 {
		t.Errorf("SDDAdapt.PhaseSkip len = %d, want 1", len(p.SDDAdapt.PhaseSkip))
	}

	// Triggers
	if len(p.Triggers.Bindings) != 1 {
		t.Errorf("Triggers.Bindings len = %d, want 1", len(p.Triggers.Bindings))
	}
	if len(p.Triggers.Events) != 2 {
		t.Errorf("Triggers.Events len = %d, want 2", len(p.Triggers.Events))
	}

	// Metadata
	if p.Metadata.Author != "test-author" {
		t.Errorf("Metadata.Author = %q, want %q", p.Metadata.Author, "test-author")
	}
	if p.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q, want %q", p.Metadata.Version, "1.0.0")
	}
}

func TestParseProfileYAML_RoundTrip(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profiles/developer.yaml")
	if err != nil {
		t.Fatalf("failed to read developer.yaml: %v", err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		t.Fatalf("ParseProfileYAML returned error: %v", err)
	}

	// Marshal back to JSON to verify round-trip
	jsonData, err := jsonMarshal(p)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored model.RoleProfile
	if err := jsonUnmarshal(jsonData, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.ID != p.ID {
		t.Errorf("round-trip ID = %q, want %q", restored.ID, p.ID)
	}
	if restored.Role != p.Role {
		t.Errorf("round-trip Role = %q, want %q", restored.Role, p.Role)
	}
}

// --- Template rendering tests ---

func TestRenderProfileTemplate_Base(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profile-templates/base.md.tmpl")
	if err != nil {
		t.Fatalf("failed to read base.md.tmpl: %v", err)
	}

	vars := map[string]string{
		"Name":        "Developer",
		"Role":        "developer",
		"Description": "Full Stack Developer profile",
	}

	result, err := RenderProfileTemplate(string(data), vars)
	if err != nil {
		t.Fatalf("RenderProfileTemplate returned error: %v", err)
	}

	if !strings.Contains(result, "Developer") {
		t.Error("rendered template should contain 'Developer'")
	}
	if strings.Contains(result, "{{.Name}}") {
		t.Error("template variable {{.Name}} should have been substituted")
	}
}

func TestRenderProfileTemplate_Developer(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profile-templates/developer.md.tmpl")
	if err != nil {
		t.Fatalf("failed to read developer.md.tmpl: %v", err)
	}

	vars := map[string]string{
		"Name":        "Full Stack Developer",
		"Role":        "developer",
		"Description": "A developer profile",
		"Tone":        "technical, code-first",
		"Focus":       "maintainable, testable code",
	}

	result, err := RenderProfileTemplate(string(data), vars)
	if err != nil {
		t.Fatalf("RenderProfileTemplate returned error: %v", err)
	}

	if !strings.Contains(result, "Full Stack Developer") {
		t.Error("rendered template should contain 'Full Stack Developer'")
	}
	if strings.Contains(result, "{{.") {
		t.Error("all template variables should have been substituted")
	}
}

func TestRenderProfileTemplate_Cybersecurity(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/profile-templates/cybersecurity.md.tmpl")
	if err != nil {
		t.Fatalf("failed to read cybersecurity.md.tmpl: %v", err)
	}

	vars := map[string]string{
		"Name":        "Cybersecurity Researcher",
		"Role":        "cybersecurity",
		"Description": "A cybersecurity profile",
		"Tone":        "adversarial, security-first",
		"Focus":       "attack vectors and threat models",
	}

	result, err := RenderProfileTemplate(string(data), vars)
	if err != nil {
		t.Fatalf("RenderProfileTemplate returned error: %v", err)
	}

	if !strings.Contains(result, "Cybersecurity Researcher") {
		t.Error("rendered template should contain 'Cybersecurity Researcher'")
	}
	if strings.Contains(result, "{{.") {
		t.Error("all template variables should have been substituted")
	}
}

func TestRenderProfileTemplate_MissingVariable(t *testing.T) {
	tmpl := "# {{.Name}} - {{.Missing}}"
	vars := map[string]string{"Name": "Test"}

	result, err := RenderProfileTemplate(tmpl, vars)
	if err != nil {
		t.Fatalf("RenderProfileTemplate returned error: %v", err)
	}

	// Missing variables are replaced with empty string
	if strings.Contains(result, "{{.Missing}}") {
		t.Error("missing variable {{.Missing}} should have been substituted with empty string")
	}
}

func TestRenderProfileTemplate_NoVariables(t *testing.T) {
	tmpl := "# Static content\nNo variables here."
	result, err := RenderProfileTemplate(tmpl, nil)
	if err != nil {
		t.Fatalf("RenderProfileTemplate returned error: %v", err)
	}
	if result != "# Static content\nNo variables here." {
		t.Errorf("static template should be returned unchanged, got %q", result)
	}
}

// --- CLI subcommand tests ---

func TestProfileSubcommands(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		wantOut string
	}{
		{
			name:    "list with no profiles",
			args:    []string{"list"},
			wantErr: false,
			wantOut: "No profiles installed",
		},
		{
			name:    "info with no profile specified",
			args:    []string{"info"},
			wantErr: true,
		},
		{
			name:    "info with nonexistent profile",
			args:    []string{"info", "nonexistent"},
			wantErr: true,
		},
		{
			name:    "search with no query",
			args:    []string{"search"},
			wantErr: true,
		},
		{
			name:    "unknown subcommand",
			args:    []string{"unknown"},
			wantErr: true,
		},
		{
			name:    "empty args",
			args:    []string{},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := RunProfile(tc.args, &out)

			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output = %q, want contains %q", out.String(), tc.wantOut)
			}
		})
	}
}

func TestProfileListWithInstalledProfiles(t *testing.T) {
	// Create a temp profiles directory with a profile
	dir := t.TempDir()
	p := &model.RoleProfile{
		ID:   "developer",
		Name: "Developer",
		Role: model.RoleDeveloper,
		Persona: model.PersonaOverride{
			Base: model.PersonaGentleman,
			Tone: "technical",
		},
		Skills: []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "test-mcp", Category: "search", Priority: "required"},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {{Rule: "qa-pass", Severity: "CRITICAL", Action: "block"}},
			},
		},
		Triggers: model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{On: model.EventPreCommit, Run: []string{"review-risk"}, Mode: model.ModeStrong},
			},
		},
	}

	profileDir := filepath.Join(dir, "developer")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a ProfileCLI with custom dir
	cli := NewProfileCLI(dir)

	var out bytes.Buffer
	err := cli.Run([]string{"list"}, &out)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !strings.Contains(out.String(), "developer") {
		t.Errorf("output should contain 'developer', got %q", out.String())
	}
}

func TestProfileInfoWithInstalledProfile(t *testing.T) {
	dir := t.TempDir()
	p := &model.RoleProfile{
		ID:          "developer",
		Name:        "Developer",
		Description: "Full Stack Developer",
		Role:        model.RoleDeveloper,
		Persona: model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "technical",
			Style: "code-first",
		},
		Skills: []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "go-testing", Priority: "secondary"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "mcp1", Category: "search", Priority: "required"},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {{Rule: "qa-pass", Severity: "CRITICAL", Action: "block"}},
			},
		},
		Triggers: model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{On: model.EventPreCommit, Run: []string{"review-risk"}, Mode: model.ModeStrong},
			},
		},
	}

	profileDir := filepath.Join(dir, "developer")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cli := NewProfileCLI(dir)

	var out bytes.Buffer
	err := cli.Run([]string{"info", "developer"}, &out)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Developer") {
		t.Errorf("output should contain profile name, got %q", output)
	}
	if !strings.Contains(output, "Full Stack Developer") {
		t.Errorf("output should contain description, got %q", output)
	}
}

func TestProfileActivateNonexistent(t *testing.T) {
	dir := t.TempDir()
	cli := NewProfileCLI(dir)

	var out bytes.Buffer
	err := cli.Run([]string{"activate", "nonexistent"}, &out)
	if err == nil {
		t.Fatal("expected error for activating nonexistent profile")
	}
}

// --- Default profiles validation ---

func TestAllDefaultProfilesAreValid(t *testing.T) {
	profilesDir := "../../internal/assets/profiles"
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		t.Fatalf("failed to read profiles directory: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no profile files found in profiles directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(profilesDir, entry.Name()))
			if err != nil {
				t.Fatalf("failed to read %s: %v", entry.Name(), err)
			}

			p, err := ParseProfileYAML(data)
			if err != nil {
				t.Fatalf("ParseProfileYAML failed for %s: %v", entry.Name(), err)
			}

			// Validate required fields
			if p.ID == "" {
				t.Error("ID should not be empty")
			}
			if p.Name == "" {
				t.Error("Name should not be empty")
			}
			if !p.Role.Valid() {
				t.Errorf("Role %q is not valid", p.Role)
			}
			if len(p.Skills) < 3 {
				t.Errorf("expected >= 3 skills, got %d", len(p.Skills))
			}
			if len(p.MCPConfig) < 2 {
				t.Errorf("expected >= 2 MCP servers, got %d", len(p.MCPConfig))
			}
			if len(p.SDDAdapt.PhaseGates) == 0 {
				t.Error("SDDAdapt.PhaseGates should not be empty")
			}
			if len(p.Triggers.Bindings) == 0 {
				t.Error("Triggers.Bindings should not be empty")
			}
		})
	}
}

// --- All default profiles are valid YAML ---

func TestAllDefaultProfilesParseAsYAML(t *testing.T) {
	profilesDir := "../../internal/assets/profiles"
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		t.Fatalf("failed to read profiles directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(profilesDir, entry.Name()))
			if err != nil {
				t.Fatalf("failed to read %s: %v", entry.Name(), err)
			}

			// Verify it's valid YAML
			var raw map[string]interface{}
			if err := yaml.Unmarshal(data, &raw); err != nil {
				t.Fatalf("invalid YAML in %s: %v", entry.Name(), err)
			}
		})
	}
}

// --- All templates render without errors ---

func TestAllTemplatesRenderWithoutErrors(t *testing.T) {
	templatesDir := "../../internal/assets/profile-templates"
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Fatalf("failed to read templates directory: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no template files found in templates directory")
	}

	vars := map[string]string{
		"Name":        "Test User",
		"Role":        "developer",
		"Description": "Test description",
		"Tone":        "test-tone",
		"Focus":       "test-focus",
		"Style":       "test-style",
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
			if err != nil {
				t.Fatalf("failed to read %s: %v", entry.Name(), err)
			}

			result, err := RenderProfileTemplate(string(data), vars)
			if err != nil {
				t.Fatalf("RenderProfileTemplate failed for %s: %v", entry.Name(), err)
			}

			if result == "" {
				t.Error("rendered template should not be empty")
			}
			if strings.Contains(result, "{{.") {
				t.Error("all template variables should have been substituted")
			}
		})
	}
}

// --- Helper functions ---

func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
