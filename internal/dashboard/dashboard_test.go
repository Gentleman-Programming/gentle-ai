package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/hub"
)

func TestServiceWorkspace(t *testing.T) {
	// Usamos la raíz del repositorio de Axiom (..)
	svc := NewService("../..")
	ws, err := svc.GetWorkspace()
	if err != nil {
		t.Fatalf("GetWorkspace falló: %v", err)
	}

	if ws.Name == "" {
		t.Errorf("nombre de workspace vacío")
	}
	if ws.Topology == "" {
		t.Errorf("topología de workspace vacía")
	}
	if len(ws.Roles) == 0 {
		t.Errorf("se esperaban roles declarados en el workspace")
	}
}

func TestServiceIncrements(t *testing.T) {
	svc := NewService("../..")
	list, err := svc.GetIncrements()
	if err != nil {
		t.Fatalf("GetIncrements falló: %v", err)
	}

	if len(list) == 0 {
		t.Fatalf("se esperaba encontrar al menos un incremento")
	}

	hasArchived := false
	hasActive := false
	for _, inc := range list {
		if inc.Type == "archived" {
			hasArchived = true
		}
		if inc.Type == "active" {
			hasActive = true
		}
	}

	if !hasArchived && !hasActive {
		t.Errorf("se esperaba encontrar al menos un incremento (activo o archivado)")
	}
}

func TestServiceIncrementDetail(t *testing.T) {
	svc := NewService("../..")

	// 1. Probar con incremento existente (INC-03 archivado)
	detail, err := svc.GetIncrementDetail("inc-03-multi-role-sdd-fan-out")
	if err != nil {
		t.Fatalf("GetIncrementDetail falló: %v", err)
	}

	if !detail.HasSpec {
		t.Errorf("se esperaba que inc-03 tuviera spec.md")
	}
	if !detail.HasDesign {
		t.Errorf("se esperaba que inc-03 tuviera design.md")
	}

	// 2. Probar con incremento inexistente
	_, errNotFound := svc.GetIncrementDetail("cambio-completamente-inventado-999")
	if errNotFound == nil {
		t.Errorf("se esperaba error 404 para incremento inexistente")
	}
}

func TestServiceRoleStatus(t *testing.T) {
	svc := NewService("../..")
	barrier, err := svc.GetRoleStatus("inc-03-multi-role-sdd-fan-out")
	if err != nil {
		t.Fatalf("GetRoleStatus falló para inc-03: %v", err)
	}

	if !barrier.Satisfied {
		t.Errorf("se esperaba que la barrera de inc-03 estuviera SATISFIED")
	}
	if len(barrier.Roles) == 0 {
		t.Errorf("se esperaban roles evaluados en inc-03")
	}
}

func TestServiceSkills(t *testing.T) {
	svc := NewService("../..")
	skills, err := svc.GetSkills()
	if err != nil {
		t.Fatalf("GetSkills falló: %v", err)
	}

	if len(skills) == 0 {
		t.Errorf("se esperaba encontrar skills en skills/ o internal/assets/skills/")
	}
}

func TestHTTPEndpoints(t *testing.T) {
	svc := NewService("../..")
	server := NewServer(svc)
	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	// 1. GET /api/workspace
	respWs, err := http.Get(ts.URL + "/api/workspace")
	if err != nil || respWs.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspace falló: status=%v, err=%v", respWs.StatusCode, err)
	}
	var wsDto WorkspaceDTO
	if err := json.NewDecoder(respWs.Body).Decode(&wsDto); err != nil {
		t.Fatalf("Error decodificando /api/workspace: %v", err)
	}
	_ = respWs.Body.Close()

	// 2. GET /api/increments
	respInc, err := http.Get(ts.URL + "/api/increments")
	if err != nil || respInc.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/increments falló: status=%v, err=%v", respInc.StatusCode, err)
	}
	var incList []IncrementSummaryDTO
	if err := json.NewDecoder(respInc.Body).Decode(&incList); err != nil {
		t.Fatalf("Error decodificando /api/increments: %v", err)
	}
	_ = respInc.Body.Close()

	// 3. GET /api/increments/{name} válido
	respDetail, err := http.Get(ts.URL + "/api/increments/inc-03-multi-role-sdd-fan-out")
	if err != nil || respDetail.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/increments/... falló: status=%v, err=%v", respDetail.StatusCode, err)
	}
	_ = respDetail.Body.Close()

	// 4. GET /api/increments/{name} 404
	resp404, err := http.Get(ts.URL + "/api/increments/no-existe")
	if err != nil || resp404.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/increments/no-existe debería retornar 404: status=%v", resp404.StatusCode)
	}
	_ = resp404.Body.Close()

	// 5. GET /api/skills
	respSkills, err := http.Get(ts.URL + "/api/skills")
	if err != nil || respSkills.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/skills falló: status=%v, err=%v", respSkills.StatusCode, err)
	}
	_ = respSkills.Body.Close()

	// 6. GET / (servido de index.html embebido)
	respRoot, err := http.Get(ts.URL + "/")
	if err != nil || respRoot.StatusCode != http.StatusOK {
		t.Fatalf("GET / falló: status=%v, err=%v", respRoot.StatusCode, err)
	}
	bodyRoot, _ := io.ReadAll(respRoot.Body)
	_ = respRoot.Body.Close()
	if !strings.Contains(string(bodyRoot), "Axiom Enterprise") {
		t.Errorf("GET / no contiene 'Axiom Enterprise'")
	}

	// 7. GET /style.css y /app.js
	respCSS, err := http.Get(ts.URL + "/style.css")
	if err != nil || respCSS.StatusCode != http.StatusOK {
		t.Errorf("GET /style.css falló: status=%v", respCSS.StatusCode)
	}
	_ = respCSS.Body.Close()

	respJS, err := http.Get(ts.URL + "/app.js")
	if err != nil || respJS.StatusCode != http.StatusOK {
		t.Errorf("GET /app.js falló: status=%v", respJS.StatusCode)
	}
	_ = respJS.Body.Close()
}

func TestPortFallback(t *testing.T) {
	// Ocupar puerto dinámico
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Error abriendo listener de prueba: %v", err)
	}
	defer l.Close()

	occupiedPort := l.Addr().(*net.TCPAddr).Port

	svc := NewService("../..")
	server := NewServer(svc)

	nextListener, chosenPort, err := server.findAvailablePort(occupiedPort)
	if err != nil {
		t.Fatalf("findAvailablePort falló: %v", err)
	}
	defer nextListener.Close()

	if chosenPort == occupiedPort {
		t.Errorf("chosenPort (%d) no debería ser igual al puerto ocupado (%d)", chosenPort, occupiedPort)
	}
	if chosenPort < occupiedPort {
		t.Errorf("chosenPort (%d) debería ser mayor que el inicial (%d)", chosenPort, occupiedPort)
	}
}

func TestServiceHandoff(t *testing.T) {
	// Crear handoff temporal en directorio temporal
	tmpDir := t.TempDir()
	changeDir := filepath.Join(tmpDir, "openspec", "changes", "test-ho")
	if err := os.MkdirAll(changeDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `---
change: test-ho
from_phase: design
to_phase: tasks
from_role: core
to_role: core
timestamp: "2026-09-14T12:00:00Z"
status: ready
---

## 1. Resumen Ejecutivo
Resumen prueba.

## 2. Artefactos Modificados y Creados
- arch1

## 3. Decisiones Técnicas y Acuerdos
- dec1

## 4. Riesgos, Bloqueos y Preguntas Abiertas
- ninguna

## 5. Instrucciones Directas para el Siguiente Rol
Proceder con tasks.
`
	if err := os.WriteFile(filepath.Join(changeDir, "handoff.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(tmpDir)
	ho, err := svc.GetHandoff("test-ho")
	if err != nil {
		t.Fatalf("GetHandoff falló: %v", err)
	}

	if ho.Metadata.Status != "ready" {
		t.Errorf("status = %s, want ready", ho.Metadata.Status)
	}
	if ho.Sections.ExecutiveSummary != "Resumen prueba." {
		t.Errorf("resumen = %q", ho.Sections.ExecutiveSummary)
	}
}

func TestSkillsInboxEndpoints(t *testing.T) {
	tmpDir := t.TempDir()

	// Crear una propuesta en el buzón dentro de tmpDir
	inboxFolder := filepath.Join(tmpDir, ".axiom", "skills", "inbox", "sample-skill")
	if err := os.MkdirAll(inboxFolder, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(inboxFolder, "SKILL.md"), []byte("# Sample Skill\n\nContenido prueba"), 0644)
	metaJSON := `{"name":"sample-skill","origin":"mined","verified":true}`
	_ = os.WriteFile(filepath.Join(inboxFolder, "metadata.json"), []byte(metaJSON), 0644)

	svc := NewService(tmpDir)
	server := NewServer(svc)
	router := server.Router()

	// 1. GET /api/skills/inbox
	reqGet := httptest.NewRequest(http.MethodGet, "/api/skills/inbox", nil)
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)

	if rrGet.Code != http.StatusOK {
		t.Fatalf("GET /api/skills/inbox retornó %d", rrGet.Code)
	}

	var proposals []SkillProposalDTO
	if err := json.NewDecoder(rrGet.Body).Decode(&proposals); err != nil {
		t.Fatalf("error decodificando respuesta de inbox: %v", err)
	}
	if len(proposals) != 1 || proposals[0].Name != "sample-skill" {
		t.Fatalf("esperada 1 propuesta con nombre 'sample-skill', obtenidas: %+v", proposals)
	}

	// 2. POST /api/skills/approve
	approveBody := `{"name":"sample-skill"}`
	reqApprove := httptest.NewRequest(http.MethodPost, "/api/skills/approve", strings.NewReader(approveBody))
	rrApprove := httptest.NewRecorder()
	router.ServeHTTP(rrApprove, reqApprove)

	if rrApprove.Code != http.StatusOK {
		t.Fatalf("POST /api/skills/approve retornó %d: %s", rrApprove.Code, rrApprove.Body.String())
	}

	// Verificar que se instaló en skills/
	installedPath := filepath.Join(tmpDir, "skills", "sample-skill", "SKILL.md")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Errorf("la skill aprobada no existe en %s", installedPath)
	}

	// 3. Crear otra propuesta y probar POST /api/skills/reject
	rejectFolder := filepath.Join(tmpDir, ".axiom", "skills", "inbox", "reject-skill")
	_ = os.MkdirAll(rejectFolder, 0755)
	_ = os.WriteFile(filepath.Join(rejectFolder, "SKILL.md"), []byte("# Reject"), 0644)

	rejectBody := `{"name":"reject-skill"}`
	reqReject := httptest.NewRequest(http.MethodPost, "/api/skills/reject", strings.NewReader(rejectBody))
	rrReject := httptest.NewRecorder()
	router.ServeHTTP(rrReject, reqReject)

	if rrReject.Code != http.StatusOK {
		t.Fatalf("POST /api/skills/reject retornó %d: %s", rrReject.Code, rrReject.Body.String())
	}

	if _, err := os.Stat(rejectFolder); !os.IsNotExist(err) {
		t.Errorf("la propuesta rechazada todavía existe en el buzón")
	}

	// 4. POST /api/skills/scan
	scanBody := `{"offline":true}`
	reqScan := httptest.NewRequest(http.MethodPost, "/api/skills/scan", strings.NewReader(scanBody))
	rrScan := httptest.NewRecorder()
	router.ServeHTTP(rrScan, reqScan)

	if rrScan.Code != http.StatusOK {
		t.Fatalf("POST /api/skills/scan retornó %d: %s", rrScan.Code, rrScan.Body.String())
	}
}

func TestSemanticEndpoints(t *testing.T) {
	svc := NewService("../..")
	server := NewServer(svc)
	router := server.Router()

	// 1. GET /api/semantic/status
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/semantic/status", nil)
	rrStatus := httptest.NewRecorder()
	router.ServeHTTP(rrStatus, reqStatus)

	if rrStatus.Code != http.StatusOK {
		t.Fatalf("GET /api/semantic/status retornó %d: %s", rrStatus.Code, rrStatus.Body.String())
	}

	var statusMap map[string]interface{}
	if err := json.Unmarshal(rrStatus.Body.Bytes(), &statusMap); err != nil {
		t.Fatalf("JSON inválido en /api/semantic/status: %v", err)
	}
	if _, ok := statusMap["active_connector"]; !ok {
		t.Errorf("campo 'active_connector' faltante en respuesta")
	}

	// 2. GET /api/semantic/symbols
	reqSymbols := httptest.NewRequest(http.MethodGet, "/api/semantic/symbols?query=Service", nil)
	rrSymbols := httptest.NewRecorder()
	router.ServeHTTP(rrSymbols, reqSymbols)

	if rrSymbols.Code != http.StatusOK {
		t.Fatalf("GET /api/semantic/symbols retornó %d: %s", rrSymbols.Code, rrSymbols.Body.String())
	}

	var symbols []map[string]interface{}
	if err := json.Unmarshal(rrSymbols.Body.Bytes(), &symbols); err != nil {
		t.Fatalf("JSON inválido en /api/semantic/symbols: %v", err)
	}

	// 3. GET /api/semantic/dependencies
	reqDeps := httptest.NewRequest(http.MethodGet, "/api/semantic/dependencies", nil)
	rrDeps := httptest.NewRecorder()
	router.ServeHTTP(rrDeps, reqDeps)

	if rrDeps.Code != http.StatusOK {
		t.Fatalf("GET /api/semantic/dependencies retornó %d: %s", rrDeps.Code, rrDeps.Body.String())
	}

	var deps []map[string]interface{}
	if err := json.Unmarshal(rrDeps.Body.Bytes(), &deps); err != nil {
		t.Fatalf("JSON inválido en /api/semantic/dependencies: %v", err)
	}
}

func TestArchiveEndpoints(t *testing.T) {
	svc := NewService("../..")
	server := NewServer(svc)
	router := server.Router()

	// 1. GET /api/archive/specs
	reqSpecs := httptest.NewRequest(http.MethodGet, "/api/archive/specs", nil)
	rrSpecs := httptest.NewRecorder()
	router.ServeHTTP(rrSpecs, reqSpecs)

	if rrSpecs.Code != http.StatusOK {
		t.Fatalf("GET /api/archive/specs retornó %d: %s", rrSpecs.Code, rrSpecs.Body.String())
	}

	var catalog map[string]interface{}
	if err := json.Unmarshal(rrSpecs.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("JSON inválido en /api/archive/specs: %v", err)
	}

	// 2. POST /api/archive/sync
	reqSync := httptest.NewRequest(http.MethodPost, "/api/archive/sync", nil)
	rrSync := httptest.NewRecorder()
	router.ServeHTTP(rrSync, reqSync)

	if rrSync.Code != http.StatusOK {
		t.Fatalf("POST /api/archive/sync retornó %d: %s", rrSync.Code, rrSync.Body.String())
	}

	// 3. GET /api/archive/specs/non-existent-domain (debe retornar 404)
	reqDetail404 := httptest.NewRequest(http.MethodGet, "/api/archive/specs/non-existent-domain", nil)
	rrDetail404 := httptest.NewRecorder()
	router.ServeHTTP(rrDetail404, reqDetail404)

	if rrDetail404.Code != http.StatusNotFound {
		t.Fatalf("se esperaba 404 para dominio inexistente, se obtuvo %d", rrDetail404.Code)
	}
}

func TestProjectsEndpoints(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-dashboard-projects-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hubConfigPath := filepath.Join(tempDir, ".axiom", "workspaces.json")
	hubMgr, err := hub.NewManager(hubConfigPath)
	if err != nil {
		t.Fatalf("error creando Hub Manager: %v", err)
	}

	// Proyecto 1 inicializado
	proj1 := filepath.Join(tempDir, "proj1")
	_ = os.MkdirAll(proj1, 0755)
	_ = os.WriteFile(filepath.Join(proj1, "axiom.yaml"), []byte("workspace:\n  name: Proj1\n  topology: monorepo-embedded\n"), 0644)
	_, _ = hubMgr.Register(proj1, "Proyecto 1", "monorepo-embedded")

	svc := NewServiceWithHub(proj1, hubMgr)
	server := NewServer(svc)
	router := server.Router()

	// 1. GET /api/projects
	reqList := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rrList := httptest.NewRecorder()
	router.ServeHTTP(rrList, reqList)

	if rrList.Code != http.StatusOK {
		t.Fatalf("GET /api/projects falló con código %d: %s", rrList.Code, rrList.Body.String())
	}
	var projList ProjectListDTO
	if err := json.Unmarshal(rrList.Body.Bytes(), &projList); err != nil {
		t.Fatalf("JSON inválido en /api/projects: %v", err)
	}
	if len(projList.Projects) != 1 {
		t.Errorf("esperado 1 proyecto en lista, obtenido %d", len(projList.Projects))
	}

	// 2. POST /api/projects/add (agregar proj2 sin axiom.yaml)
	proj2 := filepath.Join(tempDir, "proj2")
	_ = os.MkdirAll(proj2, 0755)
	_ = os.WriteFile(filepath.Join(proj2, "go.mod"), []byte("module proj2\ngo 1.25\n"), 0644)

	addBody, _ := json.Marshal(ProjectAddRequest{
		Path: proj2,
		Name: "Proyecto 2",
	})
	reqAdd := httptest.NewRequest(http.MethodPost, "/api/projects/add", bytes.NewReader(addBody))
	rrAdd := httptest.NewRecorder()
	router.ServeHTTP(rrAdd, reqAdd)

	if rrAdd.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/add falló con código %d: %s", rrAdd.Code, rrAdd.Body.String())
	}

	// 3. POST /api/projects/switch (cambiar a proj2)
	switchBody, _ := json.Marshal(ProjectSwitchRequest{
		Path: proj2,
	})
	reqSwitch := httptest.NewRequest(http.MethodPost, "/api/projects/switch", bytes.NewReader(switchBody))
	rrSwitch := httptest.NewRecorder()
	router.ServeHTTP(rrSwitch, reqSwitch)

	if rrSwitch.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/switch falló con código %d: %s", rrSwitch.Code, rrSwitch.Body.String())
	}

	// 4. GET /api/workspace en proj2 (debe retornar IsConfigured = false y Zero-Config)
	reqWs := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	rrWs := httptest.NewRecorder()
	router.ServeHTTP(rrWs, reqWs)

	if rrWs.Code != http.StatusOK {
		t.Fatalf("GET /api/workspace falló con código %d: %s", rrWs.Code, rrWs.Body.String())
	}
	var wsDTO WorkspaceDTO
	if err := json.Unmarshal(rrWs.Body.Bytes(), &wsDTO); err != nil {
		t.Fatalf("JSON inválido en /api/workspace: %v", err)
	}
	if wsDTO.IsConfigured {
		t.Errorf("esperado IsConfigured=false para proyecto 2 antes de init")
	}

	// 5. POST /api/projects/init (inicializar proj2 con 1 clic)
	initBody, _ := json.Marshal(ProjectInitRequest{
		Path: proj2,
		Name: "Proyecto 2",
	})
	reqInit := httptest.NewRequest(http.MethodPost, "/api/projects/init", bytes.NewReader(initBody))
	rrInit := httptest.NewRecorder()
	router.ServeHTTP(rrInit, reqInit)

	if rrInit.Code != http.StatusOK {
		t.Fatalf("POST /api/projects/init falló con código %d: %s", rrInit.Code, rrInit.Body.String())
	}

	// 6. Verificar que proj2 ahora sí está configurado
	reqWsAfter := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	rrWsAfter := httptest.NewRecorder()
	router.ServeHTTP(rrWsAfter, reqWsAfter)

	var wsAfterDTO WorkspaceDTO
	_ = json.Unmarshal(rrWsAfter.Body.Bytes(), &wsAfterDTO)
	if !wsAfterDTO.IsConfigured {
		t.Errorf("esperado IsConfigured=true tras ejecutar init")
	}
}

func TestMigrateCumulativeEndpoint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-cumulative-test-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Estructura de cambio con tareas incompletas
	changeDir := filepath.Join(tempDir, "openspec", "changes", "feature-x")
	_ = os.MkdirAll(changeDir, 0755)

	tasksContent := `# Tareas
- [x] Tarea completada 1
- [ ] Tarea pendiente advisory 1
- [ ] Tarea pendiente advisory 2
`
	_ = os.WriteFile(filepath.Join(changeDir, "tasks.docs.md"), []byte(tasksContent), 0644)

	svc := NewService(tempDir)
	server := NewServer(svc)
	router := server.Router()

	body, _ := json.Marshal(MigrateCumulativeRequest{
		Change: "feature-x",
		Role:   "docs",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/increments/migrate-cumulative", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/increments/migrate-cumulative retornó %d: %s", rr.Code, rr.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("error deserializando respuesta: %v", err)
	}

	if res["migrated"].(float64) != 2 {
		t.Errorf("esperadas 2 tareas migradas, obtenido %v", res["migrated"])
	}

	// Verificar que se creó cumulative-docs/tasks.md
	cumulativeFile := filepath.Join(tempDir, "openspec", "changes", "cumulative-docs", "tasks.md")
	content, err := os.ReadFile(cumulativeFile)
	if err != nil {
		t.Fatalf("no se creó el archivo acumulativo: %v", err)
	}
	if !strings.Contains(string(content), "Tarea pendiente advisory 1") || !strings.Contains(string(content), "Tarea pendiente advisory 2") {
		t.Errorf("contenido acumulativo incompleto: %s", string(content))
	}
}

func TestInteractiveSDDOrchestrationEndpoints(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "axiom-interactive-sdd-*")
	if err != nil {
		t.Fatalf("error creando tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Inicializar estructura básica de openspec
	_ = os.MkdirAll(filepath.Join(tempDir, "openspec", "changes"), 0755)

	svc := NewService(tempDir)
	server := NewServer(svc)
	router := server.Router()

	// 1. POST /api/increments - Creación válida
	reqBodyValid, _ := json.Marshal(CreateIncrementRequest{
		Name:   "inc-test-auth",
		Intent: "Añadir autenticación segura mediante tokens JWT",
		Type:   "feature",
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/increments", bytes.NewReader(reqBodyValid))
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Fatalf("POST /api/increments esperado 201 Created, obtenido %d: %s", rr1.Code, rr1.Body.String())
	}
	var res1 CreateIncrementResponse
	if err := json.Unmarshal(rr1.Body.Bytes(), &res1); err != nil {
		t.Fatalf("error decodificando respuesta de /api/increments: %v", err)
	}
	if !res1.Success || res1.Name != "inc-test-auth" {
		t.Errorf("respuesta inesperada: %+v", res1)
	}

	// Comprobar que proposal.md existe y está en español
	proposalPath := filepath.Join(tempDir, "openspec", "changes", "inc-test-auth", "proposal.md")
	proposalContent, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("proposal.md no fue creado en disco: %v", err)
	}
	if !strings.Contains(string(proposalContent), "# Propuesta: ") || !strings.Contains(string(proposalContent), "Añadir autenticación segura") {
		t.Errorf("proposal.md no contiene la plantilla esperada en español: %s", string(proposalContent))
	}

	// 2. POST /api/increments - Rechazo de nombre inválido
	reqBodyInvalidName, _ := json.Marshal(CreateIncrementRequest{
		Name:   "Nombre Invalido Con Espacios",
		Intent: "Intento cualquiera",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/increments", bytes.NewReader(reqBodyInvalidName))
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("esperado 400 Bad Request para nombre inválido, obtenido %d", rr2.Code)
	}

	// 3. POST /api/increments - Rechazo por colisión / duplicado
	req3 := httptest.NewRequest(http.MethodPost, "/api/increments", bytes.NewReader(reqBodyValid))
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusBadRequest {
		t.Errorf("esperado 400 Bad Request por incremento duplicado, obtenido %d", rr3.Code)
	}

	// 4. POST /api/increments/continue - Para cambio existente
	reqBodyContinue, _ := json.Marshal(IncrementActionRequest{Name: "inc-test-auth"})
	req4 := httptest.NewRequest(http.MethodPost, "/api/increments/continue", bytes.NewReader(reqBodyContinue))
	rr4 := httptest.NewRecorder()
	router.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("POST /api/increments/continue esperado 200 OK, obtenido %d: %s", rr4.Code, rr4.Body.String())
	}
	var res4 IncrementActionResponse
	if err := json.Unmarshal(rr4.Body.Bytes(), &res4); err != nil {
		t.Fatalf("error decodificando respuesta de continue: %v", err)
	}
	if !res4.Success {
		t.Errorf("esperado success=true en continue")
	}

	// 5. POST /api/increments/continue - Para cambio inexistente
	reqBodyContinue404, _ := json.Marshal(IncrementActionRequest{Name: "cambio-fantasma-999"})
	req5 := httptest.NewRequest(http.MethodPost, "/api/increments/continue", bytes.NewReader(reqBodyContinue404))
	rr5 := httptest.NewRecorder()
	router.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusNotFound {
		t.Errorf("esperado 404 Not Found para continue con cambio inexistente, obtenido %d", rr5.Code)
	}

	// 6. POST /api/increments/verify - Para cambio inexistente
	reqBodyVerify404, _ := json.Marshal(IncrementActionRequest{Name: "cambio-fantasma-999"})
	req6 := httptest.NewRequest(http.MethodPost, "/api/increments/verify", bytes.NewReader(reqBodyVerify404))
	rr6 := httptest.NewRecorder()
	router.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusNotFound {
		t.Errorf("esperado 404 Not Found para verify con cambio inexistente, obtenido %d", rr6.Code)
	}

	// 7. POST /api/handoffs - Creación válida
	reqBodyHandoff, _ := json.Marshal(CreateHandoffRequest{
		Change:           "inc-test-auth",
		FromPhase:        "design",
		ToPhase:          "tasks",
		FromRole:         "architect",
		ToRole:           "backend",
		Status:           "ready",
		ExecutiveSummary: "Diseño de autenticación completado y aprobado",
		Artifacts:        "openspec/changes/inc-test-auth/design.md",
		Decisions:        "Tokens JWT con rotación simétrica",
		Risks:            "Expiración de claves en sesiones activas",
		Instructions:     "Implementar middleware de validación en Go",
	})
	req7 := httptest.NewRequest(http.MethodPost, "/api/handoffs", bytes.NewReader(reqBodyHandoff))
	rr7 := httptest.NewRecorder()
	router.ServeHTTP(rr7, req7)

	if rr7.Code != http.StatusCreated {
		t.Fatalf("POST /api/handoffs esperado 201 Created, obtenido %d: %s", rr7.Code, rr7.Body.String())
	}
	var res7 CreateHandoffResponse
	if err := json.Unmarshal(rr7.Body.Bytes(), &res7); err != nil {
		t.Fatalf("error decodificando respuesta de /api/handoffs: %v", err)
	}
	if !res7.Success {
		t.Errorf("esperado success=true en /api/handoffs")
	}

	// Comprobar que handoff.md existe y contiene Frontmatter y encabezados en español
	handoffPath := filepath.Join(tempDir, "openspec", "changes", "inc-test-auth", "handoff.md")
	handoffContent, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("handoff.md no fue creado en disco: %v", err)
	}
	if !strings.Contains(string(handoffContent), "from_phase: design") ||
		!strings.Contains(string(handoffContent), "# 1. Resumen Ejecutivo") ||
		!strings.Contains(string(handoffContent), "Tokens JWT con rotación simétrica") {
		t.Errorf("handoff.md no contiene el formato canónico esperado: %s", string(handoffContent))
	}

	// 8. POST /api/handoffs - Rechazo por fase inválida
	reqBodyHandoffBadPhase, _ := json.Marshal(CreateHandoffRequest{
		Change:           "inc-test-auth",
		FromPhase:        "fase-inventada",
		ToPhase:          "apply",
		FromRole:         "architect",
		ToRole:           "backend",
		Status:           "ready",
		ExecutiveSummary: "Resumen cualquiera",
	})
	req8 := httptest.NewRequest(http.MethodPost, "/api/handoffs", bytes.NewReader(reqBodyHandoffBadPhase))
	rr8 := httptest.NewRecorder()
	router.ServeHTTP(rr8, req8)
	if rr8.Code != http.StatusBadRequest {
		t.Errorf("esperado 400 Bad Request para handoff con fase inválida, obtenido %d", rr8.Code)
	}
}

func TestEcosystemEndpoints(t *testing.T) {
	svc := NewService("../..")
	srv := NewServer(svc)
	router := srv.Router()

	// 1. GET /api/ecosystem/doctor
	req1 := httptest.NewRequest(http.MethodGet, "/api/ecosystem/doctor", nil)
	rr1 := httptest.NewRecorder()
	router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("esperado 200 OK para /api/ecosystem/doctor, obtenido %d", rr1.Code)
	}
	var doc DoctorReport
	if err := json.Unmarshal(rr1.Body.Bytes(), &doc); err != nil {
		t.Fatalf("error decodificando /api/ecosystem/doctor: %v", err)
	}
	if len(doc.Checks) == 0 {
		t.Errorf("se esperaban chequeos de salud en /api/ecosystem/doctor")
	}

	// 2. GET /api/ecosystem/backups
	req2 := httptest.NewRequest(http.MethodGet, "/api/ecosystem/backups", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("esperado 200 OK para /api/ecosystem/backups, obtenido %d", rr2.Code)
	}
	var backups []BackupItem
	if err := json.Unmarshal(rr2.Body.Bytes(), &backups); err != nil {
		t.Fatalf("error decodificando /api/ecosystem/backups: %v", err)
	}

	// 3. POST /api/ecosystem/backups/create
	reqBodyCreate, _ := json.Marshal(BackupActionRequest{Description: "snapshot de prueba unitaria"})
	req3 := httptest.NewRequest(http.MethodPost, "/api/ecosystem/backups/create", bytes.NewReader(reqBodyCreate))
	rr3 := httptest.NewRecorder()
	router.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusCreated {
		t.Errorf("esperado 201 Created para /api/ecosystem/backups/create, obtenido %d", rr3.Code)
	}
	var created BackupItem
	if err := json.Unmarshal(rr3.Body.Bytes(), &created); err != nil {
		t.Fatalf("error decodificando respuesta creación respaldo: %v", err)
	}
	if created.Name == "" {
		t.Errorf("se esperaba ID de snapshot no vacío")
	}

	// 4. POST /api/ecosystem/backups/restore - Validación de nombre vacío
	reqBodyRestoreBad, _ := json.Marshal(BackupActionRequest{Name: ""})
	req4 := httptest.NewRequest(http.MethodPost, "/api/ecosystem/backups/restore", bytes.NewReader(reqBodyRestoreBad))
	rr4 := httptest.NewRecorder()
	router.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusBadRequest {
		t.Errorf("esperado 400 Bad Request para restore sin nombre, obtenido %d", rr4.Code)
	}

	// 5. GET /api/ecosystem/models
	req5 := httptest.NewRequest(http.MethodGet, "/api/ecosystem/models", nil)
	rr5 := httptest.NewRecorder()
	router.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Errorf("esperado 200 OK para /api/ecosystem/models, obtenido %d", rr5.Code)
	}
	var models ModelAssignmentsDTO
	if err := json.Unmarshal(rr5.Body.Bytes(), &models); err != nil {
		t.Fatalf("error decodificando /api/ecosystem/models: %v", err)
	}
	if models.ActivePersona == "" {
		t.Errorf("esperada ActivePersona no vacía en /api/ecosystem/models")
	}

	// 6. POST /api/ecosystem/sync
	req6 := httptest.NewRequest(http.MethodPost, "/api/ecosystem/sync", nil)
	rr6 := httptest.NewRecorder()
	router.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK && rr6.Code != http.StatusInternalServerError {
		t.Errorf("código inesperado para sync: %d", rr6.Code)
	}

	// 7. POST /api/ecosystem/upgrade
	req7 := httptest.NewRequest(http.MethodPost, "/api/ecosystem/upgrade", nil)
	rr7 := httptest.NewRecorder()
	router.ServeHTTP(rr7, req7)
	if rr7.Code != http.StatusOK && rr7.Code != http.StatusInternalServerError {
		t.Errorf("código inesperado para upgrade: %d", rr7.Code)
	}
}
