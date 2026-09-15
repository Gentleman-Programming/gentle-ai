package dashboard

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if !hasArchived {
		t.Errorf("se esperaba encontrar incrementos archivados (ej. inc-01, inc-02, inc-03)")
	}
	if !hasActive {
		t.Errorf("se esperaba encontrar al menos un incremento activo")
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

