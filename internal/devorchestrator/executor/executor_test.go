package executor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
)

// MockRunner is a simple mock for testing the concurrent engine.
type MockRunner struct {
	mu            sync.Mutex
	CallCount     int
	Delay         time.Duration
	InjectedError error
}

func (m *MockRunner) Run(ctx context.Context, b batch.ExecutionBatch, prompt string) error {
	// Simulate work delay
	time.Sleep(m.Delay)

	m.mu.Lock()
	m.CallCount++
	m.mu.Unlock()

	// Return error for a specific repo to test error mapping,
	// or return the globally injected error.
	if b.RepoName == "fail-repo" {
		return errors.New("simulated failure")
	}

	return m.InjectedError
}

func TestConcurrentEngine_ExecuteBatches_Parallelism(t *testing.T) {
	runner := &MockRunner{
		Delay: 100 * time.Millisecond,
	}

	// 3 workers means 3 batches can run perfectly in parallel
	engine := New(runner, 3)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-3", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{
		"repo-1": "prompt-1",
		"repo-2": "prompt-2",
		"repo-3": "prompt-3",
	}

	start := time.Now()
	errs, _ := engine.ExecuteBatches(context.Background(), batches, prompts)
	duration := time.Since(start)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if runner.CallCount != 3 {
		t.Errorf("expected 3 calls, got %d", runner.CallCount)
	}

	// If it ran sequentially, it would take ~300ms.
	// Running concurrently, it should take ~100ms.
	// We allow some overhead margin.
	if duration >= 250*time.Millisecond {
		t.Errorf("execution was too slow (%v), expected it to run in parallel", duration)
	}
}

func TestConcurrentEngine_ExecuteBatches_LimitsConcurrency(t *testing.T) {
	runner := &MockRunner{
		Delay: 100 * time.Millisecond,
	}

	// 1 worker means it has to run sequentially
	engine := New(runner, 1)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{
		"repo-1": "prompt-1",
		"repo-2": "prompt-2",
	}

	start := time.Now()
	engine.ExecuteBatches(context.Background(), batches, prompts)
	duration := time.Since(start)

	// Since maxWorkers is 1, it must take at least 200ms
	if duration < 200*time.Millisecond {
		t.Errorf("execution was too fast (%v), expected it to run sequentially", duration)
	}
}

func TestConcurrentEngine_ExecuteBatches_IgnoresNotReady(t *testing.T) {
	runner := &MockRunner{
		Delay: 10 * time.Millisecond,
	}

	engine := New(runner, 5)

	batches := []batch.ExecutionBatch{
		{RepoName: "repo-1", AgentName: "agent-a", Ready: true},
		{RepoName: "repo-2", AgentName: "agent-a", Ready: false}, // Should be ignored
		{RepoName: "repo-3", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{}

	errs, _ := engine.ExecuteBatches(context.Background(), batches, prompts)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if runner.CallCount != 2 {
		t.Errorf("expected 2 calls, got %d", runner.CallCount)
	}
}

func TestConcurrentEngine_ExecuteBatches_CollectsErrors(t *testing.T) {
	runner := &MockRunner{
		Delay: 10 * time.Millisecond,
	}

	engine := New(runner, 2)

	batches := []batch.ExecutionBatch{
		{RepoName: "good-repo", AgentName: "agent-a", Ready: true},
		{RepoName: "fail-repo", AgentName: "agent-a", Ready: true},
	}
	prompts := map[string]string{}

	errs, _ := engine.ExecuteBatches(context.Background(), batches, prompts)

	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d", len(errs))
	}

	err, ok := errs["fail-repo"]
	if !ok {
		t.Fatalf("expected error for 'fail-repo', got none")
	}

	expectedPrefix := "agent agent-a failed on repo \"fail-repo\""
	if err.Error()[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("unexpected error message: %v", err)
	}
}

// FlakyRunner fails the first FailCount calls for a given RepoName, then
// succeeds on every call after that -- used to prove bounded retry recovers
// a transient failure (H-09a).
type FlakyRunner struct {
	mu         sync.Mutex
	FailCount  int
	CallCounts map[string]int
}

func (r *FlakyRunner) Run(_ context.Context, b batch.ExecutionBatch, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CallCounts == nil {
		r.CallCounts = make(map[string]int)
	}
	r.CallCounts[b.RepoName]++
	if r.CallCounts[b.RepoName] <= r.FailCount {
		return errors.New("transient failure")
	}
	return nil
}

// AlwaysFailRunner fails every call, unconditionally.
type AlwaysFailRunner struct {
	mu        sync.Mutex
	CallCount int
}

func (r *AlwaysFailRunner) Run(_ context.Context, _ batch.ExecutionBatch, _ string) error {
	r.mu.Lock()
	r.CallCount++
	r.mu.Unlock()
	return errors.New("permanent failure")
}

// TestConcurrentEngine_ExecuteBatches_RetriesTransientFailureThenSucceeds
// covers H-09a: a dispatch failing once then succeeding within the retry
// bound is recorded succeeded, with the attempt count never exceeding the
// bound. The injected Sleep never actually waits.
func TestConcurrentEngine_ExecuteBatches_RetriesTransientFailureThenSucceeds(t *testing.T) {
	runner := &FlakyRunner{FailCount: 1}
	engine := New(runner, 1)
	engine.MaxAttempts = 3
	engine.BaseBackoff = 10 * time.Millisecond
	var sleepCalls []time.Duration
	engine.Sleep = func(d time.Duration) { sleepCalls = append(sleepCalls, d) }

	batches := []batch.ExecutionBatch{{RepoName: "flaky-repo", AgentName: "agent-a", Ready: true}}
	errs, attempts := engine.ExecuteBatches(context.Background(), batches, map[string]string{})

	if len(errs) != 0 {
		t.Fatalf("expected no errors after recovering within the retry bound, got %v", errs)
	}
	if got := attempts["flaky-repo"]; got != 2 {
		t.Errorf("attempts[flaky-repo] = %d, want 2 (one failure then one success)", got)
	}
	if got := attempts["flaky-repo"]; got > engine.MaxAttempts {
		t.Errorf("attempts[flaky-repo] = %d, exceeded configured MaxAttempts %d", got, engine.MaxAttempts)
	}
	if len(sleepCalls) == 0 {
		t.Error("expected the injected backoff Sleep to be called at least once between attempts")
	}
}

// TestConcurrentEngine_ExecuteBatches_ExhaustsRetriesRecordsFailure covers
// H-09a: a dispatch failing every attempt up to the bound is recorded
// failed, called exactly MaxAttempts times (no circuit breaker cutting it
// short, no extra attempt beyond the bound; H-09b's rollback is out of scope).
func TestConcurrentEngine_ExecuteBatches_ExhaustsRetriesRecordsFailure(t *testing.T) {
	runner := &AlwaysFailRunner{}
	engine := New(runner, 1)
	engine.MaxAttempts = 3
	engine.BaseBackoff = 10 * time.Millisecond
	engine.Sleep = func(time.Duration) {} // no-op: never actually wait in tests

	batches := []batch.ExecutionBatch{{RepoName: "doomed-repo", AgentName: "agent-a", Ready: true}}
	errs, attempts := engine.ExecuteBatches(context.Background(), batches, map[string]string{})

	if _, failed := errs["doomed-repo"]; !failed {
		t.Fatalf("expected doomed-repo to be recorded as failed, got errs = %v", errs)
	}
	if got := attempts["doomed-repo"]; got != engine.MaxAttempts {
		t.Errorf("attempts[doomed-repo] = %d, want exactly MaxAttempts (%d)", got, engine.MaxAttempts)
	}
	if runner.CallCount != engine.MaxAttempts {
		t.Errorf("runner called %d times, want exactly MaxAttempts (%d) -- no circuit breaker should cut retries short", runner.CallCount, engine.MaxAttempts)
	}
	if len(errs) != 1 {
		t.Errorf("expected exactly 1 failed repo recorded, got %d: %v", len(errs), errs)
	}
}

// TestConcurrentEngine_ExecuteBatches_ZeroMaxAttemptsPreservesSingleAttempt
// proves MaxAttempts <= 0 (the zero value returned by New) preserves the
// exact pre-H-09a behavior of a single, non-retried attempt per repo.
func TestConcurrentEngine_ExecuteBatches_ZeroMaxAttemptsPreservesSingleAttempt(t *testing.T) {
	runner := &AlwaysFailRunner{}
	engine := New(runner, 1) // MaxAttempts left at its zero value

	batches := []batch.ExecutionBatch{{RepoName: "doomed-repo", AgentName: "agent-a", Ready: true}}
	errs, attempts := engine.ExecuteBatches(context.Background(), batches, map[string]string{})

	if _, failed := errs["doomed-repo"]; !failed {
		t.Fatalf("expected doomed-repo to be recorded as failed, got errs = %v", errs)
	}
	if got := attempts["doomed-repo"]; got != 1 {
		t.Errorf("attempts[doomed-repo] = %d, want exactly 1 (MaxAttempts<=0 preserves single-attempt behavior)", got)
	}
	if runner.CallCount != 1 {
		t.Errorf("runner called %d times, want exactly 1", runner.CallCount)
	}
}
