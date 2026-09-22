package kickoff

import (
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

func approvedRoleApplyRecord(role string, at time.Time) GateRecord {
	return GateRecord{
		Gate:       RoleApplyGate(role),
		Decision:   DecisionApproved,
		Reason:     "implementacion completa",
		Actor:      "maintainer",
		RecordedAt: at,
	}
}

func closureTestWorkspaceConfig(roles ...string) *workspace.WorkspaceConfig {
	cfg := &workspace.WorkspaceConfig{Roles: map[string]workspace.RoleConfig{}}
	for _, r := range roles {
		cfg.Roles[r] = workspace.RoleConfig{Name: r}
	}
	return cfg
}

// TestIntegrationHandoffProducesValidReadyHandoff is task 17.1's base case:
// the produced handoff carries FromPhase apply, ToPhase verify, Status
// ready, and passes handoff.Validate's five-non-empty-section and
// transition checks end to end.
func TestIntegrationHandoffProducesValidReadyHandoff(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	roster := multirole.Roster{
		Source: multirole.RosterSourceKickoff,
		Roles: []multirole.RoleAssignment{
			{Role: "fullstack", GatePolicy: multirole.PolicyBlocking},
		},
	}
	gates := GateLedger{Records: []GateRecord{
		approvedRoleApplyRecord("fullstack", base),
	}}
	artifacts := ArtifactInventory{
		RoleTasksFiles:     map[string]string{"fullstack": "tasks.md"},
		RoleVerifyFiles:    map[string]string{"fullstack": "verify-report.fullstack.md"},
		GlobalVerifyReport: "verify-report.md",
	}

	h, err := IntegrationHandoff("inc-99-closure", roster, gates, artifacts)
	if err != nil {
		t.Fatalf("IntegrationHandoff() error = %v", err)
	}
	if h.Metadata.FromPhase != handoff.PhaseApply || h.Metadata.ToPhase != handoff.PhaseVerify {
		t.Fatalf("transicion = %s -> %s, se esperaba apply -> verify", h.Metadata.FromPhase, h.Metadata.ToPhase)
	}
	if h.Metadata.Status != handoff.StatusReady {
		t.Fatalf("Status = %q, se esperaba ready", h.Metadata.Status)
	}
	if h.Metadata.Change != "inc-99-closure" {
		t.Fatalf("Change = %q, se esperaba inc-99-closure", h.Metadata.Change)
	}

	if err := handoff.Validate(h, closureTestWorkspaceConfig("fullstack")); err != nil {
		t.Fatalf("handoff.Validate() error = %v, se esperaba un relevo valido", err)
	}
}

// TestIntegrationHandoffQARoleWinsToRoleEvenWhenNotLastClosed is D-11's
// first resolution rule: a roster role literally named "qa" (case
// insensitive) always becomes to_role, even when it was NOT the last role
// whose gate was approved.
func TestIntegrationHandoffQARoleWinsToRoleEvenWhenNotLastClosed(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	roster := multirole.Roster{Roles: []multirole.RoleAssignment{
		{Role: "core", GatePolicy: multirole.PolicyBlocking},
		{Role: "web", GatePolicy: multirole.PolicyBlocking},
		{Role: "qa", GatePolicy: multirole.PolicyBlocking},
	}}
	// qa approved FIRST, web approved LAST: rule 1 must still pick "qa" as
	// to_role, and "web" (the genuinely last-closed role) as from_role.
	gates := GateLedger{Records: []GateRecord{
		approvedRoleApplyRecord("qa", base),
		approvedRoleApplyRecord("core", base.Add(time.Minute)),
		approvedRoleApplyRecord("web", base.Add(2*time.Minute)),
	}}
	artifacts := ArtifactInventory{
		RoleTasksFiles: map[string]string{
			"core": "tasks.core.md", "web": "tasks.web.md", "qa": "tasks.qa.md",
		},
	}

	h, err := IntegrationHandoff("inc-99-closure", roster, gates, artifacts)
	if err != nil {
		t.Fatalf("IntegrationHandoff() error = %v", err)
	}
	if h.Metadata.ToRole != "qa" {
		t.Fatalf("ToRole = %q, se esperaba qa (regla 1 de D-11)", h.Metadata.ToRole)
	}
	if h.Metadata.FromRole != "web" {
		t.Fatalf("FromRole = %q, se esperaba web (el ultimo rol cerrado)", h.Metadata.FromRole)
	}

	// Section 2 must consolidate ALL THREE roles, not only the last closed
	// one (web) nor only the to_role (qa).
	for _, want := range []string{"core", "web", "qa", "tasks.core.md", "tasks.web.md", "tasks.qa.md"} {
		if !strings.Contains(h.Sections.Artifacts, want) {
			t.Errorf("Sections.Artifacts no menciona %q; se esperaba consolidar los tres roles:\n%s", want, h.Sections.Artifacts)
		}
	}
}

// TestIntegrationHandoffSingleRoleAutoHandoff is D-11's second resolution
// rule: a single-role roster (the fullstack default) auto-relays to
// itself — from_role and to_role are both that one role.
func TestIntegrationHandoffSingleRoleAutoHandoff(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	roster := multirole.Roster{Roles: []multirole.RoleAssignment{
		{Role: "fullstack", GatePolicy: multirole.PolicyBlocking},
	}}
	gates := GateLedger{Records: []GateRecord{approvedRoleApplyRecord("fullstack", base)}}

	h, err := IntegrationHandoff("inc-99-closure", roster, gates, ArtifactInventory{})
	if err != nil {
		t.Fatalf("IntegrationHandoff() error = %v", err)
	}
	if h.Metadata.FromRole != "fullstack" || h.Metadata.ToRole != "fullstack" {
		t.Fatalf("FromRole/ToRole = %q/%q, se esperaba fullstack/fullstack (auto-relevo)", h.Metadata.FromRole, h.Metadata.ToRole)
	}
}

// TestIntegrationHandoffFallsBackToLastClosedRole is D-11's third
// resolution rule: with no "qa" role and more than one role in the roster,
// to_role falls back to whichever role's gate was approved last.
func TestIntegrationHandoffFallsBackToLastClosedRole(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	roster := multirole.Roster{Roles: []multirole.RoleAssignment{
		{Role: "core", GatePolicy: multirole.PolicyBlocking},
		{Role: "web", GatePolicy: multirole.PolicyBlocking},
	}}
	gates := GateLedger{Records: []GateRecord{
		approvedRoleApplyRecord("core", base),
		approvedRoleApplyRecord("web", base.Add(time.Minute)),
	}}

	h, err := IntegrationHandoff("inc-99-closure", roster, gates, ArtifactInventory{})
	if err != nil {
		t.Fatalf("IntegrationHandoff() error = %v", err)
	}
	if h.Metadata.ToRole != "web" || h.Metadata.FromRole != "web" {
		t.Fatalf("FromRole/ToRole = %q/%q, se esperaba web/web (ultimo rol cerrado, sin qa en el roster)", h.Metadata.FromRole, h.Metadata.ToRole)
	}
}

// TestIntegrationHandoffNamesGlobalVerifyReportInDirectInstructions confirms
// Section 5 names the global verify report path IntegrationHandoff routes
// the "verify" phase toward (REQ-21.15).
func TestIntegrationHandoffNamesGlobalVerifyReportInDirectInstructions(t *testing.T) {
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	roster := multirole.Roster{Roles: []multirole.RoleAssignment{
		{Role: "fullstack", GatePolicy: multirole.PolicyBlocking},
	}}
	gates := GateLedger{Records: []GateRecord{approvedRoleApplyRecord("fullstack", base)}}
	artifacts := ArtifactInventory{GlobalVerifyReport: "verify-report.md"}

	h, err := IntegrationHandoff("inc-99-closure", roster, gates, artifacts)
	if err != nil {
		t.Fatalf("IntegrationHandoff() error = %v", err)
	}
	if !strings.Contains(h.Sections.DirectInstructions, "verify-report.md") {
		t.Errorf("Sections.DirectInstructions no menciona verify-report.md:\n%s", h.Sections.DirectInstructions)
	}
}

// TestIntegrationHandoffRejectsEmptyRoster confirms IntegrationHandoff
// refuses to build a handoff with no roles: from_role/to_role would be
// empty, which handoff.Validate already rejects, but this names the
// failure at the source instead of letting it surface later as a generic
// validation error.
func TestIntegrationHandoffRejectsEmptyRoster(t *testing.T) {
	_, err := IntegrationHandoff("inc-99-closure", multirole.Roster{}, GateLedger{}, ArtifactInventory{})
	if err == nil {
		t.Fatal("IntegrationHandoff() = nil error, se esperaba rechazo: roster vacio")
	}
}

// TestIntegrationHandoffRejectsRosterWithoutAnyApprovedRoleApplyGate
// confirms IntegrationHandoff cannot derive a "last closed role" from a
// ledger that never approved any of the roster's role-apply gates, and
// fails with a named error rather than silently emitting an empty
// FromRole/ToRole.
func TestIntegrationHandoffRejectsRosterWithoutAnyApprovedRoleApplyGate(t *testing.T) {
	roster := multirole.Roster{Roles: []multirole.RoleAssignment{
		{Role: "fullstack", GatePolicy: multirole.PolicyBlocking},
	}}
	_, err := IntegrationHandoff("inc-99-closure", roster, GateLedger{}, ArtifactInventory{})
	if err == nil {
		t.Fatal("IntegrationHandoff() = nil error, se esperaba rechazo: ningun role-apply aprobado en el ledger")
	}
}
