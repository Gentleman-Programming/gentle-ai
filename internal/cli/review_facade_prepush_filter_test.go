package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestCompactLeafProvablyUnrelatedToPrePushCandidate is the RED-marked
// regression for issue #1886: the bounded pre-push fast path is provably
// correct for an eligible leaf (matching BaseRef + disjoint genesis paths)
// and refuses to skip otherwise (ineligible kind, nil GenesisPaths, BaseRef
// mismatch, overlapping paths). One table-driven case per branch keeps the
// assertion surface explicit without re-running the live, per-leaf
// git-subprocess path that the production filter now avoids.
// regression: issue #1886.
func TestCompactLeafProvablyUnrelatedToPrePushCandidate(t *testing.T) {
	baseDiff := reviewtransaction.Snapshot{Kind: reviewtransaction.TargetBaseDiff, BaseTree: "abc123"}
	tests := []struct {
		name      string
		state     reviewtransaction.CompactState
		liveBase  string
		livePaths []string
		wantSkip  bool
	}{
		{
			name:      "eligible leaf with matching BaseRef and disjoint genesis paths skips",
			state:     reviewtransaction.CompactState{Schema: reviewtransaction.CompactStateSchema, GenesisPaths: []string{"foo/bar.txt"}, CurrentSnapshot: baseDiff},
			liveBase:  "abc123",
			livePaths: []string{"different/path.txt"},
			wantSkip:  true,
		},
		{
			name:      "TargetCurrentChanges kind does not skip",
			state:     reviewtransaction.CompactState{Schema: reviewtransaction.CompactStateSchema, GenesisPaths: []string{"foo/bar.txt"}, CurrentSnapshot: reviewtransaction.Snapshot{Kind: reviewtransaction.TargetCurrentChanges, BaseTree: "abc123"}},
			liveBase:  "abc123",
			livePaths: []string{"foo/bar.txt"},
			wantSkip:  false,
		},
		{
			name:      "nil GenesisPaths does not skip",
			state:     reviewtransaction.CompactState{Schema: reviewtransaction.CompactStateSchema, CurrentSnapshot: baseDiff},
			liveBase:  "abc123",
			livePaths: []string{"foo/bar.txt"},
			wantSkip:  false,
		},
		{
			name:      "BaseRef mismatch does not skip",
			state:     reviewtransaction.CompactState{Schema: reviewtransaction.CompactStateSchema, GenesisPaths: []string{"foo/bar.txt"}, CurrentSnapshot: reviewtransaction.Snapshot{Kind: reviewtransaction.TargetBaseDiff, BaseTree: "different-tree"}},
			liveBase:  "abc123",
			livePaths: []string{"foo/bar.txt"},
			wantSkip:  false,
		},
		{
			name:      "overlapping paths does not skip",
			state:     reviewtransaction.CompactState{Schema: reviewtransaction.CompactStateSchema, GenesisPaths: []string{"foo/bar.txt"}, CurrentSnapshot: baseDiff},
			liveBase:  "abc123",
			livePaths: []string{"foo/bar.txt"},
			wantSkip:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompactLeafProvablyUnrelatedToPrePushCandidate(tt.state, tt.liveBase, tt.livePaths); got != tt.wantSkip {
				t.Fatalf("CompactLeafProvablyUnrelatedToPrePushCandidate() = %v, want %v", got, tt.wantSkip)
			}
		})
	}
}
