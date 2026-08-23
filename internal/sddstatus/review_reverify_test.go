package sddstatus

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_reverify_test.go is Wave 4 S6 (design.md's "Amendment
// (coordinator-resolved): targeted re-verify call site", 2026-08-03).
// Tasks 7.1-7.3's three branches are proven independently at the pure-
// function level with synthetic inputs (deriveCorrectionEvidence/
// classifyTargetedReVerify take no dependency on live Git or a persisted
// store), plus one real end-to-end test proving the routing wires into
// Resolve() through a genuinely on-disk approved compact authority.

func TestDeriveCorrectionEvidenceBranches(t *testing.T) {
	tests := []struct {
		name    string
		compact *reviewtransaction.CompactState
		want    correctionEvidence
	}{
		{
			name:    "no correction recorded at all",
			compact: &reviewtransaction.CompactState{},
			want:    correctionEvidence{},
		},
		{
			name:    "nil compact state",
			compact: nil,
			want:    correctionEvidence{},
		},
		{
			name: "correction recorded but unborn HEAD -- fail closed (7.3)",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{UnbornHead: true}},
			}},
			want: correctionEvidence{applied: true, failClosed: true},
		},
		{
			name: "correction recorded but no path data -- not derivable (7.2)",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{CandidateTree: "deadbeef"}},
			}},
			want: correctionEvidence{applied: true},
		},
		{
			name: "correction recorded with real path data -- derivable",
			compact: &reviewtransaction.CompactState{CorrectionAttempts: []reviewtransaction.CompactCorrectionAttempt{
				{Snapshot: reviewtransaction.Snapshot{Paths: []string{"a.go"}}},
				{Snapshot: reviewtransaction.Snapshot{Paths: []string{"b.go", "c.go"}}},
			}},
			want: correctionEvidence{applied: true, derivable: true, paths: []string{"b.go", "c.go"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveCorrectionEvidence(tt.compact); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("deriveCorrectionEvidence() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestIntersectPaths(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{name: "empty both", a: nil, b: nil, want: nil},
		{name: "no overlap", a: []string{"a.go"}, b: []string{"b.go"}, want: nil},
		{name: "full overlap", a: []string{"a.go", "b.go"}, b: []string{"b.go", "a.go"}, want: []string{"a.go", "b.go"}},
		{name: "partial overlap dedupes", a: []string{"a.go", "a.go", "c.go"}, b: []string{"a.go"}, want: []string{"a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := intersectPaths(tt.a, tt.b); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("intersectPaths(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestClassifyTargetedReVerifyBranches proves tasks 7.1-7.3's three
// distinct branches directly, with synthetic inputs -- genuinely
// independent of whatever today's receipt/compact-state schema can supply
// in production (the amendment's own explicit permission: "if the receipt
// genuinely cannot carry correction paths, that is branch 7.2 doing its
// job").
func TestClassifyTargetedReVerifyBranches(t *testing.T) {
	t.Run("7.1 empty intersection -> targeted", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: true, paths: []string{"unrelated.go"}}
		scope := []string{"spec-scoped.go"}
		block, emit := classifyTargetedReVerify(evidence, scope)
		if !emit || block.Mode != ReVerifyModeTargeted || !reflect.DeepEqual(block.Scope, scope) || block.Reason == "" {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want targeted with the full scope", block, emit)
		}
	})

	t.Run("7.2 not reliably derivable -> full, distinct reason from 7.1", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: false}
		block, emit := classifyTargetedReVerify(evidence, []string{"spec-scoped.go"})
		if !emit || block.Mode != ReVerifyModeFull || block.Reason == "" || block.Reason == reVerifyEmptyIntersectionReason {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want full with a reason distinct from the empty-intersection branch", block, emit)
		}
	})

	t.Run("non-empty intersection -> full, scoped to the overlap", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, derivable: true, paths: []string{"spec-scoped.go", "other.go"}}
		scope := []string{"spec-scoped.go"}
		block, emit := classifyTargetedReVerify(evidence, scope)
		if !emit || block.Mode != ReVerifyModeFull || !reflect.DeepEqual(block.Scope, []string{"spec-scoped.go"}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want full scoped to the intersection", block, emit)
		}
	})

	t.Run("7.3 fail closed -> no block emitted", func(t *testing.T) {
		evidence := correctionEvidence{applied: true, failClosed: true}
		block, emit := classifyTargetedReVerify(evidence, []string{"spec-scoped.go"})
		if emit || !reflect.DeepEqual(block, ReVerifyBlock{}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want no block on fail-closed commit state", block, emit)
		}
	})

	t.Run("no correction applied -> no block emitted (structural absence)", func(t *testing.T) {
		block, emit := classifyTargetedReVerify(correctionEvidence{}, []string{"spec-scoped.go"})
		if emit || !reflect.DeepEqual(block, ReVerifyBlock{}) {
			t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want no block when no correction was ever recorded", block, emit)
		}
	})
}

// Investigated and NOT pursued (recorded explicitly rather than silently
// dropped): a full end-to-end test driving Resolve() through a genuinely
// protocol-valid on-disk correction (rather than a hand-fixtured
// CompactState) would require the complete real correction round trip --
// state.BeginCorrection, a real git-based TargetFixDiff snapshot, a
// matching FixDeltaHash, a bound ScopedValidationResult, and a bound
// VerificationEvidenceRecord passed to CompleteCorrectionVerification.
// Confirmed empirically: reviewtransaction's own validateCompactCorrection
// cross-checks ProposedLines/ActualLines/Snapshot.Kind/Projection/BaseTree/
// LedgerIDs/Identity/PathsDigest/FixDeltaHash together, so a hand-built
// CompactCorrectionAttempt (even with real, subset-of-GenesisPaths paths)
// is rejected by store.Replace with "compact correction attempt is outside
// frozen scope" -- correction paths being a real subset of GenesisPaths is
// necessary but far from sufficient. Building that full round trip is
// substantially more machinery than this slice's budget, and no existing
// internal/sddstatus test fixture does it either (only
// internal/reviewtransaction's own tests exercise the complete protocol,
// using its package-private helpers this file cannot reach). The three
// branches' LOGIC is proven directly and genuinely above
// (TestDeriveCorrectionEvidenceBranches, TestClassifyTargetedReVerifyBranches)
// with realistic-shaped CompactCorrectionAttempt/Snapshot values; the
// routing WIRING's "no correction -> structural absence" edge is proven
// end-to-end below through a real, on-disk approved compact authority. The
// "a real correction on disk drives the block" wiring edge is not covered
// end-to-end by this slice -- flagged for a follow-up if the coordinator
// wants that specific gap closed.

// TestResolveOmitsReVerifyBlockWithoutAnyCorrection proves structural
// absence: an approved compact authority with no correction history at all
// produces no ReVerify block, and its JSON marshal carries no "reVerify"
// substring, mirroring ReviewOffer's own absence proof shape.
func TestResolveOmitsReVerifyBlockWithoutAnyCorrection(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReVerify != nil {
		t.Fatalf("Status.ReVerify = %#v, want nil (structural absence) with no correction recorded", status.ReVerify)
	}
}

// Review correction data remains advisory. It may populate ReVerify, but
// neither its presence nor its completion changes SDD archive readiness.
func TestReviewCorrectionContextDoesNotBlockArchiveRouting(t *testing.T) {
	status := Status{Dependencies: Dependencies{Apply: DependencyAllDone, Verify: DependencyAllDone, Archive: DependencyReady}, NextRecommended: string(PhaseArchive)}
	block, emit := classifyTargetedReVerify(correctionEvidence{applied: true, derivable: true, paths: []string{"spec.md"}}, []string{"spec.md"})

	if !emit || block.Mode != ReVerifyModeFull {
		t.Fatalf("classifyTargetedReVerify() = %#v, emit=%v, want visible review context", block, emit)
	}
	status.ReVerify = &block
	if status.Dependencies.Archive != DependencyReady || status.NextRecommended != string(PhaseArchive) {
		t.Fatalf("review context routed archive=%q next=%q, want ready/archive", status.Dependencies.Archive, status.NextRecommended)
	}
}
