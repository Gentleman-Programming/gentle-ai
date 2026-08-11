package reviewtransaction

import (
	"context"
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
