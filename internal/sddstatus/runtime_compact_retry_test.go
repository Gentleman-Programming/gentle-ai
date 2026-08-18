package sddstatus

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

type compactAcquireOutcome struct {
	result CompactAttemptResult
	err    error
}

func retryStore(t *testing.T) (context.Context, RuntimeStore, itemPlanCandidate) {
	t.Helper()
	ctx, store, plan := concurrentStore(t)
	store.acquireRetryTimeout, store.acquireRetryPoll = 250*time.Millisecond, 10*time.Millisecond
	return ctx, store, plan
}

func holdRuntimeLock(t *testing.T, store RuntimeStore) *reviewtransaction.AuthorityFileLock {
	t.Helper()
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	lock, err := reviewtransaction.AcquireAuthorityFileLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func TestCompactAcquireRetriesHeldAuthorityLock(t *testing.T) {
	ctx, store, plan := retryStore(t)
	lock := holdRuntimeLock(t, store)
	request := CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", "wait")}
	result := make(chan compactAcquireOutcome, 1)
	go func() { got, err := store.Acquire(ctx, request); result <- compactAcquireOutcome{got, err} }()
	time.Sleep(30 * time.Millisecond)
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err != nil || got.result.State != CompactStateProceed || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("retried acquire=%#v records=%d", got, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireRetryDeadlineAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
	}{{"deadline", false}, {"cancel", true}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store, plan := retryStore(t)
			store.acquireRetryTimeout, store.acquireRetryPoll = 60*time.Millisecond, 20*time.Millisecond
			lock := holdRuntimeLock(t, store)
			defer lock.Release()
			if tc.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				go func() { time.Sleep(25 * time.Millisecond); cancel() }()
			}
			started := time.Now()
			result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", tc.name)})
			elapsed := time.Since(started)
			if countRuntimeRecords(t, store.Dir) != 0 {
				t.Fatal("contended acquire published")
			}
			if tc.cancel {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel=%#v %v", result, err)
				}
			} else if err != nil || result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation || elapsed < 50*time.Millisecond || elapsed > 180*time.Millisecond {
				t.Fatalf("deadline=%#v %v elapsed=%s", result, err, elapsed)
			}
		})
	}
}

func TestConcurrentDisjointAcquireOriginalCalls(t *testing.T) {
	for run := 0; run < 3; run++ {
		ctx, store, plan := retryStore(t)
		lock := holdRuntimeLock(t, store)
		defer lock.Release()
		start := make(chan struct{})
		results := make(chan compactAcquireOutcome, 2)
		contended := make(chan struct{}, 2)
		var observed [2]int
		var ready sync.WaitGroup
		ready.Add(2)
		for index, item := range []string{"a", "b"} {
			attemptStore := store
			var first sync.Once
			attemptStore.acquireAttemptObserved = func(err error) {
				observed[index]++
				if errors.Is(err, reviewtransaction.ErrStoreLockContended) {
					first.Do(func() { contended <- struct{}{} })
				}
			}
			request := CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, item, "disjoint-"+item)}
			go func(store RuntimeStore) {
				ready.Done()
				<-start
				result, err := store.Acquire(ctx, request)
				results <- compactAcquireOutcome{result, err}
			}(attemptStore)
		}
		ready.Wait()
		close(start)
		for range 2 {
			<-contended
		}
		if err := lock.Release(); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if got := <-results; got.err != nil || got.result.State != CompactStateProceed {
				t.Fatalf("run=%d acquire=%#v", run, got)
			}
		}
		status, err := store.Status()
		if err != nil || observed[0] < 2 || observed[1] < 2 || status.runtimeActiveCount() != 2 || countRuntimeRecords(t, store.Dir) != 2 {
			t.Fatalf("run=%d observed=%v status=%#v %v", run, observed, status, err)
		}
	}
}

func TestConcurrentOverlappingAcquireReloadsSemanticRefusal(t *testing.T) {
	ctx, store, plan := retryStore(t)
	lock := holdRuntimeLock(t, store)
	defer lock.Release()
	start := make(chan struct{})
	results := make(chan compactAcquireOutcome, 2)
	contended := make(chan struct{}, 2)
	var observed [2]int
	for index, id := range []string{"one", "two"} {
		attemptStore := store
		var first sync.Once
		attemptStore.acquireAttemptObserved = func(err error) {
			observed[index]++
			if errors.Is(err, reviewtransaction.ErrStoreLockContended) {
				first.Do(func() { contended <- struct{}{} })
			}
		}
		request := CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", id)}
		go func(store RuntimeStore) {
			<-start
			result, err := store.Acquire(ctx, request)
			results <- compactAcquireOutcome{result, err}
		}(attemptStore)
	}
	close(start)
	for range 2 {
		<-contended
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	proceed, blocked := 0, 0
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.result.State == CompactStateProceed {
			proceed++
		} else if got.result.Reason == CompactBlockActiveAttempt {
			blocked++
		} else {
			t.Fatalf("overlap=%#v", got)
		}
	}
	if proceed != 1 || blocked != 1 || observed[0] < 2 || observed[1] < 2 || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("proceed=%d blocked=%d observed=%v records=%d", proceed, blocked, observed, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireContentionReplaysSameRequest(t *testing.T) {
	ctx, store, plan := retryStore(t)
	lock := holdRuntimeLock(t, store)
	request := CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", "same-request")}
	results := make(chan compactAcquireOutcome, 2)
	for range 2 {
		go func() { result, err := store.Acquire(ctx, request); results <- compactAcquireOutcome{result, err} }()
	}
	time.Sleep(30 * time.Millisecond)
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.result != second.result || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("first=%#v second=%#v records=%d", first, second, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireDoesNotRetryNonTransientFailures(t *testing.T) {
	ctx, store, _ := retryStore(t)
	store.acquireRetryPoll = 100 * time.Millisecond
	broken := store
	broken.Repo = filepath.Join(store.Repo, "missing")
	started := time.Now()
	result, err := broken.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{RequestID: "candidate", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 1}})
	if err != nil || result.Reason != CompactBlockCandidateUnavailable || time.Since(started) >= store.acquireRetryPoll || countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatalf("candidate=%#v err=%v", result, err)
	}
	first, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{RequestID: "conflict", WorkUnit: "a", EvidenceGoal: "a", MaxAttempts: 1, MaxChangedLines: 1}})
	if err != nil || first.State != CompactStateProceed {
		t.Fatalf("first=%#v %v", first, err)
	}
	before := countRuntimeRecords(t, store.Dir)
	started = time.Now()
	result, err = store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{RequestID: "conflict", WorkUnit: "other", EvidenceGoal: "other", MaxAttempts: 1, MaxChangedLines: 1}})
	if err != nil || result.Reason != CompactBlockInvalidContinuation || time.Since(started) >= store.acquireRetryPoll || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("request conflict=%#v err=%v records=%d", result, err, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireNonTransientClassifierRunsOnce(t *testing.T) {
	ctx, store, plan := retryStore(t)
	store.acquireRetryPoll = 100 * time.Millisecond
	observed := 0
	store.acquireAttemptObserved = func(error) { observed++ }
	concurrentAcquire(t, ctx, store, plan, "a", "owner")
	observed = 0
	before := countRuntimeRecords(t, store.Dir)
	started := time.Now()
	result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", "overlap")})
	if err != nil || result.Reason != CompactBlockActiveAttempt || observed != 1 || time.Since(started) >= store.acquireRetryPoll || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("semantic refusal result=%#v err=%v observed=%d records=%d", result, err, observed, countRuntimeRecords(t, store.Dir))
	}

	// A request-id digest conflict is a public non-lock concurrent-update
	// equivalent: it reaches compactMutationFailure without retrying.
	observed = 0
	conflict := runtimePlanRequest(t, store, plan, "a", "owner")
	conflict.EvidenceGoal = "changed"
	result, err = store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: conflict})
	if err != nil || result.Reason != CompactBlockInvalidContinuation || observed != 1 || countRuntimeRecords(t, store.Dir) != before {
		t.Fatalf("request conflict result=%#v err=%v observed=%d records=%d", result, err, observed, countRuntimeRecords(t, store.Dir))
	}
}

func TestCompactAcquireDoesNotRetryBareConcurrentUpdate(t *testing.T) {
	ctx, store, plan := retryStore(t)
	store.acquireRetryPoll = 100 * time.Millisecond
	original := runtimeAcquireAuthorityFileLock
	t.Cleanup(func() { runtimeAcquireAuthorityFileLock = original })
	var observed []error
	store.acquireAttemptObserved = func(err error) { observed = append(observed, err) }
	runtimeAcquireAuthorityFileLock = func(string) (*reviewtransaction.AuthorityFileLock, error) {
		return nil, reviewtransaction.ErrConcurrentUpdate
	}

	started := time.Now()
	result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: runtimePlanRequest(t, store, plan, "a", "bare-concurrent-update")})
	if err != nil || len(observed) != 1 || !errors.Is(observed[0], ErrRuntimeConcurrentUpdate) ||
		errors.Is(observed[0], reviewtransaction.ErrStoreLockContended) || result.State != CompactStateBlocked ||
		result.Reason != CompactBlockInvalidContinuation || time.Since(started) >= store.acquireRetryPoll || countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatalf("bare concurrent update result=%#v err=%v observed=%v", result, err, observed)
	}
	// Committed-publication recovery remains covered by
	// TestRuntimeLedgerCrashWindowsUseImmutableRecordReplay; its global sync seam
	// is intentionally not duplicated at the compact boundary here.
}
