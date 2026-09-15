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
	var adoptedSkillsCount int

	// 1. Si no existe axiom.yaml o se fuerza la recreación
	if !alreadyExisted || opts.Force {
		tech, err := ini.detector.Detect(absPath)
		if err != nil {
			tech = &TechDetection{
				PrimaryLanguage:  "generic",
				RecommendedRoles: map[string][]string{"core": {"generic"}, "qa": {"verification"}},
			}
		}

		// Determinar roles efectivos:
		var effectiveRoles []RoleInput
		if len(opts.Roles) > 0 {
			effectiveRoles = opts.Roles
		} else if len(tech.ConfiguredRoles) > 0 {
			effectiveRoles = tech.ConfiguredRoles
		} else {
			// Regla de negocio: Si no se proveen ni detectan roles, rol único 'fullstack'
			primaryTech := tech.Frameworks
			if len(primaryTech) == 0 && tech.PrimaryLanguage != "" {
				primaryTech = []string{tech.PrimaryLanguage}
			}
			effectiveRoles = []RoleInput{
				{
					Key:          "fullstack",
					Name:         opts.Name + " Fullstack",
					Repositories: []string{"."},
					NonBlocking:  false,
					Tech:         primaryTech,
				},
			}
		}

		specsRepo := "openspec"
		if tech.SpecsRepository != "" {
			specsRepo = tech.SpecsRepository
		}

		yamlContent := buildAxiomYamlWithRoles(opts.Name, opts.Topology, specsRepo, tech.DomainContext, effectiveRoles)
		if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
			return nil, fmt.Errorf("fallo al escribir %s: %w", configPath, err)
		}
		createdFiles = append(createdFiles, configPath)
		adoptedSkillsCount = len(tech.AdoptedSkills)
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
		ConfigPath:         configPath,
		Record:             record,
		CreatedFiles:       createdFiles,
		AlreadyExisted:     alreadyExisted && !opts.Force,
		AdoptedSkillsCount: adoptedSkillsCount,
	}, nil
}

func buildAxiomYaml(name, topology string, tech *TechDetection) string {
	var roles []RoleInput
	if tech != nil && len(tech.ConfiguredRoles) > 0 {
		roles = tech.ConfiguredRoles
	} else {
		var techList []string
		if tech != nil {
			techList = tech.Frameworks
			if len(techList) == 0 && tech.PrimaryLanguage != "" {
				techList = []string{tech.PrimaryLanguage}
			}
		}
		roles = []RoleInput{
			{
				Key:          "fullstack",
				Name:         name + " Fullstack",
				Repositories: []string{"."},
				NonBlocking:  false,
				Tech:         techList,
			},
		}
	}

	specsRepo := "openspec"
	domainContext := ""
	if tech != nil {
		if tech.SpecsRepository != "" {
			specsRepo = tech.SpecsRepository
		}
		domainContext = tech.DomainContext
	}

	return buildAxiomYamlWithRoles(name, topology, specsRepo, domainContext, roles)
}

func buildAxiomYamlWithRoles(name, topology, specsRepo, domainContext string, roles []RoleInput) string {
	var sb strings.Builder
	sb.WriteString("workspace:\n")
	sb.WriteString(fmt.Sprintf("  name: %q\n", name))
	sb.WriteString(fmt.Sprintf("  topology: %q\n", topology))
	if specsRepo == "" {
		specsRepo = "openspec"
	}
	sb.WriteString(fmt.Sprintf("  specs_repository: %q\n", specsRepo))
	sb.WriteString("  root: \".\"\n\n")

	sb.WriteString("roles:\n")
	for _, r := range roles {
		roleKey := strings.TrimSpace(r.Key)
		if roleKey == "" {
			roleKey = slugify(r.Name)
		}
		sb.WriteString(fmt.Sprintf("  %s:\n", roleKey))
		roleName := strings.TrimSpace(r.Name)
		if roleName == "" {
			roleName = strings.Title(roleKey)
		}
		sb.WriteString(fmt.Sprintf("    name: %q\n", roleName))

		gatePolicy := "blocking"
		if r.NonBlocking {
			gatePolicy = "advisory"
		}
		sb.WriteString(fmt.Sprintf("    gate_policy: %q\n", gatePolicy))

		sb.WriteString("    repositories:\n")
		repos := r.Repositories
		if len(repos) == 0 {
			repos = []string{"."}
		}
		for _, repoPath := range repos {
			sb.WriteString(fmt.Sprintf("      - path: %q\n", filepath.ToSlash(strings.TrimSpace(repoPath))))
		}

		if len(r.Tech) > 0 {
			sb.WriteString("    tech:\n")
			for _, t := range r.Tech {
				sb.WriteString(fmt.Sprintf("      - %q\n", strings.TrimSpace(t)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("governance:\n")
	sb.WriteString("  language: \"es\"\n")
	sb.WriteString("  shared_memory: \"engram\"\n")
	if strings.TrimSpace(domainContext) != "" {
		sb.WriteString("  context: |\n")
		for _, line := range strings.Split(domainContext, "\n") {
			sb.WriteString(fmt.Sprintf("    %s\n", line))
		}
	}

	return sb.String()
}
