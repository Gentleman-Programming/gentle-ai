package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCapturedVerificationEvidencePersistsOutcomeAndRejectsConflictingReplay(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "captured-outcome-binding")
	store := storeCompactStartAuthority(t, repo, state)
	revision := mustLoadCompactRecord(t, store).Revision
	payload := []byte("repository verification failed\n")
	request := CaptureVerificationEvidenceRequest{
		StoreDir: store.Dir, LineageID: state.LineageID, AuthorityRevision: revision,
		Target: state.CurrentSnapshot, Payload: payload, Outcome: VerificationOutcomeFailed,
	}

	captured, err := PublishCapturedVerificationEvidence(request)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Record.Outcome != VerificationOutcomeFailed || captured.Record.RawPayloadSHA256 != payloadDigest(payload) ||
		captured.Record.RawPayloadBytes != int64(len(payload)) || !validSHA256(captured.Record.RecordDigest) {
		t.Fatalf("captured evidence record = %#v", captured.Record)
	}
	replayed, err := PublishCapturedVerificationEvidence(request)
	if err != nil || !reflect.DeepEqual(replayed, captured) {
		t.Fatalf("exact replay = %#v, %v; want %#v", replayed, err, captured)
	}

	conflict := request
	conflict.Outcome = VerificationOutcomePassed
	if _, err := PublishCapturedVerificationEvidence(conflict); !errors.Is(err, ErrCapturedVerificationEvidenceConflict) {
		t.Fatalf("conflicting outcome replay error = %v", err)
	}
	loaded, err := ReadCapturedVerificationEvidence(store.Dir, state.LineageID, revision, state.CurrentSnapshot)
	if err != nil || !reflect.DeepEqual(loaded, captured) {
		t.Fatalf("captured evidence after conflict = %#v, %v; want %#v", loaded, err, captured)
	}
}

func TestCapturedVerificationEvidenceAttachesOnlyToByteIdenticalLegacyRawPayload(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "legacy-evidence-attachment")
	store := storeCompactStartAuthority(t, repo, state)
	revision := mustLoadCompactRecord(t, store).Revision
	legacyDir := filepath.Join(store.Dir, CompactFinalEvidenceDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPayload := []byte("legacy raw evidence\n")
	if err := os.WriteFile(filepath.Join(legacyDir, CompactFinalEvidenceFile), legacyPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadCapturedVerificationEvidence(store.Dir, state.LineageID, revision, state.CurrentSnapshot); !errors.Is(err, ErrCapturedVerificationEvidenceMetadataMissing) {
		t.Fatalf("legacy raw read error = %v", err)
	}
	request := CaptureVerificationEvidenceRequest{
		StoreDir: store.Dir, LineageID: state.LineageID, AuthorityRevision: revision,
		Target: state.CurrentSnapshot, Payload: []byte("different evidence\n"), Outcome: VerificationOutcomePassed,
	}
	if _, err := PublishCapturedVerificationEvidence(request); !errors.Is(err, ErrCapturedVerificationEvidenceConflict) {
		t.Fatalf("different legacy attachment error = %v", err)
	}
	request.Payload = legacyPayload
	captured, err := PublishCapturedVerificationEvidence(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(captured.Payload, legacyPayload) || captured.Record.TargetIdentity != state.CurrentSnapshot.Identity {
		t.Fatalf("legacy attachment = %#v", captured)
	}
}

func TestCompleteCorrectionVerificationIsAtomicAndCandidateBound(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "atomic-correction-verification")
	revision := hash("a")
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: append([]string(nil), state.FixFindingIDs...), FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	payload := []byte("repository verification failed\n")
	failed, err := NewVerificationEvidenceRecord(state.LineageID, revision, fix, payload, VerificationOutcomeFailed)
	if err != nil {
		t.Fatal(err)
	}
	before := state
	if err := state.CompleteCorrectionVerification(fix, 1, validation, failed, payload); err == nil {
		t.Fatal("failed repository verification accepted the correction")
	}
	if !reflect.DeepEqual(state, before) || len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 {
		t.Fatalf("failed bundle consumed correction state: before=%#v after=%#v", before, state)
	}

	passedPayload := []byte("repository verification passed\n")
	passed, err := NewVerificationEvidenceRecord(state.LineageID, revision, fix, passedPayload, VerificationOutcomePassed)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteCorrectionVerification(fix, 1, validation, passed, passedPayload); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved || len(state.CorrectionAttempts) != 1 || state.CumulativeCorrectionLines != 1 ||
		state.EvidenceHash != payloadDigest(passedPayload) || state.EvidenceRecordDigest != passed.RecordDigest ||
		state.EvidenceOutcome != VerificationOutcomePassed || state.EvidenceTargetIdentity != fix.Identity ||
		state.EvidenceAuthorityRevision != revision {
		t.Fatalf("atomic correction verification state = %#v", state)
	}
	if _, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: before.CurrentSnapshot.CandidateTree, IntendedUntracked: before.InitialSnapshot.IntendedUntracked, LedgerIDs: before.FixFindingIDs}); err != nil {
		t.Fatal(err)
	}
}

func TestProceduralCorrectionVerificationEscalatesWithoutConsumingCorrection(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "procedural-correction-verification")
	payload := []byte("repository verification tool crashed\n")
	record, err := NewVerificationEvidenceRecord(state.LineageID, hash("a"), fix, payload, VerificationOutcomeProceduralFailure)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EscalateCorrectionVerification(fix, record, payload); err != nil {
		t.Fatal(err)
	}
	if state.State != StateEscalated || len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 ||
		state.CorrectionVerificationTarget == nil || state.CorrectionVerificationTarget.Identity != fix.Identity ||
		state.EvidenceOutcome != VerificationOutcomeProceduralFailure {
		t.Fatalf("procedural correction escalation = %#v", state)
	}
}
