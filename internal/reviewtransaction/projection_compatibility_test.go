package reviewtransaction

import (
	"context"
	"testing"
)

func TestCompactTargetProjectionsCompatible(t *testing.T) {
	tests := []struct {
		name                                    string
		existingKind, requestedKind             TargetKind
		existingProjection, requestedProjection Projection
		want                                    bool
	}{
		{
			name:                "exact projection remains compatible",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetCurrentChanges,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionStaged,
			want:                true,
		},
		{
			name:                "workspace alias accepts default workspace",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetCurrentChanges,
			existingProjection:  ProjectionWorkspace,
			requestedProjection: "",
			want:                true,
		},
		{
			name:                "default workspace alias accepts workspace",
			existingKind:        TargetBaseWorkspaceOverlay,
			requestedKind:       TargetBaseWorkspaceOverlay,
			existingProjection:  "",
			requestedProjection: ProjectionWorkspace,
			want:                true,
		},
		{
			name:                "staged current changes accepts workspace current changes",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetCurrentChanges,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionWorkspace,
			want:                true,
		},
		{
			name:                "workspace current changes accepts staged current changes",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetCurrentChanges,
			existingProjection:  ProjectionWorkspace,
			requestedProjection: ProjectionStaged,
			want:                true,
		},
		{
			name:                "staged base diff accepts workspace base diff",
			existingKind:        TargetBaseDiff,
			requestedKind:       TargetBaseDiff,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionWorkspace,
			want:                true,
		},
		{
			name:                "workspace base diff accepts staged base diff",
			existingKind:        TargetBaseDiff,
			requestedKind:       TargetBaseDiff,
			existingProjection:  ProjectionWorkspace,
			requestedProjection: ProjectionStaged,
			want:                true,
		},
		{
			name:                "staged current changes accepts workspace base diff",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetBaseDiff,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionWorkspace,
			want:                true,
		},
		{
			name:                "workspace base diff accepts staged current changes",
			existingKind:        TargetBaseDiff,
			requestedKind:       TargetCurrentChanges,
			existingProjection:  ProjectionWorkspace,
			requestedProjection: ProjectionStaged,
			want:                true,
		},
		{
			name:                "staged overlay does not accept workspace base diff",
			existingKind:        TargetBaseWorkspaceOverlay,
			requestedKind:       TargetBaseDiff,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionWorkspace,
			want:                false,
		},
		{
			name:                "staged current changes does not accept workspace fix diff",
			existingKind:        TargetCurrentChanges,
			requestedKind:       TargetFixDiff,
			existingProjection:  ProjectionStaged,
			requestedProjection: ProjectionWorkspace,
			want:                false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactTargetProjectionsCompatible(tt.existingKind, tt.existingProjection, tt.requestedKind, tt.requestedProjection); got != tt.want {
				t.Fatalf("compactTargetProjectionsCompatible() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestProjectionCompatibilityGovernancePredicates(t *testing.T) {
	emptyProof := hashCanonical("gentle-ai.intended-untracked/v1")
	base := Snapshot{
		Kind:                   TargetCurrentChanges,
		Projection:             ProjectionStaged,
		BaseTree:               "base",
		CandidateTree:          "candidate",
		PathsDigest:            digestPaths([]string{"tracked.go"}),
		Paths:                  []string{"tracked.go"},
		IntendedUntracked:      []string{},
		IntendedUntrackedProof: emptyProof,
	}
	live := Snapshot{
		Kind: TargetBaseDiff, Projection: ProjectionWorkspace,
		BaseTree: base.BaseTree, CandidateTree: base.CandidateTree,
		PathsDigest: base.PathsDigest, Paths: append([]string(nil), base.Paths...),
		IntendedUntracked: append([]string(nil), base.IntendedUntracked...), IntendedUntrackedProof: base.IntendedUntrackedProof,
	}
	state := CompactState{
		InitialSnapshot: base,
		CurrentSnapshot: base,
		GenesisPaths:    append([]string(nil), base.Paths...),
	}
	requested := state
	requested.InitialSnapshot = live
	transaction := Transaction{
		Snapshot:           base,
		GenesisPaths:       append([]string(nil), base.Paths...),
		BaseTree:           base.BaseTree,
		FinalCandidateTree: base.CandidateTree,
	}

	if !compactStartDeliveryScopeMatches(state, requested) {
		t.Fatal("compactStartDeliveryScopeMatches() rejected equivalent content scope")
	}
	if !compactLiveTargetMatchesValidatedSnapshot(state, live, true) {
		t.Fatal("compactLiveTargetMatchesValidatedSnapshot() rejected exact candidate")
	}
	if !legacyLiveTargetMatchesValidatedSnapshot(transaction, live) {
		t.Fatal("legacyLiveTargetMatchesValidatedSnapshot() rejected exact candidate")
	}
}

func TestDiscoveryExactCandidateIgnoresIntendedUntrackedSideBand(t *testing.T) {
	emptyProof := hashCanonical("gentle-ai.intended-untracked/v1")
	base := Snapshot{
		Kind:                   TargetCurrentChanges,
		Projection:             ProjectionStaged,
		BaseTree:               "base",
		CandidateTree:          "candidate",
		PathsDigest:            digestPaths([]string{"tracked.go"}),
		Paths:                  []string{"tracked.go"},
		IntendedUntracked:      []string{},
		IntendedUntrackedProof: emptyProof,
	}
	live := Snapshot{
		Kind: TargetBaseDiff, Projection: ProjectionWorkspace,
		BaseTree: base.BaseTree, CandidateTree: base.CandidateTree,
		PathsDigest: base.PathsDigest, Paths: append([]string(nil), base.Paths...),
		IntendedUntracked: []string{"draft.md"}, IntendedUntrackedProof: "different-proof",
	}
	state := CompactState{
		InitialSnapshot: base,
		CurrentSnapshot: base,
		GenesisPaths:    append([]string(nil), base.Paths...),
	}
	requested := state
	requested.InitialSnapshot = live
	transaction := Transaction{
		Snapshot:           base,
		GenesisPaths:       append([]string(nil), base.Paths...),
		BaseTree:           base.BaseTree,
		FinalCandidateTree: live.CandidateTree,
	}

	if compactStartDeliveryScopeMatches(state, requested) {
		t.Fatal("compactStartDeliveryScopeMatches() accepted differing intended-untracked side-band")
	}
	if !compactLiveTargetMatchesValidatedSnapshot(state, live, true) {
		t.Fatal("compactLiveTargetMatchesValidatedSnapshot() rejected exact candidate side-band variation")
	}
	if compactLiveTargetMatchesValidatedSnapshot(state, live, false) {
		t.Fatal("compactLiveTargetMatchesValidatedSnapshot() accepted side-band variation without exact candidate requirement")
	}
	if !legacyLiveTargetMatchesValidatedSnapshot(transaction, live) {
		t.Fatal("legacyLiveTargetMatchesValidatedSnapshot() rejected exact candidate side-band variation")
	}
}

func TestApprovedStagedCurrentChangesReceiptGovernsWorkspaceBaseDiffAfterCommit(t *testing.T) {
	repo, state, _, _ := approvedCurrentChangesScopeRecoveryFixtureForProjection(t, "projection-compatible-status", ProjectionStaged)
	gitSnapshot(t, repo, "commit", "-m", "deliver staged candidate")

	target := Target{
		Kind: TargetBaseDiff, BaseRef: state.InitialSnapshot.BaseTree,
		IntendedUntracked: []string{},
	}
	requested := newCompactStartStateForTarget(t, repo, "projection-compatible-start", target)
	requested.PolicyHash = state.PolicyHash
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || started.Action != CompactStartReuseReceipt || started.Record.State.LineageID != state.LineageID {
		t.Fatalf("START after commit = %#v, %v", started, err)
	}

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: target, LineageID: state.LineageID,
	})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.State != StateApproved ||
		status.Action != TargetStatusActionValidate || status.ReceiptIdentity == "" || status.LineageID != state.LineageID {
		t.Fatalf("STATUS after commit = %#v, %v", status, err)
	}
}
