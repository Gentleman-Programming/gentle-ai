package reviewtransaction

// #1842 three-fact enforcement contract.
//
// The public enforcement label is derived from three independent facts that
// never overwrite one another:
//
//   - policy: the kill switch this package already owns (off|on);
//   - controller: the installed always-loaded controller contract
//     (absent|declared|written|certified_current|stale|revoked|inconclusive|failed);
//   - delivery_gate: the delivery boundary the operator sees
//     (absent|advisory_current|enforced_current|stale|revoked|inconclusive|failed).
//
// Ownership is the point of the split: the mode store owns policy, the
// managed-resource identity machinery owns the installed controller and gate
// extents, and the runtime certification records own promotion to current.
// No source of truth is added here; this file only names the facts, derives
// the public label from them, and fails closed whenever a fact cannot be
// read exactly.

// RDDControllerState is the controller fact: how far an always-loaded
// controller contract stands on this installation.
type RDDControllerState string

const (
	// RDDControllerAbsent means no controller contract is installed or declared.
	RDDControllerAbsent RDDControllerState = "absent"
	// RDDControllerDeclared means configuration declares a controller that is
	// not installed. A declaration alone backs no enforcement posture.
	RDDControllerDeclared RDDControllerState = "declared"
	// RDDControllerWritten means the controller contract is installed on disk.
	// Installation is observable; its freshness is not asserted here.
	RDDControllerWritten RDDControllerState = "written"
	// RDDControllerCertifiedCurrent means a runtime certification record
	// proves the installed controller is the current one. It is reachable only
	// once certification records exist.
	RDDControllerCertifiedCurrent RDDControllerState = "certified_current"
	// RDDControllerStale means the installed controller is proven out of date.
	RDDControllerStale RDDControllerState = "stale"
	// RDDControllerRevoked means the controller contract was withdrawn.
	RDDControllerRevoked RDDControllerState = "revoked"
	// RDDControllerInconclusive means the evidence about the controller
	// contradicts itself or cannot be ordered; it is not a soft absent.
	RDDControllerInconclusive RDDControllerState = "inconclusive"
	// RDDControllerFailed means reading the controller fact failed. It never
	// fails open into a stronger state.
	RDDControllerFailed RDDControllerState = "failed"
)

// RDDDeliveryGateState is the delivery-gate fact: what stands between
// finished work and delivery.
type RDDDeliveryGateState string

const (
	// RDDDeliveryGateAbsent means no delivery gate is installed.
	RDDDeliveryGateAbsent RDDDeliveryGateState = "absent"
	// RDDDeliveryGateAdvisoryCurrent means an advisory gate is current: it
	// reports on delivery without blocking it.
	RDDDeliveryGateAdvisoryCurrent RDDDeliveryGateState = "advisory_current"
	// RDDDeliveryGateEnforcedCurrent means a current gate refuses delivery the
	// operator has not cleared. Only a non-bypassable repository/ref boundary
	// may ever be reported this way.
	RDDDeliveryGateEnforcedCurrent RDDDeliveryGateState = "enforced_current"
	// RDDDeliveryGateStale means the gate is proven out of date.
	RDDDeliveryGateStale RDDDeliveryGateState = "stale"
	// RDDDeliveryGateRevoked means the gate was withdrawn.
	RDDDeliveryGateRevoked RDDDeliveryGateState = "revoked"
	// RDDDeliveryGateInconclusive means the gate evidence contradicts itself
	// or cannot be ordered.
	RDDDeliveryGateInconclusive RDDDeliveryGateState = "inconclusive"
	// RDDDeliveryGateFailed means reading the gate fact failed.
	RDDDeliveryGateFailed RDDDeliveryGateState = "failed"
)

// RDDEnforcementLabel is the public enforcement posture derived from the
// three facts. It is a report, never an authorization: nothing gates on it,
// and the kill switch keeps deciding reviews on its own.
type RDDEnforcementLabel string

const (
	// RDDEnforcementOff means the kill switch keeps receipt-driven development
	// off, so no enforcement posture stands on top of it.
	RDDEnforcementOff RDDEnforcementLabel = "off"
	// RDDEnforcementAvailable means reviews are available and no delivery gate
	// is proven current. It is the floor for an on switch, and the fail-closed
	// landing for every fact this build cannot read exactly.
	RDDEnforcementAvailable RDDEnforcementLabel = "available"
	// RDDEnforcementAdvisory means a current advisory gate reports on delivery
	// without blocking it, backed by a controller at least written.
	RDDEnforcementAdvisory RDDEnforcementLabel = "advisory"
	// RDDEnforcementManaged means a non-bypassable repository/ref boundary
	// refuses uncleared delivery, certified current. It is reserved: only the
	// exact pair certified_current controller and enforced_current gate may
	// claim it, proven by positive and negative native tests.
	RDDEnforcementManaged RDDEnforcementLabel = "managed"
)

// RDDEnforcementStatus is the enforcement projection `review mode status`
// reports beside the mode projection. Both facts and the label are always
// populated; absent is a stated fact, never a missing field.
type RDDEnforcementStatus struct {
	Controller   RDDControllerState   `json:"controller"`
	DeliveryGate RDDDeliveryGateState `json:"delivery_gate"`
	Enforcement  RDDEnforcementLabel  `json:"enforcement"`
}

// DeriveEnforcement derives the public label from the three facts. It is pure
// on purpose: the matrix is the contract, and every rule below is the
// fail-closed direction the issue approved.
//
// A policy that is not exactly on reports off, mirroring the mode store's own
// any-off-wins posture; nothing stands on top of a disabled switch. Managed
// requires the exact certified controller and enforced gate pair — an
// enforced gate backed by a merely written controller is not yet provably
// non-bypassable, so it lands on the available floor with its facts still
// visible beside the label. Advisory requires a current advisory gate that a
// controller at least written can back, because a gate that only configuration
// promises is a posture nothing enforces yet.
//
// Every other combination — stale, revoked, inconclusive, failed, absent, or
// a value this build does not recognize at all — derives available. Escalating
// on unreadable or tampered facts is the one defect this function exists to
// make impossible: #3284 is what a partial truth reported as a working switch
// looks like, and RDDModeReach already documents that lesson for the write
// side. This is the read side of the same discipline.
func DeriveEnforcement(
	policy RDDMode,
	controller RDDControllerState,
	gate RDDDeliveryGateState,
) RDDEnforcementLabel {
	if policy != RDDModeOn {
		return RDDEnforcementOff
	}
	if controller == RDDControllerCertifiedCurrent && gate == RDDDeliveryGateEnforcedCurrent {
		return RDDEnforcementManaged
	}
	if gate == RDDDeliveryGateAdvisoryCurrent &&
		(controller == RDDControllerWritten || controller == RDDControllerCertifiedCurrent) {
		return RDDEnforcementAdvisory
	}
	return RDDEnforcementAvailable
}

// ResolveRDDEnforcement projects the enforcement facts for a resolved mode.
//
// S1 reads nothing new: no controller extents, no certification records, and
// no delivery boundary exist yet, so both facts are structurally absent and
// the label can only be off or available. That is the honesty rule, not a
// placeholder to be papered over: absent-certified never yields managed, and
// `managed` stays unreachable by construction until the slices that own the
// facts land with their own proof. Codex adapters stay `available` under this
// projection until a trustworthy loader oracle exists.
func ResolveRDDEnforcement(mode RDDModeStatus) RDDEnforcementStatus {
	controller := rddControllerFact()
	gate := rddDeliveryGateFact()
	return RDDEnforcementStatus{
		Controller:   controller,
		DeliveryGate: gate,
		Enforcement:  DeriveEnforcement(mode.Effective, controller, gate),
	}
}

// rddControllerFact reads the installed controller fact from the
// managed-resource identity machinery that owns the extents. S1 returns
// structurally absent: the always-loaded controller contract is a later
// slice, and inventing an extent read here would add a source of truth the
// issue explicitly forbids.
func rddControllerFact() RDDControllerState { return RDDControllerAbsent }

// rddDeliveryGateFact reads the delivery-gate fact. S1 returns structurally
// absent: the enforced repository/ref boundary arrives with positive and
// negative native tests, and until then no gate may be reported current.
func rddDeliveryGateFact() RDDDeliveryGateState { return RDDDeliveryGateAbsent }
