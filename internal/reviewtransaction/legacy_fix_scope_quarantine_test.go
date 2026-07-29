package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// legacyFixScopeFixture reproduces the three historical Gentle Pi shapes: two
// terminal complete-fix records and one followed by validate-fix.
func legacyFixScopeFixture(t *testing.T, repo, lineage string, added []string, successor bool) (Store, string, string) {
	t.Helper()
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	tx := newTestTransaction(t, ModeOrdinary4R)
	tx.LineageID, tx.GenesisPaths = lineage, nil
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	if err := freezeTestFindings(tx, []Finding{{ID: "R1-SCOPE", Severity: "CRITICAL"}}); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: head, Transaction: *tx})
	if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-SCOPE", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "historical proof"}}); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: head, Transaction: *tx})
	if err := tx.BeginFix(hash("a")); err != nil {
		t.Fatal(err)
	}
	head = writeStoreEvent(t, store, Record{Operation: "review/begin-fix", PreviousRevision: head, Transaction: *tx})
	fix := tx.Snapshot
	fix.Kind, fix.BaseTree, fix.CandidateTree, fix.LedgerIDs = TargetFixDiff, tx.FinalCandidateTree, tree("c"), []string{"R1-SCOPE"}
	fix.Paths, err = canonicalPaths(append(append([]string{}, fix.Paths...), added...))
	if err != nil {
		t.Fatal(err)
	}
	fix.PathsDigest, fix.Identity = digestPaths(fix.Paths), hash("c")
	completed := *tx
	completed.Snapshot, completed.FinalCandidateTree, completed.FixDeltaHash, completed.State, completed.GenesisPaths = fix, fix.CandidateTree, FixDeltaHashForSnapshot(fix), StateFixValidating, nil
	complete := writeStoreEvent(t, store, Record{Operation: "review/complete-fix", PreviousRevision: head, Transaction: completed})
	head = complete
	if successor {
		validated := historicalValidationTransition(completed)
		head = writeStoreEvent(t, store, Record{Operation: "review/validate-fix", PreviousRevision: head, Transaction: validated})
	}
	return store, head, complete
}

func legacyFixScopeRequest(t *testing.T, repo, lineage, head, event, alias string) LegacyFixScopeQuarantineRequest {
	t.Helper()
	_, repository, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	r := LegacyFixScopeQuarantineRequest{LineageID: lineage, ExpectedRevision: head, ExpectedDiagnostic: legacyFixScopeInventoryDiagnostic(event), Disposition: LegacyFixScopeQuarantineDisposition, ExpectedAnomalySet: legacyFixScopeScopeOnlyAnomalySet, Reason: "quarantine exact historical complete-fix scope expansion", Actor: "maintainer@example.com"}
	if alias != "" {
		r.ExpectedAnomalySet, r.ExpectedValidateFixAliasRevision, r.ExpectedValidateFixAliasOperation = legacyFixScopeCombinedAnomalySet, alias, "review/validate-fix"
	}
	r.MaintainerAuthorization = legacyFixScopeQuarantineAuthorizationBinding(repository, r.LineageID, r.ExpectedRevision, r.ExpectedDiagnostic, r.Disposition, r.ExpectedAnomalySet, r.ExpectedValidateFixAliasRevision, r.ExpectedValidateFixAliasOperation, r.Actor, r.Reason)
	return r
}

func legacyFixScopeAliasRevision(t *testing.T, store Store, head string) string {
	t.Helper()
	for revision := head; revision != ""; {
		record, loaded, err := store.loadRevision(revision)
		if err != nil {
			t.Fatal(err)
		}
		if record.Operation == "review/validate-fix" {
			return loaded
		}
		revision = record.PreviousRevision
	}
	t.Fatal("missing validate-fix alias")
	return ""
}

func TestQuarantineHistoricalLegacyFixScopeReproducesPiShapes(t *testing.T) {
	for _, tt := range []struct {
		name, lineage string
		added         []string
		successor     bool
	}{
		{"complete target", "issue-1099-complete-target-review-20260710", []string{"lib/sdd-preflight.ts"}, false},
		{"final complete", "issue-1099-final-complete-review-20260710", []string{"assets/agents/gentle-ai-worker.md"}, false},
		{"corrected candidate", "issue-1099-corrected-candidate-review-20260710", []string{"extensions/gentle-ai.ts", "tests/gentle-ai.test.ts"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, head, event := legacyFixScopeFixture(t, repo, tt.lineage, tt.added, tt.successor)
			alias := ""
			if tt.successor {
				alias = legacyFixScopeAliasRevision(t, store, head)
			}
			request := legacyFixScopeRequest(t, repo, tt.lineage, head, event, alias)
			report, err := InventoryAuthority(context.Background(), repo)
			if err != nil || len(report.Entries) != 1 || !reflect.DeepEqual(report.Entries[0].Problems, []string{request.ExpectedDiagnostic}) {
				t.Fatalf("inventory diagnostic = %#v, %v", report, err)
			}
			committed, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			proof := committed.LegacyFixScopeQuarantine
			if committed.Status != CompactReclaimCommitted || proof == nil || proof.EventRevision != event || proof.HeadRevision != head || !reflect.DeepEqual(proof.ExpandedCorrectionPaths, tt.added) || proof.AnomalySet != request.ExpectedAnomalySet {
				t.Fatalf("invalid quarantine proof: %#v", committed)
			}
			if _, err := os.Lstat(store.Dir); !os.IsNotExist(err) {
				t.Fatalf("source remains: %v", err)
			}
			replayed, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
			if err != nil || !reflect.DeepEqual(replayed, committed) {
				t.Fatalf("replay = %#v, %v", replayed, err)
			}
		})
	}
}

func legacyFixScopePhysicalChild(store Store, head, name string) string {
	if name == "event" {
		return filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json")
	}
	return filepath.Join(store.Dir, name)
}

func TestQuarantineHistoricalLegacyFixScopeRejectsSymlinkedPhysicalChildren(t *testing.T) {
	for _, tt := range []struct {
		name, child string
		residue     bool
	}{
		{"source HEAD", "HEAD", false}, {"source events", "events", false}, {"source event", "event", false}, {"source LOCK", "LOCK", false},
		{"residue HEAD", "HEAD", true}, {"residue events", "events", true}, {"residue event", "event", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, head, event := legacyFixScopeFixture(t, repo, "symlink-"+strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"), []string{"lib/scope.go"}, false)
			request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
			if tt.residue {
				record, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
				if err != nil {
					t.Fatal(err)
				}
				store.Dir = filepath.Join(record.QuarantinePath, "residue")
			}
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("must not change"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := legacyFixScopePhysicalChild(store, head, tt.child)
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "unsafe physical") {
				t.Fatalf("symlink refusal = %v", err)
			}
			if payload, err := os.ReadFile(target); err != nil || string(payload) != "must not change" {
				t.Fatalf("symlink target was changed: %q, %v", payload, err)
			}
		})
	}
}

func TestQuarantineHistoricalLegacyFixScopeRejectsUnauthorizedClasses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*LegacyFixScopeQuarantineRequest)
		want   string
	}{
		{"stale CAS", func(r *LegacyFixScopeQuarantineRequest) { r.ExpectedRevision = hash("f") }, ErrConcurrentUpdate.Error()},
		{"reordered anomalies", func(r *LegacyFixScopeQuarantineRequest) {
			r.ExpectedAnomalySet = "validate_fix_alias,complete_fix_scope_expansion"
		}, "supports only canonical"},
		{"scope alias fields", func(r *LegacyFixScopeQuarantineRequest) { r.ExpectedValidateFixAliasOperation = "review/validate-fix" }, "scope-only anomaly set forbids"},
		{"wrong diagnostic", func(r *LegacyFixScopeQuarantineRequest) { r.ExpectedDiagnostic = "other" }, "supports only"},
		{"multiline actor", func(r *LegacyFixScopeQuarantineRequest) { r.Actor += "\nextra" }, "LF-only"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, head, event := legacyFixScopeFixture(t, repo, "reject-"+strings.ReplaceAll(strings.ToLower(tt.name), " ", "-"), []string{"lib/scope.go"}, false)
			request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
			tt.mutate(&request)
			if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if _, err := os.Lstat(store.Dir); err != nil {
				t.Fatalf("refusal moved source: %v", err)
			}
		})
	}
}

func TestQuarantineHistoricalLegacyFixScopeRejectsFabricatedReplayAndRace(t *testing.T) {
	t.Run("fabricated audit", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, event := legacyFixScopeFixture(t, repo, "replay-fabricated", []string{"lib/scope.go"}, false)
		request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
		record, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		prepared := record
		prepared.Status, prepared.QuarantinePath = CompactReclaimPrepared, filepath.Join(filepath.Dir(record.QuarantinePath), store.lineageID+"-prepared")
		payload, err := json.Marshal(prepared)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(prepared.QuarantinePath, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prepared.QuarantinePath, "reclaim-record.json"), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if replayed, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err != nil || !reflect.DeepEqual(replayed, record) {
			t.Fatalf("prepared replay = %#v, %v", replayed, err)
		}
		conflictPath := filepath.Join(filepath.Dir(record.QuarantinePath), store.lineageID+"-conflict")
		payload, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(conflictPath, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(conflictPath, "reclaim-record.json"), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "mismatched committed replay audit path") {
			t.Fatalf("conflicting committed replay = %v", err)
		}
		if err := os.RemoveAll(conflictPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"), []byte(`{"schema":"gentle-ai.review-reclaim-record/v1","status":"committed","lineage_id":"`+record.LineageID+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "incomplete or mismatched") {
			t.Fatalf("fabricated replay = %v", err)
		}
	})
	t.Run("maintenance before local lock rechecks source", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, event := legacyFixScopeFixture(t, repo, "recheck-source", []string{"lib/scope.go"}, false)
		request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
		entered, release := make(chan struct{}), make(chan struct{})
		var gate sync.Mutex
		blocked := false
		original := legacyFixScopeBeforeMaintenance
		legacyFixScopeBeforeMaintenance = func() {
			gate.Lock()
			wait := !blocked
			blocked = true
			gate.Unlock()
			if wait {
				close(entered)
				<-release
			}
		}
		t.Cleanup(func() { legacyFixScopeBeforeMaintenance = original })
		result := make(chan CompactReclaimRecord, 1)
		resultErr := make(chan error, 1)
		go func() {
			record, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
			result <- record
			resultErr <- err
		}()
		<-entered
		committed, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-resultErr; err != nil {
			t.Fatal(err)
		}
		if replayed := <-result; !reflect.DeepEqual(replayed, committed) {
			t.Fatalf("concurrent retry = %#v, want %#v", replayed, committed)
		}
	})
	t.Run("active lock", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, event := legacyFixScopeFixture(t, repo, "active-lock", []string{"lib/scope.go"}, false)
		lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.release()
		_, err = QuarantineHistoricalLegacyFixScope(context.Background(), repo, legacyFixScopeRequest(t, repo, store.lineageID, head, event, ""))
		if !errors.Is(err, ErrAuthorityLockTimeout) {
			t.Fatalf("lock error = %v", err)
		}
	})
}

func TestQuarantineHistoricalLegacyFixScopeReplayRejectsV2AndStaleResidue(t *testing.T) {
	t.Run("post-lock compact v2", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, event := legacyFixScopeFixture(t, repo, "replay-v2-race", []string{"lib/scope.go"}, false)
		request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
		if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err != nil {
			t.Fatal(err)
		}
		entered, release := make(chan struct{}), make(chan struct{})
		original := legacyFixScopeAfterMaintenance
		legacyFixScopeAfterMaintenance = func() { close(entered); <-release }
		t.Cleanup(func() { legacyFixScopeAfterMaintenance = original })
		result := make(chan error, 1)
		go func() {
			_, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
			result <- err
		}()
		<-entered
		base, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(base, "v2", store.lineageID), 0o755); err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-result; err == nil || !strings.Contains(err.Error(), "also exists in compact-v2 authority") {
			t.Fatalf("replay compact-v2 exclusion = %v", err)
		}
	})
	t.Run("orphan prepared", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, event := legacyFixScopeFixture(t, repo, "stale-prepared", []string{"lib/scope.go"}, false)
		request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
		base, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(base, "quarantine", store.lineageID+"-prepared")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(CompactReclaimRecord{Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: store.lineageID})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "reclaim-record.json"), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(store.Dir); err != nil {
			t.Fatal(err)
		}
		if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "stale or orphan prepared") {
			t.Fatalf("orphan prepared = %v", err)
		}
	})
}

func TestQuarantineHistoricalLegacyFixScopeRejectsUnexpectedPhysicalResidue(t *testing.T) {
	for _, residue := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "replay residue"}[residue], func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, head, event := legacyFixScopeFixture(t, repo, "unexpected-residue-"+map[bool]string{false: "source", true: "replay"}[residue], []string{"lib/scope.go"}, false)
			request := legacyFixScopeRequest(t, repo, store.lineageID, head, event, "")
			if residue {
				record, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request)
				if err != nil {
					t.Fatal(err)
				}
				store.Dir = filepath.Join(record.QuarantinePath, "residue")
			}
			if err := os.WriteFile(filepath.Join(store.Dir, "events", "orphan.json"), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := QuarantineHistoricalLegacyFixScope(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "non-HEAD-chain physical event") {
				t.Fatalf("unexpected nested residue = %v", err)
			}
		})
	}
}
