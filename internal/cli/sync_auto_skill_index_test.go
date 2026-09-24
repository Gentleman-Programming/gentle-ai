package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestRunSyncAutoRegeneratesSkillIndexInMultirepo(t *testing.T) {
	home := t.TempDir()
	workspaceDir := t.TempDir()

	specsDir := filepath.Join(workspaceDir, "repo-specs")
	backendDir := filepath.Join(workspaceDir, "repo-backend")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backendDir, 0755); err != nil {
		t.Fatal(err)
	}

	canonicalConfig := filepath.Join(specsDir, "axiom.yaml")
	configContent := `
workspace:
  name: "MultirepoAutoSync"
  topology: "multirepo"
  specs_repository: "repo-specs"
roles:
  backend:
    name: "Backend Role"
    repositories:
      - path: "repo-backend"
`
	if err := os.WriteFile(canonicalConfig, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Puntero en la raíz del workspace
	pointerPath := filepath.Join(workspaceDir, ".axiom-workspace")
	if err := os.WriteFile(pointerPath, []byte("config: repo-specs/axiom.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// AGENTS.md en la raíz del workspace
	agentsPath := filepath.Join(workspaceDir, "AGENTS.md")
	agentsInitial := "# Proyecto\n\n## Overview\n\n## Skills\n\n<!-- axiom:skills-index -->\n<!-- /axiom:skills-index -->\n"
	if err := os.WriteFile(agentsPath, []byte(agentsInitial), 0644); err != nil {
		t.Fatal(err)
	}

	// Crear skills en repo-specs y repo-backend
	specSkillDir := filepath.Join(specsDir, "skills", "spec-guide")
	_ = os.MkdirAll(specSkillDir, 0755)
	_ = os.WriteFile(filepath.Join(specSkillDir, "SKILL.md"), []byte("---\nname: spec-guide\ndescription: Guia de arquitectura\n---\n"), 0644)

	backendSkillDir := filepath.Join(backendDir, "skills", "backend-guide")
	_ = os.MkdirAll(backendSkillDir, 0755)
	_ = os.WriteFile(filepath.Join(backendSkillDir, "SKILL.md"), []byte("---\nname: backend-guide\ndescription: Guia de backend\n---\n"), 0644)

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentSkills},
	}

	result, err := RunSyncWithSelectionScoped(home, workspaceDir, ScopeWorkspace, selection)
	if err != nil {
		t.Fatalf("RunSyncWithSelectionScoped error = %v", err)
	}

	if !result.SkillRegistryRefreshed {
		t.Errorf("SkillRegistryRefreshed = false, want true")
	}
	if result.SkillsIndexed < 2 {
		t.Errorf("SkillsIndexed = %d, want at least 2", result.SkillsIndexed)
	}

	// Comprobar que AGENTS.md contiene las skills descubiertas con rutas relativas
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("error leyendo AGENTS.md: %v", err)
	}

	agentsStr := string(agentsContent)
	if !strings.Contains(agentsStr, "repo-specs/skills/spec-guide/SKILL.md") {
		t.Errorf("AGENTS.md no contiene la ruta relativa de spec-guide:\n%s", agentsStr)
	}
	if !strings.Contains(agentsStr, "repo-backend/skills/backend-guide/SKILL.md") {
		t.Errorf("AGENTS.md no contiene la ruta relativa de backend-guide:\n%s", agentsStr)
	}

	// Comprobar reporte de sync
	report := RenderSyncReport(result)
	if !strings.Contains(report, "Skills indexed:") {
		t.Errorf("RenderSyncReport() no incluye 'Skills indexed:'; reporte:\n%s", report)
	}
}
