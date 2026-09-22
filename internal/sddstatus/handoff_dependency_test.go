package sddstatus

import (
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

// sealPerCheckpointFullstackKickoff mirrors sealFullstackKickoff
// (status_test.go) but seals handoff_policy: per_checkpoint instead of
// none — the exact D-12 trigger this file's tests exercise, independent
// of roster size.
func sealPerCheckpointFullstackKickoff(t *testing.T, changeRoot, changeName string, style kickoff.ExecutionStyle) {
	t.Helper()
	_, ok, err := kickoff.Seal(changeRoot, kickoff.Kickoff{
		Schema: kickoff.KickoffSchemaV1, Change: changeName, SealedBy: "test",
		Config: kickoff.FlowConfig{
			FlowMode: kickoff.FlowSDD, ExecutionStyle: style, ExecutionStyleSource: "explicit",
			HandoffPolicy: kickoff.HandoffPerCheckpoint,
			Roles:         []kickoff.KickoffRole{{Role: "fullstack", GatePolicy: "blocking", TasksFile: "tasks.md", VerifyFile: "verify-report.md"}},
		},
		Lifecycle: kickoff.Lifecycle{DeploymentTarget: "local", PostArchivePolicy: "bug_only"},
	})
	if err != nil || !ok {
		t.Fatalf("sealPerCheckpointFullstackKickoff: kickoff.Seal() = (ok=%v, err=%v), want a successful preparation seal", ok, err)
	}
}

func writeIntegrationHandoffFixture(t *testing.T, changeRoot string, status handoff.Status) {
	t.Helper()
	h := &handoff.Handoff{
		Metadata: handoff.Metadata{
			Change: "governed-handoff", FromPhase: handoff.PhaseApply, ToPhase: handoff.PhaseVerify,
			FromRole: "fullstack", ToRole: "fullstack", Timestamp: time.Now().UTC(), Status: status,
		},
		Sections: handoff.Sections{
			ExecutiveSummary: "resumen", Artifacts: "artefactos", Decisions: "decisiones",
			RisksAndBlockers: "riesgos", DirectInstructions: "instrucciones",
		},
	}
	if err := handoff.WriteFile(changeRoot+"/handoff.md", h); err != nil {
		t.Fatalf("handoff.WriteFile() error = %v", err)
	}
}

// TestVerifyDependencyReadyWhenHandoffReadyToVerify is task 18.3's first
// case: with handoff.md in status ready and to_phase verify,
// dependencies.Verify becomes ready.
func TestVerifyDependencyReadyWhenHandoffReadyToVerify(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-handoff"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealPerCheckpointFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	writeIntegrationHandoffFixture(t, changeRoot, handoff.StatusReady)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyReady {
		t.Fatalf("Dependencies.Verify = %q, want ready (handoff.md status ready, to_phase verify)", status.Dependencies.Verify)
	}
}

// TestVerifyDependencyBlockedWhenHandoffBlocked is task 18.3's second case:
// handoff.md in status blocked forces dependencies.Verify to blocked, with
// a genuine reason citing the read state.
func TestVerifyDependencyBlockedWhenHandoffBlocked(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-handoff"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealPerCheckpointFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	writeIntegrationHandoffFixture(t, changeRoot, handoff.StatusBlocked)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyBlocked {
		t.Fatalf("Dependencies.Verify = %q, want blocked (handoff.md status blocked)", status.Dependencies.Verify)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "blocked") {
		t.Fatalf("BlockedReasons = %v, want a genuine reason citing the handoff's blocked status", status.BlockedReasons)
	}
}

// TestVerifyDependencyUnaffectedWhenHandoffAbsentAndRoleStillOpen is task
// 18.3's third case, non-blocking half: an absent handoff.md is treated as
// "no relay yet", not a block, when the role has not closed yet either.
func TestVerifyDependencyUnaffectedWhenHandoffAbsentAndRoleStillOpen(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-handoff"
	changeRoot := seedReadyChange(t, root, changeName, "- [ ] 1.1 Wire routes\n")
	sealPerCheckpointFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	// No handoff.md written, and no role-apply gate approved: the role has
	// not closed, so the absence of a relay is simply "not produced yet".

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyReady {
		t.Fatalf("Dependencies.Verify = %q, want ready (baseline unaffected: core artifacts are done and the handoff's absence must not block on its own)", status.Dependencies.Verify)
	}
}

// TestVerifyDependencyBlockedWhenHandoffAbsentButLastRoleAlreadyClosed is
// task 18.3's third case, blocking half: once every role-apply gate is
// already approved (LastRoleClosed true), a MISSING handoff.md is a
// genuine anomaly — the relay should exist by now — and blocks Verify
// instead of silently reporting "ready".
func TestVerifyDependencyBlockedWhenHandoffAbsentButLastRoleAlreadyClosed(t *testing.T) {
	root := t.TempDir()
	changeName := "governed-handoff"
	changeRoot := seedReadyChange(t, root, changeName, "- [x] 1.1 Wire routes\n")
	sealPerCheckpointFullstackKickoff(t, changeRoot, changeName, kickoff.ExecutionCheckpointed)
	if err := kickoff.AppendGate(changeRoot, kickoff.GateRecord{
		Gate: kickoff.RoleApplyGate("fullstack"), Decision: kickoff.DecisionApproved,
		Reason: "implementacion completa", Actor: "maintainer",
	}); err != nil {
		t.Fatalf("kickoff.AppendGate() error = %v", err)
	}
	// handoff.md deliberately NOT written, simulating a caller that closed
	// the last role without producing the expected relay.

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyBlocked {
		t.Fatalf("Dependencies.Verify = %q, want blocked (last role closed but handoff.md is missing)", status.Dependencies.Verify)
	}
}
