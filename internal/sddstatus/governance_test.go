package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

// validGovernanceGateResult returns a well-formed envelope for the "spec"
// gate. Every rejection test below mutates one field of this base value, so
// each test proves exactly one rule instead of accidentally exercising two
// at once.
func validGovernanceGateResult() SDDGovernanceGateResult {
	return SDDGovernanceGateResult{
		Schema:    SDDGovernanceGateSchema,
		Contract:  SDDGovernanceContractV1,
		Operation: gateOperation,
		Action:    gateActionRequired,
		Blocking:  true,
		Change:    "inc-99-governance-example",
		Gate:      string(kickoff.GateSpec),
		Criteria:  gateCriteria(kickoff.GateState{Key: kickoff.GateSpec}),
		Headline:  "La compuerta de spec necesita tu decision",
		Reason:    "La fase spec concluyo y espera aprobacion antes de iniciar design",
		Value:     "Evita avanzar a design con un spec incompleto o con huecos sin señalar",
		Evidence:  []string{"openspec/changes/inc-99-governance-example/specs/example/spec.md"},
		Choices: []consentenvelope.Choice{
			{
				Answer: gateAnswerApproved, Label: "Aprobar",
				Effect:     "Habilita el inicio de design",
				Invocation: "axiom sdd gate record --cwd . --change inc-99-governance-example --gate spec --decision approved --reason cubre-la-intencion",
			},
			{
				Answer: gateAnswerRejected, Label: "Rechazar",
				Effect:     "Mantiene spec abierto a remediacion",
				Invocation: "axiom sdd gate record --cwd . --change inc-99-governance-example --gate spec --decision rejected --reason falta-caso-borde",
			},
		},
		OffPath: consentenvelope.OffPath{
			Note:    "Puedes revisar el estado sin decidir todavia",
			Command: "axiom sdd status --cwd . --change inc-99-governance-example",
		},
	}
}

func TestSDDGovernanceGateResultValidateAcceptsWellFormedEnvelope(t *testing.T) {
	if err := validGovernanceGateResult().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, se esperaba aceptacion de un envoltorio bien formado", err)
	}
}

// TestSDDGovernanceGateResultValidateRejectsUnknownSchema is task 12.1's
// first required case: Validate() must reject any schema other than
// gentle-ai.sdd-governance.gate/v1, including the sibling edit-authority
// schema (design.md S1.3 rule 4: never reuse the consent schema).
func TestSDDGovernanceGateResultValidateRejectsUnknownSchema(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{name: "esquema vacio", schema: ""},
		{name: "esquema del envoltorio de consentimiento hermano", schema: SDDIntegrationConsentSchema},
		{name: "esquema con version distinta", schema: "gentle-ai.sdd-governance.gate/v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validGovernanceGateResult()
			result.Schema = tt.schema
			if err := result.Validate(); err == nil {
				t.Fatalf("Validate() = nil, se esperaba rechazo por esquema %q", tt.schema)
			}
		})
	}
}

// TestSDDGovernanceGateResultValidateRejectsChoiceSetsOtherThanApprovedRejected
// is task 12.1's second required case, calcado de
// internal/consentenvelope/envelope.go:61-65 (ValidateCompleteness): exactly
// two choices, "approved" then "rejected", in that order.
func TestSDDGovernanceGateResultValidateRejectsChoiceSetsOtherThanApprovedRejected(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SDDGovernanceGateResult)
	}{
		{
			name: "orden invertido",
			mutate: func(r *SDDGovernanceGateResult) {
				r.Choices[0], r.Choices[1] = r.Choices[1], r.Choices[0]
			},
		},
		{
			name:   "answer ajeno al vocabulario de compuertas",
			mutate: func(r *SDDGovernanceGateResult) { r.Choices[0].Answer = "granted" },
		},
		{
			name:   "una sola choice",
			mutate: func(r *SDDGovernanceGateResult) { r.Choices = r.Choices[:1] },
		},
		{
			name: "tres choices",
			mutate: func(r *SDDGovernanceGateResult) {
				r.Choices = append(r.Choices, r.Choices[0])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validGovernanceGateResult()
			tt.mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatalf("Validate() = nil, se esperaba rechazo por conjunto de choices invalido (%s)", tt.name)
			}
		})
	}
}

// TestSDDGovernanceGateResultValidateRejectsInvocationNotStartingWithGateRecord
// is task 12.1's third required case and T-9's named adversarial vector: an
// invocation with the "sdd-attempt" prefix must never validate as a gate
// choice, because that would relay a quality decision as edit-authority
// escalation (design.md S1.3 rule 4).
func TestSDDGovernanceGateResultValidateRejectsInvocationNotStartingWithGateRecord(t *testing.T) {
	tests := []struct {
		name       string
		invocation string
	}{
		{name: "prefijo sdd-attempt grant (T-9)", invocation: "axiom sdd-attempt grant --cwd . --change inc-99-governance-example --root ."},
		{name: "prefijo sdd status", invocation: "axiom sdd status --cwd . --change inc-99-governance-example"},
		{name: "cadena vacia", invocation: ""},
		{name: "verbo similar sin el espacio final del prefijo", invocation: "axiom sdd gate recorder --cwd ."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validGovernanceGateResult()
			result.Choices[0].Invocation = tt.invocation
			if err := result.Validate(); err == nil {
				t.Fatalf("Validate() = nil, se esperaba rechazo por invocacion %q", tt.invocation)
			}
		})
	}
}

// TestSDDGovernanceGateResultValidateAcceptsEachDesignCriteriaRow is task
// 12.1's fourth required case: every one of design.md S5.5's five gate
// criteria rows, taken as-is from the shared gateCriteria table, must
// validate for its own gate identity.
func TestSDDGovernanceGateResultValidateAcceptsEachDesignCriteriaRow(t *testing.T) {
	tests := []struct {
		name  string
		state kickoff.GateState
	}{
		{name: "spec", state: kickoff.GateState{Key: kickoff.GateSpec}},
		{name: "design", state: kickoff.GateState{Key: kickoff.GateDesign}},
		{name: "tasks", state: kickoff.GateState{Key: kickoff.GateTasks}},
		{name: "role-apply", state: kickoff.GateState{Key: kickoff.RoleApplyGate("fullstack"), Blocks: "fullstack"}},
		{name: "integration", state: kickoff.GateState{Key: kickoff.GateIntegration}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validGovernanceGateResult()
			result.Gate = string(tt.state.Key)
			result.Criteria = gateCriteria(tt.state)
			if len(result.Criteria) == 0 {
				t.Fatalf("gateCriteria(%v) = empty, se esperaba al menos una lente para esta compuerta", tt.state.Key)
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, se esperaba aceptacion para la compuerta %q con sus criterios tal cual", err, tt.state.Key)
			}
		})
	}
}

// --- loadGovernance: task 12.2's translation from []kickoff.GateState to
// the per-status view. Not itemised in tasks.md 12.1's prose (which lists
// only Validate()'s adversarial cases), but it is production code this same
// file introduces, so it gets its own RED coverage under strict TDD before
// governance.go exists (this file references loadGovernance, Governance,
// GovernanceRoster, and gateCriteria, none of which exist yet).

func TestLoadGovernanceReturnsNilWithoutErrorWhenNoKickoffIsSealed(t *testing.T) {
	changeRoot := t.TempDir()
	got, err := loadGovernance(changeRoot)
	if err != nil {
		t.Fatalf("loadGovernance() error = %v, se esperaba nil para un cambio sin kickoff.yaml", err)
	}
	if got != nil {
		t.Fatalf("loadGovernance() = %#v, se esperaba nil para un cambio sin kickoff.yaml", got)
	}
}

// TestLoadGovernanceReturnsNilForContinuousExecutionStyle is Phase 13's
// extension of loadGovernance (task 13.2, D-05): a sealed kickoff in
// continuous mode must report NO governance at all, not a Governance value
// with an empty Gates slice. kickoff.EvaluateGates already returns an empty
// slice for continuous mode on its own, so this proves loadGovernance goes
// one step further and reports structural absence, matching the field-level
// contract Status.Governance documents.
func TestLoadGovernanceReturnsNilForContinuousExecutionStyle(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
	_, ok, err := kickoff.Seal(changeRoot, kickoff.Kickoff{
		Schema: kickoff.KickoffSchemaV1, Change: filepath.Base(changeRoot), SealedBy: "test",
		Config: kickoff.FlowConfig{
			FlowMode: kickoff.FlowSDD, ExecutionStyle: kickoff.ExecutionContinuous,
			ExecutionStyleSource: "explicit", HandoffPolicy: kickoff.HandoffNone,
			Roles: []kickoff.KickoffRole{{Role: "fullstack", GatePolicy: "blocking", TasksFile: "tasks.md", VerifyFile: "verify-report.md"}},
		},
		Lifecycle: kickoff.Lifecycle{DeploymentTarget: "local", PostArchivePolicy: "bug_only"},
	})
	if err != nil || !ok {
		t.Fatalf("kickoff.Seal() = (ok=%v, err=%v), se esperaba un sellado exitoso de preparacion", ok, err)
	}

	got, err := loadGovernance(changeRoot)
	if err != nil {
		t.Fatalf("loadGovernance() error = %v", err)
	}
	if got != nil {
		t.Fatalf("loadGovernance() = %#v, se esperaba nil para un sello en modo continuo (D-05)", got)
	}
}

func TestLoadGovernancePropagatesACorruptKickoffAsANamedError(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, filepath.Join(changeRoot, kickoff.KickoffFileName), "{ not: valid: yaml")
	if _, err := loadGovernance(changeRoot); err == nil {
		t.Fatal("loadGovernance() = nil error, se esperaba un error nombrado para kickoff.yaml corrupto (T-10: nunca degradar a \"sin sello\")")
	}
}

func TestLoadGovernanceTranslatesSealedKickoffAndLedgerIntoGateStates(t *testing.T) {
	changeRoot := t.TempDir()
	writeFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
	writeFile(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
	writeFile(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
	writeFile(t, filepath.Join(changeRoot, "specs", "example", "spec.md"), "### Requirement: X\n")

	sealed, sealedOK, err := kickoff.Seal(changeRoot, kickoff.Kickoff{
		Schema: kickoff.KickoffSchemaV1, Change: filepath.Base(changeRoot),
		SealedBy: "test",
		Config: kickoff.FlowConfig{
			FlowMode: kickoff.FlowSDD, ExecutionStyle: kickoff.ExecutionCheckpointed,
			ExecutionStyleSource: "explicit", HandoffPolicy: kickoff.HandoffNone,
			Roles: []kickoff.KickoffRole{{Role: "fullstack", GatePolicy: "blocking", TasksFile: "tasks.md", VerifyFile: "verify-report.md"}},
		},
		Lifecycle: kickoff.Lifecycle{DeploymentTarget: "local", PostArchivePolicy: "bug_only"},
	})
	if err != nil || !sealedOK {
		t.Fatalf("kickoff.Seal() = (%v, %v, %v), se esperaba un sellado exitoso de preparacion", sealed, sealedOK, err)
	}

	got, err := loadGovernance(changeRoot)
	if err != nil {
		t.Fatalf("loadGovernance() error = %v", err)
	}
	if got == nil {
		t.Fatal("loadGovernance() = nil, se esperaba una vista de gobernanza para un cambio sellado")
	}
	if got.Kickoff.Change != filepath.Base(changeRoot) {
		t.Fatalf("Governance.Kickoff.Change = %q, want %q", got.Kickoff.Change, filepath.Base(changeRoot))
	}
	if got.Roster.Source != governanceRosterSourceKickoff {
		t.Fatalf("Governance.Roster.Source = %q, want %q", got.Roster.Source, governanceRosterSourceKickoff)
	}
	if len(got.Roster.Roles) != 1 || got.Roster.Roles[0] != "fullstack" {
		t.Fatalf("Governance.Roster.Roles = %v, want [fullstack]", got.Roster.Roles)
	}
	// specs/design are "done" (non-empty), tasks.md exists but the roster's
	// single role has one pending task, so its role-apply gate must not be
	// open yet: EvaluateGates opens spec+design, and never opens role-apply
	// for a role RolePending reports as still pending.
	statusByKey := map[kickoff.GateKey]string{}
	for _, g := range got.Gates {
		statusByKey[g.Key] = g.Status
	}
	if statusByKey[kickoff.GateSpec] != "pending" {
		t.Fatalf("gate spec status = %q, want pending (artifact present, no decision recorded yet)", statusByKey[kickoff.GateSpec])
	}
	if _, roleApplyOpened := statusByKey[kickoff.RoleApplyGate("fullstack")]; roleApplyOpened {
		t.Fatalf("role-apply:fullstack should not be open yet: its one task is still pending")
	}

	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateSpec, Decision: kickoff.DecisionApproved, Reason: "cubre la intencion", Actor: "test",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	got, err = loadGovernance(changeRoot)
	if err != nil {
		t.Fatalf("loadGovernance() (segunda lectura) error = %v", err)
	}
	statusByKey = map[kickoff.GateKey]string{}
	for _, g := range got.Gates {
		statusByKey[g.Key] = g.Status
	}
	if statusByKey[kickoff.GateSpec] != "approved" {
		t.Fatalf("gate spec status tras AppendGate = %q, want approved", statusByKey[kickoff.GateSpec])
	}
}

// --- newGovernanceGateQuestion (design.md S5.5): the production
// constructor that fills SDDGovernanceGateResult for one open gate. Until
// this constructor existed, gateCriteria and SDDGovernanceGateResult.Validate
// had no production caller: only governance_test.go referenced them.

func TestNewGovernanceGateQuestionBuildsValidEnvelopeCarryingGateCriteria(t *testing.T) {
	gate := kickoff.GateState{Key: kickoff.GateSpec, Status: "pending"}
	artifactPaths := ArtifactPaths{Specs: []string{"openspec/changes/inc-99/specs/example/spec.md"}}

	result, err := newGovernanceGateQuestion("inc-99-example", "/repo", gate, artifactPaths)
	if err != nil {
		t.Fatalf("newGovernanceGateQuestion() error = %v, want a valid envelope for a recognised gate key", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("result.Validate() error = %v, want the constructor to always return a valid envelope", err)
	}
	if result.Change != "inc-99-example" || result.Gate != string(kickoff.GateSpec) {
		t.Fatalf("result.Change/Gate = %q/%q, want inc-99-example/spec", result.Change, result.Gate)
	}
	wantCriteria := gateCriteria(gate)
	if len(result.Criteria) != len(wantCriteria) {
		t.Fatalf("result.Criteria = %v, want exactly gateCriteria's own lenses %v", result.Criteria, wantCriteria)
	}
	for i, criterion := range wantCriteria {
		if result.Criteria[i] != criterion {
			t.Fatalf("result.Criteria[%d] = %q, want %q (gateCriteria's own lens)", i, result.Criteria[i], criterion)
		}
	}
	if len(result.Evidence) != 1 || result.Evidence[0] != artifactPaths.Specs[0] {
		t.Fatalf("result.Evidence = %v, want the spec artifact path %q", result.Evidence, artifactPaths.Specs[0])
	}
}

// TestNewGovernanceGateQuestionChoicesCarryRunnableGateRecordInvocations is
// the orchestrator's second required case: the approved and rejected
// choices both carry a runnable `axiom sdd gate record` invocation naming
// their own --decision, never the sdd-attempt vocabulary T-9 guards
// against.
func TestNewGovernanceGateQuestionChoicesCarryRunnableGateRecordInvocations(t *testing.T) {
	gate := kickoff.GateState{Key: kickoff.GateDesign, Status: "pending"}
	result, err := newGovernanceGateQuestion("inc-99-example", "/repo", gate, ArtifactPaths{})
	if err != nil {
		t.Fatalf("newGovernanceGateQuestion() error = %v", err)
	}
	if len(result.Choices) != 2 {
		t.Fatalf("len(result.Choices) = %d, want exactly 2 (approved, rejected)", len(result.Choices))
	}
	approved, rejected := result.Choices[0], result.Choices[1]
	if approved.Answer != gateAnswerApproved || !strings.HasPrefix(approved.Invocation, gateRecordInvocationPrefix) ||
		!strings.Contains(approved.Invocation, "--decision approved") || !strings.Contains(approved.Invocation, "--gate design") {
		t.Fatalf("approved choice = %+v, want a runnable %q invocation with --decision approved --gate design", approved, gateRecordInvocationPrefix)
	}
	if rejected.Answer != gateAnswerRejected || !strings.HasPrefix(rejected.Invocation, gateRecordInvocationPrefix) ||
		!strings.Contains(rejected.Invocation, "--decision rejected") || !strings.Contains(rejected.Invocation, "--gate design") {
		t.Fatalf("rejected choice = %+v, want a runnable %q invocation with --decision rejected --gate design", rejected, gateRecordInvocationPrefix)
	}
	if strings.HasPrefix(approved.Invocation, "sdd-attempt") || strings.Contains(approved.Invocation, " sdd-attempt ") {
		t.Fatalf("approved choice = %+v, must never carry the sdd-attempt vocabulary (T-9)", approved)
	}
}

// TestNewGovernanceGateQuestionOffPathReentersNativeStatus is the
// orchestrator's third required case: the off path never decides the gate,
// it only re-enters through native SDD status.
func TestNewGovernanceGateQuestionOffPathReentersNativeStatus(t *testing.T) {
	gate := kickoff.GateState{Key: kickoff.GateTasks, Status: "pending"}
	result, err := newGovernanceGateQuestion("inc-99-example", "/repo", gate, ArtifactPaths{})
	if err != nil {
		t.Fatalf("newGovernanceGateQuestion() error = %v", err)
	}
	if !strings.HasPrefix(result.OffPath.Command, gateStatusInvocationPrefix) {
		t.Fatalf("result.OffPath.Command = %q, want it to start with %q", result.OffPath.Command, gateStatusInvocationPrefix)
	}
	if result.OffPath.Note == "" {
		t.Fatal("result.OffPath.Note is empty, want an explanation of the deliberate off path")
	}
}

// TestNewGovernanceGateQuestionUsesRoleApplyCriteriaForRoleApplyGate proves
// the constructor recognises a role-apply:<role> gate the same way
// gateCriteria does, rather than only the four fixed keys.
func TestNewGovernanceGateQuestionUsesRoleApplyCriteriaForRoleApplyGate(t *testing.T) {
	gate := kickoff.GateState{Key: kickoff.RoleApplyGate("backend"), Blocks: "backend", Status: "pending"}
	result, err := newGovernanceGateQuestion("inc-99-example", "/repo", gate, ArtifactPaths{Tasks: []string{"openspec/changes/inc-99/tasks.backend.md"}})
	if err != nil {
		t.Fatalf("newGovernanceGateQuestion() error = %v", err)
	}
	if len(result.Criteria) != len(roleApplyGateCriteria) {
		t.Fatalf("result.Criteria = %v, want roleApplyGateCriteria %v", result.Criteria, roleApplyGateCriteria)
	}
	if len(result.Evidence) != 1 || result.Evidence[0] != "openspec/changes/inc-99/tasks.backend.md" {
		t.Fatalf("result.Evidence = %v, want the role's own tasks file", result.Evidence)
	}
}

// TestNewGovernanceGateQuestionRejectsUnrecognisedGateKey proves the
// constructor is validated by construction (design.md S5.5): a gate key
// that gateCriteria cannot resolve any lenses for must never produce a
// malformed envelope with empty Criteria -- it must fail loudly instead.
func TestNewGovernanceGateQuestionRejectsUnrecognisedGateKey(t *testing.T) {
	gate := kickoff.GateState{Key: kickoff.GateKey("unrecognised-gate"), Status: "pending"}
	if _, err := newGovernanceGateQuestion("inc-99-example", "/repo", gate, ArtifactPaths{}); err == nil {
		t.Fatal("newGovernanceGateQuestion() error = nil, want a named error for a gate key gateCriteria cannot resolve")
	}
}
