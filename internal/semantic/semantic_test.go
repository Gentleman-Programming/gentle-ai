package semantic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectorAgentConfigs(t *testing.T) {
	tempHome := t.TempDir()

	// Crear archivo mcp_config.json simulado para Antigravity con Serena
	antigravityDir := filepath.Join(tempHome, ".gemini", "antigravity")
	if err := os.MkdirAll(antigravityDir, 0755); err != nil {
		t.Fatalf("error creando directorio de prueba: %v", err)
	}

	mcpContent := `{
		"mcpServers": {
			"serena": {
				"command": "npx",
				"args": ["-y", "serena-mcp"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(antigravityDir, "mcp_config.json"), []byte(mcpContent), 0644); err != nil {
		t.Fatalf("error escribiendo mcp_config.json: %v", err)
	}

	detector := NewDetectorWithCustomHome(tempHome, func(file string) (string, error) {
		if file == "codegraph" {
			return "/usr/local/bin/codegraph", nil
		}
		return "", os.ErrNotExist
	})

	agents := detector.DetectAgents()
	if len(agents) == 0 {
		t.Fatalf("se esperaban agentes detectados")
	}

	var antigravityAgent *AgentToolStatus
	for i := range agents {
		if agents[i].AgentName == "Google Antigravity" {
			antigravityAgent = &agents[i]
			break
		}
	}

	if antigravityAgent == nil || !antigravityAgent.Configured {
		t.Errorf("se esperaba que Google Antigravity estuviera configurado con serena")
	}

	if !detector.CheckSerenaAvailability(agents) {
		t.Errorf("se esperaba que Serena estuviera disponible")
	}

	if !detector.CheckCodeGraphAvailability(agents) {
		t.Errorf("se esperaba que CodeGraph estuviera disponible por PATH")
	}
}

func TestEngineExtractSymbolsAndDependencies(t *testing.T) {
	tempDir := t.TempDir()

	sampleGoCode := `// Package sample es un paquete de prueba para el motor semántico.
package sample

import (
	"context"
	"fmt"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// RepositoryManager define la interfaz para gestionar repositorios.
type RepositoryManager interface {
	GetRepository(ctx context.Context, id string) (string, error)
	ListRepositories() []string
}

// Service implementa la lógica central.
type Service struct {
	workspaceRoot string
	active        bool
}

// NewService inicializa un nuevo servicio.
func NewService(root string) *Service {
	return &Service{workspaceRoot: root, active: true}
}

// Execute ejecuta una acción sobre el servicio.
func (s *Service) Execute(action string) (bool, error) {
	fmt.Println(action)
	return true, nil
}
`

	sampleFile := filepath.Join(tempDir, "service.go")
	if err := os.WriteFile(sampleFile, []byte(sampleGoCode), 0644); err != nil {
		t.Fatalf("error escribiendo archivo go de prueba: %v", err)
	}

	engine := NewEngine()
	res, err := engine.AnalyzeDirectory(tempDir, "core")
	if err != nil {
		t.Fatalf("error analizando directorio: %v", err)
	}

	if len(res.Packages) != 1 || res.Packages[0] != "sample" {
		t.Errorf("se esperaba paquete 'sample', se obtuvo %v", res.Packages)
	}

	// Comprobar símbolos extraídos
	symbolsByName := make(map[string]SymbolItem)
	for _, s := range res.Symbols {
		symbolsByName[s.Name] = s
	}

	// 1. Interfaz
	if iface, ok := symbolsByName["RepositoryManager"]; !ok {
		t.Errorf("no se extrajo la interfaz RepositoryManager")
	} else {
		if iface.Kind != KindInterface {
			t.Errorf("tipo incorrecto para RepositoryManager: %s", iface.Kind)
		}
		if !strings.Contains(iface.Signature, "interface") {
			t.Errorf("firma incorrecta para interfaz: %s", iface.Signature)
		}
	}

	// 2. Struct
	if str, ok := symbolsByName["Service"]; !ok {
		t.Errorf("no se extrajo el struct Service")
	} else {
		if str.Kind != KindStruct {
			t.Errorf("tipo incorrecto para Service: %s", str.Kind)
		}
	}

	// 3. Función
	if fn, ok := symbolsByName["NewService"]; !ok {
		t.Errorf("no se extrajo la función NewService")
	} else {
		if fn.Kind != KindFunc {
			t.Errorf("tipo incorrecto para NewService: %s", fn.Kind)
		}
		if !strings.Contains(fn.Signature, "func NewService") {
			t.Errorf("firma incorrecta para NewService: %s", fn.Signature)
		}
	}

	// 4. Método
	if meth, ok := symbolsByName["Execute"]; !ok {
		t.Errorf("no se extrajo el método Execute")
	} else {
		if meth.Kind != KindMethod {
			t.Errorf("tipo incorrecto para Execute: %s", meth.Kind)
		}
		if meth.Receiver != "*Service" {
			t.Errorf("receptor incorrecto para Execute: %s", meth.Receiver)
		}
	}

	// Comprobar dependencias de importación
	if len(res.Dependencies) != 3 {
		t.Errorf("se esperaban 3 dependencias, se obtuvieron %d", len(res.Dependencies))
	}

	var hasWorkspaceDep bool
	for _, dep := range res.Dependencies {
		if strings.Contains(dep.TargetPackage, "workspace") {
			hasWorkspaceDep = true
			if !dep.IsInternal {
				t.Errorf("la dependencia workspace debió ser clasificada como interna")
			}
		}
	}
	if !hasWorkspaceDep {
		t.Errorf("no se detectó la dependencia con workspace")
	}
}

func TestServiceFindSymbolsAndFallback(t *testing.T) {
	tempWorkspace := t.TempDir()

	// Crear archivo axiom.yaml con conector explícito 'serena' pero sin estar instalado
	axiomYAML := `workspace:
  name: "TestProject"
  topology: "monorepo-embedded"
  specs_repository: "."
roles:
  core:
    name: "Equipo Core"
    repositories:
      - path: "."
governance:
  semantic_analysis: "serena"
`
	if err := os.WriteFile(filepath.Join(tempWorkspace, "axiom.yaml"), []byte(axiomYAML), 0644); err != nil {
		t.Fatalf("error escribiendo axiom.yaml: %v", err)
	}

	// Crear código Go
	goCode := `package testpkg
type Handler struct{}
func NewHandler() *Handler { return &Handler{} }
`
	if err := os.WriteFile(filepath.Join(tempWorkspace, "handler.go"), []byte(goCode), 0644); err != nil {
		t.Fatalf("error escribiendo handler.go: %v", err)
	}

	// Detector sin Serena ni CodeGraph
	detector := NewDetectorWithCustomHome(t.TempDir(), func(file string) (string, error) {
		return "", os.ErrNotExist
	})

	srv := NewService(tempWorkspace, detector, NewEngine())

	status, err := srv.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("error obteniendo status: %v", err)
	}

	if status.ConfiguredConnector != ConnectorSerena {
		t.Errorf("se esperaba conector configurado 'serena', se obtuvo %s", status.ConfiguredConnector)
	}

	if status.ActiveConnector != ConnectorNativeAST {
		t.Errorf("se esperaba fallback activo a 'native-ast', se obtuvo %s", status.ActiveConnector)
	}

	if len(status.Warnings) == 0 {
		t.Errorf("se esperaba advertencia por fallback de conector no disponible")
	}

	if status.TotalSymbols == 0 {
		t.Errorf("se esperaba al menos 1 símbolo indexado")
	}

	// Test filtrado de símbolos
	symbols, err := srv.FindSymbols(SemanticQuery{Query: "Handler", Kind: KindStruct})
	if err != nil {
		t.Fatalf("error buscando símbolos: %v", err)
	}
	if len(symbols) != 1 || symbols[0].Name != "Handler" {
		t.Errorf("filtro no retornó el símbolo esperado: %v", symbols)
	}
}

func TestServiceReindexCodeGraph(t *testing.T) {
	tempWorkspace := t.TempDir()
	detector := NewDetectorWithCustomHome(t.TempDir(), func(file string) (string, error) {
		return "", os.ErrNotExist
	})
	srv := NewService(tempWorkspace, detector, NewEngine())

	res, err := srv.ReindexCodeGraph(context.Background())
	if err != nil {
		t.Fatalf("error inesperado en ReindexCodeGraph: %v", err)
	}
	if !res.Success {
		t.Errorf("se esperaba éxito en la respuesta de reindexación informativa")
	}
	if res.Message == "" {
		t.Errorf("se esperaba mensaje en el resultado de reindexación")
	}
	if res.Duration == "" {
		t.Errorf("se esperaba duración en el resultado de reindexación")
	}
}

