package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Initializer orquesta la creación del andamiaje de Axiom sobre repositorios nuevos o existentes.
type Initializer struct {
	manager  *Manager
	detector *Detector
}

// NewInitializer construye una instancia del inicializador.
func NewInitializer(mgr *Manager, det *Detector) *Initializer {
	if det == nil {
		det = NewDetector()
	}
	return &Initializer{
		manager:  mgr,
		detector: det,
	}
}

// Init inicializa un workspace generando axiom.yaml, carpetas canónicas y registrándolo en el Hub.
func (ini *Initializer) Init(opts InitOptions) (*InitResult, error) {
	if opts.Path == "" {
		opts.Path = "."
	}

	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("ruta de inicialización inválida: %w", err)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("no se pudo asegurar la existencia del directorio destino: %w", err)
	}

	if opts.Name == "" {
		opts.Name = filepath.Base(absPath)
	}
	if opts.Topology == "" {
		opts.Topology = "monorepo-embedded"
	}

	configPath := filepath.Join(absPath, "axiom.yaml")
	alreadyExisted := fileExists(configPath)
	createdFiles := make([]string, 0)

	// 1. Si no existe axiom.yaml o se fuerza la recreación
	if !alreadyExisted || opts.Force {
		tech, err := ini.detector.Detect(absPath)
		if err != nil {
			tech = &TechDetection{
				PrimaryLanguage:  "generic",
				RecommendedRoles: map[string][]string{"core": {"generic"}, "qa": {"verification"}},
			}
		}

		yamlContent := buildAxiomYaml(opts.Name, opts.Topology, tech)
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			return nil, fmt.Errorf("fallo al escribir %s: %w", configPath, err)
		}
		createdFiles = append(createdFiles, configPath)
	}

	// 2. Crear carpetas canónicas del arnés SDD
	standardDirs := []string{
		filepath.Join(absPath, ".axiom", "inbox", "skills"),
		filepath.Join(absPath, "openspec", "specs"),
		filepath.Join(absPath, "openspec", "changes"),
	}

	for _, d := range standardDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("error creando directorio estándar %s: %w", d, err)
		}
	}

	// 3. Registrar en el catálogo global si el gestor está provisto
	var record WorkspaceRecord
	if ini.manager != nil {
		rec, err := ini.manager.Register(absPath, opts.Name, opts.Topology)
		if err != nil {
			return nil, fmt.Errorf("proyecto configurado pero no pudo registrarse en el Hub: %w", err)
		}
		_, _ = ini.manager.SetActive(rec.ID)
		record = *rec
	} else {
		record = WorkspaceRecord{
			ID:           slugify(opts.Name),
			Name:         opts.Name,
			Path:         absPath,
			Topology:     opts.Topology,
			IsConfigured: true,
		}
	}

	return &InitResult{
		ConfigPath:     configPath,
		Record:         record,
		CreatedFiles:   createdFiles,
		AlreadyExisted: alreadyExisted && !opts.Force,
	}, nil
}

func buildAxiomYaml(name, topology string, tech *TechDetection) string {
	coreTechList := ""
	if roles, ok := tech.RecommendedRoles["core"]; ok && len(roles) > 0 {
		for _, t := range roles {
			coreTechList += fmt.Sprintf("      - %q\n", t)
		}
	} else {
		coreTechList = fmt.Sprintf("      - %q\n", tech.PrimaryLanguage)
	}

	qaTechList := ""
	if roles, ok := tech.RecommendedRoles["qa"]; ok && len(roles) > 0 {
		for _, t := range roles {
			qaTechList += fmt.Sprintf("      - %q\n", t)
		}
	} else {
		qaTechList = "      - \"verification\"\n"
	}

	return fmt.Sprintf(`workspace:
  name: %q
  topology: %q
  specs_repository: "openspec"
  root: "."

roles:
  core:
    name: %q
    repositories:
      - path: "."
    tech:
%s
  qa:
    name: "Quality Assurance & Verification"
    repositories:
      - path: "."
    tech:
%s
governance:
  language: "es"
  shared_memory: "engram"
`, name, topology, name+" Core Engine", strings.TrimRight(coreTechList, "\n"), strings.TrimRight(qaTechList, "\n"))
}
