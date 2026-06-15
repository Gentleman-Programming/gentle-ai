package profile_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/profile"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestNewProfileValidator(t *testing.T) {
	v := profile.NewProfileValidator()
	if v == nil {
		t.Fatal("NewProfileValidator returned nil")
	}
}

func TestValidatorCompleteProfile(t *testing.T) {
	v := profile.NewProfileValidator()
	p := &model.RoleProfile{
		ID:          "dev-01",
		Name:        "Developer",
		Description: "Standard developer profile",
		Role:        model.RoleDeveloper,
		Persona: model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "technical, code-first",
			Style: "clean architecture",
			Focus: "backend systems",
			Rules: []string{"write tests first", "follow SOLID"},
		},
		Skills: []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-verify", Priority: "primary"},
			{ID: "go-testing", Priority: "secondary"},
			{ID: "branch-pr", Priority: "shared"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "context7", Category: "search", Priority: "required"},
			{Name: "playwright", Category: "testing", Priority: "recommended"},
			{Name: "github-mcp", Category: "scm", Priority: "optional"},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-design": {
					{Phase: "sdd-design", Rule: "architecture-review", Severity: "HIGH", Action: "block"},
				},
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "qa-pass", Severity: "CRITICAL", Action: "block"},
					{Phase: "sdd-apply", Rule: "lint-pass", Severity: "HIGH", Action: "block"},
				},
				"sdd-verify": {
					{Phase: "sdd-verify", Rule: "tests-pass", Severity: "CRITICAL", Action: "block"},
				},
			},
		},
		Triggers: model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{On: model.EventPreCommit, Run: []string{"review-risk"}, Mode: model.ModeStrong},
				{On: model.EventPostSDDPhase, Run: []string{"judgment-day"}, Mode: model.ModeStrong},
				{On: model.EventPrePR, Run: []string{"code-review"}, Mode: model.ModeAdvisory},
			},
		},
	}

	score, err := v.Validate(p)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if score.Overall < 80.0 {
		t.Errorf("expected overall score >= 80, got %.1f", score.Overall)
	}
}

func TestValidatorMinimalProfile(t *testing.T) {
	v := profile.NewProfileValidator()
	p := &model.RoleProfile{
		ID:          "minimal",
		Name:        "Minimal",
		Description: "Bare minimum profile",
		Role:        model.RoleDeveloper,
		Persona: model.PersonaOverride{
			Base: model.PersonaNeutral,
			Tone: "neutral",
		},
		Skills: []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
		},
	}

	score, err := v.Validate(p)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	// Minimal profile should still score, possibly below threshold
	if score.Overall < 0 || score.Overall > 100 {
		t.Errorf("score overall = %.1f, want 0-100 range", score.Overall)
	}
}

func TestValidatorIsQualified(t *testing.T) {
	v := profile.NewProfileValidator()

	tests := []struct {
		name  string
		score model.QualityScore
		want  bool
	}{
		{
			name:  "above threshold",
			score: model.QualityScore{Overall: 85.0},
			want:  true,
		},
		{
			name:  "at threshold",
			score: model.QualityScore{Overall: 80.0},
			want:  true,
		},
		{
			name:  "below threshold",
			score: model.QualityScore{Overall: 79.9},
			want:  false,
		},
		{
			name:  "way below",
			score: model.QualityScore{Overall: 30.0},
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := v.IsQualified(&tc.score); got != tc.want {
				t.Errorf("IsQualified(%.1f) = %v, want %v", tc.score.Overall, got, tc.want)
			}
		})
	}
}

func TestValidatorCustomMinScore(t *testing.T) {
	v := profile.NewProfileValidator()
	v.MinOverallScore = 50.0

	score := &model.QualityScore{Overall: 55.0}
	if !v.IsQualified(score) {
		t.Error("IsQualified should pass with lowered threshold")
	}
}

func TestValidatorEmptyProfile(t *testing.T) {
	v := profile.NewProfileValidator()
	p := &model.RoleProfile{}

	score, err := v.Validate(p)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	// Empty profile should score very low
	if score.Overall >= 80.0 {
		t.Errorf("empty profile should score below 80, got %.1f", score.Overall)
	}
}

func TestValidatorScoreBreakdown(t *testing.T) {
	v := profile.NewProfileValidator()
	p := &model.RoleProfile{
		ID:          "full-01",
		Name:        "Full Profile",
		Description: "Complete profile with all fields",
		Role:        model.RoleCybersecurity,
		Persona: model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "adversarial, security-first",
			Style: "evidence-based",
			Focus: "attack vectors",
			Rules: []string{"always verify", "document findings"},
		},
		Skills: []model.SkillRef{
			{ID: "pentest-methodology", Priority: "primary"},
			{ID: "vuln-classes", Priority: "secondary"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "nuclei-mcp", Category: "dast", Priority: "required", QualityScore: 92},
			{Name: "semgrep-mcp", Category: "sast", Priority: "recommended", QualityScore: 85},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "sast-scan", Severity: "CRITICAL", Action: "block"},
					{Phase: "sdd-apply", Rule: "sca-scan", Severity: "CRITICAL", Action: "block"},
				},
				"sdd-verify": {
					{Phase: "sdd-verify", Rule: "dast-scan", Severity: "HIGH", Action: "block"},
				},
			},
		},
	}

	score, err := v.Validate(p)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	// All sub-scores should be populated
	if score.PersonaMatch <= 0 {
		t.Error("PersonaMatch should be > 0 for complete persona")
	}
	if score.SkillRelevance <= 0 {
		t.Error("SkillRelevance should be > 0 for non-empty skills")
	}
	if score.MCPUtility <= 0 {
		t.Error("MCPUtility should be > 0 for non-empty MCP config")
	}
	if score.SDDQuality <= 0 {
		t.Error("SDDQuality should be > 0 for non-empty SDD adaptations")
	}
}
