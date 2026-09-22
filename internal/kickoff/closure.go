package kickoff

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
)

// ArtifactInventory carries the artifact file paths IntegrationHandoff
// consolidates into the integration handoff document. IntegrationHandoff
// is pure with respect to the filesystem: it trusts these paths exactly as
// given and never calls os.Stat or os.ReadFile itself — the caller
// (internal/cli/sdd_gate.go, Phase 18) resolves which files actually exist
// before building this value, the same separation machine.go's own purity
// already established for the gate state machine (D-08).
type ArtifactInventory struct {
	// RoleTasksFiles and RoleVerifyFiles map each roster role's identifier
	// to the path of its tasks/verify-report file, but ONLY when the
	// caller confirmed that file exists. A role absent from either map is
	// reported in Section 2 as "not yet produced", never silently omitted.
	RoleTasksFiles  map[string]string
	RoleVerifyFiles map[string]string
	// GlobalVerifyReport is the path the consolidated "verify" phase
	// (REQ-21.15) is expected to write. It is named in Section 5
	// regardless of whether it exists yet: this handoff routes TO that
	// phase, which produces it next.
	GlobalVerifyReport string
}

// IntegrationHandoff builds the integration handoff document REQ-21.14
// requires when the last active role's apply gate is approved: a
// handoff.Handoff with FromPhase apply, ToPhase verify, and status ready,
// consolidating the artifacts and gate decisions of every role in roster —
// not only whichever role happened to close last (D-11).
//
// It reuses handoff.Handoff/handoff.WriteFile as the canonical relay
// schema and never invents a second format: this function only assembles
// the value, callers validate it (handoff.Validate) and write it
// (handoff.WriteFile) exactly like any other relay in this codebase.
func IntegrationHandoff(changeName string, roster multirole.Roster, gates GateLedger, artifacts ArtifactInventory) (*handoff.Handoff, error) {
	if len(roster.Roles) == 0 {
		return nil, fmt.Errorf("no se puede construir el relevo de integracion de %q: el roster esta vacio", changeName)
	}

	lastClosedRole, err := lastClosedRoleFromLedger(roster, gates)
	if err != nil {
		return nil, fmt.Errorf("construir el relevo de integracion de %q: %w", changeName, err)
	}
	toRole := resolveIntegrationToRole(roster, lastClosedRole)

	roleNames := sortedRosterRoleNames(roster)
	sections := handoff.Sections{
		ExecutiveSummary:   integrationExecutiveSummary(changeName, roleNames),
		Artifacts:          integrationArtifactsSection(roleNames, artifacts),
		Decisions:          integrationDecisionsSection(roster, roleNames),
		RisksAndBlockers:   integrationRisksSection(roster),
		DirectInstructions: integrationDirectInstructionsSection(artifacts),
	}

	return &handoff.Handoff{
		Metadata: handoff.Metadata{
			Change:    changeName,
			FromPhase: handoff.PhaseApply,
			ToPhase:   handoff.PhaseVerify,
			FromRole:  lastClosedRole,
			ToRole:    toRole,
			Timestamp: time.Now().UTC(),
			Status:    handoff.StatusReady,
		},
		Sections: sections,
	}, nil
}

// lastClosedRoleFromLedger finds the roster role whose role-apply gate was
// approved most recently in gates' append-only history. Since GateLedger
// never removes or reorders a record (REQ-21.12), the LAST matching
// approval in Records IS the most recent one — no timestamp comparison is
// needed, only append order.
func lastClosedRoleFromLedger(roster multirole.Roster, gates GateLedger) (string, error) {
	rosterGateKeys := make(map[GateKey]string, len(roster.Roles))
	for _, role := range roster.Roles {
		rosterGateKeys[RoleApplyGate(role.Role)] = role.Role
	}

	lastClosed := ""
	for _, record := range gates.Records {
		if record.Decision != DecisionApproved {
			continue
		}
		if role, ok := rosterGateKeys[record.Gate]; ok {
			lastClosed = role
		}
	}
	if lastClosed == "" {
		return "", fmt.Errorf("el ledger de compuertas no registra ninguna aprobacion role-apply para el roster sellado")
	}
	return lastClosed, nil
}

// resolveIntegrationToRole applies D-11's exact three-tier precedence:
//  1. a roster role literally named "qa" (case-insensitive), if it exists;
//  2. a single-role roster's own role (fullstack's auto-handoff);
//  3. otherwise, the role whose gate closed last.
func resolveIntegrationToRole(roster multirole.Roster, lastClosedRole string) string {
	for _, role := range roster.Roles {
		if strings.EqualFold(role.Role, "qa") {
			return role.Role
		}
	}
	if len(roster.Roles) == 1 {
		return roster.Roles[0].Role
	}
	return lastClosedRole
}

func sortedRosterRoleNames(roster multirole.Roster) []string {
	names := make([]string, 0, len(roster.Roles))
	for _, r := range roster.Roles {
		names = append(names, r.Role)
	}
	sort.Strings(names)
	return names
}

func integrationExecutiveSummary(changeName string, roleNames []string) string {
	return fmt.Sprintf(
		"Se ha concluido la implementacion de todos los roles activos del cambio %q. Este relevo consolida el trabajo de %d rol(es) participante(s) (%s) y abre la fase `verify` global de la solucion (REQ-21.13, REQ-21.14).",
		changeName, len(roleNames), strings.Join(roleNames, ", "),
	)
}

func integrationArtifactsSection(roleNames []string, artifacts ArtifactInventory) string {
	var lines []string
	for _, role := range roleNames {
		tasksFile := "(no producido todavia)"
		if path, ok := artifacts.RoleTasksFiles[role]; ok && path != "" {
			tasksFile = path
		}
		verifyFile := "(no producido todavia)"
		if path, ok := artifacts.RoleVerifyFiles[role]; ok && path != "" {
			verifyFile = path
		}
		lines = append(lines, fmt.Sprintf("- Rol %q: tareas en %s, verificacion en %s", role, tasksFile, verifyFile))
	}
	return strings.Join(lines, "\n")
}

func integrationDecisionsSection(roster multirole.Roster, roleNames []string) string {
	summary := fmt.Sprintf("Todas las compuertas `role-apply` del roster (%s) estan aprobadas en el ledger de compuertas (D-10).", strings.Join(roleNames, ", "))
	if roster.Source != "" {
		summary += fmt.Sprintf(" Origen del roster: %s.", roster.Source)
	}
	return summary
}

func integrationRisksSection(roster multirole.Roster) string {
	if roster.Conflict != nil {
		return fmt.Sprintf("Discrepancia no bloqueante entre el roster sellado y design.md: %s", roster.Conflict.Detail)
	}
	return "Ninguno detectado por este relevo."
}

func integrationDirectInstructionsSection(artifacts ArtifactInventory) string {
	globalReport := artifacts.GlobalVerifyReport
	if globalReport == "" {
		globalReport = "verify-report.md"
	}
	return fmt.Sprintf(
		"Ejecuta la fase `verify` global de la solucion, considerando el trabajo consolidado de todos los roles, y emite el informe consolidado en `%s`, distinto de los informes `verify-report.<rol>.md` de cada rol (REQ-21.15).",
		globalReport,
	)
}
