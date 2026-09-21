package kickoff

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
)

// TestEvaluateGatesContinuousModeIgnoresEverythingElse locks down D-05: in
// continuous mode EvaluateGates must short-circuit before it ever
// considers Roles, Artifacts, RolePending, VerifyFound, or Ledger. Every
// other field below is deliberately configured as if ALL gates should be
// open and approved, so a non-empty result here would prove the
// short-circuit is fake.
func TestEvaluateGatesContinuousModeIgnoresEverythingElse(t *testing.T) {
	in := Inputs{
		Execution: ExecutionContinuous,
		Roles: []multirole.RoleAssignment{
			{Role: "core", GatePolicy: multirole.PolicyBlocking},
		},
		Artifacts:   map[string]string{"spec": "d1", "design": "d2", "tasks": "d3", "tasks.core": "d4"},
		RolePending: map[string]int{"core": 0},
		VerifyFound: true,
		Ledger: GateLedger{
			Schema: GateLedgerSchemaV1,
			Records: []GateRecord{
				{Gate: GateSpec, Decision: DecisionApproved, Actor: "maintainer"},
			},
		},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v, se esperaba nil en modo continuo", err)
	}
	if len(states) != 0 {
		t.Fatalf("EvaluateGates() = %+v, se esperaba vacio en modo continuo aunque todo lo demas este listo (D-05)", states)
	}
}

// TestEvaluateGatesCheckpointedArtifactNotDoneStaysClosed asserts that a
// gate whose artifact has not reached "done" (empty digest) is absent from
// the result entirely — it is not merely "pending", it does not exist yet.
func TestEvaluateGatesCheckpointedArtifactNotDoneStaysClosed(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Artifacts: map[string]string{"spec": ""},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	for _, g := range states {
		if g.Key == GateSpec {
			t.Fatalf("EvaluateGates() = %+v, la compuerta spec no debia existir sin artefacto listo", states)
		}
	}
}

// TestEvaluateGatesCheckpointedArtifactDigestPresentOpensPending asserts
// that a present digest opens the gate in "pending" — no ledger record
// exists yet for it.
func TestEvaluateGatesCheckpointedArtifactDigestPresentOpensPending(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Artifacts: map[string]string{"spec": "sha256:abc"},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	state := findGateState(t, states, GateSpec)
	if state.Status != "pending" {
		t.Errorf("Status = %q, se esperaba pending", state.Status)
	}
	if state.Blocks != "design" {
		t.Errorf("Blocks = %q, se esperaba design", state.Blocks)
	}
	if state.Reopened {
		t.Error("Reopened = true, se esperaba false para una compuerta nunca decidida")
	}
}

// TestEvaluateGatesTasksGateRequiresEveryRoleTasksFileInMultiRole covers
// the "tasks" gate's extra multi-role condition from D-08: with more than
// one role, the shared tasks digest alone is not enough — every
// "tasks.<role>" digest must also be present.
func TestEvaluateGatesTasksGateRequiresEveryRoleTasksFileInMultiRole(t *testing.T) {
	roles := []multirole.RoleAssignment{
		{Role: "core", GatePolicy: multirole.PolicyBlocking},
		{Role: "web", GatePolicy: multirole.PolicyBlocking},
	}

	t.Run("missing one role tasks file keeps it closed", func(t *testing.T) {
		in := Inputs{
			Execution: ExecutionCheckpointed,
			Roles:     roles,
			Artifacts: map[string]string{"tasks": "d3", "tasks.core": "d5"},
		}
		states, err := EvaluateGates(in)
		if err != nil {
			t.Fatalf("EvaluateGates() error = %v", err)
		}
		for _, g := range states {
			if g.Key == GateTasks {
				t.Fatalf("EvaluateGates() = %+v, tasks no debia abrirse sin tasks.web", states)
			}
		}
	})

	t.Run("every role tasks file present opens it", func(t *testing.T) {
		in := Inputs{
			Execution: ExecutionCheckpointed,
			Roles:     roles,
			Artifacts: map[string]string{"tasks": "d3", "tasks.core": "d5", "tasks.web": "d6"},
		}
		states, err := EvaluateGates(in)
		if err != nil {
			t.Fatalf("EvaluateGates() error = %v", err)
		}
		state := findGateState(t, states, GateTasks)
		if state.Status != "pending" {
			t.Errorf("Status = %q, se esperaba pending", state.Status)
		}
	})
}

// TestEvaluateGatesFixedOrderIndependentOfRoleDeclarationOrder is the
// order-of-evaluation regression: spec, design, tasks, then one
// role-apply:<role> per role sorted by role name (never by roster
// declaration order), then integration.
func TestEvaluateGatesFixedOrderIndependentOfRoleDeclarationOrder(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Roles: []multirole.RoleAssignment{
			{Role: "web", GatePolicy: multirole.PolicyBlocking},
			{Role: "core", GatePolicy: multirole.PolicyBlocking},
			{Role: "qa", GatePolicy: multirole.PolicyDeferred},
		},
		Artifacts: map[string]string{
			"spec": "d1", "design": "d2", "tasks": "d3",
			"tasks.web": "d4", "tasks.core": "d5", "tasks.qa": "d6",
		},
		RolePending: map[string]int{"web": 0, "core": 0, "qa": 0},
		VerifyFound: true,
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}

	wantOrder := []GateKey{
		GateSpec, GateDesign, GateTasks,
		RoleApplyGate("core"), RoleApplyGate("qa"), RoleApplyGate("web"),
		GateIntegration,
	}
	if len(states) != len(wantOrder) {
		t.Fatalf("len(states) = %d, se esperaba %d; states=%+v", len(states), len(wantOrder), states)
	}
	for i, want := range wantOrder {
		if states[i].Key != want {
			t.Fatalf("states[%d].Key = %q, se esperaba %q (orden completo: %+v)", i, states[i].Key, want, states)
		}
	}
}

// TestEvaluateGatesRoleApplyGateStaysClosedWithoutExplicitPendingCount
// guards against a Go map footgun: an absent RolePending entry must never
// be silently treated as "0 pending" — that would open a role-apply gate
// for a role nobody ever actually reported on.
func TestEvaluateGatesRoleApplyGateStaysClosedWithoutExplicitPendingCount(t *testing.T) {
	in := Inputs{
		Execution:   ExecutionCheckpointed,
		Roles:       []multirole.RoleAssignment{{Role: "core", GatePolicy: multirole.PolicyBlocking}},
		Artifacts:   map[string]string{"tasks.core": "d5"},
		RolePending: map[string]int{},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	for _, g := range states {
		if g.Key == RoleApplyGate("core") {
			t.Fatalf("EvaluateGates() = %+v, role-apply:core no debia abrirse sin un recuento explicito de pendientes", states)
		}
	}
}

// findGateState is the shared table-lookup helper for the tests in this
// file: it fails the test immediately if the requested key is absent,
// instead of letting a nil dereference obscure which case failed.
func findGateState(t *testing.T, states []GateState, key GateKey) GateState {
	t.Helper()
	for _, g := range states {
		if g.Key == key {
			return g
		}
	}
	t.Fatalf("no se encontro la compuerta %q en %+v", key, states)
	return GateState{}
}
