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

// TestEvaluateGatesRejectedGateStaysRejectedWhenDigestUnchanged is D-08's
// first reopening rule: a rejection against the artifact that is STILL the
// current one is not stale — it must keep blocking exactly as recorded.
func TestEvaluateGatesRejectedGateStaysRejectedWhenDigestUnchanged(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Artifacts: map[string]string{"spec": "d1"},
		Ledger: GateLedger{
			Schema: GateLedgerSchemaV1,
			Records: []GateRecord{
				{Gate: GateSpec, Decision: DecisionRejected, Reason: "falta el caso borde Y", ArtifactDigest: "d1", Actor: "maintainer"},
			},
		},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	state := findGateState(t, states, GateSpec)
	if state.Status != "rejected" {
		t.Errorf("Status = %q, se esperaba rejected (digest sin cambios)", state.Status)
	}
	if state.Reason != "falta el caso borde Y" {
		t.Errorf("Reason = %q, se esperaba el motivo del ultimo rechazo", state.Reason)
	}
	if state.Reopened {
		t.Error("Reopened = true, se esperaba false: el digest no cambio")
	}
}

// TestEvaluateGatesRejectedGateReopensWhenDigestChanges is D-08's second
// reopening rule, and REQ-21.12 without an explicit "reopen" verb: once
// the judged artifact's digest differs from the one the rejection judged,
// remediation happened, and the gate returns to pending on its own.
func TestEvaluateGatesRejectedGateReopensWhenDigestChanges(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Artifacts: map[string]string{"spec": "d2-remediado"},
		Ledger: GateLedger{
			Schema: GateLedgerSchemaV1,
			Records: []GateRecord{
				{Gate: GateSpec, Decision: DecisionRejected, Reason: "falta el caso borde Y", ArtifactDigest: "d1", Actor: "maintainer"},
			},
		},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	state := findGateState(t, states, GateSpec)
	if state.Status != "pending" {
		t.Errorf("Status = %q, se esperaba pending tras remediar (digest distinto)", state.Status)
	}
	if !state.Reopened {
		t.Error("Reopened = false, se esperaba true: el digest cambio tras el rechazo")
	}
}

// TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange is D-08's
// most sensitive rule: approval is TERMINAL. Neither a later artifact edit
// nor a stray extra ledger record after the approval may undo it — undoing
// it would deadlock sdd-apply against its own tasks.md checkbox edits.
func TestEvaluateGatesApprovedGateNeverInvalidatedByDigestChange(t *testing.T) {
	t.Run("digest changed after approval", func(t *testing.T) {
		in := Inputs{
			Execution: ExecutionCheckpointed,
			Artifacts: map[string]string{"spec": "d999-editado-tras-aprobar"},
			Ledger: GateLedger{
				Schema: GateLedgerSchemaV1,
				Records: []GateRecord{
					{Gate: GateSpec, Decision: DecisionRejected, Reason: "primero", ArtifactDigest: "d1", Actor: "maintainer"},
					{Gate: GateSpec, Decision: DecisionApproved, Reason: "cubierto en REQ-21.4", ArtifactDigest: "d2", Actor: "maintainer"},
				},
			},
		}
		states, err := EvaluateGates(in)
		if err != nil {
			t.Fatalf("EvaluateGates() error = %v", err)
		}
		state := findGateState(t, states, GateSpec)
		if state.Status != "approved" {
			t.Errorf("Status = %q, se esperaba approved (terminal, D-08)", state.Status)
		}
		if state.Reason != "cubierto en REQ-21.4" {
			t.Errorf("Reason = %q, se esperaba el motivo de la aprobacion", state.Reason)
		}
		if state.Reopened {
			t.Error("Reopened = true, una compuerta approved nunca se reabre")
		}
	})

	t.Run("stray record appended after approval", func(t *testing.T) {
		in := Inputs{
			Execution: ExecutionCheckpointed,
			Artifacts: map[string]string{"spec": "d1"},
			Ledger: GateLedger{
				Schema: GateLedgerSchemaV1,
				Records: []GateRecord{
					{Gate: GateSpec, Decision: DecisionApproved, Reason: "cubierto", ArtifactDigest: "d1", Actor: "maintainer"},
					{Gate: GateSpec, Decision: DecisionRejected, Reason: "anomalia de datos", ArtifactDigest: "d1", Actor: "maintainer"},
				},
			},
		}
		states, err := EvaluateGates(in)
		if err != nil {
			t.Fatalf("EvaluateGates() error = %v", err)
		}
		state := findGateState(t, states, GateSpec)
		if state.Status != "approved" {
			t.Errorf("Status = %q, se esperaba approved: una aprobacion es terminal para toda la historia de la compuerta", state.Status)
		}
	})
}

// TestEvaluateGatesMultiRoleGeneratesExactlyOneRoleApplyGatePerRole covers
// D-08's roster requirement directly: a three-role roster with mixed gate
// policies produces exactly three role-apply gates, each keyed only via
// RoleApplyGate — never a hand-built "role-apply:"+role literal.
func TestEvaluateGatesMultiRoleGeneratesExactlyOneRoleApplyGatePerRole(t *testing.T) {
	in := Inputs{
		Execution: ExecutionCheckpointed,
		Roles: []multirole.RoleAssignment{
			{Role: "core", GatePolicy: multirole.PolicyBlocking},
			{Role: "web", GatePolicy: multirole.PolicyBlocking},
			{Role: "qa", GatePolicy: multirole.PolicyDeferred},
		},
		Artifacts:   map[string]string{"tasks.core": "d1", "tasks.web": "d2", "tasks.qa": "d3"},
		RolePending: map[string]int{"core": 0, "web": 0, "qa": 0},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	wantKeys := map[GateKey]bool{
		RoleApplyGate("core"): true,
		RoleApplyGate("web"):  true,
		RoleApplyGate("qa"):   true,
	}
	gotKeys := make(map[GateKey]bool, len(states))
	for _, g := range states {
		gotKeys[g.Key] = true
	}
	if len(gotKeys) != len(wantKeys) {
		t.Fatalf("states = %+v, se esperaban exactamente %d compuertas role-apply", states, len(wantKeys))
	}
	for key := range wantKeys {
		if !gotKeys[key] {
			t.Errorf("falta la compuerta %q en %+v", key, states)
		}
	}
}

// TestEvaluateGatesRoleApplyGateReopensWhenRoleTasksDigestChanges proves
// the digest reopening rule applies uniformly to role-apply gates, not
// only to the four fixed ones: a rejected role-apply reopens once that
// role's own tasks file changes.
func TestEvaluateGatesRoleApplyGateReopensWhenRoleTasksDigestChanges(t *testing.T) {
	in := Inputs{
		Execution:   ExecutionCheckpointed,
		Roles:       []multirole.RoleAssignment{{Role: "core", GatePolicy: multirole.PolicyBlocking}},
		Artifacts:   map[string]string{"tasks.core": "d2-remediado"},
		RolePending: map[string]int{"core": 0},
		Ledger: GateLedger{
			Schema: GateLedgerSchemaV1,
			Records: []GateRecord{
				{Gate: RoleApplyGate("core"), Decision: DecisionRejected, Reason: "cobertura insuficiente", ArtifactDigest: "d1", Actor: "maintainer"},
			},
		},
	}
	states, err := EvaluateGates(in)
	if err != nil {
		t.Fatalf("EvaluateGates() error = %v", err)
	}
	state := findGateState(t, states, RoleApplyGate("core"))
	if state.Status != "pending" || !state.Reopened {
		t.Errorf("role-apply:core = %+v, se esperaba pending+Reopened tras cambiar tasks.core", state)
	}
}

// TestLastRoleClosedRequiresEveryRoleApproved covers REQ-21.13's core
// predicate: the notice and the integration handoff must wait for every
// active role, not just most of them.
func TestLastRoleClosedRequiresEveryRoleApproved(t *testing.T) {
	roster := []multirole.RoleAssignment{
		{Role: "core", GatePolicy: multirole.PolicyBlocking},
		{Role: "web", GatePolicy: multirole.PolicyBlocking},
		{Role: "qa", GatePolicy: multirole.PolicyDeferred},
	}

	t.Run("N-1 of N approved is not closed", func(t *testing.T) {
		gates := []GateState{
			{Key: RoleApplyGate("core"), Status: "approved"},
			{Key: RoleApplyGate("web"), Status: "approved"},
			{Key: RoleApplyGate("qa"), Status: "pending"},
		}
		if LastRoleClosed(roster, gates) {
			t.Error("LastRoleClosed() = true, se esperaba false con un rol todavia pendiente")
		}
	})

	t.Run("all N approved is closed", func(t *testing.T) {
		gates := []GateState{
			{Key: RoleApplyGate("core"), Status: "approved"},
			{Key: RoleApplyGate("web"), Status: "approved"},
			{Key: RoleApplyGate("qa"), Status: "approved"},
		}
		if !LastRoleClosed(roster, gates) {
			t.Error("LastRoleClosed() = false, se esperaba true con los N roles aprobados")
		}
	})

	t.Run("one rejected role stays open even if the rest are approved", func(t *testing.T) {
		gates := []GateState{
			{Key: RoleApplyGate("core"), Status: "approved"},
			{Key: RoleApplyGate("web"), Status: "rejected"},
			{Key: RoleApplyGate("qa"), Status: "approved"},
		}
		if LastRoleClosed(roster, gates) {
			t.Error("LastRoleClosed() = true, se esperaba false: un rol rechazado nunca cierra el ultimo rol (REQ-21.13, tercer escenario)")
		}
	})
}

// TestLastRoleClosedSingleFullstackRoleClosesImmediately is REQ-21.13's
// second scenario: a one-role roster satisfies "last role" the moment that
// single role's own apply gate is approved.
func TestLastRoleClosedSingleFullstackRoleClosesImmediately(t *testing.T) {
	roster := []multirole.RoleAssignment{{Role: "fullstack", GatePolicy: multirole.PolicyBlocking}}
	gates := []GateState{{Key: RoleApplyGate("fullstack"), Status: "approved"}}
	if !LastRoleClosed(roster, gates) {
		t.Error("LastRoleClosed() = false, se esperaba true de inmediato para un roster de un unico rol fullstack aprobado")
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
