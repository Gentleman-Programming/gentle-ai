package multirole

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// noFullstackWorkspaceConfig declares roles but never "fullstack" — the
// exact H-4 shape (design.md: axiom.yaml declares core/qa/e2e, never
// fullstack).
func noFullstackWorkspaceConfig() *workspace.WorkspaceConfig {
	return &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"core": {Name: "core"},
			"qa":   {Name: "qa"},
		},
	}
}

// TestRoleExistsAcceptsReservedFullstackWithoutWorkspaceDeclaration is task
// 15.4's RED: roleExists must recognise the reserved "fullstack" identity
// even when the workspace's axiom.yaml never declares it (D-07) — this is
// the exact gap H-4 names.
func TestRoleExistsAcceptsReservedFullstackWithoutWorkspaceDeclaration(t *testing.T) {
	if !roleExists(noFullstackWorkspaceConfig(), "fullstack") {
		t.Fatal("roleExists(cfg, \"fullstack\") = false, se esperaba true (D-07: identidad reservada)")
	}
	if !roleExists(noFullstackWorkspaceConfig(), "Fullstack") {
		t.Fatal("roleExists(cfg, \"Fullstack\") = false, se esperaba true (insensible a mayusculas)")
	}
}

// TestRoleExistsStillRejectsUndeclaredOrdinaryRole is task 15.4's second
// case: an ordinary, non-reserved role absent from axiom.yaml (here
// "database", REQ-1.1's own second scenario) must keep producing false —
// the reserved branch is additive, never a general bypass.
func TestRoleExistsStillRejectsUndeclaredOrdinaryRole(t *testing.T) {
	if roleExists(noFullstackWorkspaceConfig(), "database") {
		t.Fatal("roleExists(cfg, \"database\") = true, se esperaba false: database no esta declarado y no es reservado")
	}
}

// TestRoleExistsStillAcceptsDeclaredOrdinaryRole confirms the pre-existing
// workspace-declaration path is untouched: a role the workspace DOES
// declare keeps resolving true, exactly as before this task's change.
func TestRoleExistsStillAcceptsDeclaredOrdinaryRole(t *testing.T) {
	if !roleExists(noFullstackWorkspaceConfig(), "core") {
		t.Fatal("roleExists(cfg, \"core\") = false, se esperaba true: core esta declarado en axiom.yaml")
	}
}

// TestRoleExistsExportedWrapperMatchesPrivateImplementation pins the new
// exported RoleExists as a pure delegate to the private roleExists this
// file already exercises above — no second, driftable copy of the
// predicate. internal/handoff's parity test (task 15.6) is the actual
// consumer of this exported form, since it cannot reach this package's
// unexported roleExists from a different package.
func TestRoleExistsExportedWrapperMatchesPrivateImplementation(t *testing.T) {
	cfg := noFullstackWorkspaceConfig()
	for _, role := range []string{"fullstack", "Fullstack", "database", "core", ""} {
		if got, want := RoleExists(cfg, role), roleExists(cfg, role); got != want {
			t.Errorf("RoleExists(cfg, %q) = %v, roleExists(cfg, %q) = %v: deben coincidir", role, got, role, want)
		}
	}
}
