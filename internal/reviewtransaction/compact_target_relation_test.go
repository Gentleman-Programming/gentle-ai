package reviewtransaction

import "testing"

func TestCompactTargetRelationAlgebra(t *testing.T) {
	frozen := Snapshot{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionWorkspace,
		BaseTree: tree("1"), CandidateTree: tree("2"), Paths: []string{"a.go", "b.go"},
	}
	compatibleProof := BaseAdvanceCompatibility{
		Status: baseAdvanceCompatibleStatus, Compatible: true,
		OriginalMergeBaseTree: frozen.BaseTree, NewBaseTree: tree("3"),
		OriginalPatchIdentity: hash("1"), DeliveredPatchIdentity: hash("1"),
		DeliveredPathsDigest: digestPaths(frozen.Paths), BaseAdvancePathsDigest: hash("3"), PathsDisjoint: true,
		MergedResultTree: tree("4"), CIAttestationArtifactHash: hash("4"),
		CIAttestationIssuer: "trusted-ci", CIStatus: "success",
	}

	tests := []struct {
		name     string
		live     Snapshot
		evidence compactTargetRelationEvidence
		want     compactTargetRelationKind
		wantPath compactPathSetRelation
	}{
		{name: "same", live: frozen, want: compactTargetSame, wantPath: compactPathsSame},
		{name: "compatible advance", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: compatibleProof.NewBaseTree,
			CandidateTree: frozen.CandidateTree, Paths: append([]string(nil), frozen.Paths...),
		}, evidence: compactTargetRelationEvidence{CompatibleAdvance: &compatibleProof}, want: compactTargetCompatibleAdvance, wantPath: compactPathsSame},
		{name: "unproven base change", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: tree("3"),
			CandidateTree: tree("5"), Paths: append([]string(nil), frozen.Paths...),
		}, want: compactTargetChangedScope, wantPath: compactPathsSame},
		{name: "provable contraction", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: frozen.BaseTree,
			CandidateTree: tree("5"), Paths: []string{"a.go"},
		}, want: compactTargetProvableContraction, wantPath: compactPathsContraction},
		{name: "retained expansion", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: frozen.BaseTree,
			CandidateTree: tree("5"), Paths: []string{"a.go", "c.go"},
		}, want: compactTargetChangedScope, wantPath: compactPathsOverlap},
		{name: "accepted degenerate recovery wrapper", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: frozen.CandidateTree,
			CandidateTree: frozen.CandidateTree, Paths: []string{},
		}, want: compactTargetChangedScope, wantPath: compactPathsDisjoint},
		{name: "unrelated disjoint target", live: Snapshot{
			Kind: frozen.Kind, Projection: frozen.Projection, BaseTree: frozen.BaseTree,
			CandidateTree: tree("5"), Paths: []string{"x.go"},
		}, want: compactTargetUnsafe, wantPath: compactPathsDisjoint},
		{name: "projection change", live: Snapshot{
			Kind: frozen.Kind, Projection: ProjectionStaged, BaseTree: frozen.BaseTree,
			CandidateTree: frozen.CandidateTree, Paths: append([]string(nil), frozen.Paths...),
		}, want: compactTargetUnsafe, wantPath: compactPathsSame},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCompactTargetRelation(frozen, tt.live, frozen.Paths, tt.evidence)
			if got.Kind != tt.want || got.Paths != tt.wantPath {
				t.Fatalf("relation = %#v, want kind %q paths %q", got, tt.want, tt.wantPath)
			}
		})
	}
}

func TestCompactTargetRelationRejectsInexactCompatibilityEvidence(t *testing.T) {
	frozen := Snapshot{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, BaseTree: tree("1"), CandidateTree: tree("2"), Paths: []string{"a.go"}}
	live := frozen
	live.BaseTree = tree("3")
	proof := BaseAdvanceCompatibility{
		Status: baseAdvanceCompatibleStatus, Compatible: true,
		OriginalMergeBaseTree: tree("9"), NewBaseTree: live.BaseTree,
		OriginalPatchIdentity: hash("1"), DeliveredPatchIdentity: hash("1"),
		DeliveredPathsDigest: hash("2"), BaseAdvancePathsDigest: hash("3"), PathsDisjoint: true,
		MergedResultTree: tree("4"), CIAttestationArtifactHash: hash("4"),
		CIAttestationIssuer: "trusted-ci", CIStatus: "success",
	}

	got := classifyCompactTargetRelation(frozen, live, frozen.Paths, compactTargetRelationEvidence{CompatibleAdvance: &proof})
	if got.Kind == compactTargetCompatibleAdvance {
		t.Fatalf("inexact compatibility evidence authorized relation: %#v", got)
	}
}
