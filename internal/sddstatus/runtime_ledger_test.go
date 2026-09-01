package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRuntimeLedgerConsumesOrdinalBeforeLaunchAndChargesNativeLines(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "rename-resistant-change")
	if err != nil {
		t.Fatal(err)
	}

	beginOne := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-1", WorkUnit: "browser-harness",
		EvidenceGoal: "prove process containment", MaxAttempts: 2, MaxChangedLines: 4,
	}
	first, err := store.Begin(context.Background(), beginOne)
	if err != nil {
		t.Fatal(err)
	}
	if first.ActiveAttempt == nil || first.ActiveAttempt.Ordinal != 1 || first.CumulativeAttempts != 1 || first.NextOrdinal != 2 {
		t.Fatalf("first begin status = %#v", first)
	}
	if first.Objective == nil || first.Objective.WorkUnit != "browser-harness" || first.Objective.EvidenceGoal != "prove process containment" {
		t.Fatalf("first objective = %#v", first.Objective)
	}

	// An exact retry after the ordinal is durable is a read-only replay.
	replayed, err := store.Begin(context.Background(), beginOne)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != first.Revision || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("begin replay revision/records = %q/%d, want %q/1", replayed.Revision, countRuntimeRecords(t, store.Dir), first.Revision)
	}

	appendRuntimeLedgerFile(t, repo, "attempt-one\n")
	finishOne := FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: "finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('1'), Diagnosis: "selector ambiguity reproduced in the bounded harness",
		HarnessDisposition: HarnessReused, CleanupEvidence: "child process group exited before the harness returned",
		ProcessEvidence: "listener and descendant scan reported no surviving process",
	}
	failed, err := store.Finish(context.Background(), finishOne)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ActiveAttempt != nil || failed.CumulativeAttempts != 1 || failed.CumulativeChangedLines != 1 || failed.DecisionRequired {
		t.Fatalf("failed finish status = %#v", failed)
	}

	// A later attempt stays within the same immutable work unit and objective.
	beginTwo := BeginAttemptRequest{
		ExpectedRevision: failed.Revision, RequestID: "begin-2", WorkUnit: "browser-harness",
		EvidenceGoal: "prove process containment", MaxAttempts: 2, MaxChangedLines: 4,
	}
	second, err := store.Begin(context.Background(), beginTwo)
	if err != nil {
		t.Fatal(err)
	}
	if second.ActiveAttempt == nil || second.ActiveAttempt.Ordinal != 2 || second.CumulativeAttempts != 2 ||
		second.Objective == nil || second.Objective.ID != first.Objective.ID || second.Objective.WorkUnit != "browser-harness" {
		t.Fatalf("second begin = %#v", second)
	}

	appendRuntimeLedgerFile(t, repo, "attempt-two\n")
	// #3815: this settlement spends the second attempt so the objective is
	// genuinely exhausted, which is what the rest of this test is about. It used
	// to be an interrupted settlement; an interrupted call that delivered
	// increment now refunds the attempt instead of discharging it, and that rule
	// has its own coverage in runtime_budget_granularity_test.go.
	interrupted, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: second.Revision, RequestID: "finish-2", Outcome: AttemptFailed,
		EvidenceRevision:   runtimeTestHash('2'),
		Diagnosis:          "executor transport closed after bounded output capture",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "cleanup trap terminated the recorded process group",
		ProcessEvidence: "post-interruption process scan found no matching descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.CumulativeChangedLines != 2 || !interrupted.DecisionRequired || interrupted.NextAction != RuntimeActionReset {
		t.Fatalf("interrupted terminal status = %#v", interrupted)
	}
	head := interrupted.Revision
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: head, RequestID: "begin-3", WorkUnit: "browser-harness",
		EvidenceGoal: "prove process containment", MaxAttempts: 2, MaxChangedLines: 4,
	})
	if !errors.Is(err, ErrRuntimeBudgetExhausted) {
		t.Fatalf("third begin error = %v, want ErrRuntimeBudgetExhausted", err)
	}
	unchanged, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != head || countRuntimeRecords(t, store.Dir) != 4 {
		t.Fatalf("exhausted begin mutated authority: revision=%q records=%d", unchanged.Revision, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeLedgerRejectsUnchargedCandidateDriftBetweenAttempts(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "candidate-continuity")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "continuity-begin-1", WorkUnit: "runtime-proof",
		EvidenceGoal: "prove candidate continuity", MaxAttempts: 3, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "attempt-one\n")
	finished, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "continuity-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "first bounded attempt failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "first cleanup completed",
		ProcessEvidence: "first process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	finishBytes, err := os.ReadFile(filepath.Join(repo, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "unreviewed-between-attempts\n")
	beforeRecords := countRuntimeRecords(t, store.Dir)
	request := BeginAttemptRequest{
		ExpectedRevision: finished.Revision, RequestID: "continuity-begin-2", WorkUnit: "runtime-proof",
		EvidenceGoal: "prove candidate continuity", MaxAttempts: 3, MaxChangedLines: 20,
	}
	_, err = store.Begin(context.Background(), request)
	if !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("inter-attempt drift error = %T %v, want ErrRuntimeObjectiveChange", err, err)
	}
	unchanged, statusErr := store.Status()
	if statusErr != nil || unchanged.Revision != finished.Revision || unchanged.CumulativeChangedLines != 1 ||
		countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("rejected drift mutated authority: status=%#v err=%v records=%d", unchanged, statusErr, countRuntimeRecords(t, store.Dir))
	}

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), finishBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	accepted, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ActiveAttempt == nil || accepted.ActiveAttempt.Ordinal != 2 || replayed.Revision != accepted.Revision ||
		countRuntimeRecords(t, store.Dir) != beforeRecords+1 {
		t.Fatalf("restored begin/replay = accepted %#v replayed %#v records=%d", accepted, replayed, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeLedgerRejectsWorkUnitChangesWithinAnObjective(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "work-unit-continuity")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "work-unit-begin-1", WorkUnit: "stable-work-unit",
		EvidenceGoal: "prove immutable provenance", MaxAttempts: 3, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "work-unit-finish-1", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "first provenance attempt failed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "provenance cleanup completed",
		ProcessEvidence: "provenance process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)
	request := BeginAttemptRequest{
		ExpectedRevision: finished.Revision, RequestID: "work-unit-begin-2", WorkUnit: "renamed-work-unit",
		EvidenceGoal: "prove immutable provenance", MaxAttempts: 3, MaxChangedLines: 20,
	}
	_, err = store.Begin(context.Background(), request)
	if !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("changed work-unit error = %T %v, want ErrRuntimeObjectiveChange", err, err)
	}
	unchanged, statusErr := store.Status()
	if statusErr != nil || unchanged.Revision != finished.Revision || unchanged.Objective == nil ||
		unchanged.Objective.WorkUnit != "stable-work-unit" || countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("changed work-unit mutated authority: status=%#v err=%v records=%d", unchanged, statusErr, countRuntimeRecords(t, store.Dir))
	}

	request.WorkUnit = "stable-work-unit"
	accepted, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Objective == nil || accepted.Objective.WorkUnit != "stable-work-unit" || accepted.ActiveAttempt == nil ||
		accepted.ActiveAttempt.Ordinal != 2 {
		t.Fatalf("corrected work-unit retry = %#v", accepted)
	}
}

func TestRuntimeLedgerCASAllowsOnlyOneConcurrentOrdinal(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "concurrent-change")
	if err != nil {
		t.Fatal(err)
	}

	const writers = 24
	start := make(chan struct{})
	var successes atomic.Int32
	var unexpectedMu sync.Mutex
	var unexpected []error
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, runErr := store.Begin(context.Background(), BeginAttemptRequest{
				ExpectedRevision: "", RequestID: fmt.Sprintf("begin-%d", index), WorkUnit: "same-objective",
				EvidenceGoal: "prove one launch", MaxAttempts: 2, MaxChangedLines: 10,
			})
			if runErr == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(runErr, ErrRuntimeRevisionConflict) && !errors.Is(runErr, ErrRuntimeConcurrentUpdate) {
				unexpectedMu.Lock()
				unexpected = append(unexpected, runErr)
				unexpectedMu.Unlock()
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if successes.Load() != 1 || len(unexpected) != 0 {
		t.Fatalf("concurrent writers: successes=%d unexpected=%v", successes.Load(), unexpected)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.CumulativeAttempts != 1 || status.ActiveAttempt == nil || status.ActiveAttempt.Ordinal != 1 || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("concurrent final status = %#v, records=%d", status, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeLedgerPersistentMissingLockPathPreservesNotExist(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "missing-lock-path-change")
	if err != nil {
		t.Fatal(err)
	}

	originalAcquire := runtimeAcquireAuthorityFileLock
	var calls int
	runtimeAcquireAuthorityFileLock = func(string) (*reviewtransaction.AuthorityFileLock, error) {
		calls++
		return nil, fmt.Errorf("review store lock could not be acquired: %w", os.ErrNotExist)
	}
	t.Cleanup(func() { runtimeAcquireAuthorityFileLock = originalAcquire })

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-missing-lock-path", WorkUnit: "same-objective",
		EvidenceGoal: "preserve persistent filesystem failure", MaxAttempts: 2, MaxChangedLines: 10,
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persistent missing lock path error = %T %v, want os.ErrNotExist", err, err)
	}
	if errors.Is(err, ErrRuntimeConcurrentUpdate) {
		t.Fatalf("persistent missing lock path error = %T %v, must not be ErrRuntimeConcurrentUpdate", err, err)
	}
	if calls != runtimeLockAcquireAttempts {
		t.Fatalf("persistent missing lock path acquisition calls = %d, want %d", calls, runtimeLockAcquireAttempts)
	}
}

func TestRuntimeLedgerTransientMissingLockPathRetriesSuccessfully(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "transient-lock-path-change")
	if err != nil {
		t.Fatal(err)
	}

	originalAcquire := runtimeAcquireAuthorityFileLock
	var calls int
	runtimeAcquireAuthorityFileLock = func(path string) (*reviewtransaction.AuthorityFileLock, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("review store lock could not be acquired: %w", os.ErrNotExist)
		}
		return originalAcquire(path)
	}
	t.Cleanup(func() { runtimeAcquireAuthorityFileLock = originalAcquire })

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-transient-lock-path", WorkUnit: "same-objective",
		EvidenceGoal: "recover from transient initialization race", MaxAttempts: 2, MaxChangedLines: 10,
	})
	if err != nil {
		t.Fatalf("begin after transient missing lock path: %v", err)
	}
	if calls != 2 {
		t.Fatalf("transient missing lock path acquisition calls = %d, want 2", calls)
	}
}

func TestRuntimeLedgerMissingLockPathThenHeldLockIsConcurrentUpdate(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "held-lock-after-missing-path-change")
	if err != nil {
		t.Fatal(err)
	}

	originalAcquire := runtimeAcquireAuthorityFileLock
	var calls int
	runtimeAcquireAuthorityFileLock = func(string) (*reviewtransaction.AuthorityFileLock, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("review store lock could not be acquired: %w", os.ErrNotExist)
		}
		return nil, reviewtransaction.ErrConcurrentUpdate
	}
	t.Cleanup(func() { runtimeAcquireAuthorityFileLock = originalAcquire })

	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-held-lock-after-missing-path", WorkUnit: "same-objective",
		EvidenceGoal: "classify an observed held lock", MaxAttempts: 2, MaxChangedLines: 10,
	})
	if !errors.Is(err, ErrRuntimeConcurrentUpdate) {
		t.Fatalf("held lock after missing path error = %T %v, want ErrRuntimeConcurrentUpdate", err, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("held lock after missing path error = %T %v, must not be os.ErrNotExist", err, err)
	}
	if calls != 2 {
		t.Fatalf("held lock after missing path acquisition calls = %d, want 2", calls)
	}
}

func TestRuntimeLedgerCrashWindowsUseImmutableRecordReplay(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "durable-change")
	if err != nil {
		t.Fatal(err)
	}
	request := BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "durable-begin", WorkUnit: "fault-window",
		EvidenceGoal: "prove durable launch charge", MaxAttempts: 2, MaxChangedLines: 8,
	}

	originalReplace := runtimeReplaceHead
	var failReplace atomic.Bool
	failReplace.Store(true)
	runtimeReplaceHead = func(source, destination string) error {
		if failReplace.CompareAndSwap(true, false) {
			return errors.New("simulated HEAD rename failure")
		}
		return originalReplace(source, destination)
	}
	t.Cleanup(func() { runtimeReplaceHead = originalReplace })

	if _, err := store.Begin(context.Background(), request); err == nil || !strings.Contains(err.Error(), "HEAD rename failure") {
		t.Fatalf("pre-HEAD crash error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(err) {
		t.Fatalf("pre-HEAD failure published HEAD: %v", err)
	}
	if countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("pre-HEAD orphan count = %d, want 1", countRuntimeRecords(t, store.Dir))
	}

	committed, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if committed.CumulativeAttempts != 1 || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("orphan replay = %#v records=%d", committed, countRuntimeRecords(t, store.Dir))
	}

	// A directory-sync failure after HEAD publication reports a committed,
	// exact-replay-safe outcome. Retrying the same request does not append.
	appendRuntimeLedgerFile(t, repo, "post-head\n")
	finish := FinishAttemptRequest{
		ExpectedRevision: committed.Revision, RequestID: "durable-finish", Outcome: AttemptFailed,
		EvidenceRevision: runtimeTestHash('3'), Diagnosis: "fault injection reached the durability boundary",
		HarnessDisposition: HarnessReused, CleanupEvidence: "fault harness cleanup completed before publication",
		ProcessEvidence: "fault harness reported no surviving process",
	}
	originalSync := runtimeSyncDirectory
	var storeSyncFailures atomic.Int32
	runtimeSyncDirectory = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(store.Dir) && storeSyncFailures.Add(1) == 1 {
			return errors.New("simulated store directory sync failure")
		}
		return originalSync(path)
	}
	t.Cleanup(func() { runtimeSyncDirectory = originalSync })

	_, err = store.Finish(context.Background(), finish)
	var publication *RuntimePublicationError
	if !errors.As(err, &publication) || publication.Revision == "" || !publication.Committed {
		t.Fatalf("post-HEAD sync error = %T %v, want committed RuntimePublicationError", err, err)
	}
	afterFailure, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Revision != publication.Revision || afterFailure.CumulativeChangedLines != 1 {
		t.Fatalf("post-HEAD authoritative state = %#v, publication=%#v", afterFailure, publication)
	}
	replayed, err := store.Finish(context.Background(), finish)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != publication.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("post-HEAD replay = %#v records=%d", replayed, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeLedgerRejectsStaleCASBeforePublicationAndCorruptChain(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "strict-chain")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-current", WorkUnit: "strict",
		EvidenceGoal: "prove strict chain", MaxAttempts: 2, MaxChangedLines: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := countRuntimeRecords(t, store.Dir)
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-stale", WorkUnit: "strict",
		EvidenceGoal: "prove strict chain", MaxAttempts: 2, MaxChangedLines: 5,
	})
	var conflict *RuntimeRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Expected != "" || conflict.Current != first.Revision {
		t.Fatalf("stale CAS error = %T %#v", err, err)
	}
	if countRuntimeRecords(t, store.Dir) != before {
		t.Fatal("stale CAS published an immutable record")
	}
	// The error already prints the current revision; it must also say how to
	// use it, or a caller who only sees the string has no route back in.
	wantRetry := fmt.Sprintf("--expected-revision %q", first.Revision)
	if !strings.Contains(conflict.Error(), wantRetry) {
		t.Fatalf("revision conflict error = %q, want it to name %q", conflict.Error(), wantRetry)
	}

	recordPath := filepath.Join(store.Dir, "records", strings.TrimPrefix(first.Revision, "sha256:")+".json")
	payload, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)/2] ^= 1
	if err := os.WriteFile(recordPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Status(); err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("tampered immutable record error = %v", err)
	}
}

func TestRuntimeLedgerLockCannotBeRedirectedThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ordinary Windows test accounts cannot reliably create symlinks")
	}
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "no-follow-lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	const sentinel = "must remain unchanged\n"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(store.Dir, "LOCK")); err != nil {
		t.Fatal(err)
	}
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "symlink-begin", WorkUnit: "strict-lock",
		EvidenceGoal: "prove lock path identity", MaxAttempts: 2, MaxChangedLines: 5,
	})
	if err == nil {
		t.Fatal("runtime begin followed a symlinked authority lock")
	}
	payload, readErr := os.ReadFile(target)
	if readErr != nil || string(payload) != sentinel {
		t.Fatalf("symlink target changed: payload=%q err=%v", payload, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked lock attempt published HEAD: %v", statErr)
	}
}

func TestRuntimeLedgerPrivateRootCannotBeRedirectedThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ordinary Windows test accounts cannot reliably create symlinks")
	}
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "no-follow-root")
	if err != nil {
		t.Fatal(err)
	}
	gentleRoot := filepath.Join(store.commonDir, "gentle-ai")
	if err := os.MkdirAll(gentleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	redirect := t.TempDir()
	if err := os.Symlink(redirect, filepath.Join(gentleRoot, "sdd-runtime")); err != nil {
		t.Fatal(err)
	}
	_, err = store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "redirect-begin", WorkUnit: "strict-root",
		EvidenceGoal: "prove private authority root", MaxAttempts: 2, MaxChangedLines: 5,
	})
	if err == nil {
		t.Fatal("runtime begin followed a symlinked private authority root")
	}
	entries, readErr := os.ReadDir(redirect)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("redirected authority target was mutated: entries=%v err=%v", entries, readErr)
	}
}

func initRuntimeLedgerRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runRuntimeLedgerGit(t, repo, "init", "-q")
	runRuntimeLedgerGit(t, repo, "config", "user.email", "runtime-ledger@example.com")
	runRuntimeLedgerGit(t, repo, "config", "user.name", "Runtime Ledger Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", "tracked.txt")
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "base")
	return repo
}

func appendRuntimeLedgerFile(t *testing.T, repo, content string) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(repo, "tracked.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func runRuntimeLedgerGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func countRuntimeRecords(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "records"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

func runtimeTestHash(char byte) string {
	return "sha256:" + strings.Repeat(string(char), 64)
}
