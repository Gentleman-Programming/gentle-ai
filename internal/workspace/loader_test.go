package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		wantErr     bool
		errContains string
		check       func(t *testing.T, cfg *WorkspaceConfig)
	}{
		{
			name: "configuracion valida multirepo completa",
			yamlData: `
workspace:
  name: "PlataformaEnterprise"
  topology: "multirepo"
  specs_repository: "especificacion"
roles:
  frontend:
    name: "Frontend Web"
    repositories:
      - path: "repos/portal"
    tech: ["react", "typescript"]
  backend:
    name: "Core API"
    repositories:
      - path: "repos/api"
    tech: ["go"]
governance:
  language: "es"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *WorkspaceConfig) {
				if cfg.Workspace.Name != "PlataformaEnterprise" {
					t.Errorf("nombre esperado 'PlataformaEnterprise', obtenido '%s'", cfg.Workspace.Name)
				}
				if cfg.Workspace.Topology != TopologyMultirepo {
					t.Errorf("topologia esperada '%s', obtenida '%s'", TopologyMultirepo, cfg.Workspace.Topology)
				}
				if len(cfg.Roles) != 2 {
					t.Errorf("roles esperados 2, obtenidos %d", len(cfg.Roles))
				}
			},
		},
		{
			name:        "archivo vacio",
			yamlData:    "   \n  ",
			wantErr:     true,
			errContains: "el archivo de configuración está vacío",
		},
		{
			name: "sin nombre de workspace",
			yamlData: `
workspace:
  topology: "monorepo-embedded"
roles:
  fullstack:
    name: "Fullstack"
    repositories:
      - path: "."
`,
			wantErr:     true,
			errContains: "'workspace.name' es obligatorio",
		},
		{
			name: "topologia invalida",
			yamlData: `
workspace:
  name: "Test"
  topology: "invalida"
roles:
  fullstack:
    name: "Fullstack"
    repositories:
      - path: "."
`,
			wantErr:     true,
			errContains: "topología desconocida",
		},
		{
			name: "sin roles declarados",
			yamlData: `
workspace:
  name: "Test"
  topology: "monorepo-embedded"
roles: {}
`,
			wantErr:     true,
			errContains: "debe definirse al menos un rol",
		},
		{
			name: "rol sin repositorios",
			yamlData: `
workspace:
  name: "Test"
  topology: "monorepo-embedded"
roles:
  frontend:
    name: "Frontend"
    repositories: []
`,
			wantErr:     true,
			errContains: "debe contener al menos un repositorio",
		},
		{
			name: "repositorio con ruta vacia",
			yamlData: `
workspace:
  name: "Test"
  topology: "monorepo-embedded"
roles:
  frontend:
    name: "Frontend"
    repositories:
      - path: "  "
`,
			wantErr:     true,
			errContains: "no puede tener una ruta vacía",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tt.yamlData))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("se esperaba error pero no se obtuvo ninguno")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error esperado que contenga '%s', pero se obtuvo: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}

			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tempDir := t.TempDir()
	validFile := filepath.Join(tempDir, "axiom.yaml")
	validContent := `
workspace:
  name: "TestFile"
  topology: "monorepo-embedded"
  specs_repository: "."
roles:
  main:
    name: "Main Role"
    repositories:
      - path: "."
`
	if err := os.WriteFile(validFile, []byte(validContent), 0644); err != nil {
		t.Fatalf("error al escribir archivo temporal: %v", err)
	}

	cfg, err := LoadConfig(validFile)
	if err != nil {
		t.Fatalf("error inesperado cargando archivo: %v", err)
	}
	if cfg.Workspace.Name != "TestFile" {
		t.Errorf("esperado 'TestFile', obtenido '%s'", cfg.Workspace.Name)
	}

	// Archivo no existente
	_, err = LoadConfig(filepath.Join(tempDir, "inexistente.yaml"))
	if err == nil {
		t.Errorf("se esperaba error de archivo no encontrado")
	}
}

func TestResolveConfigFile_DirectAxiomYaml(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "axiom.yaml")
	if err := os.WriteFile(configPath, []byte("dummy: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Pasando la ruta directa al archivo
	resolved, err := ResolveConfigFile(configPath)
	if err != nil || resolved != configPath {
		t.Fatalf("esperado %s, obtenido %s, err: %v", configPath, resolved, err)
	}

	// 2. Pasando el directorio contenedor
	resolvedDir, err := ResolveConfigFile(tempDir)
	if err != nil || resolvedDir != configPath {
		t.Fatalf("esperado %s desde directorio, obtenido %s, err: %v", configPath, resolvedDir, err)
	}
}

func TestResolveConfigFile_PointerWithConfig(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, "repo-specs")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	canonicalConfig := filepath.Join(specsDir, "axiom.yaml")
	validContent := `
workspace:
  name: "PlataformaCanonica"
  topology: "multirepo"
  specs_repository: "repo-specs"
roles:
  backend:
    name: "Backend"
    repositories:
      - path: "backend"
`
	if err := os.WriteFile(canonicalConfig, []byte(validContent), 0644); err != nil {
		t.Fatal(err)
	}

	pointerPath := filepath.Join(tempDir, ".axiom-workspace")
	pointerContent := "config: repo-specs/axiom.yaml\n"
	if err := os.WriteFile(pointerPath, []byte(pointerContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Resolver pasando el directorio del workspace
	resolved, err := ResolveConfigFile(tempDir)
	if err != nil {
		t.Fatalf("error resolviendo puntero: %v", err)
	}
	if resolved != canonicalConfig {
		t.Errorf("esperado %s, obtenido %s", canonicalConfig, resolved)
	}

	// 2. Cargar directamente pasando el directorio del workspace
	cfg, err := LoadConfig(tempDir)
	if err != nil {
		t.Fatalf("error cargando config desde directorio con puntero: %v", err)
	}
	if cfg.Workspace.Name != "PlataformaCanonica" {
		t.Errorf("esperado 'PlataformaCanonica', obtenido '%s'", cfg.Workspace.Name)
	}

	// 3. Cargar pasando filepath.Join(tempDir, "axiom.yaml") que no existe localmente
	cfgFromVirtual, err := LoadConfig(filepath.Join(tempDir, "axiom.yaml"))
	if err != nil {
		t.Fatalf("error cargando virtual axiom.yaml via fallback puntero: %v", err)
	}
	if cfgFromVirtual.Workspace.Name != "PlataformaCanonica" {
		t.Errorf("esperado 'PlataformaCanonica', obtenido '%s'", cfgFromVirtual.Workspace.Name)
	}
}

func TestResolveConfigFile_PointerWithSpecs(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, "especificacion")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	canonicalConfig := filepath.Join(specsDir, "axiom.yaml")
	if err := os.WriteFile(canonicalConfig, []byte("dummy: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	pointerPath := filepath.Join(tempDir, ".axiom-workspace")
	pointerContent := "specs: especificacion\n"
	if err := os.WriteFile(pointerPath, []byte(pointerContent), 0644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveConfigFile(tempDir)
	if err != nil {
		t.Fatalf("error resolviendo puntero con specs: %v", err)
	}
	if resolved != canonicalConfig {
		t.Errorf("esperado %s, obtenido %s", canonicalConfig, resolved)
	}
}

func TestResolveConfigFile_FallbackSubdirectory(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, "openspec")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	canonicalConfig := filepath.Join(specsDir, "axiom.yaml")
	if err := os.WriteFile(canonicalConfig, []byte("dummy: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Sin .axiom-workspace ni axiom.yaml en la raíz: debe auto-descubrir openspec/axiom.yaml
	resolved, err := ResolveConfigFile(tempDir)
	if err != nil {
		t.Fatalf("error en auto-descubrimiento de subdirectorio: %v", err)
	}
	if resolved != canonicalConfig {
		t.Errorf("esperado %s, obtenido %s", canonicalConfig, resolved)
	}
}

func TestResolveConfigFile_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	_, err := ResolveConfigFile(tempDir)
	if err == nil {
		t.Fatalf("se esperaba error de archivo no encontrado")
	}
}

