package reviewtransaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// approvedCurrentChangesDivergedBaseFixture reproduces issue #1376: a
// current-changes review approved on a feature branch, delivered byte-identical
// in one commit, while the publication base advances with a disjoint change.
func approvedCurrentChangesDivergedBaseFixture(t *testing.T, lineage string) (string, CompactState, CompactReceipt, string) {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "checkout", "-qb", "feature")
	gitSnapshot(t, repo, "config", "branch.feature.remote", "origin")
	gitSnapshot(t, repo, "config", "branch.feature.merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
	gitSnapshot(t, repo, "checkout", "-q", branch)
	writeSnapshotFile(t, repo, "base-only.txt", "base advance\n")
	gitSnapshot(t, repo, "add", "base-only.txt")
	gitSnapshot(t, repo, "commit", "-m", "advance publication base")
	gitSnapshot(t, repo, "push", "-q", "origin", branch)
	gitSnapshot(t, repo, "checkout", "-q", "feature")
	return repo, state, receipt, "origin/" + branch
}

func TestCompactPrePRGateReconcilesApprovedCurrentChangesReceiptAcrossDivergedBase(t *testing.T) {
	repo, state, receipt, baseRef := approvedCurrentChangesDivergedBaseFixture(t, "compact-current-diverged-base")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateAllow {
		t.Fatalf("diverged-base current-changes pre-PR = %#v", got)
	}
	if got.Context.BaseAdvance == nil || !got.Context.BaseAdvance.Compatible || got.Context.BaseAdvance.Status != "current-changes-boundary-compatible" {
		t.Fatalf("boundary compatibility proof = %#v", got.Context.BaseAdvance)
	}
	if got.Context.BaseAdvance.CIAttestationArtifactHash != "" || got.Context.BaseAdvance.CIAttestationIssuer != "" {
		t.Fatalf("boundary compatibility must not claim CI attestation = %#v", got.Context.BaseAdvance)
	}
	payload, err := json.Marshal(got.Context)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseGateContext(payload)
	if err != nil || !reflect.DeepEqual(parsed, got.Context) {
		t.Fatalf("reconciled pre-PR context round trip = %#v, %v", parsed, err)
	}
}

func TestCompactPrePRAssessmentTreatsReconcilableCurrentChangesReceiptAsExact(t *testing.T) {
	repo, state, _, baseRef := approvedCurrentChangesDivergedBaseFixture(t, "compact-current-diverged-assess")
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := AssessCompactGateTarget(context.Background(), repo, record.State, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Applicability != CompactGateTargetExact {
		t.Fatalf("reconcilable current-changes applicability = %q; actual=%#v", assessment.Applicability, assessment.Actual)
	}
}

func TestCompactPrePRGateFailsClosedWhenDeliveredContentDrifts(t *testing.T) {
	repo, state, receipt, baseRef := approvedCurrentChangesDivergedBaseFixture(t, "compact-current-diverged-drift")
	writeSnapshotFile(t, repo, "tracked.txt", "unreviewed drift\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "unreviewed drift")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateScopeChanged {
		t.Fatalf("drifted current-changes pre-PR = %#v, want scope changed", got)
	}
}

func TestCompactPrePRGateFailsClosedForMultipleDeliveryCommits(t *testing.T) {
	repo, state, receipt, baseRef := approvedCurrentChangesDivergedBaseFixture(t, "compact-current-diverged-commits")
	writeSnapshotFile(t, repo, "tracked.txt", "temporary unreviewed bytes\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "temporary delivery")
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "restore approved candidate")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateScopeChanged {
		t.Fatalf("multiple delivery commits pre-PR = %#v, want scope changed", got)
	}
}

func TestCompactPrePRGateFailsClosedWhenPublicationRangeExceedsGenesis(t *testing.T) {
	repo, state, receipt, baseRef := approvedCurrentChangesDivergedBaseFixture(t, "compact-current-diverged-range")
	writeSnapshotFile(t, repo, "secret.txt", "must never be published\n")
	gitSnapshot(t, repo, "add", "secret.txt")
	gitSnapshot(t, repo, "commit", "-m", "intermediate secret")
	if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "remove intermediate secret")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateScopeChanged {
		t.Fatalf("hidden publication-range path pre-PR = %#v, want scope changed", got)
	}
}

func TestCompactPrePRGateFailsClosedForEmptyGenesisSelfDiffReceipt(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "checkout", "-qb", "feature")
	gitSnapshot(t, repo, "config", "branch.feature.remote", "origin")
	gitSnapshot(t, repo, "config", "branch.feature.merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "unreviewed delivery\n")
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "unreviewed delivery")
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-empty-genesis-self-diff", []string{})
	if len(state.GenesisPaths) != 0 || state.InitialSnapshot.BaseTree != state.InitialSnapshot.CandidateTree {
		t.Fatalf("fixture expected an empty self-diff review, got paths=%v", state.GenesisPaths)
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch})
	if got.Result != GateScopeChanged {
		t.Fatalf("empty-genesis self-diff pre-PR = %#v, want scope changed", got)
	}
}
