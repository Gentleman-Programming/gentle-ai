package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"gopkg.in/yaml.v3"
)

// ProfileCLI manages role-based profile operations.
type ProfileCLI struct {
	ProfilesDir string
}

// NewProfileCLI returns a ProfileCLI targeting the given profiles directory.
func NewProfileCLI(profilesDir string) *ProfileCLI {
	return &ProfileCLI{ProfilesDir: profilesDir}
}

// Run dispatches a profile subcommand and writes output to w.
func (c *ProfileCLI) Run(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile {list,generate,install,activate,info,search}")
	}

	subcmd := args[0]
	switch subcmd {
	case "list":
		return c.runList(w)
	case "generate":
		return c.runGenerate(args[1:], w)
	case "install":
		return c.runInstall(args[1:], w)
	case "activate":
		return c.runActivate(args[1:], w)
	case "info":
		return c.runInfo(args[1:], w)
	case "search":
		return c.runSearch(args[1:], w)
	default:
		return fmt.Errorf("unknown subcommand %q", subcmd)
	}
}

// RunProfile is the top-level entry point for profile subcommands.
func RunProfile(args []string, w io.Writer) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	profilesDir := filepath.Join(homeDir, ".gentle-ai", "profiles")
	cli := NewProfileCLI(profilesDir)
	return cli.Run(args, w)
}

// --- subcommand implementations ---

func (c *ProfileCLI) runList(w io.Writer) error {
	entries, err := os.ReadDir(c.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(w, "No profiles installed.")
			return nil
		}
		return fmt.Errorf("failed to read profiles directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profilePath := filepath.Join(c.ProfilesDir, entry.Name(), "profile.yaml")
		if _, err := os.Stat(profilePath); err != nil {
			// Also check profile.json for backward compat
			profilePath = filepath.Join(c.ProfilesDir, entry.Name(), "profile.json")
			if _, err := os.Stat(profilePath); err != nil {
				continue
			}
		}

		data, err := os.ReadFile(profilePath)
		if err != nil {
			continue
		}

		p, err := ParseProfileYAML(data)
		if err != nil {
			continue
		}

		fmt.Fprintf(w, "  %-20s  %s\n", p.ID, p.Name)
		count++
	}

	if count == 0 {
		fmt.Fprintln(w, "No profiles installed.")
	}

	return nil
}

func (c *ProfileCLI) runGenerate(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile generate <role>")
	}

	role := model.RoleEnum(args[0])
	if !role.Valid() {
		return fmt.Errorf("invalid role %q (valid: developer, cybersecurity, marketing, education, design, data-science)", args[0])
	}

	fmt.Fprintf(w, "Generating profile for role: %s\n", role)
	fmt.Fprintf(w, "Profile generation requires discovery engine and template engine.\n")
	fmt.Fprintf(w, "Use 'gentle-ai profile install %s' to install the default profile.\n", role)
	return nil
}

func (c *ProfileCLI) runInstall(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile install <role>")
	}

	role := model.RoleEnum(args[0])
	if !role.Valid() {
		return fmt.Errorf("invalid role %q", args[0])
	}

	// Check if already installed
	profileDir := filepath.Join(c.ProfilesDir, string(role))
	if _, err := os.Stat(profileDir); err == nil {
		fmt.Fprintf(w, "Profile %q is already installed.\n", role)
		return nil
	}

	// Load the default profile YAML
	defaultData, err := loadDefaultProfile(role)
	if err != nil {
		return fmt.Errorf("failed to load default profile for %q: %w", role, err)
	}

	// Parse to validate
	p, err := ParseProfileYAML(defaultData)
	if err != nil {
		return fmt.Errorf("invalid default profile for %q: %w", role, err)
	}

	// Save to profiles directory
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	outPath := filepath.Join(profileDir, "profile.yaml")
	if err := os.WriteFile(outPath, defaultData, 0o644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	fmt.Fprintf(w, "Installed profile %q (%s)\n", p.Name, p.ID)
	return nil
}

func (c *ProfileCLI) runActivate(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile activate <profile-id>")
	}

	id := args[0]
	profilePath := filepath.Join(c.ProfilesDir, id, "profile.yaml")

	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("profile %q not found: %w", id, err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		return fmt.Errorf("failed to parse profile %q: %w", id, err)
	}

	fmt.Fprintf(w, "Activated profile %q (%s)\n", p.Name, p.ID)
	fmt.Fprintf(w, "Role: %s\n", p.Role)
	fmt.Fprintf(w, "Persona: %s\n", p.Persona.Base)
	return nil
}

func (c *ProfileCLI) runInfo(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile info <profile-id>")
	}

	id := args[0]
	profilePath := filepath.Join(c.ProfilesDir, id, "profile.yaml")

	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("profile %q not found: %w", id, err)
	}

	p, err := ParseProfileYAML(data)
	if err != nil {
		return fmt.Errorf("failed to parse profile %q: %w", id, err)
	}

	fmt.Fprintf(w, "Profile: %s\n", p.Name)
	fmt.Fprintf(w, "ID:      %s\n", p.ID)
	fmt.Fprintf(w, "Role:    %s\n", p.Role)
	fmt.Fprintf(w, "Description: %s\n", p.Description)
	fmt.Fprintf(w, "\nPersona:\n")
	fmt.Fprintf(w, "  Base:  %s\n", p.Persona.Base)
	fmt.Fprintf(w, "  Tone:  %s\n", p.Persona.Tone)
	fmt.Fprintf(w, "  Style: %s\n", p.Persona.Style)
	fmt.Fprintf(w, "\nSkills (%d):\n", len(p.Skills))
	for _, s := range p.Skills {
		fmt.Fprintf(w, "  - %s (%s)\n", s.ID, s.Priority)
	}
	fmt.Fprintf(w, "\nMCP Servers (%d):\n", len(p.MCPConfig))
	for _, m := range p.MCPConfig {
		fmt.Fprintf(w, "  - %s [%s] %s\n", m.Name, m.Category, m.Priority)
	}
	if len(p.SDDAdapt.PhaseGates) > 0 {
		fmt.Fprintf(w, "\nSDD Phase Gates:\n")
		for phase, rules := range p.SDDAdapt.PhaseGates {
			fmt.Fprintf(w, "  %s: %d gate(s)\n", phase, len(rules))
		}
	}
	if len(p.Triggers.Bindings) > 0 {
		fmt.Fprintf(w, "\nTrigger Bindings (%d):\n", len(p.Triggers.Bindings))
		for _, b := range p.Triggers.Bindings {
			fmt.Fprintf(w, "  on %s → run %s (%s)\n", b.On, strings.Join(b.Run, ", "), b.Mode)
		}
	}
	return nil
}

func (c *ProfileCLI) runSearch(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gentle-ai profile search <query>")
	}

	query := strings.ToLower(strings.Join(args, " "))
	fmt.Fprintf(w, "Searching registry for %q...\n", query)
	fmt.Fprintf(w, "Registry search requires network access.\n")
	fmt.Fprintf(w, "Use 'gentle-ai profile list' to see locally installed profiles.\n")
	return nil
}

// --- YAML parsing ---

// yamlRoleProfile is an intermediate struct for YAML deserialization.
// The YAML keys use snake_case while the Go struct uses PascalCase JSON tags.
type yamlRoleProfile struct {
	ID          string              `yaml:"id"`
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Role        string              `yaml:"role"`
	Focus       []string            `yaml:"focus"`
	Persona     yamlPersonaOverride `yaml:"persona"`
	Skills      []yamlSkillRef      `yaml:"skills"`
	MCPConfig   []yamlMCPServerRef  `yaml:"mcp_config"`
	SDDAdapt    yamlSDDAdaptations  `yaml:"sdd_adaptations"`
	Triggers    yamlTriggerRuleSet  `yaml:"triggers"`
	ModelProfile yamlModelProfile   `yaml:"model_profile"`
	Quality     yamlQualityScore    `yaml:"quality"`
	Metadata    yamlProfileMetadata `yaml:"metadata"`
}

type yamlPersonaOverride struct {
	Base  string   `yaml:"base"`
	Tone  string   `yaml:"tone"`
	Style string   `yaml:"style"`
	Focus string   `yaml:"focus"`
	Rules []string `yaml:"rules"`
}

type yamlSkillRef struct {
	ID        string  `yaml:"id"`
	Priority  string  `yaml:"priority"`
	Relevance float64 `yaml:"relevance"`
}

type yamlMCPServerRef struct {
	Name         string            `yaml:"name"`
	URL          string            `yaml:"url"`
	Command      string            `yaml:"command"`
	Args         []string          `yaml:"args"`
	Category     string            `yaml:"category"`
	Priority     string            `yaml:"priority"`
	QualityScore float64           `yaml:"quality_score"`
	FreeTier     bool              `yaml:"free_tier"`
	RiskLevel    string            `yaml:"risk_level"`
	EnvVars      map[string]string `yaml:"env_vars"`
}

type yamlGateRule struct {
	Phase    string `yaml:"phase"`
	Rule     string `yaml:"rule"`
	Severity string `yaml:"severity"`
	Source   string `yaml:"source"`
	Action   string `yaml:"action"`
	Message  string `yaml:"message"`
}

type yamlSDDAdaptations struct {
	PhaseGates  map[string][]yamlGateRule `yaml:"phase_gates"`
	PhaseSkip   []string                  `yaml:"phase_skip"`
	PhaseExtra  []string                  `yaml:"phase_extra"`
	PhaseModels map[string]string         `yaml:"phase_models"`
}

type yamlTriggerWhen struct {
	Always       bool     `yaml:"always"`
	PathGlobs    []string `yaml:"path_globs"`
	MinDiffLines int      `yaml:"min_diff_lines"`
	Phases       []string `yaml:"phases"`
	Combine      string   `yaml:"combine"`
}

type yamlTriggerBinding struct {
	On     string           `yaml:"on"`
	When   yamlTriggerWhen  `yaml:"when"`
	Run    []string         `yaml:"run"`
	Mode   string           `yaml:"mode"`
	Reason string           `yaml:"reason"`
}

type yamlTriggerRuleSet struct {
	Events   []string            `yaml:"events"`
	Bindings []yamlTriggerBinding `yaml:"bindings"`
}

type yamlModelProfile struct {
	Name              string                          `yaml:"name"`
	OrchestratorModel yamlModelAssignment             `yaml:"orchestrator_model"`
	PhaseAssignments  map[string]yamlModelAssignment   `yaml:"phase_assignments"`
}

type yamlModelAssignment struct {
	ProviderID string `yaml:"provider_id"`
	ModelID    string `yaml:"model_id"`
	Effort     string `yaml:"effort"`
}

type yamlQualityScore struct {
	Overall         float64           `yaml:"overall"`
	PersonaMatch    float64           `yaml:"persona_match"`
	SkillRelevance  float64           `yaml:"skill_relevance"`
	MCPUtility      float64           `yaml:"mcp_utility"`
	SDDQuality      float64           `yaml:"sdd_quality"`
	TriggerAccuracy float64           `yaml:"trigger_accuracy"`
	ValidatedAt     string            `yaml:"validated_at"`
	Details         map[string]string `yaml:"details"`
}

type yamlProfileMetadata struct {
	Author      string   `yaml:"author"`
	Version     string   `yaml:"version"`
	Tags        []string `yaml:"tags"`
	Downloads   int      `yaml:"downloads"`
	LastUpdated string   `yaml:"last_updated"`
	License     string   `yaml:"license"`
	Repository  string   `yaml:"repository"`
}

// ParseProfileYAML parses a YAML byte slice into a RoleProfile.
func ParseProfileYAML(data []byte) (*model.RoleProfile, error) {
	var raw yamlRoleProfile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}

	if raw.ID == "" {
		return nil, fmt.Errorf("missing required field: id")
	}
	if raw.Role == "" {
		return nil, fmt.Errorf("missing required field: role")
	}

	role := model.RoleEnum(raw.Role)
	if !role.Valid() {
		return nil, fmt.Errorf("invalid role %q", raw.Role)
	}

	p := &model.RoleProfile{
		ID:          raw.ID,
		Name:        raw.Name,
		Description: raw.Description,
		Role:        role,
		Focus:       raw.Focus,
	}

	// Persona
	p.Persona = model.PersonaOverride{
		Base:  model.PersonaID(raw.Persona.Base),
		Tone:  raw.Persona.Tone,
		Style: raw.Persona.Style,
		Focus: raw.Persona.Focus,
		Rules: raw.Persona.Rules,
	}

	// Skills
	p.Skills = make([]model.SkillRef, len(raw.Skills))
	for i, s := range raw.Skills {
		p.Skills[i] = model.SkillRef{
			ID:        model.SkillID(s.ID),
			Priority:  s.Priority,
			Relevance: s.Relevance,
		}
	}

	// MCP Config
	p.MCPConfig = make([]model.MCPServerRef, len(raw.MCPConfig))
	for i, m := range raw.MCPConfig {
		p.MCPConfig[i] = model.MCPServerRef{
			Name:         m.Name,
			URL:          m.URL,
			Command:      m.Command,
			Args:         m.Args,
			Category:     m.Category,
			Priority:     m.Priority,
			QualityScore: m.QualityScore,
			FreeTier:     m.FreeTier,
			RiskLevel:    m.RiskLevel,
			EnvVars:      m.EnvVars,
		}
	}

	// SDD Adaptations
	p.SDDAdapt = model.SDDAdaptations{
		PhaseSkip:   raw.SDDAdapt.PhaseSkip,
		PhaseExtra:  raw.SDDAdapt.PhaseExtra,
		PhaseModels: raw.SDDAdapt.PhaseModels,
	}
	if raw.SDDAdapt.PhaseGates != nil {
		p.SDDAdapt.PhaseGates = make(map[string][]model.GateRule)
		for phase, rules := range raw.SDDAdapt.PhaseGates {
			gateRules := make([]model.GateRule, len(rules))
			for i, r := range rules {
				gateRules[i] = model.GateRule{
					Phase:    r.Phase,
					Rule:     r.Rule,
					Severity: r.Severity,
					Source:   r.Source,
					Action:   r.Action,
					Message:  r.Message,
				}
			}
			p.SDDAdapt.PhaseGates[phase] = gateRules
		}
	}

	// Triggers
	p.Triggers = model.TriggerRuleSet{
		Events: make([]model.TriggerEvent, len(raw.Triggers.Events)),
	}
	for i, e := range raw.Triggers.Events {
		p.Triggers.Events[i] = model.TriggerEvent(e)
	}
	p.Triggers.Bindings = make([]model.TriggerBinding, len(raw.Triggers.Bindings))
	for i, b := range raw.Triggers.Bindings {
		p.Triggers.Bindings[i] = model.TriggerBinding{
			On: model.TriggerEvent(b.On),
			When: model.TriggerWhen{
				Always:       b.When.Always,
				PathGlobs:    b.When.PathGlobs,
				MinDiffLines: b.When.MinDiffLines,
				Phases:       b.When.Phases,
				Combine:      b.When.Combine,
			},
			Run:    b.Run,
			Mode:   model.TriggerMode(b.Mode),
			Reason: b.Reason,
		}
	}

	// Model Profile
	p.ModelProfile = model.Profile{
		Name: raw.ModelProfile.Name,
		OrchestratorModel: model.ModelAssignment{
			ProviderID: raw.ModelProfile.OrchestratorModel.ProviderID,
			ModelID:    raw.ModelProfile.OrchestratorModel.ModelID,
			Effort:     raw.ModelProfile.OrchestratorModel.Effort,
		},
	}
	if raw.ModelProfile.PhaseAssignments != nil {
		p.ModelProfile.PhaseAssignments = make(map[string]model.ModelAssignment)
		for phase, a := range raw.ModelProfile.PhaseAssignments {
			p.ModelProfile.PhaseAssignments[phase] = model.ModelAssignment{
				ProviderID: a.ProviderID,
				ModelID:    a.ModelID,
				Effort:     a.Effort,
			}
		}
	}

	// Quality
	p.Quality = model.QualityScore{
		Overall:         raw.Quality.Overall,
		PersonaMatch:    raw.Quality.PersonaMatch,
		SkillRelevance:  raw.Quality.SkillRelevance,
		MCPUtility:      raw.Quality.MCPUtility,
		SDDQuality:      raw.Quality.SDDQuality,
		TriggerAccuracy: raw.Quality.TriggerAccuracy,
		ValidatedAt:     raw.Quality.ValidatedAt,
		Details:         raw.Quality.Details,
	}

	// Metadata
	p.Metadata = model.ProfileMetadata{
		Author:      raw.Metadata.Author,
		Version:     raw.Metadata.Version,
		Tags:        raw.Metadata.Tags,
		Downloads:   raw.Metadata.Downloads,
		LastUpdated: raw.Metadata.LastUpdated,
		License:     raw.Metadata.License,
		Repository:  raw.Metadata.Repository,
	}

	return p, nil
}

// RenderProfileTemplate renders a Go text template with the given variables.
func RenderProfileTemplate(tmplContent string, vars map[string]string) (string, error) {
	t, err := template.New("profile").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// loadDefaultProfile returns the default profile YAML for the given role.
// Default profiles are embedded in the binary via the profiles directory.
func loadDefaultProfile(role model.RoleEnum) ([]byte, error) {
	filename := string(role) + ".yaml"
	profilesDir := filepath.Join("internal", "assets", "profiles")

	data, err := os.ReadFile(filepath.Join(profilesDir, filename))
	if err != nil {
		return nil, fmt.Errorf("default profile %q not found: %w", role, err)
	}

	return data, nil
}
