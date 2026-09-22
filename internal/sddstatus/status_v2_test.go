package sddstatus

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

func TestProjectStatusV2RejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Status)
		want   string
	}{
		{
			name: "unknown next action",
			mutate: func(status *Status) {
				status.NextRecommended = "working"
			},
			want: `unsupported SDD v2 next action "working"`,
		},
		{
			name: "unknown artifact state",
			mutate: func(status *Status) {
				status.Artifacts["proposal"] = "checking"
			},
			want: `unsupported SDD v2 artifact "proposal" state "checking"`,
		},
		{
			name: "unknown artifact store",
			mutate: func(status *Status) {
				status.ArtifactStore = "workrun"
			},
			want: `unsupported SDD v2 artifact store "workrun"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
			tt.mutate(&status)
			_, err := ProjectStatusV2(status)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ProjectStatusV2() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStatusRenderersEmbedOnlyStatusV2Projection(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)

	rendered := map[string]string{
		"markdown":     RenderMarkdown(status),
		"dispatcher":   RenderDispatcherMarkdown(status),
		"native phase": RenderNativePhasePrompt(status, PhaseApply),
	}
	for name, output := range rendered {
		t.Run(name, func(t *testing.T) {
			for _, forbidden := range []string{"runtimeStatus", "internal-only", "reviewGate", "reviewTransaction", "reVerify"} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("%s leaked %q:\n%s", name, forbidden, output)
				}
			}
			if !strings.Contains(output, `"schemaVersion": 2`) {
				t.Fatalf("%s omitted v2 projected SDD status:\n%s", name, output)
			}
		})
	}
}

// --- INC-21 Phase 14 (P3d): v2 projection and "await-gate" routing
// (design.md S4.3, S5.4, D-09; spec.md REQ-21.7 to REQ-21.10's BDD
// scenarios that produce an observable route). Every fixture here builds
// through the public Resolve pipeline rather than hand-assembling a
// Status, so these tests exercise the real translation Phase 13 wired,
// projected through ProjectStatusV2.

// TestProjectStatusV2GovernancePresentWithSealAndPendingGate covers
// REQ-21.7's first scenario at the v2 boundary: a sealed, checkpointed
// change with no gate decision yet projects a non-nil Governance carrying
// kickoff, gates, and roster.
func TestProjectStatusV2GovernancePresentWithSealAndPendingGate(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-pending"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.Governance == nil {
		t.Fatal("StatusV2Projection.Governance = nil, want a populated governance block for a sealed checkpointed change")
	}
	if projection.Governance.Kickoff.Schema != kickoff.KickoffSchemaV1 {
		t.Fatalf("Governance.Kickoff.Schema = %q, want %q", projection.Governance.Kickoff.Schema, kickoff.KickoffSchemaV1)
	}
	if projection.Governance.Kickoff.ExecutionStyle != string(kickoff.ExecutionCheckpointed) {
		t.Fatalf("Governance.Kickoff.ExecutionStyle = %q, want %q", projection.Governance.Kickoff.ExecutionStyle, kickoff.ExecutionCheckpointed)
	}
	foundPendingSpec := false
	for _, g := range projection.Governance.Gates {
		if g.Key == string(kickoff.GateSpec) && g.Status == "pending" {
			foundPendingSpec = true
		}
	}
	if !foundPendingSpec {
		t.Fatalf("Governance.Gates = %+v, want a pending spec gate", projection.Governance.Gates)
	}
	if projection.Governance.Roster.Source != governanceRosterSourceKickoff ||
		len(projection.Governance.Roster.Roles) != 1 || projection.Governance.Roster.Roles[0] != "fullstack" {
		t.Fatalf("Governance.Roster = %+v, want {kickoff [fullstack]}", projection.Governance.Roster)
	}
}

// TestProjectStatusV2GovernanceAbsentWithoutSealOmitsJSONKey repeats Phase
// 11's control-gate assertion at the v2 boundary: an unsealed change never
// names "governance" in the serialized document (omitempty), not merely a
// nil Go field.
func TestProjectStatusV2GovernanceAbsentWithoutSealOmitsJSONKey(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-unsealed"
	seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.Governance != nil {
		t.Fatalf("StatusV2Projection.Governance = %+v, want nil for an unsealed change", projection.Governance)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), `"governance"`) {
		t.Fatalf("serialized StatusV2Projection names \"governance\" for an unsealed change: %s", encoded)
	}
}

// TestProjectStatusV2GovernanceAbsentForContinuousSeal is REQ-21.7's second
// scenario at the v2 boundary: a continuous-execution seal chains phases
// without ever presenting a gate, so Governance stays nil there too, not
// only for a change with no seal at all.
func TestProjectStatusV2GovernanceAbsentForContinuousSeal(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-continuous"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionContinuous)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.Governance != nil {
		t.Fatalf("StatusV2Projection.Governance = %+v, want nil for a continuous-execution seal (REQ-21.7)", projection.Governance)
	}
	if projection.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply: continuous mode chains phases without any gate (REQ-21.7)", projection.NextRecommended)
	}
}

// TestProjectStatusV2GovernanceSurfacesRejectedGateReason proves the wire
// shape carries D-09's exact registered rejection reason, not a paraphrase,
// through the v2 gateV2 projection.
func TestProjectStatusV2GovernanceSurfacesRejectedGateReason(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-rejected"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	specPath := filepath.Join(changeRoot, "specs", "auth", "spec.md")
	digest, err := kickoff.ArtifactDigest([]string{specPath})
	if err != nil {
		t.Fatalf("kickoff.ArtifactDigest() error = %v", err)
	}
	const rejectionReason = "Falta cubrir el caso de sesion expirada"
	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateSpec, Decision: kickoff.DecisionRejected, Reason: rejectionReason,
		ArtifactDigest: digest, Actor: "reviewer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.NextRecommended != "spec" {
		t.Fatalf("NextRecommended = %q, want spec", projection.NextRecommended)
	}
	found := false
	for _, g := range projection.Governance.Gates {
		if g.Key == string(kickoff.GateSpec) {
			found = true
			if g.Status != "rejected" || g.Reason != rejectionReason {
				t.Fatalf("gateV2 for spec = %+v, want status=rejected reason=%q", g, rejectionReason)
			}
		}
	}
	if !found {
		t.Fatal("Governance.Gates has no entry for spec")
	}
}

// TestProjectStatusV2GovernanceApprovedGatesDoNotOverrideRouting is
// REQ-21.8/21.9/21.10's "approved habilita la siguiente fase" pattern: once
// spec, design, and tasks are all approved and no role-apply gate has
// opened yet, ordinary routing resumes exactly as it would without
// governance -- an approved gate must never block or reroute.
func TestProjectStatusV2GovernanceApprovedGatesDoNotOverrideRouting(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-approved"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	for _, approval := range []struct {
		gate kickoff.GateKey
		path string
	}{
		{kickoff.GateSpec, filepath.Join(changeRoot, "specs", "auth", "spec.md")},
		{kickoff.GateDesign, filepath.Join(changeRoot, "design.md")},
		{kickoff.GateTasks, filepath.Join(changeRoot, "tasks.md")},
	} {
		digest, err := kickoff.ArtifactDigest([]string{approval.path})
		if err != nil {
			t.Fatalf("kickoff.ArtifactDigest(%s) error = %v", approval.path, err)
		}
		if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
			Gate: approval.gate, Decision: kickoff.DecisionApproved, Reason: "ok", ArtifactDigest: digest, Actor: "reviewer",
		}); err != nil {
			t.Fatalf("kickoff.AppendGate(%s) error = %v", approval.gate, err)
		}
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	// role-apply:fullstack has not opened yet (its one task is still
	// pending), so no gate is open at all: ordinary routing must resume.
	if projection.NextRecommended != "apply" {
		t.Fatalf("NextRecommended = %q, want apply: three approved gates must never block or reroute once decided", projection.NextRecommended)
	}
}

// TestNonPhaseRoutingInstructionsPrintsAwaitGateInvocations is task 14.1's
// last required case: "await-gate" gains its own case in
// nonPhaseRoutingInstructions, printing the two exact executable
// invocations for the specific gate currently open -- one to inspect it,
// one to record a decision -- exactly as the existing "select-change" case
// already does for its own two re-entry commands.
func TestNonPhaseRoutingInstructionsPrintsAwaitGateInvocations(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-instructions"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "await-gate" {
		t.Fatalf("NextRecommended = %q, want await-gate (fixture precondition)", status.NextRecommended)
	}
	if !statusV2NextRecommended(status.NextRecommended) {
		t.Fatal("statusV2NextRecommended(await-gate) = false, want true: this is an additive v2 enum value")
	}

	instructions, ok := nonPhaseRoutingInstructions(status)
	if !ok {
		t.Fatal("nonPhaseRoutingInstructions() ok = false, want true for await-gate")
	}
	joined := strings.Join(instructions, "\n")
	for _, want := range []string{
		"axiom sdd gate show", "axiom sdd gate record",
		"--change " + changeName, "--gate spec",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("instructions = %v, want containing %q", instructions, want)
		}
	}
}

// --- Status.GateQuestion / StatusV2Projection.GateQuestion (design.md
// S5.5): the resolver wiring that turns SDDGovernanceGateResult from a
// tested-but-never-constructed type into the envelope an "await-gate" route
// actually carries.

// TestResolveAssignsGateQuestionForPendingGate proves the resolver builds
// and assigns a valid gate-question envelope for the exact gate
// firstOpenGate names when NextRecommended is "await-gate".
func TestResolveAssignsGateQuestionForPendingGate(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-gate-question-pending"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "await-gate" {
		t.Fatalf("NextRecommended = %q, want await-gate (fixture precondition)", status.NextRecommended)
	}
	if status.GateQuestion == nil {
		t.Fatal("status.GateQuestion = nil, want a populated envelope while a gate is pending")
	}
	if err := status.GateQuestion.Validate(); err != nil {
		t.Fatalf("status.GateQuestion.Validate() error = %v", err)
	}
	if status.GateQuestion.Gate != string(kickoff.GateSpec) || status.GateQuestion.Change != changeName {
		t.Fatalf("status.GateQuestion.Gate/Change = %q/%q, want spec/%q", status.GateQuestion.Gate, status.GateQuestion.Change, changeName)
	}
	if len(status.GateQuestion.Criteria) == 0 {
		t.Fatal("status.GateQuestion.Criteria is empty, want design.md S5.5's spec lenses")
	}
}

// TestResolveGateQuestionNilWhenRejectedGateRoutesToOwningPhase proves the
// envelope is scoped to the exact gate "await-gate" names: a rejected gate
// whose digest still matches the artifact reroutes to remediation instead
// (D-09), and must never carry a fresh approve/reject question of its own
// even though Governance itself stays populated.
func TestResolveGateQuestionNilWhenRejectedGateRoutesToOwningPhase(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-gate-question-rejected"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	specPath := filepath.Join(changeRoot, "specs", "auth", "spec.md")
	digest, err := kickoff.ArtifactDigest([]string{specPath})
	if err != nil {
		t.Fatalf("kickoff.ArtifactDigest() error = %v", err)
	}
	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.GateSpec, Decision: kickoff.DecisionRejected, Reason: "falta caso borde",
		ArtifactDigest: digest, Actor: "reviewer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.NextRecommended != "spec" {
		t.Fatalf("NextRecommended = %q, want spec (rejected gate's owning phase, fixture precondition)", status.NextRecommended)
	}
	if status.Governance == nil {
		t.Fatal("status.Governance = nil, want the sealed governance view to stay populated")
	}
	if status.GateQuestion != nil {
		t.Fatalf("status.GateQuestion = %+v, want nil: a rejected gate routed to remediation carries no fresh decision question", status.GateQuestion)
	}
}

// TestProjectStatusV2GateQuestionPresentWithPendingGate proves the v2 wire
// projection carries the same envelope Status.GateQuestion does, reusing
// SDDGovernanceGateResult directly (no *V2 translation type), exactly as
// Consent already does for its own sibling schema.
func TestProjectStatusV2GateQuestionPresentWithPendingGate(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-gate-question-pending"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.GateQuestion == nil {
		t.Fatal("StatusV2Projection.GateQuestion = nil, want a populated envelope while a gate is pending")
	}
	if projection.GateQuestion != status.GateQuestion {
		t.Fatal("StatusV2Projection.GateQuestion is not the same envelope as Status.GateQuestion: Consent's precedent passes it through unchanged")
	}
}

// TestProjectStatusV2GateQuestionAbsentWithoutSealOmitsJSONKey repeats the
// D-05 control-gate assertion for the new field: an unsealed change never
// names "gateQuestion" in the serialized document (omitempty), not merely a
// nil Go field.
func TestProjectStatusV2GateQuestionAbsentWithoutSealOmitsJSONKey(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-v2-gate-question-unsealed"
	seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	if projection.GateQuestion != nil {
		t.Fatalf("StatusV2Projection.GateQuestion = %+v, want nil for an unsealed change", projection.GateQuestion)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), `"gateQuestion"`) {
		t.Fatalf("serialized StatusV2Projection names \"gateQuestion\" for an unsealed change: %s", encoded)
	}
}
