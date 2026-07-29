package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestChainBundleRoundTripsFrozenCorrectionBudgetInputs(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseChainBundle(payload)
	if err != nil {
		t.Fatal(err)
	}
	record, err := parseRecordPayload(parsed.Events[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if record.Transaction.OriginalChangedLines == nil || *record.Transaction.OriginalChangedLines != 2 || record.Transaction.CorrectionBudget == nil || *record.Transaction.CorrectionBudget != 1 {
		t.Fatalf("round-tripped bundle budget inputs = %#v", record.Transaction)
	}
}

func TestBundleImportRejectsLegacyShapedBoundedGenesis(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, originalChangedLines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{
		LineageID: "legacy-portable", Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("d"), RiskLevel: risk, SelectedLenses: []string{LensReliability}, OriginalChangedLines: &originalChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	legacy := withoutCorrectionBudgetFields(t, *tx)
	local := Store{Dir: filepath.Join(t.TempDir(), "legacy-store")}
	writeStoreEvent(t, local, Record{Operation: "review/start", Transaction: legacy})
	bundle, err := local.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ImportBundle(context.Background(), repo, bundle, BundleImportExpectation{
		LineageID: tx.LineageID, Snapshot: snapshot, PolicyHash: tx.PolicyHash,
		GenesisRevision: bundle.GenesisRevision, HeadRevision: bundle.HeadRevision,
		ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest,
	})
	if err == nil || !strings.Contains(err.Error(), "requires correction budget fields") {
		t.Fatalf("ImportBundle(legacy-shaped bounded chain) error = %v", err)
	}
	store, storeErr := AuthoritativeStore(context.Background(), repo, tx.LineageID)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if _, _, loadErr := store.Load(); !errors.Is(loadErr, os.ErrNotExist) {
		t.Fatalf("rejected legacy bundle installed authority: %v", loadErr)
	}
}

func TestChainBundleRoundTripBootstrapsRepositoryDerivedStore(t *testing.T) {
	source := initSnapshotRepo(t)
	tx, receipt, request := nativeGateFixture(t, source, "portable-lineage")
	sourceStore, err := AuthoritativeStore(context.Background(), source, tx.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	head := appendApprovedStoreChain(t, sourceStore, tx)
	bundle, err := sourceStore.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle() error = %v", err)
	}
	if bundle.GenesisRevision == "" || bundle.HeadRevision != head || bundle.ChainIdentity == "" || bundle.BundleDigest == "" || len(bundle.Events) != 5 {
		t.Fatalf("exported bundle is incomplete: %#v", bundle)
	}

	clone := cloneReviewRepository(t, source)
	expectedSnapshot, err := (SnapshotBuilder{Repo: clone}).Build(context.Background(), request.Target)
	if err != nil {
		t.Fatal(err)
	}
	expectation := BundleImportExpectation{
		LineageID: tx.LineageID, Snapshot: expectedSnapshot,
		PolicyHash: tx.PolicyHash, LedgerHash: tx.LedgerHash, EvidenceHash: tx.EvidenceHash, FixDeltaHash: tx.FixDeltaHash,
		Receipt: receipt, GenesisRevision: bundle.GenesisRevision, HeadRevision: bundle.HeadRevision,
		ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest,
	}
	imported, err := ImportBundle(context.Background(), clone, bundle, expectation)
	if err != nil {
		t.Fatalf("ImportBundle() error = %v", err)
	}
	if imported.HeadRevision != head || imported.Identity != bundle.ChainIdentity {
		t.Fatalf("imported chain = %#v", imported)
	}
	again, err := ImportBundle(context.Background(), clone, bundle, expectation)
	if err != nil || again.HeadRevision != head {
		t.Fatalf("ImportBundle(idempotent) = %#v, %v", again, err)
	}

	request.StoreRevision = bundle.HeadRevision
	request.GenesisRevision = bundle.GenesisRevision
	request.ChainIdentity = bundle.ChainIdentity
	request.BundleDigest = bundle.BundleDigest
	if evaluation := EvaluateNativeGate(context.Background(), clone, receipt, request); evaluation.Result != GateAllow {
		t.Fatalf("EvaluateNativeGate(imported chain) = %#v", evaluation)
	}
}

func TestNonterminalBundleImportsForResumptionWithoutReceipt(t *testing.T) {
	source := initSnapshotRepo(t)
	snapshot, err := (SnapshotBuilder{Repo: source}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{LineageID: "resume-lineage", Mode: ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: hash("a")})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), source, tx.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}
	if bundle.TerminalReceipt != nil {
		t.Fatal("reviewing bundle unexpectedly has a terminal receipt")
	}
	clone := cloneReviewRepository(t, source)
	current, err := (SnapshotBuilder{Repo: clone}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportBundle(context.Background(), clone, bundle, BundleImportExpectation{LineageID: tx.LineageID, Snapshot: current, PolicyHash: tx.PolicyHash, FixDeltaHash: EmptyFixDeltaHash, GenesisRevision: bundle.GenesisRevision, HeadRevision: bundle.HeadRevision, ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest}); err != nil {
		t.Fatalf("ImportBundle(nonterminal) error = %v", err)
	}
}

func TestCorrectedChainBundleRoundTripUsesDeliveredContentEquivalence(t *testing.T) {
	source := initSnapshotRepo(t)
	fixture := correctedBundleFixture(t, source, "portable-corrected-lineage")
	bundle, err := fixture.Store.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle() error = %v", err)
	}

	clone := cloneReviewRepository(t, source)
	expectedSnapshot, err := (SnapshotBuilder{Repo: clone}).Build(context.Background(), fixture.Request.Target)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotsEqual(expectedSnapshot, fixture.Transaction.Snapshot) {
		t.Fatal("corrected recovery fixture unexpectedly has structurally equal lifecycle and fix-diff snapshots")
	}
	expectation := correctedBundleExpectation(fixture, bundle, expectedSnapshot)
	imported, err := ImportBundle(context.Background(), clone, bundle, expectation)
	if err != nil {
		t.Fatalf("ImportBundle(corrected lineage) error = %v", err)
	}
	if imported.HeadRevision != bundle.HeadRevision || imported.Identity != bundle.ChainIdentity {
		t.Fatalf("imported corrected chain = %#v", imported)
	}

	fixture.Request.StoreRevision = bundle.HeadRevision
	fixture.Request.GenesisRevision = bundle.GenesisRevision
	fixture.Request.ChainIdentity = bundle.ChainIdentity
	fixture.Request.BundleDigest = bundle.BundleDigest
	if evaluation := EvaluateNativeGate(context.Background(), clone, fixture.Receipt, fixture.Request); evaluation.Result != GateScopeChanged {
		t.Fatalf("EvaluateNativeGate(imported corrected chain) = %#v", evaluation)
	}
}

func TestChainBundleImportRejectsTamperingTruncationAndWrongBindings(t *testing.T) {
	source := initSnapshotRepo(t)
	fixture := correctedBundleFixture(t, source, "portable-reject-lineage")
	original, err := fixture.Store.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(bundle *ChainBundle, expectation *BundleImportExpectation)
	}{
		{
			name: "tampered event bytes",
			mutate: func(bundle *ChainBundle, _ *BundleImportExpectation) {
				bundle.Events[len(bundle.Events)-1].Payload = append([]byte{}, bundle.Events[len(bundle.Events)-1].Payload...)
				bundle.Events[len(bundle.Events)-1].Payload[0] ^= 1
			},
		},
		{
			name: "truncated chain with recomputed digest",
			mutate: func(bundle *ChainBundle, _ *BundleImportExpectation) {
				bundle.Events = bundle.Events[1:]
				bundle.BundleDigest = bundleDigest(*bundle)
			},
		},
		{
			name: "different lineage",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.LineageID = "different-lineage"
			},
		},
		{
			name: "tampered delivered content",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.Snapshot.CandidateTree = strings.Repeat("f", 40)
			},
		},
		{
			name: "wrong delivered path scope",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.Snapshot.PathsDigest = hash("1")
			},
		},
		{
			name: "wrong policy",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.PolicyHash = hash("2")
			},
		},
		{
			name: "wrong ledger",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.LedgerHash = hash("3")
			},
		},
		{
			name: "wrong evidence",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.EvidenceHash = hash("4")
			},
		},
		{
			name: "wrong fix delta",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.FixDeltaHash = hash("5")
			},
		},
		{
			name: "wrong receipt generation",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.Receipt.Generation++
			},
		},
		{
			name: "wrong receipt mode",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.Receipt.Mode = ModeJudgmentDay
			},
		},
		{
			name: "authoritative intended-untracked proof mismatch",
			mutate: func(bundle *ChainBundle, expectation *BundleImportExpectation) {
				rewriteBundleFixSnapshots(t, bundle, func(snapshot *Snapshot) {
					snapshot.IntendedUntrackedProof = hash("6")
				})
				expectation.HeadRevision = bundle.HeadRevision
				expectation.ChainIdentity = bundle.ChainIdentity
				expectation.BundleDigest = bundle.BundleDigest
			},
		},
		{
			name: "wrong expected bundle identity",
			mutate: func(_ *ChainBundle, expectation *BundleImportExpectation) {
				expectation.BundleDigest = hash("f")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := cloneReviewRepository(t, source)
			expectedSnapshot, err := (SnapshotBuilder{Repo: clone}).Build(context.Background(), fixture.Request.Target)
			if err != nil {
				t.Fatal(err)
			}
			bundle := cloneChainBundle(t, original)
			expectation := correctedBundleExpectation(fixture, original, expectedSnapshot)
			tt.mutate(&bundle, &expectation)
			if _, err := ImportBundle(context.Background(), clone, bundle, expectation); err == nil {
				t.Fatal("ImportBundle() accepted an untrusted or mismatched bundle")
			}
			destination, err := AuthoritativeStore(context.Background(), clone, fixture.Transaction.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(destination.Dir, "HEAD")); !os.IsNotExist(err) {
				t.Fatalf("failed import installed authoritative HEAD: %v", err)
			}
		})
	}
}

func TestChainBundleValidationRejectsForgedReleaseTransitions(t *testing.T) {
	withoutRelease := approvedStoreTransaction(t, "bundle-without-release")
	withoutReleaseStore := Store{Dir: filepath.Join(t.TempDir(), "without-release")}
	appendApprovedStoreChain(t, withoutReleaseStore, withoutRelease)
	withoutReleaseBundle, err := withoutReleaseStore.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}

	withRelease := approvedStoreTransaction(t, "bundle-with-release")
	release := testReleaseEvidence(withRelease.FinalCandidateTree)
	withRelease.Release = cloneReleaseEvidence(&release)
	withReleaseStore := Store{Dir: filepath.Join(t.TempDir(), "with-release")}
	appendApprovedStoreChain(t, withReleaseStore, withRelease)
	withReleaseBundle, err := withReleaseStore.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}

	mutatedRelease := release
	mutatedRelease.ConfigurationHash = hash("7")
	tests := []struct {
		name   string
		bundle ChainBundle
		mutate func(record *Record)
	}{
		{
			name:   "late terminal injection",
			bundle: withoutReleaseBundle,
			mutate: func(record *Record) {
				if record.Transaction.State == StateApproved {
					record.Transaction.Release = cloneReleaseEvidence(&release)
				}
			},
		},
		{
			name:   "bound evidence mutation",
			bundle: withReleaseBundle,
			mutate: func(record *Record) {
				if record.Transaction.State == StateFinalVerifying {
					record.Transaction.Release = cloneReleaseEvidence(&mutatedRelease)
				}
			},
		},
		{
			name:   "bound evidence removal",
			bundle: withReleaseBundle,
			mutate: func(record *Record) {
				if record.Transaction.State == StateFinalVerifying {
					record.Transaction.Release = nil
				}
			},
		},
		{
			name:   "binding under a different operation",
			bundle: withReleaseBundle,
			mutate: func(record *Record) {
				if record.Operation == "review/bind-release-evidence" {
					record.Operation = "review/resume"
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := cloneChainBundle(t, tt.bundle)
			rewriteBundleRecords(t, &bundle, tt.mutate)
			if _, err := validateChainBundle(bundle); !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validateChainBundle() error = %v, want ErrInvalidSuccessor", err)
			}
		})
	}
}

type correctedBundleTestFixture struct {
	Transaction Transaction
	Receipt     Receipt
	Request     GateRequest
	Store       Store
}

func TestLegacyClassificationBundleRemainsReadable(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	frozen := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: *tx})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{
		FindingID: "R1-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "legacy concrete proof",
	}})
	classification := tx.Classifications["R1-001"]
	classification.Causality = ""
	tx.Classifications["R1-001"] = classification
	writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: frozen, Transaction: *tx})

	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle(legacy) error = %v", err)
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseChainBundle(payload)
	if err != nil {
		t.Fatalf("ParseChainBundle(legacy) error = %v", err)
	}
	if parsed.HeadRevision != bundle.HeadRevision || len(parsed.Events) != 3 {
		t.Fatalf("legacy bundle round trip = %#v", parsed)
	}
}

func TestChainBundlePreservesHistoricalJudgmentDayFreezeBytes(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeJudgmentDay)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	if err := tx.RecordJudgeProofs([]JudgeProof{{JudgeID: "judge-a", ExecutionHash: hash("a"), ResultHash: hash("b"), Blind: true, Confirmed: true}, {JudgeID: "judge-b", ExecutionHash: hash("c"), ResultHash: hash("d"), Blind: true, Confirmed: true}}, hash("e")); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/record-judge-proofs", PreviousRevision: head, Transaction: *tx})
	legacy := historicalFreezeTransition(t, *tx)
	head = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: head, Transaction: legacy})
	payload, err := os.ReadFile(filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json"))
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle() error = %v", err)
	}
	last := bundle.Events[len(bundle.Events)-1]
	if last.Revision != head || !bytes.Equal(last.Payload, payload) {
		t.Fatalf("bundle changed historical event: %#v", last)
	}
	if _, err := ParseChainBundle(mustJSON(t, bundle)); err != nil {
		t.Fatalf("ParseChainBundle() error = %v", err)
	}
}

func correctedBundleFixture(t *testing.T, repo, lineage string) correctedBundleTestFixture {
	t.Helper()
	artifacts := t.TempDir()
	policyPath := filepath.Join(artifacts, "policy.md")
	ledgerPath := filepath.Join(artifacts, "ledger.json")
	fixDeltaPath := filepath.Join(artifacts, "fix-delta.patch")
	evidencePath := filepath.Join(artifacts, "evidence.md")
	for path, content := range map[string]string{
		policyPath:   "bounded policy\n",
		ledgerPath:   "{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[{\"id\":\"BRT1-005\",\"lens\":\"resilience\",\"location\":\"internal/reviewtransaction/bundle.go\",\"severity\":\"CRITICAL\",\"claim\":\"corrected lineages cannot recover authority\",\"proof_refs\":[\"bundle.go:209\"]}]}",
		fixDeltaPath: "portable recovery correction\n",
		evidencePath: "verified corrected delivery\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	baseRevision := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "delivery.txt", "initial delivery\n")
	initial, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{"delivery.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := HashArtifact(policyPath)
	ledgerHash, _ := HashLedgerArtifact(ledgerPath)
	fixDeltaHash, _ := HashArtifact(fixDeltaPath)
	evidenceHash, _ := HashArtifact(evidencePath)
	transaction, err := NewTransaction(Start{
		LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1, Snapshot: initial, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision := ""
	appendState := func(operation string) {
		t.Helper()
		var appendErr error
		revision, appendErr = store.Append(revision, Record{Operation: operation, Transaction: *transaction})
		if appendErr != nil {
			t.Fatalf("Append(%s) error = %v", operation, appendErr)
		}
	}

	if err := transaction.StartReview(); err != nil {
		t.Fatal(err)
	}
	appendState("review/start")
	finding := Finding{
		ID: "BRT1-005", Lens: "resilience", Location: "internal/reviewtransaction/bundle.go",
		Severity: "CRITICAL", Claim: "corrected lineages cannot recover authority", ProofRefs: []string{"bundle.go:209"},
	}
	ledger, err := CanonicalLedger([]Finding{finding})
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.FreezeFindings([]Finding{finding}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	appendState("review/freeze-findings")
	if _, err := transaction.ClassifyEvidence([]FindingEvidence{{
		FindingID: "BRT1-005", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "corrected clean-clone import was rejected",
	}}); err != nil {
		t.Fatal(err)
	}
	appendState("review/classify")
	if err := transaction.BeginFix(hash("7")); err != nil {
		t.Fatal(err)
	}
	appendState("review/begin-fix")
	writeSnapshotFile(t, repo, "delivery.txt", "corrected delivery\n")
	fixSnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: transaction.FinalCandidateTree,
		IntendedUntracked: []string{"delivery.txt"}, LedgerIDs: []string{"BRT1-005"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.CompleteFix(fixSnapshot, fixDeltaHash, []string{"BRT1-005"}); err != nil {
		t.Fatal(err)
	}
	appendState("review/complete-fix")
	if err := transaction.ValidateFixDelta([]string{"BRT1-005"}, true); err != nil {
		t.Fatal(err)
	}
	appendState("review/validate-fix-delta")
	if err := transaction.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	appendState("review/begin-final-verification")
	if err := transaction.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	appendState("review/complete-final-verification")
	receipt, err := transaction.Receipt()
	if err != nil {
		t.Fatal(err)
	}

	gitSnapshot(t, repo, "add", "--", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "corrected delivery")
	finalRevision := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	return correctedBundleTestFixture{
		Transaction: *transaction,
		Receipt:     receipt,
		Store:       store,
		Request: GateRequest{
			Schema: GateRequestSchema, Gate: GatePostApply,
			Target:         Target{Kind: TargetExactRevision, Revision: baseRevision + ".." + finalRevision},
			PolicyArtifact: policyPath, LedgerArtifact: ledgerPath,
			FixDeltaArtifact: fixDeltaPath, EvidenceArtifact: evidencePath,
		},
	}
}

func correctedBundleExpectation(fixture correctedBundleTestFixture, bundle ChainBundle, snapshot Snapshot) BundleImportExpectation {
	return BundleImportExpectation{
		LineageID: fixture.Transaction.LineageID, Snapshot: snapshot,
		PolicyHash: fixture.Receipt.PolicyHash, LedgerHash: fixture.Receipt.LedgerHash,
		EvidenceHash: fixture.Receipt.EvidenceHash, FixDeltaHash: fixture.Receipt.FixDeltaHash,
		Receipt: fixture.Receipt, GenesisRevision: bundle.GenesisRevision, HeadRevision: bundle.HeadRevision,
		ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest,
	}
}

func rewriteBundleFixSnapshots(t *testing.T, bundle *ChainBundle, mutate func(snapshot *Snapshot)) {
	t.Helper()
	rewriteBundleRecords(t, bundle, func(record *Record) {
		if record.Transaction.Snapshot.Kind == TargetFixDiff {
			mutate(&record.Transaction.Snapshot)
		}
	})
}

func rewriteBundleRecords(t *testing.T, bundle *ChainBundle, mutate func(record *Record)) {
	t.Helper()
	records := make([]Record, len(bundle.Events))
	for index, event := range bundle.Events {
		record, err := parseRecordPayload(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		mutate(&record)
		records[index] = record
	}
	revisions := make([]string, len(records))
	previous := ""
	for index, record := range records {
		record.PreviousRevision = previous
		payload, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		payload = append(payload, '\n')
		sum := sha256.Sum256(payload)
		revision := "sha256:" + hex.EncodeToString(sum[:])
		bundle.Events[index] = ChainBundleEvent{Revision: revision, Payload: payload}
		revisions[index] = revision
		previous = revision
	}
	bundle.GenesisRevision = revisions[0]
	bundle.HeadRevision = revisions[len(revisions)-1]
	bundle.ChainIdentity = chainIdentity(revisions)
	terminal := records[len(records)-1].Transaction
	bundle.LineageID = terminal.LineageID
	bundle.Generation = terminal.Generation
	bundle.FinalSnapshotIdentity = terminal.Snapshot.Identity
	bundle.PolicyHash = terminal.PolicyHash
	bundle.LedgerHash = terminal.LedgerHash
	bundle.EvidenceHash = terminal.EvidenceHash
	receipt, err := terminal.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	bundle.TerminalReceipt = &receipt
	bundle.BundleDigest = bundleDigest(*bundle)
}

func cloneReviewRepository(t *testing.T, source string) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "clone")
	command := exec.Command("git", "clone", "-q", source, destination)
	command.Env = sanitizedGitEnvironment(os.Environ(), nil)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, output)
	}
	return destination
}

func cloneChainBundle(t *testing.T, bundle ChainBundle) ChainBundle {
	t.Helper()
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var cloned ChainBundle
	if err := json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
