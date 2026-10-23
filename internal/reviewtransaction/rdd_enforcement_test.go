package reviewtransaction

import "testing"

// rddControllerStatesForMatrix lists every controller state of the #1842
// three-fact contract, so the derivation sweeps cover the whole fact space
// rather than the states a caller happens to reach today.
var rddControllerStatesForMatrix = []RDDControllerState{
	RDDControllerAbsent,
	RDDControllerDeclared,
	RDDControllerWritten,
	RDDControllerCertifiedCurrent,
	RDDControllerStale,
	RDDControllerRevoked,
	RDDControllerInconclusive,
	RDDControllerFailed,
}

var rddDeliveryGateStatesForMatrix = []RDDDeliveryGateState{
	RDDDeliveryGateAbsent,
	RDDDeliveryGateAdvisoryCurrent,
	RDDDeliveryGateEnforcedCurrent,
	RDDDeliveryGateStale,
	RDDDeliveryGateRevoked,
	RDDDeliveryGateInconclusive,
	RDDDeliveryGateFailed,
}

// TestDeriveEnforcementReportsOffForEveryPolicyThatIsNotOn pins the policy
// fact's precedence: the public label reports off unless the kill switch
// resolved exactly on, mirroring the mode store's own any-off-wins posture.
// Whatever the controller and gate facts say, a switch that is off, unset, or
// unintelligible never projects an enforcement posture on top of that.
func TestDeriveEnforcementReportsOffForEveryPolicyThatIsNotOn(t *testing.T) {
	policies := []RDDMode{RDDModeOff, RDDModeUnset, RDDMode("managed"), RDDMode("")}
	for _, policy := range policies {
		for _, controller := range rddControllerStatesForMatrix {
			for _, gate := range rddDeliveryGateStatesForMatrix {
				if label := DeriveEnforcement(policy, controller, gate); label != RDDEnforcementOff {
					t.Fatalf("DeriveEnforcement(%q, %q, %q) = %q, want off: a policy that is not exactly on never projects enforcement",
						policy, controller, gate, label)
				}
			}
		}
	}
}

// TestDeriveEnforcementReservesManagedForCertifiedControllerAndEnforcedGate
// is the reservation the issue approved: managed is the non-bypassable
// repository/ref boundary, and it is claimed only when the controller fact is
// certified current and the delivery gate is enforced current. Every other
// combination — including an enforced gate backed by a merely written
// controller, and a certified controller beside a merely advisory gate — must
// stay below it.
func TestDeriveEnforcementReservesManagedForCertifiedControllerAndEnforcedGate(t *testing.T) {
	if label := DeriveEnforcement(RDDModeOn, RDDControllerCertifiedCurrent, RDDDeliveryGateEnforcedCurrent); label != RDDEnforcementManaged {
		t.Fatalf("the one reserved cell derived as %q, want managed", label)
	}
	for _, controller := range rddControllerStatesForMatrix {
		for _, gate := range rddDeliveryGateStatesForMatrix {
			label := DeriveEnforcement(RDDModeOn, controller, gate)
			reserved := controller == RDDControllerCertifiedCurrent && gate == RDDDeliveryGateEnforcedCurrent
			if reserved != (label == RDDEnforcementManaged) {
				t.Fatalf("DeriveEnforcement(on, %q, %q) = %q; managed must be reserved for certified_current controller with enforced_current gate",
					controller, gate, label)
			}
		}
	}
}

// TestDeriveEnforcementAdvisoryNeedsCurrentGateAndWrittenController pins the
// advisory row: an advisory delivery gate is reported only while a controller
// that is at least written can back it. A gate that is merely declared cannot
// report a posture nothing enforces yet, and stale, revoked, inconclusive, or
// failed facts never escalate past available.
func TestDeriveEnforcementAdvisoryNeedsCurrentGateAndWrittenController(t *testing.T) {
	for _, controller := range rddControllerStatesForMatrix {
		for _, gate := range rddDeliveryGateStatesForMatrix {
			label := DeriveEnforcement(RDDModeOn, controller, gate)
			wantAdvisory := gate == RDDDeliveryGateAdvisoryCurrent &&
				(controller == RDDControllerWritten || controller == RDDControllerCertifiedCurrent)
			if wantAdvisory != (label == RDDEnforcementAdvisory) {
				t.Fatalf("DeriveEnforcement(on, %q, %q) = %q; advisory must require an advisory_current gate backed by a written controller",
					controller, gate, label)
			}
		}
	}
}

// TestDeriveEnforcementFailsClosedOnUnknownFacts is the tampered-record row of
// the matrix: a controller or gate value this build does not recognize — a
// hand-edited record, or one written by a future build — derives available,
// never managed and never advisory. The derivation may only escalate on facts
// it can read exactly.
func TestDeriveEnforcementFailsClosedOnUnknownFacts(t *testing.T) {
	unknownControllers := []RDDControllerState{"", "certified", "written_current", "enforced", "current", "managed"}
	unknownGates := []RDDDeliveryGateState{"", "advisory", "enforced", "advisory_stale", "certified_current"}
	for _, controller := range unknownControllers {
		for _, gate := range rddDeliveryGateStatesForMatrix {
			if label := DeriveEnforcement(RDDModeOn, controller, gate); label != RDDEnforcementAvailable {
				t.Fatalf("unknown controller %q with gate %q derived %q, want available: unrecognized facts never escalate",
					controller, gate, label)
			}
		}
	}
	for _, controller := range rddControllerStatesForMatrix {
		for _, gate := range unknownGates {
			if label := DeriveEnforcement(RDDModeOn, controller, gate); label != RDDEnforcementAvailable {
				t.Fatalf("controller %q with unknown gate %q derived %q, want available: unrecognized facts never escalate",
					controller, gate, label)
			}
		}
	}
}

// TestResolveRDDEnforcementProjectsStructurallyAbsentFacts pins the S1
// honesty rule: until the controller extents (S2), the certification records
// (S3, blocked on the runtime oracle), and the delivery boundary (S4) exist,
// the resolver reports both facts as absent and never derives managed or
// advisory. The projection is a contract, not a promise about machinery that
// has not landed, so absent-certified can never yield managed by construction.
func TestResolveRDDEnforcementProjectsStructurallyAbsentFacts(t *testing.T) {
	modes := []RDDModeStatus{
		{Schema: RDDModeStatusSchema, Effective: RDDModeOn, Source: RDDModeSourceDefault},
		{Schema: RDDModeStatusSchema, Effective: RDDModeOn, Source: RDDModeSourceGlobal, Global: RDDModeOn},
		{Schema: RDDModeStatusSchema, Effective: RDDModeOff, Source: RDDModeSourceGlobal, Global: RDDModeOff},
		{Schema: RDDModeStatusSchema, Effective: RDDModeOff, Source: RDDModeSourceCloneLocal, CloneLocal: RDDModeOff},
		failedClosedRDDModeStatus(RDDModeSourceGlobal),
	}
	for _, mode := range modes {
		enforcement := ResolveRDDEnforcement(mode)
		if enforcement.Controller != RDDControllerAbsent || enforcement.DeliveryGate != RDDDeliveryGateAbsent {
			t.Fatalf("resolver invented facts %#v for mode %#v; S1 reports both as structurally absent", enforcement, mode)
		}
		want := RDDEnforcementAvailable
		if !mode.Enabled() {
			want = RDDEnforcementOff
		}
		if enforcement.Enforcement != want {
			t.Fatalf("resolver derived %q for mode %#v, want %q", enforcement.Enforcement, mode, want)
		}
	}
}
