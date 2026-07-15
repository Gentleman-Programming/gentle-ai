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
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWriteAtomicPropagatesParentDirectorySyncFailure(t *testing.T) {
	originalGOOS := reviewRuntimeGOOS
	originalSync := syncReviewDirectory
	reviewRuntimeGOOS = func() string { return "linux" }
	syncReviewDirectory = func(string) error { return errors.New("disk sync failed") }
	t.Cleanup(func() {
		reviewRuntimeGOOS = originalGOOS
		syncReviewDirectory = originalSync
	})

	err := writeAtomic(filepath.Join(t.TempDir(), "state.json"), []byte("{}\n"), 0o644)
	if err == nil || !strings.Contains(err.Error(), "sync parent directory") {
		t.Fatalf("writeAtomic() error = %v, want parent-directory sync failure", err)
	}
}

func TestWriteAtomicToleratesUnsupportedParentDirectorySync(t *testing.T) {
	tests := []struct {
		name string
		goos string
		err  error
	}{
		{name: "unix invalid operation", goos: "darwin", err: syscall.EINVAL},
		{name: "unsupported filesystem", goos: "linux", err: errors.ErrUnsupported},
		{name: "windows permission", goos: "windows", err: os.ErrPermission},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGOOS := reviewRuntimeGOOS
			originalSync := syncReviewDirectory
			reviewRuntimeGOOS = func() string { return tt.goos }
			syncReviewDirectory = func(string) error { return tt.err }
			t.Cleanup(func() {
				reviewRuntimeGOOS = originalGOOS
				syncReviewDirectory = originalSync
			})

			if err := writeAtomic(filepath.Join(t.TempDir(), "state.json"), []byte("{}\n"), 0o644); err != nil {
				t.Fatalf("writeAtomic() unsupported directory sync error = %v", err)
			}
		})
	}
}

func TestStoreIsAppendOnlyAtomicAndRejectsStaleWriters(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	first, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if _, err := store.Append("", Record{Operation: "stale", Transaction: *tx}); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("Append(stale) error = %v, want ErrConcurrentUpdate", err)
	}
	loaded, revision, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if revision != first || loaded.Operation != "review/start" {
		t.Fatalf("Load() = revision %q record %#v", revision, loaded)
	}
	entries, err := os.ReadDir(filepath.Join(store.Dir, "events"))
	if err != nil {
		t.Fatalf("ReadDir(events) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("event count = %d, want 1 append-only record", len(entries))
	}
}

func TestStoreAppendRepairsInterruptedEventAndIsIdempotentAtHead(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	first, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_ = freezeTestFindings(tx, []Finding{})
	record := Record{Operation: "review/freeze-findings", Transaction: *tx}
	linked := record
	linked.Schema = RecordSchema
	linked.PreviousRevision = first
	payload, err := json.MarshalIndent(linked, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	wantRevision := "sha256:" + hex.EncodeToString(sum[:])
	eventPath := filepath.Join(store.Dir, "events", strings.TrimPrefix(wantRevision, "sha256:")+".json")
	if err := os.WriteFile(eventPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := store.Append(first, record)
	if err != nil {
		t.Fatalf("Append(repair linked event) error = %v", err)
	}
	if got != wantRevision {
		t.Fatalf("Append(repair) revision = %q, want %q", got, wantRevision)
	}
	got, err = store.Append(first, record)
	if err != nil || got != wantRevision {
		t.Fatalf("Append(identical committed retry) = %q, %v; want %q, nil", got, err, wantRevision)
	}
	if _, err := store.Append(first, Record{Operation: "different-content", Transaction: *tx}); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("Append(different retry) error = %v, want ErrConcurrentUpdate", err)
	}
	if _, err := store.Append(hash("f"), record); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("Append(stale predecessor) error = %v, want ErrConcurrentUpdate", err)
	}
	if head, err := readRevision(filepath.Join(store.Dir, "HEAD")); err != nil || head != wantRevision {
		t.Fatalf("HEAD = %q, %v; want %q", head, err, wantRevision)
	}
}

func TestStoreLockReportsLiveOwnerAndCannotBeStolen(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		t.Fatalf("acquireStoreLock(first) error = %v", err)
	}
	defer lock.release()

	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_, err = store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if !errors.Is(err, ErrConcurrentUpdate) || !strings.Contains(err.Error(), "pid=") || !strings.Contains(err.Error(), "host=") {
		t.Fatalf("Append(while live owner holds lock) error = %v, want actionable owner contention", err)
	}
}

func TestStoreLockRecoversCrashAndCorruptOwnerRecords(t *testing.T) {
	for _, content := range []string{
		"not-json\n",
		`{"schema":"gentle-ai.review-store-lock/v1","owner_id":"crashed","pid":999999,"host":"gone","acquired_at":"2000-01-01T00:00:00Z"}` + "\n",
	} {
		t.Run(content[:min(len(content), 8)], func(t *testing.T) {
			store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
			if err := os.MkdirAll(store.Dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(store.Dir, "LOCK"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			tx := newTestTransaction(t, ModeOrdinary4R)
			_ = tx.StartReview()
			if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
				t.Fatalf("Append() did not recover an unlocked stale owner record: %v", err)
			}
		})
	}
}

func TestStoreLockIsReleasedWhenOwnerProcessExits(t *testing.T) {
	if os.Getenv("GENTLE_AI_LOCK_EXIT_HELPER") == "1" {
		lock, err := acquireStoreLock(os.Getenv("GENTLE_AI_LOCK_EXIT_PATH"))
		if err != nil {
			t.Fatal(err)
		}
		_ = lock
		return
	}
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	command := exec.Command(os.Args[0], "-test.run=^TestStoreLockIsReleasedWhenOwnerProcessExits$")
	command.Env = append(os.Environ(), "GENTLE_AI_LOCK_EXIT_HELPER=1", "GENTLE_AI_LOCK_EXIT_PATH="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lock owner helper: %v\n%s", err, output)
	}
	lock, err := acquireStoreLock(path)
	if err != nil {
		t.Fatalf("kernel lock remained held after process exit: %v", err)
	}
	defer lock.release()
}

func TestConcurrentStoreLockRecoverersCannotBothWin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt stale owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	type result struct {
		lock *storeLock
		err  error
	}
	start := make(chan struct{})
	release := make(chan struct{})
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			lock, err := acquireStoreLock(path)
			results <- result{lock: lock, err: err}
			if err == nil {
				<-release
				_ = lock.release()
			}
		}()
	}
	close(start)
	first := <-results
	second := <-results
	winners := 0
	for _, candidate := range []result{first, second} {
		if candidate.err == nil {
			winners++
		} else if !errors.Is(candidate.err, ErrConcurrentUpdate) {
			t.Fatalf("contender error = %v, want ErrConcurrentUpdate", candidate.err)
		}
	}
	if winners != 1 {
		t.Fatalf("simultaneous stale-lock recoverers = %d winners, want exactly 1", winners)
	}
	close(release)
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("store lock recovery waited without a bound")
	}
}

func TestStoreRejectsRegressiveOrUnrelatedSuccessorAtCurrentRevision(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	first, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatalf("Append(start) error = %v", err)
	}
	if err := freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Append(first, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatalf("Append(freeze) error = %v", err)
	}

	regressive := newTestTransaction(t, ModeOrdinary4R)
	_ = regressive.StartReview()
	if _, err := store.Append(second, Record{Operation: "retry/reset", Transaction: *regressive}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(regressive) error = %v, want ErrInvalidSuccessor", err)
	}

	unrelated := *tx
	unrelated.LineageID = "different-lineage"
	if _, err := store.Append(second, Record{Operation: "retry/replace", Transaction: unrelated}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(unrelated) error = %v, want ErrInvalidSuccessor", err)
	}

	loaded, revision, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if revision != second || loaded.Transaction.State != StateFindingsFrozen || loaded.Transaction.Counters.FullReviews != 1 {
		t.Fatalf("authoritative state changed after rejected replacements: revision=%q transaction=%#v", revision, loaded.Transaction)
	}
}

func TestStoreRejectsCounterAndOutcomeRegression(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	first, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	second, err := store.Append(first, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "failing focused test"}})
	third, err := store.Append(second, Record{Operation: "review/classify", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}

	regressive := *tx
	regressive.State = StateFindingsFrozen
	regressive.Outcomes = map[string]EvidenceOutcome{}
	regressive.FixFindingIDs = []string{}
	if _, err := store.Append(third, Record{Operation: "retry/regress", Transaction: regressive}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(regressive outcome) error = %v, want ErrInvalidSuccessor", err)
	}
}

func TestStoreLoadsLegacyClassificationAndAppendsItsLegalSuccessor(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	frozen := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: *tx})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{
		FindingID: "R1-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "legacy concrete proof",
	}})
	legacyClassification := tx.Classifications["R1-001"]
	legacyClassification.Causality = ""
	tx.Classifications["R1-001"] = legacyClassification
	classified := writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: frozen, Transaction: *tx})

	loaded, revision, err := store.Load()
	if err != nil {
		t.Fatalf("Load(legacy classification) error = %v", err)
	}
	if revision != classified || !loaded.Transaction.legacyCausality {
		t.Fatalf("legacy classification load = revision %q transaction %#v", revision, loaded.Transaction)
	}
	if err := loaded.Transaction.BeginFix(hash("2")); err != nil {
		t.Fatalf("BeginFix(legacy successor) error = %v", err)
	}
	if _, err := store.Append(revision, Record{Operation: "review/begin-fix", Transaction: loaded.Transaction}); err != nil {
		t.Fatalf("Append(legacy successor) error = %v", err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatalf("LoadChain(legacy successor) error = %v", err)
	}
	if got := chain.Records[len(chain.Records)-1].Transaction; got.State != StateFixing || !got.legacyCausality {
		t.Fatalf("legacy successor replay = %#v", got)
	}
}

func TestStoreLoadsLegacyBoundedLineageAndCompletesFixWithoutNewBudgetSemantics(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	originalChangedLines := 196
	tx, err := NewTransaction(Start{
		LineageID: "legacy-bounded", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: newTestTransaction(t, ModeOrdinary4R).Snapshot, PolicyHash: hash("d"),
		RiskLevel: RiskMedium, SelectedLenses: []string{LensReliability}, OriginalChangedLines: &originalChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	legacy := withoutCorrectionBudgetFields(t, *tx)
	revision := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: legacy})
	finding := Finding{ID: "REL-001", Lens: "reliability", Location: "internal/example.go:1", Severity: "CRITICAL", Claim: "legacy regression", ProofRefs: []string{"legacy test failed"}}
	if err := legacy.RecordLensResult(LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"legacy focused test exited 1"}}); err != nil {
		t.Fatal(err)
	}
	revision = writeStoreEvent(t, store, Record{Operation: "review/record-lens-result", PreviousRevision: revision, Transaction: legacy})
	if err := freezeTestFindings(&legacy, []Finding{finding}); err != nil {
		t.Fatal(err)
	}
	revision = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: revision, Transaction: legacy})
	if _, err := legacy.ClassifyEvidence([]FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "legacy changed hunk"}}); err != nil {
		t.Fatal(err)
	}
	revision = writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: revision, Transaction: legacy})

	loaded, loadedRevision, err := store.Load()
	if err != nil || loadedRevision != revision || !loaded.Transaction.legacyCorrectionBudget {
		t.Fatalf("Load(legacy bounded) = revision %q transaction %#v err %v", loadedRevision, loaded.Transaction, err)
	}
	if err := loaded.Transaction.BeginFix(hash("2")); err != nil {
		t.Fatalf("BeginFix(legacy bounded) error = %v", err)
	}
	revision, err = store.Append(revision, Record{Operation: "review/begin-fix", Transaction: loaded.Transaction})
	if err != nil {
		t.Fatal(err)
	}
	fix := loaded.Transaction.Snapshot
	fix.Kind, fix.BaseTree, fix.CandidateTree = TargetFixDiff, loaded.Transaction.FinalCandidateTree, tree("c")
	fix.LedgerIDs, fix.Identity = []string{finding.ID}, hash("3")
	if err := loaded.Transaction.CompleteFix(fix, hash("4"), fix.LedgerIDs); err != nil {
		t.Fatalf("CompleteFix(legacy bounded) error = %v", err)
	}
	if _, err := store.Append(revision, Record{Operation: "review/complete-fix", Transaction: loaded.Transaction}); err != nil {
		t.Fatalf("Append(legacy bounded fix) error = %v", err)
	}
}

func TestStoreReplaysOnlyDocumentedHistoricalV1Aliases(t *testing.T) {
	t.Run("ordinary targeted validation operation in legacy fix delta position", func(t *testing.T) {
		store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
		tx := newTestTransaction(t, ModeOrdinary4R)
		if err := tx.StartReview(); err != nil {
			t.Fatal(err)
		}
		head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
		if err := freezeTestFindings(tx, []Finding{{ID: "R1-DET", Severity: "CRITICAL"}}); err != nil {
			t.Fatal(err)
		}
		head = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: head, Transaction: *tx})
		_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-DET", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "historical proof"}})
		head = writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: head, Transaction: *tx})
		if err := tx.BeginFix(hash("a")); err != nil {
			t.Fatal(err)
		}
		head = writeStoreEvent(t, store, Record{Operation: "review/begin-fix", PreviousRevision: head, Transaction: *tx})
		fix := tx.Snapshot
		fix.Kind, fix.BaseTree, fix.CandidateTree, fix.LedgerIDs, fix.Identity = TargetFixDiff, tx.FinalCandidateTree, tree("c"), []string{"R1-DET"}, hash("b")
		if err := tx.CompleteFix(fix, hash("c"), fix.LedgerIDs); err != nil {
			t.Fatal(err)
		}
		head = writeStoreEvent(t, store, Record{Operation: "review/complete-fix", PreviousRevision: head, Transaction: *tx})
		legacy := historicalValidationTransition(*tx)
		head = writeStoreEvent(t, store, Record{Operation: "review/validate-targeted-fix", PreviousRevision: head, Transaction: legacy})
		assertHistoricalChainRoundTrips(t, store, head, legacy)
	})

	t.Run("Judgment Day historical findings freeze", func(t *testing.T) {
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
		assertHistoricalChainRoundTrips(t, store, head, legacy)
	})

	t.Run("ordinary v1.49 historical findings freeze", func(t *testing.T) {
		store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
		tx := newTestTransaction(t, ModeOrdinary4R)
		if err := tx.StartReview(); err != nil {
			t.Fatal(err)
		}
		head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
		legacy := historicalFreezeTransition(t, *tx)
		head = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: head, Transaction: legacy})
		assertHistoricalChainRoundTrips(t, store, head, legacy)
	})

	t.Run("misplaced targeted validation operation", func(t *testing.T) {
		store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
		tx := newTestTransaction(t, ModeOrdinary4R)
		if err := tx.StartReview(); err != nil {
			t.Fatal(err)
		}
		head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
		writeStoreEvent(t, store, Record{Operation: "review/validate-targeted-fix", PreviousRevision: head, Transaction: historicalFreezeTransition(t, *tx)})
		if _, err := store.LoadChain(); !errors.Is(err, ErrInvalidSuccessor) {
			t.Fatalf("LoadChain() error = %v, want ErrInvalidSuccessor", err)
		}
	})
}

func TestStoreReplaysPublishedV149Ordinary4RAuthority(t *testing.T) {
	fixture := filepath.Join("testdata", "v1.49.0-ordinary-4r")
	checksums := map[string]string{
		"HEAD":                   "5c6444bb299691060d3d6b449f3177275b02ab472b246d082615b0d851e7b56f",
		"artifacts/receipt.json": "e219f2c50ec3c5cf7c83a9844d955511c07041cbfdc9f8530cc6f9bd558d2fa2",
		"events/5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f.json": "5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f",
		"events/9b7dc5776fcad044ac56798b9ca3c823b53a3486816c27234ff537dbde2ee0ef.json": "9b7dc5776fcad044ac56798b9ca3c823b53a3486816c27234ff537dbde2ee0ef",
		"events/b7d4df583b8e1bb952c6f021e5aeb015cb837cdbf81f827007ca42c29b13278c.json": "b7d4df583b8e1bb952c6f021e5aeb015cb837cdbf81f827007ca42c29b13278c",
		"events/bd3ac2bea5b0c51c7205479d680b907b5b88a88c24be899a7cf0e6843d3d23eb.json": "bd3ac2bea5b0c51c7205479d680b907b5b88a88c24be899a7cf0e6843d3d23eb",
		"events/d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86.json": "d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86",
	}
	for name, want := range checksums {
		payload, err := os.ReadFile(filepath.Join(fixture, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		got := sha256.Sum256(payload)
		if hex.EncodeToString(got[:]) != want {
			t.Fatalf("fixture checksum %s = %x, want %s", name, got, want)
		}
	}
	chain, err := (Store{Dir: fixture}).LoadChain()
	if err != nil {
		t.Fatalf("LoadChain(published v1.49.0) error = %v", err)
	}
	if len(chain.Records) != 5 || chain.Records[len(chain.Records)-1].Transaction.State != StateApproved {
		t.Fatalf("LoadChain(published v1.49.0) = %#v", chain)
	}
}

func TestPublishedV149FreezeCompatibilityRejectsUnreproducedChanges(t *testing.T) {
	store := Store{Dir: filepath.Join("testdata", "v1.49.0-ordinary-4r")}
	start, _, err := store.loadRevision("sha256:d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86")
	if err != nil {
		t.Fatal(err)
	}
	freeze, _, err := store.loadRevision("sha256:5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePersistedV1Successor(start.Transaction, freeze.Transaction, freeze.Operation, 1); err != nil {
		t.Fatalf("published freeze transition error = %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*Transaction)
	}{
		{name: "empty findings representation", mutate: func(next *Transaction) { next.Findings = nil }},
		{name: "derived findings hash", mutate: func(next *Transaction) { next.LedgerFindingsHash = hash("changed findings hash") }},
		{name: "external ledger hash format", mutate: func(next *Transaction) { next.LedgerHash = "not-a-sha256" }},
		{name: "evidence", mutate: func(next *Transaction) { next.EvidenceHash = hash("changed evidence") }},
		{name: "criteria", mutate: func(next *Transaction) {
			next.OriginalCriteria = &ValidationCheck{EvidenceHash: hash("evidence"), FixDeltaHash: hash("delta"), Passed: true}
		}},
		{name: "lineage", mutate: func(next *Transaction) { next.LineageID = "different-lineage" }},
		{name: "policy", mutate: func(next *Transaction) { next.PolicyHash = hash("changed policy") }},
		{name: "snapshot", mutate: func(next *Transaction) { next.Snapshot.Identity = hash("changed snapshot") }},
		{name: "outcomes", mutate: func(next *Transaction) { next.Outcomes = map[string]EvidenceOutcome{"unknown": OutcomeInfo} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			next := freeze.Transaction
			test.mutate(&next)
			if err := validatePersistedV1Successor(start.Transaction, next, freeze.Operation, 1); !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validatePersistedV1Successor() error = %v, want ErrInvalidSuccessor", err)
			}
		})
	}
}

func TestValidatePersistedV1SuccessorRejectsUnknownAndNonEquivalentAliases(t *testing.T) {
	previous := ordinaryAtFixing(t)
	legacy := historicalValidationTransition(*previous)
	for _, test := range []struct {
		name      string
		operation string
		mutate    func(*Transaction)
	}{
		{name: "unknown operation", operation: "review/unknown-v1-alias"},
		{name: "changed immutable policy", operation: "review/validate-targeted-fix", mutate: func(next *Transaction) { next.PolicyHash = hash("z") }},
		{name: "changed validation counter", operation: "review/validate-targeted-fix", mutate: func(next *Transaction) { next.Counters.ScopedFixValidations++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			next := legacy
			if test.mutate != nil {
				test.mutate(&next)
			}
			if err := validatePersistedV1Successor(*previous, next, test.operation, 7); !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validatePersistedV1Successor() error = %v, want ErrInvalidSuccessor", err)
			}
		})
	}
}

func TestValidatePersistedV1SuccessorRejectsHistoricalFreezeMutation(t *testing.T) {
	previous := newTestTransaction(t, ModeOrdinary4R)
	if err := previous.StartReview(); err != nil {
		t.Fatal(err)
	}
	legacy := historicalFreezeTransition(t, *previous)
	for _, test := range []struct {
		name   string
		mutate func(*Transaction)
	}{
		{name: "policy", mutate: func(next *Transaction) { next.PolicyHash = hash("changed policy") }},
		{name: "snapshot", mutate: func(next *Transaction) { next.Snapshot.Identity = hash("changed snapshot") }},
		{name: "counter", mutate: func(next *Transaction) { next.Counters.FullReviews++ }},
		{name: "finding routing", mutate: func(next *Transaction) { next.Outcomes["R1-I01"] = OutcomeCorroborated }},
	} {
		t.Run(test.name, func(t *testing.T) {
			next := legacy
			next.Outcomes = cloneOutcomes(legacy.Outcomes)
			test.mutate(&next)
			if err := validatePersistedV1Successor(*previous, next, "review/freeze-findings", 1); !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validatePersistedV1Successor() error = %v, want ErrInvalidSuccessor", err)
			}
		})
	}
}

func TestValidatePersistedV1SuccessorRejectsModernBoundedAliases(t *testing.T) {
	t.Run("targeted validation alias", func(t *testing.T) {
		previous := ordinaryAtFixing(t)
		previous.Mode = ModeOrdinaryBounded
		next := historicalValidationTransition(*previous)
		if err := validatePersistedV1Successor(*previous, next, "review/validate-targeted-fix", 7); !errors.Is(err, ErrInvalidSuccessor) {
			t.Fatalf("validatePersistedV1Successor() error = %v, want ErrInvalidSuccessor", err)
		}
	})

	t.Run("external ledger findings freeze", func(t *testing.T) {
		previous := newTestTransaction(t, ModeOrdinary4R)
		if err := previous.StartReview(); err != nil {
			t.Fatal(err)
		}
		previous.Mode = ModeOrdinaryBounded
		next := historicalFreezeTransition(t, *previous)
		if err := validatePersistedV1Successor(*previous, next, "review/freeze-findings", 1); !errors.Is(err, ErrInvalidSuccessor) {
			t.Fatalf("validatePersistedV1Successor() error = %v, want ErrInvalidSuccessor", err)
		}
	})
}

func assertHistoricalChainRoundTrips(t *testing.T, store Store, head string, terminal Transaction) {
	t.Helper()
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatalf("LoadChain() error = %v", err)
	}
	if chain.HeadRevision != head || chain.Records[len(chain.Records)-1].Transaction.Counters != terminal.Counters || chain.Identity != chainIdentity(chain.Revisions) {
		t.Fatalf("loaded chain did not preserve historical identity: %#v", chain)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle() error = %v", err)
	}
	last := bundle.Events[len(bundle.Events)-1]
	payload, err := os.ReadFile(filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if last.Revision != head || !bytes.Equal(last.Payload, payload) {
		t.Fatalf("bundle changed historical event: %#v", last)
	}
	if _, err := ParseChainBundle(mustJSON(t, bundle)); err != nil {
		t.Fatalf("ParseChainBundle() error = %v", err)
	}
}

func historicalValidationTransition(previous Transaction) Transaction {
	next := previous
	next.State = StateReadyFinalVerification
	next.Counters.ScopedFixValidations++
	next.OriginalCriteria = nil
	next.CorrectionRegression = nil
	return next
}

func historicalFreezeTransition(t *testing.T, previous Transaction) Transaction {
	t.Helper()
	next := previous
	next.Findings = []Finding{{ID: "R1-I01", Lens: "reliability", Location: "internal/example.go:1", Severity: "WARNING", Claim: "historical finding", ProofRefs: []string{"historical proof"}}}
	next.Outcomes = map[string]EvidenceOutcome{"R1-I01": OutcomeInfo}
	next.LedgerHash = hash("f")
	ledger, err := CanonicalLedger(next.Findings)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(ledger)
	if next.LedgerHash == "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatal("v1.49 fixture did not retain an external ledger hash")
	}
	next.LedgerFindingsHash = findingsHash(next.Findings)
	next.State = StateFindingsFrozen
	return next
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestStoreRejectsFreshLegacyShapedBoundedGenesis(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	legacy := withoutCorrectionBudgetFields(t, *tx)
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: legacy}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(fresh legacy-shaped bounded genesis) error = %v, want ErrInvalidSuccessor", err)
	}
	if _, _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected legacy-shaped genesis created authority: %v", err)
	}
}

func TestSuccessorKeepsOriginalRiskInputsAndBudgetImmutable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Transaction)
	}{
		{name: "risk tier", mutate: func(tx *Transaction) { tx.RiskLevel = RiskHigh }},
		{name: "selected lenses", mutate: func(tx *Transaction) { tx.SelectedLenses = []string{LensRisk} }},
		{name: "original changed lines", mutate: func(tx *Transaction) { value := 197; tx.OriginalChangedLines = &value }},
		{name: "correction budget", mutate: func(tx *Transaction) { value := 97; tx.CorrectionBudget = &value }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := budgetedAtFixRequired(t, 196)
			next := *previous
			if err := next.BeginFix(hash("2"), 98); err != nil {
				t.Fatal(err)
			}
			tt.mutate(&next)
			if err := validateSuccessor(*previous, next, "review/begin-fix"); !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validateSuccessor() error = %v, want ErrInvalidSuccessor", err)
			}
		})
	}
}

func TestAuthoritativeStoreRejectsForgedRepositoryDerivedBudgetInputsAndActualLines(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate one\ncandidate two\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, originalChangedLines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{
		LineageID: "repository-budget", Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: hash("d"), RiskLevel: risk, SelectedLenses: []string{LensReliability}, OriginalChangedLines: &originalChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, tx.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	forged := *tx
	forgedOriginal := originalChangedLines + 2
	forgedBudget, _ := CorrectionBudget(forgedOriginal)
	forged.OriginalChangedLines, forged.CorrectionBudget = &forgedOriginal, &forgedBudget
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: forged}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged original count) error = %v, want ErrInvalidSuccessor", err)
	}
	revision, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "REL-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate regression", ProofRefs: []string{"focused test failed"}}
	if err := tx.RecordLensResult(LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"focused test exited 1"}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, Record{Operation: "review/record-lens-result", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{finding}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, Record{Operation: "review/classify-evidence", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFix(hash("2"), *tx.CorrectionBudget); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Append(revision, Record{Operation: "review/begin-fix", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "corrected one\ncandidate two\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: tx.FinalCandidateTree, IntendedUntracked: []string{}, LedgerIDs: []string{finding.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFix(fix, hash("4"), fix.LedgerIDs, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(revision, Record{Operation: "review/complete-fix", Transaction: *tx}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged actual count) error = %v, want ErrInvalidSuccessor", err)
	}
	loaded, head, err := store.Load()
	if err != nil || head != revision || loaded.Transaction.State != StateFixing {
		t.Fatalf("rejected actual count mutated authority: head=%q transaction=%#v err=%v", head, loaded.Transaction, err)
	}
}

func withoutCorrectionBudgetFields(t *testing.T, transaction Transaction) Transaction {
	t.Helper()
	payload, err := json.Marshal(transaction)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"original_changed_lines", "correction_budget", "proposed_correction_lines", "actual_correction_lines"} {
		delete(raw, field)
	}
	payload, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := ParseTransaction(payload)
	if err != nil {
		t.Fatal(err)
	}
	return legacy
}

func TestValidateSuccessorEnforcesReleaseBindingTimingAndImmutability(t *testing.T) {
	ready := newTestTransaction(t, ModeOrdinary4R)
	if err := ready.StartReview(); err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(ready, []Finding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ready.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	release := testReleaseEvidence(ready.FinalCandidateTree)
	bound := *ready
	if err := bound.BindReleaseEvidence(release); err != nil {
		t.Fatalf("BindReleaseEvidence() error = %v", err)
	}
	mutatedRelease := release
	mutatedRelease.ConfigurationHash = hash("7")
	if err := bound.BindReleaseEvidence(mutatedRelease); err == nil {
		t.Fatal("BindReleaseEvidence() replaced an existing release binding")
	}

	verifyingWithoutRelease := *ready
	if err := verifyingWithoutRelease.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	approvedWithoutRelease := verifyingWithoutRelease
	if err := approvedWithoutRelease.CompleteFinalVerification(hash("8"), true); err != nil {
		t.Fatal(err)
	}
	verifyingWithRelease := bound
	if err := verifyingWithRelease.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		previous  Transaction
		next      Transaction
		operation string
		wantError bool
	}{
		{
			name:     "legal ready-state binding",
			previous: *ready, next: bound,
			operation: "review/bind-release-evidence",
		},
		{
			name:     "binding under a different operation",
			previous: *ready, next: bound,
			operation: "review/begin-final-verification", wantError: true,
		},
		{
			name:     "late injection while final verification begins",
			previous: *ready, next: func() Transaction {
				forged := verifyingWithoutRelease
				forged.Release = cloneReleaseEvidence(&release)
				return forged
			}(),
			operation: "review/begin-final-verification", wantError: true,
		},
		{
			name:     "final approval injection",
			previous: verifyingWithoutRelease, next: func() Transaction {
				forged := approvedWithoutRelease
				forged.Release = cloneReleaseEvidence(&release)
				return forged
			}(),
			operation: "review/complete-final-verification", wantError: true,
		},
		{
			name:     "bound release mutation",
			previous: bound, next: func() Transaction {
				forged := verifyingWithRelease
				forged.Release = cloneReleaseEvidence(&mutatedRelease)
				return forged
			}(),
			operation: "review/begin-final-verification", wantError: true,
		},
		{
			name:     "bound release removal",
			previous: bound, next: func() Transaction {
				forged := verifyingWithRelease
				forged.Release = nil
				return forged
			}(),
			operation: "review/begin-final-verification", wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSuccessor(tt.previous, tt.next, tt.operation)
			if tt.wantError && !errors.Is(err, ErrInvalidSuccessor) {
				t.Fatalf("validateSuccessor() error = %v, want ErrInvalidSuccessor", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("validateSuccessor() error = %v", err)
			}
		})
	}
}

func TestValidateSuccessorAllowsReleaseBindAfterJSONNormalizesEmptyCollections(t *testing.T) {
	previous := newTestTransaction(t, ModeOrdinary4R)
	if err := previous.StartReview(); err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(previous, []Finding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	previous.Snapshot.LedgerIDs = nil
	next := *previous
	next.Snapshot.LedgerIDs = []string{}
	if err := next.BindReleaseEvidence(testReleaseEvidence(next.FinalCandidateTree)); err != nil {
		t.Fatal(err)
	}
	if err := validateSuccessor(*previous, next, "review/bind-release-evidence"); err != nil {
		t.Fatalf("validateSuccessor() rejected semantically identical JSON-normalized release bind: %v", err)
	}
}

func TestStoreLoadRejectsIncompleteAndIllegalPredecessorChains(t *testing.T) {
	approved := approvedStoreTransaction(t, "chain-lineage")
	reviewing := newTestTransaction(t, ModeOrdinary4R)
	reviewing.LineageID = approved.LineageID
	if err := reviewing.StartReview(); err != nil {
		t.Fatal(err)
	}
	frozen := *reviewing
	if err := freezeTestFindings(&frozen, []Finding{}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		seed  func(t *testing.T, store Store)
		write Record
	}{
		{
			name:  "standalone terminal event",
			write: Record{Operation: "review/complete-final-verification", Transaction: approved},
		},
		{
			name:  "missing predecessor",
			write: Record{Operation: "review/complete-final-verification", PreviousRevision: hash("a"), Transaction: approved},
		},
		{
			name: "cyclic predecessor alias",
			seed: func(t *testing.T, store Store) {
				record := Record{Schema: RecordSchema, Operation: "review/complete-final-verification", PreviousRevision: hash("c"), Transaction: approved}
				writeStoreEventAtRevision(t, store, hash("c"), record)
			},
			write: Record{Operation: "review/complete-final-verification", PreviousRevision: hash("c"), Transaction: approved},
		},
		{
			name: "regressive successor",
			seed: func(t *testing.T, store Store) {
				genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *reviewing})
				writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: frozen})
			},
			write: Record{Operation: "retry/reset", Transaction: *reviewing},
		},
		{
			name: "terminal inserted without legal predecessor",
			seed: func(t *testing.T, store Store) {
				writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *reviewing})
			},
			write: Record{Operation: "review/complete-final-verification", Transaction: approved},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
			if tt.seed != nil {
				tt.seed(t, store)
			}
			if tt.name == "regressive successor" || tt.name == "terminal inserted without legal predecessor" {
				previous, err := readRevision(filepath.Join(store.Dir, "HEAD"))
				if err != nil {
					t.Fatal(err)
				}
				tt.write.PreviousRevision = previous
			}
			writeStoreEvent(t, store, tt.write)
			if _, _, err := store.Load(); err == nil {
				t.Fatal("Load() accepted an incomplete or illegal predecessor chain")
			}
		})
	}
}

func TestStoreLoadRejectsHashValidSemanticFindingBypasses(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) (Store, Record)
	}{
		{
			name: "findings frozen jumps to ready without classification or outcome",
			build: func(t *testing.T) (Store, Record) {
				store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
				tx := newTestTransaction(t, ModeOrdinary4R)
				_ = tx.StartReview()
				genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
				_ = freezeTestFindings(tx, []Finding{
					{ID: "R1-001", Severity: "CRITICAL"},
					{ID: "R1-I01", Severity: "WARNING"},
				})
				frozen := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: *tx})
				forged := *tx
				forged.State = StateReadyFinalVerification
				return store, Record{Operation: "forged/skip-classification", PreviousRevision: frozen, Transaction: forged}
			},
		},
		{
			name: "evidence classified clears pending refuter without consuming batch",
			build: func(t *testing.T) (Store, Record) {
				store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
				tx := newTestTransaction(t, ModeOrdinary4R)
				_ = tx.StartReview()
				genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
				_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
				frozen := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: *tx})
				_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-001", Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "concurrency trace"}})
				classified := writeStoreEvent(t, store, Record{Operation: "review/classify", PreviousRevision: frozen, Transaction: *tx})
				forged := *tx
				forged.State = StateReadyFinalVerification
				forged.PendingRefuterIDs = []string{}
				return store, Record{Operation: "forged/skip-refuter", PreviousRevision: classified, Transaction: forged}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, forged := tt.build(t)
			writeStoreEvent(t, store, forged)
			if _, _, err := store.Load(); err == nil {
				t.Fatal("Load() accepted a hash-valid chain that bypassed severe finding resolution")
			}
		})
	}
}

func TestStoreLoadsV149FreezeWithRetainedExternalLedgerHash(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	if err := freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}}); err != nil {
		t.Fatal(err)
	}
	historical := *tx
	historical.LedgerHash = hash("d")
	if historical.LedgerHash == tx.LedgerHash {
		t.Fatal("historical ledger hash accidentally equals the canonical ledger hash")
	}
	writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: historical})
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatalf("LoadChain() error = %v", err)
	}
	if got := chain.Records[len(chain.Records)-1].Transaction.LedgerHash; got != historical.LedgerHash {
		t.Fatalf("historical retained ledger hash = %q, want %q", got, historical.LedgerHash)
	}
}

func TestAuthoritativeStoreUsesRepositoryCommonDirectoryAndCanonicalLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := AuthoritativeStore(context.Background(), repo, "trusted-lineage-1")
	if err != nil {
		t.Fatalf("AuthoritativeStore() error = %v", err)
	}
	commonDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	want := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v1", "trusted-lineage-1")
	if store.Dir != want {
		t.Fatalf("authoritative store = %q, want %q", store.Dir, want)
	}

	for _, lineage := range []string{"../escape", "lineage/escape", "Lineage", "lineage--alias", "lineage_1", ".", "lineage."} {
		t.Run(lineage, func(t *testing.T) {
			if _, err := AuthoritativeStore(context.Background(), repo, lineage); err == nil {
				t.Fatalf("AuthoritativeStore() accepted non-canonical lineage %q", lineage)
			}
		})
	}
}

func TestRepositoryIdentityIgnoresHostileGitSelectionEnvironment(t *testing.T) {
	repo := initSnapshotRepo(t)
	hostile := initSnapshotRepo(t)
	hostileGitDir := filepath.Join(hostile, ".git")
	for name, value := range map[string]string{
		"GIT_DIR":                          hostileGitDir,
		"GIT_WORK_TREE":                    hostile,
		"GIT_COMMON_DIR":                   hostileGitDir,
		"GIT_INDEX_FILE":                   filepath.Join(hostileGitDir, "index"),
		"GIT_OBJECT_DIRECTORY":             filepath.Join(hostileGitDir, "objects"),
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": filepath.Join(hostileGitDir, "objects"),
		"GIT_SHALLOW_FILE":                 filepath.Join(hostileGitDir, "shallow"),
		"GIT_GRAFT_FILE":                   filepath.Join(hostileGitDir, "info", "grafts"),
		"GIT_REPLACE_REF_BASE":             "refs/replace-hostile/",
	} {
		t.Setenv(name, value)
	}

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build() under hostile Git environment error = %v", err)
	}
	wantTree := strings.TrimSpace(gitSnapshotWithoutLocalEnv(t, repo, "rev-parse", "HEAD^{tree}"))
	if snapshot.BaseTree != wantTree || snapshot.CandidateTree != wantTree {
		t.Fatalf("snapshot trees = %q/%q, want repository tree %q", snapshot.BaseTree, snapshot.CandidateTree, wantTree)
	}

	store, err := AuthoritativeStore(context.Background(), repo, "hostile-env-lineage")
	if err != nil {
		t.Fatalf("AuthoritativeStore() under hostile Git environment error = %v", err)
	}
	wantCommonDir := strings.TrimSpace(gitSnapshotWithoutLocalEnv(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	wantStore := filepath.Join(wantCommonDir, "gentle-ai", "review-transactions", "v1", "hostile-env-lineage")
	if store.Dir != wantStore {
		t.Fatalf("authoritative store = %q, want %q", store.Dir, wantStore)
	}
}

func TestAuthoritativeStorePreservesLinkedWorktreeCommonDirectory(t *testing.T) {
	repo := initSnapshotRepo(t)
	worktree := filepath.Join(t.TempDir(), "linked-worktree")
	gitSnapshot(t, repo, "worktree", "add", "--detach", worktree, "HEAD")

	store, err := AuthoritativeStore(context.Background(), worktree, "worktree-lineage")
	if err != nil {
		t.Fatalf("AuthoritativeStore(linked worktree) error = %v", err)
	}
	wantCommonDir := strings.TrimSpace(gitSnapshotWithoutLocalEnv(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	want := filepath.Join(wantCommonDir, "gentle-ai", "review-transactions", "v1", "worktree-lineage")
	if store.Dir != want {
		t.Fatalf("linked-worktree store = %q, want common store %q", store.Dir, want)
	}
}

func gitSnapshotWithoutLocalEnv(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Env = []string{"HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("sanitized git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func TestStoreLoadChainBindsGenesisHeadAndOrderedIdentity(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	genesis, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatal(err)
	}
	head, err := store.Append(genesis, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatalf("LoadChain() error = %v", err)
	}
	if chain.GenesisRevision != genesis || chain.HeadRevision != head || len(chain.Records) != 2 || len(chain.Revisions) != 2 || !validSHA256(chain.Identity) {
		t.Fatalf("LoadChain() = %#v", chain)
	}
}

func TestValidateSuccessorRejectsTamperedOutOfScopeFixSnapshot(t *testing.T) {
	previous := ordinaryAtFixing(t)
	next := *previous
	next.State = StateFixValidating
	next.Snapshot = previous.Snapshot
	next.Snapshot.Kind = TargetFixDiff
	next.Snapshot.BaseTree = previous.FinalCandidateTree
	next.Snapshot.CandidateTree = tree("c")
	next.Snapshot.LedgerIDs = []string{"R1-DET"}
	next.Snapshot.Paths = []string{"internal/tampered.go"}
	next.Snapshot.Identity = hash("3")
	next.FinalCandidateTree = next.Snapshot.CandidateTree
	next.FixDeltaHash = FixDeltaHashForSnapshot(next.Snapshot)

	if err := validateSuccessor(*previous, next, "review/complete-fix"); err == nil {
		t.Fatal("validateSuccessor() accepted a hash-valid fix snapshot outside genesis scope")
	}
}

func approvedStoreTransaction(t *testing.T, lineage string) Transaction {
	t.Helper()
	tx := newTestTransaction(t, ModeOrdinary4R)
	tx.LineageID = lineage
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(hash("2"), true); err != nil {
		t.Fatal(err)
	}
	return *tx
}

func testReleaseEvidence(releaseTree string) ReleaseEvidence {
	return ReleaseEvidence{
		ReleaseTree: releaseTree, ConfigurationHash: hash("2"),
		GeneratedArtifactHash: hash("3"), ProvenanceHash: hash("4"),
		PublicationBoundaryHash: hash("5"), PublicationState: PublicationStateSealed,
		EvidenceFreshnessHash: hash("6"), EvidenceFreshnessState: EvidenceFreshnessCurrent,
	}
}

func writeStoreEvent(t *testing.T, store Store, record Record) string {
	t.Helper()
	record.Schema = RecordSchema
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	revision := "sha256:" + hex.EncodeToString(sum[:])
	writeStoreEventPayload(t, store, revision, payload)
	return revision
}

func writeStoreEventAtRevision(t *testing.T, store Store, revision string, record Record) {
	t.Helper()
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeStoreEventPayload(t, store, revision, append(payload, '\n'))
}

func writeStoreEventPayload(t *testing.T, store Store, revision string, payload []byte) {
	t.Helper()
	events := filepath.Join(store.Dir, "events")
	if err := os.MkdirAll(events, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(events, strings.TrimPrefix(revision, "sha256:")+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte(revision+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
