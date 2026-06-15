package model_test

import (
	"encoding/json"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestRoleEnumValid verifies that Valid accepts exactly the seven
// known role values and rejects empty, unknown, and uppercase inputs.
func TestRoleEnumValid(t *testing.T) {
	tests := []struct {
		name  string
		input model.RoleEnum
		want  bool
	}{
		{"developer", model.RoleDeveloper, true},
		{"cybersecurity", model.RoleCybersecurity, true},
		{"marketing", model.RoleMarketing, true},
		{"education", model.RoleEducation, true},
		{"design", model.RoleDesign, true},
		{"data-science", model.RoleDataScience, true},
		{"custom", model.RoleCustom, true},
		{"empty", model.RoleEnum(""), false},
		{"unknown", model.RoleEnum("xyz_nonexistent_123"), false},
		{"uppercase", model.RoleEnum("Developer"), false},
		{"with spaces", model.RoleEnum("data science"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.Valid(); got != tc.want {
				t.Errorf("RoleEnum(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestRoleEnumString verifies the string representation matches the constant value.
func TestRoleEnumString(t *testing.T) {
	tests := []struct {
		input model.RoleEnum
		want  string
	}{
		{model.RoleDeveloper, "developer"},
		{model.RoleCybersecurity, "cybersecurity"},
		{model.RoleDataScience, "data-science"},
	}
	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			if got := string(tc.input); got != tc.want {
				t.Errorf("string(RoleEnum(%q)) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestRoleProfileJSONRoundTrip verifies that a RoleProfile survives
// JSON marshal/unmarshal without data loss.
func TestRoleProfileJSONRoundTrip(t *testing.T) {
	original := model.RoleProfile{
		ID:          "cybersecurity-pentester",
		Name:        "Cybersecurity Pentester",
		Description: "Full offensive security profile with pentest tools",
		Role:        model.RoleCybersecurity,
		Focus:       []string{"pentesting", "malware-analysis"},
		Persona: model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "direct, adversarial",
			Style: "evidence-based, methodical",
			Focus: "attack vectors and threat models",
			Rules: []string{"always verify before exploiting", "document all findings"},
		},
		Skills: []model.SkillRef{
			{ID: "pentest-methodology", Priority: "primary", Relevance: 0.95},
			{ID: "vuln-classes", Priority: "secondary", Relevance: 0.80},
		},
		MCPConfig: []model.MCPServerRef{
			{
				Name:         "nuclei-mcp",
				URL:          "https://github.com/projectdiscovery/nuclei-mcp",
				Command:      "npx",
				Args:         []string{"nuclei-mcp"},
				Category:     "dast",
				Priority:     "required",
				QualityScore: 92,
				FreeTier:     true,
			},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "sast-scan", Severity: "CRITICAL", Source: "security", Action: "block"},
				},
			},
		},
		Triggers: model.TriggerRuleSet{
			Events: []model.TriggerEvent{model.EventPreCommit},
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPreCommit,
					When: model.TriggerWhen{PathGlobs: []string{"**/*.go"}},
					Run:  []string{"review-risk"},
					Mode: model.ModeStrong,
				},
			},
		},
		Quality: model.QualityScore{
			Overall:        95.0,
			PersonaMatch:   90.0,
			SkillRelevance: 95.0,
			MCPUtility:     85.0,
			SDDQuality:     98.0,
		},
		Metadata: model.ProfileMetadata{
			Author:  "security-team",
			Version: "1.0.0",
			Tags:    []string{"security", "pentest"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored model.RoleProfile
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify identity fields
	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Role != original.Role {
		t.Errorf("Role = %q, want %q", restored.Role, original.Role)
	}
	if restored.Description != original.Description {
		t.Errorf("Description = %q, want %q", restored.Description, original.Description)
	}

	// Verify persona
	if restored.Persona.Tone != original.Persona.Tone {
		t.Errorf("Persona.Tone = %q, want %q", restored.Persona.Tone, original.Persona.Tone)
	}
	if len(restored.Persona.Rules) != len(original.Persona.Rules) {
		t.Errorf("Persona.Rules len = %d, want %d", len(restored.Persona.Rules), len(original.Persona.Rules))
	}

	// Verify skills
	if len(restored.Skills) != len(original.Skills) {
		t.Fatalf("Skills len = %d, want %d", len(restored.Skills), len(original.Skills))
	}
	if restored.Skills[0].ID != original.Skills[0].ID {
		t.Errorf("Skills[0].ID = %q, want %q", restored.Skills[0].ID, original.Skills[0].ID)
	}
	if restored.Skills[0].Relevance != original.Skills[0].Relevance {
		t.Errorf("Skills[0].Relevance = %v, want %v", restored.Skills[0].Relevance, original.Skills[0].Relevance)
	}

	// Verify MCP config
	if len(restored.MCPConfig) != len(original.MCPConfig) {
		t.Fatalf("MCPConfig len = %d, want %d", len(restored.MCPConfig), len(original.MCPConfig))
	}
	if restored.MCPConfig[0].Name != original.MCPConfig[0].Name {
		t.Errorf("MCPConfig[0].Name = %q, want %q", restored.MCPConfig[0].Name, original.MCPConfig[0].Name)
	}
	if restored.MCPConfig[0].FreeTier != original.MCPConfig[0].FreeTier {
		t.Errorf("MCPConfig[0].FreeTier = %v, want %v", restored.MCPConfig[0].FreeTier, original.MCPConfig[0].FreeTier)
	}

	// Verify SDD adaptations
	if len(restored.SDDAdapt.PhaseGates) != len(original.SDDAdapt.PhaseGates) {
		t.Errorf("SDDAdapt.PhaseGates len = %d, want %d", len(restored.SDDAdapt.PhaseGates), len(original.SDDAdapt.PhaseGates))
	}

	// Verify quality
	if restored.Quality.Overall != original.Quality.Overall {
		t.Errorf("Quality.Overall = %v, want %v", restored.Quality.Overall, original.Quality.Overall)
	}

	// Verify metadata
	if restored.Metadata.Author != original.Metadata.Author {
		t.Errorf("Metadata.Author = %q, want %q", restored.Metadata.Author, original.Metadata.Author)
	}
}

// TestRoleProfileZeroValue verifies that a zero-value RoleProfile
// is valid JSON and has sensible defaults.
func TestRoleProfileZeroValue(t *testing.T) {
	var p model.RoleProfile

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal zero value failed: %v", err)
	}

	var restored model.RoleProfile
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal zero value failed: %v", err)
	}

	if restored.Role != "" {
		t.Errorf("zero-value Role = %q, want empty", restored.Role)
	}
	if restored.Skills != nil {
		t.Errorf("zero-value Skills = %v, want nil", restored.Skills)
	}
	if restored.MCPConfig != nil {
		t.Errorf("zero-value MCPConfig = %v, want nil", restored.MCPConfig)
	}
}

// TestGateRuleJSON verifies GateRule serialization.
func TestGateRuleJSON(t *testing.T) {
	rule := model.GateRule{
		Phase:    "sdd-apply",
		Rule:     "sast-scan",
		Severity: "CRITICAL",
		Source:   "security",
		Action:   "block",
		Message:  "SAST scan must pass before proceeding",
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("json.Marshal GateRule failed: %v", err)
	}

	var restored model.GateRule
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal GateRule failed: %v", err)
	}

	if restored.Phase != rule.Phase {
		t.Errorf("Phase = %q, want %q", restored.Phase, rule.Phase)
	}
	if restored.Severity != rule.Severity {
		t.Errorf("Severity = %q, want %q", restored.Severity, rule.Severity)
	}
	if restored.Action != rule.Action {
		t.Errorf("Action = %q, want %q", restored.Action, rule.Action)
	}
}

// TestPersonaOverrideOmitEmpty verifies omitempty behavior on PersonaOverride.
func TestPersonaOverrideOmitEmpty(t *testing.T) {
	po := model.PersonaOverride{
		Base: model.PersonaGentleman,
		Tone: "direct",
	}

	data, err := json.Marshal(po)
	if err != nil {
		t.Fatalf("json.Marshal PersonaOverride failed: %v", err)
	}

	// Style, Focus, Rules should be omitted
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal raw failed: %v", err)
	}

	if _, ok := raw["style"]; ok {
		t.Error("expected 'style' to be omitted from JSON")
	}
	if _, ok := raw["focus"]; ok {
		t.Error("expected 'focus' to be omitted from JSON")
	}
	if _, ok := raw["rules"]; ok {
		t.Error("expected 'rules' to be omitted from JSON")
	}
	if _, ok := raw["tone"]; !ok {
		t.Error("expected 'tone' to be present in JSON")
	}
}
