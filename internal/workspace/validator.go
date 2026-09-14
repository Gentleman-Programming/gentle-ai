package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FS es una interfaz para abstraer las operaciones del sistema de archivos y facilitar pruebas unitarias.
type FS interface {
	Stat(name string) (os.FileInfo, error)
	IsDir(path string) (bool, error)
}

// OSFileSystem es la implementación por defecto que interactúa con el sistema de archivos real del sistema operativo.
type OSFileSystem struct{}

func (OSFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (OSFileSystem) IsDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

// DefaultFS devuelve la implementación estándar del sistema de archivos.
func DefaultFS() FS {
	return OSFileSystem{}
}

// Validate comprueba deterministamente que un WorkspaceConfig cumple las reglas de topología de Axiom en baseDir.
func Validate(fs FS, cfg *WorkspaceConfig, baseDir string) (*ValidationReport, error) {
	if cfg == nil {
		return nil, fmt.Errorf("la configuración del workspace no puede ser nula")
	}

	cleanBaseDir := filepath.Clean(baseDir)
	report := &ValidationReport{
		Valid:         true,
		Topology:      cfg.Workspace.Topology,
		WorkspaceRoot: cleanBaseDir,
		CheckedPaths:  make([]string, 0),
		Errors:        make([]string, 0),
		Warnings:      make([]string, 0),
	}

	// 1. Verificar la carpeta maestra
	isDir, err := fs.IsDir(cleanBaseDir)
	if err != nil || !isDir {
		report.Valid = false
		report.Errors = append(report.Errors, fmt.Sprintf("la carpeta maestra del workspace '%s' no existe o no es un directorio accesible", cleanBaseDir))
		return report, nil
	}
	report.CheckedPaths = append(report.CheckedPaths, cleanBaseDir)

	// 2. Validación de repositorio canónico de especificaciones según la topología
	specsRepo := strings.TrimSpace(cfg.Workspace.SpecsRepository)

	switch cfg.Workspace.Topology {
	case TopologyMonorepoEmbedded:
		// En monorepo embebido, specs puede ser "." o una subcarpeta como "openspec" o "specs"
		if specsRepo != "" && specsRepo != "." {
			targetSpecs := filepath.Join(cleanBaseDir, specsRepo)
			report.CheckedPaths = append(report.CheckedPaths, targetSpecs)
			if ok, err := fs.IsDir(targetSpecs); err != nil || !ok {
				report.Valid = false
				report.Errors = append(report.Errors, fmt.Sprintf("el repositorio de especificaciones '%s' declarado para monorepo embebido no existe", specsRepo))
			}
		}

	case TopologyMonorepoDecoupled:
		// En monorepo desacoplado, specs DEBE ser un repositorio independiente (no ".")
		if specsRepo == "" || specsRepo == "." {
			report.Valid = false
			report.Errors = append(report.Errors, "la topología 'monorepo-decoupled' requiere un repositorio de especificaciones independiente en la carpeta maestra (no puede ser '.' ni estar vacío)")
		} else {
			targetSpecs := filepath.Join(cleanBaseDir, specsRepo)
			report.CheckedPaths = append(report.CheckedPaths, targetSpecs)
			if ok, err := fs.IsDir(targetSpecs); err != nil || !ok {
				report.Valid = false
				report.Errors = append(report.Errors, fmt.Sprintf("el repositorio de especificaciones desacoplado '%s' no existe en la carpeta maestra", specsRepo))
			}
		}

	case TopologyMultirepo:
		// En multirepo, es REQUISITO OBLIGATORIO tener un repositorio canónico de specs
		if specsRepo == "" || specsRepo == "." {
			report.Valid = false
			report.Errors = append(report.Errors, "los proyectos con topología 'multirepo' requieren obligatoriamente un repositorio canónico de especificaciones dentro de la carpeta maestra común ('specs_repository' no puede ser '.' ni estar vacío)")
		} else {
			targetSpecs := filepath.Join(cleanBaseDir, specsRepo)
			report.CheckedPaths = append(report.CheckedPaths, targetSpecs)
			if ok, err := fs.IsDir(targetSpecs); err != nil || !ok {
				report.Valid = false
				report.Errors = append(report.Errors, fmt.Sprintf("el repositorio canónico de especificaciones '%s' no existe en la carpeta maestra", specsRepo))
			}
		}
	}

	// 3. Validación de repositorios asignados a cada rol
	for roleKey, role := range cfg.Roles {
		for _, repo := range role.Repositories {
			cleanRepoPath := filepath.Clean(strings.TrimSpace(repo.Path))
			var fullRepoPath string
			if filepath.IsAbs(cleanRepoPath) {
				fullRepoPath = cleanRepoPath
			} else {
				fullRepoPath = filepath.Join(cleanBaseDir, cleanRepoPath)
			}

			report.CheckedPaths = append(report.CheckedPaths, fullRepoPath)
			if ok, err := fs.IsDir(fullRepoPath); err != nil || !ok {
				report.Valid = false
				report.Errors = append(report.Errors, fmt.Sprintf("el repositorio '%s' asignado al rol '%s' no existe en la carpeta maestra (ruta: '%s')", repo.Path, roleKey, fullRepoPath))
			}
		}
	}

	if len(report.Errors) > 0 {
		report.Valid = false
	}

	return report, nil
}
