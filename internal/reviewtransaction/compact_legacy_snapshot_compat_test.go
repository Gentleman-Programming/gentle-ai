package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func legacyCompactStateForTest(state CompactState) CompactState {
	state.InitialSnapshot.Identity, state.CurrentSnapshot.Identity = legacyCompactSnapshotIdentity(state.InitialSnapshot), legacyCompactSnapshotIdentity(state.CurrentSnapshot)
	for index := range state.CorrectionAttempts {
		attempt := &state.CorrectionAttempts[index]
		attempt.Snapshot.Identity = legacyCompactSnapshotIdentity(attempt.Snapshot)
		if attempt.CorrectionTargetIdentity != "" {
			attempt.CorrectionTargetIdentity = attempt.Snapshot.Identity
		}
	}
	if state.Recovery != nil && state.Recovery.Evidence != nil {
		evidence := state.Recovery.Evidence
		evidence.SuccessorTargetIdentity = state.InitialSnapshot.Identity
		evidence.SourceCorrectionAttempt.Snapshot.Identity = legacyCompactSnapshotIdentity(evidence.SourceCorrectionAttempt.Snapshot)
		evidence.SourceCorrectionAttempt.CorrectionTargetIdentity = evidence.SourceCorrectionAttempt.Snapshot.Identity
		evidence.TargetedValidationRequest.CorrectionTargetIdentity = evidence.SourceCorrectionAttempt.Snapshot.Identity
		evidence.TargetedValidationRequest.RequestHash = targetedValidationRequestHash(evidence.TargetedValidationRequest)
		evidence.SourceCorrectionAttempt.TargetedValidationRequestHash = evidence.TargetedValidationRequest.RequestHash
	}
	return state
}

func TestLegacySnapshotIdentityFixedVectors(t *testing.T) {
	tests := []struct {
		name       string
		kind       TargetKind
		projection Projection
		intended   []string
		want       string
	}{
		{"workspace", TargetCurrentChanges, ProjectionWorkspace, []string{"new.txt"}, "sha256:7e67dee95db569fd7a247cc868dde34d5d1c8bf2eca51df8c1783476adec06db"},
		{"staged", TargetCurrentChanges, ProjectionStaged, []string{}, "sha256:0032af9e8e0ccf7df94e51a32d709d956391d487c51af750f0df8049542082bb"},
		{"overlay workspace", TargetBaseWorkspaceOverlay, ProjectionWorkspace, []string{"new.txt"}, "sha256:cd6e9d4dafa4f2d8090936df5e61650a97b7a4257a451255d5eceefee06a3dbe"},
		{"staged overlay", TargetBaseWorkspaceOverlay, ProjectionStaged, []string{}, "sha256:d51b087975053a622b7cda6e60338feb2f37c86a08b152917e97cd50c058459b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := Snapshot{Kind: tt.kind, Projection: tt.projection, BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40),
				PathsDigest: "sha256:" + strings.Repeat("c", 64), IntendedUntrackedProof: "sha256:" + strings.Repeat("d", 64), IntendedUntracked: tt.intended, LedgerIDs: []string{"R1"}}
			if got := legacyCompactSnapshotIdentity(snapshot); got != tt.want {
				t.Fatalf("legacy identity = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCompactStoreLoadsLegacySnapshotIdentityReadOnly(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	current := newCompactTestState(t, repo, "legacy-snapshot-identity")
	legacy := legacyCompactStateForTest(current)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, legacy.LineageID)
	written := writeCompactFixtureRecord(t, store, legacy)
	before, _ := os.ReadFile(store.StatePath())
	loaded := mustLoadCompactRecord(t, store)
	if !loaded.HistoricalCompat || loaded.Revision != written.Revision || loaded.State.InitialSnapshot.Identity != legacy.InitialSnapshot.Identity {
		t.Fatalf("legacy record = %#v", loaded)
	}
	_, mutationErr := store.Replace(loaded.Revision, "review/start", current)
	_, exportErr := store.ExportTransport()
	_, _, captureErr := store.ResolveAdmittedReviewerResult(context.Background(), loaded.Revision, legacy.InitialSnapshot.Identity, FrozenCandidateContext{}, ArtifactSubject{TargetIdentity: legacy.InitialSnapshot.Identity, AuthorityRevision: loaded.Revision, Lens: legacy.SelectedLenses[0]})
	if !errors.Is(mutationErr, ErrHistoricalCompatReadOnly) || !errors.Is(exportErr, ErrHistoricalCompatReadOnly) || !errors.Is(captureErr, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical errors: mutate=%v export=%v capture=%v", mutationErr, exportErr, captureErr)
	}
	if after, err := os.ReadFile(store.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("legacy record bytes changed: %v", err)
	}
	validateErr := legacy.Validate()
	_, newErr := NewCompactState(Start{LineageID: "legacy-new-state", Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: legacy.InitialSnapshot, PolicyHash: current.PolicyHash, RiskLevel: current.RiskLevel, SelectedLenses: current.SelectedLenses, OriginalChangedLines: &current.OriginalChangedLines})
	evidenceErr := (SnapshotBuilder{Repo: repo}).ValidateEvidence(context.Background(), legacy.InitialSnapshot)
	if validateErr == nil || newErr == nil || evidenceErr == nil {
		t.Fatalf("legacy identity escaped strict validation: state=%v new=%v evidence=%v", validateErr, newErr, evidenceErr)
	}
}

func TestCompactStoreValidatesNestedLegacySnapshots(t *testing.T) {
	t.Run("correction and mixed policy", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		state := legacyCompactStateForTest(accountingOnlyEscalatedState(t, repo, "legacy-correction-attempt"))
		store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
		writeCompactFixtureRecord(t, store, state)
		if loaded := mustLoadCompactRecord(t, store); !loaded.HistoricalCompat {
			t.Fatalf("legacy correction = %#v", loaded)
		}
		state.CorrectionAttempts[0].Snapshot.Identity = snapshotIdentityForProjection(state.CorrectionAttempts[0].Snapshot.Kind, state.CorrectionAttempts[0].Snapshot.Projection, state.CorrectionAttempts[0].Snapshot.BaseTree, state.CorrectionAttempts[0].Snapshot.CandidateTree, state.CorrectionAttempts[0].Snapshot.PathsDigest, state.CorrectionAttempts[0].Snapshot.IntendedUntrackedProof, state.CorrectionAttempts[0].Snapshot.IntendedUntracked, state.CorrectionAttempts[0].Snapshot.LedgerIDs)
		writeCompactFixtureRecord(t, store, state)
		if _, err := store.Load(); err == nil {
			t.Fatal("mixed legacy/current correction snapshots loaded")
		}
	})
	t.Run("recovered source and invalidation recursion", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		predecessor := accountingOnlyEscalatedState(t, repo, "legacy-recovered-predecessor")
		_, predecessorRecord := persistEscalatedRecoveryFixture(t, repo, predecessor)
		successor := recoveredEvidenceSuccessor(t, repo, predecessor, "legacy-recovered-successor")
		const actor, reason = "maintainer", "legacy fixture"
		recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision, Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor, MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, predecessorRecord.Revision, successor.InitialSnapshot.Identity, actor, reason)})
		if err != nil {
			t.Fatal(err)
		}
		legacyPredecessor, _, _ := makeCompactRecord(legacyCompactStateForTest(predecessor))
		legacy := legacyCompactStateForTest(recovered.State)
		legacy.Recovery.PredecessorRevision = legacyPredecessor.Revision
		request := &legacy.Recovery.Evidence.TargetedValidationRequest
		request.ExpectedRevision, request.TargetIdentity = legacyPredecessor.Revision, legacyCompactStateForTest(predecessor).InitialSnapshot.Identity
		request.RequestHash = targetedValidationRequestHash(*request)
		legacy.Recovery.Evidence.SourceCorrectionAttempt.TargetedValidationRequestHash = request.RequestHash
		store, _ := CompactAuthoritativeStore(context.Background(), repo, legacy.LineageID)
		writeCompactFixtureRecord(t, store, legacy)
		invalidated := newCompactTestState(t, repo, "legacy-invalidated")
		if err := invalidated.Invalidate("historical invalidation"); err != nil {
			t.Fatal(err)
		}
		invalidatedStore, _ := CompactAuthoritativeStore(context.Background(), repo, invalidated.LineageID)
		writeCompactFixtureRecord(t, invalidatedStore, legacyCompactStateForTest(invalidated))
		if !mustLoadCompactRecord(t, store).HistoricalCompat || !mustLoadCompactRecord(t, invalidatedStore).HistoricalCompat {
			t.Fatal("legacy recovery or invalidation did not load historically")
		}
	})
}

func TestCompactStoreRejectsInauthenticLegacyRecords(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	base := newCompactTestState(t, repo, "legacy-negative")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, base.LineageID)
	tests := []struct {
		name   string
		mutate func(*CompactState)
	}{
		{"arbitrary identity", func(s *CompactState) {
			s.InitialSnapshot.Identity, s.CurrentSnapshot.Identity = "sha256:"+strings.Repeat("e", 64), "sha256:"+strings.Repeat("e", 64)
		}},
		{"metadata mismatch", func(s *CompactState) {
			s.InitialSnapshot.PathsDigest, s.CurrentSnapshot.PathsDigest = "sha256:"+strings.Repeat("f", 64), "sha256:"+strings.Repeat("f", 64)
		}},
		{"mixed policies", func(s *CompactState) { s.InitialSnapshot.Identity = legacyCompactSnapshotIdentity(s.InitialSnapshot) }},
		{"unrelated semantics", func(s *CompactState) { *s = legacyCompactStateForTest(*s); s.Generation = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := base
			tt.mutate(&state)
			writeCompactFixtureRecord(t, store, state)
			if _, err := store.Load(); err == nil {
				t.Fatal("invalid record loaded")
			}
		})
	}
	record := writeCompactFixtureRecord(t, store, legacyCompactStateForTest(base))
	payload, _ := os.ReadFile(store.StatePath())
	payload = bytes.Replace(payload, []byte(record.Revision), []byte("sha256:"+strings.Repeat("0", 64)), 1)
	_ = os.WriteFile(store.StatePath(), payload, 0o644)
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("corrupted checksum error = %v", err)
	}
}
