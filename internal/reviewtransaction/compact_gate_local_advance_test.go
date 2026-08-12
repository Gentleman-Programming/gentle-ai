package reviewtransaction

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// #2388: an explicit parent base selects the same B1-to-M proof for a staged
// pre-commit merge and the committed pre-push merge. Neither path mints a new
// receipt; both retain the approved B0-to-C0 authority.
func TestCompactLocalBaseAdvanceAllowsExplicitParentMerge(t *testing.T) {
	tests := []struct {
		name      string
		gate      GateKind
		mergeArgs []string
		rawBase   bool
	}{
		{name: "staged pre-commit", gate: GatePreCommit, mergeArgs: []string{"merge", "main", "--no-commit", "--no-ff"}},
		{name: "staged pre-commit with raw advertised parent", gate: GatePreCommit, mergeArgs: []string{"merge", "main", "--no-commit", "--no-ff"}, rawBase: true},
		{name: "committed pre-push", gate: GatePrePush, mergeArgs: []string{"merge", "main", "--no-edit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
			state, receipt := approvedCompactPrePRFixture(t, fixture)
			gitSnapshot(t, fixture.repo, tt.mergeArgs...)
			baseRef := "main"
			if tt.rawBase {
				baseRef = trimGit(gitSnapshot(t, fixture.repo, "rev-parse", "main"))
			}
			got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
				Gate: tt.gate, LineageID: state.LineageID, BaseRef: baseRef,
			})
			if got.Result != GateAllow || got.Context.BaseAdvance == nil ||
				got.Context.BaseAdvance.MergedResultTree != got.Context.CandidateTree {
				t.Fatalf("explicit parent merge = %#v, want approved B0-to-C0 receipt to allow B1-to-M", got)
			}
		})
	}
}

func TestCompactLocalBaseAdvanceRejectsUnadvertisedExplicitParent(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)
	gitSnapshot(t, fixture.repo, "branch", "local-b1", fixture.originalBaseCommit)
	gitSnapshot(t, fixture.repo, "checkout", "local-b1")
	writeSnapshotFile(t, fixture.repo, "local-only.txt", "unadvertised parent advance\n")
	gitSnapshot(t, fixture.repo, "add", "local-only.txt")
	gitSnapshot(t, fixture.repo, "commit", "-m", "local parent advance")
	localB1 := trimGit(gitSnapshot(t, fixture.repo, "rev-parse", "HEAD"))
	gitSnapshot(t, fixture.repo, "checkout", "feature")
	gitSnapshot(t, fixture.repo, "merge", "local-b1", "--no-commit", "--no-ff")

	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePreCommit, LineageID: state.LineageID, BaseRef: localB1,
	})
	if got.Result == GateAllow || !strings.Contains(got.Reason, "advertised tracking branch") {
		t.Fatalf("unadvertised explicit local parent = %#v, want fail-closed tracking-boundary denial", got)
	}
}

func TestCompactPrePushForkSelectorStaysSymbolicDuringBaseAdvanceProof(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)

	// The feature branch publishes to origin/feature, while the reviewed base
	// is advertised by a distinct upstream remote. Keep origin/feature behind
	// HEAD so the raw-commit control has a different tracking boundary.
	gitSnapshot(t, fixture.repo, "--git-dir", fixture.remote, "update-ref", "refs/heads/feature", fixture.originalBaseCommit)
	gitSnapshot(t, fixture.repo, "config", "branch.feature.merge", "refs/heads/feature")
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	gitSnapshot(t, fixture.repo, "clone", "--bare", fixture.remote, upstream)
	gitSnapshot(t, fixture.repo, "remote", "add", "upstream", upstream)

	got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: state.LineageID, BaseRef: "upstream/main",
	})
	if got.Result != GateAllow || got.Context.BaseAdvance == nil || !got.Context.BaseAdvance.Compatible {
		t.Fatalf("symbolic fork base = %#v, want allow with compatible base proof", got)
	}

	rawUpstream := trimGit(gitSnapshot(t, fixture.repo, "rev-parse", "main"))
	raw := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: state.LineageID, BaseRef: rawUpstream,
	})
	if raw.Result == GateAllow || !strings.Contains(raw.Reason, "advertised tracking branch") {
		t.Fatalf("raw nontracking base = %#v, want tracking-boundary denial", raw)
	}

	for _, selector := range []string{"upstream/missing", "main", "local-only"} {
		if selector == "local-only" {
			gitSnapshot(t, fixture.repo, "branch", selector, "HEAD")
		}
		denied := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
			Gate: GatePrePush, LineageID: state.LineageID, BaseRef: selector,
		})
		if denied.Result == GateAllow {
			t.Fatalf("selector %q was accepted", selector)
		}
	}
}
