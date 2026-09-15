package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetector(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-detector-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	detector := NewDetector()

	// 1. Proyecto Go
	goDir := filepath.Join(tempDir, "go-proj")
	_ = os.MkdirAll(goDir, 0755)
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module example.com/test\ngo 1.25\n"), 0644)

	detGo, err := detector.Detect(goDir)
	if err != nil {
		t.Fatalf("error detectando Go: %v", err)
	}
	if detGo.PrimaryLanguage != "go" {
		t.Errorf("esperado 'go', obtenido '%s'", detGo.PrimaryLanguage)
	}
	if !detGo.HasTests {
		t.Errorf("esperado HasTests=true para Go")
	}

	// 2. Proyecto .NET / C#
	dotnetDir := filepath.Join(tempDir, "dotnet-proj")
	_ = os.MkdirAll(dotnetDir, 0755)
	_ = os.WriteFile(filepath.Join(dotnetDir, "App.csproj"), []byte("<Project></Project>"), 0644)

	detDotnet, err := detector.Detect(dotnetDir)
	if err != nil {
		t.Fatalf("error detectando dotnet: %v", err)
	}
	if detDotnet.PrimaryLanguage != "csharp" {
		t.Errorf("esperado 'csharp', obtenido '%s'", detDotnet.PrimaryLanguage)
	}

	// 3. Proyecto Node / React
	nodeDir := filepath.Join(tempDir, "node-proj")
	_ = os.MkdirAll(nodeDir, 0755)
	_ = os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte(`{"dependencies": {"react": "^18.0.0"}, "devDependencies": {"vitest": "^1.0.0"}}`), 0644)
	_ = os.WriteFile(filepath.Join(nodeDir, "tsconfig.json"), []byte(`{}`), 0644)

	detNode, err := detector.Detect(nodeDir)
	if err != nil {
		t.Fatalf("error detectando node: %v", err)
	}
	if detNode.PrimaryLanguage != "typescript" {
		t.Errorf("esperado 'typescript', obtenido '%s'", detNode.PrimaryLanguage)
	}
	hasReact := false
	for _, f := range detNode.Frameworks {
		if f == "react" {
			hasReact = true
			break
		}
	}
	if !hasReact {
		t.Errorf("esperado framework 'react' detectado")
	}
	if !detNode.HasTests {
		t.Errorf("esperado HasTests=true para vitest")
	}
}

func TestManagerOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-manager-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, ".axiom", "workspaces.json")
	mgr, err := NewManager(configPath)
	if err != nil {
		t.Fatalf("error creando Manager: %v", err)
	}

	// 1. Cargar configuración inicial vacía
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("error en Load(): %v", err)
	}
	if len(cfg.Workspaces) != 0 {
		t.Errorf("esperado 0 workspaces, obtenido %d", len(cfg.Workspaces))
	}

	// 2. Registrar primer proyecto
	proj1 := filepath.Join(tempDir, "proj1")
	_ = os.MkdirAll(proj1, 0755)
	_ = os.WriteFile(filepath.Join(proj1, "axiom.yaml"), []byte("workspace:\n  name: Proj1\n"), 0644)

	rec1, err := mgr.Register(proj1, "Proyecto 1", "monorepo-embedded")
	if err != nil {
		t.Fatalf("error registrando proj1: %v", err)
	}
	if rec1.ID != "proyecto-1" {
		t.Errorf("esperado id 'proyecto-1', obtenido '%s'", rec1.ID)
	}
	if !rec1.IsConfigured {
		t.Errorf("esperado IsConfigured=true para proj1")
	}

	// 3. Obtener activo (debe ser rec1 automáticamente)
	active, err := mgr.GetActive()
	if err != nil {
		t.Fatalf("error en GetActive(): %v", err)
	}
	if active.ID != rec1.ID {
		t.Errorf("esperado activo '%s', obtenido '%s'", rec1.ID, active.ID)
	}

	// 4. Registrar segundo proyecto (sin axiom.yaml)
	proj2 := filepath.Join(tempDir, "proj2")
	_ = os.MkdirAll(proj2, 0755)

	rec2, err := mgr.Register(proj2, "Proyecto 2", "monorepo-embedded")
	if err != nil {
		t.Fatalf("error registrando proj2: %v", err)
	}
	if rec2.IsConfigured {
		t.Errorf("esperado IsConfigured=false para proj2")
	}

	// 5. Conmutar activo a proj2
	updatedActive, err := mgr.SetActive(rec2.ID)
	if err != nil {
		t.Fatalf("error en SetActive: %v", err)
	}
	if updatedActive.ID != rec2.ID {
		t.Errorf("esperado nuevo activo '%s', obtenido '%s'", rec2.ID, updatedActive.ID)
	}

	// 6. Listar
	list, err := mgr.List()
	if err != nil {
		t.Fatalf("error en List(): %v", err)
	}
	if len(list) != 2 {
		t.Errorf("esperado 2 workspaces, obtenidos %d", len(list))
	}

	// 7. Idempotencia de registro (mismo path, actualiza nombre)
	rec1Updated, err := mgr.Register(proj1, "Proyecto 1 Renombrado", "monorepo-embedded")
	if err != nil {
		t.Fatalf("error re-registrando proj1: %v", err)
	}
	if rec1Updated.Name != "Proyecto 1 Renombrado" {
		t.Errorf("esperado nombre actualizado, obtenido '%s'", rec1Updated.Name)
	}
	listAfterReReg, _ := mgr.List()
	if len(listAfterReReg) != 2 {
		t.Errorf("re-registro no debe duplicar la entrada: esperado 2, obtenido %d", len(listAfterReReg))
	}

	// 8. Desregistrar
	if err := mgr.Unregister(rec2.ID); err != nil {
		t.Fatalf("error desregistrando: %v", err)
	}
	listAfterDel, _ := mgr.List()
	if len(listAfterDel) != 1 {
		t.Errorf("esperado 1 workspace tras desregistro, obtenido %d", len(listAfterDel))
	}
}

func TestInitializer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-init-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, ".axiom", "workspaces.json")
	mgr, _ := NewManager(configPath)
	det := NewDetector()
	ini := NewInitializer(mgr, det)

	// Inicializar en una carpeta con un go.mod
	targetProj := filepath.Join(tempDir, "mi-servicio-go")
	_ = os.MkdirAll(targetProj, 0755)
	_ = os.WriteFile(filepath.Join(targetProj, "go.mod"), []byte("module mi-servicio\ngo 1.25\n"), 0644)

	res, err := ini.Init(InitOptions{
		Path: targetProj,
		Name: "Mi Servicio Go",
	})
	if err != nil {
		t.Fatalf("error en Init: %v", err)
	}

	if res.AlreadyExisted {
		t.Errorf("no debía existir previamente")
	}

	// Verificar axiom.yaml
	yamlBytes, err := os.ReadFile(res.ConfigPath)
	if err != nil {
		t.Fatalf("error leyendo axiom.yaml generado: %v", err)
	}
	content := string(yamlBytes)
	if !strings.Contains(content, "name: \"Mi Servicio Go\"") {
		t.Errorf("contenido no incluye el nombre esperado: %s", content)
	}
	if !strings.Contains(content, "\"go\"") {
		t.Errorf("contenido no detectó la tecnología 'go': %s", content)
	}

	// Verificar carpetas creadas
	dirsToCheck := []string{
		filepath.Join(targetProj, ".axiom", "inbox", "skills"),
		filepath.Join(targetProj, "openspec", "specs"),
		filepath.Join(targetProj, "openspec", "changes"),
	}
	for _, d := range dirsToCheck {
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			t.Errorf("directorio requerido %s no fue creado", d)
		}
	}

	// Verificar registro en Manager
	active, err := mgr.GetActive()
	if err != nil {
		t.Fatalf("error recuperando activo del hub: %v", err)
	}
	if active.Path != targetProj {
		t.Errorf("esperado path '%s', obtenido '%s'", targetProj, active.Path)
	}

	// Re-ejecutar Init sin force: debe reportar AlreadyExisted=true y no sobreescribir
	res2, err := ini.Init(InitOptions{
		Path: targetProj,
	})
	if err != nil {
		t.Fatalf("error en segundo Init: %v", err)
	}
	if !res2.AlreadyExisted {
		t.Errorf("esperado AlreadyExisted=true en segunda llamada")
	}
}

func TestSmartAdoptionWithExistingSDDAndAgents(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-smart-adopt-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Crear estructura simulada tipo Ludeka
	openSpecDir := filepath.Join(tempDir, "openspec")
	_ = os.MkdirAll(filepath.Join(openSpecDir, "specs"), 0755)
	_ = os.MkdirAll(filepath.Join(openSpecDir, "changes"), 0755)

	configYamlContent := `schema: "1.0"
context: "Glosario y reglas del juego Ludeka."
projects:
  - path: "src/Ludeka.Web"
    stack: ".NET 10 Blazor Web App"
  - path: "src/Ludeka.Core"
    stack: ".NET 10 Class Library"
  - path: "tests/Ludeka.UnitTests"
    stack: ".NET 10 xUnit"
`
	_ = os.WriteFile(filepath.Join(openSpecDir, "config.yaml"), []byte(configYamlContent), 0644)

	// Crear agentes y skills preexistentes
	skillsDir := filepath.Join(tempDir, ".agents", "skills")
	_ = os.MkdirAll(filepath.Join(skillsDir, "sdd-explore"), 0755)
	_ = os.WriteFile(filepath.Join(skillsDir, "sdd-explore", "SKILL.md"), []byte("description: \"Fase de exploración inicial\"\n"), 0644)

	_ = os.MkdirAll(filepath.Join(skillsDir, "fluentui-blazor"), 0755)
	_ = os.WriteFile(filepath.Join(skillsDir, "fluentui-blazor", "SKILL.md"), []byte("description: \"Componentes Fluent UI Blazor\"\n"), 0644)

	githubDir := filepath.Join(tempDir, ".github")
	_ = os.MkdirAll(githubDir, 0755)
	_ = os.WriteFile(filepath.Join(githubDir, "copilot-instructions.md"), []byte("# Copilot Rules\n"), 0644)

	detector := NewDetector()
	det, err := detector.Detect(tempDir)
	if err != nil {
		t.Fatalf("error en Detect: %v", err)
	}

	if !det.HasExistingSDD {
		t.Errorf("esperado HasExistingSDD=true")
	}
	if det.SpecsRepository != "openspec" {
		t.Errorf("esperado SpecsRepository='openspec', obtenido '%s'", det.SpecsRepository)
	}
	if !strings.Contains(det.DomainContext, "Ludeka") {
		t.Errorf("esperado DomainContext conteniendo 'Ludeka', obtenido '%s'", det.DomainContext)
	}
	if len(det.Projects) != 3 {
		t.Errorf("esperado 3 proyectos detectados, obtenidos %d", len(det.Projects))
	}
	if len(det.ConfiguredRoles) != 3 {
		t.Errorf("esperado 3 roles inferidos (core, web, qa), obtenidos %d", len(det.ConfiguredRoles))
	}
	if len(det.AdoptedSkills) < 3 {
		t.Errorf("esperado al menos 3 skills/agentes detectados, obtenidos %d", len(det.AdoptedSkills))
	}

	// Inicializar con adopción inteligente
	ini := NewInitializer(nil, detector)
	res, err := ini.Init(InitOptions{
		Path: tempDir,
		Name: "Ludeka Adopted",
	})
	if err != nil {
		t.Fatalf("error en Init con adopción: %v", err)
	}

	yamlBytes, err := os.ReadFile(res.ConfigPath)
	if err != nil {
		t.Fatalf("error leyendo axiom.yaml generado: %v", err)
	}
	content := string(yamlBytes)

	// Verificar asignación exacta de repositorios a roles
	if !strings.Contains(content, "src/Ludeka.Web") {
		t.Errorf("axiom.yaml debe contener la ruta 'src/Ludeka.Web': %s", content)
	}
	if !strings.Contains(content, "src/Ludeka.Core") {
		t.Errorf("axiom.yaml debe contener la ruta 'src/Ludeka.Core': %s", content)
	}
	if !strings.Contains(content, "tests/Ludeka.UnitTests") {
		t.Errorf("axiom.yaml debe contener la ruta 'tests/Ludeka.UnitTests': %s", content)
	}
	if !strings.Contains(content, "Glosario y reglas del juego Ludeka.") {
		t.Errorf("axiom.yaml debe contener el contexto de gobernanza: %s", content)
	}
}

func TestInteractiveRolesAndNonBlocking(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-interactive-roles-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ini := NewInitializer(nil, nil)
	res, err := ini.Init(InitOptions{
		Path: tempDir,
		Name: "Custom Roles Proj",
		Roles: []RoleInput{
			{
				Key:          "backend",
				Name:         "Backend Engine",
				Repositories: []string{"server"},
				NonBlocking:  false,
				Tech:         []string{"go"},
			},
			{
				Key:          "docs",
				Name:         "Documentation",
				Repositories: []string{"docs"},
				NonBlocking:  true,
				Tech:         []string{"markdown"},
			},
		},
	})
	if err != nil {
		t.Fatalf("error en Init con roles personalizados: %v", err)
	}

	yamlBytes, _ := os.ReadFile(res.ConfigPath)
	content := string(yamlBytes)

	if !strings.Contains(content, "backend:") || !strings.Contains(content, "gate_policy: \"blocking\"") {
		t.Errorf("esperado rol 'backend' con gate_policy blocking: %s", content)
	}
	if !strings.Contains(content, "docs:") || !strings.Contains(content, "gate_policy: \"advisory\"") {
		t.Errorf("esperado rol 'docs' con gate_policy advisory: %s", content)
	}
}

