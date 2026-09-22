package kickoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

func writeDesignMD(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "design.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("preparar design.md: %v", err)
	}
	return path
}

// repoLikeWorkspaceConfig mirrors this repository's own axiom.yaml: roles
// core and e2e declared, no fullstack (H-4).
func repoLikeWorkspaceConfig() *workspace.WorkspaceConfig {
	return &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"core": {Name: "core"},
			"e2e":  {Name: "e2e"},
		},
	}
}

// Case (a), sub-case 1: design.md without an explicit roles block falls back
// to whatever DetectRoles already infers today (here, "core", because the
// workspace declares it) - NEVER "fullstack". This is the exact assertion
// that fixes O-1.
func TestInferKickoffDesignWithoutExplicitRolesFreezesCurrentFallback(t *testing.T) {
	changeRoot := t.TempDir()
	designPath := writeDesignMD(t, changeRoot, "# Diseno\n\nSin bloque de roles declarado.\n")

	k, note, err := InferKickoff(designPath, repoLikeWorkspaceConfig())
	if err != nil {
		t.Fatalf("InferKickoff() error inesperado = %v", err)
	}
	if note != "" {
		t.Errorf("nota = %q, se esperaba vacia en el caso feliz", note)
	}
	if len(k.Config.Roles) != 1 || k.Config.Roles[0].Role != "core" {
		t.Fatalf("Roles = %+v, se esperaba exactamente [core] (el fallback vigente), nunca fullstack", k.Config.Roles)
	}
	if k.Config.ExecutionStyle != ExecutionContinuous {
		t.Errorf("ExecutionStyle = %q, se esperaba continuous", k.Config.ExecutionStyle)
	}
	if k.Config.ExecutionStyleSource != "inferred" {
		t.Errorf("ExecutionStyleSource = %q, se esperaba inferred", k.Config.ExecutionStyleSource)
	}
	if k.Config.HandoffPolicy != HandoffNone {
		t.Errorf("HandoffPolicy = %q, se esperaba none", k.Config.HandoffPolicy)
	}
	if k.SealedBy != "inferred" {
		t.Errorf("SealedBy = %q, se esperaba inferred", k.SealedBy)
	}
	for _, r := range k.Config.Roles {
		if strings.EqualFold(r.Role, "fullstack") {
			t.Fatalf("el retro-sello introdujo fullstack pese a que DetectRoles ya resolvia %q sin error", r.Role)
		}
	}
}

// Case (a), sub-case 2: design.md WITH an explicit multi-role block freezes
// exactly those roles.
func TestInferKickoffDesignWithExplicitMultiRoleFreezesThoseRoles(t *testing.T) {
	changeRoot := t.TempDir()
	designPath := writeDesignMD(t, changeRoot,
		"# Diseno\n\n```yaml\nroles:\n  - role: core\n    gate_policy: blocking\n  - role: e2e\n    gate_policy: deferred\n```\n")

	k, note, err := InferKickoff(designPath, repoLikeWorkspaceConfig())
	if err != nil {
		t.Fatalf("InferKickoff() error inesperado = %v", err)
	}
	if note != "" {
		t.Errorf("nota = %q, se esperaba vacia", note)
	}
	if len(k.Config.Roles) != 2 {
		t.Fatalf("Roles = %+v, se esperaban 2 roles declarados explicitamente", k.Config.Roles)
	}
	seen := map[string]bool{}
	for _, r := range k.Config.Roles {
		seen[r.Role] = true
		if r.Role == "fullstack" {
			t.Fatal("el retro-sello introdujo fullstack junto a roles explicitos")
		}
	}
	if !seen["core"] || !seen["e2e"] {
		t.Fatalf("Roles = %+v, se esperaban exactamente core y e2e", k.Config.Roles)
	}
}

// Case (b): no design.md exists yet. Nothing reads roles today, so there is
// no prior behaviour to preserve: seal the single fullstack role.
func TestInferKickoffNoDesignSealsFullstackBlocking(t *testing.T) {
	changeRoot := t.TempDir()
	designPath := filepath.Join(changeRoot, "design.md") // never written: absent on purpose

	k, note, err := InferKickoff(designPath, repoLikeWorkspaceConfig())
	if err != nil {
		t.Fatalf("InferKickoff() error inesperado = %v", err)
	}
	if note != "" {
		t.Errorf("nota = %q, se esperaba vacia", note)
	}
	if len(k.Config.Roles) != 1 || k.Config.Roles[0].Role != "fullstack" {
		t.Fatalf("Roles = %+v, se esperaba exactamente [fullstack]", k.Config.Roles)
	}
	if k.Config.Roles[0].GatePolicy != multirole.PolicyBlocking {
		t.Errorf("GatePolicy = %q, se esperaba blocking", k.Config.Roles[0].GatePolicy)
	}
	if k.Config.Roles[0].TasksFile != "tasks.md" || k.Config.Roles[0].VerifyFile != "verify-report.md" {
		t.Errorf("ficheros de rol = (%q, %q), se esperaba (tasks.md, verify-report.md)",
			k.Config.Roles[0].TasksFile, k.Config.Roles[0].VerifyFile)
	}
}

// Case (c): DetectRoles itself errors (a role declared in design.md is
// absent from axiom.yaml). InferKickoff must infer NOTHING, propagate the
// existing error intact, and surface a non-blocking note.
func TestInferKickoffDetectRolesErrorInfersNothingAndPropagatesIntact(t *testing.T) {
	changeRoot := t.TempDir()
	designPath := writeDesignMD(t, changeRoot,
		"# Diseno\n\n```yaml\nroles:\n  - role: database\n    gate_policy: blocking\n```\n")

	_, note, err := InferKickoff(designPath, repoLikeWorkspaceConfig())
	if err == nil {
		t.Fatal("InferKickoff() con un rol ausente de axiom.yaml debia propagar el error de DetectRoles")
	}

	_, directErr := multirole.DetectRoles(designPath, repoLikeWorkspaceConfig())
	if directErr == nil {
		t.Fatal("fixture invalido: DetectRoles no fallo como se esperaba")
	}
	if err.Error() != directErr.Error() {
		t.Fatalf("InferKickoff() error = %q, se esperaba el error intacto de DetectRoles = %q", err.Error(), directErr.Error())
	}
	if note == "" {
		t.Error("se esperaba una nota no bloqueante cuando DetectRoles falla")
	}
}

// Refactor guardrail (3.3): InferKickoff never writes kickoff.yaml. The
// caller seals the returned value via Seal.
func TestInferKickoffNeverWritesKickoffFile(t *testing.T) {
	changeRoot := t.TempDir()
	designPath := writeDesignMD(t, changeRoot, "# Diseno\n\nSin roles.\n")

	if _, _, err := InferKickoff(designPath, repoLikeWorkspaceConfig()); err != nil {
		t.Fatalf("InferKickoff() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(changeRoot, KickoffFileName)); err == nil {
		t.Fatal("InferKickoff() escribio kickoff.yaml; debe ser puro respecto a E/S de escritura")
	}
}
