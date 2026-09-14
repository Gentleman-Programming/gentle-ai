package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockFS simula un sistema de archivos en memoria para pruebas deterministas
type mockFS struct {
	existingDirs map[string]bool
}

func newMockFS() *mockFS {
	return &mockFS{
		existingDirs: make(map[string]bool),
	}
}

func (m *mockFS) addDir(path string) {
	m.existingDirs[filepath.Clean(path)] = true
}

func (m *mockFS) Stat(name string) (os.FileInfo, error) {
	if m.existingDirs[filepath.Clean(name)] {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFS) IsDir(path string) (bool, error) {
	clean := filepath.Clean(path)
	if m.existingDirs[clean] {
		return true, nil
	}
	return false, os.ErrNotExist
}

func TestValidateTopology(t *testing.T) {
	baseDir := filepath.Clean("/workspace")

	t.Run("monorepo embebido valido en raiz", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "SingleApp",
				Topology:        TopologyMonorepoEmbedded,
				SpecsRepository: ".",
			},
			Roles: map[string]RoleConfig{
				"fullstack": {
					Name: "Fullstack",
					Repositories: []RepositoryEntry{
						{Path: "."},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if !report.Valid {
			t.Errorf("se esperaba que el reporte fuera valido, errores: %v", report.Errors)
		}
	})

	t.Run("monorepo embebido con subcarpeta openspec valida", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "openspec"))

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "SingleApp",
				Topology:        TopologyMonorepoEmbedded,
				SpecsRepository: "openspec",
			},
			Roles: map[string]RoleConfig{
				"fullstack": {
					Name: "Fullstack",
					Repositories: []RepositoryEntry{
						{Path: "."},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if !report.Valid {
			t.Errorf("se esperaba valido, errores: %v", report.Errors)
		}
	})

	t.Run("monorepo desacoplado valido", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "especificacion"))
		fs.addDir(filepath.Join(baseDir, "monorepo-app"))

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "DecoupledProject",
				Topology:        TopologyMonorepoDecoupled,
				SpecsRepository: "especificacion",
			},
			Roles: map[string]RoleConfig{
				"team": {
					Name: "Team",
					Repositories: []RepositoryEntry{
						{Path: "monorepo-app"},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if !report.Valid {
			t.Errorf("se esperaba valido, errores: %v", report.Errors)
		}
	})

	t.Run("monorepo desacoplado rechazado si falta repositorio de specs", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "monorepo-app"))

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "DecoupledProject",
				Topology:        TopologyMonorepoDecoupled,
				SpecsRepository: "especificacion", // No agregada al fs
			},
			Roles: map[string]RoleConfig{
				"team": {
					Name: "Team",
					Repositories: []RepositoryEntry{
						{Path: "monorepo-app"},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if report.Valid {
			t.Errorf("se esperaba reporte invalido por falta de specs")
		}
		assertContains(t, report.Errors, "repositorio de especificaciones desacoplado 'especificacion' no existe")
	})

	t.Run("multirepo valido con roles y repositorios completos", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "especificacion"))
		fs.addDir(filepath.Join(baseDir, "frontend-app"))
		fs.addDir(filepath.Join(baseDir, "backend-api"))
		fs.addDir(filepath.Join(baseDir, "infra"))

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "FederatedPlatform",
				Topology:        TopologyMultirepo,
				SpecsRepository: "especificacion",
			},
			Roles: map[string]RoleConfig{
				"frontend": {
					Name: "Frontend",
					Repositories: []RepositoryEntry{
						{Path: "frontend-app"},
					},
				},
				"backend": {
					Name: "Backend",
					Repositories: []RepositoryEntry{
						{Path: "backend-api"},
					},
				},
				"devops": {
					Name: "DevOps",
					Repositories: []RepositoryEntry{
						{Path: "infra"},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if !report.Valid {
			t.Errorf("se esperaba valido, errores: %v", report.Errors)
		}
	})

	t.Run("multirepo rechazado si no tiene repositorio canonico de specs", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "frontend-app"))

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "FederatedPlatform",
				Topology:        TopologyMultirepo,
				SpecsRepository: "", // Vacio
			},
			Roles: map[string]RoleConfig{
				"frontend": {
					Name: "Frontend",
					Repositories: []RepositoryEntry{
						{Path: "frontend-app"},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if report.Valid {
			t.Errorf("se esperaba rechazo por falta de specs_repository")
		}
		assertContains(t, report.Errors, "requieren obligatoriamente un repositorio canónico de especificaciones")
	})

	t.Run("multirepo rechazado si repositorio de rol no existe", func(t *testing.T) {
		fs := newMockFS()
		fs.addDir(baseDir)
		fs.addDir(filepath.Join(baseDir, "especificacion"))
		// Falta frontend-app

		cfg := &WorkspaceConfig{
			Workspace: WorkspaceSection{
				Name:            "FederatedPlatform",
				Topology:        TopologyMultirepo,
				SpecsRepository: "especificacion",
			},
			Roles: map[string]RoleConfig{
				"frontend": {
					Name: "Frontend",
					Repositories: []RepositoryEntry{
						{Path: "frontend-app"},
					},
				},
			},
		}

		report, err := Validate(fs, cfg, baseDir)
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		if report.Valid {
			t.Errorf("se esperaba invalido por repo de rol faltante")
		}
		assertContains(t, report.Errors, "el repositorio 'frontend-app' asignado al rol 'frontend' no existe")
	})
}

func assertContains(t *testing.T, list []string, substr string) {
	t.Helper()
	for _, item := range list {
		if strings.Contains(item, substr) {
			return
		}
	}
	t.Errorf("se esperaba encontrar la subcadena '%s' en la lista: %v", substr, list)
}
