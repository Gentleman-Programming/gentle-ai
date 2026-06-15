package profile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/profile"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// helper: create a temp profiles dir with a profile JSON file
func setupLoaderDir(t *testing.T, id string, p *model.RoleProfile) string {
	t.Helper()
	dir := t.TempDir()
	profileDir := filepath.Join(dir, id)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.json"), data, 0o644); err != nil {
		t.Fatalf("write profile.json failed: %v", err)
	}
	return dir
}

func sampleProfile(id string, role model.RoleEnum) *model.RoleProfile {
	return &model.RoleProfile{
		ID:          id,
		Name:        "Test " + string(role),
		Description: "A test profile",
		Role:        role,
		Persona: model.PersonaOverride{
			Base: model.PersonaGentleman,
			Tone: "test-tone",
		},
		Skills: []model.SkillRef{
			{ID: "sdd-apply", Priority: "primary"},
		},
		MCPConfig: []model.MCPServerRef{
			{Name: "test-mcp", Category: "test", Priority: "required"},
		},
		SDDAdapt: model.SDDAdaptations{
			PhaseGates: map[string][]model.GateRule{
				"sdd-apply": {
					{Phase: "sdd-apply", Rule: "test-rule", Severity: "CRITICAL", Action: "block"},
				},
			},
		},
		Triggers: model.TriggerRuleSet{
			Bindings: []model.TriggerBinding{
				{On: model.EventPreCommit, Run: []string{"review-risk"}, Mode: model.ModeStrong},
			},
		},
	}
}

func TestNewProfileLoader(t *testing.T) {
	loader := profile.NewProfileLoader(t.TempDir())
	if loader == nil {
		t.Fatal("NewProfileLoader returned nil")
	}
}

func TestLoaderLoad(t *testing.T) {
	p := sampleProfile("dev-01", model.RoleDeveloper)
	dir := setupLoaderDir(t, "dev-01", p)

	loader := profile.NewProfileLoader(dir)
	loaded, err := loader.Load("dev-01")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.ID != "dev-01" {
		t.Errorf("ID = %q, want %q", loaded.ID, "dev-01")
	}
	if loaded.Role != model.RoleDeveloper {
		t.Errorf("Role = %q, want %q", loaded.Role, model.RoleDeveloper)
	}
}

func TestLoaderLoadNotFound(t *testing.T) {
	loader := profile.NewProfileLoader(t.TempDir())

	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestLoaderList(t *testing.T) {
	dir := t.TempDir()
	// Create two profiles
	for _, id := range []string{"dev-01", "sec-01"} {
		role := model.RoleDeveloper
		if id == "sec-01" {
			role = model.RoleCybersecurity
		}
		p := sampleProfile(id, role)
		profileDir := filepath.Join(dir, id)
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		data, _ := json.Marshal(p)
		os.WriteFile(filepath.Join(profileDir, "profile.json"), data, 0o644)
	}

	loader := profile.NewProfileLoader(dir)
	summaries, err := loader.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("List returned %d items, want 2", len(summaries))
	}
}

func TestLoaderActivate(t *testing.T) {
	p := sampleProfile("dev-01", model.RoleDeveloper)
	dir := setupLoaderDir(t, "dev-01", p)

	loader := profile.NewProfileLoader(dir)
	plan, err := loader.Activate("dev-01")
	if err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}
	if plan == nil {
		t.Fatal("Activate returned nil plan")
	}

	// Should have persona change
	if plan.PersonaChange == nil {
		t.Error("PersonaChange should be set")
	}

	// Should have skills to add
	if len(plan.SkillsToAdd) == 0 {
		t.Error("SkillsToAdd should not be empty")
	}

	// Should have MCP servers
	if len(plan.MCPServers) == 0 {
		t.Error("MCPServers should not be empty")
	}

	// Should have SDD adaptations
	if plan.SDDAdaptations == nil {
		t.Error("SDDAdaptations should be set")
	}
}

func TestLoaderActivateNotFound(t *testing.T) {
	loader := profile.NewProfileLoader(t.TempDir())

	_, err := loader.Activate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestLoaderSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	loader := profile.NewProfileLoader(dir)

	p := sampleProfile("custom-01", model.RoleCustom)
	if err := loader.Save(p); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := loader.Load("custom-01")
	if err != nil {
		t.Fatalf("Load after Save returned error: %v", err)
	}
	if loaded.ID != "custom-01" {
		t.Errorf("ID = %q, want %q", loaded.ID, "custom-01")
	}
}

func TestLoaderRemove(t *testing.T) {
	p := sampleProfile("dev-01", model.RoleDeveloper)
	dir := setupLoaderDir(t, "dev-01", p)

	loader := profile.NewProfileLoader(dir)
	if err := loader.Remove("dev-01"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	_, err := loader.Load("dev-01")
	if err == nil {
		t.Fatal("expected error after Remove")
	}
}

func TestLoaderRemoveNotFound(t *testing.T) {
	loader := profile.NewProfileLoader(t.TempDir())

	err := loader.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for removing nonexistent profile")
	}
}

func TestActivationPlanToSelection(t *testing.T) {
	plan := &profile.ActivationPlan{
		SkillsToAdd: []model.SkillID{"sdd-apply", "go-testing"},
		MCPServers: []model.MCPServerRef{
			{Name: "test-mcp", Category: "test", Priority: "required"},
		},
	}

	s := &model.Selection{
		ModelAssignments: make(map[string]model.ModelAssignment),
	}

	plan.ToSelection(s)

	// ToSelection should not panic on empty model overrides
	if len(s.ModelAssignments) != 0 {
		t.Errorf("ModelAssignments should be empty, got %d", len(s.ModelAssignments))
	}
}

func TestLoaderActiveProfile(t *testing.T) {
	dir := t.TempDir()
	loader := profile.NewProfileLoader(dir)

	// Initially no active profile
	if loader.GetActiveID() != "" {
		t.Error("initial ActiveID should be empty")
	}

	// Create and activate
	p := sampleProfile("sec-01", model.RoleCybersecurity)
	setupDir := filepath.Join(dir, "sec-01")
	os.MkdirAll(setupDir, 0o755)
	data, _ := json.Marshal(p)
	os.WriteFile(filepath.Join(setupDir, "profile.json"), data, 0o644)

	_, err := loader.Activate("sec-01")
	if err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}

	if loader.GetActiveID() != "sec-01" {
		t.Errorf("ActiveID = %q, want %q", loader.GetActiveID(), "sec-01")
	}
}
