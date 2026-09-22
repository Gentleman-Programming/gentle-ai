package sddstatus

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/pathquote"
)

// SDDGovernanceGateSchema identifies INC-21's block-review gate question.
// It is a schema OF ITS OWN, distinct from SDDIntegrationConsentSchema: a
// gate approval is a quality decision about a planning artifact or a role's
// implementation, never edit-authority escalation and never the RDD outcome
// document reviewed code carries (design.md S1.3, rule 4 — the schema must
// never be confused with gentle-ai.sdd-integration.consent/v1, or a relay
// would route a quality decision toward sdd-attempt grant).
const SDDGovernanceGateSchema = "gentle-ai.sdd-governance.gate/v1"

// SDDGovernanceContractV1 names the sdd-governance contract the envelope
// belongs to.
const SDDGovernanceContractV1 = "gentle-ai.sdd-governance/v1"

const (
	// gateOperation and gateActionRequired are this envelope's identity
	// fields, mirroring SDDIntegrationConsentResult's own
	// sddConsentOperation/sddConsentActionRequired pair.
	gateOperation      = "sdd-gate.record"
	gateActionRequired = "gate_decision_required"

	// gateAnswerApproved and gateAnswerRejected are the machine answer
	// tokens: the SDD gate vocabulary (design.md S1.3), never the RDD
	// "granted"/"declined" pair.
	gateAnswerApproved = "approved"
	gateAnswerRejected = "rejected"

	// gateRecordInvocationPrefix is the ONLY prefix a gate choice's
	// invocation may start with. Both choices share it (unlike the
	// edit-authority envelope's grant/decline pair, which name two
	// different verbs): approving and rejecting the same gate are both
	// `axiom sdd gate record ...`, differing only in --decision. T-9 names
	// the adversarial case this guards: an invocation beginning with
	// "sdd-attempt" must never validate here, or a relay would treat a
	// quality decision as edit-authority escalation.
	gateRecordInvocationPrefix = "axiom sdd gate record "

	// gateStatusInvocationPrefix is the off-path re-entry: declaring no
	// decision yet always re-enters through native SDD status, never
	// through a review or edit-authority command.
	gateStatusInvocationPrefix = "axiom sdd status "

	// gateShowInvocationPrefix names the read-only inspection verb for one
	// gate's current state and history, distinct from the decision-recording
	// gateRecordInvocationPrefix. Both prefixes share the single "axiom sdd
	// gate" root so a caller building either invocation never invents a
	// third spelling of the same verb family.
	gateShowInvocationPrefix = "axiom sdd gate show "
)

// SDDGovernanceGateResult is the typed blocking question one open block
// review gate raises (design.md S5.5). Like SDDIntegrationConsentResult it
// is a Lossless Blocking Prompt: WHY a decision is needed, the COMPLETE
// choice set, and the EXACT runnable way to answer, scoped to one gate of
// one change. It never authorizes delivery (design.md S1.3): the only way
// to approve a gate is `axiom sdd gate record --decision approved`.
type SDDGovernanceGateResult struct {
	Schema    string `json:"schema"`
	Contract  string `json:"contract"`
	Operation string `json:"operation"`
	Action    string `json:"action"`
	// Blocking marks this envelope as a decision the caller must relay
	// before the gated phase or role-apply can proceed; nothing has been
	// recorded yet.
	Blocking bool `json:"blocking"`
	// Change and Gate are the identity block: which change is blocked and
	// which gate key is awaiting a decision.
	Change string `json:"change"`
	Gate   string `json:"gate"`
	// Criteria are design.md S5.5's objective lenses for this gate key, the
	// propuesta's REQ-3 "review lenses" made concrete per gate.
	Criteria []string `json:"criteria"`
	Headline string   `json:"headline"`
	Reason   string   `json:"reason"`
	Value    string   `json:"value"`
	Evidence []string `json:"evidence"`
	// Choices carries exactly the approved and rejected tokens, in that
	// order; both invocations share the gateRecordInvocationPrefix verb.
	Choices []consentenvelope.Choice `json:"choices"`
	// OffPath documents the deliberate alternative outside the choice set:
	// inspect the change without deciding yet, then re-enter through
	// native status.
	OffPath consentenvelope.OffPath `json:"off_path"`
}

// Validate enforces the envelope's SDD governance identity and delegates
// the generic completeness half to the shared consent-envelope core
// (internal/consentenvelope), exactly as SDDIntegrationConsentResult does
// for its own sibling schema.
func (r SDDGovernanceGateResult) Validate() error {
	if r.Schema != SDDGovernanceGateSchema || r.Contract != SDDGovernanceContractV1 ||
		r.Operation != gateOperation || r.Action != gateActionRequired || !r.Blocking {
		return errors.New("invalid SDD governance gate question identity") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if strings.TrimSpace(r.Change) == "" {
		return errors.New("SDD governance gate question requires the blocked change name") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if strings.TrimSpace(r.Gate) == "" {
		return errors.New("SDD governance gate question requires the gate key") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if len(r.Criteria) == 0 {
		return errors.New("SDD governance gate question requires at least one review criterion") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}

	core := consentenvelope.Core{
		Headline: r.Headline, Reason: r.Reason, Value: r.Value,
		Evidence: r.Evidence, Choices: r.Choices, OffPath: r.OffPath,
	}
	if err := core.ValidateCompleteness(gateAnswerApproved, gateAnswerRejected); err != nil {
		return err
	}

	for _, choice := range r.Choices {
		if !strings.HasPrefix(choice.Invocation, gateRecordInvocationPrefix) {
			return fmt.Errorf("SDD governance gate choice %q does not start with the gate record invocation: %q", choice.Answer, choice.Invocation) // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
		}
	}
	if !strings.HasPrefix(r.OffPath.Command, gateStatusInvocationPrefix) {
		return errors.New("SDD governance gate off path must re-enter through native status") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	return nil
}

// gateCriteriaFixed holds design.md S5.5's review lenses for the four fixed
// gate keys. role-apply:<role> is handled separately by gateCriteria below,
// since its key varies by role and can never be a map literal.
var gateCriteriaFixed = map[kickoff.GateKey][]string{
	kickoff.GateSpec: {
		"¿Cubre la intención original?",
		"¿Cubre casos borde y escenarios funcionales?",
		"¿Qué huecos o dudas abiertas quedan?",
	},
	kickoff.GateDesign: {
		"¿Satisface todos los requerimientos de la spec?",
		"¿Respeta las tecnologías de axiom.yaml y los patrones del repositorio?",
		"¿Qué desviación arquitectónica introduce?",
	},
	kickoff.GateTasks: {
		"¿Es coherente el reparto entre roles?",
		"¿Son las tareas suficientemente atómicas y verificables?",
	},
	kickoff.GateIntegration: {
		"¿Hay evidencia de integración o despliegue?",
		"¿De qué clase y verificable?",
	},
}

// roleApplyGateCriteria are design.md S5.5's lenses for every role-apply:<role>
// gate: the same three questions regardless of which role closed.
var roleApplyGateCriteria = []string{
	"¿Conforme al diseño de su rol?",
	"¿Cumple la spec asignada?",
	"¿Compila limpio, pasan sus pruebas, cobertura y estilo?",
}

// gateCriteria resolves design.md S5.5's review lenses for one evaluated
// gate. It recognises a role-apply gate by reconstructing its key from
// state.Blocks (kickoff.RoleApplyGate(state.Blocks) — machine.go sets
// Blocks to the bare role name for exactly this shape), the same technique
// Phase 10's gateKeyInRoster already uses, rather than re-deriving the
// "role-apply:" literal a second time in this package.
func gateCriteria(state kickoff.GateState) []string {
	if criteria, ok := gateCriteriaFixed[state.Key]; ok {
		return criteria
	}
	if state.Key == kickoff.RoleApplyGate(state.Blocks) {
		return roleApplyGateCriteria
	}
	return nil
}

// gateEvidence names the artifact locations this gate's decision should
// weigh (design.md S5.5: "rutas del artefacto"). It recognises the gate key
// exactly the way gateCriteria does, so the two functions can never disagree
// about which gate a given kickoff.GateState represents. Evidence stops at
// LOCATIONS on purpose: detecting concrete gaps inside an artifact's
// content is the human reviewer's own judgment call, never something this
// envelope fabricates on their behalf. The returned slice is never nil
// (consentenvelope.Core.ValidateCompleteness treats nil as a construction
// bug), only possibly empty.
func gateEvidence(state kickoff.GateState, artifactPaths ArtifactPaths) []string {
	switch state.Key {
	case kickoff.GateSpec:
		return append([]string{}, artifactPaths.Specs...)
	case kickoff.GateDesign:
		return append([]string{}, artifactPaths.Design...)
	case kickoff.GateTasks:
		return append([]string{}, artifactPaths.Tasks...)
	}
	if state.Key == kickoff.RoleApplyGate(state.Blocks) {
		return append([]string{}, artifactPaths.Tasks...)
	}
	return []string{}
}

// newGovernanceGateQuestion builds the typed blocking question for one open
// block review gate (design.md S5.5): design.md S5.5's review lenses for
// the gate key become Criteria via the existing gateCriteria, both choices
// share gateRecordInvocationPrefix and differ only by --decision, and the
// off path re-enters through native status without deciding
// (gateStatusInvocationPrefix) -- the same shape newEditAuthorityConsent
// already established for its own sibling schema. Unlike that sibling,
// Criteria depends on gateCriteria recognising the gate key, so this
// constructor validates its own output and returns an error rather than
// ever handing a malformed envelope to a caller: there is no human input
// left to sanity-check once Validate() has run.
func newGovernanceGateQuestion(change, workspaceRoot string, gate kickoff.GateState, artifactPaths ArtifactPaths) (SDDGovernanceGateResult, error) {
	criteria := gateCriteria(gate)
	if len(criteria) == 0 {
		return SDDGovernanceGateResult{}, fmt.Errorf("no design.md S5.5 review criteria recognised for gate %q", gate.Key) // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}

	gateKey := string(gate.Key)
	cwd := pathquote.Quote(workspaceRoot)
	showInvocation := fmt.Sprintf("%s--cwd %s --change %s --gate %s", gateShowInvocationPrefix, cwd, change, gateKey)
	statusInvocation := fmt.Sprintf("%s--cwd %s --change %s", gateStatusInvocationPrefix, cwd, change)

	result := SDDGovernanceGateResult{
		Schema:    SDDGovernanceGateSchema,
		Contract:  SDDGovernanceContractV1,
		Operation: gateOperation,
		Action:    gateActionRequired,
		Blocking:  true,
		Change:    change,
		Gate:      gateKey,
		Criteria:  criteria,
		Headline:  fmt.Sprintf("Block review gate %q is awaiting your decision.", gateKey),
		Reason:    fmt.Sprintf("change %q has not approved or rejected this gate yet, and design.md's block-review policy blocks every later phase or role until it is (design.md S5.5).", change),
		Value:     "Approving unblocks the next phase or role; rejecting keeps this gate open to remediation. Either decision, and its reason, is recorded in the change's gate ledger for later audit.",
		Evidence:  gateEvidence(gate, artifactPaths),
		Choices: []consentenvelope.Choice{
			{
				Answer:     gateAnswerApproved,
				Label:      "Approve this gate",
				Effect:     fmt.Sprintf("Records an approved decision for gate %q and unblocks the next phase or role.", gateKey),
				Invocation: fmt.Sprintf("%s--cwd %s --change %s --gate %s --decision approved --reason <your-reason>", gateRecordInvocationPrefix, cwd, change, gateKey),
			},
			{
				Answer:     gateAnswerRejected,
				Label:      "Reject this gate",
				Effect:     fmt.Sprintf("Keeps gate %q open to remediation; nothing later starts until it is re-approved.", gateKey),
				Invocation: fmt.Sprintf("%s--cwd %s --change %s --gate %s --decision rejected --reason <your-reason>", gateRecordInvocationPrefix, cwd, change, gateKey),
			},
		},
		OffPath: consentenvelope.OffPath{
			Note:    fmt.Sprintf("To inspect this gate without deciding yet, run '%s', then re-enter through '%s'.", showInvocation, statusInvocation),
			Command: statusInvocation,
		},
	}
	if err := result.Validate(); err != nil {
		return SDDGovernanceGateResult{}, fmt.Errorf("build governance gate question for %q: %w", gateKey, err)
	}
	return result, nil
}

// Governance is the per-status view of one change's sealed kickoff
// configuration and block-review gate ledger (design.md S4.3). Status only
// ever carries a non-nil Governance when the change has a sealed
// kickoff.yaml AND its execution style is not continuous (D-05): the
// resolver never even calls loadGovernance otherwise, so the unsealed and
// continuous cases cost exactly nothing extra.
type Governance struct {
	Kickoff kickoff.Kickoff
	Gates   []kickoff.GateState
	Roster  GovernanceRoster
}

// GovernanceRoster is this slice's minimal roster view: which roles the
// sealed kickoff names, in the order it names them. Source is always
// "kickoff" here: D-06's multirole.ResolveRoster stratification (kickoff |
// design | fallback) is Phase 16's job, and a Governance value never exists
// without a sealed kickoff to source its roster from.
type GovernanceRoster struct {
	Source string
	Roles  []string
}

// governanceRosterSourceKickoff is the only GovernanceRoster.Source value
// this slice ever produces.
const governanceRosterSourceKickoff = "kickoff"

// firstOpenGate returns the first gate in governance.Gates whose decision
// is not yet "approved" -- pending or genuinely rejected -- in the fixed
// evaluation order kickoff.EvaluateGates already returns them in (machine.go:
// spec -> design -> tasks -> role-apply:<role...> -> integration). Every
// governance-aware routing and reporting function in this package
// (resolveNextRecommended, artifactBlockedReasons,
// nonPhaseRoutingInstructions) shares this single lookup, so none of them
// can ever name a different gate than the others are reporting on.
func firstOpenGate(governance *Governance) (kickoff.GateState, bool) {
	for _, gate := range governance.Gates {
		if gate.Status != "approved" {
			return gate, true
		}
	}
	return kickoff.GateState{}, false
}

// loadGovernance reads the sealed kickoff and its gate ledger for
// changeRoot and evaluates the current state of every governed gate. It
// returns (nil, nil) when the change has no sealed kickoff.yaml: exactly
// kickoff.Load's own "not sealed yet" contract, forwarded unchanged. A
// corrupt kickoff.yaml or gates.yaml is a named error, never degraded to
// "no governance" (T-10).
//
// D-05's continuous-mode zero cost is the CALLER's responsibility (never
// call this function unless execution_style != continuous), not this
// function's: kickoff.EvaluateGates already returns an empty gate slice for
// continuous mode on its own, so calling this function in that mode is
// merely redundant I/O, never incorrect.
func loadGovernance(changeRoot string) (*Governance, error) {
	sealed, err := kickoff.Load(changeRoot)
	if err != nil {
		return nil, err
	}
	if sealed == nil {
		return nil, nil
	}
	// D-05: a continuous-execution seal reports NO governance, not a
	// Governance value with an empty Gates slice. EvaluateGates already
	// short-circuits to an empty slice for continuous mode on its own, but
	// this function goes one step further and reports structural absence,
	// matching what Status.Governance documents for every caller.
	if sealed.Config.ExecutionStyle == kickoff.ExecutionContinuous {
		return nil, nil
	}

	ledger, err := kickoff.LoadGates(changeRoot)
	if err != nil {
		return nil, err
	}

	roles := make([]multirole.RoleAssignment, 0, len(sealed.Config.Roles))
	roleNames := make([]string, 0, len(sealed.Config.Roles))
	for _, role := range sealed.Config.Roles {
		roles = append(roles, multirole.RoleAssignment{Role: role.Role, GatePolicy: role.GatePolicy})
		roleNames = append(roleNames, role.Role)
	}

	artifacts, rolePending, err := governanceArtifactInputs(changeRoot, sealed.Config.Roles)
	if err != nil {
		return nil, err
	}

	gates, err := kickoff.EvaluateGates(kickoff.Inputs{
		Execution:   sealed.Config.ExecutionStyle,
		Roles:       roles,
		Artifacts:   artifacts,
		RolePending: rolePending,
		VerifyFound: hasContent(filepath.Join(changeRoot, "verify-report.md")),
		Ledger:      ledger,
	})
	if err != nil {
		return nil, err
	}

	return &Governance{
		Kickoff: *sealed,
		Gates:   gates,
		Roster:  GovernanceRoster{Source: governanceRosterSourceKickoff, Roles: roleNames},
	}, nil
}

// governanceArtifactInputs gathers the digest and pending-task inputs
// kickoff.EvaluateGates needs: the shared "spec"/"design"/"tasks" digests
// (each set only once resolveArtifactPaths reports that artifact as
// present and non-empty -- an artifact that has not reached "done" yet
// stays absent from Artifacts, per Inputs' own contract), each role's own
// "tasks.<role>" digest, and each role's current pending-task count from
// its own tasks file.
func governanceArtifactInputs(changeRoot string, roles []kickoff.KickoffRole) (map[string]string, map[string]int, error) {
	artifactPaths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		return nil, nil, err
	}

	artifacts := make(map[string]string, 3+len(roles))
	for _, entry := range []struct {
		key   string
		paths []string
	}{
		{key: "spec", paths: artifactPaths.Specs},
		{key: "design", paths: artifactPaths.Design},
		{key: "tasks", paths: artifactPaths.Tasks},
	} {
		if err := setGovernanceArtifactDigest(entry.key, entry.paths, artifacts); err != nil {
			return nil, nil, err
		}
	}

	rolePending := make(map[string]int, len(roles))
	for _, role := range roles {
		roleTasksPath := filepath.Join(changeRoot, role.TasksFile)
		if hasContent(roleTasksPath) {
			roleKey := "tasks." + strings.ToLower(role.Role)
			if err := setGovernanceArtifactDigest(roleKey, []string{roleTasksPath}, artifacts); err != nil {
				return nil, nil, err
			}
		}
		progress, err := countTaskProgress(roleTasksPath)
		if err != nil {
			if os.IsNotExist(err) {
				// RolePending intentionally leaves this role untracked: a
				// role whose tasks file was never found is "unknown", never
				// "0 pending" (machine.go's own documented contract for
				// roleApplyGateOpen).
				continue
			}
			return nil, nil, err
		}
		rolePending[role.Role] = progress.Pending
	}
	return artifacts, rolePending, nil
}

// setGovernanceArtifactDigest sets artifacts[key] to paths' stable digest,
// but only when every one of paths already exists and is non-empty; an
// artifact that has not reached "done" yet is left absent from the map
// entirely, matching kickoff.Inputs.Artifacts' documented meaning.
func setGovernanceArtifactDigest(key string, paths []string, artifacts map[string]string) error {
	if len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		if !hasContent(path) {
			return nil
		}
	}
	digest, err := kickoff.ArtifactDigest(paths)
	if err != nil {
		return err
	}
	artifacts[key] = digest
	return nil
}

// verifyDependencyFromHandoff is D-12's rule: REQ-21.15 forbids starting
// the global "verify" phase before the integration handoff says it is
// ready. It only ever CONSULTS openspec/changes/<change>/handoff.md when
// the sealed kickoff's roster is multi-role or its handoff_policy is
// per_checkpoint; every other sealed change (a single role with the
// fullstack default's handoff_policy: none) keeps baseline exactly as
// resolveDependencies already computed it — this is a deliberate,
// documented cost boundary (design.md S4.5/D-12), not an oversight: a
// single-role, no-intermediate-handoff change never produces a handoff.md
// to read until the very last (and only) role closes, at which point
// coreReady/applyState already govern Verify through the existing logic.
//
// It returns the adjusted DependencyState and a non-empty genuine reason
// exactly when it blocks Verify for a cause the caller must surface in
// BlockedReasons; an unchanged baseline always pairs with an empty reason.
func verifyDependencyFromHandoff(governance *Governance, changeRoot string, baseline DependencyState) (DependencyState, string) {
	if governance == nil {
		return baseline, ""
	}
	multiRole := len(governance.Roster.Roles) > 1
	perCheckpoint := governance.Kickoff.Config.HandoffPolicy == kickoff.HandoffPerCheckpoint
	if !multiRole && !perCheckpoint {
		return baseline, ""
	}

	h, err := handoff.ParseFile(filepath.Join(changeRoot, "handoff.md"))
	if err != nil {
		// Absent or unreadable handoff.md is "no relay produced yet", not a
		// block -- UNLESS every role already closed (LastRoleClosed): the
		// relay SHOULD exist by then, and reporting Verify "ready" anyway
		// would let it start without the consolidated instructions REQ-21.14
		// promises it.
		if lastRoleClosedForGovernance(governance) {
			return DependencyBlocked, "El relevo de integracion openspec/changes/.../handoff.md no existe o no se pudo leer, pese a que todos los roles del roster ya estan cerrados; genera el relevo antes de iniciar la fase verify global (REQ-21.14, REQ-21.15)."
		}
		return baseline, ""
	}

	switch h.Metadata.Status {
	case handoff.StatusReady:
		if h.Metadata.ToPhase == handoff.PhaseVerify {
			return DependencyReady, ""
		}
		return baseline, ""
	case handoff.StatusBlocked, handoff.StatusNeedsClarification:
		return DependencyBlocked, fmt.Sprintf(
			"El relevo de integracion tiene status %q; la fase verify global no puede iniciarse hasta resolver ese bloqueo (REQ-21.15).",
			h.Metadata.Status,
		)
	default:
		return baseline, ""
	}
}

// archiveDependencyFromGovernance is D-13's rule: REQ-21.16 forbids
// archiving a change before its "integration" gate is approved, no matter
// how favorable verify-report.md already is. It only ever NARROWS
// baseline, from ready to blocked, and only when governance is non-nil (a
// sealed, non-continuous kickoff): an unsealed or continuous-mode change
// keeps resolveDependencies' existing dependencies.Archive computation
// exactly as it was before this increment (D-13's own documented zero-cost
// boundary). It never upgrades an already-blocked baseline to ready: a
// change whose core artifacts or apply are not done is not archive-ready
// regardless of any integration evidence, so governance never needs to add
// a second, redundant reason for the same outcome.
func archiveDependencyFromGovernance(governance *Governance, changeName string, baseline DependencyState) (DependencyState, string) {
	if governance == nil || baseline != DependencyReady {
		return baseline, ""
	}
	for _, gate := range governance.Gates {
		if gate.Key == kickoff.GateIntegration && gate.Status == "approved" {
			return DependencyReady, ""
		}
	}
	return DependencyBlocked, fmt.Sprintf(
		"El cambio %q no puede archivarse todavia: la compuerta \"integration\" no esta aprobada; ejecuta `axiom sdd gate record --gate integration --decision approved --evidence-kind pr_merged --commit <sha>` (o --evidence-kind deployment|attestation) para registrar la evidencia de integracion o despliegue (REQ-21.16).",
		changeName,
	)
}

// lastRoleClosedForGovernance reuses kickoff.LastRoleClosed against the
// governance snapshot's own roster and already-evaluated gate states,
// rather than re-deriving "every role-apply gate approved" a second way.
func lastRoleClosedForGovernance(governance *Governance) bool {
	roster := make([]multirole.RoleAssignment, 0, len(governance.Roster.Roles))
	for _, role := range governance.Roster.Roles {
		roster = append(roster, multirole.RoleAssignment{Role: role})
	}
	return kickoff.LastRoleClosed(roster, governance.Gates)
}
