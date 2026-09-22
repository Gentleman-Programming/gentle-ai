package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pathquote"
)

// lastRoleNoticeMarker is the stable phrase RunSDDGate's last-role notice
// always contains. Phase 18 replaces the notice's full body (and adds the
// actual handoff.md write); this phase only routes to it and proves it
// fires exactly once under the right condition (REQ-21.13). Keeping this
// as a named constant, rather than duplicating a literal string at every
// call and assertion site, is what lets both this file and its test check
// "did the notice fire" without agreeing on prose by coincidence.
const lastRoleNoticeMarker = "ultimo rol activo"

// RunSDDGate is the CLI entry point for `axiom sdd gate <record|show>`
// (design.md S5.7). Like RunSDDKickoff, it is a thin I/O adapter: gate
// vocabulary, roster membership, and last-role detection all delegate to
// internal/kickoff.
func RunSDDGate(args []string, stdout io.Writer) error {
	if hasGovernanceHelpFlag(args) {
		return renderSDDGateHelp(stdout)
	}
	if len(args) == 0 {
		return fmt.Errorf("gate requiere un subcomando: record o show; ejecuta `axiom sdd gate --help`")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "record":
		return runSDDGateRecord(rest, stdout)
	case "show":
		return runSDDGateShow(rest, stdout)
	default:
		return fmt.Errorf("subcomando %q no reconocido para gate; opciones: record, show; ejecuta `axiom sdd gate --help`", sub)
	}
}

func renderSDDGateHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Uso: axiom sdd gate <record|show> [argumentos]")
	_, _ = fmt.Fprintln(stdout, "  record --cwd <ruta> --change <nombre> --gate spec|design|tasks|role-apply:<rol>|integration --decision approved|rejected [--reason \"<texto>\"] [--evidence-kind pr_merged|deployment|attestation] [--commit <sha>] [--base-ref <ref>] [--evidence \"<texto>\"] [--actor <id>] [--json]")
	_, _ = fmt.Fprintln(stdout, "  show --cwd <ruta> --change <nombre> [--json]")
	return nil
}

func runSDDGateRecord(args []string, stdout io.Writer) error {
	parsed, err := kickoff.ParseGateRecordArgs(args)
	if err != nil {
		return err
	}
	workspaceRoot, changeRoot, err := resolveGovernanceChangeRoot(parsed.CWD, parsed.Change)
	if err != nil {
		return err
	}

	sealed, err := kickoff.Load(changeRoot)
	if err != nil {
		return err
	}

	var roster []multirole.RoleAssignment
	if sealed != nil {
		roster = rosterFromKickoff(*sealed)
	}
	isRoleApply := isRoleApplyGate(parsed.Gate)
	if isRoleApply {
		if sealed == nil {
			return fmt.Errorf("no se puede registrar %q: el cambio %q no tiene un kickoff sellado con roster de roles; sella el kickoff primero, por ejemplo con `axiom sdd kickoff seal --cwd %s --change %s --execution-style checkpointed --handoff-policy none`",
				parsed.Gate, parsed.Change, pathquote.Quote(workspaceRoot), pathquote.Quote(parsed.Change))
		}
		if !gateKeyInRoster(parsed.Gate, roster) {
			// refusal:by-design operator-knowledge: only the operator knows which sealed role they meant to target; the message already lists the complete valid roster, and no runnable command can pick a role for them -- the roster is fixed at kickoff time and this verb never re-seals it
			return fmt.Errorf("%q no pertenece al roster sellado de %q (roster: %s)", parsed.Gate, parsed.Change, strings.Join(rosterRoleNames(roster), ", "))
		}
	}

	actor := parsed.Actor
	if actor == "" {
		actor = "cli"
	}
	digest, err := gateArtifactDigest(changeRoot, parsed.Gate, sealed)
	if err != nil {
		return err
	}
	record := kickoff.GateRecord{
		Gate:           parsed.Gate,
		Decision:       parsed.Decision,
		Reason:         parsed.Reason,
		ArtifactDigest: digest,
		Actor:          actor,
		RecordedAt:     time.Now().UTC(),
	}
	// Evidence verification runs, and can refuse, BEFORE kickoff.AppendGate:
	// an unconfirmed pr_merged ancestor must append nothing to gates.yaml
	// (task 21.1's second case), which is only guaranteed by checking first.
	if parsed.EvidenceKind != "" {
		evidence, evidenceErr := verifyGateEvidence(workspaceRoot, parsed)
		if evidenceErr != nil {
			return evidenceErr
		}
		record.EvidenceKind = evidence.Kind
		record.EvidenceRef = evidence.Ref
		record.EvidenceBaseRef = evidence.BaseRef
		record.Verified = evidence.Verified
	}
	if err := kickoff.AppendGate(changeRoot, record); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "Compuerta %q registrada (%s) para %q.\n", parsed.Gate, parsed.Decision, parsed.Change); err != nil {
		return err
	}

	if isRoleApply && parsed.Decision == kickoff.DecisionApproved && sealed != nil {
		closed, ledger, evalErr := lastRoleJustClosed(changeRoot, roster)
		if evalErr != nil {
			return evalErr
		}
		if closed {
			if err := writeIntegrationHandoff(changeRoot, parsed.Change, roster, ledger, sealed.Config.Roles); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(stdout, "Aviso: se ha concluido la implementacion del %s del cambio %q; se genera el relevo de integracion para la verificacion global de la solucion (REQ-21.13).\n", lastRoleNoticeMarker, parsed.Change); err != nil {
				return err
			}
		}
	}

	return nil
}

func runSDDGateShow(args []string, stdout io.Writer) error {
	parsed, err := kickoff.ParseShowArgs(args)
	if err != nil {
		return err
	}
	_, changeRoot, err := resolveGovernanceChangeRoot(parsed.CWD, parsed.Change)
	if err != nil {
		return err
	}

	ledger, err := kickoff.LoadGates(changeRoot)
	if err != nil {
		return err
	}
	if len(ledger.Records) == 0 {
		_, err := fmt.Fprintf(stdout, "El cambio %q no tiene ninguna decision de compuerta registrada todavia.\n", parsed.Change)
		return err
	}
	for _, rec := range ledger.Records {
		if _, err := fmt.Fprintf(stdout, "%s: %s (%s) — %s\n", rec.Gate, rec.Decision, rec.Actor, rec.Reason); err != nil {
			return err
		}
	}
	return nil
}

// isRoleApplyGate reports whether key is one of the four fixed gate keys
// (false) or a role-apply:<rol> key (true). It relies only on exported
// kickoff constants — not on a second "role-apply:" string literal — so
// this package never risks constructing or recognising that prefix
// differently than kickoff.RoleApplyGate does.
func isRoleApplyGate(key kickoff.GateKey) bool {
	switch key {
	case kickoff.GateSpec, kickoff.GateDesign, kickoff.GateTasks, kickoff.GateIntegration:
		return false
	default:
		return true
	}
}

func rosterFromKickoff(k kickoff.Kickoff) []multirole.RoleAssignment {
	roster := make([]multirole.RoleAssignment, 0, len(k.Config.Roles))
	for _, r := range k.Config.Roles {
		roster = append(roster, multirole.RoleAssignment{Role: r.Role, GatePolicy: r.GatePolicy})
	}
	return roster
}

func rosterRoleNames(roster []multirole.RoleAssignment) []string {
	names := make([]string, 0, len(roster))
	for _, r := range roster {
		names = append(names, r.Role)
	}
	return names
}

// gateKeyInRoster checks role-apply:<rol> membership by re-deriving each
// roster role's own canonical gate key via kickoff.RoleApplyGate and
// comparing, instead of splitting parsed.Gate's role segment back out by
// hand — the same drift kickoff.RoleApplyGate's own doc comment warns
// against, avoided here without needing a second exported helper in
// internal/kickoff.
func gateKeyInRoster(key kickoff.GateKey, roster []multirole.RoleAssignment) bool {
	for _, r := range roster {
		if kickoff.RoleApplyGate(r.Role) == key {
			return true
		}
	}
	return false
}

// lastRoleJustClosed reports whether every role-apply gate in roster is
// approved in the change's ledger, reusing kickoff.EvaluateGates and
// kickoff.LastRoleClosed (task 10.2) rather than re-deriving gate status by
// hand. It also returns the loaded ledger itself: task 18.2's caller needs
// it again to build the integration handoff (kickoff.IntegrationHandoff
// derives the last-closed role from this exact same ledger), and loading
// it twice would risk the two reads observing different states on a
// concurrently-modified gates.yaml.
//
// It marks every roster role as already "reached" (RolePending: 0)
// because, at this call site, apply-progress tracking is not the question
// being asked — that belongs to sddstatus (wired in Phase 13), which this
// verb does not read. The question EvaluateGates answers here is narrower
// and fully answerable from the ledger alone: given the sealed roster, has
// every role's role-apply:<role> gate ended up approved? A role whose
// last-recorded decision is a rejection is still reported "rejected" by
// resolveGateStatus regardless of this stand-in RolePending map, so an
// unresolved role still correctly keeps LastRoleClosed false.
func lastRoleJustClosed(changeRoot string, roster []multirole.RoleAssignment) (bool, kickoff.GateLedger, error) {
	if len(roster) == 0 {
		return false, kickoff.GateLedger{}, nil
	}
	ledger, err := kickoff.LoadGates(changeRoot)
	if err != nil {
		return false, kickoff.GateLedger{}, err
	}
	rolePending := make(map[string]int, len(roster))
	for _, r := range roster {
		rolePending[r.Role] = 0
	}
	states, err := kickoff.EvaluateGates(kickoff.Inputs{
		Execution:   kickoff.ExecutionCheckpointed,
		Roles:       roster,
		RolePending: rolePending,
		Ledger:      ledger,
	})
	if err != nil {
		return false, kickoff.GateLedger{}, err
	}
	return kickoff.LastRoleClosed(roster, states), ledger, nil
}

// writeIntegrationHandoff builds the integration handoff via
// kickoff.IntegrationHandoff (Phase 17) and writes it to
// openspec/changes/<change>/handoff.md, invoked exactly once, right after
// the last active role's role-apply gate is approved (REQ-21.13,
// REQ-21.14). It reuses handoff.WriteFile, the same writer
// cmd/axiom/main.go's runHandoffCreate already uses, so this is not a
// second handoff-writing code path.
func writeIntegrationHandoff(changeRoot, changeName string, roster []multirole.RoleAssignment, ledger kickoff.GateLedger, sealedRoles []kickoff.KickoffRole) error {
	h, err := kickoff.IntegrationHandoff(
		changeName,
		multirole.Roster{Roles: roster, Source: multirole.RosterSourceKickoff},
		ledger,
		buildIntegrationArtifactInventory(changeRoot, sealedRoles),
	)
	if err != nil {
		return fmt.Errorf("construir el relevo de integracion de %q: %w", changeName, err)
	}
	if err := handoff.WriteFile(filepath.Join(changeRoot, "handoff.md"), h); err != nil {
		return fmt.Errorf("escribir el relevo de integracion de %q: %w", changeName, err)
	}
	return nil
}

// buildIntegrationArtifactInventory resolves, for each sealed role,
// whether its tasks/verify files actually exist under changeRoot.
// kickoff.IntegrationHandoff is pure and never touches the filesystem
// itself (closure.go's own documented contract) — this adapter is the one
// place that does, exactly like governanceArtifactInputs already does the
// equivalent resolution for internal/sddstatus's own gate digests.
func buildIntegrationArtifactInventory(changeRoot string, roles []kickoff.KickoffRole) kickoff.ArtifactInventory {
	inventory := kickoff.ArtifactInventory{
		RoleTasksFiles:     make(map[string]string, len(roles)),
		RoleVerifyFiles:    make(map[string]string, len(roles)),
		GlobalVerifyReport: "verify-report.md",
	}
	for _, role := range roles {
		if fileHasContent(filepath.Join(changeRoot, role.TasksFile)) {
			inventory.RoleTasksFiles[role.Role] = role.TasksFile
		}
		if fileHasContent(filepath.Join(changeRoot, role.VerifyFile)) {
			inventory.RoleVerifyFiles[role.Role] = role.VerifyFile
		}
	}
	return inventory
}

// fileHasContent reports whether path exists, is a regular file, and is
// non-empty — the same "has this artifact actually been produced" test
// internal/sddstatus's own hasContent applies, duplicated narrowly here
// because internal/cli has no dependency on internal/sddstatus's
// unexported helpers.
func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
