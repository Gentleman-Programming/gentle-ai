package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/discovery"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// RoleProfileRequest is the user's input for profile generation.
type RoleProfileRequest struct {
	Role       model.RoleEnum
	Focus      []string
	Budget     string   // "free-only", "standard", "premium"
	Experience string   // "junior", "mid", "senior"
	KnownTools []string // tools to exclude from discovery
}

// ProfileGenerator orchestrates the generation pipeline:
//
//	discover → select skills → template → validate
type ProfileGenerator struct {
	discovery *discovery.DiscoveryEngine
	templates *TemplateEngine
	validator *ProfileValidator
}

// NewProfileGenerator returns a ProfileGenerator with the given dependencies.
func NewProfileGenerator(
	d *discovery.DiscoveryEngine,
	t *TemplateEngine,
	v *ProfileValidator,
) *ProfileGenerator {
	return &ProfileGenerator{
		discovery: d,
		templates: t,
		validator: v,
	}
}

// Generate produces a complete RoleProfile from a request. It queries
// the discovery engine, selects skills, applies templates, and validates
// the result. Discovery failures are handled gracefully — the profile
// is still produced from templates.
func (g *ProfileGenerator) Generate(ctx context.Context, req RoleProfileRequest) (*model.RoleProfile, error) {
	// 1. Discover MCP servers (best-effort)
	var discovered []model.MCPServerRef
	if g.discovery != nil {
		result, err := g.discovery.Search(ctx, discovery.DiscoveryQuery{
			Role:       req.Role,
			Focus:      req.Focus,
			Budget:     req.Budget,
			KnownTools: req.KnownTools,
		})
		if err == nil && result != nil {
			discovered = result.MCPServers
		}
		// On error: continue with empty discovery (graceful degradation)
	}

	// 2. Build the profile
	p := &model.RoleProfile{
		ID:          generateID(req.Role),
		Name:        generateName(req.Role),
		Description: generateDescription(req.Role, req.Focus),
		Role:        req.Role,
		Focus:       req.Focus,
	}

	// 3. Apply persona defaults for the role
	p.Persona = defaultPersona(req.Role)

	// 4. Select skills based on role
	p.Skills = selectSkills(req.Role)

	// 5. Merge discovered MCP servers into config
	p.MCPConfig = discovered

	// 6. Apply SDD adaptations
	p.SDDAdapt = defaultSDDAdaptations(req.Role)

	// 7. Apply default trigger rules
	p.Triggers = defaultTriggers(req.Role)

	// 8. Validate quality
	score, err := g.validator.Validate(p)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	p.Quality = *score

	if !g.validator.IsQualified(score) {
		return nil, fmt.Errorf(
			"profile quality score %.1f is below threshold %.1f",
			score.Overall, g.validator.MinOverallScore,
		)
	}

	return p, nil
}

// --- defaults per role ---

func generateID(role model.RoleEnum) string {
	return string(role)
}

func generateName(role model.RoleEnum) string {
	name := string(role)
	// Capitalize first letter
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	// Handle hyphenated names
	name = strings.ReplaceAll(name, "-", " ")
	return name
}

func generateDescription(role model.RoleEnum, focus []string) string {
	base := fmt.Sprintf("Auto-generated %s profile", role)
	if len(focus) > 0 {
		base += fmt.Sprintf(" focused on %s", strings.Join(focus, ", "))
	}
	return base
}

func defaultPersona(role model.RoleEnum) model.PersonaOverride {
	switch role {
	case model.RoleCybersecurity:
		return model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "adversarial, security-first",
			Style: "evidence-based, methodical",
			Focus: "attack vectors and threat models",
			Rules: []string{
				"always verify before exploiting",
				"document all findings",
				"follow responsible disclosure",
			},
		}
	case model.RoleDeveloper:
		return model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "technical, code-first",
			Style: "clean architecture, SOLID principles",
			Focus: "maintainable, testable code",
			Rules: []string{
				"write tests first",
				"follow SOLID principles",
			},
		}
	case model.RoleMarketing:
		return model.PersonaOverride{
			Base:  model.PersonaNeutral,
			Tone:  "persuasive, data-driven",
			Style: "conversion-focused, brand-consistent",
			Focus: "audience engagement and ROI",
		}
	case model.RoleEducation:
		return model.PersonaOverride{
			Base:  model.PersonaGentleman,
			Tone:  "Socratic, concept-first",
			Style: "constructivism, scaffolded learning",
			Focus: "deep understanding over memorization",
			Rules: []string{
				"ask before telling",
				"use analogies for complex concepts",
			},
		}
	case model.RoleDesign:
		return model.PersonaOverride{
			Base:  model.PersonaNeutral,
			Tone:  "visual, system-thinking",
			Style: "user-centered, accessible",
			Focus: "usability and aesthetic consistency",
		}
	case model.RoleDataScience:
		return model.PersonaOverride{
			Base:  model.PersonaNeutral,
			Tone:  "analytical, evidence-based",
			Style: "statistical rigor, reproducible",
			Focus: "data-driven decision making",
			Rules: []string{
				"always show confidence intervals",
				"validate assumptions before modeling",
			},
		}
	default:
		return model.PersonaOverride{
			Base: model.PersonaNeutral,
			Tone: "professional",
		}
	}
}

func selectSkills(role model.RoleEnum) []model.SkillRef {
	switch role {
	case model.RoleCybersecurity:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-verify", Priority: "primary"},
			{ID: "pentest-methodology", Priority: "primary"},
			{ID: "vuln-classes", Priority: "secondary"},
		}
	case model.RoleDeveloper:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-verify", Priority: "primary"},
			{ID: "go-testing", Priority: "secondary"},
			{ID: "branch-pr", Priority: "shared"},
		}
	case model.RoleMarketing:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "branch-pr", Priority: "shared"},
		}
	case model.RoleEducation:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-verify", Priority: "primary"},
			{ID: "sdd-explore", Priority: "secondary"},
		}
	case model.RoleDesign:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-design", Priority: "primary"},
		}
	case model.RoleDataScience:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
			{ID: "sdd-verify", Priority: "primary"},
		}
	default:
		return []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
		}
	}
}

func defaultTriggers(role model.RoleEnum) model.TriggerRuleSet {
	events := []model.TriggerEvent{
		model.EventPreCommit,
		model.EventPrePR,
		model.EventPostSDDPhase,
	}

	switch role {
	case model.RoleCybersecurity:
		return model.TriggerRuleSet{
			Events: events,
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPreCommit,
					When: model.TriggerWhen{PathGlobs: []string{"**/*.go", "**/*.py"}},
					Run:  []string{"review-risk"},
					Mode: model.ModeStrong,
				},
				{
					On:   model.EventPostSDDPhase,
					When: model.TriggerWhen{Phases: []string{"design", "apply", "verify"}},
					Run:  []string{"judgment-day"},
					Mode: model.ModeStrong,
				},
				{
					On:   model.EventPrePR,
					When: model.TriggerWhen{Always: true},
					Run:  []string{"code-review"},
					Mode: model.ModeAdvisory,
				},
			},
		}
	case model.RoleDeveloper:
		return model.TriggerRuleSet{
			Events: events,
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPreCommit,
					When: model.TriggerWhen{PathGlobs: []string{"**/*.go"}},
					Run:  []string{"review-risk"},
					Mode: model.ModeStrong,
				},
				{
					On:   model.EventPrePR,
					When: model.TriggerWhen{Always: true},
					Run:  []string{"code-review"},
					Mode: model.ModeAdvisory,
				},
				{
					On:   model.EventPostSDDPhase,
					When: model.TriggerWhen{Phases: []string{"apply", "verify"}},
					Run:  []string{"judgment-day"},
					Mode: model.ModeAdvisory,
				},
			},
		}
	default:
		return model.TriggerRuleSet{
			Events: events,
			Bindings: []model.TriggerBinding{
				{
					On:   model.EventPrePR,
					When: model.TriggerWhen{Always: true},
					Run:  []string{"code-review"},
					Mode: model.ModeAdvisory,
				},
			},
		}
	}
}

func defaultSDDAdaptations(role model.RoleEnum) model.SDDAdaptations {
	switch role {
	case model.RoleCybersecurity:
		return model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-design": {
					{Phase: "sdd-design", Rule: "stride-threat-model", Severity: "CRITICAL", Source: "security", Action: "block"},
				},
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "sast-scan", Severity: "CRITICAL", Source: "security", Action: "block"},
					{Phase: "sdd-apply", Rule: "sca-scan", Severity: "CRITICAL", Source: "security", Action: "block"},
					{Phase: "sdd-apply", Rule: "secrets-detection", Severity: "CRITICAL", Source: "security", Action: "block"},
				},
				"sdd-verify": {
					{Phase: "sdd-verify", Rule: "dast-scan", Severity: "HIGH", Source: "security", Action: "block"},
				},
			},
		}
	case model.RoleDeveloper:
		return model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "qa-pass", Severity: "CRITICAL", Source: "qa", Action: "block"},
					{Phase: "sdd-apply", Rule: "lint-pass", Severity: "HIGH", Source: "qa", Action: "block"},
				},
				"sdd-verify": {
					{Phase: "sdd-verify", Rule: "tests-pass", Severity: "CRITICAL", Source: "qa", Action: "block"},
				},
			},
		}
	case model.RoleMarketing:
		return model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "seo-score", Severity: "MEDIUM", Source: "content", Action: "warn"},
				},
			},
		}
	default:
		return model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "qa-pass", Severity: "CRITICAL", Source: "qa", Action: "block"},
				},
			},
		}
	}
}
