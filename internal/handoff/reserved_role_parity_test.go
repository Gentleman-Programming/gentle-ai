package handoff

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// parityWorkspaceConfig declares roles but never "fullstack" — the same H-4
// shape internal/multirole/detector_test.go's noFullstackWorkspaceConfig
// uses, so both copies of roleExists are exercised against an identical
// workspace shape.
func parityWorkspaceConfig() *workspace.WorkspaceConfig {
	return &workspace.WorkspaceConfig{
		Roles: map[string]workspace.RoleConfig{
			"core": {Name: "core"},
			"qa":   {Name: "qa"},
		},
	}
}

// reservedRoleParityCorpus is the shared corpus of role names task 15.6
// requires: reserved identities in several cases, workspace-declared
// ordinary roles, an undeclared ordinary role, and an empty string. Both
// copies of roleExists must agree on every single entry.
var reservedRoleParityCorpus = []string{
	"fullstack",
	"Fullstack",
	"FULLSTACK",
	"core",
	"qa",
	"database",
	"",
}

// TestReservedRoleParityBetweenMultiroleAndHandoff is task 15.6's property:
// internal/multirole/detector.go's roleExists and this package's own
// validator.go roleExists must produce EXACTLY the same verdict for every
// name in the shared corpus. It calls this package's private roleExists
// directly (same package) and multirole's exported RoleExists (the real
// implementation DetectRoles itself consults, not a re-derived copy) — a
// future third copy of this predicate that silently diverges would fail
// this test the moment it disagrees with either existing copy.
func TestReservedRoleParityBetweenMultiroleAndHandoff(t *testing.T) {
	cfg := parityWorkspaceConfig()
	for _, role := range reservedRoleParityCorpus {
		t.Run(role, func(t *testing.T) {
			gotHandoff := roleExists(cfg, role)
			gotMultirole := multirole.RoleExists(cfg, role)
			if gotHandoff != gotMultirole {
				t.Fatalf("handoff.roleExists(cfg, %q) = %v, multirole.RoleExists(cfg, %q) = %v: las dos copias deben coincidir", role, gotHandoff, role, gotMultirole)
			}
		})
	}
}

// TestReservedRoleParityBothRecognizeFullstackWithoutWorkspaceDeclaration is
// the concrete regression this parity test exists to prevent: both copies
// must accept "fullstack" even though parityWorkspaceConfig never declares
// it (D-07). A property test that only proved agreement without also
// proving WHAT they agree on could pass by both sides being wrong the same
// way; this pins the actual expected value too.
func TestReservedRoleParityBothRecognizeFullstackWithoutWorkspaceDeclaration(t *testing.T) {
	cfg := parityWorkspaceConfig()
	if !roleExists(cfg, "fullstack") {
		t.Error("handoff.roleExists(cfg, \"fullstack\") = false, se esperaba true")
	}
	if !multirole.RoleExists(cfg, "fullstack") {
		t.Error("multirole.RoleExists(cfg, \"fullstack\") = false, se esperaba true")
	}
}
