package multirole

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
)

const sampleDesignWithRoles = `# Documento de Diseño

## Roles Participantes

` + "```yaml" + `
roles:
  - role: backend
    gate_policy: blocking
    repositories: ["api"]
  - role: frontend
    gate_policy: blocking
    repositories: ["web"]
  - role: e2e
    gate_policy: deferred
    repositories: ["tests/e2e"]
` + "```" + `

## Arquitectura
Detalles técnicos...
`

func TestParseRolesMarkdown(t *testing.T) {
	roles, err := ParseRolesMarkdown(sampleDesignWithRoles)
	if err != nil {
		t.Fatalf("ParseRolesMarkdown falló: %v", err)
	}

	if len(roles) != 3 {
		t.Fatalf("Se esperaban 3 roles, obtenidos %d", len(roles))
	}

	if roles[0].Role != "backend" || roles[0].GatePolicy != PolicyBlocking {
		t.Errorf("Rol 0 inesperado: %+v", roles[0])
	}
	if roles[1].Role != "frontend" || roles[1].GatePolicy != PolicyBlocking {
		t.Errorf("Rol 1 inesperado: %+v", roles[1])
	}
	if roles[2].Role != "e2e" || roles[2].GatePolicy != PolicyDeferred {
		t.Errorf("Rol 2 inesperado: %+v", roles[2])
	}
}

func TestDetectRolesFallback(t *testing.T) {
	tmpDir := t.TempDir()
	designPath := filepath.Join(tmpDir, "design.md")
	if err := os.WriteFile(designPath, []byte("# Diseño Simple\nSin roles explícitos."), 0644); err != nil {
		t.Fatal(err)
	}

	wsConfig := &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"core": {Name: "Core Engine"},
		},
	}

	roles, err := DetectRoles(designPath, wsConfig)
	if err != nil {
		t.Fatalf("DetectRoles falló en fallback: %v", err)
	}

	if len(roles) != 1 {
		t.Fatalf("Se esperaba 1 rol por fallback, obtenidos %d", len(roles))
	}
	if roles[0].Role != "core" || roles[0].GatePolicy != PolicyBlocking {
		t.Errorf("Rol de fallback inesperado: %+v", roles[0])
	}
}

func TestDetectRolesInvalidRole(t *testing.T) {
	tmpDir := t.TempDir()
	designPath := filepath.Join(tmpDir, "design.md")
	content := `# Diseño
` + "```yaml" + `
roles:
  - role: unknown-role
    gate_policy: blocking
` + "```"
	if err := os.WriteFile(designPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	wsConfig := &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"backend": {Name: "Backend"},
		},
	}

	_, err := DetectRoles(designPath, wsConfig)
	if err == nil {
		t.Fatal("Se esperaba error por rol inexistente pero fue nil")
	}
	if !strings.Contains(err.Error(), "no existe en la configuración") {
		t.Errorf("Mensaje de error inesperado: %v", err)
	}
}

func TestCountTasks(t *testing.T) {
	sampleTasks := `# Tareas
- [x] Tarea 1 completada
- [X] Tarea 2 completada con mayúscula
- [ ] Tarea 3 pendiente
- [ ] Tarea 4 pendiente
- [x] Tarea 5 completada
`
	prog := CountTasks(sampleTasks)
	if prog.Total != 5 {
		t.Errorf("Total esperado 5, obtenido %d", prog.Total)
	}
	if prog.Completed != 3 {
		t.Errorf("Completadas esperado 3, obtenido %d", prog.Completed)
	}
	if prog.Pending != 2 {
		t.Errorf("Pendientes esperado 2, obtenido %d", prog.Pending)
	}
	if prog.Percent != 60.0 {
		t.Errorf("Porcentaje esperado 60.0, obtenido %f", prog.Percent)
	}
}

func TestExtractPendingTasksAndFormat(t *testing.T) {
	tasksContent := `- [x] Tarea hecha
- [ ] Probar flujo OAuth2 con Google
- [ ] Verificar timeout en staging
`
	deferred := ExtractPendingTasks("inc-03-test", "e2e", "spec.md", "changeDir/", tasksContent)
	if len(deferred) != 2 {
		t.Fatalf("Se esperaban 2 tareas diferidas, obtenidas %d", len(deferred))
	}

	if deferred[0].TaskText != "Probar flujo OAuth2 con Google" {
		t.Errorf("Texto de tarea inesperado: %q", deferred[0].TaskText)
	}

	formatted := FormatDeferredTaskMarkdown(deferred[0])
	if !strings.Contains(formatted, "[Ref: inc-03-test] (e2e) Probar flujo OAuth2") {
		t.Errorf("Formato de tarea diferida inválido: %s", formatted)
	}
}

func TestEvaluateBarrierAllBlockingPass(t *testing.T) {
	tmpDir := t.TempDir()

	// backend: 100% tareas, verify pass
	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.backend.md"), []byte("- [x] Tarea 1\n- [x] Tarea 2"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "verify-report.backend.md"), []byte("verdict: pass\n"), 0644)

	// frontend: 100% tareas, verify pass
	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.frontend.md"), []byte("- [x] UI 1"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "verify-report.frontend.md"), []byte("verdict: pass\n"), 0644)

	roles := []RoleAssignment{
		{Role: "backend", GatePolicy: PolicyBlocking},
		{Role: "frontend", GatePolicy: PolicyBlocking},
	}

	report, err := EvaluateBarrier(tmpDir, "inc-test", roles)
	if err != nil {
		t.Fatalf("EvaluateBarrier falló: %v", err)
	}

	if !report.Satisfied {
		t.Errorf("Se esperaba Satisfied: true, pero fue false con bloqueos: %v", report.Blockers)
	}
	if len(report.Blockers) != 0 {
		t.Errorf("Bloqueos inesperados: %v", report.Blockers)
	}
}

func TestEvaluateBarrierBlockedByPendingTasks(t *testing.T) {
	tmpDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.backend.md"), []byte("- [x] Tarea 1\n- [ ] Tarea 2 pendiente"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "verify-report.backend.md"), []byte("verdict: pass\n"), 0644)

	roles := []RoleAssignment{
		{Role: "backend", GatePolicy: PolicyBlocking},
	}

	report, err := EvaluateBarrier(tmpDir, "inc-test", roles)
	if err != nil {
		t.Fatalf("EvaluateBarrier falló: %v", err)
	}

	if report.Satisfied {
		t.Error("Se esperaba Satisfied: false debido a tareas pendientes")
	}
	if len(report.Blockers) == 0 || !strings.Contains(report.Blockers[0], "tarea(s) pendiente(s)") {
		t.Errorf("Bloqueo esperado no encontrado: %v", report.Blockers)
	}
}

func TestEvaluateBarrierBlockedByVerifyFail(t *testing.T) {
	tmpDir := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.backend.md"), []byte("- [x] Tarea 1"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "verify-report.backend.md"), []byte("verdict: fail\n"), 0644)

	roles := []RoleAssignment{
		{Role: "backend", GatePolicy: PolicyBlocking},
	}

	report, err := EvaluateBarrier(tmpDir, "inc-test", roles)
	if err != nil {
		t.Fatalf("EvaluateBarrier falló: %v", err)
	}

	if report.Satisfied {
		t.Error("Se esperaba Satisfied: false debido a fallo de verificación")
	}
	if len(report.Blockers) == 0 || !strings.Contains(report.Blockers[0], "verificación no superada") {
		t.Errorf("Bloqueo esperado no encontrado: %v", report.Blockers)
	}
}

func TestEvaluateBarrierDeferredRoleWithPendingTasks(t *testing.T) {
	tmpDir := t.TempDir()

	// backend (blocking) completado y verificado
	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.backend.md"), []byte("- [x] Tarea 1"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "verify-report.backend.md"), []byte("verdict: pass\n"), 0644)

	// e2e (deferred) tiene 1 tarea completada y 2 pendientes
	_ = os.WriteFile(filepath.Join(tmpDir, "tasks.e2e.md"), []byte("- [x] Test login\n- [ ] Test checkout\n- [ ] Test logout"), 0644)

	roles := []RoleAssignment{
		{Role: "backend", GatePolicy: PolicyBlocking},
		{Role: "e2e", GatePolicy: PolicyDeferred},
	}

	report, err := EvaluateBarrier(tmpDir, "inc-test", roles)
	if err != nil {
		t.Fatalf("EvaluateBarrier falló: %v", err)
	}

	// Debe ser SATISFIED porque los roles blocking están listos
	if !report.Satisfied {
		t.Errorf("Se esperaba Satisfied: true con rol diferido pendiente, pero fue false: %v", report.Blockers)
	}

	// Debe contener advertencia y tareas diferidas extraídas
	if len(report.Warnings) == 0 {
		t.Error("Se esperaba advertencia para el rol diferido")
	}
	if len(report.DeferredTasks) != 2 {
		t.Fatalf("Se esperaban 2 tareas diferidas extraídas, obtenidas %d", len(report.DeferredTasks))
	}
	if report.DeferredTasks[0].TaskText != "Test checkout" {
		t.Errorf("Texto diferido inesperado: %q", report.DeferredTasks[0].TaskText)
	}
}

func TestMigrateDeferredTasks(t *testing.T) {
	tmpDir := t.TempDir()
	cumulativeFile := filepath.Join(tmpDir, "e2e-cumulative", "tasks.md")

	tasks := []DeferredTask{
		{
			Change:       "inc-02-auth",
			Role:         "e2e",
			TaskText:     "Probar login Google",
			SpecRef:      "openspec/specs/auth/spec.md",
			ChangeDirRef: "openspec/changes/archive/inc-02-auth/",
		},
	}

	if err := MigrateDeferredTasks(cumulativeFile, tasks); err != nil {
		t.Fatalf("MigrateDeferredTasks falló: %v", err)
	}

	data, err := os.ReadFile(cumulativeFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "[Ref: inc-02-auth] (e2e) Probar login Google") {
		t.Errorf("Contenido acumulativo no contiene la tarea formateada: %s", content)
	}
}
