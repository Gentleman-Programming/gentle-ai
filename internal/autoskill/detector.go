package autoskill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// Detector analiza el sistema de archivos del workspace y de los repositorios asignados a cada rol.
type Detector struct {
	skillsMap []Technology
}

// NewDetector inicializa un detector con el mapa de tecnologías provisto o el mapa por defecto.
func NewDetector(customMap []Technology) *Detector {
	if len(customMap) == 0 {
		customMap = SKILLS_MAP
	}
	return &Detector{skillsMap: customMap}
}

// Detect escanea el workspace y retorna las tecnologías detectadas asociadas a sus respectivos roles.
func (d *Detector) Detect(workspaceRoot string, targetRole string) ([]DetectedTech, error) {
	var results []DetectedTech
	seen := make(map[string]bool)

	// 1. Intentar cargar axiom.yaml para conocer los roles y sus repositorios
	configPath := filepath.Join(workspaceRoot, "axiom.yaml")
	cfg, err := workspace.LoadConfig(configPath)

	targets := make(map[string][]string) // role -> []absPaths
	if err == nil && cfg != nil && len(cfg.Roles) > 0 {
		for roleID, roleDef := range cfg.Roles {
			if targetRole != "" && roleID != targetRole {
				continue
			}
			for _, repo := range roleDef.Repositories {
				p := repo.Path
				if !filepath.IsAbs(p) {
					p = filepath.Join(workspaceRoot, p)
				}
				targets[roleID] = append(targets[roleID], p)
			}
			// Si no tiene repositorios explícitos, usar workspaceRoot
			if len(roleDef.Repositories) == 0 {
				targets[roleID] = append(targets[roleID], workspaceRoot)
			}
		}
	} else {
		// Fallback: usar workspaceRoot con rol default "core"
		roleName := "core"
		if targetRole != "" {
			roleName = targetRole
		}
		targets[roleName] = []string{workspaceRoot}
	}

	// 2. Evaluar cada ruta de repositorio
	for role, paths := range targets {
		for _, p := range paths {
			if _, statErr := os.Stat(p); statErr != nil {
				continue
			}

			detectedList := d.scanDirectory(p)
			for _, tech := range detectedList {
				key := role + ":" + tech.ID
				if !seen[key] {
					seen[key] = true
					results = append(results, DetectedTech{
						Tech: tech,
						Role: role,
						Path: p,
					})
				}
			}
		}
	}

	return results, nil
}

func (d *Detector) scanDirectory(dir string) []Technology {
	var matched []Technology

	packages := extractPackageJSONDependencies(dir)
	hasGoMod := fileExists(filepath.Join(dir, "go.mod"))
	hasCargoToml := fileExists(filepath.Join(dir, "Cargo.toml"))

	for _, tech := range d.skillsMap {
		if d.matchesTech(dir, tech, packages, hasGoMod, hasCargoToml) {
			matched = append(matched, tech)
		}
	}

	return matched
}

func (d *Detector) matchesTech(dir string, tech Technology, packages map[string]bool, hasGoMod, hasCargoToml bool) bool {
	// 1. Reglas por Paquetes en package.json
	for _, pkg := range tech.Detect.Packages {
		if packages[pkg] {
			return true
		}
	}

	// 2. Reglas por Archivos de Configuración
	for _, cfg := range tech.Detect.ConfigFiles {
		if fileExists(filepath.Join(dir, cfg)) {
			return true
		}
	}

	// 3. Reglas especiales para Go / Rust
	if tech.ID == "go" && hasGoMod {
		return true
	}
	if tech.ID == "rust" && hasCargoToml {
		return true
	}

	// 4. Reglas por Extensiones de Archivo en la raíz y primer nivel
	if len(tech.Detect.FileExtensions) > 0 {
		if hasAnyExtension(dir, tech.Detect.FileExtensions) {
			return true
		}
	}

	return false
}

func extractPackageJSONDependencies(dir string) map[string]bool {
	deps := make(map[string]bool)
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return deps
	}

	var parsed struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		return deps
	}

	for k := range parsed.Dependencies {
		deps[k] = true
	}
	for k := range parsed.DevDependencies {
		deps[k] = true
	}
	for k := range parsed.PeerDependencies {
		deps[k] = true
	}
	for k := range parsed.OptionalDependencies {
		deps[k] = true
	}

	return deps
}

func hasAnyExtension(dir string, extensions []string) bool {
	extMap := make(map[string]bool)
	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extMap[strings.ToLower(ext)] = true
	}

	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".axiom" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if extMap[ext] {
			found = true
			return filepath.SkipDir
		}

		return nil
	})

	return found
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
