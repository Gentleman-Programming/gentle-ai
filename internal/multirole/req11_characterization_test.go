package multirole

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// req11ControlWorkspaceConfig mirrors this repository's own axiom.yaml
// closely enough for REQ-1.1's own three scenarios
// (openspec/specs/multi-role-fan-out/spec.md): "backend" and "e2e" declared
// (the exact role names the spec's first scenario names), "core" declared
// for the single-role fallback scenario. "database" is deliberately
// absent: REQ-1.1's second scenario needs exactly that gap, and "fullstack"
// is deliberately absent too — this is the control fixture that proves
// D-07's compatibility table before task 15.3-15.7 ever touch roleExists.
func req11ControlWorkspaceConfig() *workspace.WorkspaceConfig {
	return &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"backend": {Name: "backend"},
			"e2e":     {Name: "e2e"},
			"core":    {Name: "core"},
		},
	}
}

func writeReq11DesignMD(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "design.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("preparar design.md: %v", err)
	}
	return path
}

// TestREQ11DetectionOfMultipleRolesWithDistinctPolicies is the control gate
// for multi-role-fan-out-engine's REQ-1.1, first scenario ("Detección
// exitosa de múltiples roles con distintas políticas",
// openspec/specs/multi-role-fan-out/spec.md:23-27): backend (blocking) and
// e2e (deferred) are both identified, and no error is reported. Task 15.1
// runs this against the UNMODIFIED package (before any roleExists change);
// task 15.8 re-runs it after 15.3-15.7 and must see the identical result —
// neither role declared here is named "fullstack", so D-07's compatibility
// table ("No cambia") must hold in practice, not just in the design's
// prose.
func TestREQ11DetectionOfMultipleRolesWithDistinctPolicies(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\n"+
		"```yaml\n"+
		"roles:\n"+
		"  - role: backend\n"+
		"    gate_policy: blocking\n"+
		"  - role: e2e\n"+
		"    gate_policy: deferred\n"+
		"```\n")

	roles, err := DetectRoles(designPath, req11ControlWorkspaceConfig())
	if err != nil {
		t.Fatalf("DetectRoles() error = %v, se esperaba exito (REQ-1.1, primer escenario)", err)
	}
	if len(roles) != 2 {
		t.Fatalf("roles = %+v, se esperaban exactamente 2", roles)
	}
	byRole := map[string]RoleAssignment{}
	for _, r := range roles {
		byRole[r.Role] = r
	}
	if byRole["backend"].GatePolicy != PolicyBlocking {
		t.Errorf("backend.GatePolicy = %q, se esperaba blocking", byRole["backend"].GatePolicy)
	}
	if byRole["e2e"].GatePolicy != PolicyDeferred {
		t.Errorf("e2e.GatePolicy = %q, se esperaba deferred", byRole["e2e"].GatePolicy)
	}
}

// TestREQ11ValidationAgainstAxiomYAMLRejectsUndeclaredRole is REQ-1.1's
// second scenario ("Validación cruzada de roles contra axiom.yaml",
// spec.md:29-33): a role declared in design.md but absent from axiom.yaml
// (here, "database") produces a named error. fullstack becoming a reserved
// identity (D-07) must never widen this rejection: "database" is not
// reserved and must stay rejected, byte-for-byte, before (15.1) and after
// (15.8) tasks 15.3-15.7.
func TestREQ11ValidationAgainstAxiomYAMLRejectsUndeclaredRole(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\n"+
		"```yaml\n"+
		"roles:\n"+
		"  - role: database\n"+
		"    gate_policy: blocking\n"+
		"```\n")

	_, err := DetectRoles(designPath, req11ControlWorkspaceConfig())
	if err == nil {
		t.Fatal("DetectRoles() = nil error, se esperaba rechazo: database no esta en axiom.yaml (REQ-1.1, segundo escenario)")
	}
	if !strings.Contains(err.Error(), "database") || !strings.Contains(err.Error(), "no existe en la configuración") {
		t.Fatalf("error = %q, se esperaba que nombrase 'database' y su ausencia en axiom.yaml", err.Error())
	}
}

// TestREQ11BackwardCompatibleSingleRoleFallback is REQ-1.1's third scenario
// ("Modo retrocompatible para diseño de rol único", spec.md:35-39): a
// design.md with no explicit roles section falls back to a single
// assignment for the workspace's main role with GatePolicy blocking, AND
// associates the canonical tasks.md file for that role — verified here
// through EvaluateBarrier's own single-role fallback (barrier.go:88-93),
// which is the concrete mechanism that makes the spec's "asocia el archivo
// canónico tasks.md" clause observable end to end, not just DetectRoles'
// own return value.
func TestREQ11BackwardCompatibleSingleRoleFallback(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\nSin seccion de roles.\n")

	roles, err := DetectRoles(designPath, req11ControlWorkspaceConfig())
	if err != nil {
		t.Fatalf("DetectRoles() error = %v, se esperaba fallback de rol unico (REQ-1.1, tercer escenario)", err)
	}
	if len(roles) != 1 {
		t.Fatalf("roles = %+v, se esperaba exactamente 1 (fallback)", roles)
	}
	if roles[0].GatePolicy != PolicyBlocking {
		t.Errorf("GatePolicy = %q, se esperaba blocking", roles[0].GatePolicy)
	}

	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("- [x] unica tarea\n"), 0644); err != nil {
		t.Fatalf("preparar tasks.md: %v", err)
	}
	report, err := EvaluateBarrier(dir, "inc-req11-control", roles)
	if err != nil {
		t.Fatalf("EvaluateBarrier() error = %v", err)
	}
	if len(report.Roles) != 1 || !report.Roles[0].TasksFound {
		t.Fatalf("EvaluateBarrier() = %+v, se esperaba que el fallback de rol unico encontrase tasks.md", report.Roles)
	}
}
