package profile_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/profile"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestNewTemplateEngine(t *testing.T) {
	eng := profile.NewTemplateEngine()
	if eng == nil {
		t.Fatal("NewTemplateEngine returned nil")
	}
}

func TestTemplateEngineRegisterAndRender(t *testing.T) {
	eng := profile.NewTemplateEngine()

	eng.RegisterBase("greeting.md", "Hello, {{.Name}}!")
	eng.RegisterRole(model.RoleDeveloper, "greeting.md", "Dev says: {{.Name}}")

	tests := []struct {
		name     string
		tmpl     string
		role     model.RoleEnum
		vars     map[string]string
		wantSubs string
	}{
		{
			name:     "base template renders",
			tmpl:     "greeting.md",
			role:     model.RoleCybersecurity,
			vars:     map[string]string{"Name": "Alice"},
			wantSubs: "Hello, Alice!",
		},
		{
			name:     "role template overrides base",
			tmpl:     "greeting.md",
			role:     model.RoleDeveloper,
			vars:     map[string]string{"Name": "Bob"},
			wantSubs: "Dev says: Bob",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := eng.Render(tc.tmpl, tc.role, tc.vars)
			if err != nil {
				t.Fatalf("Render returned error: %v", err)
			}
			if !strings.Contains(got, tc.wantSubs) {
				t.Errorf("Render() = %q, want substring %q", got, tc.wantSubs)
			}
		})
	}
}

func TestTemplateEngineRenderMissingTemplate(t *testing.T) {
	eng := profile.NewTemplateEngine()

	_, err := eng.Render("nonexistent.md", model.RoleDeveloper, nil)
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}

func TestTemplateEngineHasTemplate(t *testing.T) {
	eng := profile.NewTemplateEngine()
	eng.RegisterBase("base.md", "content")

	if !eng.HasTemplate("base.md", model.RoleDeveloper) {
		t.Error("HasTemplate should find base template for any role")
	}
	if eng.HasTemplate("missing.md", model.RoleDeveloper) {
		t.Error("HasTemplate should return false for unregistered template")
	}
}

func TestTemplateEngineHasRoleTemplate(t *testing.T) {
	eng := profile.NewTemplateEngine()
	eng.RegisterBase("doc.md", "base content")
	eng.RegisterRole(model.RoleCybersecurity, "doc.md", "security content")

	// Role-specific template exists
	if !eng.HasRoleTemplate("doc.md", model.RoleCybersecurity) {
		t.Error("HasRoleTemplate should find cybersecurity override")
	}
	// Different role doesn't have override
	if eng.HasRoleTemplate("doc.md", model.RoleDeveloper) {
		t.Error("HasRoleTemplate should return false for developer")
	}
}

func TestTemplateEngineListTemplates(t *testing.T) {
	eng := profile.NewTemplateEngine()
	eng.RegisterBase("a.md", "a")
	eng.RegisterBase("b.md", "b")
	eng.RegisterRole(model.RoleDeveloper, "c.md", "c")

	list := eng.ListBaseTemplates()
	if len(list) != 2 {
		t.Errorf("ListBaseTemplates returned %d items, want 2", len(list))
	}
}

func TestTemplateEngineRenderNoVars(t *testing.T) {
	eng := profile.NewTemplateEngine()
	eng.RegisterBase("plain.md", "no variables here")

	got, err := eng.Render("plain.md", model.RoleDeveloper, nil)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if got != "no variables here" {
		t.Errorf("Render() = %q, want %q", got, "no variables here")
	}
}

func TestTemplateEngineRenderMultipleVars(t *testing.T) {
	eng := profile.NewTemplateEngine()
	eng.RegisterBase("config.md", "Role: {{.Role}}, Focus: {{.Focus}}")

	got, err := eng.Render("config.md", model.RoleDeveloper, map[string]string{
		"Role":  "cybersecurity",
		"Focus": "pentesting",
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(got, "Role: cybersecurity") {
		t.Errorf("Render() = %q, want contains 'Role: cybersecurity'", got)
	}
	if !strings.Contains(got, "Focus: pentesting") {
		t.Errorf("Render() = %q, want contains 'Focus: pentesting'", got)
	}
}
