package semantic

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// Service proporciona la interfaz de alto nivel para operaciones semánticas en el workspace.
type Service struct {
	workspaceRoot string
	detector      *Detector
	engine        *Engine
}

// NewService inicializa una nueva instancia del servicio semántico.
func NewService(workspaceRoot string, detector *Detector, engine *Engine) *Service {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	if detector == nil {
		detector = NewDetector()
	}
	if engine == nil {
		engine = NewEngine()
	}
	return &Service{
		workspaceRoot: workspaceRoot,
		detector:      detector,
		engine:        engine,
	}
}

// GetStatus evalúa la salud y configuración de los conectores semánticos en el workspace.
func (s *Service) GetStatus(ctx context.Context) (*SemanticStatus, error) {
	status := &SemanticStatus{
		ConfiguredConnector: ConnectorAuto,
		ActiveConnector:     ConnectorNativeAST,
		NativeASTReady:      true,
		Agents:              make([]AgentToolStatus, 0),
		Warnings:            make([]string, 0),
	}

	// 1. Cargar configuración de axiom.yaml si existe
	cfg, err := s.loadWorkspaceConfig()
	if err == nil && cfg != nil && cfg.Governance.SemanticAnalysis != "" {
		status.ConfiguredConnector = ConnectorType(strings.ToLower(cfg.Governance.SemanticAnalysis))
	}

	// 2. Diagnosticar agentes y herramientas
	agents := s.detector.DetectAgents()
	status.Agents = agents

	status.SerenaAvailable = s.detector.CheckSerenaAvailability(agents)
	status.CodeGraphAvailable = s.detector.CheckCodeGraphAvailability(agents)

	// 3. Resolver conector activo
	switch status.ConfiguredConnector {
	case ConnectorSerena:
		if status.SerenaAvailable {
			status.ActiveConnector = ConnectorSerena
		} else {
			status.ActiveConnector = ConnectorNativeAST
			status.Warnings = append(status.Warnings, "Serena MCP está configurado en axiom.yaml pero no se detectó en los agentes ni en PATH. Activado fallback a native-ast.")
		}
	case ConnectorCodeGraph:
		if status.CodeGraphAvailable {
			status.ActiveConnector = ConnectorCodeGraph
		} else {
			status.ActiveConnector = ConnectorNativeAST
			status.Warnings = append(status.Warnings, "CodeGraph está configurado en axiom.yaml pero no está disponible en PATH ni en agentes. Activado fallback a native-ast.")
		}
	case ConnectorNativeAST:
		status.ActiveConnector = ConnectorNativeAST
	default: // Auto
		if status.SerenaAvailable {
			status.ActiveConnector = ConnectorSerena
		} else if status.CodeGraphAvailable {
			status.ActiveConnector = ConnectorCodeGraph
		} else {
			status.ActiveConnector = ConnectorNativeAST
		}
	}

	// 4. Calcular métricas básicas del workspace
	symbols, err := s.FindSymbols(SemanticQuery{})
	if err == nil {
		status.TotalSymbols = len(symbols)
		pkgMap := make(map[string]bool)
		for _, sym := range symbols {
			pkgMap[sym.Package] = true
		}
		status.TotalPackages = len(pkgMap)
	}

	return status, nil
}

// FindSymbols extrae y filtra símbolos de código en los repositorios del workspace.
func (s *Service) FindSymbols(q SemanticQuery) ([]SymbolItem, error) {
	pathsToScan := s.resolvePathsForRole(q.Role)

	seenDirs := make(map[string]bool)
	seenSymbols := make(map[string]bool)
	var allSymbols []SymbolItem

	for roleName, dirPath := range pathsToScan {
		cleanDir := filepath.Clean(dirPath)
		if seenDirs[cleanDir] {
			continue
		}
		seenDirs[cleanDir] = true

		res, err := s.engine.AnalyzeDirectory(cleanDir, roleName)
		if err != nil {
			continue
		}
		for _, sym := range res.Symbols {
			key := sym.FilePath + ":" + sym.Name + ":" + strconv.Itoa(sym.LineNumber)
			if !seenSymbols[key] {
				seenSymbols[key] = true
				allSymbols = append(allSymbols, sym)
			}
		}
	}

	// Filtrado por query y kind
	queryLower := strings.ToLower(q.Query)
	kindFilter := q.Kind

	var filtered []SymbolItem
	for _, sym := range allSymbols {
		if queryLower != "" && !strings.Contains(strings.ToLower(sym.Name), queryLower) && !strings.Contains(strings.ToLower(sym.Signature), queryLower) {
			continue
		}
		if kindFilter != "" && sym.Kind != kindFilter {
			continue
		}
		filtered = append(filtered, sym)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Name == filtered[j].Name {
			return filtered[i].FilePath < filtered[j].FilePath
		}
		return filtered[i].Name < filtered[j].Name
	})

	return filtered, nil
}

// InspectDependencies extrae el mapa de dependencias de paquetes para un rol o para todo el workspace.
func (s *Service) InspectDependencies(role string) ([]DependencyRelation, error) {
	pathsToScan := s.resolvePathsForRole(role)

	seenDirs := make(map[string]bool)
	var allDeps []DependencyRelation
	depMap := make(map[string]bool)

	for roleName, dirPath := range pathsToScan {
		cleanDir := filepath.Clean(dirPath)
		if seenDirs[cleanDir] {
			continue
		}
		seenDirs[cleanDir] = true

		res, err := s.engine.AnalyzeDirectory(cleanDir, roleName)
		if err != nil {
			continue
		}
		for _, dep := range res.Dependencies {
			key := dep.SourcePackage + "->" + dep.TargetPackage
			if !depMap[key] {
				depMap[key] = true
				allDeps = append(allDeps, dep)
			}
		}
	}

	sort.Slice(allDeps, func(i, j int) bool {
		if allDeps[i].SourcePackage == allDeps[j].SourcePackage {
			return allDeps[i].TargetPackage < allDeps[j].TargetPackage
		}
		return allDeps[i].SourcePackage < allDeps[j].SourcePackage
	})

	return allDeps, nil
}

// resolvePathsForRole resuelve los directorios a escanear a partir de axiom.yaml o directorio actual.
func (s *Service) resolvePathsForRole(targetRole string) map[string]string {
	results := make(map[string]string)

	cfg, err := s.loadWorkspaceConfig()
	if err == nil && cfg != nil && len(cfg.Roles) > 0 {
		for rName, rCfg := range cfg.Roles {
			if targetRole != "" && rName != targetRole {
				continue
			}
			for _, repo := range rCfg.Repositories {
				p := repo.Path
				if !filepath.IsAbs(p) {
					p = filepath.Join(s.workspaceRoot, p)
				}
				results[rName] = p
			}
		}
	}

	// Si no hay roles configurados o no se encontraron rutas, escanear la raíz del workspace
	if len(results) == 0 {
		roleName := "default"
		if targetRole != "" {
			roleName = targetRole
		}
		results[roleName] = s.workspaceRoot
	}

	return results
}

func (s *Service) loadWorkspaceConfig() (*workspace.WorkspaceConfig, error) {
	configPath := s.workspaceRoot
	if fi, err := os.Stat(configPath); err == nil && fi.IsDir() {
		configPath = filepath.Join(configPath, "axiom.yaml")
	}
	return workspace.LoadConfig(configPath)
}
