package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAssessAuthorityRepairAtRepositoryRootAddsNoGitSubprocess(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	originalCommand := gitCommandContext
	t.Cleanup(func() { gitCommandContext = originalCommand })
	count := 0
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		count++
		return originalCommand(ctx, name, args...)
	}
	assessment, err := AssessAuthorityRepairAtRepositoryRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := assessment.Validate(); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("authority repair assessment added %d Git subprocesses after STATUS root resolution", count)
	}
}

func TestAssessAuthorityRepairDerivesExactCurrentLegacyHeadDeterministically(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, _, offending := legacyAliasRepairFixture(t, repo, "classified-alias")
	offendingRecord, _, err := store.loadRevision(offending)
	if err != nil {
		t.Fatal(err)
	}
	successor := offendingRecord.Transaction
	if err := successor.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	currentHead := writeStoreEvent(t, store, Record{
		Operation: "review/begin-final-verification", PreviousRevision: offending, Transaction: successor,
	})
	if currentHead == offending {
		t.Fatal("fixture did not advance legacy HEAD beyond the offending alias event")
	}
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, root)

	first, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated assessment changed:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	if first.Status != AuthorityRepairEligible || first.Class != AuthorityRepairClassLegacyV1HistoricalAlias ||
		first.Cause != AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias || first.Disposition != AuthorityRepairDispositionQuarantineHistoricalAlias ||
		first.Candidate == nil || first.Candidate.LineageID != "classified-alias" || first.Candidate.Revision != currentHead ||
		first.Candidate.Revision == offending || first.Candidate.ChainIdentity == "" ||
		!reflect.DeepEqual(first.Candidate.Operations, []string{"review/validate-fix"}) ||
		first.RepositoryBinding == "" || first.AuthorizationSchema != AuthorityRepairAuthorizationSchema {
		t.Fatalf("eligible assessment = %#v", first)
	}
	if first.Counts.Events != first.Candidate.EventCount {
		t.Fatalf("public unique event count = %d, want candidate cardinality %d", first.Counts.Events, first.Candidate.EventCount)
	}
	if after := authorityBytes(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("repair assessment changed authority bytes")
	}
	payload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(repo)) || bytes.Contains(payload, []byte(store.Dir)) {
		t.Fatalf("repair assessment leaked a private path: %s", payload)
	}

	linked := filepath.Join(t.TempDir(), "linked")
	gitSnapshot(t, repo, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { gitSnapshot(t, repo, "worktree", "remove", "--force", linked) })
	linkedAssessment, err := AssessAuthorityRepair(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	if linkedAssessment.RepositoryBinding != first.RepositoryBinding {
		t.Fatalf("linked repository binding = %q, want %q", linkedAssessment.RepositoryBinding, first.RepositoryBinding)
	}
}

func TestAssessAuthorityRepairStopsUnknownMultipleMixedAndCollidingInventory(t *testing.T) {
	for _, tt := range []struct {
		name string
		want AuthorityRepairStatus
		make func(t *testing.T, repo string)
	}{
		{
			name: "unknown alias", want: AuthorityRepairUnsupported,
			make: func(t *testing.T, repo string) {
				legacyAliasRepairFixture(t, repo, "unknown-alias", "review/unknown-fix")
			},
		},
		{
			name: "multiple eligible", want: AuthorityRepairAmbiguous,
			make: func(t *testing.T, repo string) {
				legacyAliasRepairFixture(t, repo, "alias-one")
				legacyAliasRepairFixture(t, repo, "alias-two", "review/complete-fix")
			},
		},
		{
			name: "eligible plus unknown", want: AuthorityRepairAmbiguous,
			make: func(t *testing.T, repo string) {
				legacyAliasRepairFixture(t, repo, "alias-known")
				legacyAliasRepairFixture(t, repo, "alias-unknown", "review/unknown-fix")
			},
		},
		{
			name: "eligible plus compact atomic residue", want: AuthorityRepairAmbiguous,
			make: func(t *testing.T, repo string) {
				legacyAliasRepairFixture(t, repo, "alias-with-residue")
				state := newCompactTestState(t, repo, "compact-reset-residue")
				store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Replace("", "review/start", state); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(store.Dir, ".atomic-interrupted"), []byte("residue\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "v1 v2 collision", want: AuthorityRepairConflicting,
			make: func(t *testing.T, repo string) {
				legacyAliasRepairFixture(t, repo, "alias-collision")
				state := newCompactTestState(t, repo, "alias-collision")
				store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Replace("", "review/start", state); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "valid invalidated is not a candidate", want: AuthorityRepairUnsupported,
			make: func(t *testing.T, repo string) {
				state := newCompactTestState(t, repo, "valid-invalidated")
				store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
				if err != nil {
					t.Fatal(err)
				}
				revision, err := store.Replace("", "review/start", state)
				if err != nil {
					t.Fatal(err)
				}
				if err := state.Invalidate("obsolete"); err != nil {
					t.Fatal(err)
				}
				if _, err := store.Replace(revision, "review/invalidate", state); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			tt.make(t, repo)
			assessment, err := AssessAuthorityRepair(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if err := assessment.Validate(); err != nil {
				t.Fatal(err)
			}
			if assessment.Status != tt.want || assessment.Candidate != nil || assessment.Class != "" ||
				assessment.Cause != "" || assessment.Disposition != "" || assessment.RepositoryBinding != "" {
				t.Fatalf("stopped assessment = %#v", assessment)
			}
		})
	}
}

func TestAssessAuthorityRepairStopsLockedAndTruncatedInventory(t *testing.T) {
	t.Run("lineage lock", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, _, _ := legacyAliasRepairFixture(t, repo, "alias-local-lock")
		lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.release()
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairConflicting || assessment.Candidate != nil {
			t.Fatalf("local-lock assessment = %#v, %v", assessment, err)
		}
	})

	t.Run("maintenance lock", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		legacyAliasRepairFixture(t, repo, "alias-maintenance-lock")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		lock, err := AcquireReviewMaintenanceExclusive(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairConflicting || assessment.Candidate != nil {
			t.Fatalf("maintenance-lock assessment = %#v, %v", assessment, err)
		}
	})

	t.Run("oversized event", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, head, _ := legacyAliasRepairFixture(t, repo, "alias-oversized")
		path := filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json")
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), authorityRepairMaxEventBytes+1), 0o644); err != nil {
			t.Fatal(err)
		}
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairTruncated || assessment.Candidate != nil {
			t.Fatalf("truncated assessment = %#v, %v", assessment, err)
		}
	})

	t.Run("too many unexpected entries", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		versionRoot := filepath.Join(root, "v1")
		if err := os.MkdirAll(versionRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		for index := 0; index <= authorityRepairMaxLineages; index++ {
			path := filepath.Join(versionRoot, fmt.Sprintf("unexpected-%03d.json", index))
			if err := os.WriteFile(path, []byte("residue\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil || assessment.Status != AuthorityRepairTruncated || assessment.Candidate != nil {
			t.Fatalf("entry-bound assessment = %#v, %v", assessment, err)
		}
	})
}

func TestAuthorityRepairBudgetAccepts1024UniqueEventsAndRejects1025(t *testing.T) {
	for _, tt := range []struct {
		name      string
		events    int
		truncated bool
	}{
		{name: "maximum", events: authorityRepairMaxEvents},
		{name: "over maximum", events: authorityRepairMaxEvents + 1, truncated: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			counts := AuthorityRepairCounts{}
			budget := authorityRepairBudget{
				ctx: context.Background(), counts: &counts,
				uniqueFiles: map[string]struct{}{}, uniqueEvents: map[string]struct{}{},
			}
			paths := make([]string, tt.events)
			for index := range paths {
				paths[index] = filepath.Join(dir, fmt.Sprintf("%04d.json", index))
				if err := os.WriteFile(paths[index], []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var readErr error
			for pass := 0; pass < 2 && readErr == nil; pass++ {
				for _, path := range paths {
					if _, readErr = budget.readEvent(path, authorityRepairMaxEventBytes); readErr != nil {
						break
					}
				}
			}
			if tt.truncated {
				if !errors.Is(readErr, errAuthorityRepairTruncated) || counts.Events != authorityRepairMaxEvents {
					t.Fatalf("over-bound scan = events %d, error %v", counts.Events, readErr)
				}
				return
			}
			if readErr != nil || counts.Events != authorityRepairMaxEvents || counts.Bytes != authorityRepairMaxEvents ||
				budget.proofReads != 2*authorityRepairMaxEvents {
				t.Fatalf("bounded two-pass scan = counts %#v, reads %d, error %v", counts, budget.proofReads, readErr)
			}
		})
	}
}

func TestAuthorityRepairAuthorizationRejectsNonExactBindings(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "alias-authorization")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil || assessment.Candidate == nil {
		t.Fatalf("assessment = %#v, %v", assessment, err)
	}
	request := ClassifiedAuthorityRepairRequest{
		Class: assessment.Class, LineageID: assessment.Candidate.LineageID, ExpectedRevision: head,
		Cause: assessment.Cause, Disposition: assessment.Disposition, RepositoryBinding: assessment.RepositoryBinding,
		Actor: "maintainer@example.com", Reason: "quarantine the approved historical alias",
	}
	request.MaintainerAuthorization = authorityRepairAuthorizationBinding(request)
	if err := validateClassifiedAuthorityRepairRequest(request, assessment); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ClassifiedAuthorityRepairRequest){
		func(value *ClassifiedAuthorityRepairRequest) { value.MaintainerAuthorization += "\n" },
		func(value *ClassifiedAuthorityRepairRequest) {
			value.MaintainerAuthorization = strings.ReplaceAll(value.MaintainerAuthorization, "\n", "\r\n")
		},
		func(value *ClassifiedAuthorityRepairRequest) { value.Actor = "other@example.com" },
		func(value *ClassifiedAuthorityRepairRequest) { value.ExpectedRevision = hash("f") },
		func(value *ClassifiedAuthorityRepairRequest) { value.RepositoryBinding = hash("e") },
	} {
		changed := request
		mutate(&changed)
		if err := validateClassifiedAuthorityRepairRequest(changed, assessment); err == nil {
			t.Fatalf("non-exact repair request accepted: %#v", changed)
		}
	}
	if !errors.Is(&AuthorityLockCancelledError{Cause: context.Canceled}, context.Canceled) {
		t.Fatal("test precondition: cancellation cause not preserved")
	}
}

func TestRepairClassifiedAuthorityCommitsAndReplaysExactAssessment(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, _, _ := legacyAliasRepairFixture(t, repo, "classified-execute")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	committed, err := RepairClassifiedAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Status != "committed" || committed.Class != assessment.Class || committed.Cause != assessment.Cause ||
		committed.Disposition != assessment.Disposition || committed.LineageID != request.LineageID ||
		committed.Revision != request.ExpectedRevision || committed.ChainIdentity != assessment.Candidate.ChainIdentity {
		t.Fatalf("classified repair execution = %#v", committed)
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatalf("classified repair source remains: %v", err)
	}
	replayed, err := RepairClassifiedAuthority(context.Background(), repo, request)
	if err != nil || !reflect.DeepEqual(replayed, committed) {
		t.Fatalf("classified repair replay = %#v, %v", replayed, err)
	}
}

func TestRepairClassifiedAuthorityRechecksHeadCASBeforeMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, _, _ := legacyAliasRepairFixture(t, repo, "classified-stale-head")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	originalHook := classifiedAuthorityRepairAfterAssessmentHook
	t.Cleanup(func() { classifiedAuthorityRepairAfterAssessmentHook = originalHook })
	classifiedAuthorityRepairAfterAssessmentHook = func() {
		record, _, loadErr := store.loadRevision(request.ExpectedRevision)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		next := record.Transaction
		if beginErr := next.BeginFinalVerification(); beginErr != nil {
			t.Fatal(beginErr)
		}
		writeStoreEvent(t, store, Record{Operation: "review/begin-final-verification", PreviousRevision: request.ExpectedRevision, Transaction: next})
	}
	if _, err := RepairClassifiedAuthority(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale classified repair = %v", err)
	}
	if _, err := os.Stat(store.Dir); err != nil {
		t.Fatalf("stale classified repair moved source: %v", err)
	}
}

func TestRepairClassifiedAuthorityRechecksCompleteInventoryUnderMaintenanceCAS(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, _, _ := legacyAliasRepairFixture(t, repo, "classified-global-cas")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	originalHook := classifiedAuthorityRepairAfterAssessmentHook
	t.Cleanup(func() { classifiedAuthorityRepairAfterAssessmentHook = originalHook })
	classifiedAuthorityRepairAfterAssessmentHook = func() {
		legacyAliasRepairFixture(t, repo, "classified-racing-candidate")
	}
	if _, err := RepairClassifiedAuthority(context.Background(), repo, request); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("classified repair accepted a newly ambiguous inventory: %v", err)
	}
	if _, err := os.Stat(store.Dir); err != nil {
		t.Fatalf("ambiguous classified repair moved original source: %v", err)
	}
}

func TestRepairClassifiedAuthorityConcurrentExecutionCommitsAndReplays(t *testing.T) {
	repo := initSnapshotRepo(t)
	legacyAliasRepairFixture(t, repo, "classified-concurrent")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	originalHook := classifiedAuthorityRepairAfterAssessmentHook
	t.Cleanup(func() { classifiedAuthorityRepairAfterAssessmentHook = originalHook })
	ready := make(chan struct{})
	release := make(chan struct{})
	var arrivals atomic.Int32
	classifiedAuthorityRepairAfterAssessmentHook = func() {
		if arrivals.Add(1) == 2 {
			close(ready)
		}
		<-release
	}
	results := make(chan ClassifiedAuthorityRepairExecution, 2)
	errorsOut := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			result, runErr := RepairClassifiedAuthority(context.Background(), repo, request)
			results <- result
			errorsOut <- runErr
		}()
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent classified repairs did not finish assessment")
	}
	close(release)
	workers.Wait()
	first, second := <-results, <-results
	for range 2 {
		if err := <-errorsOut; err != nil {
			t.Fatalf("concurrent classified repair: %v", err)
		}
	}
	if !reflect.DeepEqual(first, second) || first.Status != "committed" {
		t.Fatalf("concurrent classified results = %#v / %#v", first, second)
	}
}

func TestRepairClassifiedAuthorityPreservesPreparedProgress(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, _, _ := legacyAliasRepairFixture(t, repo, "classified-prepared-progress")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	originalPersist := persistReclaimRecord
	t.Cleanup(func() { persistReclaimRecord = originalPersist })
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted {
			return errors.New("injected committed audit failure")
		}
		return originalPersist(record)
	}
	progress, err := RepairClassifiedAuthority(context.Background(), repo, request)
	if err == nil {
		t.Fatal("classified repair unexpectedly completed")
	}
	if progress.Status != CompactReclaimPrepared || progress.LineageID != request.LineageID || progress.Revision != request.ExpectedRevision {
		t.Fatalf("classified wrapper discarded prepared progress: %#v, %v", progress, err)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("prepared progress did not preserve moved-source truth: %v", statErr)
	}
}

func TestRepairClassifiedAuthorityInitialCommitRequiresPhysicalReadback(t *testing.T) {
	repo := initSnapshotRepo(t)
	legacyAliasRepairFixture(t, repo, "classified-initial-readback")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	originalPersist := persistReclaimRecord
	corrupted := false
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if err := originalPersist(record); err != nil {
			return err
		}
		if record.Status == CompactReclaimCommitted && !corrupted {
			corrupted = true
			events, err := os.ReadDir(filepath.Join(record.QuarantinePath, "residue", "events"))
			if err != nil || len(events) == 0 {
				t.Fatalf("read committed replay events = %d, %v", len(events), err)
			}
			path := filepath.Join(record.QuarantinePath, "residue", "events", events[0].Name())
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			payload[0] ^= 1
			if err := os.WriteFile(path, payload, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}
	t.Cleanup(func() { persistReclaimRecord = originalPersist })
	progress, err := RepairClassifiedAuthority(context.Background(), repo, request)
	var typed *ClassifiedAuthorityRepairProgressError
	if err == nil || !errors.As(err, &typed) || progress.Status != CompactReclaimCommitted || typed.Progress != progress || !typed.ExactReplaySafe {
		t.Fatalf("classified repair accepted corrupt initial commit readback: %#v, %T %v", progress, err, err)
	}
}

func TestRepairClassifiedAuthorityResumesEachDurablePhase(t *testing.T) {
	for _, phase := range []string{compactReclaimPhasePrepared, compactReclaimPhaseRenamed, compactReclaimPhaseCommitted} {
		t.Run(phase, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			store, _, _ := legacyAliasRepairFixture(t, repo, "classified-resume-"+phase)
			assessment, err := AssessAuthorityRepair(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			request := classifiedAuthorityRepairRequest(t, assessment)
			originalHook := compactReclaimPhaseHook
			injected := errors.New("injected " + phase + " interruption")
			fired := false
			compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
				if current == phase && !fired {
					fired = true
					return injected
				}
				return originalHook(ctx, current, record)
			}
			t.Cleanup(func() { compactReclaimPhaseHook = originalHook })
			progress, err := RepairClassifiedAuthority(context.Background(), repo, request)
			if !errors.Is(err, injected) {
				t.Fatalf("phase %s error = %v", phase, err)
			}
			var typed *ClassifiedAuthorityRepairProgressError
			if !errors.As(err, &typed) || typed.Progress != progress || !validSHA256(progress.RecordIdentity) ||
				!validSHA256(progress.RequestDigest) || !validSHA256(progress.AssessmentDigest) ||
				!typed.ExactReplaySafe {
				t.Fatalf("phase %s progress = %#v, %T %v", phase, progress, err, err)
			}
			wantStatus := CompactReclaimPrepared
			if phase == compactReclaimPhaseCommitted {
				wantStatus = CompactReclaimCommitted
			}
			if progress.Status != wantStatus {
				t.Fatalf("phase %s status = %q, want %q", phase, progress.Status, wantStatus)
			}
			compactReclaimPhaseHook = originalHook
			replayed, err := RepairClassifiedAuthority(context.Background(), repo, request)
			if err != nil || replayed.Status != CompactReclaimCommitted || replayed.RecordIdentity != progress.RecordIdentity {
				t.Fatalf("phase %s restart = %#v, %v", phase, replayed, err)
			}
			if _, err := os.Lstat(store.Dir); !os.IsNotExist(err) {
				t.Fatalf("phase %s restart left source: %v", phase, err)
			}
		})
	}

	t.Run("prepare persistence failure is not durable progress", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		store, _, _ := legacyAliasRepairFixture(t, repo, "classified-prepare-failure")
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		request := classifiedAuthorityRepairRequest(t, assessment)
		originalPersist := persistReclaimRecord
		injected := errors.New("injected prepare persistence failure")
		persistReclaimRecord = func(record CompactReclaimRecord) error {
			if record.Status == CompactReclaimPrepared {
				return injected
			}
			return originalPersist(record)
		}
		t.Cleanup(func() { persistReclaimRecord = originalPersist })
		progress, err := RepairClassifiedAuthority(context.Background(), repo, request)
		var typed *ClassifiedAuthorityRepairProgressError
		if !errors.Is(err, injected) || errors.As(err, &typed) || progress != (ClassifiedAuthorityRepairExecution{}) {
			t.Fatalf("prepare failure invented durable progress: %#v, %T %v", progress, err, err)
		}
		if _, err := os.Lstat(store.Dir); err != nil {
			t.Fatalf("prepare failure moved source: %v", err)
		}
		persistReclaimRecord = originalPersist
		if replayed, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil || replayed.Status != CompactReclaimCommitted {
			t.Fatalf("repair after nondurable prepare failure = %#v, %v", replayed, err)
		}
	})
}

func TestRepairClassifiedAuthorityTimeoutsPreserveEachDurablePhase(t *testing.T) {
	for _, phase := range []string{compactReclaimPhasePrepared, compactReclaimPhaseRenamed, compactReclaimPhaseCommitted} {
		t.Run(phase, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			legacyAliasRepairFixture(t, repo, "classified-timeout-"+phase)
			assessment, err := AssessAuthorityRepair(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			request := classifiedAuthorityRepairRequest(t, assessment)
			originalHook := compactReclaimPhaseHook
			compactReclaimPhaseHook = func(ctx context.Context, current string, record CompactReclaimRecord) error {
				if current == phase {
					<-ctx.Done()
					return ctx.Err()
				}
				return originalHook(ctx, current, record)
			}
			t.Cleanup(func() { compactReclaimPhaseHook = originalHook })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			progress, err := RepairClassifiedAuthority(ctx, repo, request)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("phase %s timeout = %v", phase, err)
			}
			var typed *ClassifiedAuthorityRepairProgressError
			if !errors.As(err, &typed) || !typed.ExactReplaySafe {
				t.Fatalf("phase %s timeout replay truth = %T %#v", phase, err, typed)
			}
			wantStatus := CompactReclaimPrepared
			if phase == compactReclaimPhaseCommitted {
				wantStatus = CompactReclaimCommitted
			}
			if progress.Status != wantStatus || !validSHA256(progress.RecordIdentity) {
				t.Fatalf("phase %s timeout progress = %#v", phase, progress)
			}
			compactReclaimPhaseHook = originalHook
			if replayed, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil || replayed.Status != CompactReclaimCommitted {
				t.Fatalf("phase %s timeout restart = %#v, %v", phase, replayed, err)
			}
		})
	}
}

func TestRepairClassifiedAuthorityRejectsCompatibilityReplayOrigin(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, head, _ := legacyAliasRepairFixture(t, repo, "classified-origin")
	assessment, err := AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	request := classifiedAuthorityRepairRequest(t, assessment)
	legacyAliasRepairFixture(t, repo, "unrelated-unknown", "review/unknown-fix")
	_, repository, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	compatibility := LegacyAliasRepairRequest{
		LineageID: request.LineageID, ExpectedRevision: head, ExpectedDiagnostic: legacyAliasRepairDiagnostic,
		Disposition: LegacyAliasRepairDisposition, Actor: request.Actor, Reason: request.Reason,
	}
	compatibility.MaintainerAuthorization = legacyAliasRepairAuthorizationBinding(
		repository, compatibility.LineageID, compatibility.ExpectedRevision, compatibility.ExpectedDiagnostic,
		compatibility.Disposition, compatibility.Actor, compatibility.Reason,
	)
	if _, err := RepairHistoricalLegacyAlias(context.Background(), repo, compatibility); err != nil {
		t.Fatal(err)
	}
	if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err == nil {
		t.Fatal("classified repair adopted a compatibility-origin audit record")
	}
}

func TestRepairClassifiedAuthorityReplayRejectsCorruptAuditAndResidue(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, recordPath, residuePath string)
	}{
		{
			name: "unknown audit field",
			mutate: func(t *testing.T, recordPath, _ string) {
				payload, err := os.ReadFile(recordPath)
				if err != nil {
					t.Fatal(err)
				}
				var raw map[string]any
				if err := json.Unmarshal(payload, &raw); err != nil {
					t.Fatal(err)
				}
				raw["untrusted"] = true
				payload, err = json.Marshal(raw)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(recordPath, payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "missing residue",
			mutate: func(t *testing.T, _ string, residuePath string) {
				if err := os.RemoveAll(residuePath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "forged chain identity",
			mutate: func(t *testing.T, recordPath, _ string) {
				payload, err := os.ReadFile(recordPath)
				if err != nil {
					t.Fatal(err)
				}
				var record CompactReclaimRecord
				if err := json.Unmarshal(payload, &record); err != nil {
					t.Fatal(err)
				}
				record.LegacyAliasRepair.ChainIdentity = hash("f")
				payload, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(recordPath, payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "replaced event",
			mutate: func(t *testing.T, _ string, residuePath string) {
				events, err := os.ReadDir(filepath.Join(residuePath, "events"))
				if err != nil || len(events) == 0 {
					t.Fatalf("read replay events = %d, %v", len(events), err)
				}
				path := filepath.Join(residuePath, "events", events[0].Name())
				payload, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				payload[0] ^= 1
				if err := os.WriteFile(path, payload, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "oversized replay record",
			mutate: func(t *testing.T, recordPath, _ string) {
				if err := os.WriteFile(recordPath, bytes.Repeat([]byte("x"), authorityRepairMaxEventBytes+1), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked nested event directory escape",
			mutate: func(t *testing.T, _ string, residuePath string) {
				eventsPath := filepath.Join(residuePath, "events")
				outside := filepath.Join(t.TempDir(), "events")
				if err := os.Rename(eventsPath, outside); err != nil {
					t.Skipf("move events outside residue unavailable: %v", err)
				}
				if err := os.Symlink(outside, eventsPath); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			legacyAliasRepairFixture(t, repo, "classified-corrupt-replay")
			assessment, err := AssessAuthorityRepair(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			request := classifiedAuthorityRepairRequest(t, assessment)
			if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil {
				t.Fatal(err)
			}
			base, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
			if err != nil || len(entries) != 1 {
				t.Fatalf("quarantine entries = %d, %v", len(entries), err)
			}
			quarantinePath := filepath.Join(base, "quarantine", entries[0].Name())
			tt.mutate(t, filepath.Join(quarantinePath, "reclaim-record.json"), filepath.Join(quarantinePath, "residue"))
			if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err == nil {
				t.Fatal("classified replay accepted corrupt audit or residue")
			}
		})
	}
}

func TestRepairClassifiedAuthorityReplayRequiresUniqueRecordAndGlobalInventory(t *testing.T) {
	t.Run("duplicate classified records", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		legacyAliasRepairFixture(t, repo, "classified-duplicate-replay")
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		request := classifiedAuthorityRepairRequest(t, assessment)
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil {
			t.Fatal(err)
		}
		base, quarantinePath, recordPath, _ := classifiedAuthorityRepairPaths(t, repo)
		payload, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		var record CompactReclaimRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			t.Fatal(err)
		}
		duplicatePath := filepath.Join(base, "quarantine", request.LineageID+"-duplicate")
		if duplicatePath == quarantinePath {
			t.Fatal("duplicate path did not differ")
		}
		if err := os.MkdirAll(duplicatePath, 0o700); err != nil {
			t.Fatal(err)
		}
		record.QuarantinePath = duplicatePath
		payload, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(duplicatePath, "reclaim-record.json"), payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("classified replay duplicate result = %v", err)
		}
	})

	t.Run("quarantine directory bound", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		legacyAliasRepairFixture(t, repo, "classified-bounded-replay")
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		request := classifiedAuthorityRepairRequest(t, assessment)
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil {
			t.Fatal(err)
		}
		base, _, _, _ := classifiedAuthorityRepairPaths(t, repo)
		for index := 0; index < authorityRepairMaxQuarantineRecords; index++ {
			if err := os.Mkdir(filepath.Join(base, "quarantine", fmt.Sprintf("noise-%03d", index)), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); !errors.Is(err, errAuthorityRepairTruncated) {
			t.Fatalf("classified replay directory bound = %v", err)
		}
	})

	t.Run("global inventory changed after commit", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		legacyAliasRepairFixture(t, repo, "classified-global-replay")
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		request := classifiedAuthorityRepairRequest(t, assessment)
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil {
			t.Fatal(err)
		}
		legacyAliasRepairFixture(t, repo, "classified-new-global-candidate")
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "full-inventory") {
			t.Fatalf("classified replay accepted changed global inventory: %v", err)
		}
	})

	t.Run("cancelled bounded discovery", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		legacyAliasRepairFixture(t, repo, "classified-cancelled-replay")
		assessment, err := AssessAuthorityRepair(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		request := classifiedAuthorityRepairRequest(t, assessment)
		if _, err := RepairClassifiedAuthority(context.Background(), repo, request); err != nil {
			t.Fatal(err)
		}
		base, _, _, _ := classifiedAuthorityRepairPaths(t, repo)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := discoverClassifiedAuthorityRepairRecord(ctx, base, request); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled classified replay discovery = %v", err)
		}
	})
}

func classifiedAuthorityRepairPaths(t *testing.T, repo string) (base, quarantinePath, recordPath, residuePath string) {
	t.Helper()
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("quarantine entries = %d, %v", len(entries), err)
	}
	quarantinePath = filepath.Join(base, "quarantine", entries[0].Name())
	return base, quarantinePath, filepath.Join(quarantinePath, "reclaim-record.json"), filepath.Join(quarantinePath, "residue")
}

func classifiedAuthorityRepairRequest(t *testing.T, assessment AuthorityRepairAssessment) ClassifiedAuthorityRepairRequest {
	t.Helper()
	if assessment.Status != AuthorityRepairEligible || assessment.Candidate == nil {
		t.Fatalf("repair assessment is not eligible: %#v", assessment)
	}
	request := ClassifiedAuthorityRepairRequest{
		Class: assessment.Class, LineageID: assessment.Candidate.LineageID, ExpectedRevision: assessment.Candidate.Revision,
		Cause: assessment.Cause, Disposition: assessment.Disposition, RepositoryBinding: assessment.RepositoryBinding,
		Actor: "maintainer@example.com", Reason: "quarantine the approved historical alias",
	}
	request.MaintainerAuthorization = authorityRepairAuthorizationBinding(request)
	return request
}
