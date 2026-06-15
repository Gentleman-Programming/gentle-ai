package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// ProfileSummary is a lightweight view of an installed profile,
// returned by List to avoid loading full profile JSON for every entry.
type ProfileSummary struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Role      model.RoleEnum `json:"role"`
	Installed bool           `json:"installed"`
	Active    bool           `json:"active"`
}

// ActivationPlan describes what needs to change when switching to a
// new profile. The caller applies these changes via existing component
// injection mechanisms.
type ActivationPlan struct {
	PersonaChange    *model.PersonaID
	RolePersonaTone  string
	RolePersonaStyle string
	SkillsToAdd      []model.SkillID
	SkillsToRemove   []model.SkillID
	MCPServers       []model.MCPServerRef
	TriggerOverrides *model.TriggerRuleSet
	ModelOverrides   map[string]model.ModelAssignment
	SDDAdaptations   *model.SDDAdaptations
}

// ToSelection converts an ActivationPlan into the existing Selection
// fields, preserving backward compatibility with the existing persona,
// skills, and model assignment systems.
func (p *ActivationPlan) ToSelection(s *model.Selection) {
	if p.PersonaChange != nil {
		s.Persona = *p.PersonaChange
	}
	for phase, assignment := range p.ModelOverrides {
		s.ModelAssignments[phase] = assignment
	}
}

// ProfileLoader manages runtime profile state: loading from disk,
// activating profiles, listing installed profiles, and persisting
// new profiles.
type ProfileLoader struct {
	ProfilesDir string
	activeID    string
}

// NewProfileLoader returns a loader that reads and writes profiles
// under the given directory (typically ~/.gentle-ai/profiles/).
func NewProfileLoader(profilesDir string) *ProfileLoader {
	return &ProfileLoader{ProfilesDir: profilesDir}
}

// Load reads a RoleProfile from disk by ID. The profile is expected
// at {ProfilesDir}/{id}/profile.json.
func (l *ProfileLoader) Load(id string) (*model.RoleProfile, error) {
	path := filepath.Join(l.ProfilesDir, id, "profile.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", id, err)
	}

	var p model.RoleProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse profile %q: %w", id, err)
	}

	return &p, nil
}

// Activate loads the named profile and builds an ActivationPlan
// describing the component changes needed to apply it. It also
// records the profile as active.
func (l *ProfileLoader) Activate(id string) (*ActivationPlan, error) {
	p, err := l.Load(id)
	if err != nil {
		return nil, err
	}

	l.activeID = id

	plan := &ActivationPlan{
		MCPServers:       p.MCPConfig,
		SDDAdaptations:   &p.SDDAdapt,
		TriggerOverrides: &p.Triggers,
		ModelOverrides:   make(map[string]model.ModelAssignment),
	}

	// Map persona base
	if p.Persona.Base != "" {
		personaID := p.Persona.Base
		plan.PersonaChange = &personaID
	}

	// Role persona content
	plan.RolePersonaTone = p.Persona.Tone
	plan.RolePersonaStyle = p.Persona.Style

	// Collect skills to add
	for _, ref := range p.Skills {
		plan.SkillsToAdd = append(plan.SkillsToAdd, ref.ID)
	}

	return plan, nil
}

// List returns summaries of all installed profiles. It scans the
// profiles directory for subdirectories containing profile.json.
func (l *ProfileLoader) List() ([]ProfileSummary, error) {
	entries, err := os.ReadDir(l.ProfilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read profiles directory: %w", err)
	}

	var summaries []ProfileSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(l.ProfilesDir, entry.Name(), "profile.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip entries without valid profile.json
		}

		var p model.RoleProfile
		if err := json.Unmarshal(data, &p); err != nil {
			continue // skip unparseable entries
		}

		summaries = append(summaries, ProfileSummary{
			ID:        p.ID,
			Name:      p.Name,
			Role:      p.Role,
			Installed: true,
			Active:    p.ID == l.activeID,
		})
	}

	return summaries, nil
}

// Save persists a RoleProfile to disk under its ID directory.
func (l *ProfileLoader) Save(p *model.RoleProfile) error {
	dir := filepath.Join(l.ProfilesDir, p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profile: %w", err)
	}

	path := filepath.Join(dir, "profile.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	return nil
}

// Remove deletes a profile from disk by ID.
func (l *ProfileLoader) Remove(id string) error {
	dir := filepath.Join(l.ProfilesDir, id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile %q not found", id)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove profile %q: %w", id, err)
	}

	if l.activeID == id {
		l.activeID = ""
	}

	return nil
}

// GetActiveID returns the ID of the currently active profile.
func (l *ProfileLoader) GetActiveID() string {
	return l.activeID
}
