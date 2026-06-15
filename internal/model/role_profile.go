package model

// RoleEnum identifies the domain this profile serves.
type RoleEnum string

const (
	// RoleDeveloper is the standard software development profile.
	RoleDeveloper RoleEnum = "developer"
	// RoleCybersecurity is the offensive/defensive security profile.
	RoleCybersecurity RoleEnum = "cybersecurity"
	// RoleMarketing is the marketing and content strategy profile.
	RoleMarketing RoleEnum = "marketing"
	// RoleEducation is the academic and teaching profile.
	RoleEducation RoleEnum = "education"
	// RoleDesign is the UI/UX and visual design profile.
	RoleDesign RoleEnum = "design"
	// RoleDataScience is the analytics and ML profile.
	RoleDataScience RoleEnum = "data-science"
	// RoleCustom is a user-defined profile not matching a built-in role.
	RoleCustom RoleEnum = "custom"
)

// Valid reports whether the role is a recognized value.
func (r RoleEnum) Valid() bool {
	switch r {
	case RoleDeveloper, RoleCybersecurity, RoleMarketing,
		RoleEducation, RoleDesign, RoleDataScience, RoleCustom:
		return true
	default:
		return false
	}
}

// PersonaOverride holds role-specific persona customizations layered on top
// of the base PersonaID. When Base is PersonaCustom, the override fields
// are used directly. Otherwise they append/modify the base persona.
type PersonaOverride struct {
	Base  PersonaID `json:"base"`              // gentleman, neutral, custom
	Tone  string    `json:"tone,omitempty"`     // "direct, adversarial"
	Style string    `json:"style,omitempty"`    // "code-first, evidence-based"
	Focus string    `json:"focus,omitempty"`    // "attack vectors and threat models"
	Rules []string  `json:"rules,omitempty"`    // additional persona rules
}

// SkillRef references a skill to load, with role-specific priority.
type SkillRef struct {
	ID        SkillID  `json:"id"`
	Priority  string   `json:"priority"`               // "primary", "secondary", "shared"
	Relevance float64  `json:"relevance,omitempty"`    // 0.0-1.0 discovery score
}

// MCPServerRef describes a single MCP server recommended for this role.
type MCPServerRef struct {
	Name         string            `json:"name"`
	URL          string            `json:"url,omitempty"`
	Command      string            `json:"command,omitempty"`      // npx, docker, etc.
	Args         []string          `json:"args,omitempty"`
	Category     string            `json:"category"`               // dast, sast, search, etc.
	Priority     string            `json:"priority"`               // required, recommended, optional
	QualityScore float64           `json:"quality_score,omitempty"` // 0-100
	FreeTier     bool              `json:"free_tier"`
	RiskLevel    string            `json:"risk_level,omitempty"`   // low, medium, high
	EnvVars      map[string]string `json:"env_vars,omitempty"`
}

// GateRule defines a quality gate enforced during a specific SDD phase.
type GateRule struct {
	Phase    string `json:"phase"`              // sdd-design, sdd-apply, sdd-verify
	Rule     string `json:"rule"`               // "stride-threat-model", "sast-scan"
	Severity string `json:"severity"`           // CRITICAL, HIGH, MEDIUM, LOW
	Source   string `json:"source"`             // owasp, qa, content, security, custom
	Action   string `json:"action,omitempty"`   // block, warn, suggest
	Message  string `json:"message,omitempty"`  // human-readable gate message
}

// SDDAdaptations customizes how the SDD pipeline behaves for this role.
type SDDAdaptations struct {
	PhaseGates  map[string][]GateRule `json:"phase_gates"`            // phase -> gates
	PhaseSkip   []string              `json:"phase_skip,omitempty"`    // phases to skip
	PhaseExtra  []string              `json:"phase_extra,omitempty"`   // extra phases to add
	PhaseModels map[string]string     `json:"phase_models,omitempty"`  // phase -> model override (alias)
}

// QualityScore captures the validation metrics for a generated profile.
type QualityScore struct {
	Overall         float64           `json:"overall"`          // 0-100
	PersonaMatch    float64           `json:"persona_match"`    // tone consistency
	SkillRelevance  float64           `json:"skill_relevance"`  // % of skills used
	MCPUtility      float64           `json:"mcp_utility"`      // % of MCPs invoked
	SDDQuality      float64           `json:"sdd_quality"`      // implementation score
	TriggerAccuracy float64           `json:"trigger_accuracy"` // true positive rate
	ValidatedAt     string            `json:"validated_at,omitempty"`
	Details         map[string]string `json:"details,omitempty"`
}

// ProfileMetadata holds community/registry metadata.
type ProfileMetadata struct {
	Author      string   `json:"author,omitempty"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Downloads   int      `json:"downloads,omitempty"`
	LastUpdated string   `json:"last_updated,omitempty"`
	License     string   `json:"license,omitempty"`
	Repository  string   `json:"repository,omitempty"`
}

// RoleProfile is the complete role-based configuration package.
// It extends model.Profile (model routing) with persona, skills, MCP,
// SDD adaptations, and trigger rules — all scoped to a specific domain.
type RoleProfile struct {
	// Identity
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Role        RoleEnum `json:"role"`
	Focus       []string `json:"focus,omitempty"`

	// Configuration axes
	Persona   PersonaOverride `json:"persona"`
	Skills    []SkillRef      `json:"skills"`
	MCPConfig []MCPServerRef  `json:"mcp_config"`
	SDDAdapt  SDDAdaptations  `json:"sdd_adaptations"`
	Triggers  TriggerRuleSet  `json:"triggers"`

	// Model routing — wraps the existing Profile for backward compat
	ModelProfile Profile `json:"model_profile"`

	// Quality & metadata
	Quality  QualityScore    `json:"quality"`
	Metadata ProfileMetadata `json:"metadata"`
}
