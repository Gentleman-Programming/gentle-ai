package livingdoc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseSpecContent(t *testing.T) {
	sampleMarkdown := `# Especificación de Prueba: Autenticación OAuth

Este módulo define los contratos para la autenticación de usuarios.

---

## 1. Capacidad: auth-flow

### Requirement: Flujo de Autorización con PKCE (REQ-1.1)

El sistema DEBE soportar el flujo PKCE.

#### Scenario: Intercambio exitoso de token con verifier válido
- **DADO** un código de autorización y un verifier
- **CUANDO** el cliente solicita el token
- **ENTONCES** se expide el JWT

#### Scenario: Rechazo de verifier inválido
- **DADO** un verifier alterado
- **CUANDO** se solicita el token
- **ENTONCES** se retorna error 401

---

### Requirement: Renovación de Token de Acceso

El sistema DEBE permitir refrescar tokens.

#### Scenario: Renovación exitosa
- **DADO** un refresh token válido
- **CUANDO** se solicita la renovación
- **ENTONCES** se entrega un nuevo token
`

	indexer := NewIndexer()
	entry, warnings := indexer.ParseSpecContent(sampleMarkdown, "auth", "specs/auth/spec.md", time.Now())

	if entry == nil {
		t.Fatalf("se esperaba entry no nulo")
	}

	if entry.Domain != "auth" {
		t.Errorf("dominio esperado 'auth', obtenido '%s'", entry.Domain)
	}

	if entry.Title != "Especificación de Prueba: Autenticación OAuth" {
		t.Errorf("título incorrecto: '%s'", entry.Title)
	}

	if len(entry.Requirements) != 2 {
		t.Fatalf("se esperaban 2 requerimientos, obtenidos %d", len(entry.Requirements))
	}

	req1 := entry.Requirements[0]
	if req1.ID != "REQ-1.1" {
		t.Errorf("ID de requerimiento esperado 'REQ-1.1', obtenido '%s'", req1.ID)
	}
	if len(req1.Scenarios) != 2 {
		t.Errorf("se esperaban 2 escenarios para req1, obtenidos %d", len(req1.Scenarios))
	}

	req2 := entry.Requirements[1]
	if !strings.HasPrefix(req2.ID, "REQ-auth-") {
		t.Errorf("ID autogenerado esperado con prefijo REQ-auth-, obtenido '%s'", req2.ID)
	}
	if len(req2.Scenarios) != 1 {
		t.Errorf("se esperaba 1 escenario para req2, obtenidos %d", len(req2.Scenarios))
	}

	if entry.TotalScenarios != 3 {
		t.Errorf("total de escenarios esperado 3, obtenido %d", entry.TotalScenarios)
	}

	if len(warnings) != 0 {
		t.Errorf("no se esperaban advertencias en especificación bien formada: %v", warnings)
	}
}

func TestScanSpecsAndSyncIndex(t *testing.T) {
	tempWorkspace := t.TempDir()
	specsDir := filepath.Join(tempWorkspace, "openspec", "specs")
	indexFile := filepath.Join(tempWorkspace, "openspec", "INDEX.md")

	// Crear spec 1: dominio workspace-topology
	spec1Dir := filepath.Join(specsDir, "workspace-topology")
	_ = os.MkdirAll(spec1Dir, 0755)
	spec1Content := `# Workspace Topology Spec
### Requirement: Validación Monorepo (REQ-1.1)
#### Scenario: Monorepo válido
- DADO un monorepo
`
	_ = os.WriteFile(filepath.Join(spec1Dir, "spec.md"), []byte(spec1Content), 0644)

	// Crear spec 2: dominio autoskills
	spec2Dir := filepath.Join(specsDir, "autoskills")
	_ = os.MkdirAll(spec2Dir, 0755)
	spec2Content := `# Autoskills Catalog Spec
### Requirement: Verificación SHA-256 (REQ-2.1)
#### Scenario: Verificación exitosa
- DADO un hash SHA-256
`
	_ = os.WriteFile(filepath.Join(spec2Dir, "spec.md"), []byte(spec2Content), 0644)

	indexer := NewIndexer()
	catalog, _, err := indexer.ScanSpecs(specsDir)
	if err != nil {
		t.Fatalf("error escaneando specs: %v", err)
	}

	if len(catalog.Specs) != 2 {
		t.Fatalf("se esperaban 2 specs indexadas, obtenidas %d", len(catalog.Specs))
	}

	if catalog.TotalRequirements != 2 || catalog.TotalScenarios != 2 {
		t.Errorf("métricas incorrectas: reqs=%d, scenarios=%d", catalog.TotalRequirements, catalog.TotalScenarios)
	}

	report, err := indexer.SyncIndex(specsDir, indexFile)
	if err != nil {
		t.Fatalf("error sincronizando índice: %v", err)
	}

	if report.SpecsCount != 2 {
		t.Errorf("SpecsCount esperado 2, obtenido %d", report.SpecsCount)
	}

	// Verificar contenido de INDEX.md generado
	indexContentBytes, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatalf("no se pudo leer INDEX.md: %v", err)
	}

	indexStr := string(indexContentBytes)
	if !strings.Contains(indexStr, "Catálogo Maestro de Especificaciones Vivas") {
		t.Errorf("INDEX.md no contiene el encabezado maestro")
	}
	if !strings.Contains(indexStr, "`workspace-topology`") || !strings.Contains(indexStr, "`autoskills`") {
		t.Errorf("INDEX.md no lista los dominios esperados")
	}
}

func TestColdStartSynthesis(t *testing.T) {
	tempWorkspace := t.TempDir()

	// Simular cambio archivado
	changeDir := filepath.Join(tempWorkspace, "openspec", "changes", "archive", "2026-09-14-inc-sample-feature")
	_ = os.MkdirAll(changeDir, 0755)

	specContent := `# Sample Feature Spec
### Requirement: Regla de Negocio (REQ-01)
#### Scenario: Escenario de prueba
- DADO un sistema en estado inicial
`
	_ = os.WriteFile(filepath.Join(changeDir, "spec.md"), []byte(specContent), 0644)

	svc := NewService(tempWorkspace, nil, nil)
	entry, err := svc.ColdStart("inc-sample-feature", "sample-feature")
	if err != nil {
		t.Fatalf("ColdStart falló: %v", err)
	}

	if entry.Domain != "sample-feature" {
		t.Errorf("dominio esperado 'sample-feature', obtenido '%s'", entry.Domain)
	}

	// Comprobar que se creó en openspec/specs/sample-feature/spec.md
	expectedSpecPath := filepath.Join(tempWorkspace, "openspec", "specs", "sample-feature", "spec.md")
	if _, err := os.Stat(expectedSpecPath); os.IsNotExist(err) {
		t.Fatalf("la spec viva no fue creada en '%s'", expectedSpecPath)
	}

	// Comprobar que se generó openspec/INDEX.md automáticamente
	expectedIndexPath := filepath.Join(tempWorkspace, "openspec", "INDEX.md")
	if _, err := os.Stat(expectedIndexPath); os.IsNotExist(err) {
		t.Fatalf("INDEX.md no fue generado automáticamente tras ColdStart")
	}

	// Comprobar GetCatalog y GetSpecDetail
	catalog, err := svc.GetCatalog(context.Background())
	if err != nil || len(catalog.Specs) != 1 {
		t.Fatalf("GetCatalog falló o retornó cantidad inesperada: %v, len=%d", err, len(catalog.Specs))
	}

	detailEntry, raw, err := svc.GetSpecDetail("sample-feature")
	if err != nil || detailEntry == nil || raw == "" {
		t.Fatalf("GetSpecDetail falló: %v", err)
	}
}
