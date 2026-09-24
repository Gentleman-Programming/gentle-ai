package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseConfig deserializa y valida la estructura sintáctica básica de axiom.yaml desde bytes.
func ParseConfig(data []byte) (*WorkspaceConfig, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("el archivo de configuración está vacío")
	}

	var cfg WorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error al parsear axiom.yaml: %w", err)
	}

	// Validaciones estructurales elementales
	if strings.TrimSpace(cfg.Workspace.Name) == "" {
		return nil, fmt.Errorf("el campo 'workspace.name' es obligatorio y no puede estar vacío")
	}

	switch cfg.Workspace.Topology {
	case TopologyMonorepoEmbedded, TopologyMonorepoDecoupled, TopologyMultirepo:
		// Topología válida
	default:
		return nil, fmt.Errorf("topología desconocida o ausente '%s'. Debe ser: %s, %s o %s",
			cfg.Workspace.Topology, TopologyMonorepoEmbedded, TopologyMonorepoDecoupled, TopologyMultirepo)
	}

	if len(cfg.Roles) == 0 {
		return nil, fmt.Errorf("debe definirse al menos un rol de desarrollo en la sección 'roles'")
	}

	for roleKey, role := range cfg.Roles {
		if strings.TrimSpace(role.Name) == "" {
			return nil, fmt.Errorf("el rol '%s' debe tener un nombre descriptivo en 'name'", roleKey)
		}
		if len(role.Repositories) == 0 {
			return nil, fmt.Errorf("el rol '%s' debe contener al menos un repositorio en 'repositories'", roleKey)
		}
		for i, repo := range role.Repositories {
			if strings.TrimSpace(repo.Path) == "" {
				return nil, fmt.Errorf("el repositorio #%d en el rol '%s' no puede tener una ruta vacía", i+1, roleKey)
			}
		}
	}

	return &cfg, nil
}

const (
	// DefaultConfigFilename es el nombre canónico del archivo de configuración de Axiom.
	DefaultConfigFilename = "axiom.yaml"

	// WorkspacePointerFilename es el nombre del archivo puntero opcional en la raíz del workspace.
	WorkspacePointerFilename = ".axiom-workspace"
)

// WorkspacePointer modela el contenido del archivo .axiom-workspace para el Patrón Canónico.
type WorkspacePointer struct {
	Config string `yaml:"config,omitempty"` // Ruta al archivo axiom.yaml (ej: "repo-specs/axiom.yaml")
	Specs  string `yaml:"specs,omitempty"`  // Ruta a la carpeta de especificaciones (ej: "repo-specs")
}

// ParsePointer deserializa el archivo puntero .axiom-workspace.
func ParsePointer(data []byte) (*WorkspacePointer, error) {
	var ptr WorkspacePointer
	if err := yaml.Unmarshal(data, &ptr); err != nil {
		return nil, fmt.Errorf("error al parsear %s: %w", WorkspacePointerFilename, err)
	}
	return &ptr, nil
}

// ResolveConfigFile resuelve la ruta efectiva al archivo axiom.yaml aplicando la cascada canónica:
// 1. Puntero .axiom-workspace en baseDir.
// 2. Archivo axiom.yaml directo en baseDir.
// 3. Fallback de auto-descubrimiento en subdirectorios típicos (repo-specs, specs, openspec, especificacion).
func ResolveConfigFile(target string) (string, error) {
	cleanTarget := filepath.Clean(target)
	info, err := os.Stat(cleanTarget)

	// Si target es un fichero existente:
	if err == nil && !info.IsDir() {
		if filepath.Base(cleanTarget) == WorkspacePointerFilename {
			return resolveFromPointerFile(cleanTarget)
		}
		return cleanTarget, nil
	}

	// Determinar el directorio base para buscar
	baseDir := cleanTarget
	if err != nil || !info.IsDir() {
		baseName := filepath.Base(cleanTarget)
		if baseName != DefaultConfigFilename && baseName != WorkspacePointerFilename && strings.Contains(baseName, ".") {
			return "", fmt.Errorf("archivo de configuración no encontrado en '%s'", cleanTarget)
		}
		baseDir = filepath.Dir(cleanTarget)
	}

	// 1. Comprobar si existe .axiom-workspace en baseDir
	pointerPath := filepath.Join(baseDir, WorkspacePointerFilename)
	if fileExists(pointerPath) {
		return resolveFromPointerFile(pointerPath)
	}

	// 2. Comprobar si existe axiom.yaml en baseDir
	directPath := filepath.Join(baseDir, DefaultConfigFilename)
	if fileExists(directPath) {
		return directPath, nil
	}

	// 3. Fallback de auto-descubrimiento en subdirectorios canónicos
	candidates := []string{"repo-specs", "specs", "openspec", ".openspec", "especificacion"}
	for _, c := range candidates {
		subPath := filepath.Join(baseDir, c, DefaultConfigFilename)
		if fileExists(subPath) {
			return subPath, nil
		}
	}

	return "", fmt.Errorf("archivo de configuración no encontrado en '%s' (ni %s ni %s)", baseDir, WorkspacePointerFilename, DefaultConfigFilename)
}

func resolveFromPointerFile(pointerPath string) (string, error) {
	data, err := os.ReadFile(pointerPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer el archivo puntero '%s': %w", pointerPath, err)
	}

	ptr, err := ParsePointer(data)
	if err != nil {
		return "", err
	}

	baseDir := filepath.Dir(pointerPath)
	if strings.TrimSpace(ptr.Config) != "" {
		targetPath := strings.TrimSpace(ptr.Config)
		if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(baseDir, targetPath)
		}
		if !fileExists(targetPath) {
			return "", fmt.Errorf("el puntero '%s' apunta a un archivo inexistente: '%s'", pointerPath, targetPath)
		}
		return filepath.Clean(targetPath), nil
	}

	if strings.TrimSpace(ptr.Specs) != "" {
		specsDir := strings.TrimSpace(ptr.Specs)
		targetPath := filepath.Join(baseDir, specsDir, DefaultConfigFilename)
		if !fileExists(targetPath) {
			return "", fmt.Errorf("el puntero '%s' indica repositorio de specs '%s', pero no contiene '%s'", pointerPath, specsDir, DefaultConfigFilename)
		}
		return filepath.Clean(targetPath), nil
	}

	return "", fmt.Errorf("el puntero '%s' debe definir 'config' o 'specs'", pointerPath)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadConfig carga el archivo axiom.yaml resolviendo punteros (.axiom-workspace) o rutas directas.
func LoadConfig(filePath string) (*WorkspaceConfig, error) {
	resolvedPath, err := ResolveConfigFile(filePath)
	if err != nil {
		// Si la resolución falla, intentar lectura directa para mantener mensajes de error heredados si aplica
		resolvedPath = filePath
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("archivo de configuración no encontrado en '%s'", filePath)
		}
		return nil, fmt.Errorf("no se pudo leer el archivo '%s': %w", resolvedPath, err)
	}

	return ParseConfig(data)
}

