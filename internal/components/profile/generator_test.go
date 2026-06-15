package profile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/discovery"
	"github.com/gentleman-programming/gentle-ai/internal/components/profile"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// --- Mock discovery source for generator tests ---

type genMockSource struct {
	results []model.MCPServerRef
	err     error
}

func (m *genMockSource) Name() string { return "mock-gen-source" }

func (m *genMockSource) Search(_ context.Context, _ discovery.DiscoveryQuery) ([]model.MCPServerRef, error) {
	return m.results, m.err
}

func (m *genMockSource) HealthCheck(_ context.Context) error { return nil }

func TestNewProfileGenerator(t *testing.T) {
	eng := discovery.NewEngine(nil)
	templ := profile.NewTemplateEngine()
	valid := profile.NewProfileValidator()

	gen := profile.NewProfileGenerator(eng, templ, valid)
	if gen == nil {
		t.Fatal("NewProfileGenerator returned nil")
	}
}

func TestGeneratorGenerateBasic(t *testing.T) {
	src := &genMockSource{
		results: []model.MCPServerRef{
			{Name: "tool-a", Category: "search", Priority: "required", QualityScore: 90},
		},
	}
	engine := discovery.NewEngine([]discovery.SourceReader{src})
	templ := profile.NewTemplateEngine()
	templ.RegisterBase("persona.md", "# {{.Role}} Persona\nTone: {{.Tone}}")
	valid := profile.NewProfileValidator()

	gen := profile.NewProfileGenerator(engine, templ, valid)

	profile, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role:       model.RoleDeveloper,
		Focus:      []string{"backend"},
		Experience: "senior",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if profile == nil {
		t.Fatal("Generate returned nil profile")
	}
	if profile.Role != model.RoleDeveloper {
		t.Errorf("Role = %q, want %q", profile.Role, model.RoleDeveloper)
	}
}

func TestGeneratorGenerateWithDiscoveryResults(t *testing.T) {
	src := &genMockSource{
		results: []model.MCPServerRef{
			{Name: "nuclei-mcp", Category: "dast", Priority: "required", QualityScore: 92, FreeTier: true},
			{Name: "semgrep-mcp", Category: "sast", Priority: "recommended", QualityScore: 85, FreeTier: true},
		},
	}
	engine := discovery.NewEngine([]discovery.SourceReader{src})
	templ := profile.NewTemplateEngine()
	valid := profile.NewProfileValidator()

	gen := profile.NewProfileGenerator(engine, templ, valid)

	p, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role:  model.RoleCybersecurity,
		Focus: []string{"pentesting"},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Discovery results should appear in MCP config
	if len(p.MCPConfig) < 2 {
		t.Errorf("expected >= 2 MCP servers from discovery, got %d", len(p.MCPConfig))
	}
}

func TestGeneratorGenerateDiscoveryFailure(t *testing.T) {
	src := &genMockSource{err: errors.New("source unavailable")}
	engine := discovery.NewEngine([]discovery.SourceReader{src})
	templ := profile.NewTemplateEngine()
	valid := profile.NewProfileValidator()
	valid.MinOverallScore = 0 // accept any score

	gen := profile.NewProfileGenerator(engine, templ, valid)

	// Should still produce a profile using templates (graceful degradation)
	p, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role: model.RoleDeveloper,
	})
	if err != nil {
		t.Fatalf("Generate should degrade gracefully, got error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil profile even with discovery failure")
	}
	if p.Role != model.RoleDeveloper {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleDeveloper)
	}
}

func TestGeneratorValidateFails(t *testing.T) {
	src := &genMockSource{results: nil}
	engine := discovery.NewEngine([]discovery.SourceReader{src})
	templ := profile.NewTemplateEngine()
	valid := profile.NewProfileValidator()
	valid.MinOverallScore = 99.0 // impossibly high threshold

	gen := profile.NewProfileGenerator(engine, templ, valid)

	_, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role: model.RoleDeveloper,
	})
	if err == nil {
		t.Fatal("expected error when quality score is below threshold")
	}
}

func TestGeneratorSetsIdentity(t *testing.T) {
	engine := discovery.NewEngine(nil)
	templ := profile.NewTemplateEngine()
	valid := profile.NewProfileValidator()
	valid.MinOverallScore = 0 // focus on identity, not quality

	gen := profile.NewProfileGenerator(engine, templ, valid)

	p, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role:  model.RoleMarketing,
		Focus: []string{"content", "seo"},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if p.Role != model.RoleMarketing {
		t.Errorf("Role = %q, want %q", p.Role, model.RoleMarketing)
	}
	if len(p.Focus) != 2 {
		t.Errorf("Focus len = %d, want 2", len(p.Focus))
	}
	if p.ID == "" {
		t.Error("ID should be set")
	}
	if p.Name == "" {
		t.Error("Name should be set")
	}
}

func TestGeneratorPersonaFromTemplate(t *testing.T) {
	engine := discovery.NewEngine(nil)
	templ := profile.NewTemplateEngine()
	templ.RegisterBase("persona.md", "# {{.Role}} Persona\nFocus: {{.Focus}}")
	valid := profile.NewProfileValidator()
	valid.MinOverallScore = 0 // focus on persona, not quality

	gen := profile.NewProfileGenerator(engine, templ, valid)

	p, err := gen.Generate(context.Background(), profile.RoleProfileRequest{
		Role:  model.RoleEducation,
		Focus: []string{"teaching", "mentoring"},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Persona should be populated from template defaults for the role
	if p.Persona.Base == "" {
		t.Error("Persona.Base should be set")
	}
	if p.Persona.Tone == "" {
		t.Error("Persona.Tone should be set for education role")
	}
}
