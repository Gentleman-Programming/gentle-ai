package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAssessTargetStatusDerivesReceiptTruthWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	tests := []struct {
		name              string
		prepare           func(t *testing.T, repo, lineage string) CompactStore
		wantApplicability TargetApplicability
		wantAction        TargetStatusAction
		wantReplay        Replayability
		wantIdentity      bool
	}{
		{
			name: "fresh reviewing receipt is expected missing",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				return storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, lineage))
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionFinalize,
			wantReplay:        ReplayabilityNotReplayable,
		},
		{
			name: "approved derived receipt is published",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				return store
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionValidate,
			wantReplay:        ReplayabilityNotReplayable,
			wantIdentity:      true,
		},
		{
			name: "approved authority with absent receipt is replayable",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				if err := os.Remove(store.ReceiptPath()); err != nil {
					t.Fatal(err)
				}
				return store
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionFinalize,
			wantReplay:        ReplayabilityExactReplaySafe,
		},
		{
			name: "wrong or corrupt receipt fails closed",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				if err := os.WriteFile(store.ReceiptPath(), []byte("{\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return store
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			lineage := "receipt-truth-" + strings.ReplaceAll(strings.Fields(tt.name)[0], "_", "-")
			store := tt.prepare(t, repo, lineage)
			authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			before := authorityBytes(t, authorityRoot)
			request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
			first, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) || first.Applicability != tt.wantApplicability || first.Action != tt.wantAction || first.Replayability != tt.wantReplay {
				t.Fatalf("receipt status = %#v, second %#v", first, second)
			}
			if tt.wantIdentity {
				payload, err := os.ReadFile(store.ReceiptPath())
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(payload)
				want := "sha256:" + hex.EncodeToString(sum[:])
				if first.ReceiptIdentity != want {
					t.Fatalf("receipt identity = %q, want %q", first.ReceiptIdentity, want)
				}
			} else if first.ReceiptIdentity != "" {
				t.Fatalf("unsafe receipt identity = %q", first.ReceiptIdentity)
			}
			if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
				t.Fatalf("receipt status mutated authority: before=%v after=%v", before, after)
			}
		})
	}
}

func TestAssessTargetStatusClassifiesAllApplicabilityStates(t *testing.T) {
	requireSnapshotGit(t)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}

	t.Run("current target fresh start", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "review-current")
		store := storeCompactStartAuthority(t, repo, state)
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}

		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCurrent || got.State != StateReviewing || got.Action != TargetStatusActionFinalize {
			t.Fatalf("status = %#v", got)
		}
		if got.LineageID != state.LineageID || got.Generation != state.Generation || got.Revision != record.Revision || got.ReceiptIdentity != "" {
			t.Fatalf("authority identity = %#v", got)
		}
		if got.OriginalChangedLines != state.OriginalChangedLines || got.Tier != state.RiskLevel || got.CorrectionBudget != state.CorrectionBudget {
			t.Fatalf("frozen review inputs = %#v", got)
		}
		if got.TargetIdentity != state.InitialSnapshot.Identity || got.Projection.CurrentCandidateTree != state.CurrentSnapshot.CandidateTree {
			t.Fatalf("projection = %#v", got.Projection)
		}
		if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
			t.Fatalf("fresh START receipt stat error = %v, want not-exist", err)
		}
	})

	t.Run("unrelated", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "first candidate\n")
		storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-old"))
		writeSnapshotFile(t, repo, "tracked.txt", "different candidate\n")
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart || got.LineageID != "" {
			t.Fatalf("status = %#v", got)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		unrelated := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-b"))
		storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-a"))
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityAmbiguous || got.Action != TargetStatusActionSelectLineage || !reflect.DeepEqual(got.CandidateLineageIDs, []string{"review-a", "review-b"}) {
			t.Fatalf("status = %#v", got)
		}
		selected := request
		selected.LineageID = "review-a"
		resolved, err := AssessTargetStatus(context.Background(), repo, selected)
		if err != nil || resolved.Applicability != TargetApplicabilityCurrent || resolved.LineageID != "review-a" {
			t.Fatalf("selected status = %#v, err = %v", resolved, err)
		}
		if err := os.WriteFile(unrelated.StatePath(), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		resolved, err = AssessTargetStatus(context.Background(), repo, selected)
		if err != nil || resolved.Applicability != TargetApplicabilityCurrent || resolved.LineageID != "review-a" {
			t.Fatalf("selected status with corrupt unrelated authority = %#v, err = %v", resolved, err)
		}
		selected.LineageID = "review-b"
		if corrupt, err := AssessTargetStatus(context.Background(), repo, selected); err != nil || corrupt.Applicability != TargetApplicabilityCorrupted {
			t.Fatalf("selected corrupt status = %#v, err = %v", corrupt, err)
		}
	})

	t.Run("corrupted", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-corrupt"))
		if err := os.WriteFile(store.StatePath(), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCorrupted || got.Action != TargetStatusActionRepairAuthority || got.Replayability != ReplayabilityManualActionRequired {
			t.Fatalf("status = %#v", got)
		}
		payload, _ := json.Marshal(got)
		if strings.Contains(string(payload), repo) || strings.Contains(string(payload), "review-state.json") {
			t.Fatalf("status exposes authority filesystem details: %s", payload)
		}
	})
}

func TestAssessTargetStatusRecognizesAuthorizedCorrection(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "review-correction-resume")
	store := storeCompactStartAuthority(t, repo, state)
	record, _ := store.Load()
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"differential failure"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed"}}}, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, _ := store.Replace(record.Revision, "review/complete-review", state)
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/begin-fix", state); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if err != nil || got.Applicability != TargetApplicabilityCurrent || got.State != StateCorrectionRequired || got.Action != TargetStatusActionFinalize {
		t.Fatalf("authorized correction status = %#v, err = %v", got, err)
	}
}

func TestCompactTargetStatusUsesCurrentProofAndLiveProjection(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	writeSnapshotFile(t, repo, "new.txt", "initial\n")
	builder := SnapshotBuilder{Repo: repo}
	initial, _ := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"new.txt"}})
	writeSnapshotFile(t, repo, "new.txt", "corrected\n")
	live, _ := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"new.txt"}})
	fix, _ := builder.Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: initial.CandidateTree, IntendedUntracked: []string{"new.txt"}, LedgerIDs: []string{"R1-001"}})
	state := CompactState{InitialSnapshot: initial, CurrentSnapshot: fix, GenesisPaths: initial.Paths}
	if !compactLiveTargetMatchesSnapshot(context.Background(), repo, state, live, true) {
		t.Fatal("terminal correction did not match current intended-untracked proof")
	}
	wrongProof := state
	wrongProof.CurrentSnapshot.IntendedUntrackedProof = initial.IntendedUntrackedProof
	if compactLiveTargetMatchesSnapshot(context.Background(), repo, wrongProof, live, true) {
		t.Fatal("terminal correction accepted a stale intended-untracked proof")
	}
	projection := targetProjectionFromCompact(state, targetProjectionFromSnapshot(live))
	if projection.Kind != live.Kind || projection.PathsDigest != live.PathsDigest || projection.IntendedUntrackedProof != live.IntendedUntrackedProof || projection.CurrentSnapshotIdentity != live.Identity || projection.InitialSnapshotIdentity != initial.Identity {
		t.Fatalf("corrected projection = %#v, live = %#v", projection, live)
	}
}

func TestAssessTargetStatusReconstructsAfterRestartWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "review-restart")
	storeCompactStartAuthority(t, repo, state)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}
	first, err := AssessTargetStatus(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AssessTargetStatus(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Projection.InitialSnapshotIdentity != state.InitialSnapshot.Identity || first.Projection.CurrentSnapshotIdentity != state.CurrentSnapshot.Identity {
		t.Fatalf("restart reconstruction differs: first=%#v second=%#v", first, second)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only status mutated authority: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusIgnoresUnrelatedValidLegacyHistory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	head := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	legacySnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: head})
	if err != nil {
		t.Fatal(err)
	}
	storeLegacyReviewingStatus(t, repo, "legacy-history", legacySnapshot)
	writeSnapshotFile(t, repo, "tracked.txt", "compact candidate\n")
	compact := newCompactTestState(t, repo, "review-current")
	storeCompactStartAuthority(t, repo, compact)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionCompact || got.LineageID != compact.LineageID {
		t.Fatalf("status = %#v", got)
	}
}

func TestAssessTargetStatusStopsApplicableNonTerminalLegacyWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "legacy candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	lineage := "legacy-read-only-status"
	storeLegacyReviewingStatus(t, repo, lineage, snapshot)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
	for attempt := 0; attempt < 2; attempt++ {
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionLegacy ||
			got.Action != TargetStatusActionStop || got.Replayability != ReplayabilityManualActionRequired || got.State != StateReviewing {
			t.Fatalf("legacy status = %#v", got)
		}
		if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(after, before) {
			t.Fatalf("attempt %d mutated legacy authority", attempt+1)
		}
	}
}

func TestAssessTargetStatusValidatesApplicableApprovedLegacyReceiptWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	tests := []struct {
		name              string
		mutateReceipt     func(t *testing.T, path string, receipt Receipt)
		wantApplicability TargetApplicability
		wantAction        TargetStatusAction
		wantReplay        Replayability
		wantIdentity      bool
	}{
		{
			name:              "approved receipt is present and valid",
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionValidate,
			wantReplay:        ReplayabilityNotReplayable,
			wantIdentity:      true,
		},
		{
			name: "approved receipt is missing",
			mutateReceipt: func(t *testing.T, path string, _ Receipt) {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
		{
			name: "approved receipt is corrupt",
			mutateReceipt: func(t *testing.T, path string, _ Receipt) {
				if err := os.WriteFile(path, []byte("{\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
		{
			name: "approved receipt belongs to different authority",
			mutateReceipt: func(t *testing.T, path string, receipt Receipt) {
				receipt.PolicyHash = hash("b")
				if err := WriteReceiptAtomic(path, receipt); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "legacy candidate\n")
			lineage := "legacy-approved-" + strings.ReplaceAll(strings.Fields(tt.name)[2], "_", "-")
			transaction, receipt, _ := nativeGateFixture(t, repo, lineage)
			store, err := AuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			appendApprovedStoreChain(t, store, transaction)
			receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
			if err := WriteReceiptAtomic(receiptPath, receipt); err != nil {
				t.Fatal(err)
			}
			if tt.mutateReceipt != nil {
				tt.mutateReceipt(t, receiptPath, receipt)
			}
			authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			before := authorityBytes(t, authorityRoot)
			request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
			first, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) || first.Applicability != tt.wantApplicability ||
				first.Action != tt.wantAction || first.Replayability != tt.wantReplay {
				t.Fatalf("legacy receipt status = %#v, second %#v", first, second)
			}
			if tt.wantIdentity {
				payload, err := os.ReadFile(receiptPath)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(payload)
				want := "sha256:" + hex.EncodeToString(sum[:])
				if first.ReceiptIdentity != want || first.AuthorityVersion != AuthorityVersionLegacy || first.State != StateApproved {
					t.Fatalf("approved legacy receipt status = %#v, want identity %q", first, want)
				}
			} else if first.ReceiptIdentity != "" || first.Action == TargetStatusActionFinalize || first.Replayability == ReplayabilityExactReplaySafe {
				t.Fatalf("invalid legacy receipt exposed compact replay semantics: %#v", first)
			}
			if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
				t.Fatalf("legacy receipt status mutated authority: before=%v after=%v", before, after)
			}
		})
	}
}

func TestAssessTargetStatusMatchesCorrectedLegacyDelivery(t *testing.T) {
	repo := initSnapshotRepo(t)
	fixture := correctedBundleFixture(t, repo, "legacy-corrected-status")
	receiptPath := filepath.Join(fixture.Store.Dir, "artifacts", "receipt.json")
	if err := WriteReceiptAtomic(receiptPath, fixture.Receipt); err != nil {
		t.Fatal(err)
	}
	base := strings.SplitN(fixture.Request.Target.Revision, "..", 2)[0]
	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: fixture.Transaction.Snapshot.IntendedUntracked}, LineageID: fixture.Transaction.LineageID})
	if err != nil || got.Applicability != TargetApplicabilityCurrent || got.State != StateApproved || got.Action != TargetStatusActionValidate || got.Projection.Kind != TargetBaseDiff {
		t.Fatalf("corrected legacy status = %#v, err = %v", got, err)
	}
	writeSnapshotFile(t, repo, "delivery.txt", "mismatched delivery\n")
	gitSnapshot(t, repo, "add", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "mismatched delivery")
	if mismatch, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: fixture.Transaction.Snapshot.IntendedUntracked}, LineageID: fixture.Transaction.LineageID}); err != nil || mismatch.Applicability != TargetApplicabilityUnrelated {
		t.Fatalf("mismatched corrected legacy status = %#v, err = %v", mismatch, err)
	}
}

func TestAssessTargetStatusFailsClosedForMixedSameLineageAuthority(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "review-collision")
	storeCompactStartAuthority(t, repo, state)
	storeLegacyReviewingStatus(t, repo, state.LineageID, state.InitialSnapshot)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCorrupted || got.Action != TargetStatusActionRepairAuthority {
		t.Fatalf("status = %#v", got)
	}
}

func storeLegacyReviewingStatus(t *testing.T, repo, lineage string, snapshot Snapshot) {
	t.Helper()
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	tx, err := NewTransaction(Start{LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetStatusResultHasNoAuthorityPathFields(t *testing.T) {
	typeOf := reflect.TypeOf(TargetStatusResult{})
	for index := 0; index < typeOf.NumField(); index++ {
		if strings.Contains(strings.ToLower(typeOf.Field(index).Name), "path") || strings.Contains(strings.ToLower(typeOf.Field(index).Name), "dir") {
			t.Fatalf("authority path-like field exposed: %s", typeOf.Field(index).Name)
		}
	}
	_ = filepath.Separator
}
