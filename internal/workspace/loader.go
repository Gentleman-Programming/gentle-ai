package workspace

import (
	"fmt"
	"os"
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

// LoadConfig carga el archivo axiom.yaml desde una ruta del sistema de archivos.
func LoadConfig(filePath string) (*WorkspaceConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("archivo de configuración no encontrado en '%s'", filePath)
		}
		return nil, fmt.Errorf("no se pudo leer el archivo '%s': %w", filePath, err)
	}

	return ParseConfig(data)
}
