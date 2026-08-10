package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ControllerState names the current observed reachability of the review
// controller contract for the active agent. The values form a fail-closed
// ladder: a stronger state never implies a weaker one, and any
// inconclusive / stale / revoked / failed reading forces DeriveEnforcement
// to refuse the stronger labels.
//
// Slice 1 of #1842 reports only Absent or Written; CertifiedCurrent is
// reserved for slice 2 once the #1873 evidence registry lands.
type ControllerState string

const (
	// ControllerStateAbsent — no controller contract installed for the
	// active agent.
	ControllerStateAbsent ControllerState = "absent"
	// ControllerStateWritten — the controller contract bytes exist on
	// disk but no runtime loader has confirmed the active client consumed
	// them. Slice 1 ceiling for Codex / Darwin arm64.
	ControllerStateWritten ControllerState = "written"
	// ControllerStateCertifiedCurrent — the runtime certification
	// registry (#1873) confirms an exact client consumed an exact
	// controller projection. Required for EnforcementManaged; not
	// reachable in slice 1.
	ControllerStateCertifiedCurrent ControllerState = "certified_current"
	// ControllerStateStale — content, effective config, or resource
	// digest changed after certification. Fail closed.
	ControllerStateStale ControllerState = "stale"
	// ControllerStateRevoked — controller removed, unsafe, or
	// explicitly retired. Fail closed.
	ControllerStateRevoked ControllerState = "revoked"
	// ControllerStateInconclusive — missing or unreadable evidence,
	// unknown schema, or insufficient credentials to probe. Fail closed.
	ControllerStateInconclusive ControllerState = "inconclusive"
	// ControllerStateFailed — probe ran and produced a typed failure
	// recorded in the evidence registry. Fail closed.
	ControllerStateFailed ControllerState = "failed"
)

// DeliveryGateState names the current reachability of a non-bypassable
// delivery gate (e.g. a GitHub required-receipt ruleset). It is the
// delivery-side counterpart of ControllerState.
//
// Slice 1 of #1842 reports only Absent for every workspace; slice 3 adds
// the certified non-bypassable boundary tests.
type DeliveryGateState string

const (
	// DeliveryGateStateAbsent — no delivery gate is installed or
	// observable. Slice 1 default.
	DeliveryGateStateAbsent DeliveryGateState = "absent"
	// DeliveryGateStateAdvisoryCurrent — a local hook or prompt
	// requests receipts but cannot reject unreviewed bytes on its own.
	// Never strong enough for EnforcementManaged.
	DeliveryGateStateAdvisoryCurrent DeliveryGateState = "advisory_current"
	// DeliveryGateStateEnforcedCurrent — an exact repository / ref
	// boundary is certified to reject unreviewed delivery. Required
	// for EnforcementManaged.
	DeliveryGateStateEnforcedCurrent DeliveryGateState = "enforced_current"
	// DeliveryGateStateStale — ruleset / workflow / receipt-subject
	// drift downgrades fail closed.
	DeliveryGateStateStale DeliveryGateState = "stale"
	// DeliveryGateStateRevoked — required check removed or renamed;
	// the previous certified boundary no longer applies.
	DeliveryGateStateRevoked DeliveryGateState = "revoked"
	// DeliveryGateStateInconclusive — unreadable ruleset state,
	// unknown schema, or insufficient credentials to probe.
	DeliveryGateStateInconclusive DeliveryGateState = "inconclusive"
	// DeliveryGateStateFailed — gate probe ran and produced a typed
	// failure. Fail closed.
	DeliveryGateStateFailed DeliveryGateState = "failed"
)

// Enforcement is the public, derived label of the three-dimension state.
// It is always derivable from Mode + Controller + DeliveryGate; callers
// must not set it directly. DeriveEnforcement owns the precedence.
//
// The four values are the public contract requested by #1842: every
// implementation route must converge on one of them, and `mode:on` must
// no longer conflate availability with enforcement.
type Enforcement string

const (
	// EnforcementOff — the user kill switch is off; receipt-driven
	// development is not active regardless of controller or gate.
	EnforcementOff Enforcement = "off"
	// EnforcementAvailable — RDD is on, the CLI exists, and the agent
	// is not wired to invoke it. The strongest label slice 1 can
	// produce for any agent that has not yet been certified by the
	// #1873 evidence registry.
	EnforcementAvailable Enforcement = "available"
	// EnforcementAdvisory — the controller contract is certified
	// current but no non-bypassable delivery boundary is in place.
	// A local hook that can be bypassed with --no-verify is Advisory,
	// never Managed.
	EnforcementAdvisory Enforcement = "advisory"
	// EnforcementManaged — both the controller and an exact,
	// non-bypassable repository / ref delivery gate are certified
	// current. Slice 3 deliverable.
	EnforcementManaged Enforcement = "managed"
)

// RDDController is the observable projection of controller reachability
// for the active agent. It is appended to RDDModeStatus and is always
// populated by ProbeRDDController; the reason field is non-empty only
// when the state is not the strongest reachable value.
type RDDController struct {
	State  ControllerState `json:"state"`
	Reason string          `json:"reason,omitempty"`
}

// RDDDeliveryGate is the observable projection of the non-bypassable
// delivery gate reachability. It is appended to RDDModeStatus and is
// always populated by ProbeRDDDeliveryGate.
type RDDDeliveryGate struct {
	State  DeliveryGateState `json:"state"`
	Reason string            `json:"reason,omitempty"`
}

// DeriveEnforcement applies the fail-closed precedence defined in the
// acceptance controls of #1842. It is pure: no IO, no side effects, no
// clock. Every state that is not the strongest reachable value forces a
// weaker label, so a stale / revoked / inconclusive / failed reading can
// never accidentally produce Advisory or Managed.
//
// The precedence is:
//   - mode != on                              -> Off
//   - controller != certified_current         -> Available
//   - controller == certified_current
//     && gate != enforced_current             -> Advisory
//   - controller == certified_current
//     && gate == enforced_current             -> Managed
func DeriveEnforcement(mode RDDMode, controller ControllerState, gate DeliveryGateState) Enforcement {
	if mode != RDDModeOn {
		return EnforcementOff
	}
	if controller != ControllerStateCertifiedCurrent {
		return EnforcementAvailable
	}
	if gate != DeliveryGateStateEnforcedCurrent {
		return EnforcementAdvisory
	}
	return EnforcementManaged
}

// reviewLedgerContractPath is the on-disk location of the canonical
// controller contract slice 1 inspects. It is intentionally the same
// path the install pipeline materialises, so file-presence probing
// stays consistent with what the user actually has on disk.
const reviewLedgerContractPath = "internal/assets/skills/_shared/review-ledger-contract.md"

// ProbeRDDController is the slice 1 reachability probe for the active
// agent's controller contract. It only inspects file presence of the
// canonical review-ledger contract — runtime certification (slice 2+
// via the #1873 evidence registry) is intentionally out of scope.
//
// The probe never claims CertifiedCurrent: a file on disk is the
// strongest slice 1 observation. A missing install root or unreadable
// filesystem fails closed to Inconclusive so callers can distinguish
// "we have no view" from "we have a view that says no".
func ProbeRDDController(ctx context.Context, repo string) (RDDController, error) {
	if err := ctx.Err(); err != nil {
		return RDDController{State: ControllerStateInconclusive, Reason: "probe context cancelled"}, err
	}
	installRoot, err := locateInstallRoot(repo)
	if err != nil {
		return RDDController{State: ControllerStateInconclusive, Reason: err.Error()}, nil
	}
	contractPath := filepath.Join(installRoot, reviewLedgerContractPath)
	info, statErr := os.Stat(contractPath)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return RDDController{State: ControllerStateAbsent, Reason: "review-ledger contract not installed"}, nil
		}
		return RDDController{State: ControllerStateFailed, Reason: statErr.Error()}, nil
	}
	if info.IsDir() {
		return RDDController{State: ControllerStateFailed, Reason: "review-ledger contract path is a directory, not a file"}, nil
	}
	return RDDController{State: ControllerStateWritten}, nil
}

// ProbeRDDDeliveryGate is the slice 1 reachability probe for a
// non-bypassable delivery gate. Slice 1 has no certified non-bypassable
// boundary in this repository; the probe therefore returns Absent for
// every workspace. Slice 3 will replace the body once the GitHub
// required-receipt capability lands.
//
// The signature is stable so callers do not need to change when slice 3
// swaps in the real probe.
func ProbeRDDDeliveryGate(ctx context.Context, repo string) (RDDDeliveryGate, error) {
	if err := ctx.Err(); err != nil {
		return RDDDeliveryGate{State: DeliveryGateStateInconclusive, Reason: "probe context cancelled"}, err
	}
	_ = repo
	return RDDDeliveryGate{State: DeliveryGateStateAbsent, Reason: "no non-bypassable delivery gate certified for slice 1"}, nil
}

// locateInstallRoot resolves the directory in which the canonical
// review-ledger contract should live. It prefers an explicit install
// root supplied through the environment (GENTLE_AI_INSTALL_ROOT) and
// falls back to repo. The fallback is intentionally narrow: slice 1
// only needs to inspect file presence, so an environment override is
// enough to support developer-machine layouts without conflating the
// repository working tree with the install footprint.
func locateInstallRoot(repo string) (string, error) {
	if root := strings.TrimSpace(os.Getenv("GENTLE_AI_INSTALL_ROOT")); root != "" {
		return root, nil
	}
	if strings.TrimSpace(repo) == "" {
		return "", errors.New("install root not derivable: set GENTLE_AI_INSTALL_ROOT or pass a non-empty repo") // refusal:by-design human-authority: the probe requires either an explicit GENTLE_AI_INSTALL_ROOT or a non-empty repo argument; both are caller-supplied, not a command this refusal can name
	}
	return repo, nil
}

// String renders the failure reason safely. It exists so callers can log
// the projection without leaking the raw reason string into user-facing
// messages by accident.
func (r RDDController) String() string {
	return fmt.Sprintf("controller=%s reason=%q", r.State, r.Reason)
}

// String renders the failure reason safely.
func (r RDDDeliveryGate) String() string {
	return fmt.Sprintf("delivery_gate=%s reason=%q", r.State, r.Reason)
}

// probeEnforcementDimensions runs the slice 1 controller and delivery
// gate probes once and returns the resulting projections. Errors are
// already encoded in the state of each return value (the probes cannot
// fail in a way the caller would handle differently), so the helper
// itself has no error return. Both probes share the same fail-closed
// semantics: a missing install root, a cancelled context, or an
// unreadable filesystem yields an Inconclusive state rather than an
// error, which is exactly the contract #1842 demands.
func probeEnforcementDimensions(ctx context.Context, repo string) (RDDController, RDDDeliveryGate) {
	controller, _ := ProbeRDDController(ctx, repo)
	gate, _ := ProbeRDDDeliveryGate(ctx, repo)
	return controller, gate
}
