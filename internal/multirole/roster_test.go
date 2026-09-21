package multirole

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

func rosterWorkspaceConfig() *workspace.WorkspaceConfig {
	return &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"core": {Name: "core"},
			"web":  {Name: "web"},
			"qa":   {Name: "qa"},
		},
	}
}

// TestResolveRosterSealedWithMatchingDesignHasNoConflict is task 16.1's
// first case: a non-empty sealed roster always wins (RosterSourceKickoff),
// and when design.md agrees with it there is no conflict to report.
func TestResolveRosterSealedWithMatchingDesignHasNoConflict(t *testing.T) {
	dir := t.TempDir()
	writeReq11DesignMD(t, dir, "# Diseno\n\n```yaml\nroles:\n  - role: core\n    gate_policy: blocking\n```\n")
	sealed := []RoleAssignment{{Role: "core", GatePolicy: PolicyBlocking}}

	roster, err := ResolveRoster(sealed, dir+"/design.md", rosterWorkspaceConfig())
	if err != nil {
		t.Fatalf("ResolveRoster() error = %v", err)
	}
	if roster.Source != RosterSourceKickoff {
		t.Errorf("Source = %q, se esperaba %q", roster.Source, RosterSourceKickoff)
	}
	if !reflect.DeepEqual(roster.Roles, sealed) {
		t.Errorf("Roles = %+v, se esperaba exactamente el sellado %+v", roster.Roles, sealed)
	}
	if roster.Conflict != nil {
		t.Errorf("Conflict = %+v, se esperaba nil: design.md coincide con el sellado", roster.Conflict)
	}
}

// TestResolveRosterSealedWinsOverConflictingDesign is task 16.1's second
// case: sealed roles are declared authoritative even when design.md
// declares DIFFERENT roles — the sealed roster is never silently replaced,
// and the discrepancy is surfaced as a non-blocking Conflict.
func TestResolveRosterSealedWinsOverConflictingDesign(t *testing.T) {
	dir := t.TempDir()
	writeReq11DesignMD(t, dir, "# Diseno\n\n```yaml\nroles:\n  - role: web\n    gate_policy: blocking\n  - role: qa\n    gate_policy: deferred\n```\n")
	sealed := []RoleAssignment{{Role: "core", GatePolicy: PolicyBlocking}}

	roster, err := ResolveRoster(sealed, dir+"/design.md", rosterWorkspaceConfig())
	if err != nil {
		t.Fatalf("ResolveRoster() error = %v", err)
	}
	if roster.Source != RosterSourceKickoff {
		t.Errorf("Source = %q, se esperaba %q (el sellado manda)", roster.Source, RosterSourceKickoff)
	}
	if !reflect.DeepEqual(roster.Roles, sealed) {
		t.Fatalf("Roles = %+v, se esperaba que el roster sellado siguiera mandando: %+v", roster.Roles, sealed)
	}
	if roster.Conflict == nil {
		t.Fatal("Conflict = nil, se esperaba un conflicto: design.md declara roles distintos del sellado")
	}
	if len(roster.Conflict.KickoffRoles) != 1 || roster.Conflict.KickoffRoles[0] != "core" {
		t.Errorf("Conflict.KickoffRoles = %+v, se esperaba [core]", roster.Conflict.KickoffRoles)
	}
	if len(roster.Conflict.DesignRoles) != 2 {
		t.Errorf("Conflict.DesignRoles = %+v, se esperaban 2 roles (web, qa)", roster.Conflict.DesignRoles)
	}
	if roster.Conflict.Detail == "" {
		t.Error("Conflict.Detail vacio, se esperaba una descripcion del desacuerdo")
	}
}

// TestResolveRosterEmptySealedDelegatesToDetectRolesDesignSource is task
// 16.1's third case: an empty sealed roster delegates to DetectRoles, and
// when design.md declares explicit roles the source is classified as
// RosterSourceDesign.
func TestResolveRosterEmptySealedDelegatesToDetectRolesDesignSource(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\n```yaml\nroles:\n  - role: core\n    gate_policy: blocking\n  - role: qa\n    gate_policy: deferred\n```\n")

	roster, err := ResolveRoster(nil, designPath, rosterWorkspaceConfig())
	if err != nil {
		t.Fatalf("ResolveRoster() error = %v", err)
	}
	if roster.Source != RosterSourceDesign {
		t.Errorf("Source = %q, se esperaba %q", roster.Source, RosterSourceDesign)
	}
	if roster.Conflict != nil {
		t.Errorf("Conflict = %+v, se esperaba nil: no hay sellado con el que discrepar", roster.Conflict)
	}

	// Equivalence: with sealed empty, the result must be IDENTICAL to what
	// DetectRoles returns directly for the same inputs — not just similar
	// in shape.
	direct, directErr := DetectRoles(designPath, rosterWorkspaceConfig())
	if directErr != nil {
		t.Fatalf("fixture invalido: DetectRoles() error = %v", directErr)
	}
	if !reflect.DeepEqual(roster.Roles, direct) {
		t.Fatalf("ResolveRoster().Roles = %+v, se esperaba identico a DetectRoles() = %+v", roster.Roles, direct)
	}
}

// TestResolveRosterEmptySealedDelegatesToDetectRolesCompatibilitySource is
// task 16.1's third case, fallback branch: design.md with no explicit roles
// section classifies as RosterSourceCompatibility, and the result still
// matches DetectRoles' own fallback output exactly.
func TestResolveRosterEmptySealedDelegatesToDetectRolesCompatibilitySource(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\nSin seccion de roles.\n")

	roster, err := ResolveRoster(nil, designPath, rosterWorkspaceConfig())
	if err != nil {
		t.Fatalf("ResolveRoster() error = %v", err)
	}
	if roster.Source != RosterSourceCompatibility {
		t.Errorf("Source = %q, se esperaba %q", roster.Source, RosterSourceCompatibility)
	}
	if roster.Conflict != nil {
		t.Errorf("Conflict = %+v, se esperaba nil", roster.Conflict)
	}

	direct, directErr := DetectRoles(designPath, rosterWorkspaceConfig())
	if directErr != nil {
		t.Fatalf("fixture invalido: DetectRoles() error = %v", directErr)
	}
	if !reflect.DeepEqual(roster.Roles, direct) {
		t.Fatalf("ResolveRoster().Roles = %+v, se esperaba identico a DetectRoles() = %+v", roster.Roles, direct)
	}
}

// TestResolveRosterEmptySealedPropagatesDetectRolesError confirms
// ResolveRoster never swallows or masks a genuine DetectRoles error (e.g. a
// design-declared role absent from axiom.yaml) when there is no sealed
// roster to fall back on.
func TestResolveRosterEmptySealedPropagatesDetectRolesError(t *testing.T) {
	dir := t.TempDir()
	designPath := writeReq11DesignMD(t, dir, "# Diseno\n\n```yaml\nroles:\n  - role: database\n    gate_policy: blocking\n```\n")

	_, err := ResolveRoster(nil, designPath, rosterWorkspaceConfig())
	if err == nil {
		t.Fatal("ResolveRoster() = nil error, se esperaba el error de DetectRoles propagado (database ausente de axiom.yaml)")
	}
}
