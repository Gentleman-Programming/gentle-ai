package profile

import (
	"math"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// DefaultMinScore is the default minimum overall quality score for a
// profile to be considered qualified.
const DefaultMinScore = 80.0

// ProfileValidator checks a generated profile for completeness and
// quality. It computes a QualityScore breakdown and compares the
// overall score against MinOverallScore.
type ProfileValidator struct {
	// MinOverallScore is the threshold that a profile's Overall score
	// must meet or exceed to be considered qualified. Defaults to 80.0.
	MinOverallScore float64
}

// NewProfileValidator returns a validator with default thresholds.
func NewProfileValidator() *ProfileValidator {
	return &ProfileValidator{
		MinOverallScore: DefaultMinScore,
	}
}

// Validate computes a QualityScore for the given profile. The score
// breakdown reflects completeness across persona, skills, MCP config,
// SDD adaptations, and trigger rules. Returns the score and nil error
// always — the caller uses IsQualified to decide pass/fail.
func (v *ProfileValidator) Validate(p *model.RoleProfile) (*model.QualityScore, error) {
	personaMatch := scorePersona(p)
	skillRelevance := scoreSkills(p)
	mcpUtility := scoreMCP(p)
	sddQuality := scoreSDD(p)
	triggerAccuracy := scoreTriggers(p)

	// Weighted overall: persona 20%, skills 20%, MCP 20%, SDD 25%, triggers 15%
	overall := (personaMatch*0.20 +
		skillRelevance*0.20 +
		mcpUtility*0.20 +
		sddQuality*0.25 +
		triggerAccuracy*0.15)

	overall = math.Round(overall*10) / 10 // one decimal

	return &model.QualityScore{
		Overall:         overall,
		PersonaMatch:    personaMatch,
		SkillRelevance:  skillRelevance,
		MCPUtility:      mcpUtility,
		SDDQuality:      sddQuality,
		TriggerAccuracy: triggerAccuracy,
		ValidatedAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// IsQualified reports whether the score meets the minimum threshold.
func (v *ProfileValidator) IsQualified(score *model.QualityScore) bool {
	return score.Overall >= v.MinOverallScore
}

// --- scoring helpers ---

func scorePersona(p *model.RoleProfile) float64 {
	score := 0.0

	// Base persona selected (20 pts)
	if p.Persona.Base != "" {
		score += 20
	}
	// Tone defined (20 pts)
	if p.Persona.Tone != "" {
		score += 20
	}
	// Style defined (20 pts)
	if p.Persona.Style != "" {
		score += 20
	}
	// Focus defined (20 pts)
	if p.Persona.Focus != "" {
		score += 20
	}
	// Rules provided (20 pts)
	if len(p.Persona.Rules) > 0 {
		score += 20
	}

	return math.Min(score, 100)
}

func scoreSkills(p *model.RoleProfile) float64 {
	n := len(p.Skills)
	switch {
	case n >= 4:
		return 100
	case n == 3:
		return 80
	case n == 2:
		return 60
	case n == 1:
		return 40
	default:
		return 0
	}
}

func scoreMCP(p *model.RoleProfile) float64 {
	n := len(p.MCPConfig)
	switch {
	case n >= 3:
		return 100
	case n == 2:
		return 75
	case n == 1:
		return 50
	default:
		return 0
	}
}

func scoreSDD(p *model.RoleProfile) float64 {
	if len(p.SDDAdapt.PhaseGates) == 0 {
		return 0
	}

	score := 0.0
	// 30 pts per phase with gates, up to 100
	for _, rules := range p.SDDAdapt.PhaseGates {
		if len(rules) > 0 {
			score += 30
		}
	}
	// Bonus for having phase_skip or phase_extra defined
	if len(p.SDDAdapt.PhaseSkip) > 0 {
		score += 5
	}
	if len(p.SDDAdapt.PhaseExtra) > 0 {
		score += 5
	}

	return math.Min(score, 100)
}

func scoreTriggers(p *model.RoleProfile) float64 {
	n := len(p.Triggers.Bindings)
	switch {
	case n >= 3:
		return 100
	case n == 2:
		return 75
	case n == 1:
		return 50
	default:
		return 0
	}
}
