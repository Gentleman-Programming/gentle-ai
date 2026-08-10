package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// withInstallRoot isolates the slice 1 probe from ambient state by
// pointing GENTLE_AI_INSTALL_ROOT at the supplied directory for the
// duration of the test. It also clears the variable so a leftover value
// from the parent process cannot leak into the probe.
func withInstallRoot(t *testing.T, root string) {
	t.Helper()
	t.Setenv("GENTLE_AI_INSTALL_ROOT", root)
}

// TestDeriveEnforcementAcceptanceControls walks every acceptance control
// listed in the #1842 issue body and asserts the documented label. The
// table is exhaustive over the 4-value public surface (Off, Available,
// Advisory, Managed) and over every Controller / DeliveryGate state that
// participates in fail-closed precedence.
func TestDeriveEnforcementAcceptanceControls(t *testing.T) {
	cases := []struct {
		name       string
		mode       RDDMode
		controller ControllerState
		gate       DeliveryGateState
		want       Enforcement
	}{
		// Acceptance control: mode on plus routing/controller files
		// only -> available.
		{
			name:       "on plus written controller plus absent gate -> available",
			mode:       RDDModeOn,
			controller: ControllerStateWritten,
			gate:       DeliveryGateStateAbsent,
			want:       EnforcementAvailable,
		},
		// Acceptance control: current OpenCode controller
		// certification and no gate -> advisory.
		{
			name:       "on plus certified_current controller plus absent gate -> advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateAbsent,
			want:       EnforcementAdvisory,
		},
		{
			name:       "on plus certified_current controller plus advisory_current gate -> advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateAdvisoryCurrent,
			want:       EnforcementAdvisory,
		},
		// Acceptance control: stale / revoked / inconclusive /
		// failed controller results downgrade immediately.
		{
			name:       "stale controller fails closed to available",
			mode:       RDDModeOn,
			controller: ControllerStateStale,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementAvailable,
		},
		{
			name:       "revoked controller fails closed to available",
			mode:       RDDModeOn,
			controller: ControllerStateRevoked,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementAvailable,
		},
		{
			name:       "inconclusive controller fails closed to available",
			mode:       RDDModeOn,
			controller: ControllerStateInconclusive,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementAvailable,
		},
		{
			name:       "failed controller fails closed to available",
			mode:       RDDModeOn,
			controller: ControllerStateFailed,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementAvailable,
		},
		{
			name:       "absent controller fails closed to available",
			mode:       RDDModeOn,
			controller: ControllerStateAbsent,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementAvailable,
		},
		// Acceptance control: local bypassable hook cannot produce
		// managed. AdvisoryCurrent covers a local hook with
		// --no-verify; the other non-enforced states mirror the
		// controller fail-closed ladder.
		{
			name:       "stale gate fails closed to advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateStale,
			want:       EnforcementAdvisory,
		},
		{
			name:       "revoked gate fails closed to advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateRevoked,
			want:       EnforcementAdvisory,
		},
		{
			name:       "inconclusive gate fails closed to advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateInconclusive,
			want:       EnforcementAdvisory,
		},
		{
			name:       "failed gate fails closed to advisory",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateFailed,
			want:       EnforcementAdvisory,
		},
		// Acceptance control: current controller plus current exact
		// repository / ref gate -> managed.
		{
			name:       "on plus certified_current controller plus enforced_current gate -> managed",
			mode:       RDDModeOn,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementManaged,
		},
		// Kill-switch off wins regardless of the other two
		// dimensions; preserves existing reviewer-mode behavior.
		{
			name:       "off plus certified_current controller plus enforced_current gate -> off",
			mode:       RDDModeOff,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementOff,
		},
		{
			name:       "unset mode plus strongest possible state -> off",
			mode:       RDDModeUnset,
			controller: ControllerStateCertifiedCurrent,
			gate:       DeliveryGateStateEnforcedCurrent,
			want:       EnforcementOff,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveEnforcement(tc.mode, tc.controller, tc.gate)
			if got != tc.want {
				t.Fatalf("DeriveEnforcement(%q, %q, %q) = %q, want %q", tc.mode, tc.controller, tc.gate, got, tc.want)
			}
		})
	}
}

// TestProbeRDDControllerReportsWrittenWhenContractInstalled pins the
// slice 1 ceiling: file presence of the canonical review-ledger
// contract is the strongest observation the probe can produce.
func TestProbeRDDControllerReportsWrittenWhenContractInstalled(t *testing.T) {
	root := t.TempDir()
	contractDir := filepath.Join(root, filepath.Dir(reviewLedgerContractPath))
	if err := os.MkdirAll(contractDir, 0o755); err != nil {
		t.Fatalf("setup: mkdir contract dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, reviewLedgerContractPath), []byte("# contract\n"), 0o644); err != nil {
		t.Fatalf("setup: write contract: %v", err)
	}
	withInstallRoot(t, root)

	ctrl, err := ProbeRDDController(context.Background(), "")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if ctrl.State != ControllerStateWritten {
		t.Fatalf("controller state = %q, want %q", ctrl.State, ControllerStateWritten)
	}
	if ctrl.Reason != "" {
		t.Fatalf("controller reason = %q, want empty (written is the strongest slice 1 observation)", ctrl.Reason)
	}
}

// TestProbeRDDControllerReportsAbsentWhenContractMissing is the
// acceptance control "mode on plus routing/controller files only ->
// available"; absent file plus absent gate yields available, which the
// CLI status consumer asserts end to end.
func TestProbeRDDControllerReportsAbsentWhenContractMissing(t *testing.T) {
	root := t.TempDir()
	withInstallRoot(t, root)

	ctrl, err := ProbeRDDController(context.Background(), "")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if ctrl.State != ControllerStateAbsent {
		t.Fatalf("controller state = %q, want %q", ctrl.State, ControllerStateAbsent)
	}
	if ctrl.Reason == "" {
		t.Fatalf("controller reason = empty, want explanation of why absent")
	}
}

// TestProbeRDDControllerReportsInconclusiveWhenNoRootDerivable covers
// the rule that zero applicable probes is inconclusive, never success.
func TestProbeRDDControllerReportsInconclusiveWhenNoRootDerivable(t *testing.T) {
	withInstallRoot(t, "")
	t.Setenv("GENTLE_AI_INSTALL_ROOT", "")

	ctrl, err := ProbeRDDController(context.Background(), "")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if ctrl.State != ControllerStateInconclusive {
		t.Fatalf("controller state = %q, want %q", ctrl.State, ControllerStateInconclusive)
	}
	if ctrl.Reason == "" {
		t.Fatalf("controller reason = empty, want explanation of inconclusive probe")
	}
}

// TestProbeRDDControllerReportsFailedWhenContractPathIsDirectory
// guards against a silent misclassification of a directory at the
// contract path.
func TestProbeRDDControllerReportsFailedWhenContractPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, reviewLedgerContractPath), 0o755); err != nil {
		t.Fatalf("setup: mkdir at contract path: %v", err)
	}
	withInstallRoot(t, root)

	ctrl, err := ProbeRDDController(context.Background(), "")
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if ctrl.State != ControllerStateFailed {
		t.Fatalf("controller state = %q, want %q", ctrl.State, ControllerStateFailed)
	}
}

// TestProbeRDDDeliveryGateReportsAbsentSlice1 pins the slice 1 gate
// ceiling: no non-bypassable boundary is certified in slice 1, so the
// probe must always return Absent regardless of repo or environment.
func TestProbeRDDDeliveryGateReportsAbsentSlice1(t *testing.T) {
	gate, err := ProbeRDDDeliveryGate(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if gate.State != DeliveryGateStateAbsent {
		t.Fatalf("gate state = %q, want %q", gate.State, DeliveryGateStateAbsent)
	}
	if gate.Reason == "" {
		t.Fatalf("gate reason = empty, want slice 1 marker")
	}
}

// TestProbesRespectContextCancellation ensures neither probe blocks on
// a cancelled context. Both must return an Inconclusive / Failed state
// plus the context error so callers can fail closed at the call site.
func TestProbesRespectContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if ctrl, err := ProbeRDDController(ctx, ""); err == nil {
		t.Fatalf("expected cancelled context to surface as probe error; got state=%q err=nil", ctrl.State)
	} else if ctrl.State != ControllerStateInconclusive {
		t.Fatalf("controller state on cancelled ctx = %q, want %q", ctrl.State, ControllerStateInconclusive)
	}

	if gate, err := ProbeRDDDeliveryGate(ctx, ""); err == nil {
		t.Fatalf("expected cancelled context to surface as probe error; got state=%q err=nil", gate.State)
	} else if gate.State != DeliveryGateStateInconclusive {
		t.Fatalf("gate state on cancelled ctx = %q, want %q", gate.State, DeliveryGateStateInconclusive)
	}
}
